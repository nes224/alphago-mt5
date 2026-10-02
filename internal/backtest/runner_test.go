package backtest_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type fireOnceStrategy struct {
	fireOnCall int
	calls      int
}

func (s *fireOnceStrategy) ID() string { return "TEST_STRATEGY" }

func (s *fireOnceStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	call := s.calls
	s.calls++
	if call != s.fireOnCall {
		return nil
	}
	return &domain.OrderSignal{
		Symbol:    tick.Symbol,
		Action:    domain.SignalAction(domain.ActionBuy),
		Price:     tick.Ask,
		Reason:    "TEST_STRATEGY (fired)",
		Timestamp: tick.Timestamp,
	}
}

// warmupTicks feeds n ticks oscillating slightly around 4000 (small nonzero
// StdDev/SizingVolatility, but comfortably under RiskManager's floor-clamped
// SL distance of 3.0 once multiplied) so CalculateOrder doesn't reject the
// eventual signal with "zero volatility".
func warmupTicks(r *backtest.Runner, symbol string, base time.Time, n int) {
	price := 4000.0
	for i := 0; i < n; i++ {
		if i%2 == 0 {
			price = 4000.0
		} else {
			price = 4000.4
		}
		r.OnTick(domain.Tick{Symbol: symbol, Bid: price, Ask: price + 0.2, Volume: 10, Timestamp: base.Add(time.Duration(i) * time.Second)})
	}
}

// testConfig is DefaultConfig with the two always-registered production
// strategies neutralized via their own existing thresholds (not a special
// test-only Runner API): LiquiditySweepStrategy's trend filter at threshold
// 0 always reads "market is trending" and suppresses every fade signal;
// VolumeExpansionStrategy is already blocked by warmupTicks' constant
// Volume (VolumeDelta=0 fails its required VolumeDelta>0 check). This keeps
// these tests focused on Runner's own open/close/exit/RiskGuard wiring
// instead of also having to dodge the real strategies firing on
// hand-crafted price data meant to exercise something else entirely.
func testConfig() backtest.Config {
	cfg := backtest.DefaultConfig("XAUUSDm", 10000.0)
	cfg.SweepTrendSlopeThreshold = 0
	cfg.SignalCooldown = 0
	return cfg
}

func newTestRunner(fireOnCall int) (*backtest.Runner, *fireOnceStrategy) {
	r := backtest.NewRunner(testConfig())
	strat := &fireOnceStrategy{fireOnCall: fireOnCall}
	r.RegisterStrategy(strat)
	return r, strat
}

func TestRunner_StopLossTouch_RecordsLossTrade(t *testing.T) {
	r, _ := newTestRunner(25)
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC) // LONDON session
	symbol := "XAUUSDm"

	warmupTicks(r, symbol, base, 25) // calls 0..24

	// Call 25: fireOnceStrategy fires BUY. Entry fills at this tick's Ask.
	triggerTime := base.Add(25 * time.Second)
	r.OnTick(domain.Tick{Symbol: symbol, Bid: 4000.0, Ask: 4000.2, Volume: 10, Timestamp: triggerTime})

	report := r.Report()
	if report.StillOpenAtEnd != 1 {
		t.Fatalf("expected the position to still be open right after entry, got StillOpenAtEnd=%d", report.StillOpenAtEnd)
	}

	// EntryPrice=4000.2, SL distance floors at MinSLDistance=3.0 -> SL=3997.2.
	// Push price down through it.
	exitTime := triggerTime.Add(1 * time.Minute)
	r.OnTick(domain.Tick{Symbol: symbol, Bid: 3997.0, Ask: 3997.2, Volume: 10, Timestamp: exitTime})

	report = r.Report()
	if report.StillOpenAtEnd != 0 {
		t.Fatalf("expected position to be closed, got StillOpenAtEnd=%d", report.StillOpenAtEnd)
	}
	if len(report.Trades) != 1 {
		t.Fatalf("expected exactly 1 trade, got %d", len(report.Trades))
	}

	trade := report.Trades[0]
	if trade.IsWin {
		t.Error("expected a LOSS trade")
	}
	if got := trade.ExitPrice; got < 3997.2-1e-6 || got > 3997.2+1e-6 {
		t.Errorf("expected exit price 3997.2 (StopLoss), got %f", got)
	}
	if got := trade.PnL; got < -15.0-1e-6 || got > -15.0+1e-6 {
		t.Errorf("expected PnL -15.0 ((3997.2-4000.2)*0.05 lot*100 contract), got %f", got)
	}
	if trade.StrategyTag != "TEST_STRATEGY" {
		t.Errorf("expected StrategyTag 'TEST_STRATEGY' (extracted before ' ('), got %q", trade.StrategyTag)
	}
	if trade.Session != domain.SessionLondon {
		t.Errorf("expected session LONDON for 08:00 UTC, got %s", trade.Session)
	}
	if report.FinalBalance != report.InitialBalance+trade.PnL {
		t.Errorf("expected FinalBalance to reflect the trade PnL, got %f (initial %f, pnl %f)", report.FinalBalance, report.InitialBalance, trade.PnL)
	}
}

func TestRunner_TakeProfitTouch_RecordsWinTrade(t *testing.T) {
	r, _ := newTestRunner(25)
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	symbol := "XAUUSDm"

	warmupTicks(r, symbol, base, 25)
	triggerTime := base.Add(25 * time.Second)
	r.OnTick(domain.Tick{Symbol: symbol, Bid: 4000.0, Ask: 4000.2, Volume: 10, Timestamp: triggerTime})

	// TP = EntryPrice(4000.2) + 3.0 = 4003.2. Push price up through it.
	r.OnTick(domain.Tick{Symbol: symbol, Bid: 4003.5, Ask: 4003.7, Volume: 10, Timestamp: triggerTime.Add(1 * time.Minute)})

	report := r.Report()
	if len(report.Trades) != 1 {
		t.Fatalf("expected exactly 1 trade, got %d", len(report.Trades))
	}
	trade := report.Trades[0]
	if !trade.IsWin {
		t.Error("expected a WIN trade")
	}
	if got := trade.ExitPrice; got < 4003.2-1e-6 || got > 4003.2+1e-6 {
		t.Errorf("expected exit price 4003.2 (TakeProfit), got %f", got)
	}
	if got := trade.PnL; got < 15.0-1e-6 || got > 15.0+1e-6 {
		t.Errorf("expected PnL +15.0, got %f", got)
	}
}

func TestRunner_RejectsSecondSignalWhilePositionOpen(t *testing.T) {
	r := backtest.NewRunner(testConfig())
	// Fires on both call 25 and call 26 -- the second must be rejected by
	// RiskGuard's one-position-per-symbol rule, not opened as a 2nd position.
	strat := &fireTwiceStrategy{fireOnCalls: map[int]bool{25: true, 26: true}}
	r.RegisterStrategy(strat)

	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)
	symbol := "XAUUSDm"
	warmupTicks(r, symbol, base, 25)
	r.OnTick(domain.Tick{Symbol: symbol, Bid: 4000.0, Ask: 4000.2, Volume: 10, Timestamp: base.Add(25 * time.Second)})
	r.OnTick(domain.Tick{Symbol: symbol, Bid: 4000.1, Ask: 4000.3, Volume: 10, Timestamp: base.Add(26 * time.Second)})

	report := r.Report()
	if report.StillOpenAtEnd != 1 {
		t.Fatalf("expected exactly 1 open position (second signal rejected), got %d", report.StillOpenAtEnd)
	}
	if len(report.Trades) != 0 {
		t.Fatalf("expected no closed trades yet, got %d", len(report.Trades))
	}
}

type fireTwiceStrategy struct {
	fireOnCalls map[int]bool
	calls       int
}

func (s *fireTwiceStrategy) ID() string { return "TEST_STRATEGY_TWICE" }

func (s *fireTwiceStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	call := s.calls
	s.calls++
	if !s.fireOnCalls[call] {
		return nil
	}
	return &domain.OrderSignal{
		Symbol:    tick.Symbol,
		Action:    domain.SignalAction(domain.ActionBuy),
		Price:     tick.Ask,
		Reason:    "TEST_STRATEGY_TWICE (fired)",
		Timestamp: tick.Timestamp,
	}
}

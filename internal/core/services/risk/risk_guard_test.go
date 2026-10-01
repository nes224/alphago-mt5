package risk_test

import (
	"errors"
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

func TestRiskGuard_DailyDrawdown_CircuitBreaker(t *testing.T) {
	cfg := risk.RiskGuardConfig{
		MaxDailyLossPercent: 0.03, // 3%
		MaxOpenPositions:    5,
		MaxSpreadPips:       1.0,
	}

	guard := risk.NewRiskGuard(cfg, 10000.0) // Start Equity 10,000 USD

	order := risk.PreparedOrder{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{Spread: 0.30}

	// 1. สภาวะปกติ: ควรยิงผ่าน
	if err := guard.ValidateOrder(order, metrics); err != nil {
		t.Fatalf("Expected order to pass, got error: %v", err)
	}

	// 2. Equity ร่วงเหลือ 9,600 USD (ลดลง 4% > เกณฑ์ 3%)
	guard.UpdateAccountEquity(9600.0)

	// 3. ต้องโดน Block ด้วย ErrDailyDrawdownExceeded
	err := guard.ValidateOrder(order, metrics)
	if err == nil {
		t.Fatal("Expected order to be blocked by Daily Drawdown, but passed")
	}
}

func TestRiskGuard_SpreadFilter(t *testing.T) {
	cfg := risk.RiskGuardConfig{
		MaxDailyLossPercent: 0.05,
		MaxOpenPositions:    5,
		MaxSpreadPips:       0.50, // Spread ห้ามเกิน $0.50
	}

	guard := risk.NewRiskGuard(cfg, 10000.0)
	order := risk.PreparedOrder{Symbol: "XAUUSDm"}
	metrics := domain.TickMetrics{Spread: 0.80} // Spread กว้างเกินไป (0.80)

	err := guard.ValidateOrder(order, metrics)
	if err == nil {
		t.Fatal("Expected order to be blocked due to high spread, but passed")
	}
}

func TestRiskGuard_ValidateOrder_BlocksOppositeSignalWhilePositionOpen(t *testing.T) {
	cfg := risk.RiskGuardConfig{MaxDailyLossPercent: 0.05, MaxOpenPositions: 5, MaxSpreadPips: 5.0}
	guard := risk.NewRiskGuard(cfg, 10000.0)
	metrics := domain.TickMetrics{Spread: 0.30}

	buyOrder := risk.PreparedOrder{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	if err := guard.ValidateOrder(buyOrder, metrics); err != nil {
		t.Fatalf("Expected first BUY to pass, got error: %v", err)
	}
	guard.MarkPositionOpened(buyOrder.Symbol)

	// A SELL signal on the same symbol while the BUY is still open must be
	// blocked — this is exactly the same-symbol BUY+SELL flip-flop bug seen live.
	sellOrder := risk.PreparedOrder{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionSell)}
	err := guard.ValidateOrder(sellOrder, metrics)
	if err == nil {
		t.Fatal("Expected SELL to be blocked while a position is already open on the same symbol")
	}
	if !errors.Is(err, risk.ErrPositionAlreadyOpen) {
		t.Errorf("Expected ErrPositionAlreadyOpen, got: %v", err)
	}

	// A different symbol must be unaffected.
	otherSymbolOrder := risk.PreparedOrder{Symbol: "EURUSD", Action: domain.SignalAction(domain.ActionBuy)}
	if err := guard.ValidateOrder(otherSymbolOrder, metrics); err != nil {
		t.Errorf("Expected order on a different symbol to pass, got error: %v", err)
	}

	// Once the position closes, the same symbol should be tradeable again.
	guard.MarkPositionClosed(buyOrder.Symbol)
	if err := guard.ValidateOrder(sellOrder, metrics); err != nil {
		t.Errorf("Expected order to pass after position closed, got error: %v", err)
	}
}

// TestRiskGuard_ResyncPositions_FixesStuckCount reproduces the live bug found
// 2026-10-01: openPositionsCount drifted to 5 (blocking every new signal)
// while MT5 actually had only 1 real open position — this happened because
// openPositionSymbols used to be RAM-only and reset to empty on every
// restart, while the persisted scalar count kept accumulating across those
// restarts without ever being decremented back down.
func TestRiskGuard_ResyncPositions_FixesStuckCount(t *testing.T) {
	cfg := risk.RiskGuardConfig{MaxDailyLossPercent: 0.05, MaxOpenPositions: 5, MaxSpreadPips: 5.0}
	guard := risk.NewRiskGuard(cfg, 10000.0)
	metrics := domain.TickMetrics{Spread: 0.30}
	order := risk.PreparedOrder{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}

	// Simulate the stuck state: count says 5, but reality (MT5) has only 1.
	for i := 0; i < 5; i++ {
		guard.MarkPositionOpened("XAUUSDm")
	}
	if err := guard.ValidateOrder(order, metrics); !errors.Is(err, risk.ErrMaxPositionsReached) {
		t.Fatalf("Expected ErrMaxPositionsReached before resync, got: %v", err)
	}

	guard.ResyncPositions(map[string]int{"XAUUSDm": 1})

	status := guard.Status()
	if status.OpenPositionsCount != 1 {
		t.Errorf("Expected OpenPositionsCount=1 after resync, got %d", status.OpenPositionsCount)
	}
	if status.OpenPositionSymbols["XAUUSDm"] != 1 {
		t.Errorf("Expected OpenPositionSymbols[XAUUSDm]=1 after resync, got %+v", status.OpenPositionSymbols)
	}

	// New signals on a different symbol must now pass sizing/count checks
	// again (same symbol is still correctly blocked, per the 1 open position).
	if err := guard.ValidateOrder(risk.PreparedOrder{Symbol: "EURUSD", Action: domain.SignalAction(domain.ActionBuy)}, metrics); err != nil {
		t.Errorf("Expected order on a different symbol to pass after resync, got error: %v", err)
	}
}

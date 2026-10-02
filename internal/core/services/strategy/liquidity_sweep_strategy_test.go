package strategy_test

import (
	"context"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

const testTrendSlopeThreshold = 1.0

func warmUpFlatRange(s *strategy.LiquiditySweepStrategy, symbol string, ask, bid float64, n int) {
	for i := 0; i < n; i++ {
		s.OnTick(domain.Tick{Symbol: symbol, Ask: ask, Bid: bid, Timestamp: time.Now()}, domain.TickMetrics{})
	}
}

func TestLiquiditySweepStrategy_BullishSweep_SweepLow_FiresBuy(t *testing.T) {
	s := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_TEST", "XAUUSDm", 5, testTrendSlopeThreshold, 0)
	warmUpFlatRange(s, "XAUUSDm", 2601.00, 2600.80, 5)

	sweepLowTick := domain.Tick{Symbol: "XAUUSDm", Ask: 2596.00, Bid: 2595.80, Timestamp: time.Now()}
	signal := s.OnTick(sweepLowTick, domain.TickMetrics{})

	if signal == nil {
		t.Fatal("Expected a BUY fade signal on Low sweep, got nil")
	}
	if signal.Action != domain.SignalAction(domain.ActionBuy) {
		t.Errorf("Expected BUY, got %s", signal.Action)
	}
	if signal.Price != sweepLowTick.Ask {
		t.Errorf("Expected signal price %.2f, got %.2f", sweepLowTick.Ask, signal.Price)
	}
}

func TestLiquiditySweepStrategy_BearishSweep_SweepHigh_FiresSell(t *testing.T) {
	s := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_TEST", "XAUUSDm", 5, testTrendSlopeThreshold, 0)
	warmUpFlatRange(s, "XAUUSDm", 2603.00, 2602.80, 5)

	sweepHighTick := domain.Tick{Symbol: "XAUUSDm", Ask: 2607.00, Bid: 2606.80, Timestamp: time.Now()}
	signal := s.OnTick(sweepHighTick, domain.TickMetrics{})

	if signal == nil {
		t.Fatal("Expected a SELL fade signal on High sweep, got nil")
	}
	if signal.Action != domain.SignalAction(domain.ActionSell) {
		t.Errorf("Expected SELL, got %s", signal.Action)
	}
	if signal.Price != sweepHighTick.Bid {
		t.Errorf("Expected signal price %.2f, got %.2f", sweepHighTick.Bid, signal.Price)
	}
}

func TestLiquiditySweepStrategy_NoSweep_ReturnsNil(t *testing.T) {
	s := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_TEST", "XAUUSDm", 5, testTrendSlopeThreshold, 0)
	warmUpFlatRange(s, "XAUUSDm", 2603.00, 2602.80, 5)

	// Stays well inside the warmed-up [2602.80, 2603.00] range -> no sweep.
	normalTick := domain.Tick{Symbol: "XAUUSDm", Ask: 2602.90, Bid: 2602.85, Timestamp: time.Now()}
	if signal := s.OnTick(normalTick, domain.TickMetrics{}); signal != nil {
		t.Fatalf("Expected nil for a tick within range, got: %+v", signal)
	}
}

func TestLiquiditySweepStrategy_SymbolMismatch_ReturnsNil(t *testing.T) {
	s := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_TEST", "XAUUSDm", 5, testTrendSlopeThreshold, 0)
	warmUpFlatRange(s, "XAUUSDm", 2603.00, 2602.80, 5)

	// Would be a clear High sweep for XAUUSDm, but this tick is for a different symbol.
	otherSymbolTick := domain.Tick{Symbol: "EURUSD", Ask: 2607.00, Bid: 2606.80, Timestamp: time.Now()}
	if signal := s.OnTick(otherSymbolTick, domain.TickMetrics{}); signal != nil {
		t.Fatalf("Expected nil for mismatched symbol, got: %+v", signal)
	}
}

func TestLiquiditySweepStrategy_QuantEngineIntegration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := strategy.NewQuantEngine(100, 5)
	sweepStrategy := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_TEST", "XAUUSDm", 5, testTrendSlopeThreshold, 0)
	engine.RegisterStrategy(sweepStrategy)

	engine.Start(ctx)

	now := time.Now()
	for i := 0; i < 5; i++ {
		engine.PushTick(domain.Tick{
			Symbol:    "XAUUSDm",
			Ask:       2603.00,
			Bid:       2602.80,
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
		})
	}
	time.Sleep(50 * time.Millisecond)

	engine.PushTick(domain.Tick{
		Symbol:    "XAUUSDm",
		Ask:       2607.00,
		Bid:       2606.80,
		Timestamp: now.Add(100 * time.Millisecond),
	})

	select {
	case signal := <-engine.SignalChannel():
		if signal.Symbol != "XAUUSDm" {
			t.Errorf("Expected symbol XAUUSDm, got %s", signal.Symbol)
		}
		if signal.Action != domain.SignalAction(domain.ActionSell) {
			t.Errorf("Expected SELL fade signal on High sweep, got %s", signal.Action)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout: expected engine to dispatch a fade signal via SignalChannel()")
	}
}

func TestLiquiditySweepStrategy_TrendingMarket_SuppressesFadeSignal(t *testing.T) {
	s := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_TEST", "XAUUSDm", 5, testTrendSlopeThreshold, 0)
	warmUpFlatRange(s, "XAUUSDm", 2603.00, 2602.80, 5)

	trendingMetrics := domain.TickMetrics{TrendSlope: testTrendSlopeThreshold * 2}
	sweepHighTick := domain.Tick{Symbol: "XAUUSDm", Ask: 2607.00, Bid: 2606.80, Timestamp: time.Now()}

	if signal := s.OnTick(sweepHighTick, trendingMetrics); signal != nil {
		t.Fatalf("Expected no fade signal while trending, got: %+v", signal)
	}
}

func TestWindow_Slope(t *testing.T) {
	flat := strategy.NewWindow(5)
	for i := 0; i < 5; i++ {
		flat.Push(100.0)
	}
	if slope := flat.Slope(); slope != 0 {
		t.Errorf("Expected flat prices to have slope 0, got %f", slope)
	}

	rising := strategy.NewWindow(5)
	for _, v := range []float64{100, 101, 102, 103, 104} {
		rising.Push(v)
	}
	if slope := rising.Slope(); slope <= 0 {
		t.Errorf("Expected a positive slope for rising prices, got %f", slope)
	}

	falling := strategy.NewWindow(5)
	for _, v := range []float64{104, 103, 102, 101, 100} {
		falling.Push(v)
	}
	if slope := falling.Slope(); slope >= 0 {
		t.Errorf("Expected a negative slope for falling prices, got %f", slope)
	}
}

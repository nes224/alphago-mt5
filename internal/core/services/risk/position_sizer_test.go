package risk_test

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

func TestRiskManager_CalculateOrder_ClampsToMinSLDistance(t *testing.T) {
	// SizingVolatility tiny (0.05) * multiplier 2.0 -> raw slDistance = 0.10, below the 3.0 floor.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0, 2.0, 1.75, false)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{SizingVolatility: 0.05, Ask: 4100.0, Bid: 4099.8, Mean: 4099.9}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.EntryPrice - order.StopLoss
	if gotDistance != 3.0 {
		t.Errorf("Expected SL distance clamped to 3.0, got %f", gotDistance)
	}
	tpDistance := order.TakeProfit - order.EntryPrice
	if tpDistance != 3.0 {
		t.Errorf("Expected TP distance clamped to 3.0, got %f", tpDistance)
	}
}

func TestRiskManager_CalculateOrder_ClampsToMaxSLDistance(t *testing.T) {
	// SizingVolatility huge (50) * multiplier 2.0 -> raw slDistance = 100, above the 20.0 ceiling.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0, 2.0, 1.75, false)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionSell)}
	metrics := domain.TickMetrics{SizingVolatility: 50.0, Ask: 4100.2, Bid: 4100.0, Mean: 4100.1}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.StopLoss - order.EntryPrice
	if gotDistance != 20.0 {
		t.Errorf("Expected SL distance clamped to 20.0, got %f", gotDistance)
	}
	tpDistance := order.EntryPrice - order.TakeProfit
	if tpDistance != 20.0 {
		t.Errorf("Expected TP distance clamped to 20.0, got %f", tpDistance)
	}
}

func TestRiskManager_CalculateOrder_WithinRangeUnclamped(t *testing.T) {
	// SizingVolatility=5 * multiplier 2.0 -> raw slDistance = 10, comfortably within [3, 20].
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0, 2.0, 1.75, false)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{SizingVolatility: 5.0, Ask: 4100.0, Bid: 4099.8, Mean: 4095.0}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.EntryPrice - order.StopLoss
	if gotDistance != 10.0 {
		t.Errorf("Expected SL distance 10.0 (unclamped), got %f", gotDistance)
	}
}

func TestRiskManager_CalculateOrder_FallsBackToStdDevWhenSizingVolatilityZero(t *testing.T) {
	// SizingVolatility=0 (e.g. M15 window not warmed up yet right after
	// startup) must fall back to the short-window StdDev instead of
	// rejecting every single signal.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0, 2.0, 1.75, false)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{SizingVolatility: 0, StdDev: 5.0, Ask: 4100.0, Bid: 4099.8, Mean: 4095.0}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.EntryPrice - order.StopLoss
	if gotDistance != 10.0 {
		t.Errorf("Expected fallback to StdDev=5.0 * multiplier 2.0 = 10.0, got %f", gotDistance)
	}
}

func TestRiskManager_CalculateOrder_RejectsWhenAccountTooSmallForVolatility(t *testing.T) {
	// balance=300, risk=1% -> riskAmount=$3. SizingVolatility=15 * multiplier
	// 2.0 -> slDistance=30 (within the wide [3,50] rails below) -> raw lot =
	// 3 / (30*100) = 0.001, below minLotSize 0.01. Must be rejected, not
	// silently floored to 0.01 (which would risk ~10x the intended 1%).
	rm := risk.NewRiskManager(0.01, 300.0, 0.01, 1.0, 3.0, 50.0, 2.0, 1.75, false)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{SizingVolatility: 15.0, Ask: 4100.0, Bid: 4099.8, Mean: 4095.0}

	order, err := rm.CalculateOrder(signal, metrics)
	if err == nil {
		t.Fatalf("expected an error rejecting the trade, got order: %+v", order)
	}
}

func TestRiskManager_CalculateOrder_UsesATRWhenEnabledAndReady(t *testing.T) {
	// UseATRForSizing=true and ATRReady=true -> ATR(10.0) * atrMultiplier(1.5)
	// = 15.0 drives slDistance, NOT SizingVolatility(5.0) * volatilityMultiplier.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 50.0, 2.0, 1.5, true)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{SizingVolatility: 5.0, ATR: 10.0, ATRReady: true, Ask: 4100.0, Bid: 4099.8, Mean: 4095.0}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.EntryPrice - order.StopLoss
	if gotDistance != 15.0 {
		t.Errorf("Expected ATR-driven SL distance 1.5*10.0=15.0, got %f", gotDistance)
	}
}

func TestRiskManager_CalculateOrder_IgnoresReadyATRWhenDisabled(t *testing.T) {
	// ATRReady=true but UseATRForSizing=false (the default) -> must still
	// fall through to SizingVolatility, proving the toggle actually gates
	// the behavior rather than auto-switching the moment ATR warms up.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 50.0, 2.0, 1.5, false)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{SizingVolatility: 5.0, ATR: 10.0, ATRReady: true, Ask: 4100.0, Bid: 4099.8, Mean: 4095.0}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.EntryPrice - order.StopLoss
	if gotDistance != 10.0 {
		t.Errorf("Expected SizingVolatility-driven SL distance 2.0*5.0=10.0 (ATR ignored while disabled), got %f", gotDistance)
	}
}

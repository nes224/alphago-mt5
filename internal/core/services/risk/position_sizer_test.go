package risk_test

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

func TestRiskManager_CalculateOrder_ClampsToMinSLDistance(t *testing.T) {
	// StdDev tiny (0.05) -> raw slDistance = 0.10, below the 3.0 floor.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{StdDev: 0.05, Ask: 4100.0, Bid: 4099.8, Mean: 4099.9}

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
	// StdDev huge (50) -> raw slDistance = 100, above the 20.0 ceiling.
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionSell)}
	metrics := domain.TickMetrics{StdDev: 50.0, Ask: 4100.2, Bid: 4100.0, Mean: 4100.1}

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
	// StdDev=5 -> raw slDistance = 10, comfortably within [3, 20].
	rm := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 3.0, 20.0)

	signal := domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{StdDev: 5.0, Ask: 4100.0, Bid: 4099.8, Mean: 4095.0}

	order, err := rm.CalculateOrder(signal, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotDistance := order.EntryPrice - order.StopLoss
	if gotDistance != 10.0 {
		t.Errorf("Expected SL distance 10.0 (unclamped), got %f", gotDistance)
	}
}

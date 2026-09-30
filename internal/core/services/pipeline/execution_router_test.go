package pipeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

// fakeEngine is a minimal ports.QuantEngine so ExecutionRouter can be tested
// in isolation, driving SignalChannel() directly instead of going through a
// real QuantEngine + strategies.
type fakeEngine struct {
	signalCh chan domain.OrderSignal
	metrics  domain.TickMetrics
}

func newFakeEngine(metrics domain.TickMetrics) *fakeEngine {
	return &fakeEngine{signalCh: make(chan domain.OrderSignal, 10), metrics: metrics}
}

func (f *fakeEngine) RegisterStrategy(s ports.QuantStrategy)            {}
func (f *fakeEngine) PushTick(tick domain.Tick)                         {}
func (f *fakeEngine) SignalChannel() <-chan domain.OrderSignal          { return f.signalCh }
func (f *fakeEngine) GetLatestMetrics(symbol string) domain.TickMetrics { return f.metrics }
func (f *fakeEngine) UpdateMetrics(symbol string, m domain.TickMetrics) {}

func TestExecutionRouter_DispatchedSignal_RecordedAndSentToOrderSink(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := newFakeEngine(domain.TickMetrics{StdDev: 1.0, Mean: 2600.0, Ask: 2601.0, Bid: 2600.8})
	riskMgr := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 0.01, 100.0)
	orderSink := make(chan risk.PreparedOrder, 10)
	router := pipeline.NewExecutionRouter(engine, riskMgr, nil, orderSink, 1)
	router.Start(ctx)

	engine.signalCh <- domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy), Reason: "TEST"}

	select {
	case order := <-orderSink:
		if order.Symbol != "XAUUSDm" {
			t.Errorf("Expected order for XAUUSDm, got %s", order.Symbol)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for order on orderSink")
	}

	recent := waitForHistory(t, router, 1)
	if recent[0].Status != pipeline.SignalStatusDispatched {
		t.Errorf("Expected status %s, got %s", pipeline.SignalStatusDispatched, recent[0].Status)
	}
}

func TestExecutionRouter_SizingRejected_RecordedAndNotDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// StdDev=0 makes RiskManager.CalculateOrder fail (zero volatility guard).
	engine := newFakeEngine(domain.TickMetrics{StdDev: 0})
	riskMgr := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 0.01, 100.0)
	orderSink := make(chan risk.PreparedOrder, 10)
	router := pipeline.NewExecutionRouter(engine, riskMgr, nil, orderSink, 1)
	router.Start(ctx)

	engine.signalCh <- domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy), Reason: "TEST"}

	recent := waitForHistory(t, router, 1)
	if recent[0].Status != pipeline.SignalStatusRejectedSizing {
		t.Errorf("Expected status %s, got %s", pipeline.SignalStatusRejectedSizing, recent[0].Status)
	}

	select {
	case order := <-orderSink:
		t.Fatalf("Expected no order to be dispatched, got: %+v", order)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestExecutionRouter_BlockedByRiskGuard_RecordedAndNotDispatched(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := newFakeEngine(domain.TickMetrics{StdDev: 1.0, Mean: 2600.0, Ask: 2601.0, Bid: 2600.8, Spread: 0.2})
	riskMgr := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.0, 0.01, 100.0)
	guard := risk.NewRiskGuard(risk.RiskGuardConfig{MaxDailyLossPercent: 0.03, MaxSpreadPips: 5.0}, 10000.0)
	guard.UpdateAccountEquity(9000.0) // trips the circuit breaker

	orderSink := make(chan risk.PreparedOrder, 10)
	router := pipeline.NewExecutionRouter(engine, riskMgr, guard, orderSink, 1)
	router.Start(ctx)

	engine.signalCh <- domain.OrderSignal{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy), Reason: "TEST"}

	recent := waitForHistory(t, router, 1)
	if recent[0].Status != pipeline.SignalStatusRejectedRiskGuard {
		t.Errorf("Expected status %s, got %s", pipeline.SignalStatusRejectedRiskGuard, recent[0].Status)
	}

	select {
	case order := <-orderSink:
		t.Fatalf("Expected no order to be dispatched, got: %+v", order)
	case <-time.After(100 * time.Millisecond):
	}
}

func waitForHistory(t *testing.T, router *pipeline.ExecutionRouter, n int) []pipeline.SignalRecord {
	t.Helper()
	for i := 0; i < 20; i++ {
		if recent := router.RecentSignals(); len(recent) >= n {
			return recent
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("Timeout waiting for %d signal history record(s)", n)
	return nil
}

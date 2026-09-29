package strategy_test

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

type MockQuantStrategy struct {
	id          string
	targetZ     float64
	signalFired bool
}

func NewMockStrategy(id string, targetZ float64) *MockQuantStrategy {
	return &MockQuantStrategy{
		id:      id,
		targetZ: targetZ,
	}
}

func (m *MockQuantStrategy) ID() string {
	return m.id
}

func (m *MockQuantStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	if math.Abs(metrics.ZScore) >= m.targetZ {
		m.signalFired = true
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionBuy),
			Price:     tick.Ask,
			Reason:    "MOCK_TEST_ZSCORE_TRIGGERED",
			Timestamp: time.Now(),
		}
	}
	return nil
}

func TestQuantEngine_MetricsCalculationAndZScore(t *testing.T) {
	engine := strategy.NewQuantEngine(100, 5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockStrat := NewMockStrategy("TEST_ZSCORE_STRAT", 2.0)
	engine.RegisterStrategy(mockStrat)

	engine.Start(ctx)

	now := time.Now()
	ticks := []domain.Tick{
		{Symbol: "XAUUSDm", Bid: 1999.5, Ask: 2000.5, OpenInterest: 1000, Timestamp: now},
		{Symbol: "XAUUSDm", Bid: 2000.0, Ask: 2001.0, OpenInterest: 1010, Timestamp: now.Add(100 * time.Millisecond)},
		{Symbol: "XAUUSDm", Bid: 1999.0, Ask: 2000.0, OpenInterest: 1020, Timestamp: now.Add(200 * time.Millisecond)},
		{Symbol: "XAUUSDm", Bid: 2000.5, Ask: 2001.5, OpenInterest: 1015, Timestamp: now.Add(300 * time.Millisecond)},
		{Symbol: "XAUUSDm", Bid: 2000.0, Ask: 2001.0, OpenInterest: 1030, Timestamp: now.Add(400 * time.Millisecond)},
	}

	for _, tick := range ticks {
		engine.PushTick(tick)
		time.Sleep(20 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)

	outlierTick := domain.Tick{
		Symbol:       "XAUUSDm",
		Bid:          2049.0,
		Ask:          2051.0,
		OpenInterest: 1100,
		Timestamp:    now.Add(500 * time.Millisecond),
	}

	engine.PushTick(outlierTick)

	select {
	case signal := <-engine.SignalChannel():
		if signal.Symbol != "XAUUSDm" {
			t.Errorf("Expected symbol XAUUSDm, got %s", signal.Symbol)
		}
		if !mockStrat.signalFired {
			t.Errorf("Expected mock strategy to fire signal")
		}
		t.Logf("Success ! Generate Signal: Action=%s, Price=%.2f, Reason=%s", signal.Action, signal.Price, signal.Reason)
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Test timed out! Engine failed to dispatch OrderSignal on high Z-score spike")
	}
}

type FunctionalStrategy struct {
	id       string
	onTickFn func(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal
}

func (f *FunctionalStrategy) ID() string { return f.id }
func (f *FunctionalStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	return f.onTickFn(tick, metrics)
}

func TestQuantEngine_FlatMarket_ZeroStdDev(t *testing.T) {
	windowSize := 5
	bufferSize := 100
	engine := strategy.NewQuantEngine(bufferSize, windowSize)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	symbol := "EURUSD"
	flatPrice := 1.0850

	for i := 0; i < windowSize; i++ {
		tick := domain.Tick{
			Symbol:       symbol,
			Bid:          flatPrice,
			Ask:          flatPrice,
			OpenInterest: 1000,
			Timestamp:    time.Now().Add(time.Duration(i) * time.Second),
		}

		metrics := engine.ProcessTick(tick)
		if math.IsNaN(metrics.StdDev) || math.IsInf(metrics.StdDev, 0) {
			t.Fatalf("Tick %d: StdDev is invalid (NaN or Inf): %f", i, metrics.StdDev)
		}
		if math.IsNaN(metrics.ZScore) || math.IsInf(metrics.ZScore, 0) {
			t.Fatalf("Tick %d: ZScore is invalid (NaN or Inf): %f", i, metrics.ZScore)
		}

		if metrics.StdDev != 0 {
			t.Errorf("Expected StdDev to be 0, got %f", metrics.StdDev)
		}
		if metrics.ZScore != 0 {
			t.Errorf("Expected ZScore to be 0, got %f", metrics.ZScore)
		}
	}
}

func TestQuantEngine_ConcurrentPush_RaceCondition(t *testing.T) {
	bufferSize := 1000
	windowSize := 10
	engine := strategy.NewQuantEngine(bufferSize, windowSize)

	engine.RegisterStrategy(&MockQuantStrategy{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine.Start(ctx)

	numGoroutines := 10
	ticksPerGoroutine := 100
	symbols := []string{"EURUSD", "GBPUSD", "USDJPY", "BTCUSD"}

	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			for i := 0; i < ticksPerGoroutine; i++ {
				symbol := symbols[(routineID+i)%len(symbols)]
				basePrice := 100.0 + float64(routineID)
				tick := domain.Tick{
					Symbol:       symbol,
					Bid:          basePrice + float64(i)*0.1,
					Ask:          basePrice + float64(i)*0.1 + 0.02,
					OpenInterest: int64(1000 + i),
					Timestamp:    time.Now().Add(time.Duration(i) * time.Millisecond),
				}

				engine.PushTick(tick)

			}
		}(g)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			time.Sleep(2 * time.Millisecond)
			engine.RegisterStrategy(&MockQuantStrategy{})
		}
	}()

	wg.Wait()

	time.Sleep(100 * time.Millisecond)
}

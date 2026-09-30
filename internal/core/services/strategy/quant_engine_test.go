package strategy_test

import (
	"context"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

type MockQuantStrategy struct {
	id      string
	targetZ float64
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
		if signal.Reason != "MOCK_TEST_ZSCORE_TRIGGERED" {
			t.Errorf("Expected mock strategy to fire signal, got Reason=%s", signal.Reason)
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

func TestCalculateMetrics_OIDeltaAndVelocities(t *testing.T) {
	engine := strategy.NewQuantEngine(100, 5)

	t1 := time.Now()
	tick1 := domain.Tick{
		Symbol:       "XAUUSDm",
		Bid:          2000.0,
		Ask:          2002.0,
		OpenInterest: 1000,
		Timestamp:    t1,
	}

	// Tick แรก: OI Delta / Velocities ต้องเป็น 0 ทั้งหมด
	m1 := engine.ProcessTick(tick1)
	if m1.OIDelta != 0 || m1.OIVelocity != 0 || m1.PriceVelocity != 0 {
		t.Errorf("Tick 1 failed: expected zero deltas, got OIDelta=%d, OIVelocity=%f, PriceVel=%f",
			m1.OIDelta, m1.OIVelocity, m1.PriceVelocity)
	}

	// Tick สอง: ผ่านไป 2 วินาที, OI เพิ่ม 100 (1000 -> 1100), MidPrice เพิ่ม $10 (2001 -> 2011)
	t2 := t1.Add(2 * time.Second)
	tick2 := domain.Tick{
		Symbol:       "XAUUSDm",
		Bid:          2010.0,
		Ask:          2012.0,
		OpenInterest: 1100,
		Timestamp:    t2,
	}

	m2 := engine.ProcessTick(tick2)

	// OIDelta = 100
	if m2.OIDelta != 100 {
		t.Errorf("Expected OIDelta 100, got %d", m2.OIDelta)
	}
	// OIVelocity = 100 / 2s = 50.0
	if m2.OIVelocity != 50.0 {
		t.Errorf("Expected OIVelocity 50.0, got %f", m2.OIVelocity)
	}
	// PriceVelocity = $10 / 2s = 5.0
	if m2.PriceVelocity != 5.0 {
		t.Errorf("Expected PriceVelocity 5.0, got %f", m2.PriceVelocity)
	}
}

func TestMicrostructure_LiquiditySweepDetection(t *testing.T) {
	windowSize := 5
	sweepDetector := strategy.NewLiquiditySweepDetector(windowSize)
	initialTicks := []domain.Tick{
		{Symbol: "XAUUSDm", Ask: 2600.00, Bid: 2599.80, Volume: 10, Timestamp: time.Now()},
		{Symbol: "XAUUSDm", Ask: 2602.00, Bid: 2601.80, Volume: 12, Timestamp: time.Now()},
		{Symbol: "XAUUSDm", Ask: 2605.00, Bid: 2604.80, Volume: 25, Timestamp: time.Now()}, // High เดิม = 2605.00
		{Symbol: "XAUUSDm", Ask: 2603.00, Bid: 2602.80, Volume: 18, Timestamp: time.Now()},
		{Symbol: "XAUUSDm", Ask: 2601.00, Bid: 2600.80, Volume: 10, Timestamp: time.Now()},
	}
	for _, tick := range initialTicks {
		sweepDetector.Update(tick)
	}

	// Case A: ราคาปกติในกรอบ -> ต้องไม่เกิด Sweep
	normalTick := domain.Tick{Symbol: "XAUUSDm", Ask: 2604.00, Bid: 2603.80, Volume: 15, Timestamp: time.Now()}
	if event := sweepDetector.DetectSweep(normalTick); event.IsSweep {
		t.Fatalf("Expected no sweep event, but got: %+v", event)
	}

	// Case B: High Liquidity Sweep (กวาด High 2605.00 -> 2607.00 พร้อม Volume พุ่ง)
	sweepHighTick := domain.Tick{
		Symbol:    "XAUUSDm",
		Ask:       2607.00, // Breakout High 2605.00
		Bid:       2606.80,
		Volume:    150, // Vol Spike
		Timestamp: time.Now(),
	}

	event := sweepDetector.DetectSweep(sweepHighTick)
	if !event.IsSweep {
		t.Fatalf("Expected Liquidity Sweep Event, but got none")
	}

	if event.SweepType != strategy.SweepTypeHigh {
		t.Errorf("Expected SweepTypeHigh, got %s", event.SweepType)
	}

	if event.SweptLevel != 2605.00 {
		t.Errorf("Expected SweptLevel 2605.00, got %.2f", event.SweptLevel)
	}
}

func TestQuantEngine_PipelineIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bufferCapicity := 100
	windowSize := 10
	quantEngine := strategy.NewQuantEngine(bufferCapicity, windowSize)

	symbol := "XAUUSDm"
	oiStrategy := strategy.NewOIExpansionStrategy(symbol, 1.5, 5.0, 0.1)
	quantEngine.RegisterStrategy(oiStrategy)

	riskManager := risk.NewRiskManager(0.01, 10000.0, 0.01, 1.00)
	orderSink := make(chan risk.PreparedOrder, bufferCapicity)
	execRouter := pipeline.NewExecutionRouter(quantEngine, riskManager, orderSink, 1)

	quantEngine.Start(ctx)
	execRouter.Start(ctx)

	basePrice := 2600.0
	// กำหนดราคาให้ผันผวนสลับไปมาอย่างชัดเจน
	priceOffsets := []float64{1.0, -1.5, 2.0, -2.5, 1.8, -1.2, 3.0, -2.0, 1.5, -0.8, 2.2, -1.1}

	// 1. Push Warm-up Ticks เพื่อสร้าง Variance ให้กับ Rolling Window
	for i, offset := range priceOffsets {
		tck := domain.Tick{
			Symbol:       symbol,
			Ask:          basePrice + offset,
			Bid:          (basePrice + offset) - 0.20,
			Volume:       10 + int64(i),
			OpenInterest: 1000, // ค่า OI นิ่งๆ ไว้ก่อน ไม่ให้ยิง Signal ในช่วง Warm-up
			Timestamp:    time.Now().Add(time.Duration(i) * time.Millisecond),
		}
		quantEngine.PushTick(tck)
		time.Sleep(5 * time.Millisecond) // ให้เวลากับ Worker Routine ในการกิน channel
	}

	// 2. Poll จนกว่า QuantEngine จะคำนวณ StdDev > 0 สำหรับ Window นี้
	assertMetricsReady := false
	for i := 0; i < 20; i++ {
		m := quantEngine.GetLatestMetrics(symbol) // หรือวิธีอ่าน state metrics ล่าสุดของ engine
		if m.StdDev > 0 {
			assertMetricsReady = true
			t.Logf("📊 Engine Metrics Ready! StdDev: %.4f", m.StdDev)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !assertMetricsReady {
		t.Fatal("Failed to warm-up QuantEngine: StdDev is still 0 after pushing ticks")
	}

	// 3. Push Spike Tick เพื่อ Trigger Signal เมื่อ Window และ Metrics พร้อมแล้ว
	spikeTick := domain.Tick{
		Symbol:       symbol,
		Ask:          2615.00,
		Bid:          2614.80,
		Volume:       100,
		OpenInterest: 1200, // เกิด OI Expansion Trigger!
		Timestamp:    time.Now().Add(200 * time.Millisecond),
	}
	quantEngine.PushTick(spikeTick)

	// 4. รอรับ PreparedOrder จาก channel
	select {
	case preparedOrder := <-orderSink:
		t.Logf("🟢 Integration Test Success! Received PreparedOrder: %+v", preparedOrder)
		if preparedOrder.Symbol != symbol {
			t.Errorf("Expected Symbol %s, got %s", symbol, preparedOrder.Symbol)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for PreparedOrder in orderSink channel.")
	}
}

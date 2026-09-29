📐 Pure Quantitative Strategy Engine Architecture Document
System: AlphaGo MT5 Quant Engine

Focus: Pure Numerical Models (Tick-by-Tick, Microstructure, Statistical SMC)

Architecture: Hexagonal Event-Driven Engine

🎯 1. Executive Summary & Paradigm Shift
ระบบ AlphaGo MT5 Engine ในเฟสนี้จะทำการสลัดทิ้ง Candlestick (OHLC) / Visual Chart Analysis ออกจาก Critical Path ของระบบโดยสิ้นเชิง

ทำไมถึงต้องเลิกใช้ แท่งเทียน (Candlestick)?
Loss of Granularity: แท่งเทียนเป็นการเอาข้อมูลราคามามัดรวมกันตามเวลา (Time Aggregation) ทำให้ Microstructure Data สูงถึง 90% หายไป

Lagging Indicator: ราคาปิดแท่ง (Close Price) ช้าเกินไปสำหรับการทำ Execution ระดับ High-Speed

Noise & Manipulation: แท่งเทียนถูกปั่นสร้างรูปทรงกราฟ (Chart Patterns / Fakeout) เพื่อหลอกระบบ Discretionary Trading ได้ง่าย

สิ่งที่ระบบนี้มุ่งเน้น (Pure Numerical Quant Data):
Order Flow & Tick Velocity Dynamics (วัดแรงซื้อขายจริงแบบ Real-time)

Mathematical SMC & Liquidity Sweep (คำนวณจุดกวาด Stop Loss ด้วยสถิติ)

Statistical Arbitrage & Z-Score Models (วัดการเบี่ยงเบนออกนอกค่าเฉลี่ยทางคณิตศาสตร์)

🏛️ 2. Architectural Data Flow (Pure Tick Pipeline)
ตัดส่วน Candle Builder ออกจาก Loop หลัก ข้อมูลจาก Socket 5556 จะวิ่งเข้าสู่ Quant Metric Calculations โดยตรง เพื่อทำ Latency ให้ต่ำกว่า 1 มิลลิวินาที

[MT5 Socket 5556] ──(Raw Tick)──> [StreamAdapter] 
                                        │
                                        ▼
                            [StrategyEngine.PushTick]
                                        │
             ┌──────────────────────────┴──────────────────────────┐
             ▼                                                     ▼
   [Tick Metrics Engine]                               [Quant Strategy Loop]
   - Price Velocity (dP/dt)                            - Z-Score Mean Reversion
   - Real-time Spread Delta                            - Liquidity Sweep Detection
   - Tick Imbalance Ratio                              - Order Flow Absorption
             │                                                     │
             └──────────────────────────┬──────────────────────────┘
                                        ▼
                           [OrderSignal (< 1ms)] 
                                        │
                                        ▼
                             [MT5 Execution Adapter]


🛠️ 3. Core Go Implementation (Hexagonal Clean Architecture)
3.1 Domain Layer: Pure Data Structures
นำไปไว้ที่ internal/core/domain/tick_metrics.go

package domain

import "time"

// Tick Quant Metrics สถิติตัวเลขระดับ Microsecond
type TickMetrics struct {
	Symbol         string
	Bid            float64
	Ask            float64
	Spread         float64
	PriceVelocity  float64 // dP/dt (อัตราเร่งของการเปลี่ยนแปลงราคา)
	TickImbalance  float64 // ความไม่สมดุลของราคา Bid/Ask
	Timestamp      time.Time
}

// ZScoreState ตัวแปรสถิติสำหรับคำนวณ Mean Reversion
type ZScoreState struct {
	Mean   float64
	StdDev float64
	ZScore float64
}

3.2 Pure Quant Strategy Interface
นำไปไว้ที่ internal/core/ports/strategy.go

package ports

import "alphago-mt5/internal/core/domain"

type QuantStrategy interface {
	ID() string
	// ประมวลผลจาก Tick สดและ Metrics ทางสถิติโดยตรง (No Candlestick)
	OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal
}

3.3 Quantitative Engine Logic
นำไปไว้ที่ internal/core/services/strategy/quant_engine.go

package strategy

import (
	"context"
	"math"
	"sync"
	"time"

	"alphago-mt5/internal/core/domain"
	"alphago-mt5/internal/core/ports"
)

type PureQuantEngine struct {
	mu           sync.RWMutex
	strategies   []ports.QuantStrategy
	tickChan     chan domain.Tick
	signalChan   chan domain.OrderSignal
	lastTickMap  map[string]domain.Tick
	priceHistory map[string][]float64 // Sliding Window สำหรับ Z-Score
	windowSize   int
}

func NewPureQuantEngine(bufferSize int, windowSize int) *PureQuantEngine {
	return &PureQuantEngine{
		strategies:   make([]ports.QuantStrategy, 0),
		tickChan:     make(chan domain.Tick, bufferSize),
		signalChan:   make(chan domain.OrderSignal, bufferSize),
		lastTickMap:  make(map[string]domain.Tick),
		priceHistory: make(map[string][]float64),
		windowSize:   windowSize,
	}
}

func (e *PureQuantEngine) RegisterStrategy(s ports.QuantStrategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies = append(e.strategies, s)
}

func (e *PureQuantEngine) PushTick(tick domain.Tick) {
	e.tickChan <- tick
}

func (e *PureQuantEngine) SignalChannel() <-chan domain.OrderSignal {
	return e.signalChan
}

func (e *PureQuantEngine) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tick, ok := <-e.tickChan:
				if !ok {
					return
				}
				e.processTick(tick)
			}
		}
	}()
}

func (e *PureQuantEngine) processTick(tick domain.Tick) {
	e.mu.Lock()
	
	// 1. คำนวณ Metrics ทางคณิตศาสตร์ real-time
	metrics := e.calculateMetrics(tick)

	// 2. กระจายงานเข้า Strategy คำนวณตัวเลข
	for _, s := range e.strategies {
		if signal := s.OnTick(tick, metrics); signal != nil {
			e.signalChan <- *signal
		}
	}
	
	e.lastTickMap[tick.Symbol] = tick
	e.mu.Unlock()
}

// calculateMetrics คำนวณ Price Velocity และ Sliding Window Statistics
func (e *PureQuantEngine) calculateMetrics(tick domain.Tick) domain.TickMetrics {
	lastTick, exists := e.lastTickMap[tick.Symbol]
	midPrice := (tick.Bid + tick.Ask) / 2.0

	var velocity float64
	if exists {
		timeDiff := tick.Timestamp.Sub(lastTick.Timestamp).Seconds()
		if timeDiff > 0 {
			priceDiff := midPrice - ((lastTick.Bid + lastTick.Ask) / 2.0)
			velocity = priceDiff / timeDiff // dP/dt
		}
	}

	// อัปเดต Sliding Window Price History
	history := e.priceHistory[tick.Symbol]
	history = append(history, midPrice)
	if len(history) > e.windowSize {
		history = history[1:]
	}
	e.priceHistory[tick.Symbol] = history

	return domain.TickMetrics{
		Symbol:        tick.Symbol,
		Bid:           tick.Bid,
		Ask:           tick.Ask,
		Spread:        tick.Ask - tick.Bid,
		PriceVelocity: velocity,
		Timestamp:     tick.Timestamp,
	}
}

3.4 ตัวอย่าง Quant Strategy: Statistical Z-Score Mean Reversion
นำไปไว้ที่ internal/core/services/strategy/zscore_strategy.go

package strategy

import (
	"math"
	"time"

	"alphago-mt5/internal/core/domain"
)

type ZScoreStrategy struct {
	symbol      string
	threshold   float64 // ค่า Standard Deviation ที่จะยิงสวน (เช่น 2.5)
	priceBuffer []float64
	bufferCap   int
}

func NewZScoreStrategy(symbol string, threshold float64, bufferCap int) *ZScoreStrategy {
	return &ZScoreStrategy{
		symbol:      symbol,
		threshold:   threshold,
		priceBuffer: make([]float64, 0, bufferCap),
		bufferCap:   bufferCap,
	}
}

func (s *ZScoreStrategy) ID() string {
	return "QUANT_ZSCORE_MEAN_REVERSION"
}

func (s *ZScoreStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	if tick.Symbol != s.symbol {
		return nil
	}

	midPrice := (tick.Bid + tick.Ask) / 2.0
	s.priceBuffer = append(s.priceBuffer, midPrice)
	if len(s.priceBuffer) > s.bufferCap {
		s.priceBuffer = s.priceBuffer[1:]
	}

	if len(s.priceBuffer) < s.bufferCap {
		return nil // รอ Data Warmup
	}

	// คำนวณ Mean และ Standard Deviation
	mean, stdDev := s.calculateStats(s.priceBuffer)
	if stdDev == 0 {
		return nil
	}

	// คำนวณ Z-Score = (Price - Mean) / StdDev
	zScore := (midPrice - mean) / stdDev

	// เงื่อนไขการส่ง Order ตามตัวเลขสถิติ (ไร้แท่งเทียน)
	if zScore <= -s.threshold {
		// ราคาตกลงมาเบี่ยงเบนออกนอกค่าเฉลี่ยเกิน -2.5 StdDev -> ยิง BUY สวนเข้าหาค่าเฉลี่ย
		return &domain.OrderSignal{
			Symbol:    s.symbol,
			Action:    domain.ActionBuy,
			Price:     tick.Ask,
			Reason:    "STATISTICAL_OVEREXTENDED_BUY",
			Timestamp: time.Now(),
		}
	} else if zScore >= s.threshold {
		// ราคาพุ่งขึ้นไปเบี่ยงเบนออกนอกค่าเฉลี่ยเกิน +2.5 StdDev -> ยิง SELL สวนเข้าหาค่าเฉลี่ย
		return &domain.OrderSignal{
			Symbol:    s.symbol,
			Action:    domain.ActionSell,
			Price:     tick.Bid,
			Reason:    "STATISTICAL_OVEREXTENDED_SELL",
			Timestamp: time.Now(),
		}
	}

	return nil
}

func (s *ZScoreStrategy) calculateStats(data []float64) (mean float64, stdDev float64) {
	var sum float64
	for _, v := range data {
		sum += v
	}
	mean = sum / float64(len(data))

	var varianceSum float64
	for _, v := range data {
		varianceSum += math.Pow(v-mean, 2)
	}
	stdDev = math.Sqrt(varianceSum / float64(len(data)))
	return mean, stdDev
}
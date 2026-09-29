package strategy

import (
	"context"
	"math"
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type QuantEngine struct {
	mu            sync.RWMutex
	strategies    []ports.QuantStrategy
	latestMetrics map[string]domain.TickMetrics
	tickChan      chan domain.Tick
	signalChan    chan domain.OrderSignal
	lastTickMap   map[string]domain.Tick
	priceHistory  map[string][]float64
	windowSize    int
}

func NewQuantEngine(bufferSize int, windowSize int) *QuantEngine {
	return &QuantEngine{
		strategies:   make([]ports.QuantStrategy, 0),
		tickChan:     make(chan domain.Tick, bufferSize),
		signalChan:   make(chan domain.OrderSignal, bufferSize),
		lastTickMap:  make(map[string]domain.Tick),
		priceHistory: make(map[string][]float64),
		windowSize:   windowSize,
	}
}

func (e *QuantEngine) RegisterStrategy(s ports.QuantStrategy) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.strategies = append(e.strategies, s)
}

func (e *QuantEngine) PushTick(tick domain.Tick) {
	e.tickChan <- tick
}

func (e *QuantEngine) SignalChannel() <-chan domain.OrderSignal {
	return e.signalChan
}

func (e *QuantEngine) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case tick, ok := <-e.tickChan:
				if !ok {
					return
				}
				e.ProcessTick(tick)
			}
		}
	}()
}

func (e *QuantEngine) calculateMetrics(tick domain.Tick) domain.TickMetrics {
	lastTick, exists := e.lastTickMap[tick.Symbol]
	midPrice := tick.MidPrice()

	var oiDelta int64 = 0
	var oiVelocity float64 = 0
	var priceVelocity float64 = 0

	if exists {
		oiDelta = tick.OpenInterest - lastTick.OpenInterest

		timeDelta := tick.Timestamp.Sub(lastTick.Timestamp).Seconds()
		if timeDelta > 0 {
			oiVelocity = float64(oiDelta) / timeDelta
			priceDelta := midPrice - lastTick.MidPrice()
			priceVelocity = priceDelta / timeDelta
		}
	}

	e.lastTickMap[tick.Symbol] = tick

	history := e.priceHistory[tick.Symbol]

	mean, stdDev, zScore := e.calculateZScore(history, midPrice)

	history = append(history, midPrice)
	if len(history) > e.windowSize {
		history = history[1:]
	}
	e.priceHistory[tick.Symbol] = history

	return domain.TickMetrics{
		Symbol:        tick.Symbol,
		Bid:           tick.Bid,
		Ask:           tick.Ask,
		Spread:        tick.Spread(),
		PriceVelocity: priceVelocity,
		OIDelta:       oiDelta,
		OIVelocity:    oiVelocity,
		OpenInterest:  tick.OpenInterest,
		Mean:          mean,
		StdDev:        stdDev,
		ZScore:        zScore,
		Timestamp:     tick.Timestamp,
	}
}

func (e *QuantEngine) calculateZScore(data []float64, currentPrice float64) (mean float64, stdDev float64, zScore float64) {
	n := float64(len(data))
	if n == 0 {
		return 0, 0, 0
	}

	var sum float64
	for _, v := range data {
		sum += v
	}

	mean = sum / n

	var varianceSum float64
	for _, v := range data {
		varianceSum += math.Pow(v-mean, 2)
	}

	stdDev = math.Sqrt(varianceSum / n)

	if stdDev > 0 {
		zScore = (currentPrice - mean) / stdDev
	}

	return mean, stdDev, zScore
}

func (e *QuantEngine) ProcessTick(tick domain.Tick) domain.TickMetrics {
	e.mu.Lock()
	defer e.mu.Unlock()

	metrics := e.calculateMetrics(tick)

	for _, s := range e.strategies {
		if signal := s.OnTick(tick, metrics); signal != nil {
			e.signalChan <- *signal
		}
	}

	e.lastTickMap[tick.Symbol] = tick
	return metrics
}

func (e *QuantEngine) GetLatestMetrics(symbol string) domain.TickMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if m, exists := e.latestMetrics[symbol]; exists {
		return m
	}

	return domain.TickMetrics{}
}

func (e *QuantEngine) UpdateMetrics(symbol string, m domain.TickMetrics) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.latestMetrics == nil {
		e.latestMetrics = make(map[string]domain.TickMetrics)
	}
	e.latestMetrics[symbol] = m
}

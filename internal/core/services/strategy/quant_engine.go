package strategy

import (
	"context"
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type QuantEngine struct {
	mu           sync.RWMutex
	strategies   []ports.QuantStrategy
	tickChan     chan domain.Tick
	signalChan   chan domain.OrderSignal
	lastTickMap  map[string]domain.Tick
	priceHistory map[string][]float64
	windowSize   int
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
				e.processTick(tick)
			}
		}
	}()
}

func (e *QuantEngine) processTick(tick domain.Tick) {
	e.mu.Lock()
	metrics := e.calculateMetrics(tick)

	for _, s := range e.strategies {
		if signal := s.OnTick(tick, metrics); signal != nil {
			e.signalChan <- *signal
		}
	}

	e.lastTickMap[tick.Symbol] = tick
	e.mu.Unlock()
}

func (e *QuantEngine) calculateMetrics(tick domain.Tick) domain.TickMetrics {
	lastTick, exists := e.lastTickMap[tick.Symbol]
	midPrice := (tick.Bid + tick.Ask) / 2.0

	var velocity float64
	if exists {
		timeDiff := tick.Timestamp.Sub(lastTick.Timestamp).Seconds()
		if timeDiff > 0 {
			priceDiff := midPrice - ((lastTick.Bid + lastTick.Ask) / 2.0)
			velocity = priceDiff / timeDiff
		}
	}

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

package ports

import "github.com/nes224/alphago-mt5/internal/core/domain"

type QuantStrategy interface {
	ID() string
	OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal
	OnCandle(candle domain.Candle) *domain.OrderSignal
}

type QuantEngine interface {
	RegisterStrategy(s QuantStrategy)
	PushTick(tick domain.Tick)
	SignalChannel() <-chan domain.OrderSignal
}

package ports

import "github.com/nes224/alphago-mt5/internal/core/domain"

type QuantStrategy interface {
	ID() string
	OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal
}

type QuantEngine interface {
	RegisterStrategy(s QuantStrategy)
	PushTick(tick domain.Tick)
	SignalChannel() <-chan domain.OrderSignal
	GetLatestMetrics(symbol string) domain.TickMetrics
	UpdateMetrics(symbol string, m domain.TickMetrics)
	GetMultiTimeframeState(symbol string) domain.MultiTimeframeState
}

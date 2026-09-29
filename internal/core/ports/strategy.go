package ports

import "github.com/nes224/alphago-mt5/internal/core/domain"

type Strategy interface {
	ID() string
	OnTick(tick domain.Tick) *domain.OrderSignal
	OnCandle(candle domain.Candle) *domain.OrderSignal
}

type StategyEngine interface {
	RegisterStrategy(s Strategy)
	PushTick(tick domain.Tick)
	SignalChannel() <-chan domain.OrderSignal
}


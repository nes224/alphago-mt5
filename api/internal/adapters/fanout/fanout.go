// Package fanout sends every market event to several publishers.
package fanout

import (
	"context"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
	"github.com/nes224/alphago-mt5/api/internal/core/ports"
)

// Publisher forwards each event to every publisher in the list, in order.
type Publisher []ports.MarketPublisher

var _ ports.MarketPublisher = Publisher(nil)

func (p Publisher) PublishTick(ctx context.Context, t domain.Tick) {
	for _, x := range p {
		x.PublishTick(ctx, t)
	}
}

func (p Publisher) PublishCandle(ctx context.Context, c domain.Candle) {
	for _, x := range p {
		x.PublishCandle(ctx, c)
	}
}

func (p Publisher) PublishTradeClosed(ctx context.Context, e domain.TradeClosed) {
	for _, x := range p {
		x.PublishTradeClosed(ctx, e)
	}
}

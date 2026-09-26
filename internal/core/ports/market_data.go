package ports

import (
	"context"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type MarketDataSubscritber interface {
	SubscribeTicks(ctx context.Context, handler func(tick domain.Tick)) error
	SubscribeCandles(ctx context.Context, handler func(candle domain.Candle)) error
	Close() error
}
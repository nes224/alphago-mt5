package ports

import (
	"context"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// MarketEventHandler รับ event ที่อ่านได้จาก MT5 stream
type MarketEventHandler interface {
	HandleTick(ctx context.Context, t domain.Tick)
	HandleTradeClosed(ctx context.Context, e domain.TradeClosed)
}

type MarketPublisher interface {
	PublishTick(ctx context.Context, t domain.Tick)
	PublishCandle(ctx context.Context, c domain.Candle)
	PublishTradeClosed(ctx context.Context, e domain.TradeClosed)
}

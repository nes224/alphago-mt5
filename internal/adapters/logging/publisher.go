package logging

import (
	"context"
	"log/slog"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type Publisher struct{ log *slog.Logger }

var _ ports.MarketPublisher = (*Publisher)(nil)

func NewPublisher(log *slog.Logger) *Publisher {
	return &Publisher{log: log.With("component", "log_publisher")}
}

func (p *Publisher) PublishTick(ctx context.Context, t domain.Tick) {
	p.log.DebugContext(ctx, "tick", "symbol", t.Symbol, "bid", t.Bid, "ask", t.Ask, "time", t.Time.Format("15:04:05"))
}

func (p *Publisher) PublishCandle(ctx context.Context, c domain.Candle) {
	if !c.Closed {
		return
	}

	p.log.InfoContext(ctx, "candle closed",
		"symbol", c.Symbol, "tf", c.Timeframe, "open_time", c.OpenTime.Format("2006-01-02 15:04"),
		"o", c.Open, "h", c.High, "l", c.Low, "c", c.Close, "ticks", c.TickCount)
}

func (p *Publisher) PublishTradeClosed(ctx context.Context, e domain.TradeClosed) {
	p.log.InfoContext(ctx, "trade closed", "symbol", e.Symbol, "ticket", e.Ticket, "profit", e.Profit)
}

package services

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
	"github.com/nes224/alphago-mt5/api/internal/core/ports"
)

type GatewayService struct {
	agg *CandleAggregator
	pub ports.MarketPublisher
	log *slog.Logger

	lastTickUnix atomic.Int64
	tickCount    atomic.Int64

	// OnCandleClose, if set, is called when a live M1 candle closes. The gateway
	// uses it to fetch the broker's closed candles for every timeframe and store
	// them, so the files stay current while it runs. It must return quickly.
	OnCandleClose func()
}

type GatewayStats struct {
	TicksReceived int64      `json:"ticks_received"`
	LastTickAt    *time.Time `json:"last_tick_at,omitempty"`
}

var _ ports.MarketEventHandler = (*GatewayService)(nil)

func NewGatewayService(pub ports.MarketPublisher, log *slog.Logger) *GatewayService {
	return &GatewayService{
		agg: NewCandleAggregator(domain.TimeframeM1),
		pub: pub,
		log: log,
	}
}

func (s *GatewayService) HandleTick(ctx context.Context, t domain.Tick) {
	s.lastTickUnix.Store(time.Now().Unix())
	s.tickCount.Add(1)
	s.pub.PublishTick(ctx, t)

	u := s.agg.OnTick(t)
	if u.Dropped {
		s.log.Warn("dropped out-of-order tick", "symbol", t.Symbol, "tick_time", t.Time, "candle_open", u.Current.OpenTime)
		return
	}
	if u.Closed != nil {
		s.pub.PublishCandle(ctx, *u.Closed)
		if s.OnCandleClose != nil {
			s.OnCandleClose()
		}
	}

	s.pub.PublishCandle(ctx, u.Current)
}

func (s *GatewayService) HandleTradeClosed(ctx context.Context, e domain.TradeClosed) {
	s.pub.PublishTradeClosed(ctx, e)
}

func (s *GatewayService) Stats() GatewayStats {
	st := GatewayStats{TicksReceived: s.tickCount.Load()}
	if u := s.lastTickUnix.Load(); u > 0 {
		t := time.Unix(u, 0).UTC()
		st.LastTickAt = &t
	}

	return st
}

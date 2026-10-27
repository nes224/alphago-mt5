package services

import (
	"log/slog"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

// Backfiller fills the gap between the newest stored candle and the broker's
// newest closed candle, for every timeframe of a symbol.
type Backfiller struct {
	history  ports.HistoryProvider
	store    ports.CandleStore
	lookback map[domain.Timeframe]time.Duration
	log      *slog.Logger
}

func NewBackfiller(h ports.HistoryProvider, s ports.CandleStore, lookback map[domain.Timeframe]time.Duration, log *slog.Logger) *Backfiller {
	if lookback == nil {
		lookback = DefaultLookback
	}
	return &Backfiller{history: h, store: s, lookback: lookback, log: log.With("component", "backfiller")}
}

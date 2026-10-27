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

type TFResult struct {
	Timeframe domain.Timeframe `json:"timeframe"`
	Written   int              `json:"written"`
	Latest    *time.Time       `json:"latest,omitempty"`
	Skipped   bool             `json:"skipped,omitempty"`
	Err       string           `json:"error,omitempty"`
}

type Report struct {
	Symbol     string     `json:"symbol"`
	ServerTime time.Time  `json:"server_time"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt time.Time  `json:"finished_at"`
	Results    []TFResult `json:"result"`
	Err        string     `json:"error,omitempty"`
}

func NewBackfiller(h ports.HistoryProvider, s ports.CandleStore, lookback map[domain.Timeframe]time.Duration, log *slog.Logger) *Backfiller {
	if lookback == nil {
		lookback = DefaultLookback
	}
	return &Backfiller{history: h, store: s, lookback: lookback, log: log.With("component", "backfiller")}
}

func (r Report) OK() bool {
	if r.Err != "" {
		return false
	}
	for _, res := range r.Results {
		if res.Err != "" {
			return false
		}
	}
	return true
}
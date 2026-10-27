package services

import (
	"context"
	"fmt"
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

func (b *Backfiller) fill(ctx context.Context, symbol string, tf domain.Timeframe, now time.Time) (res TFResult, historyErr error) {
	res.Timeframe = tf
	log := b.log.With("symbol", symbol, "tf", tf)

	latest, hasLatest, err := b.store.LatestCandle(symbol, tf)
	if err != nil {
		res.Err = fmt.Sprintf("read store: %v", err)
		log.Warn("backfill skipped timeframe", "err", err)
		return res, nil
	}

	if hasLatest {
		t := latest.OpenTime
		res.Latest = &t
	}

	from, need := BackfillFrom(latest, hasLatest, now, tf, b.lookback[tf])
	if !need {
		res.Skipped = true
		return res, nil
	}

	candles, err := b.history.Rates(ctx, symbol, tf, from, now)
	if err != nil {
		res.Err = fmt.Sprintf("rates: %v", err)
		log.Warn("backfill stopped", "from", from, "err", err)
		return res, err
	}

	if err := b.store.WriteCandles(symbol, tf, candles); err != nil {
		res.Err = fmt.Sprintf("write store: %v", err)
		log.Warn("backfill skipped timeframe", "err", err)
		return res, nil
	}

	for _, c := range candles {
		if !c.Closed {
			continue
		}
		res.Written++
		if res.Latest == nil || c.OpenTime.After(*res.Latest) {
			res.Latest = &c.OpenTime

		}
	}

	return res, nil
}

func (b *Backfiller) run(ctx context.Context, symbol string, tfs []domain.Timeframe) Report {
	rep := Report{Symbol: symbol, StartedAt: time.Now()}

	now, err := b.history.ServerTime(ctx, symbol)
	if err != nil {
		rep.Err = fmt.Sprintf("server time: %v", err)
		b.log.Warn("backfill aborted", "symbol", symbol, "err", err)
		rep.FinishedAt = time.Now()
		return rep
	}
	rep.ServerTime = now

	for i, tf := range tfs {
		res, historyErr := b.fill(ctx, symbol, tf, now)
		rep.Results = append(rep.Results, res)
		if historyErr != nil {
			for _, rest := range tfs[i+1:] {
				rep.Results = append(rep.Results, TFResult{Timeframe: rest, Err: fmt.Sprintf("not attempted: %v", historyErr)})
			}
			break
		}
	}
	rep.FinishedAt = time.Now()
	return rep
}

func (b *Backfiller) Run(ctx context.Context, symbol string) Report {
	return b.run(ctx, symbol, domain.AllTimeframes)
}

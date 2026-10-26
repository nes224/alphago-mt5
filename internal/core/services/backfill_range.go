package services

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

const day = 24 * time.Hour

var DefaultLookback = map[domain.Timeframe]time.Duration{
	domain.TimeframeM1:    30 * day,
	domain.TimeframeM5:    180 * day,
	domain.TimeframeM15:   180 * day,
	domain.TimeframeM30:   180 * day,
	domain.TimeframeH1:    730 * day,
	domain.TimeframeH4:    730 * day,
	domain.TimeframeDaily: 3650 * day,
}

// BackfillFrom returns the time a backfill for tf should start from.
//
//   - hasLatest: start at the newest stored candle (fetch it again so it matches the broker)
//   - otherwise: start at now minus lookback
//
// ok=false means there is nothing to fetch yet: not even one full candle fits before now,
// or tf is unknown.

func BackfillFrom(latest domain.Candle, hasLatest bool, now time.Time, tf domain.Timeframe, lookback time.Duration) (from time.Time, ok bool) {
	dur := tf.Duration()
	if dur == 0 {
		return time.Time{}, false
	}

	if hasLatest {
		from = latest.OpenTime
	} else {
		from = now.Add(-lookback)
	}

	if from.Add(dur).After(now) {
		return time.Time{}, false
	}
	return from, true
}

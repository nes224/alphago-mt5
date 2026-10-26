package services

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func TestBackfillFrom(t *testing.T) {
	now := time.Date(2026, 10, 26, 3, 37, 0, 0, time.UTC)
	at := func(s string) time.Time {
		v, err := time.Parse("2006-01-02 15:04", s)
		if err != nil {
			panic(err)
		}
		return v
	}
	stored := func(s string) domain.Candle { return domain.Candle{OpenTime: at(s)} }

	tests := []struct {
		name      string
		latest    domain.Candle
		hasLatest bool
		tf        domain.Timeframe
		lookback  time.Duration
		wantFrom  time.Time
		wantOK    bool
	}{
		{"first run uses lookback", domain.Candle{}, false, domain.TimeframeM1, 30 * day, at("2026-09-26 03:37"), true},
		{"resumes from newest stored candle", stored("2026-10-26 03:06"), true, domain.TimeframeM1, 30 * day, at("2026-10-26 03:06"), true},
		{"over a weekend", stored("2026-10-23 20:59"), true, domain.TimeframeM1, 30 * day, at("2026-10-23 20:59"), true},
		{"H4 resumes from stored candle", stored("2026-10-25 20:00"), true, domain.TimeframeH4, 730 * day, at("2026-10-25 20:00"), true},
		{"newest stored candle is still forming", stored("2026-10-26 03:37"), true, domain.TimeframeM1, 30 * day, time.Time{}, false},
		{"lookback shorter than one candle", domain.Candle{}, false, domain.TimeframeH4, time.Hour, time.Time{}, false},
		{"unknown timeframe", domain.Candle{}, false, "W1", 30 * day, time.Time{}, false},
	}
	for _, tt := range tests {
		gotFrom, gotOK := BackfillFrom(tt.latest, tt.hasLatest, now, tt.tf, tt.lookback)
		if gotOK != tt.wantOK || !gotFrom.Equal(tt.wantFrom) {
			t.Errorf("%s: got (%v, %v), want (%v, %v)", tt.name, gotFrom, gotOK, tt.wantFrom, tt.wantOK)
		}
	}
}

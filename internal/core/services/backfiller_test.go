package services

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type rateCall struct {
	tf   domain.Timeframe
	from time.Time
}

type fakeHistory struct {
	now      time.Time
	timeErr  error
	failOn   map[domain.Timeframe]error
	requests []rateCall
}

type fakeStore struct {
	data   map[domain.Timeframe]map[int64]domain.Candle
	failOn map[domain.Timeframe]error
}

var testNow = time.Date(2026, 10, 26, 3, 37, 30, 0, time.UTC)

func quite() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func (f *fakeHistory) ServerTime(context.Context, string) (time.Time, error) {
	return f.now, f.timeErr
}

func (f *fakeHistory) Rates(_ context.Context, symbol string, tf domain.Timeframe, from, to time.Time) ([]domain.Candle, error) {
	f.requests = append(f.requests, rateCall{tf, from})
	if err := f.failOn[tf]; err != nil {
		return nil, err
	}

	dur := tf.Duration()
	var out []domain.Candle
	for t := from.Truncate(dur); !t.After(to); t = t.Add(dur) {
		if t.Before(from) {
			continue
		}
		out = append(out, domain.Candle{
			Symbol: symbol, Timeframe: tf, OpenTime: t,
			Open: 1, High: 2, Low: 0.5, Close: 1.5,
			Closed: !t.Add(dur).After(f.now),
		})
	}

	return out, nil
}

func newFakeStore() *fakeStore {
	return &fakeStore{data: map[domain.Timeframe]map[int64]domain.Candle{}, failOn: map[domain.Timeframe]error{}}
}

func (s *fakeStore) LatestCandle(_ string, tf domain.Timeframe) (domain.Candle, bool, error) {
	if err := s.failOn[tf]; err != nil {
		return domain.Candle{}, false, err
	}

	var latest domain.Candle
	ok := false
	for _, c := range s.data[tf] {
		if !ok || c.OpenTime.After(latest.OpenTime) {
			latest, ok = c, true
		}
	}

	return latest, ok, nil
}

func (s *fakeStore) WriteCandles(_ string, tf domain.Timeframe, candles []domain.Candle) error {
	if s.data[tf] == nil {
		s.data[tf] = map[int64]domain.Candle{}
	}
	for _, c := range candles {
		if c.Closed {
			s.data[tf][c.OpenTime.Unix()] = c
		}
	}

	return nil
}

func TestBackfiller_FirstRunFillsEveryTimeframe(t *testing.T) {
	h := &fakeHistory{}
	s := newFakeStore()
	lookback := map[domain.Timeframe]time.Duration{}
	for _, tf := range domain.AllTimeframes {
		lookback[tf] = 10 * tf.Duration()
	}

	rep := NewBackfiller(h, s, lookback, quite()).Run(context.Background(), "XAUUSDm")

	if !rep.OK() || len(rep.Results) != len(domain.AllTimeframes) {
		t.Fatalf("report = %+v", rep)
	}
	for i, tf := range domain.AllTimeframes {
		if got := h.requests[i]; got.tf != tf || !got.from.Equal(testNow.Add(-lookback[tf])) {
			t.Errorf("request %d = %+v, want %s from now-lookback", i, got, tf)
		}
		res := rep.Results[i]
		if res.Written == 0 || res.Written != len(s.data[tf]) {
			t.Errorf("%s: written %d, stored %d", tf, res.Written, len(s.data[tf]))
		}
		for _, c := range s.data[tf] {
			if !c.Closed {
				t.Errorf("%s: forming candle %v was stored", tf, c.OpenTime)
			}
		}
	}

}

package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
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

func TestBackfiller_SecondRunResumesFromLatest(t *testing.T) {
	h := &fakeHistory{now: testNow}
	s := newFakeStore()
	b := NewBackfiller(h, s, map[domain.Timeframe]time.Duration{domain.TimeframeM1: time.Hour}, quite())

	first := b.RunTF(context.Background(), "XAUUSDm", domain.TimeframeM1)
	latest := *first.Results[0].Latest

	h.now = testNow.Add(10 * time.Minute)
	h.requests = nil
	second := b.RunTF(context.Background(), "XAUUSDm", domain.TimeframeM1)

	if !h.requests[0].from.Equal(latest) {
		t.Errorf("second run from %v, want latest stored %v", h.requests[0].from, latest)
	}
	if got := second.Results[0].Written; got != 11 {
		t.Errorf("written = %d, want 11", got)
	}
	if want := first.Results[0].Written + 10; len(s.data[domain.TimeframeM1]) != want {
		t.Errorf("stored = %d, want %d (no duplicates)", len(s.data[domain.TimeframeM1]), want)
	}

}

func TestBackfiller_StoreErrorSkipsOnlyThatTimeframe(t *testing.T) {
	h := &fakeHistory{now: testNow}
	s := newFakeStore()
	s.failOn[domain.TimeframeM5] = errors.New("corrupt file")

	rep := NewBackfiller(h, s, nil, quite()).Run(context.Background(), "XAUUSDm")

	if rep.OK() {
		t.Fatal("report should not be OK")
	}

	for _, res := range rep.Results {
		bad := res.Timeframe == domain.TimeframeM5
		if bad != (res.Err != "") {
			t.Errorf("%s: err = %q", res.Timeframe, res.Err)
		}
	}
	if len(h.requests) != len(domain.AllTimeframes)-1 {
		t.Errorf("requests = %d, want %d", len(h.requests), len(domain.AllTimeframes)-1)
	}
}

// an EA failure stops the run: the remaining timeframes are reported as not attempted
func TestBackfiller_HistoryErrorStopsTheRun(t *testing.T) {
	h := &fakeHistory{now: testNow, failOn: map[domain.Timeframe]error{domain.TimeframeM15: errors.New("EA di not respond")}}
	rep := NewBackfiller(h, newFakeStore(), map[domain.Timeframe]time.Duration{
		domain.TimeframeM1: time.Hour, domain.TimeframeM5: time.Hour, domain.TimeframeM15: time.Hour,
	}, quite()).Run(context.Background(), "XAUUSDm")

	if len(h.requests) != 3 {
		t.Errorf("requests = %d, want 3 (M1, M5, M15)", len(h.requests))
	}
	if len(rep.Results) != len(domain.AllTimeframes) {
		t.Fatalf("results = %d, want one per timeframe", len(rep.Results))
	}
	for _, res := range rep.Results[3:] {
		if !strings.HasPrefix(res.Err, "not attempted") {
			t.Errorf("%s: err = %q, want not attempted", res.Timeframe, res.Err)
		}
	}
}

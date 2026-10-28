package services

import (
	"context"
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
	data map[domain.Timeframe]map[int64]domain.Candle
	failOn map[domain.Timeframe]error
}

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
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

func (f *fakeHistory) ServerTime(context.Context, string) (time.Time, error) {
	return f.now, f.timeErr
}

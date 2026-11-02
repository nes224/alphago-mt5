package services

import (
	"context"
	"sync/atomic"
	"time"
)

// gatedHistory blocks every run in ServerTime until the test lets it go,
// so the test controls exactly when a run is "in progress".
type gatedHistory struct {
	calls   atomic.Int32
	release chan struct{}
}

func (g *gatedHistory) ServerTime(ctx context.Context, _ string) (time.Time, error) {
	g.calls.Add(1)
	select {
	case <-g.release:
	case <-ctx.Done():
		return time.Time{}, ctx.Err()
	}

	return testNow, nil
}


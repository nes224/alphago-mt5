package services

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
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

func (g *gatedHistory) Rates(context.Context, string, domain.Timeframe, time.Time, time.Time) ([]domain.Candle, error) {
	return nil, nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

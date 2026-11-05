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

func startRunner(t *testing.T, h *gatedHistory, symbols ...string) *BackfillRunner {
	t.Helper()
	r := NewBackfillRunner(NewBackfiller(h, newFakeStore(), nil, discardLog()), symbols)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return r
}

func TestBackfillRunner_TriggersDuringRunQueueOneRerun(t *testing.T) {
	h := &gatedHistory{release: make(chan struct{})}
	r := startRunner(t, h, "XAUUSDm")
	r.Trigger()

	waitFor(t, "first run to start", func() bool { return h.calls.Load() == 1 })
	if !r.Status().Running {
		t.Error("status should say running")
	}

	for i := 0; i < 5; i++ {
		r.Trigger()
	}

	h.release <- struct{}{}
	waitFor(t, "second run to start", func() bool { return h.calls.Load() == 2 })
	h.release <- struct{}{}
	waitFor(t, "second run to finish", func() bool { return r.Status().Runs == 2 })

	time.Sleep(20 * time.Millisecond)
	if got := h.calls.Load(); got != 2 {
		t.Errorf("runs started = %d, want 2", got)
	}
}

func TestBackfillRunner_FirstDoneAndStatus(t *testing.T) {
	h := &gatedHistory{release: make(chan struct{})}
	r := startRunner(t, h, "XAUUSDm", "BTCUSDm")

	select {
	case <-r.FirstDone():
		t.Fatal("FirstDone closed before any run")
	default:
	}

	r.Trigger()
	h.release <- struct{}{} // XAUUSDm
	h.release <- struct{}{} // BTCUSDm

	select {
	case <-r.FirstDone():
	case <-time.After(time.Second):
		t.Fatal("FirstDone not closed after the first run")
	}

	st := r.Status()
	if st.Running || st.Runs != 1 || len(st.Last) != 2 || st.Last[0].Symbol != "XAUUSDm" || st.Last[1].Symbol != "BTCUSDm" {
		t.Errorf("status = %+v", st)
	}

}

// Run must return when its context is cancelled, even in the middle of a run
func TestBackfillRunner_StopsOneCancel(t *testing.T) {
	h := &gatedHistory{release: make(chan struct{})}
	r := NewBackfillRunner(NewBackfiller(h, newFakeStore(), nil, discardLog()), []string{"XAUUSDm"})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx); close(done) }()

	r.Trigger()
	waitFor(t, "run to start", func() bool { return h.calls.Load() == 1 })
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

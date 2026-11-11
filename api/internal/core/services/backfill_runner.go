package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// BackfillRunner runs the Backfiller for every symbol in a single goroutine.
// Triggers never block and never overlap: a trigger that arrives while a run is
// in progress queues exactly one more run, however many times it is called.
type BackfillRunner struct {
	b       *Backfiller
	symbols []string
	trigger chan struct{}

	// RetryAfter is how long to wait before running again after a failed run.
	RetryAfter time.Duration

	running atomic.Bool
	runs    atomic.Int64

	mu   sync.Mutex
	last map[string]Report

	firstOnce sync.Once
	firstDone chan struct{}
}

// BackfillStatus is a snapshot for /health and the API.
type BackfillStatus struct {
	Running bool     `json:"running"`
	Runs    int64    `json:"runs"`
	Last    []Report `json:"last"`
}

func NewBackfillRunner(b *Backfiller, symbols []string) *BackfillRunner {
	return &BackfillRunner{
		b:          b,
		symbols:    symbols,
		trigger:    make(chan struct{}, 1),
		RetryAfter: 30 * time.Second,
		last:       make(map[string]Report),
		firstDone:  make(chan struct{}),
	}
}

func (r *BackfillRunner) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

func (r *BackfillRunner) Run(ctx context.Context) {
	var retry <-chan time.Time // nil = no retry pending (a nil channel never fires)
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.trigger:
		case <-retry:
		}
		retry = nil
		if ok := r.runOnce(ctx); !ok && ctx.Err() == nil {
			retry = time.After(r.RetryAfter)
		}
	}
}

// runOnce backfills every symbol and reports whether all of them succeeded.
func (r *BackfillRunner) runOnce(ctx context.Context) bool {
	r.running.Store(true)
	defer r.running.Store(false)

	ok := true
	for _, sym := range r.symbols {
		if ctx.Err() != nil {
			return false
		}
		rep := r.b.Run(ctx, sym)
		ok = ok && rep.OK()
		r.mu.Lock()
		r.last[sym] = rep
		r.mu.Unlock()
	}
	r.runs.Add(1)
	r.firstOnce.Do(func() { close(r.firstDone) })
	return ok
}

func (r *BackfillRunner) Status() BackfillStatus {
	st := BackfillStatus{Running: r.running.Load(), Runs: r.runs.Load()}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sym := range r.symbols {
		if rep, ok := r.last[sym]; ok {
			st.Last = append(st.Last, rep)
		}
	}
	return st
}

func (r *BackfillRunner) FirstDone() <-chan struct{} { return r.firstDone }

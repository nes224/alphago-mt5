package services

import (
	"context"
	"sync"
	"sync/atomic"
)

// BackfillRunner runs the Backfiller for every symbol in a single goroutine.
// Triggers never block and never overlap: a trigger that arrives while a run is
// in progress queues exactly one more run, however many times it is called.
type BackfillRunner struct {
	b       *Backfiller
	symbols []string
	trigger chan struct{}

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
		b:         b,
		symbols:   symbols,
		trigger:   make(chan struct{}, 1),
		last:      make(map[string]Report),
		firstDone: make(chan struct{}),
	}
}

func (r *BackfillRunner) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

func (r *BackfillRunner) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.trigger:
			r.runOnce(ctx)
		}
	}
}

func (r *BackfillRunner) runOnce(ctx context.Context) {
	r.running.Store(true)
	defer r.running.Store(false)

	for _, sym := range r.symbols {
		if ctx.Err() != nil {
			return
		}
		rep := r.b.Run(ctx, sym)
		r.mu.Lock()
		r.last[sym] = rep
		r.mu.Unlock()
	}
	r.runs.Add(1)
	r.firstOnce.Do(func() { close(r.firstDone) })
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

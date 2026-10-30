package services

import (
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

func NewBackfillRunner(b *Backfiller, symbols []string) *BackfillRunner {
	return &BackfillRunner{
		b:         b,
		symbols:   symbols,
		trigger:   make(chan struct{}, 1),
		last:      make(map[string]Report),
		firstDone: make(chan struct{}),
	}
}


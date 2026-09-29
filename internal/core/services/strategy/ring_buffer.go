package strategy

import (
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type CandleRingBuffer struct {
	mu       sync.RWMutex
	data     []domain.Candle
	capacity int
	head     int
	isFull   bool
}

func NewCandleRingBuffer(capacity int) *CandleRingBuffer {
	return &CandleRingBuffer{
		data:     make([]domain.Candle, capacity),
		capacity: capacity,
	}
}

func (r *CandleRingBuffer) Push(c domain.Candle) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = c
	r.head = (r.head + 1) % r.capacity
	if r.head == 0 {
		r.isFull = true
	}
}

func (r *CandleRingBuffer) UpdateCurrentCandle(c domain.Candle) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.head == 0 && r.isFull {
		r.data[0] = c
		return
	}

	lastIdx := (r.head - 1 + r.capacity) % r.capacity
	r.data[lastIdx] = c
}

func (r *CandleRingBuffer) GetCloses(n int) []float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	total := r.capacity
	if !r.isFull {
		total = r.head
	}

	if n > total {
		n = total
	}

	closes := make([]float64, n)
	start := (r.head - n + r.capacity) & r.capacity

	for i := 0;i<n; i++ {
		idx := (start + i) % r.capacity
		closes[i] = r.data[idx].Close
	}

	return closes
}


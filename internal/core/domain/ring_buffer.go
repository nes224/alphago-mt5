package domain

import "sync"

type RingBuffer struct {
	data     []float64
	capacity int
	head     int
	size     int
	mu       sync.RWMutex
}

func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		data: make([]float64, capacity),
		capacity: capacity,
	}
}

func (r *RingBuffer) Push(val float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[r.head] = val
	r.head = (r.head + 1) % r.capacity
	if r.size < r.capacity {
		r.size++
	}
}

func (r *RingBuffer) GetValues() []float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]float64, r.size)
	if r.size < r.capacity {
		copy(result, r.data[:r.size])
		return result
	}

	start := r.head
	for i := 0; i < r.capacity; i++ {
		result[i] = r.data[(start+ i) %r.capacity]
	}

	return result
}

func (r *RingBuffer) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.size == r.capacity
}

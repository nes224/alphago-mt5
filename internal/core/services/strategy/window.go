package strategy

import "math"

type Window struct {
	data     []float64
	capacity int
}

func NewWindow(capacity int) *Window {
	return &Window{
		data:     make([]float64, 0, capacity),
		capacity: capacity,
	}
}

func (w *Window) Push(v float64) {
	w.data = append(w.data, v)
	if len(w.data) > w.capacity {
		w.data = w.data[1:]
	}
}

func (w *Window) Size() int {
	return len(w.data)
}

func (w *Window) Mean() float64 {
	n := float64(len(w.data))
	if n == 0 {
		return 0
	}

	var sum float64
	for _, v := range w.data {
		sum += v
	}

	return sum / n
}

func (w *Window) StdDev() float64 {
	n := float64(len(w.data))
	if n == 0 {
		return 0
	}

	mean := w.Mean()
	var varianceSum float64
	for _, v := range w.data {
		varianceSum += math.Pow(v-mean, 2)
	}

	return math.Sqrt(varianceSum / n)
}

func (w *Window) Slope() float64 {
	n := float64(len(w.data))
	if n < 2 {
		return 0
	}

	var sumX, sumY, sumXY, sumX2 float64
	for i, v := range w.data {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumX2 += x * x
	}

	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}

	return (n*sumXY - sumX*sumY) / denominator
}

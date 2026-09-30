package strategy

import "math"

// Window is a fixed-capacity rolling buffer of prices used to compute
// a running mean/stddev per symbol for z-score calculations.
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

// Slope returns the least-squares linear regression slope of the buffered
// prices against their tick index (oldest=0 .. newest=n-1). It approximates
// how strongly the market is trending: a slope near 0 means sideways/ranging,
// a large |slope| means a directional trend.
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

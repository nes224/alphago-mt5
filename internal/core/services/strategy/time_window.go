package strategy

import (
	"math"
	"time"
)

type TimeWindow struct {
	duration time.Duration
	points   []timedPoint
}

type timedPoint struct {
	t     time.Time
	price float64
}

func NewTimeWindow(duration time.Duration) *TimeWindow {
	return &TimeWindow{duration: duration}
}

func (tw *TimeWindow) Push(t time.Time, price float64) {
	tw.points = append(tw.points, timedPoint{t: t, price: price})

	cutoff := t.Add(-tw.duration)
	evictUntil := 0
	for evictUntil < len(tw.points) && tw.points[evictUntil].t.Before(cutoff) {
		evictUntil++
	}
	if evictUntil > 0 {
		tw.points = tw.points[evictUntil:]
	}
}

func (tw *TimeWindow) Size() int {
	return len(tw.points)
}

func (tw *TimeWindow) Span() time.Duration {
	if len(tw.points) < 2 {
		return 0
	}
	return tw.points[len(tw.points)-1].t.Sub(tw.points[0].t)
}

func (tw *TimeWindow) Slope() float64 {
	n := float64(len(tw.points))
	if n < 2 {
		return 0
	}

	t0 := tw.points[0].t
	var sumX, sumY, sumXY, sumX2 float64
	for _, p := range tw.points {
		x := p.t.Sub(t0).Seconds()
		y := p.price
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}

	denominator := n*sumX2 - sumX*sumX
	if denominator == 0 {
		return 0
	}

	return (n*sumXY - sumX*sumY) / denominator
}

func (tw *TimeWindow) Mean() float64 {
	n := float64(len(tw.points))
	if n == 0 {
		return 0
	}

	var sum float64
	for _, p := range tw.points {
		sum += p.price
	}
	return sum / n
}

func (tw *TimeWindow) StdDev() float64 {
	n := float64(len(tw.points))
	if n == 0 {
		return 0
	}

	mean := tw.Mean()
	var varianceSum float64
	for _, p := range tw.points {
		varianceSum += math.Pow(p.price-mean, 2)
	}
	return math.Sqrt(varianceSum / n)
}

const minDirectionWarmupFraction = 0.8
func (tw *TimeWindow) Direction(minSlope float64) int {
	if tw.Size() < 2 {
		return 0
	}
	if tw.Span() < time.Duration(float64(tw.duration)*minDirectionWarmupFraction) {
		return 0
	}
	s := tw.Slope()
	if s > minSlope {
		return 1
	}
	if s < -minSlope {
		return -1
	}
	return 0
}

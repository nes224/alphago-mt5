package strategy

import (
	"math"
	"time"
)

// TimeWindow is a rolling buffer that evicts entries older than `duration`
// relative to the timestamp of the latest push — not a fixed tick count.
// This is what lets a window actually represent a named timeframe (H4,
// Daily, ...) regardless of how many ticks arrive per second, unlike the
// old tick-count Window used for the Dual-Window Trend Filter: that one
// was SUPERSEDED (2026-09-30, see ROADMAP.md) exactly because a fixed tick
// count could span anywhere from 2 to 20 minutes depending on how busy the
// market was, so it never lined up with any real timeframe.
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

// Push appends a new price at timestamp t and evicts anything older than
// `duration` before t — using t (the tick's own timestamp) as "now" rather
// than time.Now(), so this also works correctly against historical/replayed
// data, not just live ticks.
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

// Span returns how much wall-clock time the currently buffered points cover
// — useful to tell "genuinely flat" apart from "not enough history yet"
// (e.g. a fresh Daily window 10 minutes after startup has Span() of ~10m,
// nowhere near a full day, so its Slope() isn't meaningful yet).
func (tw *TimeWindow) Span() time.Duration {
	if len(tw.points) < 2 {
		return 0
	}
	return tw.points[len(tw.points)-1].t.Sub(tw.points[0].t)
}

// Slope returns the least-squares linear regression slope of price against
// elapsed seconds since the oldest buffered point — NOT tick index — so it
// stays comparable across windows regardless of tick density.
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

// Mean returns the average price of the currently buffered points.
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

// StdDev returns the standard deviation of the currently buffered prices —
// used as the volatility measure for position sizing (RiskManager), kept
// separate from the tick-count Window's StdDev (which reacts to the most
// recent handful of ticks, too jumpy for sizing a stop distance).
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

// minDirectionWarmupFraction is how much of a TimeWindow's configured
// duration must actually be covered by buffered points (Span()) before
// Direction() will trust Slope() at all — otherwise a window that's only
// seen a handful of ticks spanning a few seconds could report a confident
// "Daily trend" that's really just noise (two ticks a few seconds apart
// will always have SOME slope). Treated identically to "not enough data"
// (returns 0 = fail-open for any caller, e.g. MultiTimeframeFilter). 0.8 is
// an estimate, not backtested — see cmd/app/main.go's own convention of
// flagging unvalidated constants.
const minDirectionWarmupFraction = 0.8

// Direction returns +1 (up), -1 (down), or 0 (flat / not enough data / not
// warmed up yet) based on whether |Slope()| clears minSlope. Span() must
// cover at least minDirectionWarmupFraction of the window's configured
// duration, or this returns 0 regardless of slope — see
// minDirectionWarmupFraction.
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

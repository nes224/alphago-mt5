package strategy

import "time"

// RollingCVD tracks Cumulative Volume Delta over a rolling time window: each
// tick is classified buy/sell aggressor via the tick rule (uptick in
// mid-price = buy, downtick = sell, unchanged = inherit the previous
// classification), its volume is signed accordingly, and only volume from
// ticks within `duration` of the latest tick counts — evicted the same way
// TimeWindow evicts by wall-clock time (each point's own timestamp, not
// time.Now()), but keeping a running sum instead of mean/stddev so eviction
// stays O(1) amortized per tick instead of rescanning.
//
// Feed this |volumeDelta| (the "how much happened on this tick" quantity
// QuantEngine.calculateMetrics already computes) — NOT raw tick.Volume.
// tick.Volume is broker-side cumulative (same shape as OpenInterest, see the
// comment on domain.TickMetrics.VolumeDelta); using it directly would make
// CVD a running total of a running total.
type RollingCVD struct {
	duration time.Duration
	points   []cvdPoint
	sum      float64

	hasLastMid bool
	lastMid    float64
	lastSign   float64 // +1 or -1, carried forward through unchanged ticks
}

type cvdPoint struct {
	t      time.Time
	signed float64
}

func NewRollingCVD(duration time.Duration) *RollingCVD {
	return &RollingCVD{duration: duration, lastSign: 1}
}

// Push classifies the tick at (t, midPrice) via the tick rule and adds
// sign * |volumeDelta| to the rolling sum.
func (c *RollingCVD) Push(t time.Time, midPrice float64, volumeDelta int64) {
	sign := c.lastSign
	if c.hasLastMid {
		if midPrice > c.lastMid {
			sign = 1
		} else if midPrice < c.lastMid {
			sign = -1
		}
	}
	c.lastSign, c.lastMid, c.hasLastMid = sign, midPrice, true

	vol := float64(volumeDelta)
	if vol < 0 {
		vol = -vol
	}
	signedVol := sign * vol

	c.points = append(c.points, cvdPoint{t: t, signed: signedVol})
	c.sum += signedVol

	cutoff := t.Add(-c.duration)
	evictUntil := 0
	for evictUntil < len(c.points) && c.points[evictUntil].t.Before(cutoff) {
		c.sum -= c.points[evictUntil].signed
		evictUntil++
	}
	if evictUntil > 0 {
		c.points = c.points[evictUntil:]
	}
}

// Value returns the current rolling Cumulative Volume Delta (positive = net
// buy pressure, negative = net sell pressure, over the configured duration).
func (c *RollingCVD) Value() float64 { return c.sum }

func (c *RollingCVD) Size() int { return len(c.points) }

// Span returns the wall-clock time covered by currently buffered points —
// same purpose as TimeWindow.Span(): tells "CVD genuinely represents the
// configured rolling window" apart from "just started, not warmed up yet".
func (c *RollingCVD) Span() time.Duration {
	if len(c.points) < 2 {
		return 0
	}
	return c.points[len(c.points)-1].t.Sub(c.points[0].t)
}

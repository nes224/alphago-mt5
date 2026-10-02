package strategy

import "time"

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

func (c *RollingCVD) Value() float64 { return c.sum }

func (c *RollingCVD) Size() int { return len(c.points) }

func (c *RollingCVD) Span() time.Duration {
	if len(c.points) < 2 {
		return 0
	}
	return c.points[len(c.points)-1].t.Sub(c.points[0].t)
}

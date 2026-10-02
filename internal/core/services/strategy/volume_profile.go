package strategy

import (
	"math"
	"time"
)

// RollingVolumeProfile buckets recent trade volume by price into fixed-size
// buckets over a rolling time window, to find the current Point of Control
// (POC) — the price bucket with the most volume recently. This bot runs 24/5
// on a CFD gold symbol with no clean session boundary, so this is
// intentionally rolling/duration-based (evicts like TimeWindow), not a
// session-anchored profile.
//
// POC() rescans the bucket map, not the tick list — this is NOT O(n ticks).
// It's O(distinct price buckets currently in the window), which for XAUUSDm
// moving a handful of dollars per hour at a $0.50 bucket size is roughly
// 10-100 buckets: trivially cheap to rescan every tick. An incremental
// max-tracker would add real bookkeeping (an evicted bucket dropping below
// the current max still requires a rescan to find the new max, so it doesn't
// actually avoid the rescan) for no measurable win at this tick rate.
type RollingVolumeProfile struct {
	bucketSize float64
	duration   time.Duration
	points     []volumePoint
	buckets    map[int64]float64
}

type volumePoint struct {
	t         time.Time
	bucketKey int64
	volume    float64
}

func NewRollingVolumeProfile(bucketSize float64, duration time.Duration) *RollingVolumeProfile {
	return &RollingVolumeProfile{
		bucketSize: bucketSize,
		duration:   duration,
		buckets:    make(map[int64]float64),
	}
}

func (p *RollingVolumeProfile) bucketKeyFor(price float64) int64 {
	return int64(math.Floor(price / p.bucketSize))
}

// Push adds `volume` (pass |VolumeDelta| — see RollingCVD's doc comment,
// same reasoning, magnitude only, direction is irrelevant for a profile) to
// the bucket containing `price`, then evicts points older than `duration`.
func (p *RollingVolumeProfile) Push(t time.Time, price, volume float64) {
	key := p.bucketKeyFor(price)
	p.points = append(p.points, volumePoint{t: t, bucketKey: key, volume: volume})
	p.buckets[key] += volume

	cutoff := t.Add(-p.duration)
	evictUntil := 0
	for evictUntil < len(p.points) && p.points[evictUntil].t.Before(cutoff) {
		old := p.points[evictUntil]
		p.buckets[old.bucketKey] -= old.volume
		if p.buckets[old.bucketKey] <= 1e-9 {
			delete(p.buckets, old.bucketKey)
		}
		evictUntil++
	}
	if evictUntil > 0 {
		p.points = p.points[evictUntil:]
	}
}

// POC returns the price at the center of the highest-volume bucket currently
// in the window, that bucket's volume, and ok=false if the window is empty.
func (p *RollingVolumeProfile) POC() (pocPrice, pocVolume float64, ok bool) {
	if len(p.buckets) == 0 {
		return 0, 0, false
	}
	var bestKey int64
	bestVol := -1.0
	for k, v := range p.buckets {
		if v > bestVol {
			bestVol, bestKey = v, k
		}
	}
	return (float64(bestKey) + 0.5) * p.bucketSize, bestVol, true
}

// VolumeAt returns the current aggregate volume in the bucket containing
// price — e.g. "is there real recent volume right at the current tick".
func (p *RollingVolumeProfile) VolumeAt(price float64) float64 {
	return p.buckets[p.bucketKeyFor(price)]
}

// Span returns the wall-clock time covered by currently buffered points —
// same warm-up purpose as TimeWindow.Span()/RollingCVD.Span().
func (p *RollingVolumeProfile) Span() time.Duration {
	if len(p.points) < 2 {
		return 0
	}
	return p.points[len(p.points)-1].t.Sub(p.points[0].t)
}

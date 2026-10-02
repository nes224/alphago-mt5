package strategy

import (
	"math"
	"time"
)

// ATRCalculator computes a Wilder-smoothed Average True Range from synthetic
// M5 (5-minute) periods built directly from ticks — no full OHLC candle
// builder, just a running High/Low/last-price per period (per ROADMAP.md
// "ATR แทน StdDev(M15)"). Periods are bucketed by truncating each tick's OWN
// timestamp to the period boundary (time.Truncate), not time.Now(),
// consistent with TimeWindow's historical-replay-safe design.
//
// Known, accepted limitation: a weekend/connection gap spanning multiple
// empty 5-min buckets is not backfilled — Push just jumps straight to the
// new bucket, silently skipping the empty ones (same spirit as TimeWindow
// going sparse under low tick rate). The first True Range computed after
// such a gap can spike because prevClose is from before the gap — this is a
// real limitation of actual ATR on any gapping market too, not specific to
// this implementation.
type ATRCalculator struct {
	periodDuration time.Duration
	numPeriods     int

	hasBucket   bool
	bucketStart time.Time
	high, low   float64
	lastPrice   float64

	hasPrevClose bool
	prevClose    float64

	trValues []float64 // buffered True Ranges until numPeriods are seen (seed phase)
	atr      float64
	seeded   bool
}

func NewATRCalculator(periodDuration time.Duration, numPeriods int) *ATRCalculator {
	return &ATRCalculator{
		periodDuration: periodDuration,
		numPeriods:     numPeriods,
		trValues:       make([]float64, 0, numPeriods),
	}
}

// Push feeds one tick's mid-price at timestamp t.
func (a *ATRCalculator) Push(t time.Time, price float64) {
	bucket := t.Truncate(a.periodDuration)

	if !a.hasBucket {
		a.hasBucket = true
		a.bucketStart = bucket
		a.high, a.low, a.lastPrice = price, price, price
		return
	}

	// Guard against a mildly out-of-order tick (network jitter) landing in a
	// bucket before the current one — fold it into the current bucket rather
	// than rolling the period backward and corrupting state.
	if bucket.Before(a.bucketStart) {
		bucket = a.bucketStart
	}

	if bucket.Equal(a.bucketStart) {
		if price > a.high {
			a.high = price
		}
		if price < a.low {
			a.low = price
		}
		a.lastPrice = price
		return
	}

	// Period rollover: finalize the period that just ended using data
	// accumulated BEFORE this tick, then start the new period with this tick.
	a.finalizePeriod()
	a.bucketStart = bucket
	a.high, a.low, a.lastPrice = price, price, price
}

func (a *ATRCalculator) finalizePeriod() {
	closePrice := a.lastPrice

	var tr float64
	if a.hasPrevClose {
		tr = math.Max(a.high-a.low, math.Max(math.Abs(a.high-a.prevClose), math.Abs(a.low-a.prevClose)))
	} else {
		tr = a.high - a.low // first period ever: no prior close to gap against
	}
	a.prevClose, a.hasPrevClose = closePrice, true

	if !a.seeded {
		a.trValues = append(a.trValues, tr)
		if len(a.trValues) >= a.numPeriods {
			var sum float64
			for _, v := range a.trValues {
				sum += v
			}
			a.atr = sum / float64(a.numPeriods)
			a.seeded = true
			a.trValues = nil
		}
		return
	}

	n := float64(a.numPeriods)
	a.atr = (a.atr*(n-1) + tr) / n // Wilder smoothing
}

// Value returns the current ATR and whether it's ready (seeded with
// numPeriods completed periods — ~70 min for the default M5×14).
func (a *ATRCalculator) Value() (atr float64, ready bool) {
	return a.atr, a.seeded
}

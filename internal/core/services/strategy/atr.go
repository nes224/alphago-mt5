package strategy

import (
	"math"
	"time"
)

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

func (a *ATRCalculator) Value() (atr float64, ready bool) {
	return a.atr, a.seeded
}

// Package indicator computes technical indicators over a series of candles.
// Every function returns one value per input candle, aligned by index;
// values that cannot be computed yet (not enough history) are NaN or 0.
package indicator

import (
	"math"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

// Closes returns the close price of every candle.
func Closes(c []domain.Candle) []float64 {
	out := make([]float64, len(c))
	for i, x := range c {
		out[i] = x.Close
	}
	return out
}

// SMA is the simple moving average of the last n values. out[i] is NaN for i < n-1.
func SMA(v []float64, n int) []float64 {
	out := make([]float64, len(v))
	sum := 0.0
	for i := range v {
		sum += v[i]
		if i >= n {
			sum -= v[i-n]
		}
		if i >= n-1 {
			out[i] = sum / float64(n)
		} else {
			out[i] = math.NaN()
		}
	}
	return out
}

// TrueRange of candle i: the largest of high-low and the gaps from the previous close.
func TrueRange(c []domain.Candle, i int) float64 {
	hl := c[i].High - c[i].Low
	if i == 0 {
		return hl
	}
	prev := c[i-1].Close
	return math.Max(hl, math.Max(math.Abs(c[i].High-prev), math.Abs(c[i].Low-prev)))
}

// ATR is Wilder's average true range over n candles. The first value (index n-1)
// is the plain average of the first n true ranges; out[i] is NaN before that.
func ATR(c []domain.Candle, n int) []float64 {
	out := make([]float64, len(c))
	sum := 0.0
	for i := range c {
		tr := TrueRange(c, i)
		switch {
		case i < n-1:
			sum += tr
			out[i] = math.NaN()
		case i == n-1:
			sum += tr
			out[i] = sum / float64(n)
		default:
			out[i] = (out[i-1]*float64(n-1) + tr) / float64(n)
		}
	}
	return out
}

// SuperTrend returns the indicator line and the trend direction (+1 up, -1 down,
// 0 while ATR is not ready). n is the ATR period, m the band multiplier.
//
// The lower band only rises while the trend is up and the upper band only falls
// while it is down. The trend turns up when the close breaks above the previous
// upper band and turns down when it breaks below the previous lower band.
func SuperTrend(c []domain.Candle, n int, m float64) (line []float64, dir []int) {
	atr := ATR(c, n)
	line = make([]float64, len(c))
	dir = make([]int, len(c))

	var up, dn float64 // final lower / upper bands of the previous candle
	trend := 0
	for i := range c {
		if math.IsNaN(atr[i]) {
			line[i] = math.NaN()
			continue
		}
		hl2 := (c[i].High + c[i].Low) / 2
		basicUp := hl2 - m*atr[i]
		basicDn := hl2 + m*atr[i]

		if trend == 0 { // first candle with an ATR value
			up, dn, trend = basicUp, basicDn, 1
			line[i], dir[i] = up, trend
			continue
		}

		prevClose := c[i-1].Close
		newUp := basicUp
		if prevClose > up {
			newUp = math.Max(basicUp, up)
		}
		newDn := basicDn
		if prevClose < dn {
			newDn = math.Min(basicDn, dn)
		}

		switch {
		case trend == -1 && c[i].Close > dn:
			trend = 1
		case trend == 1 && c[i].Close < up:
			trend = -1
		}
		up, dn = newUp, newDn

		dir[i] = trend
		if trend == 1 {
			line[i] = up
		} else {
			line[i] = dn
		}
	}
	return line, dir
}

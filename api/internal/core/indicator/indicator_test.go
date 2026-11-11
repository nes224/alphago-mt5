package indicator

import (
	"math"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

var t0 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

// candle with open=close=price and a fixed +/-1 range
func flat(i int, price float64) domain.Candle {
	return domain.Candle{OpenTime: t0.Add(time.Duration(i) * time.Hour), Open: price, High: price + 1, Low: price - 1, Close: price}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestSMA(t *testing.T) {
	got := SMA([]float64{1, 2, 3, 4, 5}, 3)
	if !math.IsNaN(got[0]) || !math.IsNaN(got[1]) {
		t.Fatalf("warm-up values should be NaN: %v", got)
	}
	for i, want := range map[int]float64{2: 2, 3: 3, 4: 4} {
		if !near(got[i], want) {
			t.Errorf("SMA[%d] = %v, want %v", i, got[i], want)
		}
	}
}

func TestTrueRange_UsesGapFromPreviousClose(t *testing.T) {
	c := []domain.Candle{
		{High: 11, Low: 9, Close: 10},
		{High: 16, Low: 14, Close: 15}, // gap up: |16-10| = 6 beats 16-14 = 2
	}
	if got := TrueRange(c, 1); !near(got, 6) {
		t.Errorf("TrueRange = %v, want 6", got)
	}
}

func TestATR_SeedAndWilderSmoothing(t *testing.T) {
	c := []domain.Candle{flat(0, 100), flat(1, 100), flat(2, 100), {High: 106, Low: 100, Close: 104}}
	got := ATR(c, 3)
	if !math.IsNaN(got[1]) {
		t.Errorf("ATR[1] should be NaN, got %v", got[1])
	}
	if !near(got[2], 2) { // three flat candles, TR 2 each
		t.Errorf("ATR[2] = %v, want 2", got[2])
	}
	// TR[3] = max(6, |106-100|, |100-100|) = 6 -> (2*2 + 6) / 3
	if !near(got[3], 10.0/3) {
		t.Errorf("ATR[3] = %v, want %v", got[3], 10.0/3)
	}
}

func TestSuperTrend_FlipsDownOnCrashAndBackUpOnRally(t *testing.T) {
	var c []domain.Candle
	for i := 0; i < 20; i++ { // steady rise
		c = append(c, flat(i, 100+float64(i)))
	}
	for i := 20; i < 30; i++ { // crash
		c = append(c, flat(i, 119-float64(i-19)*5))
	}
	for i := 30; i < 45; i++ { // strong rally
		c = append(c, flat(i, 69+float64(i-29)*5))
	}
	_, dir := SuperTrend(c, 5, 2)

	if dir[3] != 0 {
		t.Errorf("dir before ATR is ready should be 0, got %d", dir[3])
	}
	if dir[19] != 1 {
		t.Errorf("dir during the rise = %d, want 1", dir[19])
	}
	if dir[29] != -1 {
		t.Errorf("dir after the crash = %d, want -1", dir[29])
	}
	if dir[44] != 1 {
		t.Errorf("dir after the rally = %d, want 1", dir[44])
	}
}

func TestSuperTrend_LineSitsBelowPriceInUptrend(t *testing.T) {
	var c []domain.Candle
	for i := 0; i < 30; i++ {
		c = append(c, flat(i, 100+float64(i)))
	}
	line, dir := SuperTrend(c, 10, 3)
	for i := 10; i < len(c); i++ {
		if dir[i] != 1 || line[i] >= c[i].Close {
			t.Fatalf("at %d: dir=%d line=%v close=%v", i, dir[i], line[i], c[i].Close)
		}
		if line[i] < line[i-1] {
			t.Fatalf("lower band fell at %d: %v -> %v", i, line[i-1], line[i])
		}
	}
}

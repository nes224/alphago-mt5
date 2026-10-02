package strategy_test

import (
	"math"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func almostEqual(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

// TestATRCalculator_SeedsAndSmoothsAgainstHandComputedValues drives a
// synthetic tick series through 1-minute periods (numPeriods=3, small on
// purpose so the test doesn't need 14 periods of fixture data) and checks
// the result against hand-computed True Range / Wilder smoothing values.
//
// Periods (bucketed by minute):
//
//	P0 [0:00-0:59]: 100, 105, 98        -> high=105 low=98  close=98
//	P1 [1:00-1:59]: 99, 110, 101        -> high=110 low=99  close=101
//	P2 [2:00-2:59]: 102, 108, 95        -> high=108 low=95  close=95
//	P3 [3:00-3:59]: 100, 103            -> high=103 low=100 close=103
//
// TR0 (no prevClose) = high-low = 105-98 = 7
// TR1 = max(110-99, |110-98|, |99-98|)  = max(11,12,1)  = 12
// TR2 = max(108-95, |108-101|,|95-101|) = max(13,7,6)   = 13
// TR3 = max(103-100,|103-95|, |100-95|) = max(3,8,5)    = 8
//
// Seed (after TR0,TR1,TR2): ATR = (7+12+13)/3 = 10.6667, ready becomes true.
// Next (Wilder, after TR3): ATR = (10.6667*2 + 8)/3 = 9.7778
func TestATRCalculator_SeedsAndSmoothsAgainstHandComputedValues(t *testing.T) {
	a := strategy.NewATRCalculator(1*time.Minute, 3)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	push := func(minSec int, price float64) {
		a.Push(base.Add(time.Duration(minSec)*time.Second), price)
	}

	// P0
	push(0, 100)
	push(20, 105)
	push(40, 98)
	if _, ready := a.Value(); ready {
		t.Fatal("expected not ready before any period has rolled over")
	}

	// Rolls P0 over (TR0=7 buffered, not seeded yet - need 3 TRs)
	push(65, 99)
	if _, ready := a.Value(); ready {
		t.Fatal("expected not ready after only 1 period finalized")
	}

	// P1
	push(80, 110)
	push(90, 101)
	// Rolls P1 over (TR1=12 buffered)
	push(125, 102)
	if _, ready := a.Value(); ready {
		t.Fatal("expected not ready after only 2 periods finalized")
	}

	// P2
	push(140, 108)
	push(150, 95)
	// Rolls P2 over (TR2=13) -> seeds: ATR = (7+12+13)/3 = 10.6667
	push(185, 100)
	atr, ready := a.Value()
	if !ready {
		t.Fatal("expected ready after 3 periods finalized")
	}
	if !almostEqual(atr, 10.6667, 0.001) {
		t.Errorf("expected seeded ATR ~10.6667, got %f", atr)
	}

	// P3
	push(200, 103)
	// Rolls P3 over (TR3=8) -> Wilder: ATR = (10.6667*2 + 8)/3 = 9.7778
	push(250, 97)
	atr, ready = a.Value()
	if !ready {
		t.Fatal("expected still ready after another period")
	}
	if !almostEqual(atr, 9.7778, 0.001) {
		t.Errorf("expected Wilder-smoothed ATR ~9.7778, got %f", atr)
	}
}

func TestATRCalculator_NeverReadyWithoutEnoughCompletedPeriods(t *testing.T) {
	a := strategy.NewATRCalculator(5*time.Minute, 14)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// Feed ticks spanning well under 14*5min = 70 minutes.
	for i := 0; i < 20; i++ {
		a.Push(base.Add(time.Duration(i)*time.Minute), 4000.0+float64(i))
	}

	if _, ready := a.Value(); ready {
		t.Error("expected not ready with fewer than 14 completed 5-min periods")
	}
}

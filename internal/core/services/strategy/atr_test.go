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

package strategy_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func TestRollingCVD_SignsVolumeByTickRule(t *testing.T) {
	c := strategy.NewRollingCVD(1 * time.Hour)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	c.Push(base, 100.0, 10)
	if got := c.Value(); got != 10 {
		t.Fatalf("expected CVD=10 after first (default buy) tick, got %f", got)
	}

	c.Push(base.Add(1*time.Second), 101.0, 5)
	if got := c.Value(); got != 15 {
		t.Fatalf("expected CVD=15 after uptick(+5), got %f", got)
	}

	c.Push(base.Add(2*time.Second), 99.0, 8)
	if got := c.Value(); got != 7 {
		t.Fatalf("expected CVD=7 after downtick(-8), got %f", got)
	}

	c.Push(base.Add(3*time.Second), 99.0, 3)
	if got := c.Value(); got != 4 {
		t.Fatalf("expected CVD=4 after unchanged tick inheriting sell(-3), got %f", got)
	}
}

func TestRollingCVD_TakesAbsOfVolumeDelta(t *testing.T) {
	c := strategy.NewRollingCVD(1 * time.Hour)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	c.Push(base, 100.0, 10)
	c.Push(base.Add(1*time.Second), 101.0, -5)
	if got := c.Value(); got != 15 {
		t.Fatalf("expected CVD=15 (abs(-5)=5 treated as buy), got %f", got)
	}
}

func TestRollingCVD_EvictsOldVolumeOutOfRollingSum(t *testing.T) {
	c := strategy.NewRollingCVD(1 * time.Minute)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	c.Push(base, 100.0, 10)                   // +10, buy (default)
	c.Push(base.Add(10*time.Second), 101, 20) // +20, buy
	if got := c.Value(); got != 30 {
		t.Fatalf("expected CVD=30 before eviction, got %f", got)
	}

	// 2 minutes later: both prior points are outside the 1-minute window.
	c.Push(base.Add(2*time.Minute), 102, 5)
	if got := c.Value(); got != 5 {
		t.Fatalf("expected CVD=5 after old volume evicted, got %f", got)
	}
}

func TestRollingCVD_SpanRequiresAtLeastTwoPoints(t *testing.T) {
	c := strategy.NewRollingCVD(1 * time.Hour)
	if span := c.Span(); span != 0 {
		t.Errorf("expected 0 span with no points, got %v", span)
	}
	c.Push(time.Now(), 100.0, 1)
	if span := c.Span(); span != 0 {
		t.Errorf("expected 0 span with 1 point, got %v", span)
	}
}

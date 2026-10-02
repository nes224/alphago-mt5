package strategy_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func TestTimeWindow_EvictsByDurationNotCount(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Minute)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		tw.Push(base.Add(time.Duration(i)*5*time.Second), 100.0)
	}
	if tw.Size() != 5 {
		t.Fatalf("expected 5 points buffered, got %d", tw.Size())
	}

	tw.Push(base.Add(2*time.Minute), 101.0)
	if tw.Size() != 1 {
		t.Errorf("expected old points evicted leaving 1, got %d", tw.Size())
	}
}

func TestTimeWindow_Slope_UsesElapsedTimeNotTickIndex(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Hour)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	tw.Push(base, 100.0)
	tw.Push(base.Add(1*time.Second), 100.1)
	tw.Push(base.Add(10*time.Second), 101.0)
	tw.Push(base.Add(50*time.Second), 105.0)

	slope := tw.Slope()
	if slope < 0.09 || slope > 0.11 {
		t.Errorf("expected slope ~0.1 (price/sec), got %f", slope)
	}
}

func TestTimeWindow_Direction_FlatWhenBelowThreshold(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Hour)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	tw.Push(base, 100.0)
	tw.Push(base.Add(1*time.Second), 100.0)

	if dir := tw.Direction(0.01); dir != 0 {
		t.Errorf("expected flat direction (0) for zero slope, got %d", dir)
	}
}

func TestTimeWindow_Direction_FailsOpenWhenSpanBelowWarmupFraction(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Hour)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tw.Push(base, 100.0)
	tw.Push(base.Add(5*time.Minute), 200.0)
	if dir := tw.Direction(0.01); dir != 0 {
		t.Errorf("expected 0 (not warmed up, span=5min < 48min threshold), got %d", dir)
	}
}

func TestTimeWindow_Direction_InsufficientDataIsFlat(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Hour)
	tw.Push(time.Now(), 100.0) // only 1 point

	if dir := tw.Direction(0.0); dir != 0 {
		t.Errorf("expected direction 0 with <2 points, got %d", dir)
	}
}

func TestTimeWindow_MeanAndStdDev(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Hour)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	prices := []float64{10, 12, 23, 23, 16, 23, 21, 16}
	for i, p := range prices {
		tw.Push(base.Add(time.Duration(i)*time.Second), p)
	}

	// Known population mean/stddev for this data set.
	if mean := tw.Mean(); mean < 17.99 || mean > 18.01 {
		t.Errorf("expected mean ~18.0, got %f", mean)
	}
	if sd := tw.StdDev(); sd < 4.89 || sd > 4.91 {
		t.Errorf("expected stddev ~4.9, got %f", sd)
	}
}

func TestTimeWindow_StdDev_EmptyIsZero(t *testing.T) {
	tw := strategy.NewTimeWindow(1 * time.Hour)
	if sd := tw.StdDev(); sd != 0 {
		t.Errorf("expected stddev 0 for empty window, got %f", sd)
	}
}

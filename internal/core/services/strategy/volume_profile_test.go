package strategy_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func TestRollingVolumeProfile_POCPicksHighestVolumeBucket(t *testing.T) {
	p := strategy.NewRollingVolumeProfile(0.5, 1*time.Hour)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// Bucket [4000.0, 4000.5): price 4000.1, volume 10+5=15
	p.Push(base, 4000.1, 10)
	p.Push(base.Add(1*time.Second), 4000.2, 5)
	// Bucket [4001.0, 4001.5): price 4001.2, volume 50 -> highest
	p.Push(base.Add(2*time.Second), 4001.2, 50)
	// Bucket [4002.0, 4002.5): price 4002.0, volume 3
	p.Push(base.Add(3*time.Second), 4002.0, 3)

	pocPrice, pocVolume, ok := p.POC()
	if !ok {
		t.Fatal("expected ok=true with data present")
	}
	if pocVolume != 50 {
		t.Errorf("expected POC volume 50, got %f", pocVolume)
	}
	wantCenter := 4001.25 // (4001.0/0.5 floor=8002 -> center = (8002+0.5)*0.5 = 4001.25
	if pocPrice != wantCenter {
		t.Errorf("expected POC price %f, got %f", wantCenter, pocPrice)
	}
}

func TestRollingVolumeProfile_VolumeAtMatchesBucketAggregate(t *testing.T) {
	p := strategy.NewRollingVolumeProfile(1.0, 1*time.Hour)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	p.Push(base, 100.2, 10)
	p.Push(base.Add(1*time.Second), 100.8, 7) // same bucket [100,101)

	if got := p.VolumeAt(100.5); got != 17 {
		t.Errorf("expected aggregate volume 17 in bucket [100,101), got %f", got)
	}
	if got := p.VolumeAt(50.0); got != 0 {
		t.Errorf("expected 0 volume in an untouched bucket, got %f", got)
	}
}

func TestRollingVolumeProfile_EvictsOldVolumeAndPOCShiftsAccordingly(t *testing.T) {
	p := strategy.NewRollingVolumeProfile(1.0, 1*time.Minute)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	// Big volume at 100 early on.
	p.Push(base, 100.5, 100)
	_, vol, _ := p.POC()
	if vol != 100 {
		t.Fatalf("expected initial POC volume 100, got %f", vol)
	}

	// Smaller volume at 200, 2 minutes later: evicts the 100-bucket entirely.
	p.Push(base.Add(2*time.Minute), 200.5, 10)
	pocPrice, pocVolume, ok := p.POC()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pocVolume != 10 {
		t.Errorf("expected POC volume to shift to 10 after eviction, got %f", pocVolume)
	}
	if pocPrice != 200.5 {
		t.Errorf("expected POC price 200.5, got %f", pocPrice)
	}
	if got := p.VolumeAt(100.5); got != 0 {
		t.Errorf("expected evicted bucket to read 0 volume, got %f", got)
	}
}

func TestRollingVolumeProfile_POCEmptyWhenNoData(t *testing.T) {
	p := strategy.NewRollingVolumeProfile(0.5, 1*time.Hour)
	if _, _, ok := p.POC(); ok {
		t.Error("expected ok=false for an empty profile")
	}
}

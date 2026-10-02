package backtest_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
)

func TestSynthesizeTicks_BullishBar_PathIsOpenLowHighClose(t *testing.T) {
	bar := backtest.Bar{
		Time:   time.Date(2026, 1, 30, 23, 50, 0, 0, time.UTC),
		Open:   100.0,
		High:   110.0,
		Low:    95.0,
		Close:  105.0, // Close >= Open -> bullish
		Volume: 100,
	}

	ticks := backtest.SynthesizeTicks(bar, "XAUUSDm", 0.1)

	wantPrices := []float64{100.0, 95.0, 110.0, 105.0} // Open, Low, High, Close
	wantOffsets := []time.Duration{0, 15 * time.Second, 30 * time.Second, 45 * time.Second}
	for i, tick := range ticks {
		if tick.Symbol != "XAUUSDm" {
			t.Errorf("tick %d: expected symbol XAUUSDm, got %s", i, tick.Symbol)
		}
		wantTime := bar.Time.Add(wantOffsets[i])
		if !tick.Timestamp.Equal(wantTime) {
			t.Errorf("tick %d: expected timestamp %v, got %v", i, wantTime, tick.Timestamp)
		}
		mid := (tick.Bid + tick.Ask) / 2
		if mid < wantPrices[i]-1e-9 || mid > wantPrices[i]+1e-9 {
			t.Errorf("tick %d: expected mid price %f, got %f", i, wantPrices[i], mid)
		}
		if spread := tick.Ask - tick.Bid; spread < 0.2-1e-9 || spread > 0.2+1e-9 {
			t.Errorf("tick %d: expected spread 0.2 (2*spreadHalf), got %f", i, spread)
		}
	}

	var totalVolume int64
	for _, tick := range ticks {
		totalVolume += tick.Volume
	}
	if totalVolume != 100 {
		t.Errorf("expected total volume to be preserved (100), got %d", totalVolume)
	}
	if ticks[0].Volume != 25 {
		t.Errorf("expected volume split evenly across 4 ticks (25 each), got %d on first tick", ticks[0].Volume)
	}
}

func TestSynthesizeTicks_BearishBar_PathIsOpenHighLowClose(t *testing.T) {
	bar := backtest.Bar{
		Time:   time.Date(2026, 1, 30, 23, 50, 0, 0, time.UTC),
		Open:   105.0,
		High:   110.0,
		Low:    95.0,
		Close:  100.0, // Close < Open -> bearish
		Volume: 101,
	}

	ticks := backtest.SynthesizeTicks(bar, "XAUUSDm", 0.1)

	wantPrices := []float64{105.0, 110.0, 95.0, 100.0} // Open, High, Low, Close
	for i, tick := range ticks {
		mid := (tick.Bid + tick.Ask) / 2
		if mid < wantPrices[i]-1e-9 || mid > wantPrices[i]+1e-9 {
			t.Errorf("tick %d: expected mid price %f, got %f", i, wantPrices[i], mid)
		}
	}
}

func TestSynthesizeTicks_VolumeRemainderIsDropped(t *testing.T) {
	bar := backtest.Bar{
		Time: time.Date(2026, 1, 30, 23, 50, 0, 0, time.UTC),
		Open: 100, High: 100, Low: 100, Close: 100,
		Volume: 7, // 7/4 = 1 remainder 3 -- remainder documented as dropped
	}

	ticks := backtest.SynthesizeTicks(bar, "XAUUSDm", 0.1)

	var total int64
	for _, tick := range ticks {
		if tick.Volume != 1 {
			t.Errorf("expected each tick to get floor(7/4)=1, got %d", tick.Volume)
		}
		total += tick.Volume
	}
	if total != 4 {
		t.Errorf("expected total 4 (remainder dropped), got %d", total)
	}
}

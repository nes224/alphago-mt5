package backtest_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
)

func TestParseBarLine_ValidLine(t *testing.T) {
	line := "2026.01.30 23:50;4865.02;4892.88;4862.7;4882.55;2598"

	bar, err := backtest.ParseBarLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantTime := time.Date(2026, 1, 30, 23, 50, 0, 0, time.UTC)
	if !bar.Time.Equal(wantTime) {
		t.Errorf("expected time %v, got %v", wantTime, bar.Time)
	}
	if bar.Open != 4865.02 || bar.High != 4892.88 || bar.Low != 4862.7 || bar.Close != 4882.55 {
		t.Errorf("expected OHLC 4865.02/4892.88/4862.7/4882.55, got %f/%f/%f/%f", bar.Open, bar.High, bar.Low, bar.Close)
	}
	if bar.Volume != 2598 {
		t.Errorf("expected volume 2598, got %d", bar.Volume)
	}
}

func TestParseBarLine_WrongFieldCount(t *testing.T) {
	_, err := backtest.ParseBarLine("2026.01.30 23:50;4865.02;4892.88;4862.7")
	if err == nil {
		t.Fatal("expected an error for a line with too few fields")
	}
}

func TestParseBarLine_BadTimestamp(t *testing.T) {
	_, err := backtest.ParseBarLine("not-a-date;4865.02;4892.88;4862.7;4882.55;2598")
	if err == nil {
		t.Fatal("expected an error for an unparseable timestamp")
	}
}

func TestParseBarLine_BadFloat(t *testing.T) {
	_, err := backtest.ParseBarLine("2026.01.30 23:50;not-a-number;4892.88;4862.7;4882.55;2598")
	if err == nil {
		t.Fatal("expected an error for an unparseable price field")
	}
}

func TestParseBarLine_BadVolume(t *testing.T) {
	_, err := backtest.ParseBarLine("2026.01.30 23:50;4865.02;4892.88;4862.7;4882.55;not-a-number")
	if err == nil {
		t.Fatal("expected an error for an unparseable volume field")
	}
}

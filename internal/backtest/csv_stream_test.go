package backtest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func writeFixtureCSV(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.csv")
	content := "Date;Open;High;Low;Close;Volume\n"
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write fixture CSV: %v", err)
	}
	return path
}

func TestStreamOHLCVCSV_FiltersByDateRangeAndSynthesizesTicks(t *testing.T) {
	path := writeFixtureCSV(t, []string{
		"2026.01.28 00:00;100;100;100;100;4", // before range, excluded
		"2026.01.29 00:00;100;105;95;102;4",  // in range
		"2026.01.30 00:00;102;108;101;106;8", // in range
		"2026.01.31 00:00;106;106;106;106;4", // after range (to is exclusive), excluded
	})

	from := time.Date(2026, 1, 29, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)

	var ticks []domain.Tick
	err := backtest.StreamOHLCVCSV(path, "XAUUSDm", from, to, 0.1, func(tick domain.Tick) {
		ticks = append(ticks, tick)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 in-range bars * 4 synthetic ticks each.
	if len(ticks) != 8 {
		t.Fatalf("expected 8 ticks (2 bars x 4), got %d", len(ticks))
	}
	for _, tick := range ticks {
		if tick.Symbol != "XAUUSDm" {
			t.Errorf("expected symbol XAUUSDm, got %s", tick.Symbol)
		}
		if tick.Timestamp.Before(from) || !tick.Timestamp.Before(to.Add(24*time.Hour)) {
			// generous upper bound since synthetic ticks land a few seconds
			// after the bar's own timestamp, which itself must be < to
			t.Errorf("tick timestamp %v outside expected range", tick.Timestamp)
		}
	}

	// Ticks must come out in chronological order (bar order preserved).
	for i := 1; i < len(ticks); i++ {
		if ticks[i].Timestamp.Before(ticks[i-1].Timestamp) {
			t.Fatalf("expected chronological order, tick %d (%v) before tick %d (%v)", i, ticks[i].Timestamp, i-1, ticks[i-1].Timestamp)
		}
	}
}

func TestStreamOHLCVCSV_WrapsParseErrorWithLineNumber(t *testing.T) {
	path := writeFixtureCSV(t, []string{
		"2026.01.29 00:00;100;105;95;102;4",
		"garbage-line-not-csv",
	})

	err := backtest.StreamOHLCVCSV(path, "XAUUSDm", time.Time{}, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), 0.1, func(domain.Tick) {})
	if err == nil {
		t.Fatal("expected an error for a malformed row")
	}
	if got := err.Error(); !strings.Contains(got, "line 3") {
		t.Errorf("expected error to mention the line number (3, accounting for the header), got: %s", got)
	}
}

func TestLastBarTime_ReturnsLastRowsTimestamp(t *testing.T) {
	path := writeFixtureCSV(t, []string{
		"2026.01.28 00:00;100;100;100;100;4",
		"2026.01.29 00:00;100;105;95;102;4",
		"2026.01.30 23:50;102;108;101;106;8",
	})

	got, err := backtest.LastBarTime(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 1, 30, 23, 50, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("expected last bar time %v, got %v", want, got)
	}
}

func TestLastBarTime_ErrorsWhenNoDataRows(t *testing.T) {
	path := writeFixtureCSV(t, nil) // header only, no data rows
	if _, err := backtest.LastBarTime(path); err == nil {
		t.Fatal("expected an error for a CSV with no data rows")
	}
}

func TestStreamOHLCVCSV_EmptyRangeProducesNoTicks(t *testing.T) {
	path := writeFixtureCSV(t, []string{
		"2026.01.29 00:00;100;105;95;102;4",
	})

	called := false
	err := backtest.StreamOHLCVCSV(path, "XAUUSDm", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC), 0.1, func(domain.Tick) {
		called = true
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Error("expected no ticks when no bars fall within the requested range")
	}
}

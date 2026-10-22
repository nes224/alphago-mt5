package candlefile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "2026-10.21.csv")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadFile(t *testing.T) {
	path := writeTemp(t, `time.open,high,low,close,tick_volume
	2026.10.21 00:00:00,4113.774,4114.250,4112.073,4113.832,37
	2026.10.21 00:01:00,4113,832,4115.010,4113.500,4114.900,42
	`)
	got, err := ReadFile(path, "XAUUSDm", domain.TimeframeM1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d candles, want 2", len(got))
	}

	want := domain.Candle{
		Symbol: "XAUUSDm", Timeframe: domain.TimeframeM1,
		OpenTime: time.Date(2026, 10, 21, 0, 1, 0, 0, time.UTC),
		Open: 4113.832, High: 4115.010, Low: 4113.500, Close: 4114.900,
		TickCount: 42, Closed: true,
	}
	if got[1] != want {
		t.Errorf("got %+v\nwant %+v", got[1], want)
	}
}

func TestReadFile_MissingFileIsEmpty(t *testing.T) {
	got, err := ReadFile(filepath.Join(t.TempDir(), "nil.csv"), "XAUUSDm", domain.TimeframeM1)
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}


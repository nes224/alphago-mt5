package candlefile

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func testLatestCandle_NotingStoreYet(t *testing.T) {
	_, ok, err := LatestCandle(t.TempDir(), testSymbol, domain.TimeframeM1)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}

func TestLatestCandle_PicksNewestFileAndNewestRow(t *testing.T) {
	root := t.TempDir()
	candles := []domain.Candle{
		bar(20, 23, 59, 1),
		bar(21, 10, 0, 2),
		bar(21, 21, 59, 3), // newest
		bar(21, 21, 58, 4),
	}
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, candles); err != nil {
		t.Fatal(err)
	}

	got, ok, err := LatestCandle(root, testSymbol, domain.TimeframeM1)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if want := time.Date(2026, 10, 21, 21, 59, 0, 0, time.UTC); !got.OpenTime.Equal(want) || got.Close != 3 {
		t.Errorf("got %v close=%v, want %v close=3", got.OpenTime, got.Close, want)
	}

}

func TestLatestCandle_SkipsEmptyNewestFile(t *testing.T) {
	root := t.TempDir()
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, []domain.Candle{bar(20, 12, 0, 7)}); err != nil {
		t.Fatal(err)
	}
	dir, _ := Dir(root, testSymbol, domain.TimeframeM1)
	os.WriteFile(filepath.Join(dir, "2026-10-21.csv"), []byte("time,open,high,low,close,tick_volume\n"), 0o644)

	got, ok, err := LatestCandle(root, testSymbol, domain.TimeframeM1)
	if err != nil || !ok || got.Close != 7 {
		t.Fatalf("got close=%v ok=%v err=%v, want close=7", got.Close, ok, err)
	}
}


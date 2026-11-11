package candlefile

import (
	"testing"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

func TestReadAll_AcrossFilesOldestFirst(t *testing.T) {
	root := t.TempDir()
	in := []domain.Candle{
		bar(22, 23, 59, 1),
		bar(21, 10, 0, 2),
		bar(23, 0, 0, 3),
	}
	if err := WriteCandles(root, testSymbol, domain.TimeframeM1, in); err != nil {
		t.Fatal(err)
	}

	got, err := ReadAll(root, testSymbol, domain.TimeframeM1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d candles, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i].OpenTime.After(got[i-1].OpenTime) {
			t.Fatalf("not sorted at %d: %v then %v", i, got[i-1].OpenTime, got[i].OpenTime)
		}
	}
}

func TestReadAll_NothingStored(t *testing.T) {
	got, err := ReadAll(t.TempDir(), testSymbol, domain.TimeframeH1)
	if err != nil || got != nil {
		t.Fatalf("got %v, %v; want nil, nil", got, err)
	}
}

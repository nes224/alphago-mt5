package backtest_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type fakeTickHistorySaver struct {
	batches [][]domain.Tick
	failOn  int // call number (1-indexed) to fail on; 0 = never fail
	calls   int
}

func (f *fakeTickHistorySaver) SaveTickHistoryBatch(ticks []domain.Tick) error {
	f.calls++
	if f.failOn != 0 && f.calls == f.failOn {
		return errors.New("simulated db error")
	}
	f.batches = append(f.batches, append([]domain.Tick(nil), ticks...))
	return nil
}

func TestImportTickHistoryFromCSV_BatchesInsertsAtBatchSize(t *testing.T) {
	// 3 bars * 4 synthetic ticks each = 12 ticks total.
	path := writeFixtureCSV(t, []string{
		"2026.01.29 00:00;100;105;95;102;4",
		"2026.01.29 00:01;102;108;101;106;8",
		"2026.01.29 00:02;106;110;104;108;4",
	})

	saver := &fakeTickHistorySaver{}
	total, err := backtest.ImportTickHistoryFromCSV(path, "XAUUSDm", time.Time{}, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), 0.1, 5, saver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 12 {
		t.Fatalf("expected 12 ticks imported, got %d", total)
	}

	// batchSize=5 over 12 ticks -> batches of 5, 5, 2 (remainder flush).
	if len(saver.batches) != 3 {
		t.Fatalf("expected 3 batch calls, got %d", len(saver.batches))
	}
	wantSizes := []int{5, 5, 2}
	for i, b := range saver.batches {
		if len(b) != wantSizes[i] {
			t.Errorf("batch %d: expected size %d, got %d", i, wantSizes[i], len(b))
		}
	}
}

func TestImportTickHistoryFromCSV_PropagatesSaveError(t *testing.T) {
	path := writeFixtureCSV(t, []string{
		"2026.01.29 00:00;100;105;95;102;4",
		"2026.01.29 00:01;102;108;101;106;8",
	})

	saver := &fakeTickHistorySaver{failOn: 1}
	_, err := backtest.ImportTickHistoryFromCSV(path, "XAUUSDm", time.Time{}, time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC), 0.1, 5, saver)
	if err == nil {
		t.Fatal("expected the save error to propagate")
	}
}

func TestImportTickHistoryFromCSV_NoTicksIsNotAnError(t *testing.T) {
	path := writeFixtureCSV(t, []string{
		"2026.01.29 00:00;100;105;95;102;4",
	})

	saver := &fakeTickHistorySaver{}
	// Range excludes the only bar.
	total, err := backtest.ImportTickHistoryFromCSV(path, "XAUUSDm", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC), 0.1, 5, saver)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 ticks imported, got %d", total)
	}
	if len(saver.batches) != 0 {
		t.Errorf("expected no batch calls for an empty range, got %d", len(saver.batches))
	}
}

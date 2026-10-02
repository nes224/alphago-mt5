package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// TickHistorySaver is the persistence port this importer needs -- satisfied
// by *database.Store in production (SaveTickHistoryBatch already exists and
// is used by the live tick recorder), faked in tests. Declared here rather
// than depended on directly so internal/backtest stays decoupled from the
// database adapter (Hexagonal Architecture).
type TickHistorySaver interface {
	SaveTickHistoryBatch(ticks []domain.Tick) error
}

// ImportTickHistoryFromCSV streams csvPath (see StreamOHLCVCSV) and flushes
// synthesized ticks to store.SaveTickHistoryBatch in chunks of at most
// batchSize, instead of one call per tick -- same rationale as the live
// tick_history recorder's own batching (inserting millions of rows one at a
// time is needlessly slow). Returns the total number of ticks successfully
// imported before any error.
//
// If a batch save fails, streaming is NOT aborted immediately (the
// StreamOHLCVCSV callback has no way to signal "stop early") -- the rest of
// the file is scanned and parsed but no further batches are saved, and the
// first save error is returned once streaming finishes. For a one-time CLI
// import tool this is an acceptable trade-off (a few extra seconds of
// parsing) rather than complicating the streaming API for an edge case.
func ImportTickHistoryFromCSV(csvPath, symbol string, from, to time.Time, spreadHalf float64, batchSize int, store TickHistorySaver) (int, error) {
	var batch []domain.Tick
	total := 0
	var saveErr error

	err := StreamOHLCVCSV(csvPath, symbol, from, to, spreadHalf, func(tick domain.Tick) {
		if saveErr != nil {
			return
		}
		batch = append(batch, tick)
		if len(batch) < batchSize {
			return
		}
		if err := store.SaveTickHistoryBatch(batch); err != nil {
			saveErr = err
			return
		}
		total += len(batch)
		batch = batch[:0]
	})
	if err != nil {
		return total, err
	}
	if saveErr != nil {
		return total, saveErr
	}

	if len(batch) > 0 {
		if err := store.SaveTickHistoryBatch(batch); err != nil {
			return total, err
		}
		total += len(batch)
	}
	return total, nil
}

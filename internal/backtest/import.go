package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type TickHistorySaver interface {
	SaveTickHistoryBatch(ticks []domain.Tick) error
}

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

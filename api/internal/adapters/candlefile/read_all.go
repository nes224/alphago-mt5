package candlefile

import (
	"path/filepath"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

// ReadAll loads every stored candle of one symbol and timeframe, oldest first.
// A symbol or timeframe with no files returns nil, nil.
func ReadAll(root, symbol string, tf domain.Timeframe) ([]domain.Candle, error) {
	dir, err := Dir(root, symbol, tf)
	if err != nil {
		return nil, err
	}
	names, err := periodFiles(dir, tf)
	if err != nil {
		return nil, err
	}

	var all []domain.Candle
	for _, name := range names {
		candles, err := ReadFile(filepath.Join(dir, name), symbol, tf)
		if err != nil {
			return nil, err
		}
		all = append(all, candles...)
	}
	return all, nil
}

package candlefile

import (
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type Store struct {
	Root string
}

var _ ports.CandleStore = Store{}

func (s Store) LatestCandle(symbol string, tf domain.Timeframe) (domain.Candle, bool, error) {
	return LatestCandle(s.Root, symbol, tf)
}

func (s Store) WriteCandles(symbol string, tf domain.Timeframe, candles []domain.Candle) error {
	return WriteCandles(s.Root, symbol, tf, candles)
}

package ports

import "github.com/nes224/alphago-mt5/internal/core/domain"

type CandleStore interface {
	LatestCandle(symbol string, tf domain.Timeframe) (c domain.Candle, ok bool, err error)
	WriteCandles(symbol string, tf domain.Timeframe, candles []domain.Candle) error
}

package ports

import "github.com/nes224/alphago-mt5/internal/core/domain"

type CandlePort interface {
	LatestCandle(symbol string, tf domain.Timeframe) (c domain.Candle, ok bool)
	WriteCandles(symbol string, tf domain.Timeframe, candles []domain.Candle) error
}
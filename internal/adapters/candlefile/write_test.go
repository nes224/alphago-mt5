package candlefile

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

const testSymbol = "XAUUSDm"

func bar(day, hh, mm int, price float64) domain.Candle {
	return domain.Candle{
		Symbol:    testSymbol,
		Timeframe: domain.TimeframeM1,
		OpenTime:  time.Date(2026, 10, day, hh, hh, 0, 0, time.UTC),
		Open:      price - 1, High: price + 1, Low: price - 2, Close: price,
		TickCount: 10,
		Closed:    true,
	}
}

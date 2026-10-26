package ports

import (
	"context"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// HistoryProvider ให้แท่งเทียนย้อนหลังจาก broker
// Rates คืนแท่งที่ open time อยู่ในช่วง [from, to] เรียงเก่า -> ใหม่
type HistoryProvider interface {
	// Rates returns candles whose open time is in [from, to], oldest first.
	// A candle that is still forming has Closed = false.
	Rates(ctx context.Context, symbol string, tf domain.Timeframe, from, to time.Time) ([]domain.Candle, error)

	// ServerTime returns the broker's current server time.

}

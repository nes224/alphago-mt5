package candlefile

import (
	"testing"
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

func readBack(t *testing.T, root string, day int) []domain.Candle {
	t.Helper()
	path, _ := FilePath(root, testSymbol, domain.TimeframeM1, time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC))
	got, err := ReadFile(path, testSymbol, domain.TimeframeM1)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func closes(cs []domain.Candle) []float64 {
	out := make([]float64, len(cs))
	for i, c := range cs {
		out[i] = c.Close
	}
	return out
}

func equalFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
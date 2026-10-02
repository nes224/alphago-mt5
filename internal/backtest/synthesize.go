package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

var synthTickOffsets = [4]time.Duration{0, 15 * time.Second, 30 * time.Second, 45 * time.Second}

func SynthesizeTicks(bar Bar, symbol string, spreadHalf float64) [4]domain.Tick {
	var prices [4]float64
	if bar.Close >= bar.Open {
		prices = [4]float64{bar.Open, bar.Low, bar.High, bar.Close}
	} else {
		prices = [4]float64{bar.Open, bar.High, bar.Low, bar.Close}
	}

	volumePerTick := bar.Volume / 4

	var ticks [4]domain.Tick
	for i, price := range prices {
		ticks[i] = domain.Tick{
			Symbol:    symbol,
			Bid:       price - spreadHalf,
			Ask:       price + spreadHalf,
			Volume:    volumePerTick,
			Timestamp: bar.Time.Add(synthTickOffsets[i]),
		}
	}
	return ticks
}

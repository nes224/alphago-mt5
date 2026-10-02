package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// synthTickOffsets spaces the 4 synthetic ticks evenly within a 1-minute bar.
var synthTickOffsets = [4]time.Duration{0, 15 * time.Second, 30 * time.Second, 45 * time.Second}

// SynthesizeTicks turns one OHLCV bar into 4 synthetic domain.Tick values, in
// chronological order, approximating an intrabar price path. This is a
// heuristic, not real intrabar data -- no OHLC bar can tell you the true
// order prices were visited in, and that order matters a lot for this
// backtest: if both a resulting order's SL and TP fall within the bar's
// [Low, High] range, which one gets "hit first" depends entirely on this
// guessed path. This is the single biggest source of inaccuracy in this
// backtest and cannot be fully corrected without real tick data.
//
// The path used: bullish bar (Close >= Open) -> Open, Low, High, Close
// (dip then rally before settling); bearish bar -> Open, High, Low, Close
// (rally then drop before settling). This is a common, defensible choice
// for OHLC-to-tick backtesting, not a proven fact about this specific data.
//
// The bar's single price column is treated as the mid price (an assumption
// -- the CSV source's actual convention, e.g. bid vs mid vs last, is
// unknown): Bid = price - spreadHalf, Ask = price + spreadHalf.
//
// Volume is split evenly across the 4 ticks via integer division; any
// remainder is dropped (e.g. Volume=7 -> 1,1,1,1, not 1,1,1,4) -- documented
// here rather than silently inflating or skewing one synthetic tick.
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

// Package backtest replays the SuperTrend + SMA strategy over stored candles
// and measures every trade in R (profit or loss as a multiple of the stop distance).
//
// Rules (see the project manual, "Trading theory"):
//   - signals are checked only when a candle closes
//   - buy: SuperTrend turns up and the close is above the SMA; sell is the mirror
//   - optional filter: only trade in the direction of SuperTrend on a higher timeframe
//   - enter at the next candle's open; stop = SLATR x ATR; target = TPR x stop distance
//   - one position at a time
//
// Candles are bid prices (as MT5 stores them). A buy is filled at ask (bid + Spread)
// and exits at bid; a sell is filled at bid and exits at ask. When the stop and the
// target are both inside one candle, the stop is assumed to come first.
package backtest

import (
	"math"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
	"github.com/nes224/alphago-mt5/api/internal/core/indicator"
)

type Params struct {
	ATRPeriod int     // ATR period used by SuperTrend and the stop
	Mult      float64 // SuperTrend band multiplier
	SMAPeriod int     // trend filter on the trading timeframe
	SLATR     float64 // stop distance in ATRs
	TPR       float64 // target distance in multiples of the stop distance
	Spread    float64 // ask - bid, in price units
}

func DefaultParams() Params {
	return Params{ATRPeriod: 10, Mult: 3, SMAPeriod: 200, SLATR: 1.5, TPR: 2, Spread: 0.25}
}

type Trade struct {
	Side       int // +1 buy, -1 sell
	SignalTime time.Time
	EntryTime  time.Time
	ExitTime   time.Time
	Entry      float64
	SL         float64
	TP         float64
	Exit       float64
	Reason     string // "tp", "sl" or "end"
	R          float64
}

type Stats struct {
	Trades       int
	Wins         int
	WinRate      float64 // 0..1
	TotalR       float64
	AvgR         float64
	ProfitFactor float64 // gross win / gross loss; +Inf when there is no loss
	MaxDrawdownR float64 // largest drop of the cumulative R curve from its peak
}

type Result struct {
	Trades []Trade
	Stats  Stats
}

// Run backtests candles (one symbol, one timeframe, oldest first). filter is an
// optional higher-timeframe series; pass nil to trade without it.
func Run(candles, filter []domain.Candle, p Params) Result {
	n := len(candles)
	if n < 2 {
		return Result{}
	}
	atr := indicator.ATR(candles, p.ATRPeriod)
	_, dir := indicator.SuperTrend(candles, p.ATRPeriod, p.Mult)
	sma := indicator.SMA(indicator.Closes(candles), p.SMAPeriod)

	var fdir []int
	if filter != nil {
		_, fdir = indicator.SuperTrend(filter, p.ATRPeriod, p.Mult)
	}
	tfDur := candles[0].Timeframe.Duration()
	var fDur time.Duration
	if len(filter) > 0 {
		fDur = filter[0].Timeframe.Duration()
	}

	var trades []Trade
	var open *Trade
	fj := -1 // index of the newest filter candle closed by the current time

	for i := 1; i < n; i++ {
		c := candles[i]

		// 1. manage the open position on this candle
		if open != nil {
			if exit, reason, done := checkExit(open, c, p.Spread); done {
				closeTrade(open, c.OpenTime, exit, reason)
				trades = append(trades, *open)
				open = nil
			}
		}

		// 2. look for a signal at the close of this candle; enter on the next one
		if open != nil || i+1 >= n || dir[i] == 0 || dir[i-1] == 0 || dir[i] == dir[i-1] {
			continue
		}
		if math.IsNaN(sma[i]) || math.IsNaN(atr[i]) {
			continue
		}
		side := dir[i]
		if side == 1 && c.Close <= sma[i] || side == -1 && c.Close >= sma[i] {
			continue
		}
		if filter != nil {
			closeAt := c.OpenTime.Add(tfDur)
			for fj+1 < len(filter) && !filter[fj+1].OpenTime.Add(fDur).After(closeAt) {
				fj++
			}
			if fj < 0 || fdir[fj] != side {
				continue
			}
		}

		next := candles[i+1]
		entry := next.Open
		if side == 1 {
			entry += p.Spread
		}
		dist := p.SLATR * atr[i]
		open = &Trade{
			Side:       side,
			SignalTime: c.OpenTime,
			EntryTime:  next.OpenTime,
			Entry:      entry,
			SL:         entry - float64(side)*dist,
			TP:         entry + float64(side)*p.TPR*dist,
		}
	}

	if open != nil { // still open at the end of the data: close at the last price
		last := candles[n-1]
		exit := last.Close
		if open.Side == -1 {
			exit += p.Spread
		}
		closeTrade(open, last.OpenTime, exit, "end")
		trades = append(trades, *open)
	}
	return Result{Trades: trades, Stats: Summarize(trades)}
}

// checkExit tests one candle against the stop and the target. The entry candle
// itself is checked too, because the position opens at its open price.
func checkExit(t *Trade, c domain.Candle, spread float64) (exit float64, reason string, done bool) {
	if c.OpenTime.Before(t.EntryTime) {
		return 0, "", false
	}
	if t.Side == 1 { // long: exits at bid
		switch {
		case c.Open <= t.SL:
			return c.Open, "sl", true // gapped through the stop
		case c.Low <= t.SL:
			return t.SL, "sl", true
		case c.Open >= t.TP:
			return c.Open, "tp", true
		case c.High >= t.TP:
			return t.TP, "tp", true
		}
		return 0, "", false
	}
	// short: exits at ask = bid + spread
	switch {
	case c.Open+spread >= t.SL:
		return c.Open + spread, "sl", true
	case c.High+spread >= t.SL:
		return t.SL, "sl", true
	case c.Open+spread <= t.TP:
		return c.Open + spread, "tp", true
	case c.Low+spread <= t.TP:
		return t.TP, "tp", true
	}
	return 0, "", false
}

func closeTrade(t *Trade, at time.Time, exit float64, reason string) {
	t.ExitTime = at
	t.Exit = exit
	t.Reason = reason
	t.R = float64(t.Side) * (exit - t.Entry) / math.Abs(t.Entry-t.SL)
}

// Summarize computes the statistics of a list of trades in time order.
func Summarize(trades []Trade) Stats {
	s := Stats{Trades: len(trades)}
	var win, loss, cum, peak float64
	for _, t := range trades {
		if t.R > 0 {
			s.Wins++
			win += t.R
		} else {
			loss -= t.R
		}
		cum += t.R
		peak = math.Max(peak, cum)
		s.MaxDrawdownR = math.Max(s.MaxDrawdownR, peak-cum)
	}
	s.TotalR = cum
	if s.Trades > 0 {
		s.WinRate = float64(s.Wins) / float64(s.Trades)
		s.AvgR = cum / float64(s.Trades)
	}
	switch {
	case loss > 0:
		s.ProfitFactor = win / loss
	case win > 0:
		s.ProfitFactor = math.Inf(1)
	}
	return s
}

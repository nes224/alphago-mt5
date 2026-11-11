package backtest

import (
	"math"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

var t0 = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

// series builds H1 candles from closes; open = previous close, range +/-0.5 around the body.
func series(tf domain.Timeframe, closes ...float64) []domain.Candle {
	out := make([]domain.Candle, len(closes))
	prev := closes[0]
	for i, c := range closes {
		out[i] = domain.Candle{
			Timeframe: tf,
			OpenTime:  t0.Add(time.Duration(i) * tf.Duration()),
			Open:      prev,
			High:      math.Max(prev, c) + 0.5,
			Low:       math.Min(prev, c) - 0.5,
			Close:     c,
		}
		prev = c
	}
	return out
}

func ramp(from, step float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = from + step*float64(i)
	}
	return out
}

func join(parts ...[]float64) []float64 {
	var out []float64
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// firstSide returns the first trade on the given side.
func firstSide(t *testing.T, trades []Trade, side int) Trade {
	t.Helper()
	for _, tr := range trades {
		if tr.Side == side {
			return tr
		}
	}
	t.Fatalf("no trade on side %d in %+v", side, trades)
	return Trade{}
}

var small = Params{ATRPeriod: 3, Mult: 1, SMAPeriod: 3, SLATR: 1, TPR: 1, Spread: 0}

// a fall then a long rise: SuperTrend turns up with the close above the SMA -> a buy that hits its target
func TestRun_BuyAfterTrendTurnsUpHitsTarget(t *testing.T) {
	c := series(domain.TimeframeH1, join(ramp(100, -1, 12), ramp(90, 2, 20))...)
	tr := firstSide(t, Run(c, nil, small).Trades, 1)
	if tr.Reason != "tp" {
		t.Fatalf("buy = %+v, want it closed at tp", tr)
	}
	if math.Abs(tr.R-1) > 1e-9 {
		t.Errorf("R = %v, want 1 (target = 1 x stop distance, no spread)", tr.R)
	}
	if !tr.EntryTime.After(tr.SignalTime) {
		t.Errorf("entry %v should come after the signal candle %v", tr.EntryTime, tr.SignalTime)
	}
}

// a rise then a crash: SuperTrend turns down -> sell
func TestRun_SellAfterTrendTurnsDown(t *testing.T) {
	c := series(domain.TimeframeH1, join(ramp(100, 1, 12), ramp(110, -2, 20))...)
	tr := firstSide(t, Run(c, nil, small).Trades, -1)
	if tr.SL <= tr.Entry || tr.TP >= tr.Entry {
		t.Errorf("sell levels wrong: %+v", tr)
	}
}

// the same buy setup is skipped when the higher timeframe trend is down
func TestRun_FilterBlocksTradesAgainstHigherTimeframe(t *testing.T) {
	c := series(domain.TimeframeH1, join(ramp(100, -1, 12), ramp(90, 2, 20))...)
	down := series(domain.TimeframeH4, ramp(200, -10, 12)...) // H4 falling the whole time
	down[0].OpenTime = t0.Add(-40 * time.Hour)
	for i := range down {
		down[i].OpenTime = down[0].OpenTime.Add(time.Duration(i) * 4 * time.Hour)
	}
	res := Run(c, down, small)
	for _, tr := range res.Trades {
		if tr.Side == 1 {
			t.Fatalf("buy %+v taken against a falling H4 trend", tr)
		}
	}
}

// a wide spread makes a long cost more than a short on the same move
func TestRun_SpreadIsPaidOnEntry(t *testing.T) {
	c := series(domain.TimeframeH1, join(ramp(100, -1, 12), ramp(90, 2, 20))...)
	p := small
	p.Spread = 0.4
	tr := firstSide(t, Run(c, nil, p).Trades, 1)
	open := c[0].Open
	for _, x := range c {
		if x.OpenTime.Equal(tr.EntryTime) {
			open = x.Open
		}
	}
	if math.Abs(tr.Entry-(open+0.4)) > 1e-9 {
		t.Errorf("entry = %v, want next open %v + spread 0.4", tr.Entry, open)
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize([]Trade{{R: 2}, {R: -1}, {R: -1}, {R: 2}, {R: -1}})
	if s.Trades != 5 || s.Wins != 2 || math.Abs(s.TotalR-1) > 1e-9 {
		t.Fatalf("stats = %+v", s)
	}
	if math.Abs(s.WinRate-0.4) > 1e-9 || math.Abs(s.ProfitFactor-4.0/3) > 1e-9 {
		t.Errorf("win rate %v, profit factor %v", s.WinRate, s.ProfitFactor)
	}
	if math.Abs(s.MaxDrawdownR-2) > 1e-9 { // peak 2 -> low 0
		t.Errorf("max drawdown = %v, want 2", s.MaxDrawdownR)
	}
}

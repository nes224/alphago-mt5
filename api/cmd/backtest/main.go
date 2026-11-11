// backtest replays the SuperTrend + SMA strategy over the candle CSV files
// written by the gateway and prints the results in R.
//
//	go run ./cmd/backtest -symbol XAUUSDm -tf H1 -filter Daily
//	go run ./cmd/backtest -symbol XAUUSDm -tf H1 -filter Daily -grid -split 0.7
//
// -split 0.7 uses the first 70% of the period to choose parameters and reports
// the last 30% separately (out-of-sample), so a result that only fits the past shows up.
package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/nes224/alphago-mt5/api/internal/adapters/candlefile"
	"github.com/nes224/alphago-mt5/api/internal/core/backtest"
	"github.com/nes224/alphago-mt5/api/internal/core/domain"
)

func main() {
	d := backtest.DefaultParams()
	symbol := flag.String("symbol", "XAUUSDm", "broker symbol")
	tf := flag.String("tf", "H1", "timeframe to trade: M1 M5 M15 M30 H1 H4 Daily")
	filterTF := flag.String("filter", "", "higher timeframe trend filter (empty = none)")
	dataDir := flag.String("data", "data/candle", "candle data directory")
	atr := flag.Int("atr", d.ATRPeriod, "ATR period")
	mult := flag.Float64("mult", d.Mult, "SuperTrend multiplier")
	sma := flag.Int("sma", d.SMAPeriod, "SMA period")
	sl := flag.Float64("sl", d.SLATR, "stop loss in ATRs")
	tp := flag.Float64("tp", d.TPR, "take profit in multiples of the stop")
	spread := flag.Float64("spread", d.Spread, "spread in price units")
	split := flag.Float64("split", 0, "fraction of the period used as in-sample (0 = no split)")
	grid := flag.Bool("grid", false, "try every parameter combination (brute force)")
	minTrades := flag.Int("min-trades", 20, "grid: ignore combinations with fewer in-sample trades")
	flag.Parse()

	if err := run(*symbol, domain.Timeframe(*tf), domain.Timeframe(*filterTF), *dataDir,
		backtest.Params{ATRPeriod: *atr, Mult: *mult, SMAPeriod: *sma, SLATR: *sl, TPR: *tp, Spread: *spread},
		*split, *grid, *minTrades); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func load(dir, symbol string, tf domain.Timeframe) ([]domain.Candle, error) {
	if tf.Minutes() == 0 {
		return nil, fmt.Errorf("unknown timeframe %q", tf)
	}
	c, err := candlefile.ReadAll(dir, symbol, tf)
	if err != nil {
		return nil, err
	}
	if len(c) == 0 {
		return nil, fmt.Errorf("no candles for %s %s in %s", symbol, tf, dir)
	}
	return c, nil
}

func run(symbol string, tf, filterTF domain.Timeframe, dir string, p backtest.Params, split float64, grid bool, minTrades int) error {
	candles, err := load(dir, symbol, tf)
	if err != nil {
		return err
	}
	var filter []domain.Candle
	if filterTF != "" {
		if filter, err = load(dir, symbol, filterTF); err != nil {
			return err
		}
	}

	first, last := candles[0].OpenTime, candles[len(candles)-1].OpenTime
	fmt.Printf("%s %s, filter %s: %d candles, %s to %s\n", symbol, tf, orNone(filterTF), len(candles), day(first), day(last))
	if filter != nil && filter[0].OpenTime.After(first) {
		fmt.Printf("note: filter data starts %s, so no trades before it\n", day(filter[0].OpenTime))
	}

	cut := last.Add(time.Second)
	if split > 0 && split < 1 {
		cut = first.Add(time.Duration(float64(last.Sub(first)) * split))
		fmt.Printf("in-sample: %s to %s, out-of-sample: %s to %s\n", day(first), day(cut), day(cut), day(last))
	}
	fmt.Println()

	if grid {
		return runGrid(candles, filter, p.Spread, cut, minTrades)
	}

	res := backtest.Run(candles, filter, p)
	fmt.Printf("params: %s\n\n", paramString(p))
	in, out := partition(res.Trades, cut)
	if cut.After(last) {
		printStats("all", backtest.Summarize(in))
	} else {
		printStats("in-sample", backtest.Summarize(in))
		printStats("out-of-sample", backtest.Summarize(out))
	}
	fmt.Println()
	printYears(res.Trades)
	return nil
}

func runGrid(candles, filter []domain.Candle, spread float64, cut time.Time, minTrades int) error {
	type row struct {
		p       backtest.Params
		in, out backtest.Stats
	}
	var rows []row
	for _, atr := range []int{7, 10, 14, 21} {
		for _, mult := range []float64{2, 2.5, 3, 3.5, 4} {
			for _, sma := range []int{50, 100, 200} {
				for _, sl := range []float64{1, 1.5, 2, 3} {
					for _, tp := range []float64{1, 1.5, 2, 3} {
						p := backtest.Params{ATRPeriod: atr, Mult: mult, SMAPeriod: sma, SLATR: sl, TPR: tp, Spread: spread}
						in, out := partition(backtest.Run(candles, filter, p).Trades, cut)
						rows = append(rows, row{p, backtest.Summarize(in), backtest.Summarize(out)})
					}
				}
			}
		}
	}

	positive, eligible := 0, 0
	for _, r := range rows {
		if r.in.Trades >= minTrades {
			eligible++
			if r.in.TotalR > 0 {
				positive++
			}
		}
	}
	fmt.Printf("%d combinations, %d with >= %d in-sample trades, %d of those positive in-sample\n\n",
		len(rows), eligible, minTrades, positive)

	sort.Slice(rows, func(i, j int) bool { return rows[i].in.TotalR > rows[j].in.TotalR })
	fmt.Println("top 10 by in-sample total R, with their out-of-sample result:")
	fmt.Printf("%-34s | %-30s | %s\n", "params", "in-sample", "out-of-sample")
	shown := 0
	for _, r := range rows {
		if r.in.Trades < minTrades {
			continue
		}
		fmt.Printf("%-34s | %-30s | %s\n", paramString(r.p), short(r.in), short(r.out))
		if shown++; shown == 10 {
			break
		}
	}
	return nil
}

func partition(trades []backtest.Trade, cut time.Time) (in, out []backtest.Trade) {
	for _, t := range trades {
		if t.SignalTime.Before(cut) {
			in = append(in, t)
		} else {
			out = append(out, t)
		}
	}
	return in, out
}

func printStats(label string, s backtest.Stats) {
	fmt.Printf("%-14s trades %4d  win %5.1f%%  total %+7.1fR  avg %+.2fR  PF %5.2f  maxDD %5.1fR\n",
		label, s.Trades, 100*s.WinRate, s.TotalR, s.AvgR, s.ProfitFactor, s.MaxDrawdownR)
}

func short(s backtest.Stats) string {
	return fmt.Sprintf("%3d tr %4.0f%% %+6.1fR PF %4.2f", s.Trades, 100*s.WinRate, s.TotalR, s.ProfitFactor)
}

func printYears(trades []backtest.Trade) {
	byYear := map[int][]backtest.Trade{}
	var years []int
	for _, t := range trades {
		y := t.SignalTime.Year()
		if _, ok := byYear[y]; !ok {
			years = append(years, y)
		}
		byYear[y] = append(byYear[y], t)
	}
	sort.Ints(years)
	for _, y := range years {
		printStats(fmt.Sprint(y), backtest.Summarize(byYear[y]))
	}
}

func paramString(p backtest.Params) string {
	return fmt.Sprintf("atr %d x%.1f sma %d sl %.1f tp %.1f", p.ATRPeriod, p.Mult, p.SMAPeriod, p.SLATR, p.TPR)
}

func day(t time.Time) string { return t.Format("2006-01-02") }

func orNone(tf domain.Timeframe) string {
	if tf == "" {
		return "none"
	}
	return string(tf)
}

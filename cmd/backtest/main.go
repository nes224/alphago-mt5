package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

const dateLayout = "2006-01-02"

func main() {
	opts := parseFlags()
	printLimitationsBanner()

	cfg := buildConfig(opts)

	runner := backtest.NewRunner(cfg)
	ticks := 0
	err := backtest.StreamOHLCVCSV(opts.csvPath, opts.symbol, opts.from, opts.to, opts.spread/2, func(tick domain.Tick) {
		runner.OnTick(tick)
		ticks++
	})
	if err != nil {
		log.Fatalf("backtest failed: %v", err)
	}

	report := runner.Report()
	printReport(opts, report, ticks)

	if opts.tradesCSVPath != "" {
		if err := writeTradesCSV(opts.tradesCSVPath, report); err != nil {
			log.Fatalf("failed to write trades CSV: %v", err)
		}
		fmt.Printf("\nPer-trade detail written to %s\n", opts.tradesCSVPath)
	}
}

type options struct {
	csvPath       string
	symbol        string
	from, to      time.Time
	balance       float64
	riskPerTrade  float64
	volMultiplier float64
	useATR        bool
	atrMultiplier float64
	spread        float64
	tradesCSVPath string
}

func parseFlags() options {
	csvPath := flag.String("csv", "", "path to a semicolon-delimited OHLCV CSV (Date;Open;High;Low;Close;Volume) -- required")
	symbol := flag.String("symbol", "XAUUSDm", "symbol to tag synthesized ticks with")
	fromStr := flag.String("from", "", "start date, YYYY-MM-DD (default: -years before -to)")
	toStr := flag.String("to", "", "end date, YYYY-MM-DD, exclusive (default: the CSV's last bar date)")
	years := flag.Int("years", 3, "when -from is omitted, how many years before -to to start from")
	balance := flag.Float64("balance", 10000, "initial simulated account balance")
	riskPerTrade := flag.Float64("risk-per-trade", 0.01, "risk per trade as a fraction of balance (0.01 = 1%)")
	volMultiplier := flag.Float64("volatility-multiplier", 2.0, "SL distance = volatility-multiplier * SizingVolatility (StdDev), when not using ATR")
	useATR := flag.Bool("use-atr", false, "drive SL distance from ATR (M5x14) instead of SizingVolatility, once ATR is warmed up")
	atrMultiplier := flag.Float64("atr-multiplier", 1.75, "SL distance = atr-multiplier * ATR, when -use-atr is set")
	spread := flag.Float64("spread", 0.30, "constant spread assumed for every synthetic tick -- the source data only has a single price, see limitations banner")
	tradesCSV := flag.String("trades-csv", "", "optional path to write a per-trade CSV dump")
	flag.Parse()

	if *csvPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -csv is required")
		flag.Usage()
		os.Exit(2)
	}

	to := time.Time{}
	if *toStr != "" {
		t, err := time.Parse(dateLayout, *toStr)
		if err != nil {
			log.Fatalf("invalid -to date %q: %v", *toStr, err)
		}
		to = t
	} else {
		t, err := backtest.LastBarTime(*csvPath)
		if err != nil {
			log.Fatalf("failed to determine default -to from CSV: %v", err)
		}
		to = t.Add(time.Minute) // make the CSV's own last bar inclusive (StreamOHLCVCSV's [from,to) is to-exclusive)
	}

	from := to.AddDate(-*years, 0, 0)
	if *fromStr != "" {
		t, err := time.Parse(dateLayout, *fromStr)
		if err != nil {
			log.Fatalf("invalid -from date %q: %v", *fromStr, err)
		}
		from = t
	}

	return options{
		csvPath:       *csvPath,
		symbol:        *symbol,
		from:          from,
		to:            to,
		balance:       *balance,
		riskPerTrade:  *riskPerTrade,
		volMultiplier: *volMultiplier,
		useATR:        *useATR,
		atrMultiplier: *atrMultiplier,
		spread:        *spread,
		tradesCSVPath: *tradesCSV,
	}
}

func buildConfig(opts options) backtest.Config {
	cfg := backtest.DefaultConfig(opts.symbol, opts.balance)
	cfg.RiskPerTradePercent = opts.riskPerTrade
	cfg.VolatilityMultiplier = opts.volMultiplier
	cfg.UseATRForSizing = opts.useATR
	cfg.ATRMultiplier = opts.atrMultiplier
	return cfg
}

// printLimitationsBanner surfaces the known accuracy limitations of this
// backtest up front, every run -- not just in docs -- so the numbers below
// are never mistaken for a profit guarantee. See the plan doc for the full
// reasoning behind each one.
func printLimitationsBanner() {
	fmt.Println("=============================================================")
	fmt.Println("BACKTEST -- KNOWN LIMITATIONS (read before trusting the numbers)")
	fmt.Println("=============================================================")
	fmt.Println("- Bar->tick synthesis is a heuristic (Open->Low->High->Close or")
	fmt.Println("  Open->High->Low->Close depending on bar direction), not real")
	fmt.Println("  intrabar data. When both a trade's SL and TP fall inside one")
	fmt.Println("  bar's range, which one 'wins' is a guess.")
	fmt.Println("- Spread is a constant assumption (-spread flag), not real")
	fmt.Println("  historical spread -- MaxSpreadPips rejection will almost")
	fmt.Println("  never trigger here.")
	fmt.Println("- Session stats assume the CSV timestamps are UTC (unverified).")
	fmt.Println("- VolumeExpansionStrategy/LiquiditySweepStrategy/CVD/Volume")
	fmt.Println("  Profile are tick-level detectors tested here on SYNTHETIC")
	fmt.Println("  ticks, not real ones -- treat results as directional")
	fmt.Println("  guidance for tuning ATR/Multi-TF/sizing constants, not as a")
	fmt.Println("  profit guarantee.")
	fmt.Println("=============================================================")
	fmt.Println()
}

func printReport(opts options, report backtest.Report, ticks int) {
	fmt.Printf("CSV:        %s\n", opts.csvPath)
	fmt.Printf("Symbol:     %s\n", opts.symbol)
	fmt.Printf("Range:      %s -> %s\n", opts.from.Format(dateLayout), opts.to.Format(dateLayout))
	fmt.Printf("Ticks:      %d (synthetic, 4 per bar)\n", ticks)
	fmt.Println()

	fmt.Printf("Total trades:     %d\n", len(report.Trades))
	fmt.Printf("Win rate:         %.1f%%\n", report.WinRate()*100)
	fmt.Printf("Total PnL:        %.2f\n", report.TotalPnL())
	fmt.Printf("Initial balance:  %.2f\n", report.InitialBalance)
	fmt.Printf("Final balance:    %.2f\n", report.FinalBalance)
	fmt.Printf("Max drawdown:     %.2f\n", report.MaxDrawdown())
	fmt.Printf("Still open:       %d\n", report.StillOpenAtEnd)
	fmt.Println()

	fmt.Println("By strategy:")
	printWinLossTable(report.WinRateByStrategy())
	fmt.Println()

	fmt.Println("By session:")
	printWinLossTable(report.WinRateBySession())
}

func printWinLossTable(m map[string]backtest.WinLoss) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		wl := m[k]
		fmt.Printf("  %-30s trades=%-5d win_rate=%5.1f%%  pnl=%.2f\n", k, wl.TotalTrades, wl.WinRate*100, wl.TotalProfit)
	}
}

func writeTradesCSV(path string, report backtest.Report) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	header := []string{"symbol", "strategy_tag", "reason", "session", "action", "entry_time", "exit_time", "entry_price", "exit_price", "lot_size", "pnl", "is_win"}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, t := range report.Trades {
		row := []string{
			t.Symbol,
			t.StrategyTag,
			t.Reason,
			t.Session,
			string(t.Action),
			t.EntryTime.Format(time.RFC3339),
			t.ExitTime.Format(time.RFC3339),
			fmt.Sprintf("%.5f", t.EntryPrice),
			fmt.Sprintf("%.5f", t.ExitPrice),
			fmt.Sprintf("%.2f", t.LotSize),
			fmt.Sprintf("%.2f", t.PnL),
			fmt.Sprintf("%t", t.IsWin),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return w.Error()
}

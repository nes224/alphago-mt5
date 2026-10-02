package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/nes224/alphago-mt5/internal/adapters/config"
	"github.com/nes224/alphago-mt5/internal/adapters/database"
	"github.com/nes224/alphago-mt5/internal/backtest"
)

const dateLayout = "2006-01-02"

func main() {
	csvPath := flag.String("csv", "", "path to a semicolon-delimited OHLCV CSV (Date;Open;High;Low;Close;Volume) -- required")
	symbol := flag.String("symbol", "XAUUSDm", "symbol to tag imported ticks with")
	fromStr := flag.String("from", "", "start date, YYYY-MM-DD (default: 2 days before -to)")
	toStr := flag.String("to", "", "end date, YYYY-MM-DD, exclusive (default: the CSV's last bar date)")
	spread := flag.Float64("spread", 0.30, "constant spread assumed for every synthesized tick (source CSV has a single price column)")
	batchSize := flag.Int("batch-size", 500, "ticks per SaveTickHistoryBatch call (matches the live recorder's own batch size)")
	flag.Parse()

	if *csvPath == "" {
		log.Fatal("Error: -csv is required")
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
		to = t.Add(time.Minute) // make the CSV's own last bar inclusive
	}

	from := to.AddDate(0, 0, -2)
	if *fromStr != "" {
		t, err := time.Parse(dateLayout, *fromStr)
		if err != nil {
			log.Fatalf("invalid -from date %q: %v", *fromStr, err)
		}
		from = t
	}

	fmt.Printf("Importing %s into tick_history for %s, range %s -> %s ...\n", *csvPath, *symbol, from.Format(dateLayout), to.Format(dateLayout))
	if to.Sub(from) > 48*time.Hour {
		fmt.Println("Note: the live backfill only reads the last 25h of tick_history -- anything older than that only matters if you want a longer archive for other purposes.")
	}

	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database connection failed: %v", err)
	}
	store := database.NewStore(db)

	total, err := backtest.ImportTickHistoryFromCSV(*csvPath, *symbol, from, to, *spread/2, *batchSize, store)
	if err != nil {
		log.Fatalf("import failed after %d ticks: %v", total, err)
	}

	fmt.Printf("Done -- imported %d synthesized ticks into tick_history.\n", total)
}

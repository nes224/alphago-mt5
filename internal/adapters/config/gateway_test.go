package config

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func TestLoadGateway_Defaults(t *testing.T) {
	for _, k := range []string{"SYMBOLS", "CANDLE_DATA_DIR", "BACKFILL_LOOKBACK"} {
		t.Setenv(k, "")
	}
	g, err := LoadGateway()
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Symbols) != 0 || g.CandleDataDir != "data/candle" || len(g.BackfillLookback) != 0 {
		t.Errorf("got symbols=%v dir=%q lookback=%v", g.Symbols, g.CandleDataDir, g.BackfillLookback)
	}
}

func TestLoadGateway_ReadsNewSettings(t *testing.T) {
	t.Setenv("SYMBOLS", "XAUUSDm, BTCUSDm")
	t.Setenv("CANDLE_DATA_DIR", "/tmp/candles")
	t.Setenv("BACKFILL_LOOKBACK", "M1=7d, H1=36h")

	g, err := LoadGateway()
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Symbols) != 2 || g.Symbols[0] != "XAUUSDm" || g.Symbols[1] != "BTCUSDm" {
		t.Errorf("symbols = %q", g.Symbols)
	}
	if g.CandleDataDir != "/tmp/candles" {
		t.Errorf("dir = %q", g.CandleDataDir)
	}
	want := map[domain.Timeframe]time.Duration{domain.TimeframeM1: 7 * 24 * time.Hour, domain.TimeframeH1: 36 * time.Hour}
	if len(g.BackfillLookback) != len(want) {
		t.Fatalf("lookback = %v, want %v", g.BackfillLookback, want)
	}
	for tf, d := range want {
		if g.BackfillLookback[tf] != d {
			t.Errorf("lookback[%s] = %v, want %v", tf, g.BackfillLookback, d)
		}
	}
}

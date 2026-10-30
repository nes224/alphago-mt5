package config

import "testing"

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
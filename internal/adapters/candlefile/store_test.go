package candlefile

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func TestStore_WriteThenLatest(t *testing.T) {
	s := Store{Root: t.TempDir()}

	if _, ok, err := s.LatestCandle(testSymbol, domain.TimeframeM1); err != nil || ok {
		t.Fatalf("empty store: ok=%v err=%v", ok, err)
	}

	if err := s.WriteCandles(testSymbol, domain.TimeframeM1, []domain.Candle{bar(21, 0, 0, 1), bar(21, 0, 1, 2)}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.LatestCandle(testSymbol, domain.TimeframeM1)
	if err != nil || !ok || got.Close != 2 {
		t.Fatalf("got close=%v ok=%v err=%v, want close=2", got.Close, ok, err)
	}
}

package candlefile

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func testLatestCandle_NotingStoreYet(t *testing.T) {
	_, ok, err := LatestCandle(t.TempDir(), testSymbol, domain.TimeframeM1)
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v, want ok=false err=nil", ok, err)
	}
}
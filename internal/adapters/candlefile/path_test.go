package candlefile

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func TestFilePath(t *testing.T) {
	at := time.Date(2026, 10, 21, 3, 6, 0, 0, time.UTC)

	tests := []struct {
		tf   domain.Timeframe
		want string
	}{
		{domain.TimeframeM1, "data/candle/xauusdm/tf_m_1/2026-10-21.csv"},
		{domain.TimeframeM5, "data/candle/xauusdm/tf_m_5/2026-10-21.csv"},
		{domain.TimeframeM15, "data/candle/xauusdm/tf_m_15/2026-10-21.csv"},
		{domain.TimeframeM30, "data/candle/xauusdm/tf_m_30/2026-10-21.csv"},
		{domain.TimeframeH1, "data/candle/xauusdm/tf_m_60/2026-10.csv"},
		{domain.TimeframeH4, "data/candle/xauusdm/tf_m_240/2026-10.csv"},
		{domain.TimeframeDaily, "data/candle/xauusdm/tf_m_1440/2026.csv"},
	}
	for _, tt := range tests {
		got, err := FilePath("data/candle", "XAUUSDm", tt.tf, at)
		if err != nil {
			t.Fatalf("%s: %v", tt.tf, err)
		}

		if got != filepath.FromSlash(tt.want) {
			t.Errorf("%s: got %s, want %s", tt.tf, got, tt.want)
		}
	}
}

func TestFilePath_NewDayNewFile(t *testing.T) {
	last, _ := FilePath("data", "XAUUSDm", domain.TimeframeM1, time.Date(2026, 10, 21, 23, 59, 0, 0, time.UTC))
	first, _ := FilePath("data", "XAUUSDm", domain.TimeframeM1, time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC))
	if last == first {
		t.Fatalf("expected different files, both are %s", last)
	}

}

// เวลาที่ไม่ใช่ UTC ต้องถูกแปลงเป็น UTC ก่อนตั้งชื่อ (วันของ broker = วันตาม UTC)
func TestFilePath_UsesUTCDay(t *testing.T) {
	bangkok := time.FixedZone("ICT", 7*3600)

	got, _ := FilePath("data", "XAUUSDm", domain.TimeframeM1, time.Date(2026, 10, 22, 2, 0, 0, 0, bangkok))
	if filepath.Base(got) != "2026-10-21.csv" {
		t.Errorf("got %s", got)
	}
}

func TestFilePath_RejectsBadInput(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for _, symbol := range []string{"", "../etc", "XAU/USD", `XAU\USD`} {
		if _, err := FilePath("data", symbol, domain.TimeframeM1, at); err == nil {
			t.Errorf("symbol %q: expected error", symbol)
		}
	}
	if _, err := FilePath("data", "XAUUSDm", "W1", at); err == nil {
		t.Error("tf W1: expected error")
	}
}

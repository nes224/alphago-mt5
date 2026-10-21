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



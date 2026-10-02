package domain_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func TestMarketSessionFromUTC(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		hour int
		want string
	}{
		{0, domain.SessionAsian},
		{7, domain.SessionAsian},
		{8, domain.SessionLondon},
		{12, domain.SessionLondon},
		{13, domain.SessionLondonNYOverlap},
		{16, domain.SessionLondonNYOverlap},
		{17, domain.SessionNewYork},
		{21, domain.SessionNewYork},
		{22, domain.SessionAsian},
		{23, domain.SessionAsian},
	}

	for _, tt := range tests {
		got := domain.MarketSessionFromUTC(day.Add(time.Duration(tt.hour) * time.Hour))
		if got != tt.want {
			t.Errorf("hour=%d: expected %s, got %s", tt.hour, tt.want, got)
		}
	}
}

func TestMarketSessionFromUTC_ConvertsNonUTCInputToUTCFirst(t *testing.T) {
	// 09:00 in UTC+7 (Bangkok) is 02:00 UTC -> Asian session, not London.
	bkk := time.FixedZone("ICT", 7*60*60)
	t9amBangkok := time.Date(2026, 10, 1, 9, 0, 0, 0, bkk)

	got := domain.MarketSessionFromUTC(t9amBangkok)
	if got != domain.SessionAsian {
		t.Errorf("expected ASIAN after converting 09:00 ICT to UTC, got %s", got)
	}
}

package domain

import "time"

const (
	SessionAsian           = "ASIAN"
	SessionLondon          = "LONDON"
	SessionLondonNYOverlap = "LONDON_NY_OVERLAP"
	SessionNewYork         = "NEW_YORK"
)

func MarketSessionFromUTC(t time.Time) string {
	hour := t.UTC().Hour()
	switch {
	case hour >= 13 && hour < 17:
		return SessionLondonNYOverlap
	case hour >= 8 && hour < 13:
		return SessionLondon
	case hour >= 17 && hour < 22:
		return SessionNewYork
	default: // 22:00-23:59 and 00:00-07:59
		return SessionAsian
	}
}

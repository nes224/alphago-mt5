package domain

import "strings"

func StrategyTagFromReason(reason string) string {
	if idx := strings.Index(reason, " ("); idx >= 0 {
		return reason[:idx]
	}
	return reason
}

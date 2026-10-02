package domain_test

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func TestStrategyTagFromReason_ExtractsSubstringBeforeParen(t *testing.T) {
	cases := []struct {
		reason string
		want   string
	}{
		{"VOLUME_EXPANSION_BUY (Z:7.75, VolVel:460.6, PriceVel:82.40)", "VOLUME_EXPANSION_BUY"},
		{"LIQUIDITY_SWEEP_FADE_SELL (SweptLevel:1925.58, Trigger:1926.03)", "LIQUIDITY_SWEEP_FADE_SELL"},
		{"NO_PARENS_HERE", "NO_PARENS_HERE"},
		{"", ""},
	}

	for _, c := range cases {
		if got := domain.StrategyTagFromReason(c.reason); got != c.want {
			t.Errorf("StrategyTagFromReason(%q) = %q, want %q", c.reason, got, c.want)
		}
	}
}

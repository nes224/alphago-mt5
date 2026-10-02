package risk_test

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

func TestWinRateGate_AllowsWhenNoStatsYet(t *testing.T) {
	g := risk.NewWinRateGate(0.50, 30)
	if !g.Allows("UNKNOWN_STRATEGY (foo)") {
		t.Error("expected fail-open: no stats recorded for this strategy yet")
	}
}

func TestWinRateGate_AllowsWhenSampleSizeTooSmall(t *testing.T) {
	g := risk.NewWinRateGate(0.50, 30)
	g.UpdateStats([]risk.StrategyWinRate{
		{StrategyTag: "VOLUME_EXPANSION_BUY", Wins: 1, Losses: 9, TotalTrades: 10, WinRate: 0.10},
	})
	// Only 10 trades recorded, below minSampleSize=30 -- must fail-open even
	// though the win rate (10%) is well below the 50% threshold.
	if !g.Allows("VOLUME_EXPANSION_BUY (Z:7.75, VolVel:460.6, PriceVel:82.40)") {
		t.Error("expected fail-open: sample size below minSampleSize")
	}
}

func TestWinRateGate_BlocksWhenEnoughSamplesAndWinRateBelowThreshold(t *testing.T) {
	g := risk.NewWinRateGate(0.50, 30)
	g.UpdateStats([]risk.StrategyWinRate{
		{StrategyTag: "VOLUME_EXPANSION_BUY", Wins: 10, Losses: 40, TotalTrades: 50, WinRate: 0.20},
	})
	if g.Allows("VOLUME_EXPANSION_BUY (Z:7.75, VolVel:460.6, PriceVel:82.40)") {
		t.Error("expected block: 50 trades (>= minSampleSize) with win rate 20% < 50% threshold")
	}
}

func TestWinRateGate_AllowsWhenEnoughSamplesAndWinRateAtOrAboveThreshold(t *testing.T) {
	g := risk.NewWinRateGate(0.50, 30)
	g.UpdateStats([]risk.StrategyWinRate{
		{StrategyTag: "LIQUIDITY_SWEEP_FADE_BUY", Wins: 28, Losses: 22, TotalTrades: 50, WinRate: 0.56},
	})
	if !g.Allows("LIQUIDITY_SWEEP_FADE_BUY (SweptLevel:1925.58, Trigger:1926.03)") {
		t.Error("expected allow: 50 trades with win rate 56% >= 50% threshold")
	}
}

func TestWinRateGate_UpdateStatsReplacesPreviousSnapshot(t *testing.T) {
	g := risk.NewWinRateGate(0.50, 30)
	g.UpdateStats([]risk.StrategyWinRate{
		{StrategyTag: "X", Wins: 10, Losses: 40, TotalTrades: 50, WinRate: 0.20},
	})
	if g.Allows("X (foo)") {
		t.Fatal("expected block on first snapshot (20% win rate)")
	}

	// A later refresh with a better track record must take effect.
	g.UpdateStats([]risk.StrategyWinRate{
		{StrategyTag: "X", Wins: 30, Losses: 20, TotalTrades: 50, WinRate: 0.60},
	})
	if !g.Allows("X (foo)") {
		t.Error("expected allow after UpdateStats refreshed to 60% win rate")
	}
}

func TestWinRateGate_IsolatesDifferentStrategyTags(t *testing.T) {
	g := risk.NewWinRateGate(0.50, 30)
	g.UpdateStats([]risk.StrategyWinRate{
		{StrategyTag: "GOOD_STRAT", Wins: 40, Losses: 10, TotalTrades: 50, WinRate: 0.80},
		{StrategyTag: "BAD_STRAT", Wins: 10, Losses: 40, TotalTrades: 50, WinRate: 0.20},
	})
	if !g.Allows("GOOD_STRAT (foo)") {
		t.Error("expected GOOD_STRAT to be allowed")
	}
	if g.Allows("BAD_STRAT (foo)") {
		t.Error("expected BAD_STRAT to be blocked")
	}
}

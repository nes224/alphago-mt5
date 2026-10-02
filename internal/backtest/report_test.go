package backtest_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/backtest"
	"github.com/nes224/alphago-mt5/internal/core/domain"
)

func sampleTrades() []backtest.Trade {
	base := time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC) // 08:00 UTC = LONDON session
	return []backtest.Trade{
		{StrategyTag: "VOLUME_EXPANSION_BUY", Session: domain.SessionLondon, PnL: 100, IsWin: true, EntryTime: base},
		{StrategyTag: "VOLUME_EXPANSION_BUY", Session: domain.SessionLondon, PnL: -50, IsWin: false, EntryTime: base.Add(time.Hour)},
		{StrategyTag: "LIQUIDITY_SWEEP_FADE_SELL", Session: domain.SessionAsian, PnL: -30, IsWin: false, EntryTime: base.Add(2 * time.Hour)},
		{StrategyTag: "LIQUIDITY_SWEEP_FADE_SELL", Session: domain.SessionAsian, PnL: 80, IsWin: true, EntryTime: base.Add(3 * time.Hour)},
	}
}

func TestReport_WinRate_OverallAcrossAllTrades(t *testing.T) {
	r := backtest.Report{Trades: sampleTrades()}
	if got := r.WinRate(); got < 0.4999 || got > 0.5001 {
		t.Errorf("expected overall win rate 0.5 (2 wins / 4 trades), got %f", got)
	}
}

func TestReport_WinRate_EmptyIsZero(t *testing.T) {
	r := backtest.Report{}
	if got := r.WinRate(); got != 0 {
		t.Errorf("expected 0 win rate with no trades, got %f", got)
	}
}

func TestReport_WinRateByStrategy_GroupsCorrectly(t *testing.T) {
	r := backtest.Report{Trades: sampleTrades()}
	byStrategy := r.WinRateByStrategy()

	ve := byStrategy["VOLUME_EXPANSION_BUY"]
	if ve.Wins != 1 || ve.Losses != 1 || ve.TotalTrades != 2 {
		t.Errorf("expected VOLUME_EXPANSION_BUY 1W/1L, got %+v", ve)
	}
	if ve.TotalProfit != 50 {
		t.Errorf("expected VOLUME_EXPANSION_BUY total profit 50 (100-50), got %f", ve.TotalProfit)
	}

	sweep := byStrategy["LIQUIDITY_SWEEP_FADE_SELL"]
	if sweep.Wins != 1 || sweep.Losses != 1 {
		t.Errorf("expected LIQUIDITY_SWEEP_FADE_SELL 1W/1L, got %+v", sweep)
	}
	if sweep.TotalProfit != 50 {
		t.Errorf("expected LIQUIDITY_SWEEP_FADE_SELL total profit 50 (-30+80), got %f", sweep.TotalProfit)
	}
}

func TestReport_WinRateBySession_GroupsCorrectly(t *testing.T) {
	r := backtest.Report{Trades: sampleTrades()}
	bySession := r.WinRateBySession()

	london := bySession[domain.SessionLondon]
	if london.Wins != 1 || london.Losses != 1 {
		t.Errorf("expected LONDON 1W/1L, got %+v", london)
	}
	asian := bySession[domain.SessionAsian]
	if asian.Wins != 1 || asian.Losses != 1 {
		t.Errorf("expected ASIAN 1W/1L, got %+v", asian)
	}
}

func TestReport_TotalPnL_SumsAllTrades(t *testing.T) {
	r := backtest.Report{Trades: sampleTrades()}
	if got := r.TotalPnL(); got != 100 {
		t.Errorf("expected total PnL 100 (100-50-30+80), got %f", got)
	}
}

func TestReport_MaxDrawdown_LargestPeakToTroughDecline(t *testing.T) {
	// Equity curve starting at 1000: 1000 -> 1100 -> 1050 -> 900 -> 1000
	// Peak 1100 at step 1, trough 900 at step 3 -> drawdown = 200.
	trades := []backtest.Trade{
		{PnL: 100},  // 1000 -> 1100 (new peak)
		{PnL: -50},  // 1100 -> 1050
		{PnL: -150}, // 1050 -> 900 (trough, drawdown from peak 1100 = 200)
		{PnL: 100},  // 900 -> 1000 (recovers partially, not a new peak)
	}
	r := backtest.Report{Trades: trades, InitialBalance: 1000}

	if got := r.MaxDrawdown(); got != 200 {
		t.Errorf("expected max drawdown 200, got %f", got)
	}
}

func TestReport_MaxDrawdown_NoDrawdownWhenAlwaysRising(t *testing.T) {
	trades := []backtest.Trade{{PnL: 10}, {PnL: 10}, {PnL: 10}}
	r := backtest.Report{Trades: trades, InitialBalance: 1000}
	if got := r.MaxDrawdown(); got != 0 {
		t.Errorf("expected 0 drawdown for a monotonically rising equity curve, got %f", got)
	}
}

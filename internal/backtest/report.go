package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// Trade is one closed simulated trade from a backtest Run. StrategyTag
// mirrors cmd/app/main.go's live extraction exactly (substring of Reason
// before " (") and Session reuses domain.MarketSessionFromUTC directly, so
// these stats are directly comparable to the live GET /api/v1/strategy-stats
// numbers.
type Trade struct {
	Symbol      string
	StrategyTag string
	Reason      string
	Session     string
	Action      domain.SignalAction
	EntryTime   time.Time
	ExitTime    time.Time
	EntryPrice  float64
	ExitPrice   float64
	LotSize     float64
	PnL         float64
	IsWin       bool
}

// Report is the result of a backtest Run. Trades must be in chronological
// order (the order Runner appends them in) -- MaxDrawdown depends on it.
type Report struct {
	Trades         []Trade
	StillOpenAtEnd int
	InitialBalance float64
	FinalBalance   float64
}

// WinLoss summarizes accumulated win/loss stats for a group of trades (by
// strategy or by session) -- same shape as the live database.WinRateStat so
// backtest and live numbers read the same way side by side.
type WinLoss struct {
	Wins        int
	Losses      int
	TotalTrades int
	WinRate     float64
	TotalProfit float64
}

func addTrade(wl WinLoss, t Trade) WinLoss {
	if t.IsWin {
		wl.Wins++
	} else {
		wl.Losses++
	}
	wl.TotalTrades++
	wl.TotalProfit += t.PnL
	if wl.TotalTrades > 0 {
		wl.WinRate = float64(wl.Wins) / float64(wl.TotalTrades)
	}
	return wl
}

// WinRate returns the overall win rate (0.0-1.0) across all trades, or 0 if
// there are none.
func (r Report) WinRate() float64 {
	if len(r.Trades) == 0 {
		return 0
	}
	wins := 0
	for _, t := range r.Trades {
		if t.IsWin {
			wins++
		}
	}
	return float64(wins) / float64(len(r.Trades))
}

// WinRateByStrategy groups trades by StrategyTag.
func (r Report) WinRateByStrategy() map[string]WinLoss {
	out := make(map[string]WinLoss)
	for _, t := range r.Trades {
		out[t.StrategyTag] = addTrade(out[t.StrategyTag], t)
	}
	return out
}

// WinRateBySession groups trades by Session (see domain.MarketSessionFromUTC).
func (r Report) WinRateBySession() map[string]WinLoss {
	out := make(map[string]WinLoss)
	for _, t := range r.Trades {
		out[t.Session] = addTrade(out[t.Session], t)
	}
	return out
}

// TotalPnL sums PnL across all trades.
func (r Report) TotalPnL() float64 {
	var total float64
	for _, t := range r.Trades {
		total += t.PnL
	}
	return total
}

// MaxDrawdown returns the largest peak-to-trough decline (in the same $
// units as PnL) in the equity curve implied by InitialBalance plus the
// cumulative PnL of Trades in order. 0 if the curve never declines from its
// running peak.
func (r Report) MaxDrawdown() float64 {
	equity := r.InitialBalance
	peak := equity
	var maxDD float64

	for _, t := range r.Trades {
		equity += t.PnL
		if equity > peak {
			peak = equity
		}
		if dd := peak - equity; dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

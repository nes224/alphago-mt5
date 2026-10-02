package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

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

type Report struct {
	Trades         []Trade
	StillOpenAtEnd int
	InitialBalance float64
	FinalBalance   float64
}

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

func (r Report) WinRateByStrategy() map[string]WinLoss {
	out := make(map[string]WinLoss)
	for _, t := range r.Trades {
		out[t.StrategyTag] = addTrade(out[t.StrategyTag], t)
	}
	return out
}

func (r Report) WinRateBySession() map[string]WinLoss {
	out := make(map[string]WinLoss)
	for _, t := range r.Trades {
		out[t.Session] = addTrade(out[t.Session], t)
	}
	return out
}

func (r Report) TotalPnL() float64 {
	var total float64
	for _, t := range r.Trades {
		total += t.PnL
	}
	return total
}

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

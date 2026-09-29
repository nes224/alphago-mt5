package risk

import (
	"fmt"
	"math"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type RiskManager struct {
	riskPerTradePercent float64
	accountBalance      float64
	minLotSize          float64
	maxLotSize          float64
}

func NewRiskManager(riskPerTradePercent, accountBalance, minLot, maxLot float64) *RiskManager {
	return &RiskManager{
		riskPerTradePercent: riskPerTradePercent,
		accountBalance:      accountBalance,
		minLotSize:          minLot,
		maxLotSize:          maxLot,
	}
}

type PreparedOrder struct {
	Symbol     string
	Action     domain.SignalAction
	LotSize    float64
	EntryPrice float64
	StopLoss   float64
	TakeProfit float64
	Reason     string
}

func (r *RiskManager) CalculateOrder(signal domain.OrderSignal, metrics domain.TickMetrics) (*PreparedOrder, error) {
	if metrics.StdDev == 0 {
		return nil, fmt.Errorf("zero volatility (StdDev=0), skipping order calculation")
	}

	slDistance := 2.0 * metrics.StdDev
	var entryPrice, stopLoss, takeProfit float64

	if signal.Action == domain.SignalAction(domain.ActionBuy) {
		entryPrice = metrics.Ask
		stopLoss = entryPrice - slDistance
		takeProfit = metrics.Mean
	} else {
		entryPrice = metrics.Bid
		stopLoss = entryPrice + slDistance
		takeProfit = metrics.Mean
	}

	riskAmount := r.accountBalance * r.riskPerTradePercent

	contractSize := 100.0
	lotSize := riskAmount / (slDistance * contractSize)

	lotSize = math.Max(r.minLotSize, math.Min(r.maxLotSize, lotSize))
	lotSize = math.Round(lotSize*100) / 100

	return &PreparedOrder{
		Symbol:     signal.Symbol,
		Action:     signal.Action,
		LotSize:    lotSize,
		EntryPrice: entryPrice,
		StopLoss:   stopLoss,
		TakeProfit: takeProfit,
		Reason:     signal.Reason,
	}, nil

}

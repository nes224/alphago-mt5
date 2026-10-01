package risk

import (
	"fmt"
	"math"
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type RiskManager struct {
	mu                  sync.RWMutex
	riskPerTradePercent float64
	accountBalance      float64
	minLotSize          float64
	maxLotSize          float64
	minSLDistance       float64 // ระยะ SL/TP ต่ำสุด (หน่วยราคา เช่น 3.0 = $3 บน Gold) กัน SL/TP แคบเกินไปจน spread กินกำไรหมด
	maxSLDistance       float64 // ระยะ SL/TP สูงสุด กันความเสี่ยงต่อไม้สูงเกินไปตอนตลาดผันผวนหนัก
}

func NewRiskManager(riskPerTradePercent, accountBalance, minLot, maxLot, minSLDistance, maxSLDistance float64) *RiskManager {
	return &RiskManager{
		riskPerTradePercent: riskPerTradePercent,
		accountBalance:      accountBalance,
		minLotSize:          minLot,
		maxLotSize:          maxLot,
		minSLDistance:       minSLDistance,
		maxSLDistance:       maxSLDistance,
	}
}

// UpdateConfig เปลี่ยน risk policy แบบ live (ไม่ต้อง restart service) — เรียก
// จาก RiskConfigService ตอน PUT /api/v1/risk/config ผ่าน validation แล้ว
func (r *RiskManager) UpdateConfig(riskPerTradePercent, minLotSize, maxLotSize, minSLDistance, maxSLDistance float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.riskPerTradePercent = riskPerTradePercent
	r.minLotSize = minLotSize
	r.maxLotSize = maxLotSize
	r.minSLDistance = minSLDistance
	r.maxSLDistance = maxSLDistance
}

type PreparedOrder struct {
	Symbol     string
	Action     domain.SignalAction
	LotSize    float64
	EntryPrice float64
	StopLoss   float64
	TakeProfit float64
	Reason     string
	OutboxID   uint // pending_orders row ID (0 ถ้าไม่มี outbox store ผูกไว้) — ให้ sender goroutine อัปเดตสถานะกลับหลังรู้ผลจริงจาก MT5
}

func (r *RiskManager) CalculateOrder(signal domain.OrderSignal, metrics domain.TickMetrics) (*PreparedOrder, error) {
	if metrics.StdDev == 0 {
		return nil, fmt.Errorf("zero volatility (StdDev=0), skipping order calculation")
	}

	r.mu.RLock()
	minSLDistance, maxSLDistance := r.minSLDistance, r.maxSLDistance
	minLotSize, maxLotSize := r.minLotSize, r.maxLotSize
	accountBalance, riskPerTradePercent := r.accountBalance, r.riskPerTradePercent
	r.mu.RUnlock()

	slDistance := 2.0 * metrics.StdDev
	slDistance = math.Max(minSLDistance, math.Min(maxSLDistance, slDistance))

	var entryPrice, stopLoss, takeProfit float64

	if signal.Action == domain.SignalAction(domain.ActionBuy) {
		entryPrice = metrics.Ask
		stopLoss = entryPrice - slDistance
		takeProfit = entryPrice + slDistance
	} else {
		entryPrice = metrics.Bid
		stopLoss = entryPrice + slDistance
		takeProfit = entryPrice - slDistance
	}

	riskAmount := accountBalance * riskPerTradePercent

	contractSize := 100.0
	lotSize := riskAmount / (slDistance * contractSize)

	lotSize = math.Max(minLotSize, math.Min(maxLotSize, lotSize))
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

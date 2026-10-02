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

	// minSLDistance/maxSLDistance ไม่ใช่ตัวขับหลักของ SL/TP อีกต่อไป (เดิมคือ
	// ตัวขับหลักจน minSLDistance ชนพื้นเกือบตลอดเวลา เพราะตั้งไว้ตอนตลาด
	// ผันผวนน้อยกว่านี้มาก — ดู ROADMAP.md "Volatility-Adaptive Position
	// Sizing") ตอนนี้เป็นแค่ราวกันตกสองข้างของ slDistance ที่คำนวณจาก
	// volatilityMultiplier × SizingVolatility แทน
	minSLDistance float64
	maxSLDistance float64

	// volatilityMultiplier คือตัวคูณ metrics.SizingVolatility (StdDev ของ M15
	// TimeWindow) เพื่อได้ SL distance — ยิ่งสูง SL ยิ่งกว้าง lot ยิ่งเล็กลงตาม
	// (ความเสี่ยงเป็นเงินคงที่ตาม riskPerTradePercent เสมอ)
	volatilityMultiplier float64

	// atrMultiplier/useATRForSizing คือทางเลือกใหม่แทน volatilityMultiplier ×
	// SizingVolatility — ใช้ metrics.ATR (M5×14 Wilder-smoothed) แทน StdDev
	// ของ M15 ตอน useATRForSizing=true และ ATR ready แล้วเท่านั้น (ดู
	// domain.RiskConfig.UseATRForSizing — default false โดยตั้งใจ)
	atrMultiplier   float64
	useATRForSizing bool
}

func NewRiskManager(riskPerTradePercent, accountBalance, minLot, maxLot, minSLDistance, maxSLDistance, volatilityMultiplier, atrMultiplier float64, useATRForSizing bool) *RiskManager {
	return &RiskManager{
		riskPerTradePercent:  riskPerTradePercent,
		accountBalance:       accountBalance,
		minLotSize:           minLot,
		maxLotSize:           maxLot,
		minSLDistance:        minSLDistance,
		maxSLDistance:        maxSLDistance,
		volatilityMultiplier: volatilityMultiplier,
		atrMultiplier:        atrMultiplier,
		useATRForSizing:      useATRForSizing,
	}
}

// UpdateConfig เปลี่ยน risk policy แบบ live (ไม่ต้อง restart service) — เรียก
// จาก RiskConfigService ตอน PUT /api/v1/risk/config ผ่าน validation แล้ว
func (r *RiskManager) UpdateConfig(riskPerTradePercent, minLotSize, maxLotSize, minSLDistance, maxSLDistance, volatilityMultiplier, atrMultiplier float64, useATRForSizing bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.riskPerTradePercent = riskPerTradePercent
	r.minLotSize = minLotSize
	r.maxLotSize = maxLotSize
	r.minSLDistance = minSLDistance
	r.maxSLDistance = maxSLDistance
	r.volatilityMultiplier = volatilityMultiplier
	r.atrMultiplier = atrMultiplier
	r.useATRForSizing = useATRForSizing
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
	r.mu.RLock()
	minSLDistance, maxSLDistance := r.minSLDistance, r.maxSLDistance
	minLotSize, maxLotSize := r.minLotSize, r.maxLotSize
	accountBalance, riskPerTradePercent := r.accountBalance, r.riskPerTradePercent
	volatilityMultiplier, atrMultiplier, useATRForSizing := r.volatilityMultiplier, r.atrMultiplier, r.useATRForSizing
	r.mu.RUnlock()

	// Fallback chain: ATR (M5×14, ถ้าเปิดใช้งานและ ready แล้ว) -> SizingVolatility
	// (StdDev ของ M15 TimeWindow) -> StdDev (window 20 tick เดิม, เฉพาะตอน M15
	// ยังไม่มีข้อมูลพอ เช่น เพิ่ง restart ไม่ถึง 15 นาที) -> error (ข้ามไม้นี้
	// ไปเลย แทนที่จะเทรดโดยไม่รู้ความผันผวนจริง)
	var volatility, multiplier float64
	switch {
	case useATRForSizing && metrics.ATRReady && metrics.ATR > 0:
		volatility, multiplier = metrics.ATR, atrMultiplier
	case metrics.SizingVolatility > 0:
		volatility, multiplier = metrics.SizingVolatility, volatilityMultiplier
	case metrics.StdDev > 0:
		volatility, multiplier = metrics.StdDev, volatilityMultiplier
	default:
		return nil, fmt.Errorf("zero volatility, skipping order calculation")
	}

	slDistance := multiplier * volatility
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

	// lotSize ต่ำกว่า minLotSize แปลว่า "บัญชีเล็กเกินไปสำหรับความผันผวนตอนนี้"
	// — เดิมปัดขึ้นเป็น minLotSize เงียบๆ ทำให้ความเสี่ยงจริงเกิน
	// riskPerTradePercent ที่ตั้งใจไว้โดยไม่มีใครรู้ ตอนนี้ข้ามไม้นี้ไปแทน
	// (ExecutionRouter มี error-handling path นี้อยู่แล้ว — บันทึกเป็น
	// SignalStatusRejectedSizing)
	if lotSize < minLotSize {
		return nil, fmt.Errorf("position too small for current volatility given account balance (need lot=%.4f, min=%.2f) — skipping to avoid exceeding %.1f%% risk", lotSize, minLotSize, riskPerTradePercent*100)
	}
	lotSize = math.Min(maxLotSize, lotSize)
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

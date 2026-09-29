package risk

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

var (
	ErrDailyDrawdownExceeded = errors.New("risk guard: daily drawdown limit exceeded (circuit breaker tripped)")
	ErrMaxPositionsReached   = errors.New("risk guard: maximum open positions limit reached")
	ErrSpreadTooHigh         = errors.New("risk guard: current spread exceeds maximum threshold")
	ErrTradingLocked         = errors.New("risk guard: trading is locked due to consecutive losses")
)

type RiskGuardConfig struct {
	MaxDailyLossPercent  float64 // เช่น 0.03 = 3% ของ Starting Daily Balance
	MaxOpenPositions     int     // จำนวน Order ที่เปิดพร้อมกันได้สูงสุด (เช่น 3)
	MaxSpreadPips        float64 // Spread สูงสุดที่ยอมให้ยิง Order (เช่น 50 pips / 0.50 สำหรับ Gold)
	MaxConsecutiveLosses int     // จำนวนการแพ้ติดกันสูงสุดก่อน Lock ชั่วคราว
}

type RiskGuard struct {
	mu                  sync.RWMutex
	config              RiskGuardConfig
	startingDailyEquity float64
	currentDailyEquity  float64
	openPositionsCount  int
	consecutiveLosses   int
	isCircuitTripped    bool
	lastResetDate       string
}

func NewRiskGuard(config RiskGuardConfig, initialEquity float64) *RiskGuard {
	today := time.Now().Format("2006-01-02")
	return &RiskGuard{
		config:              config,
		startingDailyEquity: initialEquity,
		currentDailyEquity:  initialEquity,
		lastResetDate:       today,
	}
}

func (rg *RiskGuard) ValidateOrder(order PreparedOrder, metrics domain.TickMetrics) error {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	// 1. Reset Daily Stats หากข้ามวันใหม่
	rg.checkDailyReset()

	// 2. เช็กว่า Circuit Breaker ทำงานอยู่หรือไม่ (Daily Loss ทะลุเป้า)
	if rg.isCircuitTripped {
		return ErrDailyDrawdownExceeded
	}

	// 3. เช็ก Spread (ป้องกันช่วง Spread ถ่างหนักๆ เช่น ข่าวออก / เปลี่ยนรอบวัน)
	if metrics.Spread > rg.config.MaxSpreadPips {
		return fmt.Errorf("%w: current spread %.2f < max allowed %.2f", ErrSpreadTooHigh, metrics.Spread, rg.config.MaxSpreadPips)
	}

	// 4. เช็กจำนวน Open Positions
	if rg.openPositionsCount >= rg.config.MaxOpenPositions {
		return fmt.Errorf("%w: active positions %d >= max %d", ErrMaxPositionsReached, rg.openPositionsCount, rg.config.MaxOpenPositions)
	}

	// 5. เช็ก Consecutive Losses Lockout
	if rg.config.MaxConsecutiveLosses > 0 && rg.consecutiveLosses >= rg.config.MaxConsecutiveLosses {
		return fmt.Errorf("%w: %d consecutive losses reach limit", ErrTradingLocked, rg.consecutiveLosses)
	}

	return nil
}

// UpdateAccountEquity อัปเดต Equity สดจาก MT5 และสับ Circuit Breaker ทันทีถ้าลบเกิน Max Daily Loss
func (rg *RiskGuard) UpdateAccountEquity(currentEquity float64) {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	rg.checkDailyReset()
	rg.currentDailyEquity = currentEquity

	drawdownPercent := (rg.startingDailyEquity - currentEquity) / rg.startingDailyEquity
	if drawdownPercent >= rg.config.MaxDailyLossPercent {
		rg.isCircuitTripped = true
	}
}

func (rg *RiskGuard) RecordTradeResult(isWin bool) {
	rg.mu.Lock()
	defer rg.mu.Unlock()

	if isWin {
		rg.consecutiveLosses = 0
	} else {
		rg.consecutiveLosses += 1
	}
}

// UpdateActivePositionsCount อัปเดตจำนวนออเดอร์ที่ถืออยู่ปัจจุบัน
func (rg *RiskGuard) UpdateActivePositionsCount(count int) {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	rg.openPositionsCount = count
}

// ResetCircuitBreaker ใช้สำหรับ Admin หรือระบบ Manual สั่งปลดล็อก Circuit Breaker
func (rg *RiskGuard) ResetCircuitBreaker() {
	rg.mu.Lock()
	defer rg.mu.Unlock()
	rg.isCircuitTripped = false
	rg.consecutiveLosses = 0
}

func (rg *RiskGuard) checkDailyReset() {
	today := time.Now().Format("2006-01-02")
	if today != rg.lastResetDate {
		rg.startingDailyEquity = rg.currentDailyEquity
		rg.isCircuitTripped = false
		rg.consecutiveLosses = 0
		rg.lastResetDate = today
	}
}

package risk

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

var (
	ErrDailyDrawdownExceeded = errors.New("risk guard: daily drawdown limit exceeded (circuit breaker tripped)")
	ErrMaxPositionsReached   = errors.New("risk guard: maximum open positions limit reached")
	ErrSpreadTooHigh         = errors.New("risk guard: current spread exceeds maximum threshold")
	ErrTradingLocked         = errors.New("risk guard: trading is locked due to consecutive losses")
	ErrPositionAlreadyOpen   = errors.New("risk guard: symbol already has an open position")
)

type RiskGuardConfig struct {
	MaxDailyLossPercent  float64 // เช่น 0.03 = 3% ของ Starting Daily Balance
	MaxOpenPositions     int     // จำนวน Order ที่เปิดพร้อมกันได้สูงสุด (เช่น 3)
	MaxSpreadPips        float64 // Spread สูงสุดที่ยอมให้ยิง Order (เช่น 50 pips / 0.50 สำหรับ Gold)
	MaxConsecutiveLosses int     // จำนวนการแพ้ติดกันสูงสุดก่อน Lock ชั่วคราว
}

// PersistedState คือ snapshot ของสถานะ RiskGuard ที่ต้องรอดจาก restart —
// ไม่ผูกกับ storage ใดๆ โดยตรง (ผ่าน StateStore แทน) เพื่อให้ package risk
// ไม่ต้องรู้จัก database/GORM เลยตามหลัก Hexagonal Architecture
type PersistedState struct {
	StartingDailyEquity float64
	CurrentDailyEquity  float64
	OpenPositionsCount  int
	ConsecutiveLosses   int
	IsCircuitTripped    bool
	LastResetDate       string
}

// StateStore เป็น port ที่ adapter ชั้นนอก (เช่น internal/adapters/database)
// ต้อง implement เพื่อให้ RiskGuard persist สถานะข้าม restart ได้
type StateStore interface {
	// LoadRiskState คืน ok=false ถ้ายังไม่เคยบันทึกไว้เลย (ครั้งแรกที่รัน)
	LoadRiskState() (state PersistedState, ok bool, err error)
	SaveRiskState(state PersistedState) error
}

type RiskGuard struct {
	mu                  sync.RWMutex
	config              RiskGuardConfig
	startingDailyEquity float64
	currentDailyEquity  float64
	openPositionsCount  int
	openPositionSymbols map[string]int // symbol -> จำนวน position ที่เปิดอยู่ตอนนี้ (เรา track เอง ไม่ได้ถาม MT5 สด)
	consecutiveLosses   int
	isCircuitTripped    bool
	lastResetDate       string
	store               StateStore
}

func NewRiskGuard(config RiskGuardConfig, initialEquity float64) *RiskGuard {
	today := time.Now().Format("2006-01-02")
	return &RiskGuard{
		config:              config,
		startingDailyEquity: initialEquity,
		currentDailyEquity:  initialEquity,
		lastResetDate:       today,
		openPositionSymbols: make(map[string]int),
	}
}

// AttachStore ผูก StateStore เข้ากับ RiskGuard — ถ้ามีสถานะที่บันทึกไว้ก่อนหน้า
// (จาก run ก่อน restart) จะโหลดมาทับค่า initialEquity ที่ตั้งไว้ตอนสร้างทันที
// เรียกครั้งเดียวตอน wiring ใน main.go ก่อน Start() ทุกตัว
func (rg *RiskGuard) AttachStore(store StateStore) error {
	state, ok, err := store.LoadRiskState()
	if err != nil {
		return fmt.Errorf("failed to load risk guard state: %w", err)
	}

	rg.mu.Lock()
	rg.store = store
	if ok {
		rg.startingDailyEquity = state.StartingDailyEquity
		rg.currentDailyEquity = state.CurrentDailyEquity
		rg.openPositionsCount = state.OpenPositionsCount
		rg.consecutiveLosses = state.ConsecutiveLosses
		rg.isCircuitTripped = state.IsCircuitTripped
		rg.lastResetDate = state.LastResetDate
	}
	rg.mu.Unlock()

	return nil
}

func (rg *RiskGuard) ValidateOrder(order PreparedOrder, metrics domain.TickMetrics) error {
	// 1 & 2. Circuit Breaker (Daily Loss) และ Consecutive Losses Lockout
	if err := rg.CheckCircuitBreaker(); err != nil {
		return err
	}

	rg.mu.Lock()
	defer rg.mu.Unlock()

	// 3. เช็ก Spread (ป้องกันช่วง Spread ถ่างหนักๆ เช่น ข่าวออก / เปลี่ยนรอบวัน)
	if metrics.Spread > rg.config.MaxSpreadPips {
		return fmt.Errorf("%w: current spread %.2f < max allowed %.2f", ErrSpreadTooHigh, metrics.Spread, rg.config.MaxSpreadPips)
	}

	// 4. เช็กจำนวน Open Positions
	if rg.openPositionsCount >= rg.config.MaxOpenPositions {
		return fmt.Errorf("%w: active positions %d >= max %d", ErrMaxPositionsReached, rg.openPositionsCount, rg.config.MaxOpenPositions)
	}

	// 5. ห้ามเปิด position ใหม่ (ไม่ว่า BUY หรือ SELL) ถ้า symbol นี้มี position
	// เปิดค้างอยู่แล้ว — กัน strategy คนละตัวยิงสวนทางกันเองบนบัญชี Hedge ที่
	// BUY/SELL ไม่หักล้างกันอัตโนมัติ (เคลียร์ตอน position ปิดจริงผ่าน MarkPositionClosed)
	if rg.openPositionSymbols[order.Symbol] > 0 {
		return fmt.Errorf("%w: %s", ErrPositionAlreadyOpen, order.Symbol)
	}

	return nil
}

// MarkPositionOpened บันทึกว่า symbol นี้มี position เปิดใหม่แล้ว — เรียกทันที
// หลัง dispatch order สำเร็จ (ExecutionRouter) กัน signal ตัวถัดไปยิงสวนทาง
// ก่อน position เดิมจะปิด
func (rg *RiskGuard) MarkPositionOpened(symbol string) {
	rg.mu.Lock()
	rg.openPositionSymbols[symbol]++
	rg.openPositionsCount++
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	rg.persist(state)
}

// MarkPositionClosed บันทึกว่า symbol นี้มี position ปิดไปแล้ว — เรียกจาก
// trade_closed event ที่ EA ส่งมาตอน OnTradeTransaction()
func (rg *RiskGuard) MarkPositionClosed(symbol string) {
	rg.mu.Lock()
	if rg.openPositionSymbols[symbol] > 0 {
		rg.openPositionSymbols[symbol]--
	}
	if rg.openPositionsCount > 0 {
		rg.openPositionsCount--
	}
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	rg.persist(state)
}

// HasOpenPosition คืน true ถ้า symbol นี้มี position เปิดค้างอยู่ตามที่เรา track เอง
func (rg *RiskGuard) HasOpenPosition(symbol string) bool {
	rg.mu.RLock()
	defer rg.mu.RUnlock()
	return rg.openPositionSymbols[symbol] > 0
}

// CheckCircuitBreaker เช็กเฉพาะสถานะ Daily Drawdown Circuit Breaker และ
// Consecutive Losses Lockout โดยไม่ต้องมี PreparedOrder/TickMetrics ประกอบ —
// ใช้ได้ทั้งจาก Quant Signal pipeline (ผ่าน ValidateOrder) และจาก Manual
// Order pipeline (เช่น TradeService) ที่ไม่มี live tick metrics ให้เช็ก spread
func (rg *RiskGuard) CheckCircuitBreaker() error {
	rg.mu.Lock()
	changed := rg.checkDailyResetLocked()
	tripped := rg.isCircuitTripped
	consecutiveLosses := rg.consecutiveLosses
	maxConsecutiveLosses := rg.config.MaxConsecutiveLosses
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	if changed {
		rg.persist(state)
	}

	if tripped {
		return ErrDailyDrawdownExceeded
	}

	if maxConsecutiveLosses > 0 && consecutiveLosses >= maxConsecutiveLosses {
		return fmt.Errorf("%w: %d consecutive losses reach limit", ErrTradingLocked, consecutiveLosses)
	}

	return nil
}

// RiskGuardStatus เป็น snapshot ของสถานะ RiskGuard ปัจจุบัน สำหรับ monitoring/API
type RiskGuardStatus struct {
	CircuitTripped      bool    `json:"circuit_tripped"`
	ConsecutiveLosses   int     `json:"consecutive_losses"`
	OpenPositionsCount  int     `json:"open_positions_count"`
	StartingDailyEquity float64 `json:"starting_daily_equity"`
	CurrentDailyEquity  float64 `json:"current_daily_equity"`
}

func (rg *RiskGuard) Status() RiskGuardStatus {
	rg.mu.RLock()
	defer rg.mu.RUnlock()

	return RiskGuardStatus{
		CircuitTripped:      rg.isCircuitTripped,
		ConsecutiveLosses:   rg.consecutiveLosses,
		OpenPositionsCount:  rg.openPositionsCount,
		StartingDailyEquity: rg.startingDailyEquity,
		CurrentDailyEquity:  rg.currentDailyEquity,
	}
}

// UpdateAccountEquity อัปเดต Equity สดจาก MT5 และสับ Circuit Breaker ทันทีถ้าลบเกิน Max Daily Loss
func (rg *RiskGuard) UpdateAccountEquity(currentEquity float64) {
	rg.mu.Lock()
	rg.checkDailyResetLocked()
	rg.currentDailyEquity = currentEquity

	drawdownPercent := (rg.startingDailyEquity - currentEquity) / rg.startingDailyEquity
	if drawdownPercent >= rg.config.MaxDailyLossPercent {
		rg.isCircuitTripped = true
	}
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	rg.persist(state)
}

func (rg *RiskGuard) RecordTradeResult(isWin bool) {
	rg.mu.Lock()
	if isWin {
		rg.consecutiveLosses = 0
	} else {
		rg.consecutiveLosses += 1
	}
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	rg.persist(state)
}

// UpdateActivePositionsCount อัปเดตจำนวนออเดอร์ที่ถืออยู่ปัจจุบัน
func (rg *RiskGuard) UpdateActivePositionsCount(count int) {
	rg.mu.Lock()
	rg.openPositionsCount = count
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	rg.persist(state)
}

// ResetCircuitBreaker ใช้สำหรับ Admin หรือระบบ Manual สั่งปลดล็อก Circuit Breaker
func (rg *RiskGuard) ResetCircuitBreaker() {
	rg.mu.Lock()
	rg.isCircuitTripped = false
	rg.consecutiveLosses = 0
	state := rg.snapshotLocked()
	rg.mu.Unlock()

	rg.persist(state)
}

// checkDailyResetLocked ต้องเรียกตอนถือ rg.mu อยู่แล้วเท่านั้น คืน true ถ้ามีการ reset จริง
func (rg *RiskGuard) checkDailyResetLocked() bool {
	today := time.Now().Format("2006-01-02")
	if today != rg.lastResetDate {
		rg.startingDailyEquity = rg.currentDailyEquity
		rg.isCircuitTripped = false
		rg.consecutiveLosses = 0
		rg.lastResetDate = today
		return true
	}
	return false
}

// snapshotLocked ต้องเรียกตอนถือ rg.mu อยู่แล้วเท่านั้น
func (rg *RiskGuard) snapshotLocked() PersistedState {
	return PersistedState{
		StartingDailyEquity: rg.startingDailyEquity,
		CurrentDailyEquity:  rg.currentDailyEquity,
		OpenPositionsCount:  rg.openPositionsCount,
		ConsecutiveLosses:   rg.consecutiveLosses,
		IsCircuitTripped:    rg.isCircuitTripped,
		LastResetDate:       rg.lastResetDate,
	}
}

// persist ต้องเรียกตอน "ไม่ถือ" rg.mu แล้วเท่านั้น (I/O ไม่ควรถือ lock ค้าง)
func (rg *RiskGuard) persist(state PersistedState) {
	if rg.store == nil {
		return
	}
	if err := rg.store.SaveRiskState(state); err != nil {
		log.Warn().Err(err).Msg("[RiskGuard] failed to persist state")
	}
}

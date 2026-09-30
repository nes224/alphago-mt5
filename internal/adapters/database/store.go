package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

// singleRowID คือ ID ตายตัวสำหรับตารางที่มีแค่แถวเดียวเสมอ (account_state,
// risk_guard_state) — ใช้แทนการมีหลายแถวแล้วต้องมานั่งหาว่าแถวไหนคือ "ปัจจุบัน"
const singleRowID = 1

// Store รวม implementation ของ risk.StateStore, pipeline.SignalStore และ
// account balance store ไว้ในที่เดียว ผูกกับ *gorm.DB ตัวเดียวกัน
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

// --- risk.StateStore ---

var _ risk.StateStore = (*Store)(nil)

func (s *Store) LoadRiskState() (risk.PersistedState, bool, error) {
	var m RiskGuardStateModel
	err := s.db.First(&m, singleRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return risk.PersistedState{}, false, nil
	}
	if err != nil {
		return risk.PersistedState{}, false, fmt.Errorf("load risk guard state: %w", err)
	}

	return risk.PersistedState{
		StartingDailyEquity: m.StartingDailyEquity,
		CurrentDailyEquity:  m.CurrentDailyEquity,
		OpenPositionsCount:  m.OpenPositionsCount,
		ConsecutiveLosses:   m.ConsecutiveLosses,
		IsCircuitTripped:    m.IsCircuitTripped,
		LastResetDate:       m.LastResetDate,
	}, true, nil
}

func (s *Store) SaveRiskState(state risk.PersistedState) error {
	m := RiskGuardStateModel{
		ID:                  singleRowID,
		StartingDailyEquity: state.StartingDailyEquity,
		CurrentDailyEquity:  state.CurrentDailyEquity,
		OpenPositionsCount:  state.OpenPositionsCount,
		ConsecutiveLosses:   state.ConsecutiveLosses,
		IsCircuitTripped:    state.IsCircuitTripped,
		LastResetDate:       state.LastResetDate,
	}
	if err := s.db.Save(&m).Error; err != nil {
		return fmt.Errorf("save risk guard state: %w", err)
	}
	return nil
}

// --- pipeline.SignalStore ---

var _ pipeline.SignalStore = (*Store)(nil)

func (s *Store) SaveSignal(rec pipeline.SignalRecord) error {
	m := SignalRecordModel{
		Symbol:    rec.Symbol,
		Action:    rec.Action,
		Status:    rec.Status,
		Reason:    rec.Reason,
		Detail:    rec.Detail,
		LotSize:   rec.LotSize,
		Timestamp: rec.Timestamp,
	}
	if err := s.db.Create(&m).Error; err != nil {
		return fmt.Errorf("save signal record: %w", err)
	}
	return nil
}

func (s *Store) RecentSignals(limit int) ([]pipeline.SignalRecord, error) {
	var rows []SignalRecordModel
	if err := s.db.Order("timestamp DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list recent signals: %w", err)
	}

	out := make([]pipeline.SignalRecord, len(rows))
	for i, m := range rows {
		out[i] = pipeline.SignalRecord{
			Symbol:    m.Symbol,
			Action:    m.Action,
			Status:    m.Status,
			Reason:    m.Reason,
			Detail:    m.Detail,
			LotSize:   m.LotSize,
			Timestamp: m.Timestamp,
		}
	}
	return out, nil
}

// --- Account balance ---

// LoadAccountBalance คืน ok=false ถ้ายังไม่เคยบันทึก balance ไว้เลย (ครั้งแรกที่รัน)
func (s *Store) LoadAccountBalance() (float64, bool, error) {
	var m AccountStateModel
	err := s.db.First(&m, singleRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("load account balance: %w", err)
	}
	return m.Balance, true, nil
}

func (s *Store) SaveAccountBalance(balance float64) error {
	m := AccountStateModel{ID: singleRowID, Balance: balance}
	if err := s.db.Save(&m).Error; err != nil {
		return fmt.Errorf("save account balance: %w", err)
	}
	return nil
}

// --- Trade Outcomes / Win-Rate per strategy ---

// TradeOutcome คือผลลัพธ์จริงของ position ที่ปิดแล้ว หลัง attribute กลับไป
// หา strategy ต้นเหตุแล้ว (ผ่าน StrategyTag) — main.go เป็นคนประกอบข้อมูลนี้
// จากการจับคู่ ticket ที่ได้ตอน dispatch order กับ ticket ที่มาใน trade_closed event
type TradeOutcome struct {
	Symbol      string
	StrategyTag string
	Reason      string
	Ticket      uint64
	Profit      float64
	IsWin       bool
	Timestamp   time.Time
}

func (s *Store) SaveTradeOutcome(o TradeOutcome) error {
	m := TradeOutcomeModel{
		Symbol:      o.Symbol,
		StrategyTag: o.StrategyTag,
		Reason:      o.Reason,
		Ticket:      o.Ticket,
		Profit:      o.Profit,
		IsWin:       o.IsWin,
		Timestamp:   o.Timestamp,
	}
	if err := s.db.Create(&m).Error; err != nil {
		return fmt.Errorf("save trade outcome: %w", err)
	}
	return nil
}

// WinRateStat สรุปสถิติแพ้/ชนะสะสมของแต่ละ strategy (แยกตาม StrategyTag)
type WinRateStat struct {
	StrategyTag string  `json:"strategy_tag"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	TotalTrades int     `json:"total_trades"`
	WinRate     float64 `json:"win_rate"` // 0.0 - 1.0
	TotalProfit float64 `json:"total_profit"`
}

// WinRateByStrategy คำนวณ win-rate สะสมของแต่ละ strategy จาก trade_outcomes ทั้งหมด
func (s *Store) WinRateByStrategy() ([]WinRateStat, error) {
	type row struct {
		StrategyTag string
		Wins        int
		Losses      int
		TotalProfit float64
	}

	var rows []row
	err := s.db.Model(&TradeOutcomeModel{}).
		Select("strategy_tag, SUM(CASE WHEN is_win THEN 1 ELSE 0 END) as wins, SUM(CASE WHEN is_win THEN 0 ELSE 1 END) as losses, SUM(profit) as total_profit").
		Group("strategy_tag").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("compute win rate by strategy: %w", err)
	}

	out := make([]WinRateStat, len(rows))
	for i, r := range rows {
		total := r.Wins + r.Losses
		var winRate float64
		if total > 0 {
			winRate = float64(r.Wins) / float64(total)
		}
		out[i] = WinRateStat{
			StrategyTag: r.StrategyTag,
			Wins:        r.Wins,
			Losses:      r.Losses,
			TotalTrades: total,
			WinRate:     winRate,
			TotalProfit: r.TotalProfit,
		}
	}
	return out, nil
}

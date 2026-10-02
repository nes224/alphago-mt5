package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

const singleRowID = 1

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

	var symbolRows []OpenPositionSymbolModel
	if err := s.db.Find(&symbolRows).Error; err != nil {
		return risk.PersistedState{}, false, fmt.Errorf("load open position symbols: %w", err)
	}
	symbols := make(map[string]int, len(symbolRows))
	for _, row := range symbolRows {
		symbols[row.Symbol] = row.Count
	}

	return risk.PersistedState{
		StartingDailyEquity: m.StartingDailyEquity,
		CurrentDailyEquity:  m.CurrentDailyEquity,
		OpenPositionsCount:  m.OpenPositionsCount,
		OpenPositionSymbols: symbols,
		ConsecutiveLosses:   m.ConsecutiveLosses,
		IsCircuitTripped:    m.IsCircuitTripped,
		LastResetDate:       m.LastResetDate,
	}, true, nil
}

func (s *Store) SaveRiskState(state risk.PersistedState) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		m := RiskGuardStateModel{
			ID:                  singleRowID,
			StartingDailyEquity: state.StartingDailyEquity,
			CurrentDailyEquity:  state.CurrentDailyEquity,
			OpenPositionsCount:  state.OpenPositionsCount,
			ConsecutiveLosses:   state.ConsecutiveLosses,
			IsCircuitTripped:    state.IsCircuitTripped,
			LastResetDate:       state.LastResetDate,
		}
		if err := tx.Save(&m).Error; err != nil {
			return fmt.Errorf("save risk guard state: %w", err)
		}

		if err := tx.Where("1 = 1").Delete(&OpenPositionSymbolModel{}).Error; err != nil {
			return fmt.Errorf("clear open position symbols: %w", err)
		}
		for symbol, count := range state.OpenPositionSymbols {
			if count <= 0 {
				continue
			}
			if err := tx.Create(&OpenPositionSymbolModel{Symbol: symbol, Count: count}).Error; err != nil {
				return fmt.Errorf("save open position symbol %s: %w", symbol, err)
			}
		}
		return nil
	})
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

type TradeOutcome struct {
	Symbol      string
	StrategyTag string
	Reason      string
	Ticket      uint64
	Profit      float64
	IsWin       bool
	Session     string // domain.Session* — main.go คำนวณจาก domain.MarketSessionFromUTC(Timestamp) ก่อนส่งมา
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
		Session:     o.Session,
		Timestamp:   o.Timestamp,
	}
	if err := s.db.Create(&m).Error; err != nil {
		return fmt.Errorf("save trade outcome: %w", err)
	}
	return nil
}

func (s *Store) SaveAttribution(ticket uint64, reason string) error {
	m := PendingAttributionModel{Ticket: ticket, Reason: reason, CreatedAt: time.Now()}
	if err := s.db.Create(&m).Error; err != nil {
		return fmt.Errorf("save pending attribution: %w", err)
	}
	return nil
}

func (s *Store) LoadAndDeleteAttribution(ticket uint64) (string, bool, error) {
	var m PendingAttributionModel
	err := s.db.First(&m, ticket).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load pending attribution: %w", err)
	}

	if err := s.db.Delete(&PendingAttributionModel{}, ticket).Error; err != nil {
		return "", false, fmt.Errorf("delete pending attribution: %w", err)
	}
	return m.Reason, true, nil
}

// --- Pending Orders (Outbox pattern) ---

var _ pipeline.OutboxStore = (*Store)(nil)

func (s *Store) CreatePendingOrder(order pipeline.PendingOrder) (uint, error) {
	m := PendingOrderModel{
		Symbol:     order.Symbol,
		Action:     order.Action,
		LotSize:    order.LotSize,
		StopLoss:   order.StopLoss,
		TakeProfit: order.TakeProfit,
		Reason:     order.Reason,
		Status:     PendingOrderStatusPending,
	}
	if err := s.db.Create(&m).Error; err != nil {
		return 0, fmt.Errorf("create pending order: %w", err)
	}
	return m.ID, nil
}

func (s *Store) MarkOrderOutcome(id uint, status string, ticket uint64, errMsg string) error {
	updates := map[string]any{
		"status":        status,
		"ticket":        ticket,
		"error_message": errMsg,
	}
	if err := s.db.Model(&PendingOrderModel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update pending order outcome: %w", err)
	}
	return nil
}

type PendingOrderRecord struct {
	ID           uint      `json:"id"`
	Symbol       string    `json:"symbol"`
	Action       string    `json:"action"`
	LotSize      float64   `json:"lot_size"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	Ticket       uint64    `json:"ticket,omitempty"`
	ErrorMessage string    `json:"error_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Store) PendingOrders(limit int) ([]PendingOrderRecord, error) {
	var rows []PendingOrderModel
	if err := s.db.Order("created_at DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list pending orders: %w", err)
	}

	out := make([]PendingOrderRecord, len(rows))
	for i, m := range rows {
		out[i] = PendingOrderRecord{
			ID:           m.ID,
			Symbol:       m.Symbol,
			Action:       m.Action,
			LotSize:      m.LotSize,
			Reason:       m.Reason,
			Status:       m.Status,
			Ticket:       m.Ticket,
			ErrorMessage: m.ErrorMessage,
			CreatedAt:    m.CreatedAt,
			UpdatedAt:    m.UpdatedAt,
		}
	}
	return out, nil
}

func (s *Store) UnresolvedPendingOrders() ([]PendingOrderRecord, error) {
	var rows []PendingOrderModel
	if err := s.db.Where("status IN ?", []string{PendingOrderStatusPending, PendingOrderStatusUnknown}).
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list unresolved pending orders: %w", err)
	}

	out := make([]PendingOrderRecord, len(rows))
	for i, m := range rows {
		out[i] = PendingOrderRecord{
			ID:           m.ID,
			Symbol:       m.Symbol,
			Action:       m.Action,
			LotSize:      m.LotSize,
			Reason:       m.Reason,
			Status:       m.Status,
			Ticket:       m.Ticket,
			ErrorMessage: m.ErrorMessage,
			CreatedAt:    m.CreatedAt,
			UpdatedAt:    m.UpdatedAt,
		}
	}
	return out, nil
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

// SessionWinRateStat สรุปสถิติแพ้/ชนะสะสมของแต่ละ market session (ASIAN/
// LONDON/LONDON_NY_OVERLAP/NEW_YORK — ดู domain.MarketSessionFromUTC)
type SessionWinRateStat struct {
	Session     string  `json:"session"`
	Wins        int     `json:"wins"`
	Losses      int     `json:"losses"`
	TotalTrades int     `json:"total_trades"`
	WinRate     float64 `json:"win_rate"`
	TotalProfit float64 `json:"total_profit"`
}

func (s *Store) WinRateBySession() ([]SessionWinRateStat, error) {
	type row struct {
		Session     string
		Wins        int
		Losses      int
		TotalProfit float64
	}

	var rows []row
	err := s.db.Model(&TradeOutcomeModel{}).
		Select("session, SUM(CASE WHEN is_win THEN 1 ELSE 0 END) as wins, SUM(CASE WHEN is_win THEN 0 ELSE 1 END) as losses, SUM(profit) as total_profit").
		Group("session").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("compute win rate by session: %w", err)
	}

	out := make([]SessionWinRateStat, len(rows))
	for i, r := range rows {
		total := r.Wins + r.Losses
		var winRate float64
		if total > 0 {
			winRate = float64(r.Wins) / float64(total)
		}
		out[i] = SessionWinRateStat{
			Session:     r.Session,
			Wins:        r.Wins,
			Losses:      r.Losses,
			TotalTrades: total,
			WinRate:     winRate,
			TotalProfit: r.TotalProfit,
		}
	}
	return out, nil
}

var _ services.RiskConfigStore = (*Store)(nil)

func (s *Store) SaveRiskConfig(cfg domain.RiskConfig) error {
	m := RiskConfigModel{
		ID:                   singleRowID,
		RiskPerTradePercent:  cfg.RiskPerTradePercent,
		MinLotSize:           cfg.MinLotSize,
		MaxLotSize:           cfg.MaxLotSize,
		MinSLDistance:        cfg.MinSLDistance,
		MaxSLDistance:        cfg.MaxSLDistance,
		VolatilityMultiplier: cfg.VolatilityMultiplier,
		ATRMultiplier:        cfg.ATRMultiplier,
		UseATRForSizing:      cfg.UseATRForSizing,
		MaxDailyLossPercent:  cfg.MaxDailyLossPercent,
		MaxOpenPositions:     cfg.MaxOpenPositions,
		MaxSpreadPips:        cfg.MaxSpreadPips,
	}
	if err := s.db.Save(&m).Error; err != nil {
		return fmt.Errorf("save risk config: %w", err)
	}
	return nil
}

func (s *Store) LoadRiskConfig() (domain.RiskConfig, bool, error) {
	var m RiskConfigModel
	err := s.db.First(&m, singleRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.RiskConfig{}, false, nil
	}
	if err != nil {
		return domain.RiskConfig{}, false, fmt.Errorf("load risk config: %w", err)
	}

	return domain.RiskConfig{
		RiskPerTradePercent:  m.RiskPerTradePercent,
		MinLotSize:           m.MinLotSize,
		MaxLotSize:           m.MaxLotSize,
		MinSLDistance:        m.MinSLDistance,
		MaxSLDistance:        m.MaxSLDistance,
		VolatilityMultiplier: m.VolatilityMultiplier,
		ATRMultiplier:        m.ATRMultiplier,
		UseATRForSizing:      m.UseATRForSizing,
		MaxDailyLossPercent:  m.MaxDailyLossPercent,
		MaxOpenPositions:     m.MaxOpenPositions,
		MaxSpreadPips:        m.MaxSpreadPips,
	}, true, nil
}

func (s *Store) SaveTickHistoryBatch(ticks []domain.Tick) error {
	if len(ticks) == 0 {
		return nil
	}

	models := make([]TickHistoryModel, len(ticks))
	for i, t := range ticks {
		models[i] = TickHistoryModel{
			Symbol:    t.Symbol,
			Bid:       t.Bid,
			Ask:       t.Ask,
			Volume:    t.Volume,
			Timestamp: t.Timestamp,
		}
	}

	if err := s.db.CreateInBatches(models, 500).Error; err != nil {
		return fmt.Errorf("save tick history batch: %w", err)
	}
	return nil
}

func (s *Store) TickHistorySince(symbol string, since time.Time) ([]domain.Tick, error) {
	var rows []TickHistoryModel
	if err := s.db.Where("symbol = ? AND timestamp >= ?", symbol, since).Order("timestamp ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list tick history since %s: %w", since, err)
	}

	out := make([]domain.Tick, len(rows))
	for i, m := range rows {
		out[i] = domain.Tick{
			Symbol:    m.Symbol,
			Bid:       m.Bid,
			Ask:       m.Ask,
			Volume:    m.Volume,
			Timestamp: m.Timestamp,
		}
	}
	return out, nil
}

func (s *Store) RecentTickHistory(symbol string, limit int) ([]domain.Tick, error) {
	var rows []TickHistoryModel
	if err := s.db.Where("symbol = ?", symbol).Order("timestamp DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list recent tick history: %w", err)
	}

	out := make([]domain.Tick, len(rows))
	for i, m := range rows {
		out[len(rows)-1-i] = domain.Tick{
			Symbol:    m.Symbol,
			Bid:       m.Bid,
			Ask:       m.Ask,
			Volume:    m.Volume,
			Timestamp: m.Timestamp,
		}
	}
	return out, nil
}

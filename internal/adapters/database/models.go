package database

import "time"

type AccountStateModel struct {
	ID        uint `gorm:"primaryKey"`
	Balance   float64
	UpdatedAt time.Time
}

func (AccountStateModel) TableName() string { return "account_state" }

type RiskConfigModel struct {
	ID                   uint `gorm:"primaryKey"`
	RiskPerTradePercent  float64
	MinLotSize           float64
	MaxLotSize           float64
	MinSLDistance        float64
	MaxSLDistance        float64
	VolatilityMultiplier float64
	ATRMultiplier        float64
	UseATRForSizing      bool
	MaxDailyLossPercent  float64
	MaxOpenPositions     int
	MaxSpreadPips        float64
	UpdatedAt            time.Time
}

func (RiskConfigModel) TableName() string { return "risk_config" }

type RiskGuardStateModel struct {
	ID                  uint `gorm:"primaryKey"`
	StartingDailyEquity float64
	CurrentDailyEquity  float64
	OpenPositionsCount  int
	ConsecutiveLosses   int
	IsCircuitTripped    bool
	LastResetDate       string
	UpdatedAt           time.Time
}

func (RiskGuardStateModel) TableName() string { return "risk_guard_state" }

type SignalRecordModel struct {
	ID        uint   `gorm:"primaryKey"`
	Symbol    string `gorm:"index"`
	Action    string
	Status    string `gorm:"index"`
	Reason    string
	Detail    string
	LotSize   float64
	Timestamp time.Time `gorm:"index"`
}

func (SignalRecordModel) TableName() string { return "signal_records" }

type TradeOutcomeModel struct {
	ID          uint   `gorm:"primaryKey"`
	Symbol      string `gorm:"index"`
	StrategyTag string `gorm:"index"` // ส่วนก่อน " (" ของ signal Reason เช่น "VOLUME_EXPANSION_BUY"
	Reason      string // Reason เต็มตอนที่ signal ถูกยิงออกไป (มี context เพิ่ม เช่น Z-Score, SweptLevel)
	Ticket      uint64 `gorm:"index"`
	Profit      float64
	IsWin       bool
	Session     string `gorm:"index"` // domain.Session* — คำนวณจาก Timestamp ตอนบันทึก (ดู domain.MarketSessionFromUTC)
	Timestamp   time.Time
}

func (TradeOutcomeModel) TableName() string { return "trade_outcomes" }

type OpenPositionSymbolModel struct {
	Symbol string `gorm:"primaryKey"`
	Count  int
}

func (OpenPositionSymbolModel) TableName() string { return "open_position_symbols" }

type PendingAttributionModel struct {
	Ticket    uint64 `gorm:"primaryKey"`
	Reason    string
	CreatedAt time.Time
}

func (PendingAttributionModel) TableName() string { return "pending_attributions" }

const (
	PendingOrderStatusPending = "PENDING"
	PendingOrderStatusSent    = "SENT"
	PendingOrderStatusFailed  = "FAILED"
	PendingOrderStatusUnknown = "UNKNOWN"
)

type PendingOrderModel struct {
	ID           uint   `gorm:"primaryKey"`
	Symbol       string `gorm:"index"`
	Action       string
	LotSize      float64
	StopLoss     float64
	TakeProfit   float64
	Reason       string
	Status       string `gorm:"index"`
	Ticket       uint64
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (PendingOrderModel) TableName() string { return "pending_orders" }

type TickHistoryModel struct {
	ID        uint   `gorm:"primaryKey"`
	Symbol    string `gorm:"index:idx_tick_history_symbol_time"`
	Bid       float64
	Ask       float64
	Volume    int64
	Timestamp time.Time `gorm:"index:idx_tick_history_symbol_time"`
}

func (TickHistoryModel) TableName() string { return "tick_history" }

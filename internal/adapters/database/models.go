package database

import "time"

// AccountStateModel เก็บ balance ปัจจุบันของบัญชี — มีแค่แถวเดียวเสมอ (ID=1)
// อัปเดตทุกครั้งที่ equity เปลี่ยน แทนที่การแก้ ACCOUNT_BALANCE ใน app.env มือ
type AccountStateModel struct {
	ID                     uint `gorm:"primaryKey"`
	Balance                float64
	RISK_PER_TRADE_PERCENT float64
	MIN_LOT_SIZE           float64
	MAX_LOT_SIZE           float64
	MIN_SL_DISTANCE        float64
	MAX_SL_DISTANCE        float64
	MAX_DAILY_LOSS_PERCENT float64
	MAX_OPEN_POSITIONS     float64
	MAX_SPREAD_PIPS        float64
	UpdatedAt              time.Time
}

func (AccountStateModel) TableName() string { return "account_state" }

// RiskGuardStateModel เก็บสถานะ RiskGuard ให้รอดจาก restart — มีแค่แถวเดียวเสมอ (ID=1)
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

// SignalRecordModel เก็บประวัติ signal/order ทุกตัวที่ ExecutionRouter ประมวลผล
// ถาวร แทนที่ in-memory ring buffer เดิมที่หายตอน restart
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

// TradeOutcomeModel เก็บผลลัพธ์จริง (กำไร/ขาดทุน) ของแต่ละ position ที่ปิดแล้ว
// พร้อม attribute กลับไปหา strategy ที่เป็นต้นเหตุ (ผ่าน StrategyTag ที่ตัดมา
// จาก signal Reason เช่น "VOLUME_EXPANSION_BUY") ใช้คำนวณ win-rate ต่อ strategy
type TradeOutcomeModel struct {
	ID          uint   `gorm:"primaryKey"`
	Symbol      string `gorm:"index"`
	StrategyTag string `gorm:"index"` // ส่วนก่อน " (" ของ signal Reason เช่น "VOLUME_EXPANSION_BUY"
	Reason      string // Reason เต็มตอนที่ signal ถูกยิงออกไป (มี context เพิ่ม เช่น Z-Score, SweptLevel)
	Ticket      uint64 `gorm:"index"`
	Profit      float64
	IsWin       bool
	Timestamp   time.Time
}

func (TradeOutcomeModel) TableName() string { return "trade_outcomes" }

// OpenPositionSymbolModel เก็บรายชื่อ symbol ที่ RiskGuard track ว่ามี position
// เปิดอยู่ตอนนี้ (แทนที่ openPositionSymbols map ที่เคยอยู่แค่ใน RAM) — ให้รอด
// จาก restart แทนที่จะลืมว่ามี position เปิดค้างอยู่ที่ symbol ไหน
type OpenPositionSymbolModel struct {
	Symbol string `gorm:"primaryKey"`
	Count  int
}

func (OpenPositionSymbolModel) TableName() string { return "open_position_symbols" }

// PendingAttributionModel เก็บ ticket -> signal Reason ระหว่างรอ trade_closed
// event กลับมา (แทนที่ ticketToReason sync.Map เดิมใน main.go ที่หายตอน restart
// ทำให้ position ที่เปิดค้างอยู่ตอน restart attribute win/loss กลับไม่ได้)
type PendingAttributionModel struct {
	Ticket    uint64 `gorm:"primaryKey"`
	Reason    string
	CreatedAt time.Time
}

func (PendingAttributionModel) TableName() string { return "pending_attributions" }

// สถานะของ PendingOrderModel — PENDING เขียนก่อนส่งจริง, แล้วอัปเดตเป็น SENT
// (มี Ticket) หรือ FAILED (broker ปฏิเสธชัดเจน รู้แน่ว่าไม่เปิด position) หรือ
// UNKNOWN (error จาก SendOrder เอง เช่น timeout/EOF — ไม่รู้ว่าเข้าตลาดจริงไหม
// ต้องเช็คมือ ไม่ auto-resend เพราะเสี่ยงเปิดซ้อน)
const (
	PendingOrderStatusPending = "PENDING"
	PendingOrderStatusSent    = "SENT"
	PendingOrderStatusFailed  = "FAILED"
	PendingOrderStatusUnknown = "UNKNOWN"
)

// PendingOrderModel คือ Outbox แถวหนึ่งต่อ order ที่ ExecutionRouter ตัดสินใจจะ
// ส่ง — เขียนไว้ก่อนจริงๆ จะยิงเข้า MT5 (ผ่าน orderSink channel) เพื่อไม่ให้
// order ที่ "บันทึกว่า DISPATCHED แล้ว" แต่ยังไม่ทันส่งจริงหายไปเงียบๆ ถ้า
// service crash ระหว่างสองจุดนี้
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

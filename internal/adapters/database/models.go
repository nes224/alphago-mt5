package database

import "time"

// AccountStateModel เก็บ balance ปัจจุบันของบัญชี — มีแค่แถวเดียวเสมอ (ID=1)
// อัปเดตทุกครั้งที่ equity เปลี่ยน แทนที่การแก้ ACCOUNT_BALANCE ใน app.env มือ
//
// ตั้งใจเก็บแค่ Balance เท่านั้น — ห้ามเพิ่ม field อื่นเข้ามาในตารางนี้ เพราะ
// SaveAccountBalance() เขียนทับทั้งแถวทุกครั้งที่ trade ปิด (ทุกไม่กี่นาที) ถ้า
// มี field อื่นอยู่ด้วยจะโดนเขียนทับกลับเป็นค่าว่างโดยไม่ตั้งใจ — risk policy
// ที่ผู้ใช้ตั้งเอง (risk %, lot limits, ฯลฯ) แยกเก็บที่ RiskConfigModel แทน
type AccountStateModel struct {
	ID        uint `gorm:"primaryKey"`
	Balance   float64
	UpdatedAt time.Time
}

func (AccountStateModel) TableName() string { return "account_state" }

// RiskConfigModel เก็บ risk policy ที่ผู้ใช้ตั้งเอง (ไม่ใช่ข้อเท็จจริงของบัญชี
// ดึงจาก MT5 ไม่ได้) — มีแค่แถวเดียวเสมอ (ID=1) ตั้งผ่าน PUT /api/v1/risk/config
// แทนการแก้ app.env + restart service
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
	Session     string `gorm:"index"` // domain.Session* — คำนวณจาก Timestamp ตอนบันทึก (ดู domain.MarketSessionFromUTC)
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

// TickHistoryModel เก็บ tick ดิบทุกตัวที่ไหลเข้าระบบ (เพิ่มจากบทสนทนา
// 2026-10-01 — ดู ROADMAP.md "Tick History Recording") — รากฐานสำหรับคำนวณ
// Order Flow/POC/ATR ย้อนหลังและทำ backtest ในอนาคต เขียนผ่าน background
// goroutine แยก (batch insert) ไม่ได้เขียนทุก tick ทีละแถว กันกระทบ critical
// path ของการเทรดจริง — ตารางนี้จะโตเร็วมาก (หลัก GB/เดือน) ยังไม่มี retention
// policy ตัดข้อมูลเก่าทิ้ง (รู้อยู่แล้ว ยังไม่ทำ เป็น follow-up)
type TickHistoryModel struct {
	ID        uint   `gorm:"primaryKey"`
	Symbol    string `gorm:"index:idx_tick_history_symbol_time"`
	Bid       float64
	Ask       float64
	Volume    int64
	Timestamp time.Time `gorm:"index:idx_tick_history_symbol_time"`
}

func (TickHistoryModel) TableName() string { return "tick_history" }

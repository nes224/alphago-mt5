package database

import "time"

// AccountStateModel เก็บ balance ปัจจุบันของบัญชี — มีแค่แถวเดียวเสมอ (ID=1)
// อัปเดตทุกครั้งที่ equity เปลี่ยน แทนที่การแก้ ACCOUNT_BALANCE ใน app.env มือ
type AccountStateModel struct {
	ID        uint `gorm:"primaryKey"`
	Balance   float64
	UpdatedAt time.Time
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

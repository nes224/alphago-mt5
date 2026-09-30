package domain

import "time"

// TradeClosedEvent มาจาก EA ผ่าน OnTradeTransaction() ตอน position ปิดจริง
// (ไม่ว่าจะโดน SL/TP หรือปิดมือ) ใช้ feed ผลแพ้/ชนะจริงกลับเข้า RiskGuard และ
// อัปเดต balance ตาม P/L จริง แทนที่จะพึ่งค่า mock/ประมาณ
type TradeClosedEvent struct {
	Symbol    string
	Ticket    uint64
	Profit    float64
	Timestamp time.Time
}

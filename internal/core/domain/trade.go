package domain

import "time"

type TradeAction string

const (
	ActionBuy    TradeAction = "BUY"
	ActionSell   TradeAction = "SELL"
	ActionClose  TradeAction = "CLOSE"
	ActionModify TradeAction = "MODIFY"
)

type TradeRequest struct {
	Action   TradeAction `json:"action"` // BUY, SELL, CLOSE, MODIFY
	Symbol   string      `json:"symbol"`
	Volume   float64     `json:"volume"`
	Ticket   uint64      `json:"ticket,omitempty"` // จำเป็นสำหรับ CLOSE / MODIFY
	SL       float64     `json:"sl"`               // Stop Loss Price
	TP       float64     `json:"tp"`               // Take Profit Price
	MagicNum int64       `json:"magic_num,omitempty"`
}

type TradeResponse struct {
	Status    string    `json:"status"` // "SUCCESS" หรือ "FAILED"
	Ticket    uint64    `json:"ticket"`
	Message   string    `json:"message"`
	Price     float64   `json:"executed_price"`
	Timestamp time.Time `json:"timestamp"`
}

func (r *TradeResponse) IsSuccess() bool {
	return r.Status == "SUCCESS"
}

// AccountInfo คือข้อมูลบัญชีสดที่ดึงจาก MT5 ตรงๆ (ผ่าน command "ACCOUNT_INFO")
// แทนที่จะอ่านค่า balance จาก app.env — AccountType มีค่าเป็น "DEMO"/"REAL"/
// "CONTEST" กันเทรดผิดบัญชีโดยไม่รู้ตัว, Symbols คือ symbol ที่อยู่ใน Market
// Watch ของ MT5 ตอนนี้ (เทรดได้จริง ไม่ใช่ symbol list ทั้งหมดที่ broker มี)
type AccountInfo struct {
	Status      string   `json:"status"`
	Balance     float64  `json:"balance"`
	Equity      float64  `json:"equity"`
	Currency    string   `json:"currency"`
	Leverage    int64    `json:"leverage"`
	AccountType string   `json:"account_type"`
	Broker      string   `json:"broker"`
	Login       int64    `json:"login"`
	Symbols     []string `json:"symbols"`
}

func (a *AccountInfo) IsSuccess() bool {
	return a.Status == "SUCCESS"
}

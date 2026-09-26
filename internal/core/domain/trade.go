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

package domain

import "time"

type TradeRequest struct {
	Action   string  `json:"action"`
	Symbol   string  `json:"symbol"`
	Volume   float64 `json:"volume"`
	SL       float64 `json:"sl"`
	TP       float64 `json:"tp"`
	MagicNum int64   `json:"magic_num"`
}

type TradeResponse struct {
	Success   bool      `json:"success"`
	Ticket    int64     `json:"ticket,omitempty"`
	Price     float64   `json:"price,omitempty"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}


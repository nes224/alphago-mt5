package domain

import "time"

type TradeClosedEvent struct {
	Symbol    string
	Ticket    uint64
	Profit    float64
	Timestamp time.Time
}

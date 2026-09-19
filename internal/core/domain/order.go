package domain

type OrderType string

const (
	OrderTypeBuy  OrderType = "BUY"
	OrderTypeSell OrderType = "SELL"
)

type TradeOrder struct {
	Symbol string    `json:"symbol"`
	Type   OrderType `json:"type"`
	Volume float64   `json:"volume"`
}

type OrderResult struct {
	Status  string `json:"status"`
	Ticket  int64  `json:"ticket,omitempty"`
	Message string `json:"message"`
}


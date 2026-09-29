package domain

import "time"

type TickMetrics struct {
	Symbol        string    `json:"symbol"`
	Bid           float64   `json:"bid"`
	Ask           float64   `json:"ask"`
	Spread        float64   `json:"spread"`
	PriceVelocity float64   `json:"price_velocity"`
	Timestamp     time.Time `json:"timestamp"`
}

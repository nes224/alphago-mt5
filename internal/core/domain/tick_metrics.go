package domain

import "time"

type TickMetrics struct {
	Symbol        string    `json:"symbol"`
	Bid           float64   `json:"bid"`
	Ask           float64   `json:"ask"`
	Spread        float64   `json:"spread"`
	PriceVelocity float64   `json:"price_velocity"`
	SpreadDelta   float64   `json:"spread_delta"`
	OIDelta       int64     `json:"oi_delta"`
	OpenInterest  int64     `json:"open_interest"`
	Mean          float64   `json:"mean"`
	StdDev        float64   `json:"std_dev"`
	ZScore        float64   `json:"z_score"`
	Timestamp     time.Time `json:"timestamp"`
}

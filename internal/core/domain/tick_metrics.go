package domain

import "time"

type TickMetrics struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	ZScore float64 `json:"z_score"`

	Bid         float64 `json:"bid"`
	Ask         float64 `json:"ask"`
	Spread      float64 `json:"spread"`
	SpreadDelta float64 `json:"spread_delta"`

	OpenInterest  int64   `json:"open_interest"`
	OIDelta       int64   `json:"oi_delta"` // ผลต่าง OI เทียบกับ Tick ก่อนหน้า (OI_t - OI_{t-1})
	OIVelocity    float64 `json:"oi_velocity"` // อัตราการเปลี่ยนแปลง OI ต่อวินาที (Delta OI / Delta Time)
	PriceVelocity float64 `json:"price_velocity"` // อัตราการเปลี่ยนแปลงราคาต่อวินาที (Delta Price / Delta Time)

	WindowSize int       `json:"window_size"`
	Timestamp  time.Time `json:"timestamp"`
}

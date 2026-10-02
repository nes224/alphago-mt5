package domain

import "time"

type TickMetrics struct {
	Symbol             string  `json:"symbol"`
	Price              float64 `json:"price"`
	Mean               float64 `json:"mean"`
	StdDev             float64 `json:"std_dev"`
	ZScore             float64 `json:"z_score"`
	TrendSlope         float64 `json:"trend_slope"`           // Least-squares slope ของราคาใน Rolling Window สั้น (WindowSize tick): |ชัน| สูง = กำลัง Trend, ใกล้ 0 = Sideway
	LongTermTrendSlope float64 `json:"long_term_trend_slope"` // Slope เดียวกันแต่คำนวณจาก Window ยาวกว่ามาก (proxy "higher timeframe") ใช้กรองทิศทางให้เข้าทางเดียวกับ trend ใหญ่

	Bid         float64 `json:"bid"`
	Ask         float64 `json:"ask"`
	Spread      float64 `json:"spread"`
	SpreadDelta float64 `json:"spread_delta"`

	DailyOpen float64 `json:"daily_open"`
	DailyHigh float64 `json:"daily_high"`
	DailyLow  float64 `json:"daily_low"`

	OpenInterest         int64   `json:"open_interest"`
	OIDelta              int64   `json:"oi_delta"`
	OIVelocity           float64 `json:"oi_velocity"`
	VolumeDelta          int64   `json:"volume_delta"`
	VolumeVelocity       float64 `json:"volume_velocity"`
	PriceVelocity        float64 `json:"price_velocity"`
	VelocityTrendSlope   float64 `json:"velocity_trend_slope"`
	VolatilityTrendSlope float64 `json:"volatility_trend_slope"`
	SizingVolatility     float64 `json:"sizing_volatility"`
	ATR                  float64 `json:"atr"`
	ATRReady             bool    `json:"atr_ready"`
	CVD                  float64 `json:"cvd"`
	CVDReady             bool    `json:"cvd_ready"`
	CVDTrendSlope        float64 `json:"cvd_trend_slope"`
	POCPrice             float64 `json:"poc_price"`
	POCVolume            float64 `json:"poc_volume"`
	POCReady             bool    `json:"poc_ready"`
	DistanceToPOC        float64 `json:"distance_to_poc"`
	VolumeAtCurrentPrice float64 `json:"volume_at_current_price"`

	WindowSize int       `json:"window_size"`
	Timestamp  time.Time `json:"timestamp"`
}

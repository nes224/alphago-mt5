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

	DailyOpen float64 `json:"daily_open"` // ราคาเปิดของวัน (mid price ของ tick แรกที่เห็นวันนั้น) รีเซ็ตตามวันที่ปฏิทิน
	DailyHigh float64 `json:"daily_high"`
	DailyLow  float64 `json:"daily_low"`

	OpenInterest   int64   `json:"open_interest"`
	OIDelta        int64   `json:"oi_delta"`        // ผลต่าง OI เทียบกับ Tick ก่อนหน้า (OI_t - OI_{t-1}) — โบรกเกอร์ CFD ส่วนใหญ่ (รวม Exness) ไม่ส่งค่านี้มาจริง จะเป็น 0 เสมอ
	OIVelocity     float64 `json:"oi_velocity"`     // อัตราการเปลี่ยนแปลง OI ต่อวินาที (Delta OI / Delta Time)
	VolumeDelta    int64   `json:"volume_delta"`    // ผลต่าง Volume เทียบกับ Tick ก่อนหน้า — ใช้แทน OIDelta สำหรับ CFD ที่ไม่มี OI จริง
	VolumeVelocity float64 `json:"volume_velocity"` // อัตราการเปลี่ยนแปลง Volume ต่อวินาที (Delta Volume / Delta Time)
	PriceVelocity  float64 `json:"price_velocity"`  // อัตราการเปลี่ยนแปลงราคาต่อวินาที (Delta Price / Delta Time)

	WindowSize int       `json:"window_size"`
	Timestamp  time.Time `json:"timestamp"`
}

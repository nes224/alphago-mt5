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

	// Reversal Detection (leading indicators, เพิ่มจากบทสนทนา 2026-09-30) — ทั้ง
	// คู่เป็นแค่ metric ให้ดู ยังไม่ได้เอาไปเป็นเงื่อนไขใน strategy ไหนโดยตรง
	// (เหมือน DailyOpen/High/Low ตอนเพิ่มเข้ามาครั้งแรก)
	VelocityTrendSlope   float64 `json:"velocity_trend_slope"`   // Slope ของ PriceVelocity เอง (อนุพันธ์อันดับ 2) ใน Rolling Window สั้น — ลบต่อเนื่องขณะ PriceVelocity ยังเป็นบวก = Momentum Deceleration (เตือนก่อน TrendSlope จะกลับเครื่องหมายจริง)
	VolatilityTrendSlope float64 `json:"volatility_trend_slope"` // Slope ของ StdDev เอง ใน Rolling Window สั้น — ลบต่อเนื่อง = Volatility Contraction (StdDev หดตัว คล้าย Bollinger Band Squeeze) มักเกิดก่อนกลับตัว/ระเบิดทิศทางแรงๆ

	// SizingVolatility คือ StdDev ของราคาใน M15 TimeWindow (เพิ่มจากบทสนทนา
	// 2026-10-01 — ดู ROADMAP.md "Volatility-Adaptive Position Sizing") ใช้แทน
	// StdDev (window 20 tick ด้านบน) สำหรับคำนวณ SL/TP ใน RiskManager —
	// window 20 tick สั้นเกินไปจนชนพื้น MinSLDistance เกือบตลอด ไม่สะท้อน
	// ความผันผวนจริงของตลาด
	SizingVolatility float64 `json:"sizing_volatility"`

	// ATR คือ True Range แบบ Wilder-smoothed จาก synthetic M5 period (14
	// period, ดู ROADMAP.md "ATR แทน StdDev(M15)") — ยังไม่ได้ใช้แทน
	// SizingVolatility เป็น default (ดู RiskConfig.UseATRForSizing) จนกว่าจะ
	// เปิดเองหลังดูค่าจริงจาก endpoint นี้สักพัก ATRReady เป็น false จนกว่าจะมี
	// 14 period ครบ (~70 นาทีหลัง restart) — ห้ามเชื่อ ATR ตอน ATRReady=false
	ATR      float64 `json:"atr"`
	ATRReady bool    `json:"atr_ready"`

	// CVD/POC (Order Flow + Liquidity confluence, 2026-10) — คำนวณและโชว์เสมอ
	// (เห็นได้ผ่าน /api/v1/status) แต่ยังไม่ได้ใช้ gate signal ไหนโดยตรง (ดู
	// LiquidityConfluenceFilter — ปิดอยู่โดย default จนกว่าจะดูค่าจริงพวกนี้
	// สักพักก่อน) ห้ามเชื่อค่าพวกนี้ตอน CVDReady/POCReady เป็น false
	CVD           float64 `json:"cvd"`             // Cumulative Volume Delta แบบ rolling 15 นาที (+แรงซื้อ / -แรงขาย)
	CVDReady      bool    `json:"cvd_ready"`       // true เมื่อ span ของ rolling CVD window ครอบคลุม duration พอแล้ว
	CVDTrendSlope float64 `json:"cvd_trend_slope"` // slope ของค่า CVD เอง (ไม่ใช่ราคา) — order flow กำลังเร่ง/ชะลอ/กลับทิศ

	POCPrice             float64 `json:"poc_price"`               // ราคากึ่งกลางของ bucket ที่มี volume มากสุด ใน rolling Volume Profile 60 นาที
	POCVolume            float64 `json:"poc_volume"`              // volume ของ bucket นั้น
	POCReady             bool    `json:"poc_ready"`               // true เมื่อ Volume Profile warm up แล้วและมีอย่างน้อย 1 bucket
	DistanceToPOC        float64 `json:"distance_to_poc"`         // Price - POCPrice (บวก = ราคาอยู่เหนือ POC)
	VolumeAtCurrentPrice float64 `json:"volume_at_current_price"` // volume สะสมปัจจุบันใน bucket ที่ราคาปัจจุบันอยู่

	WindowSize int       `json:"window_size"`
	Timestamp  time.Time `json:"timestamp"`
}

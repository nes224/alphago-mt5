package domain

// RiskConfig คือ risk policy ที่ผู้ใช้ตั้งเอง ไม่ใช่ข้อเท็จจริงของบัญชี MT5
// (ดึงจาก MT5 ไม่ได้ ต่างจาก AccountInfo — MT5 ไม่รู้ว่าผู้ใช้อยากเสี่ยงแค่ไหน)
// ตั้งค่าได้ผ่าน PUT /api/v1/risk/config แทนการแก้ app.env + restart service
type RiskConfig struct {
	RiskPerTradePercent float64 `json:"risk_per_trade_percent"`
	MinLotSize          float64 `json:"min_lot_size"`
	MaxLotSize          float64 `json:"max_lot_size"`

	// MinSLDistance/MaxSLDistance ไม่ใช่ตัวขับหลักของ SL/TP อีกต่อไป (เดิมเป็น
	// ตัวขับหลักจน MinSLDistance ชนพื้นเกือบตลอดเวลาเพราะตั้งค่าไว้ตอนตลาด
	// ผันผวนน้อยกว่านี้มาก — ดู ROADMAP.md "Volatility-Adaptive Position
	// Sizing") ตอนนี้เป็นแค่ "ราวกันตก" สองข้างของ slDistance ที่คำนวณจาก
	// VolatilityMultiplier × SizingVolatility แทน
	MinSLDistance float64 `json:"min_sl_distance"`
	MaxSLDistance float64 `json:"max_sl_distance"`

	// VolatilityMultiplier คือตัวคูณ SizingVolatility (StdDev ของ M15 TimeWindow
	// ใน TickMetrics) เพื่อได้ SL distance — ยิ่งสูง SL ยิ่งกว้าง โดนเขี่ยยาก
	// ขึ้นแต่ lot เล็กลงตาม (ความเสี่ยงเป็นเงินยังคงที่ตาม RiskPerTradePercent)
	VolatilityMultiplier float64 `json:"volatility_multiplier"`

	// ATRMultiplier คือตัวคูณ ATR (M5×14, แทน SizingVolatility StdDev — ดู
	// ROADMAP.md "ATR แทน StdDev(M15)") เพื่อได้ SL distance — แยกจาก
	// VolatilityMultiplier เพราะ ATR กับ StdDev คนละ scale กัน ตัวคูณเดียวกัน
	// ใช้แทนกันไม่ได้
	ATRMultiplier float64 `json:"atr_multiplier"`

	// UseATRForSizing เปิดใช้ ATR เป็นตัวขับหลักของ SL/TP แทน SizingVolatility
	// — default false (ปิด) โดยตั้งใจ แม้ ATR จะ ready แล้วก็ตาม (ดู
	// domain.TickMetrics.ATRReady) ให้ผู้ใช้เปิดเองผ่าน PUT /api/v1/risk/config
	// หลังจากดูค่า ATR จริงใน /api/v1/status สักพักก่อน ไม่ใช่สลับพฤติกรรม
	// sizing อัตโนมัติตอน ATR warm up เสร็จเงียบๆ
	UseATRForSizing bool `json:"use_atr_for_sizing"`

	MaxDailyLossPercent float64 `json:"max_daily_loss_percent"`
	MaxOpenPositions    int     `json:"max_open_positions"`
	MaxSpreadPips       float64 `json:"max_spread_pips"`
}

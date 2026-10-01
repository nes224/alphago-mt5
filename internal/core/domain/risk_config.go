package domain

// RiskConfig คือ risk policy ที่ผู้ใช้ตั้งเอง ไม่ใช่ข้อเท็จจริงของบัญชี MT5
// (ดึงจาก MT5 ไม่ได้ ต่างจาก AccountInfo — MT5 ไม่รู้ว่าผู้ใช้อยากเสี่ยงแค่ไหน)
// ตั้งค่าได้ผ่าน PUT /api/v1/risk/config แทนการแก้ app.env + restart service
type RiskConfig struct {
	RiskPerTradePercent float64 `json:"risk_per_trade_percent"`
	MinLotSize          float64 `json:"min_lot_size"`
	MaxLotSize          float64 `json:"max_lot_size"`
	MinSLDistance       float64 `json:"min_sl_distance"`
	MaxSLDistance       float64 `json:"max_sl_distance"`
	MaxDailyLossPercent float64 `json:"max_daily_loss_percent"`
	MaxOpenPositions    int     `json:"max_open_positions"`
	MaxSpreadPips       float64 `json:"max_spread_pips"`
}

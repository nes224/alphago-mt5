package domain

type RiskConfig struct {
	RiskPerTradePercent float64 `json:"risk_per_trade_percent"`
	MinLotSize          float64 `json:"min_lot_size"`
	MaxLotSize          float64 `json:"max_lot_size"`

	MinSLDistance float64 `json:"min_sl_distance"`
	MaxSLDistance float64 `json:"max_sl_distance"`

	VolatilityMultiplier float64 `json:"volatility_multiplier"`

	ATRMultiplier   float64 `json:"atr_multiplier"`
	UseATRForSizing bool    `json:"use_atr_for_sizing"`

	MaxDailyLossPercent float64 `json:"max_daily_loss_percent"`
	MaxOpenPositions    int     `json:"max_open_positions"`
	MaxSpreadPips       float64 `json:"max_spread_pips"`
}

package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv            string `mapstructure:"APP_ENV"`
	MT5Host           string `mapstructure:"MT5_HOST"`
	MT5Port           int    `mapstructure:"MT5_PORT"`        // Order command socket (SendOrder/CloseOrder/...)
	MT5StreamPort     int    `mapstructure:"MT5_STREAM_PORT"` // Live tick/price stream socket (separate listener on the EA side)
	MT5TimeoutSeconds int    `mapstructure:"MT5_TIMEOUT_SECONDS"`

	// DatabaseURL เชื่อม PostgreSQL สำหรับ persist account balance, risk guard
	// state และ signal history ข้าม restart (internal/adapters/database)
	DatabaseURL string `mapstructure:"DATABASE_URL"`

	// Risk & Position Sizing — ACCOUNT_BALANCE ใน app.env เป็นแค่ "seed" สำหรับ
	// รันครั้งแรกเท่านั้น หลังจากนั้นระบบจะอ่าน/เขียน balance จริงจาก Postgres
	// (ตาราง account_state) แทน ค่าอื่นๆ เป็น safety limit ป้องกันไม้ใหญ่/เสี่ยงเกินไป
	AccountBalance      float64 `mapstructure:"ACCOUNT_BALANCE"`
	RiskPerTradePercent float64 `mapstructure:"RISK_PER_TRADE_PERCENT"`
	MinLotSize          float64 `mapstructure:"MIN_LOT_SIZE"`
	MaxLotSize          float64 `mapstructure:"MAX_LOT_SIZE"`

	// MinSLDistance/MaxSLDistance ไม่ใช่ตัวขับหลักของ SL/TP อีกต่อไปตั้งแต่
	// Volatility-Adaptive Position Sizing (2026-10-01 — ดู ROADMAP.md) — ตอนนี้
	// เป็นแค่ราวกันตกสองข้างของ VolatilityMultiplier × SizingVolatility ค่าที่
	// เคยตั้งไว้ (3.0/20.0) คำนวณไว้ตอนตลาดผันผวนน้อยกว่านี้มาก ถ้าตั้งไว้แคบ
	// เกินไปจะไปจำกัด SL ที่คำนวณจาก volatility จริงไม่ให้กว้างได้เท่าที่ควร —
	// ปรับผ่าน PUT /api/v1/risk/config ได้โดยไม่ต้องแก้ไฟล์นี้/restart
	MinSLDistance float64 `mapstructure:"MIN_SL_DISTANCE"`
	MaxSLDistance float64 `mapstructure:"MAX_SL_DISTANCE"`

	// VolatilityMultiplier คือตัวคูณ SizingVolatility (StdDev ของ M15) เพื่อได้
	// SL distance — ค่าเริ่มต้น 2.0 ยังไม่ผ่าน backtest จริง เป็นค่าประมาณ
	VolatilityMultiplier float64 `mapstructure:"VOLATILITY_MULTIPLIER"`

	// ATRMultiplier คือตัวคูณ ATR (M5×14) แทน VolatilityMultiplier ตอน
	// UseATRForSizing=true — คนละ scale กับ StdDev จึงต้องมีค่า default ของตัว
	// เอง ยังไม่ผ่าน backtest จริง เป็นค่าประมาณ
	ATRMultiplier   float64 `mapstructure:"ATR_MULTIPLIER"`
	UseATRForSizing bool    `mapstructure:"USE_ATR_FOR_SIZING"`

	MaxDailyLossPercent float64 `mapstructure:"MAX_DAILY_LOSS_PERCENT"`
	MaxOpenPositions    int     `mapstructure:"MAX_OPEN_POSITIONS"`
	MaxSpreadPips       float64 `mapstructure:"MAX_SPREAD_PIPS"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

	viper.SetDefault("MT5_STREAM_PORT", 5556)
	viper.SetDefault("DATABASE_URL", "host=localhost user=alphago password=alphago dbname=alphago port=5434 sslmode=disable")
	viper.SetDefault("ACCOUNT_BALANCE", 1000.0)
	viper.SetDefault("RISK_PER_TRADE_PERCENT", 0.01)
	viper.SetDefault("MIN_LOT_SIZE", 0.01)
	viper.SetDefault("MAX_LOT_SIZE", 0.05)
	viper.SetDefault("MIN_SL_DISTANCE", 3.0)
	viper.SetDefault("MAX_SL_DISTANCE", 20.0)
	viper.SetDefault("VOLATILITY_MULTIPLIER", 2.0)
	viper.SetDefault("ATR_MULTIPLIER", 1.75)
	viper.SetDefault("USE_ATR_FOR_SIZING", false)
	viper.SetDefault("MAX_DAILY_LOSS_PERCENT", 0.03)
	viper.SetDefault("MAX_OPEN_POSITIONS", 5)
	viper.SetDefault("MAX_SPREAD_PIPS", 5.0)
	viper.AutomaticEnv()

	err = viper.ReadInConfig()
	if err != nil {
		return Config{}, fmt.Errorf("failed to read to config: %w", err)
	}

	err = viper.Unmarshal(&config)
	if err != nil {
		return Config{}, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return config, nil
}

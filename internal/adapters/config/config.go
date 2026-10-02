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

	DatabaseURL         string  `mapstructure:"DATABASE_URL"`
	AccountBalance      float64 `mapstructure:"ACCOUNT_BALANCE"`
	RiskPerTradePercent float64 `mapstructure:"RISK_PER_TRADE_PERCENT"`
	MinLotSize          float64 `mapstructure:"MIN_LOT_SIZE"`
	MaxLotSize          float64 `mapstructure:"MAX_LOT_SIZE"`

	MinSLDistance        float64 `mapstructure:"MIN_SL_DISTANCE"`
	MaxSLDistance        float64 `mapstructure:"MAX_SL_DISTANCE"`
	VolatilityMultiplier float64 `mapstructure:"VOLATILITY_MULTIPLIER"`
	ATRMultiplier        float64 `mapstructure:"ATR_MULTIPLIER"`
	UseATRForSizing      bool    `mapstructure:"USE_ATR_FOR_SIZING"`

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

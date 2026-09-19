package config

import (
	"fmt"

	"github.com/spf13/viper"
)

type Config struct {
	AppEnv            string `mapstructure:"APP_ENV"`
	MT5Host           string `mapstructure:"MT5_HOST"`
	MT5Port           int    `mapstructure:"MT5_PORT"`
	MT5TimeoutSeconds int    `mapstructure:"MT5_TIMEOUT_SECONDS"`
}

func LoadConfig(path string) (config Config, err error) {
	viper.AddConfigPath(path)
	viper.SetConfigName("app")
	viper.SetConfigType("env")

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

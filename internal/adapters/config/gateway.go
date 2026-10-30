package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type GateWay struct {
	MT5Host       string
	MT5StreamPort int
	MT5CmdPort    int
	MT5Timeout    time.Duration
	HTTPPort      int
	LogLevel      string

	Symbols          []string
	CandleDataDir    string
	BackfillLookback map[domain.Timeframe]time.Duration
}

func (g GateWay) StreamAddr() string {
	return net.JoinHostPort(g.MT5Host, strconv.Itoa(g.MT5StreamPort))
}

func (g GateWay) CmdAddr() string {
	return net.JoinHostPort(g.MT5Host, strconv.Itoa(g.MT5CmdPort))
}

func LoadGateway() (GateWay, error) {
	var err error
	g := GateWay{
		MT5Host:  envStr("MT5_HOST", "127.0.0.1"),
		LogLevel: envStr("LOG_LEVEL", "info"),
	}

	if g.MT5StreamPort, err = envInt("MT5_STREAM_PORT", 5556); err != nil {
		return g, err
	}

	if g.MT5CmdPort, err = envInt("MT5_PORT", 5555); err != nil {
		return g, err
	}

	timeoutSec, err := envInt("MT5_TIMEOUT_SECONDS", 10)
	if err != nil {
		return g, err
	}

	if timeoutSec <= 0 {
		return g, fmt.Errorf("config: MT5_TIMEOUT_SECONDS must be > 0, got %d", timeoutSec)
	}

	g.MT5Timeout = time.Duration(timeoutSec) * time.Second

	if g.HTTPPort, err = envInt("GATEWAY_HTTP_PORT", 8081); err != nil {
		return g, err
	}

	if g.Symbols, err = parseSymbols(os.Getenv("SYMBOLS")); err != nil {
		return g, fmt.Errorf("config: SYMBOLS: %w", err)
	}

	g.CandleDataDir = envStr("CANDLE_DATA_DIR", "data/candle")

	if g.BackfillLookback, err = parseLookback(os.Getenv("BACKFILL_LOOKBACK")); err != nil {
		return g, fmt.Errorf("config: BACKFILL_LOOKBACK: %w", err)
	}

	return g, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s=%q is not an integer", key, v)
	}
	return n, nil
}

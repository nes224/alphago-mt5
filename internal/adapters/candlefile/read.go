package candlefile

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

const timeLayout = "2006.01.02 15:04:05"

var header = []string{"time", "open", "high", "low", "close", "tick_volume"}

func ReadFile(path, symbol string, tf domain.Timeframe) ([]domain.Candle, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = len(header)

	first, err := r.Read()
	if err == io.EOF {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if !equal(first, header) {
		return nil, fmt.Errorf("%s: unexpected header %v, want %v", path, first, header)
	}

	var candles []domain.Candle
	for {
		rec, err := r.Read()
		if err == io.EOF {
			return candles, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		c, err := parseRow(rec,symbol, tf)
		if err != nil {
			line, _ := r.FieldPos(0)
			return nil, fmt.Errorf("%s line %d: %w", path, line, err)
		}
		candles = append(candles, c)
	}
}

func parseRow(rec []string, symbol string, tf domain.Timeframe) (domain.Candle, error) {
	t, err := time.Parse(timeLayout, rec[0])
	if err != nil {
		return domain.Candle{}, fmt.Errorf("time %q: %w", rec[0], err)
	}

	var ohlc [4]float64
	for i := range ohlc {
		if ohlc[i], err = strconv.ParseFloat(rec[i+1], 64); err != nil {
			return domain.Candle{}, fmt.Errorf("%s %q: %w", header[i+1], rec[i+1], err)
		}
	}

	ticks, err := strconv.Atoi(rec[5])
	if err != nil {
		return domain.Candle{}, fmt.Errorf("tick_volume %q: %w", rec[5], err)
	}

	return domain.Candle{
		Symbol:    symbol,
		Timeframe: tf,
		OpenTime:  t,
		Open:      ohlc[0],
		High:      ohlc[1],
		Low:       ohlc[2],
		Close:     ohlc[3],
		TickCount: ticks,
		Closed:    true,
	}, nil

}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

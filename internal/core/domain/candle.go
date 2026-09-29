package domain

import "time"

type Candle struct {
	Symbol    string    `json:"symbol"`
	Timeframe string    `json:"timeframe"`
	Open      float64   `json:"open"`
	High      float64   `jso:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}

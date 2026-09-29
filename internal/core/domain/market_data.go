package domain

import "time"

type Tick struct {
	Symbol    string    `json:"symbol"`
	Bid       float64   `json:"bid"`
	Ask       float64   `json:"ask"`
	Volume    int64     `json:"volume"`
	Timestamp time.Time `json:"timestamp"`
}




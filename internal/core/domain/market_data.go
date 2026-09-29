package domain

import "time"

type Tick struct {
	Symbol       string    `json:"symbol"`
	Bid          float64   `json:"bid"`
	Ask          float64   `json:"ask"`
	Volume       int64     `json:"volume"`
	OpenInterest int64     `json:"open_interest"`
	Timestamp    time.Time `json:"timestamp"`
}

func (t Tick) MidPrice() float64 {
	return (t.Bid + t.Ask) / 2.0
}

func (t Tick) Spread() float64 {
	return t.Ask - t.Bid
}

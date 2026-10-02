package domain

import "time"

type TimeframeState struct {
	Direction int           `json:"direction"`
	Slope     float64       `json:"slope"`
	Span      time.Duration `json:"span_seconds"`
	WarmedUp  bool          `json:"warmed_up"`
}

type MultiTimeframeState struct {
	Daily      TimeframeState `json:"daily"`
	H4         TimeframeState `json:"h4"`
	M30        TimeframeState `json:"m30"`
	M15        TimeframeState `json:"m15"`
	Bias       int            `json:"bias"` // +1/-1/0 — 0 means no agreed Daily+H4 direction yet
	AllowsBuy  bool           `json:"allows_buy"`
	AllowsSell bool           `json:"allows_sell"`
	HasData    bool           `json:"has_data"` // false if this symbol has never seen a tick
}

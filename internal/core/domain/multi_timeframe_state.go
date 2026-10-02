package domain

import "time"

// TimeframeState is the observable state of one timeframe window inside the
// Multi-Timeframe Confirmation filter — exposed via GET /api/v1/status so an
// operator can watch it actually warm up live instead of trusting it
// blindly. WarmedUp mirrors the exact condition the filter's Direction
// check applies internally (buffered span against a fraction of the
// window's configured duration); Direction is 0 whenever !WarmedUp.
type TimeframeState struct {
	Direction int           `json:"direction"`
	Slope     float64       `json:"slope"`
	Span      time.Duration `json:"span_seconds"`
	WarmedUp  bool          `json:"warmed_up"`
}

// MultiTimeframeState is the full observable state of a symbol's
// Bias (Daily+H4) -> Confirmation (M30 or M15) pipeline for both possible
// actions.
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

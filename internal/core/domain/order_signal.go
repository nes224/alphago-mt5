package domain

import "time"

type SignalAction string

type OrderSignal struct {
	Symbol     string       `json:"symbol"`
	Action     SignalAction `json:"action"`
	Volume     float64      `json:"volume"`      // Lot size (ถ้าเป็น 0 อาจให้ Risk Engine คำนวณให้ต่อ)
	StopLoss   float64      `json:"stop_loss"`   // ราคา SL (ถ้ามี)
	TakeProfit float64      `json:"take_profit"` // ราคา TP (ถ้ามี)
	Price      float64      `json:"price"`       // ราคา ณ ตอนเกิดสัญญาณ
	Reason     string       `json:"reason"`      // เหตุผลในการออก Signal (เช่น "EMA_CROSS_OVER", "SPIKE_PROTECTION")
	Timestamp  time.Time    `json:"timestamp"`
}

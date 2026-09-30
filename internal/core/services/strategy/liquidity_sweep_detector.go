package strategy

import (
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type SweepType string

const (
	SweepTypeNone SweepType = "NONE"
	SweepTypeHigh SweepType = "SWEEP_HIGH"
	SweepTypeLow  SweepType = "SWEEP_LOW"
)

// SweepEvent เก็บข้อมูลรายละเอียดเมื่อเกิดการ Sweep Liquidity
type SweepEvent struct {
	IsSweep      bool
	SweepType    SweepType
	SweptLevel   float64
	TriggerPrice float64
}

// LiquiditySweepDetector บันทึก Rolling Window ของ N-Ticks เพื่อหา S&R และดักจับ Sweep
type LiquiditySweepDetector struct {
	mu         sync.RWMutex
	windowSize int
	ticks      []domain.Tick
}

func NewLiquiditySweepDetector(windowSize int) *LiquiditySweepDetector {
	return &LiquiditySweepDetector{
		windowSize: windowSize,
		ticks:      make([]domain.Tick, 0, windowSize),
	}
}

func (d *LiquiditySweepDetector) Update(tick domain.Tick) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.ticks) >= d.windowSize {
		d.ticks = d.ticks[1:]
	}

	d.ticks = append(d.ticks, tick)
}

func (d *LiquiditySweepDetector) HighLow() (high, low float64, ok bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.ticks) == 0 {
		return 0, 0, false
	}

	high = d.ticks[0].Ask
	low = d.ticks[0].Bid

	for _, t := range d.ticks {
		if t.Ask > high {
			high = t.Ask
		}
		if t.Bid < low {
			low = t.Bid
		}
	}

	return high, low, true
}

func (d *LiquiditySweepDetector) DetectSweep(currentTick domain.Tick) SweepEvent {
	high, low, ok := d.HighLow()
	if !ok {
		return SweepEvent{IsSweep: false, SweepType: SweepTypeNone}
	}
	// 1. Check Sweep High (Buy-side Liquidity Sweep)
	// ถ้าราคา Ask ทะลุ High เดิมใน Buffer ขึ้นไป
	if currentTick.Ask > high {
		return SweepEvent{
			IsSweep:      true,
			SweepType:    SweepTypeHigh,
			SweptLevel:   high,
			TriggerPrice: currentTick.Ask,
		}
	}

	// 2. Check Sweep Low (Sell-side Liquidity Sweep)
	// ถ้าราคา Bid ทะลุ Low เดิมใน Buffer ลงไป
	if currentTick.Bid < low {
		return SweepEvent{
			IsSweep:      true,
			SweepType:    SweepTypeLow,
			SweptLevel:   low,
			TriggerPrice: currentTick.Bid,
		}
	}

	return SweepEvent{IsSweep: false, SweepType: SweepTypeNone}
}

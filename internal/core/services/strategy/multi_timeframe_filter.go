package strategy

import (
	"sync"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type MultiTimeframeFilter struct {
	mu          sync.Mutex
	minSlope    float64
	symbolState map[string]*symbolWindows
}

type symbolWindows struct {
	daily *TimeWindow
	h4    *TimeWindow
	m30   *TimeWindow
	m15   *TimeWindow
}

func NewMultiTimeframeFilter(minSlope float64) *MultiTimeframeFilter {
	return &MultiTimeframeFilter{
		minSlope:    minSlope,
		symbolState: make(map[string]*symbolWindows),
	}
}

func (f *MultiTimeframeFilter) PushTick(symbol string, price float64, t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()

	sw, ok := f.symbolState[symbol]
	if !ok {
		sw = &symbolWindows{
			daily: NewTimeWindow(24 * time.Hour),
			h4:    NewTimeWindow(4 * time.Hour),
			m30:   NewTimeWindow(30 * time.Minute),
			m15:   NewTimeWindow(15 * time.Minute),
		}
		f.symbolState[symbol] = sw
	}

	sw.daily.Push(t, price)
	sw.h4.Push(t, price)
	sw.m30.Push(t, price)
	sw.m15.Push(t, price)
}

func (f *MultiTimeframeFilter) Allows(symbol string, action domain.SignalAction) bool {
	f.mu.Lock()
	sw, ok := f.symbolState[symbol]
	f.mu.Unlock()
	if !ok {
		return true
	}

	dailyDir := sw.daily.Direction(f.minSlope)
	h4Dir := sw.h4.Direction(f.minSlope)

	// ไม่มีทิศทางชัดเจนพอใน Daily หรือ H4 (ข้อมูลยังไม่พอ หรือตลาด sideway
	// จริงๆ) -> ไม่ขวาง ปล่อยให้ entry trigger เดิมตัดสินใจเอง
	if dailyDir == 0 || h4Dir == 0 {
		return true
	}
	// Daily กับ H4 ชี้คนละทาง -> ไม่เทรดทั้งคู่ทาง (Bias ไม่ชัดเจน)
	if dailyDir != h4Dir {
		return false
	}
	bias := dailyDir

	m30Dir := sw.m30.Direction(f.minSlope)
	m15Dir := sw.m15.Direction(f.minSlope)
	// M30 หรือ M15 อันใดอันหนึ่งพอ ไม่ต้องตรงทั้งคู่ (timeframe เล็กมี noise
	// ตามธรรมชาติ บังคับให้ตรงทั้งคู่จะเข้มเกินไปแบบ Dual-Window เดิม)
	confirmed := (m30Dir != 0 && m30Dir == bias) || (m15Dir != 0 && m15Dir == bias)
	if !confirmed {
		return false
	}

	actionDir := 1
	if action == domain.SignalAction(domain.ActionSell) {
		actionDir = -1
	}
	return actionDir == bias
}

func timeframeState(tw *TimeWindow, minSlope float64) domain.TimeframeState {
	return domain.TimeframeState{
		Direction: tw.Direction(minSlope),
		Slope:     tw.Slope(),
		Span:      tw.Span(),
		WarmedUp:  tw.Span() >= time.Duration(float64(tw.duration)*minDirectionWarmupFraction),
	}
}

func (f *MultiTimeframeFilter) State(symbol string) domain.MultiTimeframeState {
	f.mu.Lock()
	sw, ok := f.symbolState[symbol]
	f.mu.Unlock()
	if !ok {
		return domain.MultiTimeframeState{AllowsBuy: true, AllowsSell: true}
	}

	dailyDir := sw.daily.Direction(f.minSlope)
	h4Dir := sw.h4.Direction(f.minSlope)
	bias := 0
	if dailyDir != 0 && dailyDir == h4Dir {
		bias = dailyDir
	}

	return domain.MultiTimeframeState{
		Daily:      timeframeState(sw.daily, f.minSlope),
		H4:         timeframeState(sw.h4, f.minSlope),
		M30:        timeframeState(sw.m30, f.minSlope),
		M15:        timeframeState(sw.m15, f.minSlope),
		Bias:       bias,
		AllowsBuy:  f.Allows(symbol, domain.SignalAction(domain.ActionBuy)),
		AllowsSell: f.Allows(symbol, domain.SignalAction(domain.ActionSell)),
		HasData:    true,
	}
}

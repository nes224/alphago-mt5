package strategy

import (
	"sync"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// MultiTimeframeFilter implements Top-Down trend confirmation (design notes,
// 2026-09-30): Daily + H4 must agree on direction (Bias) — disagreement
// blocks both directions — then M30 or M15 must agree with that Bias
// (Confirmation) before a tick-level entry signal is allowed through. Entry
// timing itself is still left entirely to VolumeExpansionStrategy /
// LiquiditySweepStrategy; this filter only answers "which direction (if
// any) is currently allowed".
//
// Supersedes the old Dual-Window Trend Filter (SUPERSEDED 2026-09-30 — see
// ROADMAP.md): that one counted a fixed number of ticks instead of real
// time, so its "long-term window" could span 2 minutes or 20 minutes
// depending on market activity and never lined up with any real timeframe.
// It also failed CLOSED (block) whenever it didn't have a confident
// direction, and a badly-guessed threshold made that true for every signal,
// permanently wedging the whole pipeline. This filter deliberately fails
// OPEN (allow) whenever there isn't a clear direction yet or not enough
// history — see Allows() below — so a wrong MinSlope threshold makes this
// filter a no-op in the worst case, not a total block.
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

// NewMultiTimeframeFilter ตั้ง minSlope ตัวเดียวใช้ร่วมกันทุกระดับ (Daily/H4/
// M30/M15) — ยังไม่ได้ผ่าน backtest จริง เป็นค่าประมาณ ต้องเก็บข้อมูล slope
// จริงจากตลาดก่อนถึงจะ tune ค่านี้ได้แม่นยำ (เหมือนค่าอื่นๆ ที่ flag ไว้ใน
// cmd/app/main.go) — แต่เพราะ fail-open ต่อให้ยังไม่แม่น ก็ไม่ทำให้ signal
// ทุกตัวถูกบล็อกแบบ Dual-Window เดิม
func NewMultiTimeframeFilter(minSlope float64) *MultiTimeframeFilter {
	return &MultiTimeframeFilter{
		minSlope:    minSlope,
		symbolState: make(map[string]*symbolWindows),
	}
}

// PushTick ป้อนราคาเข้าทุก TimeWindow ของ symbol นี้พร้อมกัน — เรียกทุก tick
// เหมือน QuantEngine.calculateMetrics ทำกับ Window เดิม
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

// Allows คืน true ถ้า action (BUY/SELL) ไปทางเดียวกับ Bias+Confirmation ของ
// symbol นี้ — คืน true (ไม่ขวาง) ถ้ายังไม่มีข้อมูลพอ หรือ Daily/H4 ยังไม่มี
// ทิศทางชัดเจน (fail-open โดยตั้งใจ — ดู comment บน struct)
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

// State returns the full observable Bias/Confirmation state for a symbol —
// see domain.MultiTimeframeState. Returns HasData=false (everything else
// zero) if the symbol has never been pushed a tick.
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

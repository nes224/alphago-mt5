package strategy

import (
	"fmt"
	"math"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// LiquiditySweepStrategy fades stop-hunt sweeps for a single symbol: a sweep
// of the recent High is faded with a SELL, a sweep of the recent Low is
// faded with a BUY, on the assumption that liquidity was grabbed before a
// reversal. To avoid fading genuine breakouts, it stays silent whenever the
// engine's TrendSlope metric shows the market is trending rather than
// ranging (see Window.Slope).
//
// minLongTermTrendSlope เป็น Dual-Window Trend Filter (ทางเลือก, 0 = ปิด):
// ถ้าตั้งไว้ จะ fade เฉพาะทิศทางที่สอดคล้องกับ trend ใหญ่เท่านั้น — sweep low
// แล้ว fade ด้วย BUY ต้องมี trend ใหญ่เป็นขาขึ้น, sweep high แล้ว fade ด้วย
// SELL ต้องมี trend ใหญ่เป็นขาลง (ซื้อตอนย่อในขาขึ้น/ขายตอนเด้งในขาลง)
type LiquiditySweepStrategy struct {
	id                    string
	symbol                string
	trendSlopeThreshold   float64
	minLongTermTrendSlope float64
	detector              *LiquiditySweepDetector
}

func NewLiquiditySweepStrategy(id string, symbol string, windowSize int, trendSlopeThreshold float64, minLongTermTrendSlope float64) *LiquiditySweepStrategy {
	return &LiquiditySweepStrategy{
		id:                    id,
		symbol:                symbol,
		trendSlopeThreshold:   trendSlopeThreshold,
		minLongTermTrendSlope: minLongTermTrendSlope,
		detector:              NewLiquiditySweepDetector(windowSize),
	}
}

func (s *LiquiditySweepStrategy) ID() string {
	return s.id
}

func (s *LiquiditySweepStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	if tick.Symbol != s.symbol {
		return nil
	}

	event := s.detector.DetectSweep(tick)
	s.detector.Update(tick)

	if event.SweepType == SweepTypeNone {
		return nil
	}

	// Market is trending, not ranging: a sweep here is more likely a real
	// breakout continuation than a stop-hunt reversal, so don't fade it.
	if math.Abs(metrics.TrendSlope) >= s.trendSlopeThreshold {
		return nil
	}

	switch event.SweepType {
	case SweepTypeHigh:
		if !s.longTermTrendAllows(metrics.LongTermTrendSlope, false) {
			return nil
		}
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionSell),
			Price:     tick.Bid,
			Reason:    fmt.Sprintf("LIQUIDITY_SWEEP_FADE_SELL (SweptLevel:%.2f, Trigger:%.2f)", event.SweptLevel, event.TriggerPrice),
			Timestamp: time.Now(),
		}
	case SweepTypeLow:
		if !s.longTermTrendAllows(metrics.LongTermTrendSlope, true) {
			return nil
		}
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionBuy),
			Price:     tick.Ask,
			Reason:    fmt.Sprintf("LIQUIDITY_SWEEP_FADE_BUY (SweptLevel:%.2f, Trigger:%.2f)", event.SweptLevel, event.TriggerPrice),
			Timestamp: time.Now(),
		}
	default:
		return nil
	}
}

// longTermTrendAllows เช็คว่าทิศทางที่จะ fade (wantBuy) สอดคล้องกับ trend ของ
// window ยาวไหม — ถ้า minLongTermTrendSlope <= 0 ถือว่าปิดการเช็คนี้ (อนุญาตเสมอ)
func (s *LiquiditySweepStrategy) longTermTrendAllows(longTermSlope float64, wantBuy bool) bool {
	if s.minLongTermTrendSlope <= 0 {
		return true
	}
	if wantBuy {
		return longTermSlope >= s.minLongTermTrendSlope
	}
	return longTermSlope <= -s.minLongTermTrendSlope
}

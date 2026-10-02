package strategy

import (
	"fmt"
	"math"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

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

func (s *LiquiditySweepStrategy) longTermTrendAllows(longTermSlope float64, wantBuy bool) bool {
	if s.minLongTermTrendSlope <= 0 {
		return true
	}
	if wantBuy {
		return longTermSlope >= s.minLongTermTrendSlope
	}
	return longTermSlope <= -s.minLongTermTrendSlope
}

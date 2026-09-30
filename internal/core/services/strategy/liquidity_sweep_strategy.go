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
type LiquiditySweepStrategy struct {
	id                  string
	symbol              string
	trendSlopeThreshold float64
	detector            *LiquiditySweepDetector
}

func NewLiquiditySweepStrategy(id string, symbol string, windowSize int, trendSlopeThreshold float64) *LiquiditySweepStrategy {
	return &LiquiditySweepStrategy{
		id:                  id,
		symbol:              symbol,
		trendSlopeThreshold: trendSlopeThreshold,
		detector:            NewLiquiditySweepDetector(windowSize),
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
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionSell),
			Price:     tick.Bid,
			Reason:    fmt.Sprintf("LIQUIDITY_SWEEP_FADE_SELL (SweptLevel:%.2f, Trigger:%.2f)", event.SweptLevel, event.TriggerPrice),
			Timestamp: time.Now(),
		}
	case SweepTypeLow:
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

package strategy

import (
	"fmt"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type OIExpansionStrategy struct {
	id            string
	targetZScore  float64
	minOIVelocity float64
	minPriceVel   float64
}

func NewOIExpansionStrategy(id string, targetZscore, minOIVelocity, minPriceVel float64) *OIExpansionStrategy {
	return &OIExpansionStrategy{
		id:            id,
		targetZScore:  targetZscore,
		minOIVelocity: minOIVelocity,
		minPriceVel:   minPriceVel,
	}
}

func (s *OIExpansionStrategy) ID() string {
	return s.id
}

func (s *OIExpansionStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	// Bullish Institutional Expansion: Z-Score พุ่งทะลุ + OI ไหลเข้า + ราคาขยับเร็วขึ้น
	if metrics.ZScore >= s.targetZScore &&
		metrics.OIDelta > 0 &&
		metrics.OIVelocity >= s.minOIVelocity &&
		metrics.PriceVelocity >= s.minPriceVel {
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionBuy),
			Price:     tick.Ask,
			Reason:    fmt.Sprintf("OI_EXPANSION_BUY (Z:%.2f, OIVel:%.1f, PriceVel:%.2f)", metrics.ZScore, metrics.OIVelocity, metrics.PriceVelocity),
			Timestamp: time.Now(),
		}
	}

	// Bearish Institutional Expansion: Z-Score ร่วงทะลุ + OI ไหลเข้า (Short Expansion) + ราคาดิ่งลง
	if metrics.ZScore <= -s.targetZScore && metrics.OIDelta > 0 && metrics.OIVelocity >= s.minOIVelocity && metrics.PriceVelocity <= -s.minPriceVel {
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionSell),
			Price:     tick.Bid,
			Reason:    fmt.Sprintf("OI_EXPANSION_SELL (Z:%.2f, OIVel:%.1f, PriceVel:%.2f)", metrics.ZScore, metrics.OIVelocity, metrics.PriceVelocity),
			Timestamp: time.Now(),
		}
	}

	return nil
}

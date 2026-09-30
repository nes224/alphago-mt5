package strategy

import (
	"fmt"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// VolumeExpansionStrategy ยิง signal เมื่อราคาเบี่ยงจากค่าเฉลี่ยแรง (Z-Score) พร้อม
// Volume ที่พุ่งขึ้นเร็วและราคาขยับไปทางเดียวกัน — ใช้ Volume แทน Open Interest
// เพราะโบรกเกอร์ CFD ส่วนใหญ่ (รวม Exness) ไม่มีข้อมูล Open Interest จริงให้
type VolumeExpansionStrategy struct {
	id                string
	targetZScore      float64
	minVolumeVelocity float64
	minPriceVel       float64
}

func NewVolumeExpansionStrategy(id string, targetZscore, minVolumeVelocity, minPriceVel float64) *VolumeExpansionStrategy {
	return &VolumeExpansionStrategy{
		id:                id,
		targetZScore:      targetZscore,
		minVolumeVelocity: minVolumeVelocity,
		minPriceVel:       minPriceVel,
	}
}

func (s *VolumeExpansionStrategy) ID() string {
	return s.id
}

func (s *VolumeExpansionStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	// Bullish Volume Expansion: Z-Score พุ่งทะลุ + Volume ไหลเข้า + ราคาขยับเร็วขึ้น
	if metrics.ZScore >= s.targetZScore &&
		metrics.VolumeDelta > 0 &&
		metrics.VolumeVelocity >= s.minVolumeVelocity &&
		metrics.PriceVelocity >= s.minPriceVel {
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionBuy),
			Price:     tick.Ask,
			Reason:    fmt.Sprintf("VOLUME_EXPANSION_BUY (Z:%.2f, VolVel:%.1f, PriceVel:%.2f)", metrics.ZScore, metrics.VolumeVelocity, metrics.PriceVelocity),
			Timestamp: time.Now(),
		}
	}

	// Bearish Volume Expansion: Z-Score ร่วงทะลุ + Volume ไหลเข้า (Short Expansion) + ราคาดิ่งลง
	if metrics.ZScore <= -s.targetZScore && metrics.VolumeDelta > 0 && metrics.VolumeVelocity >= s.minVolumeVelocity && metrics.PriceVelocity <= -s.minPriceVel {
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionSell),
			Price:     tick.Bid,
			Reason:    fmt.Sprintf("VOLUME_EXPANSION_SELL (Z:%.2f, VolVel:%.1f, PriceVel:%.2f)", metrics.ZScore, metrics.VolumeVelocity, metrics.PriceVelocity),
			Timestamp: time.Now(),
		}
	}

	return nil
}

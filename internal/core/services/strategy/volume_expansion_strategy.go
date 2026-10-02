package strategy

import (
	"fmt"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// VolumeExpansionStrategy ยิง signal เมื่อราคาเบี่ยงจากค่าเฉลี่ยแรง (Z-Score) พร้อม
// Volume ที่พุ่งขึ้นเร็วและราคาขยับไปทางเดียวกัน — ใช้ Volume แทน Open Interest
// เพราะโบรกเกอร์ CFD ส่วนใหญ่ (รวม Exness) ไม่มีข้อมูล Open Interest จริงให้
//
// minLongTermTrendSlope เป็น Dual-Window Trend Filter (ทางเลือก, 0 = ปิด):
// ถ้าตั้งไว้ จะยิง signal เฉพาะทิศทางที่สอดคล้องกับ trend ของ window ยาว
// (เทรดตามทิศทางใหญ่ ไม่สวนกระแส)
type VolumeExpansionStrategy struct {
	id                    string
	targetZScore          float64
	minVolumeVelocity     float64
	minPriceVel           float64
	minLongTermTrendSlope float64
}

func NewVolumeExpansionStrategy(id string, targetZscore, minVolumeVelocity, minPriceVel, minLongTermTrendSlope float64) *VolumeExpansionStrategy {
	return &VolumeExpansionStrategy{
		id:                    id,
		targetZScore:          targetZscore,
		minVolumeVelocity:     minVolumeVelocity,
		minPriceVel:           minPriceVel,
		minLongTermTrendSlope: minLongTermTrendSlope,
	}
}

func (s *VolumeExpansionStrategy) ID() string {
	return s.id
}

func (s *VolumeExpansionStrategy) OnTick(tick domain.Tick, metrics domain.TickMetrics) *domain.OrderSignal {
	if metrics.ZScore >= s.targetZScore &&
		metrics.VolumeDelta > 0 &&
		metrics.VolumeVelocity >= s.minVolumeVelocity &&
		metrics.PriceVelocity >= s.minPriceVel &&
		s.longTermTrendAllows(metrics.LongTermTrendSlope, true) {
		return &domain.OrderSignal{
			Symbol:    tick.Symbol,
			Action:    domain.SignalAction(domain.ActionBuy),
			Price:     tick.Ask,
			Reason:    fmt.Sprintf("VOLUME_EXPANSION_BUY (Z:%.2f, VolVel:%.1f, PriceVel:%.2f)", metrics.ZScore, metrics.VolumeVelocity, metrics.PriceVelocity),
			Timestamp: time.Now(),
		}
	}

	if metrics.ZScore <= -s.targetZScore &&
		metrics.VolumeDelta > 0 &&
		metrics.VolumeVelocity >= s.minVolumeVelocity &&
		metrics.PriceVelocity <= -s.minPriceVel &&
		s.longTermTrendAllows(metrics.LongTermTrendSlope, false) {
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

func (s *VolumeExpansionStrategy) longTermTrendAllows(longTermSlope float64, wantBuy bool) bool {
	if s.minLongTermTrendSlope <= 0 {
		return true
	}
	if wantBuy {
		return longTermSlope >= s.minLongTermTrendSlope
	}
	return longTermSlope <= -s.minLongTermTrendSlope
}

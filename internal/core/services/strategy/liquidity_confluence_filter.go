package strategy

import (
	"math"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

type LiquidityConfluenceFilter struct {
	minCVDMagnitude    float64 // |CVD| below this = "no clear order flow" -> fail-open
	maxDistanceFromPOC float64 // price further than this from POC = "no liquidity backing" -> block
}

func NewLiquidityConfluenceFilter(minCVDMagnitude, maxDistanceFromPOC float64) *LiquidityConfluenceFilter {
	return &LiquidityConfluenceFilter{
		minCVDMagnitude:    minCVDMagnitude,
		maxDistanceFromPOC: maxDistanceFromPOC,
	}
}

func (f *LiquidityConfluenceFilter) Allows(metrics domain.TickMetrics, action domain.SignalAction) bool {
	return !f.cvdBlocks(metrics, action) && !f.pocBlocks(metrics)
}

func (f *LiquidityConfluenceFilter) cvdBlocks(metrics domain.TickMetrics, action domain.SignalAction) bool {
	if !metrics.CVDReady || math.Abs(metrics.CVD) < f.minCVDMagnitude {
		return false // not enough signal -> no opinion -> allow
	}
	actionDir := 1
	if action == domain.SignalAction(domain.ActionSell) {
		actionDir = -1
	}
	cvdDir := 1
	if metrics.CVD < 0 {
		cvdDir = -1
	}
	return cvdDir != actionDir
}

func (f *LiquidityConfluenceFilter) pocBlocks(metrics domain.TickMetrics) bool {
	if !metrics.POCReady {
		return false // profile not warmed up -> no opinion -> allow
	}
	return math.Abs(metrics.DistanceToPOC) > f.maxDistanceFromPOC
}

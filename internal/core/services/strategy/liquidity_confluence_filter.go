package strategy

import (
	"math"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// LiquidityConfluenceFilter gates entries on real order-flow + liquidity
// backing: Cumulative Volume Delta (CVD) must agree with the signal's
// direction, AND the current price must be near the rolling Volume
// Profile's Point of Control (POC) — i.e. there's real recent volume
// transacted near here, not just a strategy threshold firing on thin ticks.
//
// Unlike MultiTimeframeFilter, this filter is STATELESS: it reads the
// CVD/POC fields QuantEngine already computed on domain.TickMetrics for this
// tick (the same values visible on /api/v1/status) rather than keeping a
// second, duplicate set of rolling windows that could silently diverge from
// what the status endpoint reports.
//
// Fails OPEN exactly like MultiTimeframeFilter: not enough data yet
// (CVDReady/POCReady false) or a reading too close to flat/absent to be
// confident means "no opinion" -> allow. Only blocks on an affirmative,
// confident disagreement — see cvdBlocks/pocBlocks. As with
// MultiTimeframeFilter, this is deliberate: a bad guessed threshold here
// should degrade to a no-op, not a total block of every signal.
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

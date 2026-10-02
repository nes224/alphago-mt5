package strategy_test

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func TestLiquidityConfluenceFilter_AllowsWhenNotWarmedUp(t *testing.T) {
	f := strategy.NewLiquidityConfluenceFilter(100.0, 2.0)
	metrics := domain.TickMetrics{
		CVD: -500, CVDReady: false, // strong disagreement, but not ready
		DistanceToPOC: 10.0, POCReady: false, // far from POC, but not ready
	}
	if !f.Allows(metrics, domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected fail-open: neither CVD nor POC is warmed up yet")
	}
}

func TestLiquidityConfluenceFilter_AllowsWhenCVDFlat(t *testing.T) {
	f := strategy.NewLiquidityConfluenceFilter(100.0, 2.0)
	metrics := domain.TickMetrics{CVD: 10, CVDReady: true, POCReady: false}
	if !f.Allows(metrics, domain.SignalAction(domain.ActionSell)) {
		t.Error("expected fail-open: CVD magnitude below minCVDMagnitude is 'no clear flow'")
	}
}

func TestLiquidityConfluenceFilter_BlocksWhenCVDDisagreesWithAction(t *testing.T) {
	f := strategy.NewLiquidityConfluenceFilter(100.0, 2.0)
	metrics := domain.TickMetrics{CVD: -500, CVDReady: true, POCReady: false}
	if f.Allows(metrics, domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected block: confident sell-pressure CVD disagrees with a BUY signal")
	}
	if !f.Allows(metrics, domain.SignalAction(domain.ActionSell)) {
		t.Error("expected allow: sell-pressure CVD agrees with a SELL signal")
	}
}

func TestLiquidityConfluenceFilter_AllowsWhenPOCNotReady(t *testing.T) {
	f := strategy.NewLiquidityConfluenceFilter(100.0, 2.0)
	metrics := domain.TickMetrics{DistanceToPOC: 999.0, POCReady: false}
	if !f.Allows(metrics, domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected fail-open: POC not warmed up yet, regardless of distance")
	}
}

func TestLiquidityConfluenceFilter_BlocksWhenFarFromPOC(t *testing.T) {
	f := strategy.NewLiquidityConfluenceFilter(100.0, 2.0)
	metrics := domain.TickMetrics{DistanceToPOC: 5.0, POCReady: true}
	if f.Allows(metrics, domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected block: price is further from POC than maxDistanceFromPOC")
	}
}

func TestLiquidityConfluenceFilter_AllowsWhenCloseToPOC(t *testing.T) {
	f := strategy.NewLiquidityConfluenceFilter(100.0, 2.0)
	metrics := domain.TickMetrics{DistanceToPOC: 1.0, POCReady: true}
	if !f.Allows(metrics, domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected allow: price is within maxDistanceFromPOC of a real volume node")
	}
}

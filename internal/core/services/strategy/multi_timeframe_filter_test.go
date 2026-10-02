package strategy_test

import (
	"testing"
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

func TestMultiTimeframeFilter_AllowsWhenNoDataYet(t *testing.T) {
	f := strategy.NewMultiTimeframeFilter(0.01)
	// No PushTick calls at all for this symbol.
	if !f.Allows("XAUUSDm", domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected fail-open (allow) when there is no data yet for the symbol")
	}
}

func pushUptrend(f *strategy.MultiTimeframeFilter, symbol string, base time.Time) {
	price := 4000.0
	const step = 5 * time.Minute
	const totalDuration = 21 * time.Hour
	ticks := int(totalDuration / step)
	for i := 0; i <= ticks; i++ {
		f.PushTick(symbol, price, base.Add(time.Duration(i)*step))
		price += 1.0 // slope ~0.0033/sec, well above the 0.001 minSlope used below
	}
}

func TestMultiTimeframeFilter_AllowsBuyAndBlocksSellInUptrend(t *testing.T) {
	f := strategy.NewMultiTimeframeFilter(0.001)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pushUptrend(f, "XAUUSDm", base)

	if !f.Allows("XAUUSDm", domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected BUY to be allowed in a confirmed uptrend")
	}
	if f.Allows("XAUUSDm", domain.SignalAction(domain.ActionSell)) {
		t.Error("expected SELL to be blocked in a confirmed uptrend")
	}
}

func TestMultiTimeframeFilter_FailsOpenWhenSpanTooShortDespiteStrongSlope(t *testing.T) {
	f := strategy.NewMultiTimeframeFilter(0.001)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	f.PushTick("XAUUSDm", 4000.0, base)
	f.PushTick("XAUUSDm", 4010.0, base.Add(3*time.Second))

	if !f.Allows("XAUUSDm", domain.SignalAction(domain.ActionBuy)) {
		t.Error("expected fail-open: Daily/H4 span is nowhere near their configured duration yet")
	}
	if !f.Allows("XAUUSDm", domain.SignalAction(domain.ActionSell)) {
		t.Error("expected fail-open for both directions while unwarmed")
	}
}

func TestMultiTimeframeFilter_DifferentSymbolsAreIndependent(t *testing.T) {
	f := strategy.NewMultiTimeframeFilter(0.001)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	pushUptrend(f, "XAUUSDm", base)

	// A symbol never pushed to must still fail-open.
	if !f.Allows("EURUSDm", domain.SignalAction(domain.ActionSell)) {
		t.Error("expected a never-seen symbol to fail-open regardless of other symbols' state")
	}
}

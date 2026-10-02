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

// pushUptrend feeds a steadily rising price series spanning enough wall-clock
// time that Daily/H4/M30/M15 windows are all actually warmed up (Span() >=
// 0.8 * their configured duration — see TimeWindow.minDirectionWarmupFraction)
// and see a clear upward slope. 5-minute spacing, 21h total: Daily never
// evicts within 21h so its span grows to the full ~21h (>=19.2h req); H4/
// M30/M15 each settle into a steady-state span equal to their own duration
// (since 5min evenly divides all of them, the oldest surviving point always
// lands exactly `duration` behind the latest one) — 4h/30min/15min, each
// comfortably above their 3.2h/24min/12min warm-up requirement.
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
	// Only 3 seconds of Daily/H4 span, but a huge slope — before the
	// warm-up fix this could spuriously report a confident Direction().
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

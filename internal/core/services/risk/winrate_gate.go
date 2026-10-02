package risk

import (
	"sync"

	"github.com/nes224/alphago-mt5/internal/core/domain"
)

// StrategyWinRate is the minimal view WinRateGate needs of a strategy's
// accumulated track record -- same shape as database.WinRateStat, defined
// locally so this package doesn't depend on the database adapter directly
// (Hexagonal Architecture). main.go maps database.WinRateStat into this.
type StrategyWinRate struct {
	StrategyTag string
	Wins        int
	Losses      int
	TotalTrades int
	WinRate     float64
}

// WinRateGate blocks a signal only if its strategy (extracted from the
// order's Reason via domain.StrategyTagFromReason) has BOTH recorded at
// least minSampleSize trades AND has a win rate below minWinRate. Fails
// OPEN (allows) whenever there isn't enough data yet -- same philosophy as
// MultiTimeframeFilter/LiquidityConfluenceFilter: trading on a strategy
// with only a handful of trades recorded is "no opinion yet", not "proven
// bad", and this gate should never be the thing that permanently wedges a
// brand new strategy that just hasn't accumulated history yet.
type WinRateGate struct {
	mu            sync.RWMutex
	stats         map[string]StrategyWinRate
	minWinRate    float64
	minSampleSize int
}

func NewWinRateGate(minWinRate float64, minSampleSize int) *WinRateGate {
	return &WinRateGate{
		stats:         make(map[string]StrategyWinRate),
		minWinRate:    minWinRate,
		minSampleSize: minSampleSize,
	}
}

// UpdateStats replaces the gate's cached win-rate snapshot wholesale -- call
// periodically (e.g. after each trade close) with fresh results from
// Store.WinRateByStrategy().
func (g *WinRateGate) UpdateStats(stats []StrategyWinRate) {
	m := make(map[string]StrategyWinRate, len(stats))
	for _, s := range stats {
		m[s.StrategyTag] = s
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.stats = m
}

// Allows extracts reason's strategy tag and checks it against the cached
// snapshot -- see the WinRateGate doc comment for the fail-open rule.
func (g *WinRateGate) Allows(reason string) bool {
	tag := domain.StrategyTagFromReason(reason)

	g.mu.RLock()
	stat, ok := g.stats[tag]
	g.mu.RUnlock()

	if !ok || stat.TotalTrades < g.minSampleSize {
		return true
	}
	return stat.WinRate >= g.minWinRate
}

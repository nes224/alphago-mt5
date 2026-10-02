package backtest

import (
	"time"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

// contractSize matches risk/position_sizer.go's assumption (100 oz/lot for
// XAUUSD) -- used here to convert a price-distance PnL into dollars the same
// way the live system implicitly does via lot sizing.
const contractSize = 100.0

// Config holds every tunable the backtest can vary, mirroring the
// equivalent constants/RiskConfig fields in cmd/app/main.go so a backtest
// run can be directly compared against (or used to tune) the live system.
// See DefaultConfig for values that currently match production.
type Config struct {
	Symbol         string
	BufferCapacity int
	WindowSize     int // tick-count window for Z-score/StdDev/Slope

	SweepWindowSize          int
	SweepTrendSlopeThreshold float64

	LongTermWindowSize     int // 0 disables the Dual-Window trend filter, matching production default
	MinLongTermTrendSlope  float64
	MultiTimeframeMinSlope float64 // 0 disables Multi-TF filter

	VolumeExpansionTargetZScore      float64
	VolumeExpansionMinVolumeVelocity float64
	VolumeExpansionMinPriceVel       float64

	SignalCooldown time.Duration

	InitialBalance       float64
	RiskPerTradePercent  float64
	MinLotSize           float64
	MaxLotSize           float64
	MinSLDistance        float64
	MaxSLDistance        float64
	VolatilityMultiplier float64
	ATRMultiplier        float64
	UseATRForSizing      bool
	MaxDailyLossPercent  float64
	MaxOpenPositions     int
	MaxSpreadPips        float64
	MaxConsecutiveLosses int
}

// DefaultConfig mirrors cmd/app/main.go's current constants/app.env defaults
// as of this writing -- kept here (not re-derived by the CLI) so there is a
// single, testable source of "what production currently does" for the
// backtest to default to.
func DefaultConfig(symbol string, initialBalance float64) Config {
	return Config{
		Symbol:         symbol,
		BufferCapacity: 1000,
		WindowSize:     20,

		SweepWindowSize:          300,
		SweepTrendSlopeThreshold: 5.0,

		LongTermWindowSize:     0, // off, matches MinLongTermTrendSlope=0 in main.go
		MinLongTermTrendSlope:  0,
		MultiTimeframeMinSlope: 0.001,

		VolumeExpansionTargetZScore:      2.0,
		VolumeExpansionMinVolumeVelocity: 50.0,
		VolumeExpansionMinPriceVel:       0.2,

		SignalCooldown: 30 * time.Second,

		InitialBalance:       initialBalance,
		RiskPerTradePercent:  0.01,
		MinLotSize:           0.01,
		MaxLotSize:           0.05,
		MinSLDistance:        3.0,
		MaxSLDistance:        20.0,
		VolatilityMultiplier: 2.0,
		ATRMultiplier:        1.75,
		UseATRForSizing:      false,
		MaxDailyLossPercent:  0.03,
		MaxOpenPositions:     5,
		MaxSpreadPips:        5.0,
		MaxConsecutiveLosses: 5,
	}
}

type openPosition struct {
	order     risk.PreparedOrder
	entryTime time.Time
}

// Runner replays a tick stream through a real strategy.QuantEngine +
// risk.RiskManager + risk.RiskGuard (the same types the live system uses --
// see KEEP IN SYNC comment in NewRunner) in a single-threaded, deterministic
// loop, simulating order fills/exits and recording closed Trades.
type Runner struct {
	symbol      string
	quantEngine *strategy.QuantEngine
	riskMgr     *risk.RiskManager
	riskGuard   *risk.RiskGuard

	clock time.Time // read by riskGuard's injected clock -- see NewRunner

	open           map[string]openPosition
	trades         []Trade
	initialBalance float64
	balance        float64
}

// NewRunner wires a fresh backtest engine from cfg.
//
// KEEP IN SYNC WITH cmd/app/main.go:registerStrategies/setupQuantEngine --
// duplicated rather than shared because cmd/app and cmd/backtest are
// separate binaries and main's wiring functions are unexported; extracting a
// shared internal/composition package was considered and rejected as scope
// creep for a research tool (see plan doc).
func NewRunner(cfg Config) *Runner {
	quantEngine := strategy.NewQuantEngine(cfg.BufferCapacity, cfg.WindowSize)
	quantEngine.SetSignalCooldown(cfg.SignalCooldown)
	if cfg.LongTermWindowSize > 0 {
		quantEngine.SetLongTermWindowSize(cfg.LongTermWindowSize)
	}
	if cfg.MultiTimeframeMinSlope > 0 {
		quantEngine.SetMultiTimeframeFilter(strategy.NewMultiTimeframeFilter(cfg.MultiTimeframeMinSlope))
	}

	volumeStrategy := strategy.NewVolumeExpansionStrategy(
		"VOLUME_EXPANSION_"+cfg.Symbol,
		cfg.VolumeExpansionTargetZScore,
		cfg.VolumeExpansionMinVolumeVelocity,
		cfg.VolumeExpansionMinPriceVel,
		cfg.MinLongTermTrendSlope,
	)
	quantEngine.RegisterStrategy(volumeStrategy)

	sweepStrategy := strategy.NewLiquiditySweepStrategy(
		"LIQUIDITY_SWEEP_FADE_"+cfg.Symbol,
		cfg.Symbol,
		cfg.SweepWindowSize,
		cfg.SweepTrendSlopeThreshold,
		cfg.MinLongTermTrendSlope,
	)
	quantEngine.RegisterStrategy(sweepStrategy)

	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{
		MaxDailyLossPercent:  cfg.MaxDailyLossPercent,
		MaxOpenPositions:     cfg.MaxOpenPositions,
		MaxSpreadPips:        cfg.MaxSpreadPips,
		MaxConsecutiveLosses: cfg.MaxConsecutiveLosses,
	}, cfg.InitialBalance)

	riskMgr := risk.NewRiskManager(
		cfg.RiskPerTradePercent, cfg.InitialBalance, cfg.MinLotSize, cfg.MaxLotSize,
		cfg.MinSLDistance, cfg.MaxSLDistance, cfg.VolatilityMultiplier,
		cfg.ATRMultiplier, cfg.UseATRForSizing,
	)

	r := &Runner{
		symbol:         cfg.Symbol,
		quantEngine:    quantEngine,
		riskMgr:        riskMgr,
		riskGuard:      riskGuard,
		open:           make(map[string]openPosition),
		initialBalance: cfg.InitialBalance,
		balance:        cfg.InitialBalance,
	}
	// RiskGuard's daily-reset boundary tracks this clock instead of
	// time.Now() -- see risk.RiskGuard.SetClock's doc comment. Note: the
	// live system's RiskManager balance is also never updated after
	// construction (a pre-existing gap, not introduced here) -- this Runner
	// mirrors that exactly rather than silently "fixing" sizing behavior the
	// live system doesn't actually have yet.
	riskGuard.SetClock(func() time.Time { return r.clock })
	return r
}

// RegisterStrategy adds an additional strategy to the underlying engine --
// mainly for tests that need deterministic signal generation instead of
// coaxing the real strategies' thresholds on hand-crafted price data.
func (r *Runner) RegisterStrategy(s ports.QuantStrategy) {
	r.quantEngine.RegisterStrategy(s)
}

// OnTick feeds one (usually synthetic) tick through the backtest: checks any
// open position for this symbol for an SL/TP touch first (closing it if so),
// then runs the tick through the strategies, then opens a new position for
// any resulting signal that clears sizing + RiskGuard.
func (r *Runner) OnTick(tick domain.Tick) {
	r.clock = tick.Timestamp

	r.checkExit(tick)
	r.quantEngine.ProcessTick(tick)
	r.drainSignals(tick)
}

// checkExit closes an open position for tick.Symbol if this tick's price
// touches its StopLoss or TakeProfit. A BUY is closed by selling (fills at
// Bid); a SELL is closed by buying back (fills at Ask) -- mirrors how
// RiskManager.CalculateOrder fills entries (Buy@Ask, Sell@Bid).
func (r *Runner) checkExit(tick domain.Tick) {
	pos, exists := r.open[tick.Symbol]
	if !exists {
		return
	}

	var exitPrice float64
	var isWin, touched bool

	switch pos.order.Action {
	case domain.SignalAction(domain.ActionBuy):
		switch {
		case tick.Bid <= pos.order.StopLoss:
			exitPrice, isWin, touched = pos.order.StopLoss, false, true
		case tick.Bid >= pos.order.TakeProfit:
			exitPrice, isWin, touched = pos.order.TakeProfit, true, true
		}
	case domain.SignalAction(domain.ActionSell):
		switch {
		case tick.Ask >= pos.order.StopLoss:
			exitPrice, isWin, touched = pos.order.StopLoss, false, true
		case tick.Ask <= pos.order.TakeProfit:
			exitPrice, isWin, touched = pos.order.TakeProfit, true, true
		}
	}

	if !touched {
		return
	}
	r.closePosition(tick.Symbol, pos, exitPrice, isWin, tick.Timestamp)
}

func (r *Runner) closePosition(symbol string, pos openPosition, exitPrice float64, isWin bool, exitTime time.Time) {
	delete(r.open, symbol)

	distance := exitPrice - pos.order.EntryPrice
	if pos.order.Action == domain.SignalAction(domain.ActionSell) {
		distance = -distance
	}
	pnl := distance * pos.order.LotSize * contractSize

	r.balance += pnl
	r.riskGuard.RecordTradeResult(isWin)
	r.riskGuard.MarkPositionClosed(symbol)
	r.riskGuard.UpdateAccountEquity(r.balance)

	strategyTag := domain.StrategyTagFromReason(pos.order.Reason)

	r.trades = append(r.trades, Trade{
		Symbol:      symbol,
		StrategyTag: strategyTag,
		Reason:      pos.order.Reason,
		Session:     domain.MarketSessionFromUTC(exitTime),
		Action:      pos.order.Action,
		EntryTime:   pos.entryTime,
		ExitTime:    exitTime,
		EntryPrice:  pos.order.EntryPrice,
		ExitPrice:   exitPrice,
		LotSize:     pos.order.LotSize,
		PnL:         pnl,
		IsWin:       isWin,
	})
}

// drainSignals processes every signal the engine emitted from the tick just
// processed -- mirrors pipeline.ExecutionRouter's live dispatch loop (sizing
// -> RiskGuard -> open), minus outbox/store/logging which have no meaning
// in a backtest.
func (r *Runner) drainSignals(tick domain.Tick) {
	for {
		select {
		case signal := <-r.quantEngine.SignalChannel():
			r.handleSignal(signal, tick)
		default:
			return
		}
	}
}

func (r *Runner) handleSignal(signal domain.OrderSignal, tick domain.Tick) {
	metrics := r.quantEngine.GetLatestMetrics(signal.Symbol)

	preparedOrder, err := r.riskMgr.CalculateOrder(signal, metrics)
	if err != nil {
		return
	}
	if err := r.riskGuard.ValidateOrder(*preparedOrder, metrics); err != nil {
		return
	}

	// tick.Timestamp, not signal.Timestamp -- the strategies stamp signals
	// with time.Now() (fine live, meaningless in a backtest replayed at
	// computational speed). See allowSignal's analogous fix in quant_engine.go.
	r.open[signal.Symbol] = openPosition{order: *preparedOrder, entryTime: tick.Timestamp}
	r.riskGuard.MarkPositionOpened(signal.Symbol)
}

// Report returns the accumulated results of every OnTick call so far.
func (r *Runner) Report() Report {
	return Report{
		Trades:         r.trades,
		StillOpenAtEnd: len(r.open),
		InitialBalance: r.initialBalance,
		FinalBalance:   r.balance,
	}
}

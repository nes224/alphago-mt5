package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/adapters/config"
	"github.com/nes224/alphago-mt5/internal/adapters/database"
	v1 "github.com/nes224/alphago-mt5/internal/adapters/http/v1"
	"github.com/nes224/alphago-mt5/internal/adapters/logging"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

const (
	BufferCapacity              = 1000
	WindowSize                  = 20
	WorkerCount                 = 4
	TradingSymbol               = "XAUUSDm"
	MaxConsecutiveLosses        = 5
	SweepWindowSize             = 300
	SweepTrendSlopeThreshold    = 5.0
	LongTermWindowSize          = 2000
	MinLongTermTrendSlope       = 0
	MultiTimeframeMinSlope      = 0.001
	LiquidityMinCVDMagnitude    = 100.0 // placeholder, unvalidated
	LiquidityMaxDistanceFromPOC = 2.0   // placeholder ($, ~4 buckets ที่ $0.50/bucket), unvalidated
	SignalCooldown              = 30 * time.Second
	TickHistoryChanBuffer       = 5000
	tickHistoryBatchSize        = 200
	tickHistoryFlushInterval    = 2 * time.Second
	BackfillLookback            = 25 * time.Hour
	MinStrategyWinRate          = 0.50
	MinWinRateSampleSize        = 30
)

func main() {
	cfg := loadConfig()
	logging.Init(cfg.AppEnv)
	log.Info().Str("env", cfg.AppEnv).Msg("🚀 Starting Alphago MT5 Service")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// init mt5 and connect to mt5
	mt5Adapter, streamAdapter := initMT5Adapters(cfg)
	defer mt5Adapter.Close()
	defer streamAdapter.Close()

	// fetch data acc from mt5
	liveAccountInfo := fetchLiveAccountInfo(ctx, mt5Adapter)

	// connect database
	store := connectDatabase(cfg)
	// init account
	accountBalance := loadOrSeedAccountBalance(store, cfg, liveAccountInfo)

	// calculate risk from database if it exist
	riskCfg := loadOrSeedRiskConfig(store, cfg)

	// init quant strategy
	quantEngine := setupQuantEngine()
	backfillQuantEngineState(quantEngine, store, TradingSymbol)
	riskGuard, riskManager := setupRiskManagement(store, riskCfg, accountBalance)

	orderSink := make(chan risk.PreparedOrder, BufferCapacity)
	execRouter := setupExecutionRouter(quantEngine, riskManager, riskGuard, orderSink, store)

	winRateGate := risk.NewWinRateGate(MinStrategyWinRate, MinWinRateSampleSize)
	refreshWinRateGate(winRateGate, store)
	execRouter.SetWinRateGate(winRateGate)

	logUnresolvedPendingOrders(store)

	quantEngine.Start(ctx)
	execRouter.Start(ctx)

	tickHistoryChan := make(chan domain.Tick, TickHistoryChanBuffer)

	go dispatchOrders(ctx, mt5Adapter, riskGuard, store, orderSink)
	go consumeTicks(ctx, streamAdapter, quantEngine, tickHistoryChan)
	go recordTickHistory(ctx, tickHistoryChan, store)
	go consumeTradeClosedEvents(ctx, streamAdapter, riskGuard, store, winRateGate)

	srv := startHTTPServer(cfg, mt5Adapter, riskManager, riskGuard, quantEngine, execRouter, store)

	waitForShutdown(ctx, srv)
}

func loadConfig() config.Config {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		os.Stderr.WriteString("Config Error: " + err.Error() + "\n")
		os.Exit(1)
	}
	return cfg
}

func initMT5Adapters(cfg config.Config) (*mt5.TCPAdapter, *mt5.StreamAdapter) {
	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)

	streamAddr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5StreamPort)
	streamAdapter := mt5.NewStreamAdapter(streamAddr)

	return mt5Adapter, streamAdapter
}

func fetchLiveAccountInfo(ctx context.Context, mt5Adapter *mt5.TCPAdapter) *domain.AccountInfo {
	liveAccountInfo, err := mt5Adapter.GetAccountInfo(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ Failed to fetch live account info from MT5 — falling back to ACCOUNT_BALANCE in app.env (เช็คว่า MT5 Terminal + EA เปิด Algo Trading อยู่ไหม)")
		return nil
	}

	event := log.Info()
	if liveAccountInfo.AccountType == "REAL" {
		event = log.Warn() // เตือนดังๆ กันเผลอยิง order เข้าบัญชีจริงโดยไม่รู้ตัว
	}
	event.
		Str("account_type", liveAccountInfo.AccountType).
		Str("broker", liveAccountInfo.Broker).
		Int64("login", liveAccountInfo.Login).
		Str("currency", liveAccountInfo.Currency).
		Int64("leverage", liveAccountInfo.Leverage).
		Strs("tradable_symbols", liveAccountInfo.Symbols).
		Msg("🔌 Connected to MT5 account")

	return liveAccountInfo
}

func connectDatabase(cfg config.Config) *database.Store {
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Database connection failed")
	}
	return database.NewStore(db)
}

func loadOrSeedAccountBalance(store *database.Store, cfg config.Config, liveAccountInfo *domain.AccountInfo) float64 {
	accountBalance, hasSavedBalance, err := store.LoadAccountBalance()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load account balance from DB")
	}
	if hasSavedBalance {
		log.Info().Float64("balance", accountBalance).Msg("💾 Loaded account balance from DB")
		return accountBalance
	}

	if liveAccountInfo != nil {
		accountBalance = liveAccountInfo.Balance
		if err := store.SaveAccountBalance(accountBalance); err != nil {
			log.Fatal().Err(err).Msg("Failed to seed account balance into DB")
		}
		log.Info().Float64("balance", accountBalance).Msg("💾 Seeded account balance in DB (live from MT5)")
	} else {
		accountBalance = cfg.AccountBalance
		if err := store.SaveAccountBalance(accountBalance); err != nil {
			log.Fatal().Err(err).Msg("Failed to seed account balance into DB")
		}
		log.Info().Float64("balance", accountBalance).Msg("💾 Seeded account balance in DB (from ACCOUNT_BALANCE in app.env — MT5 unreachable)")
	}
	return accountBalance
}

func loadOrSeedRiskConfig(store *database.Store, cfg config.Config) domain.RiskConfig {
	riskCfg, hasSavedConfig, err := store.LoadRiskConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load risk config from DB")
	}
	if hasSavedConfig {
		if riskCfg.VolatilityMultiplier <= 0 {
			log.Warn().Msg("⚠️ risk_config row missing VolatilityMultiplier (เก่ากว่าฟีเจอร์นี้) — เติมจาก app.env ให้อัตโนมัติ")
			riskCfg.VolatilityMultiplier = cfg.VolatilityMultiplier
			if err := store.SaveRiskConfig(riskCfg); err != nil {
				log.Fatal().Err(err).Msg("Failed to backfill VolatilityMultiplier into DB")
			}
		}

		if riskCfg.ATRMultiplier <= 0 {
			log.Warn().Msg("⚠️ risk_config row missing ATRMultiplier (เก่ากว่าฟีเจอร์นี้) — เติมจาก app.env ให้อัตโนมัติ")
			riskCfg.ATRMultiplier = cfg.ATRMultiplier
			if err := store.SaveRiskConfig(riskCfg); err != nil {
				log.Fatal().Err(err).Msg("Failed to backfill ATRMultiplier into DB")
			}
		}

		log.Info().Msg("💾 Loaded risk config from DB (overrides app.env)")
		return riskCfg
	}

	riskCfg = domain.RiskConfig{
		RiskPerTradePercent:  cfg.RiskPerTradePercent,
		MinLotSize:           cfg.MinLotSize,
		MaxLotSize:           cfg.MaxLotSize,
		MinSLDistance:        cfg.MinSLDistance,
		MaxSLDistance:        cfg.MaxSLDistance,
		VolatilityMultiplier: cfg.VolatilityMultiplier,
		ATRMultiplier:        cfg.ATRMultiplier,
		UseATRForSizing:      cfg.UseATRForSizing,
		MaxDailyLossPercent:  cfg.MaxDailyLossPercent,
		MaxOpenPositions:     cfg.MaxOpenPositions,
		MaxSpreadPips:        cfg.MaxSpreadPips,
	}
	if err := store.SaveRiskConfig(riskCfg); err != nil {
		log.Fatal().Err(err).Msg("Failed to seed risk config into DB")
	}
	log.Info().Msg("💾 Seeded risk config in DB (from app.env) — change live via PUT /api/v1/risk/config")
	return riskCfg
}

func setupQuantEngine() *strategy.QuantEngine {
	quantEngine := strategy.NewQuantEngine(BufferCapacity, WindowSize)
	quantEngine.SetSignalCooldown(SignalCooldown)
	quantEngine.SetLongTermWindowSize(LongTermWindowSize)
	quantEngine.SetMultiTimeframeFilter(strategy.NewMultiTimeframeFilter(MultiTimeframeMinSlope))
	registerStrategies(quantEngine)

	return quantEngine
}

func backfillQuantEngineState(quantEngine *strategy.QuantEngine, store *database.Store, symbol string) {
	since := time.Now().Add(-BackfillLookback)
	ticks, err := store.TickHistorySince(symbol, since)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ Failed to load tick_history for backfill — starting cold (Multi-TF/ATR/CVD will warm up from live ticks only)")
		return
	}
	if len(ticks) == 0 {
		log.Info().Str("symbol", symbol).Msg("ℹ️ No tick_history found for backfill yet — starting cold")
		return
	}

	quantEngine.Backfill(ticks)
	log.Info().
		Str("symbol", symbol).
		Int("ticks", len(ticks)).
		Time("since", ticks[0].Timestamp).
		Msg("🔁 Backfilled QuantEngine state from tick_history")
}

func registerStrategies(quantEngine *strategy.QuantEngine) {
	volumeStrategy := strategy.NewVolumeExpansionStrategy("VOLUME_EXPANSION_XAUUSD", 2.0, 50.0, 0.2, MinLongTermTrendSlope)
	quantEngine.RegisterStrategy(volumeStrategy)
	log.Info().Str("strategy", volumeStrategy.ID()).Msg("✅ Registered Strategy")

	sweepStrategy := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_FADE_XAUUSD", TradingSymbol, SweepWindowSize, SweepTrendSlopeThreshold, MinLongTermTrendSlope)
	quantEngine.RegisterStrategy(sweepStrategy)
	log.Info().Str("strategy", sweepStrategy.ID()).Msg("✅ Registered Strategy")
}

func setupRiskManagement(store *database.Store, riskCfg domain.RiskConfig, accountBalance float64) (*risk.RiskGuard, *risk.RiskManager) {
	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{
		MaxDailyLossPercent:  riskCfg.MaxDailyLossPercent,
		MaxOpenPositions:     riskCfg.MaxOpenPositions,
		MaxSpreadPips:        riskCfg.MaxSpreadPips,
		MaxConsecutiveLosses: MaxConsecutiveLosses,
	}, accountBalance)
	if err := riskGuard.AttachStore(store); err != nil {
		log.Fatal().Err(err).Msg("Failed to attach store to RiskGuard")
	}

	riskManager := risk.NewRiskManager(riskCfg.RiskPerTradePercent, accountBalance, riskCfg.MinLotSize, riskCfg.MaxLotSize, riskCfg.MinSLDistance, riskCfg.MaxSLDistance, riskCfg.VolatilityMultiplier, riskCfg.ATRMultiplier, riskCfg.UseATRForSizing)

	return riskGuard, riskManager
}

func setupExecutionRouter(quantEngine *strategy.QuantEngine, riskManager *risk.RiskManager, riskGuard *risk.RiskGuard, orderSink chan risk.PreparedOrder, store *database.Store) *pipeline.ExecutionRouter {
	execRouter := pipeline.NewExecutionRouter(quantEngine, riskManager, riskGuard, orderSink, WorkerCount)
	execRouter.AttachStore(store)
	execRouter.AttachOutboxStore(store)
	return execRouter
}

func logUnresolvedPendingOrders(store *database.Store) {
	unresolved, err := store.UnresolvedPendingOrders()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to check unresolved pending orders on startup")
		return
	}
	for _, o := range unresolved {
		log.Warn().
			Uint("id", o.ID).
			Str("symbol", o.Symbol).
			Str("action", o.Action).
			Str("status", o.Status).
			Time("created_at", o.CreatedAt).
			Msg("⚠️ Unresolved order from a previous run — verify manually in MT5 (tab Trade) before trading this symbol again")
	}
}

func dispatchOrders(ctx context.Context, mt5Adapter *mt5.TCPAdapter, riskGuard *risk.RiskGuard, store *database.Store, orderSink <-chan risk.PreparedOrder) {
	for {
		select {
		case <-ctx.Done():
			return
		case preparedOrder, ok := <-orderSink:
			if !ok {
				return
			}
			dispatchOrder(ctx, mt5Adapter, riskGuard, store, preparedOrder)
		}
	}
}

func dispatchOrder(ctx context.Context, mt5Adapter *mt5.TCPAdapter, riskGuard *risk.RiskGuard, store *database.Store, preparedOrder risk.PreparedOrder) {
	tradeReq := domain.TradeRequest{
		Symbol: preparedOrder.Symbol,
		Action: domain.TradeAction(preparedOrder.Action),
		Volume: preparedOrder.LotSize,
		SL:     preparedOrder.StopLoss,
		TP:     preparedOrder.TakeProfit,
	}

	resp, err := mt5Adapter.SendOrder(ctx, tradeReq)
	switch {
	case err != nil:
		log.Error().Err(err).Str("symbol", tradeReq.Symbol).Msg("❌ Failed to send order to MT5 — verify manually in MT5 before retrying this symbol")
		if preparedOrder.OutboxID != 0 {
			if uerr := store.MarkOrderOutcome(preparedOrder.OutboxID, database.PendingOrderStatusUnknown, 0, err.Error()); uerr != nil {
				log.Warn().Err(uerr).Msg("Failed to mark outbox row as unknown")
			}
		}

	case resp.IsSuccess():
		if err := store.SaveAttribution(resp.Ticket, preparedOrder.Reason); err != nil {
			log.Warn().Err(err).Msg("Failed to save pending attribution")
		}
		if preparedOrder.OutboxID != 0 {
			if uerr := store.MarkOrderOutcome(preparedOrder.OutboxID, database.PendingOrderStatusSent, resp.Ticket, ""); uerr != nil {
				log.Warn().Err(uerr).Msg("Failed to mark outbox row as sent")
			}
		}
		log.Info().
			Str("action", string(tradeReq.Action)).
			Str("symbol", tradeReq.Symbol).
			Float64("lot", tradeReq.Volume).
			Uint64("ticket", resp.Ticket).
			Msg("🟢 ORDER DISPATCHED TO MT5")

	default:
		riskGuard.MarkPositionClosed(preparedOrder.Symbol)
		if preparedOrder.OutboxID != 0 {
			if uerr := store.MarkOrderOutcome(preparedOrder.OutboxID, database.PendingOrderStatusFailed, 0, resp.Message); uerr != nil {
				log.Warn().Err(uerr).Msg("Failed to mark outbox row as failed")
			}
		}
		log.Warn().
			Str("action", string(tradeReq.Action)).
			Str("symbol", tradeReq.Symbol).
			Str("broker_message", resp.Message).
			Msg("🔴 ORDER REJECTED BY MT5")
	}
}

func consumeTicks(ctx context.Context, streamAdapter *mt5.StreamAdapter, quantEngine *strategy.QuantEngine, historyChan chan<- domain.Tick) {
	tickChan, err := streamAdapter.SubscribeTicks(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe ticks")
		return
	}

	log.Info().Msg("🔌 Listening to Live Tick Stream...")
	for tick := range tickChan {
		quantEngine.PushTick(tick)

		select {
		case historyChan <- tick:
		default:
			log.Warn().Str("symbol", tick.Symbol).Msg("[TickHistory] recorder backlog full, dropping tick")
		}
	}
	log.Info().Msg("Tick consumer stopped")
}

func recordTickHistory(ctx context.Context, historyChan <-chan domain.Tick, store *database.Store) {
	batch := make([]domain.Tick, 0, tickHistoryBatchSize)
	ticker := time.NewTicker(tickHistoryFlushInterval)
	defer ticker.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := store.SaveTickHistoryBatch(batch); err != nil {
			log.Warn().Err(err).Int("count", len(batch)).Msg("Failed to save tick history batch")
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case tick, ok := <-historyChan:
			if !ok {
				flush()
				return
			}
			batch = append(batch, tick)
			if len(batch) >= tickHistoryBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func consumeTradeClosedEvents(ctx context.Context, streamAdapter *mt5.StreamAdapter, riskGuard *risk.RiskGuard, store *database.Store, winRateGate *risk.WinRateGate) {
	tradeClosedChan, err := streamAdapter.SubscribeTradeEvents(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe trade closed events")
		return
	}

	for event := range tradeClosedChan {
		handleTradeClosed(riskGuard, store, winRateGate, event)
	}
}

func refreshWinRateGate(gate *risk.WinRateGate, store *database.Store) {
	stats, err := store.WinRateByStrategy()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to refresh win-rate gate stats")
		return
	}

	out := make([]risk.StrategyWinRate, len(stats))
	for i, s := range stats {
		out[i] = risk.StrategyWinRate{
			StrategyTag: s.StrategyTag,
			Wins:        s.Wins,
			Losses:      s.Losses,
			TotalTrades: s.TotalTrades,
			WinRate:     s.WinRate,
		}
	}
	gate.UpdateStats(out)
}

func handleTradeClosed(riskGuard *risk.RiskGuard, store *database.Store, winRateGate *risk.WinRateGate, event domain.TradeClosedEvent) {
	isWin := event.Profit > 0
	riskGuard.RecordTradeResult(isWin)
	riskGuard.MarkPositionClosed(event.Symbol)

	currentBalance, _, err := store.LoadAccountBalance()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to load account balance while processing trade close")
		return
	}
	newBalance := currentBalance + event.Profit
	if err := store.SaveAccountBalance(newBalance); err != nil {
		log.Warn().Err(err).Msg("Failed to save account balance after trade close")
	}
	riskGuard.UpdateAccountEquity(newBalance)

	reason, ok, attrErr := store.LoadAndDeleteAttribution(event.Ticket)
	if attrErr != nil {
		log.Warn().Err(attrErr).Uint64("ticket", event.Ticket).Msg("Failed to load pending attribution")
	}
	if ok {
		strategyTag := domain.StrategyTagFromReason(reason)

		if err := store.SaveTradeOutcome(database.TradeOutcome{
			Symbol:      event.Symbol,
			StrategyTag: strategyTag,
			Reason:      reason,
			Ticket:      event.Ticket,
			Profit:      event.Profit,
			IsWin:       isWin,
			Session:     domain.MarketSessionFromUTC(event.Timestamp),
			Timestamp:   event.Timestamp,
		}); err != nil {
			log.Warn().Err(err).Msg("Failed to save trade outcome")
		} else {
			refreshWinRateGate(winRateGate, store)
		}
	} else {
		log.Warn().Uint64("ticket", event.Ticket).Msg("Trade closed but no matching signal reason found (service restarted while position was open?)")
	}

	log.Info().
		Str("symbol", event.Symbol).
		Uint64("ticket", event.Ticket).
		Float64("profit", event.Profit).
		Float64("new_balance", newBalance).
		Msg("💰 Trade closed")
}

func startHTTPServer(
	cfg config.Config,
	mt5Adapter *mt5.TCPAdapter,
	riskManager *risk.RiskManager,
	riskGuard *risk.RiskGuard,
	quantEngine *strategy.QuantEngine,
	execRouter *pipeline.ExecutionRouter,
	store *database.Store,
) *http.Server {
	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"env":    cfg.AppEnv,
		})
	})

	r = v1.RouterV1(r, mt5Adapter, riskManager, riskGuard, quantEngine, execRouter, store, TradingSymbol)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Info().Str("addr", srv.Addr).Msg("🌐 HTTP Server is running")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP Server listen error")
		}
	}()

	return srv
}

func waitForShutdown(ctx context.Context, srv *http.Server) {
	<-ctx.Done()
	log.Info().Msg("🛑 Shutting down Alphago MT5 Service gracefully...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP Server forced to shutdown")
	}

	log.Info().Msg("👋 Server exiting successfully.")
}

package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	BufferCapacity = 1000
	WindowSize     = 20
	WorkerCount    = 4

	TradingSymbol = "XAUUSDm"

	MaxConsecutiveLosses = 5

	// SweepWindowSize นับเป็นจำนวน tick ไม่ใช่เวลา — 20 tick แคบเกินไปมากบน
	// tick สดจริง (ราคาแทบทุก tick ทำ high/low ใหม่เทียบกับ 20 tick ล่าสุด
	// อยู่แล้วโดยธรรมชาติ ไม่ใช่การกวาด stop จริง) ขยับขึ้นมาก แต่ก็ยังเป็น
	// ค่าประมาณ ต้องดู tick rate จริงแล้วปรับอีกที
	SweepWindowSize = 300
	// SweepTrendSlopeThreshold เป็นค่าเริ่มต้นแบบหยาบ ยังไม่ได้ผ่าน backtest —
	// ปรับตามพฤติกรรมราคาจริงของ TradingSymbol ทีหลัง
	SweepTrendSlopeThreshold = 5.0

	// LongTermWindowSize คือ Dual-Window Trend Filter — window ที่ยาวกว่า
	// WindowSize (20 tick) มาก ใช้เป็น proxy "higher timeframe" โดยไม่ต้อง
	// สร้าง candle aggregator จริง ยังเป็นค่าประมาณ ต้องดู tick rate จริงก่อนปรับ
	LongTermWindowSize = 2000
	// MinLongTermTrendSlope เป็น threshold ตัดสินว่า slope ของ window ยาวถือว่า
	// "มีทิศทาง" พอจะใช้กรองหรือยัง — ปิดไว้ก่อน (0 = ปิด) เพราะค่าจริงที่วัดได้
	// จาก /api/v1/status (~0.0009) เล็กกว่าค่าเดิมที่เดาไว้ (0.05) เกือบ 60 เท่า
	// ทำให้ gate บล็อกทุก signal ไม่มีวันผ่านเลย — ต้องเก็บข้อมูล slope จริงช่วง
	// ที่ตลาด trend ชัดๆ ก่อน ถึงจะตั้ง threshold ที่ใช้งานได้จริง
	MinLongTermTrendSlope = 0

	// SignalCooldown กันไม่ให้ QuantEngine ยิง signal ถี่เกินไปต่อ symbol
	// ไม่ว่า threshold ของ strategy ตัวไหนจะยังไม่ได้ tune ดีแค่ไหนก็ตาม —
	// เป็น safety net ชั้นสุดท้ายก่อนถึง Risk Guard
	SignalCooldown = 30 * time.Second
)

func main() {
	cfg := loadConfig()
	logging.Init(cfg.AppEnv)
	log.Info().Str("env", cfg.AppEnv).Msg("🚀 Starting Alphago MT5 Service")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	mt5Adapter, streamAdapter := initMT5Adapters(cfg)
	defer mt5Adapter.Close()
	defer streamAdapter.Close()

	liveAccountInfo := fetchLiveAccountInfo(ctx, mt5Adapter)

	store := connectDatabase(cfg)
	accountBalance := loadOrSeedAccountBalance(store, cfg, liveAccountInfo)
	riskCfg := loadOrSeedRiskConfig(store, cfg)

	quantEngine := setupQuantEngine()
	riskGuard, riskManager := setupRiskManagement(store, riskCfg, accountBalance)

	orderSink := make(chan risk.PreparedOrder, BufferCapacity)
	execRouter := setupExecutionRouter(quantEngine, riskManager, riskGuard, orderSink, store)

	logUnresolvedPendingOrders(store)

	quantEngine.Start(ctx)
	execRouter.Start(ctx)

	go dispatchOrders(ctx, mt5Adapter, riskGuard, store, orderSink)
	go consumeTicks(ctx, streamAdapter, quantEngine)
	go consumeTradeClosedEvents(ctx, streamAdapter, riskGuard, store)

	srv := startHTTPServer(cfg, mt5Adapter, riskManager, riskGuard, quantEngine, execRouter, store)

	waitForShutdown(ctx, srv)
}

// loadConfig อ่าน app.env — logger ยังไม่พร้อมใช้ตอนนี้ (ต้องมี cfg.AppEnv
// ก่อน) เลยเขียน error ลง os.Stderr ตรงๆ แทน
func loadConfig() config.Config {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		os.Stderr.WriteString("Config Error: " + err.Error() + "\n")
		os.Exit(1)
	}
	return cfg
}

// initMT5Adapters สร้าง adapter ทั้งคู่ (Command TCP สำหรับส่ง order/ขอข้อมูล
// บัญชี และ Market Data Stream สำหรับรับ tick/trade_closed event)
func initMT5Adapters(cfg config.Config) (*mt5.TCPAdapter, *mt5.StreamAdapter) {
	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)

	streamAddr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5StreamPort)
	streamAdapter := mt5.NewStreamAdapter(streamAddr)

	return mt5Adapter, streamAdapter
}

// fetchLiveAccountInfo ดึงข้อมูลบัญชีสดจาก MT5 (balance, account type, broker,
// symbol ที่เทรดได้) — ต้องมี MT5 Terminal + EA เปิด Algo Trading อยู่แล้ว
// ตอนนี้ ถ้าต่อไม่ได้ (เช่นยังไม่ได้ attach EA) คืน nil ให้ผู้เรียก fallback
// ไปใช้ app.env แทน ไม่ fail การ start ทั้งระบบ
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

// connectDatabase เปิด connection ไปยัง PostgreSQL — persist account balance /
// risk config / risk guard state / signal history ข้าม restart
func connectDatabase(cfg config.Config) *database.Store {
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Database connection failed")
	}
	return database.NewStore(db)
}

// loadOrSeedAccountBalance โหลด balance จาก DB ถ้าเคยบันทึกไว้ (run ก่อนหน้า)
// ไม่งั้น seed ด้วย balance สดจาก MT5 (ถ้าดึงได้) หรือ ACCOUNT_BALANCE ใน
// app.env (ถ้าดึงจาก MT5 ไม่ได้) — หลังจากนี้ DB คือ source of truth เสมอ
// อัปเดตตาม P/L เหตุการณ์จริง ไม่อ่านจาก MT5 สดซ้ำอีก
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

// loadOrSeedRiskConfig โหลด risk policy จาก DB ถ้าเคยตั้งผ่าน PUT
// /api/v1/risk/config มาก่อน ไม่งั้น seed จากค่าใน app.env — ค่าที่ seed ไว้
// ยัง "ปรับทีหลังได้โดยไม่ต้อง restart" ผ่าน endpoint เดียวกัน
func loadOrSeedRiskConfig(store *database.Store, cfg config.Config) domain.RiskConfig {
	riskCfg, hasSavedConfig, err := store.LoadRiskConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load risk config from DB")
	}
	if hasSavedConfig {
		log.Info().Msg("💾 Loaded risk config from DB (overrides app.env)")
		return riskCfg
	}

	riskCfg = domain.RiskConfig{
		RiskPerTradePercent: cfg.RiskPerTradePercent,
		MinLotSize:          cfg.MinLotSize,
		MaxLotSize:          cfg.MaxLotSize,
		MinSLDistance:       cfg.MinSLDistance,
		MaxSLDistance:       cfg.MaxSLDistance,
		MaxDailyLossPercent: cfg.MaxDailyLossPercent,
		MaxOpenPositions:    cfg.MaxOpenPositions,
		MaxSpreadPips:       cfg.MaxSpreadPips,
	}
	if err := store.SaveRiskConfig(riskCfg); err != nil {
		log.Fatal().Err(err).Msg("Failed to seed risk config into DB")
	}
	log.Info().Msg("💾 Seeded risk config in DB (from app.env) — change live via PUT /api/v1/risk/config")
	return riskCfg
}

// setupQuantEngine สร้าง PureQuantEngine และลงทะเบียน strategy ทั้งหมด
func setupQuantEngine() *strategy.QuantEngine {
	quantEngine := strategy.NewQuantEngine(BufferCapacity, WindowSize)
	quantEngine.SetSignalCooldown(SignalCooldown)
	quantEngine.SetLongTermWindowSize(LongTermWindowSize)

	registerStrategies(quantEngine)

	return quantEngine
}

func registerStrategies(quantEngine *strategy.QuantEngine) {
	// Register Volume Expansion Strategy (ใช้ Volume แทน Open Interest เพราะ
	// Exness/โบรกเกอร์ CFD ไม่ส่งข้อมูล Open Interest จริงมาให้)
	// minVolumeVelocity ขยับขึ้นจาก 1.0 (ไวเกินไปมาก ยิงแทบทุก tick บนข้อมูลจริง)
	// — ยังเป็นค่าประมาณ ต้องดู VolVel จริงจาก /api/v1/signals แล้ว tune ต่อ
	volumeStrategy := strategy.NewVolumeExpansionStrategy("VOLUME_EXPANSION_XAUUSD", 2.0, 50.0, 0.2, MinLongTermTrendSlope)
	quantEngine.RegisterStrategy(volumeStrategy)
	log.Info().Str("strategy", volumeStrategy.ID()).Msg("✅ Registered Strategy")

	// Register Liquidity Sweep Fade Strategy
	sweepStrategy := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_FADE_XAUUSD", TradingSymbol, SweepWindowSize, SweepTrendSlopeThreshold, MinLongTermTrendSlope)
	quantEngine.RegisterStrategy(sweepStrategy)
	log.Info().Str("strategy", sweepStrategy.ID()).Msg("✅ Registered Strategy")
}

// setupRiskManagement สร้าง RiskGuard (circuit breaker, position tracking,
// persist ผ่าน store) และ RiskManager (position sizing) จาก risk policy ที่
// โหลด/seed ไว้แล้ว
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

	riskManager := risk.NewRiskManager(riskCfg.RiskPerTradePercent, accountBalance, riskCfg.MinLotSize, riskCfg.MaxLotSize, riskCfg.MinSLDistance, riskCfg.MaxSLDistance)

	return riskGuard, riskManager
}

// setupExecutionRouter ผูก ExecutionRouter เข้ากับ store ทั้งสองแบบ (signal
// history ธรรมดา + outbox pattern สำหรับ order ที่กำลังจะส่ง)
func setupExecutionRouter(quantEngine *strategy.QuantEngine, riskManager *risk.RiskManager, riskGuard *risk.RiskGuard, orderSink chan risk.PreparedOrder, store *database.Store) *pipeline.ExecutionRouter {
	execRouter := pipeline.NewExecutionRouter(quantEngine, riskManager, riskGuard, orderSink, WorkerCount)
	execRouter.AttachStore(store)
	execRouter.AttachOutboxStore(store)
	return execRouter
}

// logUnresolvedPendingOrders เตือนตอน startup ถ้าเจอ outbox row ที่ยังค้าง
// PENDING (crash ระหว่างตัดสินใจส่งกับส่งจริง) หรือ UNKNOWN (SendOrder error
// เช่น timeout/EOF ไม่รู้ว่าเข้าตลาดจริงไหม) — ไม่ auto-resend เพราะเสี่ยง
// เปิด position ซ้อน แค่เตือนดังๆ ให้เช็คมือผ่าน MT5 tab Trade ก่อน (ดู
// รายละเอียดที่ GET /api/v1/pending-orders)
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

// dispatchOrders ดึง PreparedOrder จาก orderSink แล้วส่งไปยัง MT5 ผ่าน TCP
// Adapter ทีละตัว จนกว่า ctx จะถูกยกเลิกหรือ channel ถูกปิด
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

// dispatchOrder ส่ง order เดียวเข้า MT5 แล้วจัดการผลลัพธ์ 3 แบบ: ส่งสำเร็จ
// (บันทึก attribution + outbox SENT), broker ปฏิเสธชัดเจน (ปลด position lock
// ทันทีเพราะรู้แน่ว่าไม่เปิด + outbox FAILED), หรือ transport error ที่ไม่รู้
// ผลจริง (outbox UNKNOWN, ไม่แตะ position lock, ไม่ auto-resend)
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

// consumeTicks สมัครรับ tick stream จาก MT5 แล้วป้อนเข้า QuantEngine ทีละ
// tick — ถ้าสมัครไม่สำเร็จแค่ log แล้วจบ (ไม่ fail ทั้งระบบ)
func consumeTicks(ctx context.Context, streamAdapter *mt5.StreamAdapter, quantEngine *strategy.QuantEngine) {
	tickChan, err := streamAdapter.SubscribeTicks(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe ticks")
		return
	}

	log.Info().Msg("🔌 Listening to Live Tick Stream...")
	for tick := range tickChan {
		quantEngine.PushTick(tick)
	}
	log.Info().Msg("Tick consumer stopped")
}

// consumeTradeClosedEvents สมัครรับ trade_closed event จาก MT5 (ใช้ connection
// เดียวกับ tick stream — ต้องแก้ EA ให้ยิง OnTradeTransaction() มาด้วย ไม่งั้น
// channel นี้จะเงียบตลอด) แล้วป้อนผลแพ้/ชนะจริงกลับเข้า RiskGuard ทีละ event
func consumeTradeClosedEvents(ctx context.Context, streamAdapter *mt5.StreamAdapter, riskGuard *risk.RiskGuard, store *database.Store) {
	tradeClosedChan, err := streamAdapter.SubscribeTradeEvents(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe trade closed events")
		return
	}

	for event := range tradeClosedChan {
		handleTradeClosed(riskGuard, store, event)
	}
}

// handleTradeClosed อัปเดต RiskGuard + balance ตาม P/L จริง แล้ว attribute
// ผลลัพธ์กลับไปหา strategy ต้นเหตุถ้าจับคู่ ticket ได้ (persist ลง
// pending_attributions แล้ว รอดจาก restart ระหว่าง position ยังเปิดค้างอยู่ได้)
func handleTradeClosed(riskGuard *risk.RiskGuard, store *database.Store, event domain.TradeClosedEvent) {
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
		strategyTag := reason
		if idx := strings.Index(reason, " ("); idx >= 0 {
			strategyTag = reason[:idx]
		}

		if err := store.SaveTradeOutcome(database.TradeOutcome{
			Symbol:      event.Symbol,
			StrategyTag: strategyTag,
			Reason:      reason,
			Ticket:      event.Ticket,
			Profit:      event.Profit,
			IsWin:       isWin,
			Timestamp:   event.Timestamp,
		}); err != nil {
			log.Warn().Err(err).Msg("Failed to save trade outcome")
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

// startHTTPServer ประกอบ gin router (health check + /api/v1/* ทั้งหมดผ่าน
// v1.RouterV1) แล้ว start ListenAndServe ใน goroutine แยก คืน *http.Server
// กลับไปให้ waitForShutdown ปิดแบบ graceful ทีหลัง
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

// waitForShutdown บล็อกจนกว่า ctx จะถูกยกเลิก (SIGINT/SIGTERM) แล้วปิด HTTP
// server แบบ graceful (รอ request ที่ค้างอยู่ให้เสร็จก่อน ไม่เกิน 5 วินาที)
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

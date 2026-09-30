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
	httphandler "github.com/nes224/alphago-mt5/internal/adapters/http"
	"github.com/nes224/alphago-mt5/internal/adapters/logging"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
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

	// SignalCooldown กันไม่ให้ QuantEngine ยิง signal ถี่เกินไปต่อ symbol
	// ไม่ว่า threshold ของ strategy ตัวไหนจะยังไม่ได้ tune ดีแค่ไหนก็ตาม —
	// เป็น safety net ชั้นสุดท้ายก่อนถึง Risk Guard
	SignalCooldown = 30 * time.Second
)

func main() {
	// 1. Load Configuration
	cfg, err := config.LoadConfig(".")
	if err != nil {
		// ยังไม่ได้ตั้งค่า logger ตรงนี้ (ต้องมี cfg.AppEnv ก่อน) — ใช้ os.Stderr ตรงๆ
		os.Stderr.WriteString("Config Error: " + err.Error() + "\n")
		os.Exit(1)
	}

	logging.Init(cfg.AppEnv)
	log.Info().Str("env", cfg.AppEnv).Msg("🚀 Starting Alphago MT5 Service")

	// 2. Master Context & OS Signal Trap (Graceful Shutdown)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 2.5 Connect to PostgreSQL — persist account balance / risk guard state /
	// signal history ข้าม restart (internal/adapters/database)
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Database connection failed")
	}
	store := database.NewStore(db)

	// ACCOUNT_BALANCE ใน app.env เป็นแค่ seed สำหรับรันครั้งแรก — ถ้าเคย
	// บันทึก balance ไว้ใน DB แล้ว (จาก run ก่อนหน้า) จะใช้ค่านั้นแทน
	accountBalance, hasSavedBalance, err := store.LoadAccountBalance()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load account balance from DB")
	}
	if !hasSavedBalance {
		accountBalance = cfg.AccountBalance
		if err := store.SaveAccountBalance(accountBalance); err != nil {
			log.Fatal().Err(err).Msg("Failed to seed account balance into DB")
		}
		log.Info().Float64("balance", accountBalance).Msg("💾 Seeded account balance in DB (from ACCOUNT_BALANCE in app.env)")
	} else {
		log.Info().Float64("balance", accountBalance).Msg("💾 Loaded account balance from DB")
	}

	// 3. Initialize Adapters (MT5 Command TCP & Market Data Stream)
	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)
	defer mt5Adapter.Close()

	streamAddr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5StreamPort)
	streamAdapter := mt5.NewStreamAdapter(streamAddr)
	defer streamAdapter.Close()

	// 4. Initialize Core Quant Engine & Strategies
	quantEngine := strategy.NewQuantEngine(BufferCapacity, WindowSize)
	quantEngine.SetSignalCooldown(SignalCooldown)

	// Register Volume Expansion Strategy (ใช้ Volume แทน Open Interest เพราะ
	// Exness/โบรกเกอร์ CFD ไม่ส่งข้อมูล Open Interest จริงมาให้)
	// minVolumeVelocity ขยับขึ้นจาก 1.0 (ไวเกินไปมาก ยิงแทบทุก tick บนข้อมูลจริง)
	// — ยังเป็นค่าประมาณ ต้องดู VolVel จริงจาก /api/v1/signals แล้ว tune ต่อ
	volumeStrategy := strategy.NewVolumeExpansionStrategy("VOLUME_EXPANSION_XAUUSD", 2.0, 50.0, 0.2)
	quantEngine.RegisterStrategy(volumeStrategy)
	log.Info().Str("strategy", volumeStrategy.ID()).Msg("✅ Registered Strategy")

	// Register Liquidity Sweep Fade Strategy
	sweepStrategy := strategy.NewLiquiditySweepStrategy("LIQUIDITY_SWEEP_FADE_XAUUSD", TradingSymbol, SweepWindowSize, SweepTrendSlopeThreshold)
	quantEngine.RegisterStrategy(sweepStrategy)
	log.Info().Str("strategy", sweepStrategy.ID()).Msg("✅ Registered Strategy")

	// 5. Initialize Risk Management & Execution Pipeline
	// ค่า risk/lot/SL-TP ดึงจาก app.env (ปรับได้โดยไม่ต้อง compile ใหม่) —
	// ยังไม่มีการดึง balance สดจาก MT5 อัตโนมัติ (แยกจาก accountBalance ที่มา
	// จาก DB/seed ด้านบน) ต้องอัปเดต balance เองเป็นระยะจนกว่าจะมี integration
	// กับ MT5 account info จริง
	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{
		MaxDailyLossPercent:  cfg.MaxDailyLossPercent,
		MaxOpenPositions:     cfg.MaxOpenPositions,
		MaxSpreadPips:        cfg.MaxSpreadPips,
		MaxConsecutiveLosses: MaxConsecutiveLosses,
	}, accountBalance)
	if err := riskGuard.AttachStore(store); err != nil {
		log.Fatal().Err(err).Msg("Failed to attach store to RiskGuard")
	}
	riskManager := risk.NewRiskManager(cfg.RiskPerTradePercent, accountBalance, cfg.MinLotSize, cfg.MaxLotSize, cfg.MinSLDistance, cfg.MaxSLDistance)

	orderSink := make(chan risk.PreparedOrder, BufferCapacity)
	execRouter := pipeline.NewExecutionRouter(quantEngine, riskManager, riskGuard, orderSink, WorkerCount)
	execRouter.AttachStore(store)

	// 6. Start Engine & Router Background Workers
	quantEngine.Start(ctx)
	execRouter.Start(ctx)

	// 7. Worker: Dispatch Prepared Orders จาก Engine ส่งไปยัง MT5 ผ่าน TCP Adapter
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case preparedOrder, ok := <-orderSink:
				if !ok {
					return
				}

				tradeReq := domain.TradeRequest{
					Symbol: preparedOrder.Symbol,
					Action: domain.TradeAction(preparedOrder.Action),
					Volume: preparedOrder.LotSize,
					SL:     preparedOrder.StopLoss,
					TP:     preparedOrder.TakeProfit,
				}

				_, err := mt5Adapter.SendOrder(ctx, tradeReq)
				if err != nil {
					log.Error().Err(err).Str("symbol", tradeReq.Symbol).Msg("❌ Failed to send order to MT5")
				} else {
					log.Info().
						Str("action", string(tradeReq.Action)).
						Str("symbol", tradeReq.Symbol).
						Float64("lot", tradeReq.Volume).
						Msg("🟢 ORDER DISPATCHED TO MT5")
				}
			}
		}
	}()

	// 8. Stream Ticks Consumer -> Push เข้า PureQuantEngine
	tickChan, err := streamAdapter.SubscribeTicks(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe ticks")
	} else {
		go func() {
			log.Info().Msg("🔌 Listening to Live Tick Stream...")
			for tick := range tickChan {
				quantEngine.PushTick(tick)
			}
			log.Info().Msg("Tick consumer stopped")
		}()
	}

	// 8.5 Trade Closed Events Consumer -> ป้อนผลแพ้/ชนะจริงกลับเข้า RiskGuard
	// และอัปเดต balance ตาม P/L จริง (ใช้ connection เดียวกับ tick stream —
	// ต้องแก้ EA ให้ยิง OnTradeTransaction() มาด้วย ไม่งั้น channel นี้จะเงียบตลอด)
	tradeClosedChan, err := streamAdapter.SubscribeTradeEvents(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe trade closed events")
	} else {
		go func() {
			for event := range tradeClosedChan {
				riskGuard.RecordTradeResult(event.Profit > 0)
				riskGuard.MarkPositionClosed(event.Symbol)

				currentBalance, _, err := store.LoadAccountBalance()
				if err != nil {
					log.Warn().Err(err).Msg("Failed to load account balance while processing trade close")
					continue
				}
				newBalance := currentBalance + event.Profit
				if err := store.SaveAccountBalance(newBalance); err != nil {
					log.Warn().Err(err).Msg("Failed to save account balance after trade close")
				}
				riskGuard.UpdateAccountEquity(newBalance)

				log.Info().
					Str("symbol", event.Symbol).
					Uint64("ticket", event.Ticket).
					Float64("profit", event.Profit).
					Float64("new_balance", newBalance).
					Msg("💰 Trade closed")
			}
		}()
	}

	// 9. Setup HTTP Services & Handlers (REST API)
	tradeService := services.NewTradeService(mt5Adapter, riskGuard)
	tradeHandler := httphandler.NewTradeHandler(tradeService)
	monitorHandler := httphandler.NewMonitorHandler(quantEngine, execRouter, riskGuard, TradingSymbol)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"env":    cfg.AppEnv,
		})
	})

	v1 := r.Group("/api/v1")
	{
		v1.POST("/trade", tradeHandler.PlaceOrder)
		v1.POST("/orders/close", tradeHandler.CloseOrder)
		v1.PUT("/orders/modify", tradeHandler.ModifyOder)
		v1.GET("/status", monitorHandler.Status)
		v1.GET("/signals", monitorHandler.RecentSignals)
	}

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// 10. Start HTTP Server
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("🌐 HTTP Server is running")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP Server listen error")
		}
	}()

	// 11. Graceful Shutdown Handler
	<-ctx.Done()
	log.Info().Msg("🛑 Shutting down Alphago MT5 Service gracefully...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP Server forced to shutdown")
	}

	log.Info().Msg("👋 Server exiting successfully.")
}

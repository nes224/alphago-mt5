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

	// 3. Initialize Adapters (MT5 Command TCP & Market Data Stream)
	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)
	defer mt5Adapter.Close()

	streamAddr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5StreamPort)
	streamAdapter := mt5.NewStreamAdapter(streamAddr)
	defer streamAdapter.Close()

	// 3.5 ดึงข้อมูลบัญชีสดจาก MT5 (balance, account type, broker, symbol ที่
	// เทรดได้) — ต้องมี MT5 Terminal + EA เปิด Algo Trading อยู่แล้วตอนนี้ ถ้า
	// ต่อไม่ได้ (เช่นยังไม่ได้ attach EA) จะ fallback ไปใช้ ACCOUNT_BALANCE จาก
	// app.env แทน ไม่ fail การ start ทั้งระบบ
	liveAccountInfo, err := mt5Adapter.GetAccountInfo(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ Failed to fetch live account info from MT5 — falling back to ACCOUNT_BALANCE in app.env (เช็คว่า MT5 Terminal + EA เปิด Algo Trading อยู่ไหม)")
	} else {
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
	}

	// 4. Connect to PostgreSQL — persist account balance / risk guard state /
	// signal history ข้าม restart (internal/adapters/database)
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Database connection failed")
	}
	store := database.NewStore(db)

	// ครั้งแรกที่รัน (ยังไม่เคยบันทึก balance ใน DB) seed ด้วย balance สดจาก
	// MT5 ถ้าดึงได้ ไม่งั้น fallback ไป ACCOUNT_BALANCE ใน app.env — หลังจากนี้
	// DB คือ source of truth เสมอ อัปเดตตาม P/L เหตุการณ์จริง ไม่อ่านจาก MT5
	// สดซ้ำอีก (กัน balance เพี้ยนจาก fee/swap ที่ DB ไม่ได้ track)
	accountBalance, hasSavedBalance, err := store.LoadAccountBalance()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load account balance from DB")
	}
	if !hasSavedBalance {
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
	} else {
		log.Info().Float64("balance", accountBalance).Msg("💾 Loaded account balance from DB")
	}

	// 5. Initialize Core Quant Engine & Strategies
	quantEngine := strategy.NewQuantEngine(BufferCapacity, WindowSize)
	quantEngine.SetSignalCooldown(SignalCooldown)
	quantEngine.SetLongTermWindowSize(LongTermWindowSize)

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

	// 6. Initialize Risk Management & Execution Pipeline
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
	execRouter.AttachOutboxStore(store)

	// เช็ค outbox ที่ค้างจาก run ก่อนหน้า (PENDING = crash ระหว่างตัดสินใจส่งกับ
	// ส่งจริง, UNKNOWN = SendOrder error เช่น timeout/EOF ไม่รู้ว่าเข้าตลาดจริง
	// ไหม) — ไม่ auto-resend เพราะเสี่ยงเปิด position ซ้อน แค่เตือนดังๆ ให้เช็ค
	// มือผ่าน MT5 tab Trade ก่อน (ดูรายละเอียดที่ GET /api/v1/pending-orders)
	if unresolved, err := store.UnresolvedPendingOrders(); err != nil {
		log.Warn().Err(err).Msg("Failed to check unresolved pending orders on startup")
	} else if len(unresolved) > 0 {
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

	// 7. Start Engine & Router Background Workers
	quantEngine.Start(ctx)
	execRouter.Start(ctx)

	// 8. Worker: Dispatch Prepared Orders จาก Engine ส่งไปยัง MT5 ผ่าน TCP Adapter
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

				resp, err := mt5Adapter.SendOrder(ctx, tradeReq)
				if err != nil {
					// Transport error (timeout/EOF/connection reset) — ไม่รู้ว่า order
					// เข้าตลาดจริงไปแล้วหรือเปล่า ไม่ auto-resend และไม่ปลด position
					// lock (เสี่ยงเปิดซ้อนถ้าจริงๆ เข้าไปแล้ว) แค่บันทึกเป็น UNKNOWN ให้
					// เช็คมือผ่าน GET /api/v1/pending-orders
					log.Error().Err(err).Str("symbol", tradeReq.Symbol).Msg("❌ Failed to send order to MT5 — verify manually in MT5 before retrying this symbol")
					if preparedOrder.OutboxID != 0 {
						if uerr := store.MarkOrderOutcome(preparedOrder.OutboxID, database.PendingOrderStatusUnknown, 0, err.Error()); uerr != nil {
							log.Warn().Err(uerr).Msg("Failed to mark outbox row as unknown")
						}
					}
				} else if resp.IsSuccess() {
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
				} else {
					// Broker ปฏิเสธ order ชัดเจน (เช่น invalid stops, margin ไม่พอ) — รู้
					// แน่นอนว่าไม่มี position เปิดจริง ปลด lock ทันทีกัน symbol นี้ค้าง
					// สถานะ "มี position เปิดอยู่" ทั้งที่ไม่มีจริง
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
		}
	}()

	// 9. Stream Ticks Consumer -> Push เข้า PureQuantEngine
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

	// 9.5 Trade Closed Events Consumer -> ป้อนผลแพ้/ชนะจริงกลับเข้า RiskGuard
	// และอัปเดต balance ตาม P/L จริง (ใช้ connection เดียวกับ tick stream —
	// ต้องแก้ EA ให้ยิง OnTradeTransaction() มาด้วย ไม่งั้น channel นี้จะเงียบตลอด)
	tradeClosedChan, err := streamAdapter.SubscribeTradeEvents(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to subscribe trade closed events")
	} else {
		go func() {
			for event := range tradeClosedChan {
				isWin := event.Profit > 0
				riskGuard.RecordTradeResult(isWin)
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

				// Attribute ผลลัพธ์กลับไปหา strategy ต้นเหตุ ถ้าจับคู่ ticket ได้ —
				// persist ลง pending_attributions แล้ว รอดจาก restart ระหว่าง
				// position ยังเปิดค้างอยู่ได้ (ต่างจาก sync.Map เดิม)
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
		}()
	}

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"env":    cfg.AppEnv,
		})
	})

	r = v1.RouterV1(r, mt5Adapter, riskGuard, quantEngine, execRouter, store, TradingSymbol)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// 11. Start HTTP Server
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("🌐 HTTP Server is running")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("HTTP Server listen error")
		}
	}()

	// 12. Graceful Shutdown Handler
	<-ctx.Done()
	log.Info().Msg("🛑 Shutting down Alphago MT5 Service gracefully...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP Server forced to shutdown")
	}

	log.Info().Msg("👋 Server exiting successfully.")
}

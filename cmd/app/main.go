package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nes224/alphago-mt5/internal/adapters/config"
	httphandler "github.com/nes224/alphago-mt5/internal/adapters/http"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
	"github.com/nes224/alphago-mt5/internal/core/services/strategy"
)

const (
	BufferCapacity   = 1000
	WindowSize       = 20
	WorkerCount      = 4
	InitialAccountEq = 10000.0

	MaxDailyLossPercent  = 0.03
	MaxOpenPositions     = 5
	MaxSpreadPips        = 5.0
	MaxConsecutiveLosses = 5
)

func main() {
	// 1. Load Configuration
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Config Error: %v", err)
	}
	fmt.Printf("🚀 Starting Alphago MT5 Service in [%s] mode ...\n", cfg.AppEnv)

	// 2. Master Context & OS Signal Trap (Graceful Shutdown)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 3. Initialize Adapters (MT5 Command TCP & Market Data Stream)
	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)
	defer mt5Adapter.Close()

	streamAddr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	streamAdapter := mt5.NewStreamAdapter(streamAddr)
	defer streamAdapter.Close()

	// 4. Initialize Core Quant Engine & Strategies
	quantEngine := strategy.NewQuantEngine(BufferCapacity, WindowSize)

	// Register OI Expansion Strategy
	oiStrategy := strategy.NewOIExpansionStrategy("OI_EXPANSION_XAUUSD", 2.0, 10.0, 0.2)
	quantEngine.RegisterStrategy(oiStrategy)
	log.Printf("✅ Registered Strategy: %s", oiStrategy.ID())

	// 5. Initialize Risk Management & Execution Pipeline
	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{
		MaxDailyLossPercent:  MaxDailyLossPercent,
		MaxOpenPositions:     MaxOpenPositions,
		MaxSpreadPips:        MaxSpreadPips,
		MaxConsecutiveLosses: MaxConsecutiveLosses,
	}, InitialAccountEq)
	riskManager := risk.NewRiskManager(0.01, InitialAccountEq, 0.01, 1.00)

	orderSink := make(chan risk.PreparedOrder, BufferCapacity)
	execRouter := pipeline.NewExecutionRouter(quantEngine, riskManager, riskGuard, orderSink, WorkerCount)

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

				// [FIX 3] แปลง risk.PreparedOrder ให้เป็น domain.TradeRequest
				tradeReq := domain.TradeRequest{
					Symbol: preparedOrder.Symbol,
					Action: domain.TradeAction(preparedOrder.Action),
					Volume: preparedOrder.LotSize,
					SL:     preparedOrder.StopLoss,
					TP:     preparedOrder.TakeProfit,
				}

				// [FIX 2] รับ Return Value 2 ค่าจาก SendOrder (response, err)
				_, err := mt5Adapter.SendOrder(ctx, tradeReq)
				if err != nil {
					log.Printf("❌ Failed to send order to MT5: %v", err)
				} else {
					log.Printf("🟢 ORDER DISPATCHED TO MT5: %s %s Lot: %.2f",
						tradeReq.Action, tradeReq.Symbol, tradeReq.Volume)
				}
			}
		}
	}()

	// 8. Stream Ticks Consumer -> Push เข้า PureQuantEngine
	tickChan, err := streamAdapter.SubscribeTicks(ctx)
	if err != nil {
		log.Printf("[Warning] Failed to subscribe ticks: %v", err)
	} else {
		go func() {
			log.Println("🔌 Listening to Live Tick Stream...")
			for tick := range tickChan {
				quantEngine.PushTick(tick)
			}
			log.Println("[StreamAdapter] Tick consumer stopped.")
		}()
	}

	// 9. Setup HTTP Services & Handlers (REST API)
	tradeService := services.NewTradeService(mt5Adapter)
	tradeHandler := httphandler.NewTradeHandler(tradeService)

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
	}

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// 10. Start HTTP Server
	go func() {
		fmt.Printf("🌐 HTTP Server is running on port %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP Server listen error: %v", err)
		}
	}()

	// 11. Graceful Shutdown Handler
	<-ctx.Done()
	log.Println("\n🛑 Shutting down Alphago MT5 Service gracefully...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP Server forced to shutdown: %v", err)
	}

	log.Println("👋 Server exiting successfully.")
}

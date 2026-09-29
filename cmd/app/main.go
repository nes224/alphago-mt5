package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nes224/alphago-mt5/internal/adapters/config"
	httphandler "github.com/nes224/alphago-mt5/internal/adapters/http" // Alias เป็น httphandler
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/services"
)

func main() {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Config Error: %v", err)
	}

	fmt.Printf("Starting Alphago MT5 Service in [%s] mode ...\n", cfg.AppEnv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)
	defer mt5Adapter.Close()

	streamAddr := fmt.Sprintf("%s:%d", cfg.MT5Host, 5556)
	streamAdapter := mt5.NewStreamAdapter(streamAddr)
	defer streamAdapter.Close()

	tickChan, err := streamAdapter.SubscribeTicks(ctx)
	if err != nil {
		log.Printf("[Warning] Failed to subscribe ticks: %v", err)
	} else {
		go func() {
			for tick := range tickChan {
				log.Printf("[Tick] Symbol: %s | Bid: %.2f | Ask: %.2f | Time: %v",
					tick.Symbol, tick.Bid, tick.Ask, tick.Timestamp)
			}
			log.Println("[StreamAdapter] Tick consumer stopped.")
		}()
	}

	tradeService := services.NewTradeService(mt5Adapter)
	tradeHandler := httphandler.NewTradeHandler(tradeService)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
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

	go func() {
		fmt.Printf("HTTP Server is running on port %s\n", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP Server listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("Shutting down Alphago MT5 Service gracefully...")

	// ให้เวลา HTTP Server เคลียร์ Request ที่ค้างอยู่ไม่เกิน 5 วินาที
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting successfully.")
}

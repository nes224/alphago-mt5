package main

import (
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

	mt5Addr := fmt.Sprintf("%s:%d", cfg.MT5Host, cfg.MT5Port)
	timeout := time.Duration(cfg.MT5TimeoutSeconds) * time.Second
	mt5Adapter := mt5.NewTCPAdapter(mt5Addr, timeout)
	defer mt5Adapter.Close()

	tradeService := services.NewTradeService(mt5Adapter)
	
	// เปลี่ยนเป็นเรียกผ่าน httphandler
	tradeHandler := httphandler.NewTradeHandler(tradeService)

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.POST("/trade", tradeHandler.ExecuteTrade)
	}

	serverPort := ":8080"
	fmt.Printf("HTTP Server is running on port %s\n", serverPort)
	if err := r.Run(serverPort); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}

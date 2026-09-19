package main

import (
	"context"
	"fmt"
	"log"

	"github.com/nes224/alphago-mt5/internal/adapters/config"
	"github.com/nes224/alphago-mt5/internal/adapters/mt5"
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
)

func main() {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Config Error: %v", err)
	}

	fmt.Printf("Starting App in [%s] mode ...\n", cfg.AppEnv)

	mt5Adapter, err := mt5.NewTCPAdapter(cfg.MT5Host, cfg.MT5Port, cfg.MT5TimeoutSeconds)
	if err != nil {
		log.Fatalf("MT5 Adapter Error: %v", err)
	}

	defer mt5Adapter.Close()

	tradeService := services.NewTradeService(mt5Adapter)

	ctx := context.Background()
	result, err := tradeService.PlaceTrade(ctx, "EURUSD", domain.OrderTypeBuy, 0.01)
	if err != nil {
		log.Fatalf("Trade Execution Failed: %v", err)
	}

	fmt.Printf("Trade Success! Ticket: %d, Message: %s\n", result.Ticket, result.Message)
}

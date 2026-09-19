package services

import (
	"context"
	"fmt"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

type TradeService struct {
	mt5Adapter ports.MT5Port
}

func NewTradeService(mt5Adapter ports.MT5Port) *TradeService {
	return &TradeService{mt5Adapter: mt5Adapter}
}

func (s *TradeService) PlaceTrade(ctx context.Context, symbol string, orderType domain.OrderType, volume float64) (*domain.OrderResult, error) {
	if volume <= 0 {
		return nil, fmt.Errorf("invalid volume: must be greater than 0")
	}

	order := domain.TradeOrder{
		Symbol: symbol,
		Type:   orderType,
		Volume: volume,
	}

	return s.mt5Adapter.ExecuteOrder(ctx, order)
}

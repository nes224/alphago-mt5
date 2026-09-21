package services

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
)

var (
	ErrInvalidVolume      = errors.New("trade volume must be greater than 0")
	ErrInvalidSymbol      = errors.New("symbol cannot be empty")
	ErrInvalidAction      = errors.New("invalid trade action (must be BUY, SELL or CLOSE)")
	ErrRiskGuardTriggered = errors.New("risk guard: trade volume exceeds limit")
)

const MaxAllowedVolume = 10.0

type TradeService struct {
	mt5Port ports.MT5Port
}

func NewTradeService(mt5Port ports.MT5Port) *TradeService {
	return &TradeService{mt5Port: mt5Port}
}

func (s *TradeService) ExecuteTrade(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
	if req.Symbol == "" {
		return nil, fmt.Errorf("validation failed: %w", ErrInvalidSymbol)
	}
	if req.Volume <= 0 {
		return nil, fmt.Errorf("validation failed: %w", ErrInvalidVolume)
	}
	if req.Action != "BUY" && req.Action != "SELL" && req.Action != "CLOSE" {
		return nil, fmt.Errorf("validation failed: %w", ErrInvalidAction)
	}

	if err := s.validateRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if req.Volume > MaxAllowedVolume { 
		return nil, fmt.Errorf("risk check failed: %w", ErrRiskGuardTriggered)
	}

	log.Printf("[TradeService] Executing order: Action=%s, Symbol=%s, Volume=%.2f", req.Action, req.Symbol, req.Volume)

	resp, err := s.mt5Port.SendOrder(ctx, req)
	if err != nil {
		log.Printf("[TradeService] Order execution failed via MT5Port: %v", err)
		return nil, fmt.Errorf("failed to execute order on MT5: %v", &err)
	}

	if !resp.Success {
		log.Printf("[TradeService] Order rejected by MT5: %s", resp.Message)
		return resp, nil
	}

	log.Printf("[TradeService] Order executed successfully! Ticket ID: %d", resp.Ticket)
	return resp, nil

}

func (s *TradeService) validateRequest(req domain.TradeRequest) error {
	if req.Symbol == "" {
		return ErrInvalidSymbol
	}

	if req.Volume <= 0 {
		return ErrInvalidVolume
	}

	if req.Action != "BUY" && req.Action != "SELL" && req.Action != "CLOSE" {
		return ErrInvalidAction
	}

	return nil
}

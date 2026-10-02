package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/ports"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

var (
	ErrInvalidVolume      = errors.New("trade volume must be greater than 0")
	ErrInvalidSymbol      = errors.New("symbol cannot be empty")
	ErrInvalidAction      = errors.New("invalid trade action (must be BUY, SELL or CLOSE)")
	ErrRiskGuardTriggered = errors.New("risk guard: trade volume exceeds limit")
	ErrTicketRequired     = errors.New("ticket ID is required for CLOSE or MODIFY action")
)

const MaxAllowedVolume = 10.0

type TradeService struct {
	mt5Port   ports.MT5Port
	riskGuard *risk.RiskGuard // optional (nil disables the check); shared with the automated ExecutionRouter pipeline
}

func NewTradeService(mt5Port ports.MT5Port, riskGuard *risk.RiskGuard) *TradeService {
	return &TradeService{mt5Port: mt5Port, riskGuard: riskGuard}
}

func (s *TradeService) ExecuteTrade(ctx context.Context, req domain.TradeRequest) (*domain.TradeResponse, error) {
	// 1. Validation Logic
	if err := s.validateRequest(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	if req.Action == domain.ActionBuy || req.Action == domain.ActionSell {
		if req.Volume > MaxAllowedVolume {
			return nil, fmt.Errorf("risk check failed: %w", ErrRiskGuardTriggered)
		}

		if s.riskGuard != nil {
			if err := s.riskGuard.CheckCircuitBreaker(); err != nil {
				return nil, fmt.Errorf("blocked by risk guard: %w", err)
			}
		}
	}

	log.Info().
		Str("action", string(req.Action)).
		Str("symbol", req.Symbol).
		Uint64("ticket", req.Ticket).
		Float64("volume", req.Volume).
		Float64("sl", req.SL).
		Float64("tp", req.TP).
		Msg("[TradeService] Executing order")

	// 3. Send to Port
	resp, err := s.mt5Port.SendOrder(ctx, req)
	if err != nil {
		log.Error().Err(err).Msg("[TradeService] Order execution failed via MT5Port")
		return nil, fmt.Errorf("failed to execute order on MT5: %w", err)
	}

	if !resp.IsSuccess() {
		log.Warn().Str("message", resp.Message).Msg("[TradeService] Order rejected by MT5")
		return resp, nil
	}

	log.Info().Uint64("ticket", resp.Ticket).Msg("[TradeService] Order executed successfully")
	return resp, nil
}

func (s *TradeService) validateRequest(req domain.TradeRequest) error {
	switch req.Action {
	case domain.ActionBuy, domain.ActionSell:
		if req.Symbol == "" {
			return ErrInvalidSymbol
		}
		if req.Volume <= 0 {
			return ErrInvalidVolume
		}
	case domain.ActionClose:
		if req.Ticket == 0 && req.Symbol == "" {
			return ErrTicketRequired
		}
	case domain.ActionModify:
		if req.Ticket == 0 {
			return ErrTicketRequired
		}
	default:
		return ErrInvalidAction
	}
	return nil
}

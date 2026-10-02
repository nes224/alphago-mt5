package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

var ErrRiskConfigNotSet = errors.New("risk config has not been set yet")

// RiskConfigStore is the persistence port RiskConfigService needs — the
// adapter (internal/adapters/database) implements this, RiskConfigService
// never imports the database package directly (Hexagonal Architecture).
type RiskConfigStore interface {
	SaveRiskConfig(cfg domain.RiskConfig) error
	LoadRiskConfig() (domain.RiskConfig, bool, error)
}

// RiskConfigService validates a risk policy change and applies it live to
// RiskManager/RiskGuard (no restart needed) before persisting it, so the
// next restart picks up the same values instead of falling back to app.env.
type RiskConfigService struct {
	store       RiskConfigStore
	riskManager *risk.RiskManager
	riskGuard   *risk.RiskGuard
}

func NewRiskConfigService(store RiskConfigStore, riskManager *risk.RiskManager, riskGuard *risk.RiskGuard) *RiskConfigService {
	return &RiskConfigService{store: store, riskManager: riskManager, riskGuard: riskGuard}
}

// SetRiskConfig validates req, applies it live, then persists it. The live
// apply happens before the DB write deliberately — if persistence fails the
// in-memory values are already correct and a retry of just the write is safe
// (re-apply is idempotent).
func (s *RiskConfigService) SetRiskConfig(ctx context.Context, req domain.RiskConfig) (*domain.RiskConfig, error) {
	if err := validateRiskConfig(req); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	s.riskManager.UpdateConfig(req.RiskPerTradePercent, req.MinLotSize, req.MaxLotSize, req.MinSLDistance, req.MaxSLDistance, req.VolatilityMultiplier, req.ATRMultiplier, req.UseATRForSizing)
	s.riskGuard.UpdateConfig(req.MaxDailyLossPercent, req.MaxOpenPositions, req.MaxSpreadPips)

	if err := s.store.SaveRiskConfig(req); err != nil {
		return nil, fmt.Errorf("failed to persist risk config: %w", err)
	}

	return &req, nil
}

// GetRiskConfig returns the last persisted risk config, or ErrRiskConfigNotSet
// if PUT /api/v1/risk/config has never been called (still running on the
// app.env seed values from startup).
func (s *RiskConfigService) GetRiskConfig(ctx context.Context) (*domain.RiskConfig, error) {
	cfg, ok, err := s.store.LoadRiskConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load risk config: %w", err)
	}
	if !ok {
		return nil, ErrRiskConfigNotSet
	}
	return &cfg, nil
}

// validateRiskConfig enforces sane bounds so a fat-fingered request can't
// silently wreck live risk management (e.g. risk_per_trade_percent=5 would
// mean risking 500% of equity per trade).
func validateRiskConfig(req domain.RiskConfig) error {
	switch {
	case req.RiskPerTradePercent <= 0 || req.RiskPerTradePercent > 1:
		return fmt.Errorf("risk_per_trade_percent must be between 0 and 1 (e.g. 0.01 = 1%%)")
	case req.MinLotSize <= 0:
		return fmt.Errorf("min_lot_size must be greater than 0")
	case req.MaxLotSize < req.MinLotSize:
		return fmt.Errorf("max_lot_size must be >= min_lot_size")
	case req.MinSLDistance <= 0:
		return fmt.Errorf("min_sl_distance must be greater than 0")
	case req.MaxSLDistance < req.MinSLDistance:
		return fmt.Errorf("max_sl_distance must be >= min_sl_distance")
	case req.VolatilityMultiplier <= 0:
		return fmt.Errorf("volatility_multiplier must be greater than 0")
	case req.ATRMultiplier <= 0:
		return fmt.Errorf("atr_multiplier must be greater than 0")
	case req.MaxDailyLossPercent <= 0 || req.MaxDailyLossPercent > 1:
		return fmt.Errorf("max_daily_loss_percent must be between 0 and 1")
	case req.MaxOpenPositions < 1:
		return fmt.Errorf("max_open_positions must be at least 1")
	case req.MaxSpreadPips <= 0:
		return fmt.Errorf("max_spread_pips must be greater than 0")
	}
	return nil
}

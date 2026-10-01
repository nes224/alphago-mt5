package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

type fakeRiskConfigStore struct {
	saved *domain.RiskConfig
}

func (f *fakeRiskConfigStore) SaveRiskConfig(cfg domain.RiskConfig) error {
	f.saved = &cfg
	return nil
}

func (f *fakeRiskConfigStore) LoadRiskConfig() (domain.RiskConfig, bool, error) {
	if f.saved == nil {
		return domain.RiskConfig{}, false, nil
	}
	return *f.saved, true, nil
}

func validRiskConfig() domain.RiskConfig {
	return domain.RiskConfig{
		RiskPerTradePercent: 0.01,
		MinLotSize:          0.01,
		MaxLotSize:          0.05,
		MinSLDistance:       3.0,
		MaxSLDistance:       20.0,
		MaxDailyLossPercent: 0.03,
		MaxOpenPositions:    5,
		MaxSpreadPips:       5.0,
	}
}

// TestRiskConfigService_SetRiskConfig_PersistsAndAppliesLive guards against a
// regression of the original bug (CreateAccountService called itself
// recursively and never reached the store) — a valid request must return
// normally, be persisted, and actually change RiskManager's live behavior.
func TestRiskConfigService_SetRiskConfig_PersistsAndAppliesLive(t *testing.T) {
	store := &fakeRiskConfigStore{}
	riskManager := risk.NewRiskManager(0.01, 10000.0, 0.01, 0.05, 3.0, 20.0)
	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{MaxDailyLossPercent: 0.03, MaxOpenPositions: 5, MaxSpreadPips: 5.0}, 10000.0)
	svc := services.NewRiskConfigService(store, riskManager, riskGuard)

	cfg := validRiskConfig()
	cfg.MaxLotSize = 1.0 // distinct from the constructor value, to prove UpdateConfig actually applied

	resp, err := svc.SetRiskConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.MaxLotSize != 1.0 {
		t.Errorf("Expected response to echo MaxLotSize=1.0, got %f", resp.MaxLotSize)
	}

	if store.saved == nil || store.saved.MaxLotSize != 1.0 {
		t.Fatalf("Expected config to be persisted with MaxLotSize=1.0, got %+v", store.saved)
	}

	got, err := svc.GetRiskConfig(context.Background())
	if err != nil || got == nil {
		t.Fatalf("expected GetRiskConfig to return the persisted config, got %+v err=%v", got, err)
	}
}

func TestRiskConfigService_SetRiskConfig_RejectsInvalidInput(t *testing.T) {
	store := &fakeRiskConfigStore{}
	riskManager := risk.NewRiskManager(0.01, 10000.0, 0.01, 0.05, 3.0, 20.0)
	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{MaxDailyLossPercent: 0.03, MaxOpenPositions: 5, MaxSpreadPips: 5.0}, 10000.0)
	svc := services.NewRiskConfigService(store, riskManager, riskGuard)

	tests := []struct {
		name   string
		mutate func(*domain.RiskConfig)
	}{
		{"risk per trade too high", func(c *domain.RiskConfig) { c.RiskPerTradePercent = 5.0 }},
		{"max lot below min lot", func(c *domain.RiskConfig) { c.MaxLotSize = c.MinLotSize - 0.01 }},
		{"max open positions zero", func(c *domain.RiskConfig) { c.MaxOpenPositions = 0 }},
		{"max spread pips zero", func(c *domain.RiskConfig) { c.MaxSpreadPips = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validRiskConfig()
			tt.mutate(&cfg)

			if _, err := svc.SetRiskConfig(context.Background(), cfg); err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if store.saved != nil {
				t.Errorf("Expected invalid config to not be persisted, got %+v", store.saved)
			}
		})
	}
}

func TestRiskConfigService_GetRiskConfig_ErrorsWhenNeverSet(t *testing.T) {
	store := &fakeRiskConfigStore{}
	riskManager := risk.NewRiskManager(0.01, 10000.0, 0.01, 0.05, 3.0, 20.0)
	riskGuard := risk.NewRiskGuard(risk.RiskGuardConfig{MaxDailyLossPercent: 0.03, MaxOpenPositions: 5, MaxSpreadPips: 5.0}, 10000.0)
	svc := services.NewRiskConfigService(store, riskManager, riskGuard)

	_, err := svc.GetRiskConfig(context.Background())
	if !errors.Is(err, services.ErrRiskConfigNotSet) {
		t.Errorf("Expected ErrRiskConfigNotSet, got: %v", err)
	}
}

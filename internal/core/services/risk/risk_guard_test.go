package risk_test

import (
	"testing"

	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

func TestRiskGuard_DailyDrawdown_CircuitBreaker(t *testing.T) {
	cfg := risk.RiskGuardConfig{
		MaxDailyLossPercent: 0.03, // 3%
		MaxOpenPositions:    5,
		MaxSpreadPips:       1.0,
	}

	guard := risk.NewRiskGuard(cfg, 10000.0) // Start Equity 10,000 USD

	order := risk.PreparedOrder{Symbol: "XAUUSDm", Action: domain.SignalAction(domain.ActionBuy)}
	metrics := domain.TickMetrics{Spread: 0.30}

	// 1. สภาวะปกติ: ควรยิงผ่าน
	if err := guard.ValidateOrder(order, metrics); err != nil {
		t.Fatalf("Expected order to pass, got error: %v", err)
	}

	// 2. Equity ร่วงเหลือ 9,600 USD (ลดลง 4% > เกณฑ์ 3%)
	guard.UpdateAccountEquity(9600.0)

	// 3. ต้องโดน Block ด้วย ErrDailyDrawdownExceeded
	err := guard.ValidateOrder(order, metrics)
	if err == nil {
		t.Fatal("Expected order to be blocked by Daily Drawdown, but passed")
	}
}

func TestRiskGuard_SpreadFilter(t *testing.T) {
	cfg := risk.RiskGuardConfig{
		MaxDailyLossPercent: 0.05,
		MaxOpenPositions:    5,
		MaxSpreadPips:       0.50, // Spread ห้ามเกิน $0.50
	}

	guard := risk.NewRiskGuard(cfg, 10000.0)
	order := risk.PreparedOrder{Symbol: "XAUUSDm"}
	metrics := domain.TickMetrics{Spread: 0.80} // Spread กว้างเกินไป (0.80)

	err := guard.ValidateOrder(order, metrics)
	if err == nil {
		t.Fatal("Expected order to be blocked due to high spread, but passed")
	}
}

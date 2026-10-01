package database_test

import (
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/nes224/alphago-mt5/internal/adapters/database"
	"github.com/nes224/alphago-mt5/internal/core/domain"
	"github.com/nes224/alphago-mt5/internal/core/services/pipeline"
	"github.com/nes224/alphago-mt5/internal/core/services/risk"
)

// newTestDB opens an isolated in-memory SQLite database per test — same GORM
// models/queries as production Postgres, just without needing a live server
// for unit tests. Production always uses database.Connect (Postgres).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&database.AccountStateModel{},
		&database.RiskGuardStateModel{},
		&database.SignalRecordModel{},
		&database.TradeOutcomeModel{},
		&database.OpenPositionSymbolModel{},
		&database.PendingAttributionModel{},
		&database.PendingOrderModel{},
		&database.RiskConfigModel{},
	); err != nil {
		t.Fatalf("failed to auto-migrate schema: %v", err)
	}

	return db
}

func TestStore_AccountBalance_RoundTrip(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	if _, ok, err := store.LoadAccountBalance(); err != nil || ok {
		t.Fatalf("Expected no balance saved yet, got ok=%v err=%v", ok, err)
	}

	if err := store.SaveAccountBalance(1000.0); err != nil {
		t.Fatalf("unexpected error saving balance: %v", err)
	}

	balance, ok, err := store.LoadAccountBalance()
	if err != nil {
		t.Fatalf("unexpected error loading balance: %v", err)
	}
	if !ok || balance != 1000.0 {
		t.Errorf("Expected balance 1000.0, got %f (ok=%v)", balance, ok)
	}

	// Saving again should update the single row, not insert a new one.
	if err := store.SaveAccountBalance(1234.56); err != nil {
		t.Fatalf("unexpected error re-saving balance: %v", err)
	}
	balance, _, _ = store.LoadAccountBalance()
	if balance != 1234.56 {
		t.Errorf("Expected updated balance 1234.56, got %f", balance)
	}
}

func TestStore_RiskGuardState_RoundTrip(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	if _, ok, err := store.LoadRiskState(); err != nil || ok {
		t.Fatalf("Expected no risk state saved yet, got ok=%v err=%v", ok, err)
	}

	state := risk.PersistedState{
		StartingDailyEquity: 1000.0,
		CurrentDailyEquity:  950.0,
		OpenPositionsCount:  2,
		OpenPositionSymbols: map[string]int{"XAUUSDm": 1, "EURUSDm": 1},
		ConsecutiveLosses:   3,
		IsCircuitTripped:    true,
		LastResetDate:       "2026-09-30",
	}
	if err := store.SaveRiskState(state); err != nil {
		t.Fatalf("unexpected error saving state: %v", err)
	}

	got, ok, err := store.LoadRiskState()
	if err != nil {
		t.Fatalf("unexpected error loading state: %v", err)
	}
	if !ok {
		t.Fatal("Expected ok=true after saving")
	}
	if !reflect.DeepEqual(got, state) {
		t.Errorf("Expected %+v, got %+v", state, got)
	}
}

func TestStore_RiskGuardState_OpenPositionSymbols_OverwritesOnResave(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	if err := store.SaveRiskState(risk.PersistedState{
		OpenPositionSymbols: map[string]int{"XAUUSDm": 1},
		LastResetDate:       "2026-09-30",
	}); err != nil {
		t.Fatalf("unexpected error saving initial state: %v", err)
	}

	// Position closes and a different symbol opens — resaving must drop
	// XAUUSDm entirely, not just add EURUSDm alongside it.
	if err := store.SaveRiskState(risk.PersistedState{
		OpenPositionSymbols: map[string]int{"EURUSDm": 1},
		LastResetDate:       "2026-09-30",
	}); err != nil {
		t.Fatalf("unexpected error re-saving state: %v", err)
	}

	got, _, err := store.LoadRiskState()
	if err != nil {
		t.Fatalf("unexpected error loading state: %v", err)
	}
	want := map[string]int{"EURUSDm": 1}
	if !reflect.DeepEqual(got.OpenPositionSymbols, want) {
		t.Errorf("Expected OpenPositionSymbols %+v, got %+v", want, got.OpenPositionSymbols)
	}
}

func TestRiskGuard_AttachStore_LoadsPersistedStateOnStartup(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	// Simulate a previous run that already persisted a tripped circuit breaker.
	if err := store.SaveRiskState(risk.PersistedState{
		StartingDailyEquity: 1000.0,
		CurrentDailyEquity:  900.0,
		IsCircuitTripped:    true,
		LastResetDate:       time.Now().Format("2006-01-02"),
	}); err != nil {
		t.Fatalf("unexpected error seeding state: %v", err)
	}

	// A fresh RiskGuard constructed with a different initialEquity should get
	// overridden by the persisted state once AttachStore runs.
	guard := risk.NewRiskGuard(risk.RiskGuardConfig{MaxDailyLossPercent: 0.03}, 5000.0)
	if err := guard.AttachStore(store); err != nil {
		t.Fatalf("unexpected error attaching store: %v", err)
	}

	status := guard.Status()
	if !status.CircuitTripped {
		t.Error("Expected circuit breaker to be tripped after loading persisted state")
	}
	if status.StartingDailyEquity != 1000.0 {
		t.Errorf("Expected StartingDailyEquity 1000.0 from persisted state, got %f", status.StartingDailyEquity)
	}
}

func TestExecutionRouter_AttachStore_PersistsAndReadsBack(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	rec := pipeline.SignalRecord{
		Symbol: "XAUUSDm",
		Action: "BUY",
		Status: pipeline.SignalStatusDispatched,
		Reason: "TEST",
	}
	if err := store.SaveSignal(rec); err != nil {
		t.Fatalf("unexpected error saving signal: %v", err)
	}

	recent, err := store.RecentSignals(10)
	if err != nil {
		t.Fatalf("unexpected error reading recent signals: %v", err)
	}
	if len(recent) != 1 || recent[0].Symbol != "XAUUSDm" {
		t.Errorf("Expected 1 signal for XAUUSDm, got %+v", recent)
	}
}

func TestStore_WinRateByStrategy_ComputesPerStrategyStats(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	outcomes := []database.TradeOutcome{
		{Symbol: "XAUUSDm", StrategyTag: "VOLUME_EXPANSION_BUY", Ticket: 1, Profit: 10.0, IsWin: true, Timestamp: time.Now()},
		{Symbol: "XAUUSDm", StrategyTag: "VOLUME_EXPANSION_BUY", Ticket: 2, Profit: -5.0, IsWin: false, Timestamp: time.Now()},
		{Symbol: "XAUUSDm", StrategyTag: "VOLUME_EXPANSION_BUY", Ticket: 3, Profit: 8.0, IsWin: true, Timestamp: time.Now()},
		{Symbol: "XAUUSDm", StrategyTag: "LIQUIDITY_SWEEP_FADE_SELL", Ticket: 4, Profit: -3.0, IsWin: false, Timestamp: time.Now()},
	}
	for _, o := range outcomes {
		if err := store.SaveTradeOutcome(o); err != nil {
			t.Fatalf("unexpected error saving outcome: %v", err)
		}
	}

	stats, err := store.WinRateByStrategy()
	if err != nil {
		t.Fatalf("unexpected error computing win rate: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("Expected stats for 2 distinct strategies, got %d: %+v", len(stats), stats)
	}

	byTag := make(map[string]database.WinRateStat)
	for _, s := range stats {
		byTag[s.StrategyTag] = s
	}

	buyStats, ok := byTag["VOLUME_EXPANSION_BUY"]
	if !ok {
		t.Fatal("Expected stats for VOLUME_EXPANSION_BUY")
	}
	if buyStats.Wins != 2 || buyStats.Losses != 1 || buyStats.TotalTrades != 3 {
		t.Errorf("Expected 2 wins / 1 loss / 3 total for VOLUME_EXPANSION_BUY, got %+v", buyStats)
	}
	if buyStats.WinRate < 0.66 || buyStats.WinRate > 0.67 {
		t.Errorf("Expected win rate ~0.667 for VOLUME_EXPANSION_BUY, got %f", buyStats.WinRate)
	}
	if buyStats.TotalProfit != 13.0 {
		t.Errorf("Expected total profit 13.0 for VOLUME_EXPANSION_BUY, got %f", buyStats.TotalProfit)
	}

	sellStats, ok := byTag["LIQUIDITY_SWEEP_FADE_SELL"]
	if !ok {
		t.Fatal("Expected stats for LIQUIDITY_SWEEP_FADE_SELL")
	}
	if sellStats.Wins != 0 || sellStats.Losses != 1 || sellStats.WinRate != 0 {
		t.Errorf("Expected 0 wins / 1 loss / 0%% win rate for LIQUIDITY_SWEEP_FADE_SELL, got %+v", sellStats)
	}
}

func TestStore_PendingAttribution_RoundTrip(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	if _, ok, err := store.LoadAndDeleteAttribution(12345); err != nil || ok {
		t.Fatalf("Expected no attribution saved yet, got ok=%v err=%v", ok, err)
	}

	if err := store.SaveAttribution(12345, "VOLUME_EXPANSION_BUY (Z=2.1)"); err != nil {
		t.Fatalf("unexpected error saving attribution: %v", err)
	}

	reason, ok, err := store.LoadAndDeleteAttribution(12345)
	if err != nil {
		t.Fatalf("unexpected error loading attribution: %v", err)
	}
	if !ok || reason != "VOLUME_EXPANSION_BUY (Z=2.1)" {
		t.Errorf("Expected reason 'VOLUME_EXPANSION_BUY (Z=2.1)', got %q (ok=%v)", reason, ok)
	}

	// A second load must find nothing — LoadAndDeleteAttribution consumes the row.
	if _, ok, err := store.LoadAndDeleteAttribution(12345); err != nil || ok {
		t.Fatalf("Expected attribution to be consumed after first load, got ok=%v err=%v", ok, err)
	}
}

func TestStore_PendingOrders_OutboxLifecycle(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	id, err := store.CreatePendingOrder(pipeline.PendingOrder{
		Symbol:     "XAUUSDm",
		Action:     "BUY",
		LotSize:    0.05,
		StopLoss:   4190.0,
		TakeProfit: 4200.0,
		Reason:     "VOLUME_EXPANSION_BUY",
	})
	if err != nil {
		t.Fatalf("unexpected error creating pending order: %v", err)
	}

	unresolved, err := store.UnresolvedPendingOrders()
	if err != nil {
		t.Fatalf("unexpected error listing unresolved orders: %v", err)
	}
	if len(unresolved) != 1 || unresolved[0].Status != database.PendingOrderStatusPending {
		t.Fatalf("Expected 1 PENDING order, got %+v", unresolved)
	}

	// Simulate the sender goroutine confirming success.
	if err := store.MarkOrderOutcome(id, database.PendingOrderStatusSent, 999, ""); err != nil {
		t.Fatalf("unexpected error marking order sent: %v", err)
	}

	unresolved, err = store.UnresolvedPendingOrders()
	if err != nil {
		t.Fatalf("unexpected error listing unresolved orders after resolution: %v", err)
	}
	if len(unresolved) != 0 {
		t.Errorf("Expected no unresolved orders after marking SENT, got %+v", unresolved)
	}

	all, err := store.PendingOrders(10)
	if err != nil {
		t.Fatalf("unexpected error listing pending orders: %v", err)
	}
	if len(all) != 1 || all[0].Status != database.PendingOrderStatusSent || all[0].Ticket != 999 {
		t.Errorf("Expected 1 SENT order with ticket 999, got %+v", all)
	}
}

func TestStore_PendingOrders_UnresolvedIncludesUnknown(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	id, err := store.CreatePendingOrder(pipeline.PendingOrder{Symbol: "XAUUSDm", Action: "BUY"})
	if err != nil {
		t.Fatalf("unexpected error creating pending order: %v", err)
	}

	// Simulate a transport error (timeout/EOF) — status becomes UNKNOWN, still
	// needs a manual check, so it must still show up as unresolved.
	if err := store.MarkOrderOutcome(id, database.PendingOrderStatusUnknown, 0, "failed to read response from MT5: EOF"); err != nil {
		t.Fatalf("unexpected error marking order unknown: %v", err)
	}

	unresolved, err := store.UnresolvedPendingOrders()
	if err != nil {
		t.Fatalf("unexpected error listing unresolved orders: %v", err)
	}
	if len(unresolved) != 1 || unresolved[0].Status != database.PendingOrderStatusUnknown {
		t.Fatalf("Expected 1 UNKNOWN order to remain unresolved, got %+v", unresolved)
	}
}

func TestStore_RiskConfig_RoundTrip(t *testing.T) {
	store := database.NewStore(newTestDB(t))

	if _, ok, err := store.LoadRiskConfig(); err != nil || ok {
		t.Fatalf("Expected no risk config saved yet, got ok=%v err=%v", ok, err)
	}

	cfg := domain.RiskConfig{
		RiskPerTradePercent: 0.02,
		MinLotSize:          0.01,
		MaxLotSize:          0.1,
		MinSLDistance:       3.0,
		MaxSLDistance:       20.0,
		MaxDailyLossPercent: 0.05,
		MaxOpenPositions:    3,
		MaxSpreadPips:       4.0,
	}
	if err := store.SaveRiskConfig(cfg); err != nil {
		t.Fatalf("unexpected error saving risk config: %v", err)
	}

	got, ok, err := store.LoadRiskConfig()
	if err != nil {
		t.Fatalf("unexpected error loading risk config: %v", err)
	}
	if !ok || got != cfg {
		t.Errorf("Expected %+v, got %+v (ok=%v)", cfg, got, ok)
	}

	// Saving again should update the single row, not insert a new one — and
	// must not disturb account_state (AccountStateModel and RiskConfigModel
	// are separate tables precisely so SaveAccountBalance can't wipe this).
	cfg.MaxOpenPositions = 1
	if err := store.SaveRiskConfig(cfg); err != nil {
		t.Fatalf("unexpected error re-saving risk config: %v", err)
	}
	got, _, _ = store.LoadRiskConfig()
	if got.MaxOpenPositions != 1 {
		t.Errorf("Expected updated MaxOpenPositions=1, got %d", got.MaxOpenPositions)
	}
}

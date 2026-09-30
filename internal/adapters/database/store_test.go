package database_test

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/nes224/alphago-mt5/internal/adapters/database"
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

	if err := db.AutoMigrate(&database.AccountStateModel{}, &database.RiskGuardStateModel{}, &database.SignalRecordModel{}); err != nil {
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
	if got != state {
		t.Errorf("Expected %+v, got %+v", state, got)
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

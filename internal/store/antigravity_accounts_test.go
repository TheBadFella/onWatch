package store

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

// Two Antigravity accounts keep snapshots, ranges and reset cycles separate.
func TestAntigravityAccounts_IsolateSnapshotsRangesAndCycles(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	work, err := s.CreateOrRestoreProviderAccount("antigravity", "work")
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	personal, err := s.CreateOrRestoreProviderAccount("antigravity", "personal")
	if err != nil {
		t.Fatalf("create personal: %v", err)
	}

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{
		CapturedAt: now,
		Email:      "work@example.com",
		Models:     []api.AntigravityModelQuota{{ModelID: "claude-4-5-sonnet", RemainingFraction: 0.50}},
		Source:     api.AntigravitySourceIDE,
	}, work.ID); err != nil {
		t.Fatalf("insert work: %v", err)
	}
	if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{
		CapturedAt: now.Add(time.Minute),
		Email:      "personal@example.com",
		Models:     []api.AntigravityModelQuota{{ModelID: "gemini-3-pro", RemainingFraction: 0.80}},
		Source:     api.AntigravitySourceIDE,
	}, personal.ID); err != nil {
		t.Fatalf("insert personal: %v", err)
	}

	latestWork, err := s.QueryLatestAntigravity(work.ID)
	if err != nil || latestWork == nil {
		t.Fatalf("latest work = %#v, %v", latestWork, err)
	}
	if latestWork.Email != "work@example.com" {
		t.Fatalf("work email = %q", latestWork.Email)
	}
	latestPersonal, err := s.QueryLatestAntigravity(personal.ID)
	if err != nil || latestPersonal == nil {
		t.Fatalf("latest personal = %#v, %v", latestPersonal, err)
	}
	if latestPersonal.Email != "personal@example.com" {
		t.Fatalf("personal email = %q", latestPersonal.Email)
	}

	rangeWork, err := s.QueryAntigravityRangeSampled(now.Add(-time.Hour), now.Add(time.Hour), work.ID)
	if err != nil {
		t.Fatalf("range work: %v", err)
	}
	if len(rangeWork) != 1 {
		t.Fatalf("work range = %d rows, want 1", len(rangeWork))
	}
	workCycle, err := s.CreateAntigravityCycle("claude-4-5-sonnet", now, nil, work.ID)
	if err != nil {
		t.Fatalf("create work cycle: %v", err)
	}
	personalCycle, err := s.CreateAntigravityCycle("claude-4-5-sonnet", now, nil, personal.ID)
	if err != nil {
		t.Fatalf("create personal cycle (same model must be allowed per account): %v", err)
	}
	if workCycle == personalCycle {
		t.Fatal("cycle IDs collided across accounts")
	}
	activeWork, err := s.QueryActiveAntigravityCycle("claude-4-5-sonnet", work.ID)
	if err != nil || activeWork == nil {
		t.Fatalf("active work = %#v, %v", activeWork, err)
	}
	activePersonal, err := s.QueryActiveAntigravityCycle("claude-4-5-sonnet", personal.ID)
	if err != nil || activePersonal == nil {
		t.Fatalf("active personal = %#v, %v", activePersonal, err)
	}

	// Closing one account cycle must not close the other.
	if err := s.CloseAntigravityCycle("claude-4-5-sonnet", now.Add(30*time.Minute), 0.5, 0.2, work.ID); err != nil {
		t.Fatalf("close work cycle: %v", err)
	}
	if active, _ := s.QueryActiveAntigravityCycle("claude-4-5-sonnet", work.ID); active != nil {
		t.Fatal("work cycle still active after close")
	}
	if active, _ := s.QueryActiveAntigravityCycle("claude-4-5-sonnet", personal.ID); active == nil {
		t.Fatal("personal cycle closed by the other account")
	}
	if history, err := s.QueryAntigravityCycleHistoryForAccount(work.ID, "claude-4-5-sonnet"); err != nil || len(history) != 1 {
		t.Fatalf("work history = %d, %v; want 1", len(history), err)
	}
	if history, err := s.QueryAntigravityCycleHistoryForAccount(personal.ID, "claude-4-5-sonnet"); err != nil || len(history) != 0 {
		t.Fatalf("personal history = %d, %v; want 0", len(history), err)
	}
}

// Legacy single-account rows are attributed to the default account, and the
// migration is idempotent.
func TestMigrateSchema_AddsAntigravityAccountAndBackfillsLegacyHistory(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	for _, stmt := range []string{
		`DROP TABLE antigravity_model_values`,
		`DROP TABLE antigravity_snapshots`,
		`DROP TABLE antigravity_reset_cycles`,
		`CREATE TABLE antigravity_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			captured_at TEXT NOT NULL,
			email TEXT,
			plan_name TEXT,
			prompt_credits REAL,
			monthly_credits INTEGER,
			raw_json TEXT,
			model_count INTEGER DEFAULT 0,
			source TEXT NOT NULL DEFAULT 'unknown'
		)`,
		`CREATE TABLE antigravity_model_values (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			snapshot_id INTEGER NOT NULL,
			model_id TEXT NOT NULL,
			label TEXT,
			remaining_fraction REAL NOT NULL DEFAULT 0,
			remaining_percent REAL NOT NULL DEFAULT 0,
			is_exhausted INTEGER NOT NULL DEFAULT 0,
			reset_time TEXT
		)`,
		`CREATE TABLE antigravity_reset_cycles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id TEXT NOT NULL,
			cycle_start TEXT NOT NULL,
			cycle_end TEXT,
			reset_time TEXT,
			peak_usage REAL NOT NULL DEFAULT 0,
			total_delta REAL NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX idx_antigravity_cycles_model_active_unique ON antigravity_reset_cycles(model_id) WHERE cycle_end IS NULL`,
	} {
		if _, err := s.db.Exec(stmt); err != nil {
			t.Fatalf("setup %q: %v", stmt, err)
		}
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`INSERT INTO antigravity_snapshots (captured_at, email, source) VALUES (?, 'legacy@example.com', 'ide')`, now); err != nil {
		t.Fatalf("insert legacy snapshot: %v", err)
	}
	if _, err := s.db.Exec(`INSERT INTO antigravity_reset_cycles (model_id, cycle_start) VALUES ('legacy-model', ?)`, now); err != nil {
		t.Fatalf("insert legacy cycle: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := s.createTables(); err != nil {
			t.Fatalf("migrateSchema run %d: %v", i+1, err)
		}
	}

	for _, table := range []string{"antigravity_snapshots", "antigravity_reset_cycles"} {
		has, err := s.tableHasColumn(table, "account_id")
		if err != nil {
			t.Fatalf("tableHasColumn(%s): %v", table, err)
		}
		if !has {
			t.Fatalf("migration did not add %s.account_id", table)
		}
	}
	defaultID := s.DefaultAntigravityAccountID()
	if defaultID <= 0 {
		t.Fatal("default Antigravity account was not created")
	}
	for _, table := range []string{"antigravity_snapshots", "antigravity_reset_cycles"} {
		var accountID int64
		if err := s.db.QueryRow(`SELECT account_id FROM ` + table + ` LIMIT 1`).Scan(&accountID); err != nil {
			t.Fatalf("read %s account_id: %v", table, err)
		}
		if accountID != defaultID {
			t.Fatalf("%s account_id = %d, want the default %d", table, accountID, defaultID)
		}
	}
}

// A legacy caller that omits the account writes to the default account, so a
// single existing account keeps working unchanged.
func TestInsertAntigravitySnapshot_LegacyCallUsesDefaultAccount(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	defaultID := s.DefaultAntigravityAccountID()
	if defaultID <= 0 {
		t.Fatal("default Antigravity account missing")
	}
	if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{
		CapturedAt: time.Now().UTC(),
		Email:      "legacy@example.com",
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var accountID int64
	if err := s.db.QueryRow(`SELECT account_id FROM antigravity_snapshots LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatalf("read account_id: %v", err)
	}
	if accountID != defaultID {
		t.Fatalf("account_id = %d, want default %d", accountID, defaultID)
	}
	latest, err := s.QueryLatestAntigravity()
	if err != nil || latest == nil || latest.Email != "legacy@example.com" {
		t.Fatalf("legacy latest = %#v, %v", latest, err)
	}
}

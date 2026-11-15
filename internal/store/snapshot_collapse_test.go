package store

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

func snapshotTimes(t *testing.T, s *Store, query string, args ...any) []time.Time {
	t.Helper()
	rows, err := s.db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []time.Time
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		ts, _ := time.Parse(time.RFC3339Nano, v)
		out = append(out, ts)
	}
	return out
}

// Polls that repeat the previous reading keep only the first and newest row of
// the run, so flat periods stop growing the database while the chart line, the
// latest poll time and the last reading before a change all stay exact.
func TestCodexRepeatedSnapshotsCollapseToRunEnds(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	reset := base.Add(3 * time.Hour)
	insert := func(minute int, account int64, util float64) {
		t.Helper()
		if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{
			CapturedAt: base.Add(time.Duration(minute) * time.Minute),
			AccountID:  account,
			PlanType:   "pro",
			Quotas:     []api.CodexQuota{{Name: "five_hour", Utilization: util, ResetsAt: &reset}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	for m := 0; m < 5; m++ {
		insert(m, 6, 20) // run of five identical readings
		insert(m, 9, 50) // interleaved account must not break account 6's run
	}
	insert(5, 6, 0) // reset observed

	got := snapshotTimes(t, s, `SELECT captured_at FROM codex_snapshots WHERE account_id = 6 ORDER BY id`)
	want := []time.Time{base, base.Add(4 * time.Minute), base.Add(5 * time.Minute)}
	if len(got) != len(want) {
		t.Fatalf("account 6 rows at %v, want %v", got, want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("account 6 rows at %v, want %v", got, want)
		}
	}
	if n := len(snapshotTimes(t, s, `SELECT captured_at FROM codex_snapshots WHERE account_id = 9`)); n != 2 {
		t.Fatalf("account 9 kept %d rows, want first and newest", n)
	}
	var orphans int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM codex_quota_values WHERE snapshot_id NOT IN (SELECT id FROM codex_snapshots)`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("orphaned quota values: %d, err %v", orphans, err)
	}
	latest, err := s.QueryLatestCodex(6)
	if err != nil || latest == nil || latest.Quotas[0].Utilization != 0 {
		t.Fatalf("latest = %+v, err %v", latest, err)
	}
}

// A change in any stored field, not just utilization, starts a new run.
func TestSnapshotCollapseComparesEveryStoredField(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	r1, r2 := base.Add(time.Hour), base.Add(2*time.Hour)
	for i, rt := range []*time.Time{&r1, &r1, &r2} {
		if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{
			CapturedAt: base.Add(time.Duration(i) * time.Minute),
			Models:     []api.AntigravityModelQuota{{ModelID: "gemini", RemainingFraction: 0.5, ResetTime: rt}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(snapshotTimes(t, s, `SELECT captured_at FROM antigravity_snapshots`)); n != 3 {
		t.Fatalf("kept %d rows, want 3: the reset time changed", n)
	}
}

func TestRepeatedSnapshotsCollapseForOtherProviders(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	for m := 0; m < 4; m++ {
		at := base.Add(time.Duration(m) * time.Minute)
		if _, err := s.InsertAnthropicSnapshot(&api.AnthropicSnapshot{CapturedAt: at, Quotas: []api.AnthropicQuota{{Name: "five_hour", Utilization: 10}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.InsertCommandCodeSnapshot(&api.CommandCodeSnapshot{CapturedAt: at, AccountID: "acct", Quotas: []api.CommandCodeQuota{{Name: "weekly", Used: 3, Limit: 10}}}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{CapturedAt: at, Models: []api.AntigravityModelQuota{{ModelID: "gemini", RemainingFraction: 0.5}}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"anthropic_snapshots", "commandcode_snapshots", "antigravity_snapshots"} {
		if n := len(snapshotTimes(t, s, `SELECT captured_at FROM `+table)); n != 2 {
			t.Fatalf("%s kept %d rows, want 2", table, n)
		}
	}
}

// History stored before collapsing existed gets the same first-and-last rule
// once, per account, without touching runs that changed.
func TestCollapseExistingRunsBackfillsOnce(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	base := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	insert := func(minute int, account int64, util float64) {
		t.Helper()
		res, err := s.db.Exec(`INSERT INTO codex_snapshots (captured_at, account_id, plan_type, raw_json, quota_count) VALUES (?, ?, 'pro', '', 1)`,
			base.Add(time.Duration(minute)*time.Minute).Format(time.RFC3339Nano), account)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if _, err := s.db.Exec(`INSERT INTO codex_quota_values (snapshot_id, quota_name, utilization) VALUES (?, 'five_hour', ?)`, id, util); err != nil {
			t.Fatal(err)
		}
	}
	for m, u := range []float64{10, 10, 10, 10, 20, 20, 10} {
		insert(m, 6, u)
		insert(m, 9, 50)
	}
	if err := s.SetSetting(snapshotRunsCollapsedKey, ""); err != nil {
		t.Fatal(err)
	}
	removed, err := s.collapseExistingRuns()
	if err != nil {
		t.Fatal(err)
	}
	// Account 6 keeps minutes 0,3 (run of 10), 4,5 (run of 20), 6; account 9 keeps 0 and 6.
	if removed != 2+5 {
		t.Fatalf("removed %d, want 7", removed)
	}
	got := snapshotTimes(t, s, `SELECT captured_at FROM codex_snapshots WHERE account_id = 6 ORDER BY id`)
	for i, m := range []int{0, 3, 4, 5, 6} {
		if i >= len(got) || !got[i].Equal(base.Add(time.Duration(m)*time.Minute)) {
			t.Fatalf("account 6 kept %v", got)
		}
	}
	var orphans int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM codex_quota_values WHERE snapshot_id NOT IN (SELECT id FROM codex_snapshots)`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("orphaned values %d, err %v", orphans, err)
	}
	if again, err := s.collapseExistingRuns(); err != nil || again != 0 {
		t.Fatalf("second run removed %d, err %v", again, err)
	}
}

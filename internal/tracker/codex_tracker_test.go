package tracker

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/notify"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func newTestCodexStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCodexTracker_Process_FirstSnapshot(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)
	snap := &api.CodexSnapshot{
		CapturedAt: now,
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 22.5, ResetsAt: &reset, Status: "healthy"},
		},
	}

	if err := tr.Process(snap); err != nil {
		t.Fatalf("Process: %v", err)
	}

	cycle, err := s.QueryActiveCodexCycle(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryActiveCodexCycle: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected active cycle")
	}
	if cycle.PeakUtilization != 22.5 {
		t.Fatalf("PeakUtilization = %.1f, want 22.5", cycle.PeakUtilization)
	}
}

func TestCodexTracker_Process_UsageIncrease(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)

	snap1 := &api.CodexSnapshot{
		CapturedAt: now,
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 20, ResetsAt: &reset, Status: "healthy"},
		},
	}
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	snap2 := &api.CodexSnapshot{
		CapturedAt: now.Add(time.Minute),
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 35, ResetsAt: &reset, Status: "healthy"},
		},
	}
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	cycle, err := s.QueryActiveCodexCycle(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryActiveCodexCycle: %v", err)
	}
	if cycle == nil {
		t.Fatal("expected active cycle")
	}
	if cycle.PeakUtilization != 35 {
		t.Fatalf("PeakUtilization = %.1f, want 35", cycle.PeakUtilization)
	}
	if cycle.TotalDelta != 15 {
		t.Fatalf("TotalDelta = %.1f, want 15", cycle.TotalDelta)
	}
}

func TestCodexTracker_Process_ResetDetection(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	resetDetected := false
	tr.SetOnReset(func(string) {
		resetDetected = true
	})

	now := time.Now().UTC()
	reset1 := now.Add(5 * time.Hour)
	reset2 := now.Add(7 * 24 * time.Hour)

	snap1 := &api.CodexSnapshot{
		CapturedAt: now,
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 45, ResetsAt: &reset1, Status: "warning"},
		},
	}
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	snap2 := &api.CodexSnapshot{
		CapturedAt: now.Add(time.Minute),
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 5, ResetsAt: &reset2, Status: "healthy"},
		},
	}
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	if !resetDetected {
		t.Fatal("expected reset callback")
	}

	history, err := s.QueryCodexCycleHistory(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryCodexCycleHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("len(history) = %d, want 1", len(history))
	}

	active, err := s.QueryActiveCodexCycle(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryActiveCodexCycle: %v", err)
	}
	if active == nil {
		t.Fatal("expected new active cycle")
	}
	if active.PeakUtilization != 5 {
		t.Fatalf("active.PeakUtilization = %.1f, want 5", active.PeakUtilization)
	}
}

func TestCodexTracker_Process_ResetTimestampDrift_DoesNotReset(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	now := time.Now().UTC()
	reset1 := now.Add(5 * time.Hour)
	reset2 := reset1.Add(10 * time.Minute)
	reset3 := reset1.Add(75 * time.Minute)

	snap1 := &api.CodexSnapshot{
		CapturedAt: now,
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 40, ResetsAt: &reset1, Status: "warning"},
		},
	}
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	snap2 := &api.CodexSnapshot{
		CapturedAt: now.Add(10 * time.Minute),
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 41, ResetsAt: &reset2, Status: "warning"},
		},
	}
	if err := tr.Process(snap2); err != nil {
		t.Fatalf("Process snap2: %v", err)
	}

	snap3 := &api.CodexSnapshot{
		CapturedAt: now.Add(75 * time.Minute),
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 41.5, ResetsAt: &reset3, Status: "warning"},
		},
	}
	if err := tr.Process(snap3); err != nil {
		t.Fatalf("Process snap3: %v", err)
	}

	history, err := s.QueryCodexCycleHistory(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryCodexCycleHistory: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("expected no completed cycles, got %d", len(history))
	}

	active, err := s.QueryActiveCodexCycle(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryActiveCodexCycle: %v", err)
	}
	if active == nil {
		t.Fatal("expected active cycle")
	}
	if active.CycleStart.Unix() != now.Unix() {
		t.Fatalf("expected original cycle start, got %v", active.CycleStart)
	}
	if active.ResetsAt == nil {
		t.Fatal("expected active reset timestamp to be tracked")
	}
	if !active.ResetsAt.Equal(reset3) {
		t.Fatalf("active.ResetsAt = %v, want %v", active.ResetsAt, reset3)
	}
}

func TestCodexTracker_UsageSummary(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)
	snap := &api.CodexSnapshot{
		CapturedAt: now,
		PlanType:   "pro",
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 28, ResetsAt: &reset, Status: "healthy"},
		},
	}

	if _, err := s.InsertCodexSnapshot(snap); err != nil {
		t.Fatalf("InsertCodexSnapshot: %v", err)
	}
	if err := tr.Process(snap); err != nil {
		t.Fatalf("Process: %v", err)
	}

	summary, err := tr.UsageSummary(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected summary")
	}
	if summary.CurrentUtil != 28 {
		t.Fatalf("CurrentUtil = %.1f, want 28", summary.CurrentUtil)
	}
	if summary.ResetsAt == nil {
		t.Fatal("expected ResetsAt")
	}
}

func TestCodexTracker_Process_ExistingCycleAfterRestart_UpdatesPeakWithoutDelta(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	base := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	reset := base.Add(3 * time.Hour)
	if _, err := s.CreateCodexCycle(store.DefaultCodexAccountID, "five_hour", base, &reset); err != nil {
		t.Fatalf("CreateCodexCycle: %v", err)
	}
	if err := s.UpdateCodexCycle(store.DefaultCodexAccountID, "five_hour", 10, 4); err != nil {
		t.Fatalf("UpdateCodexCycle: %v", err)
	}

	quota := api.CodexQuota{Name: "five_hour", Utilization: 30, ResetsAt: &reset, Status: "healthy"}
	if err := tr.processQuota(store.DefaultCodexAccountID, quota, base.Add(10*time.Minute)); err != nil {
		t.Fatalf("processQuota: %v", err)
	}

	cycle, err := s.QueryActiveCodexCycle(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("QueryActiveCodexCycle: %v", err)
	}
	if cycle.PeakUtilization != 30 {
		t.Fatalf("PeakUtilization = %.1f, want 30", cycle.PeakUtilization)
	}
	if cycle.TotalDelta != 4 {
		t.Fatalf("TotalDelta = %.1f, want 4", cycle.TotalDelta)
	}
}

func TestCodexTracker_UsageSummary_UsesCycleResetWhenLatestQuotaMissing(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	base := time.Date(2026, 3, 4, 11, 0, 0, 0, time.UTC)
	resetPrimary := base.Add(4 * time.Hour)
	resetOther := base.Add(2 * time.Hour)

	snap1 := &api.CodexSnapshot{
		CapturedAt: base,
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 25, ResetsAt: &resetPrimary, Status: "healthy"},
		},
	}
	if _, err := s.InsertCodexSnapshot(snap1); err != nil {
		t.Fatalf("InsertCodexSnapshot snap1: %v", err)
	}
	if err := tr.Process(snap1); err != nil {
		t.Fatalf("Process snap1: %v", err)
	}

	snap2 := &api.CodexSnapshot{
		CapturedAt: base.Add(2 * time.Minute),
		Quotas: []api.CodexQuota{
			{Name: "other", Utilization: 10, ResetsAt: &resetOther, Status: "healthy"},
		},
	}
	if _, err := s.InsertCodexSnapshot(snap2); err != nil {
		t.Fatalf("InsertCodexSnapshot snap2: %v", err)
	}

	summary, err := tr.UsageSummary(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if summary.ResetsAt == nil {
		t.Fatal("expected ResetsAt from active cycle")
	}
	if !summary.ResetsAt.Equal(resetPrimary) {
		t.Fatalf("ResetsAt = %v, want %v", summary.ResetsAt, resetPrimary)
	}
	if summary.CurrentUtil != 0 {
		t.Fatalf("CurrentUtil = %.1f, want 0 when quota missing in latest snapshot", summary.CurrentUtil)
	}
}

func TestCodexTracker_UsageSummary_CalculatesRateAndClampsProjectedUtil(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	tr := NewCodexTracker(s, slog.Default())

	cycleStart := time.Now().UTC().Add(-2 * time.Hour)
	resetAt := time.Now().UTC().Add(2 * time.Hour)
	if _, err := s.CreateCodexCycle(store.DefaultCodexAccountID, "five_hour", cycleStart, &resetAt); err != nil {
		t.Fatalf("CreateCodexCycle: %v", err)
	}
	if err := s.UpdateCodexCycle(store.DefaultCodexAccountID, "five_hour", 95, 40); err != nil {
		t.Fatalf("UpdateCodexCycle: %v", err)
	}

	snap := &api.CodexSnapshot{
		CapturedAt: time.Now().UTC(),
		Quotas: []api.CodexQuota{
			{Name: "five_hour", Utilization: 90, ResetsAt: &resetAt, Status: "warning"},
		},
	}
	if _, err := s.InsertCodexSnapshot(snap); err != nil {
		t.Fatalf("InsertCodexSnapshot: %v", err)
	}

	summary, err := tr.UsageSummary(store.DefaultCodexAccountID, "five_hour")
	if err != nil {
		t.Fatalf("UsageSummary: %v", err)
	}
	if summary.CurrentRate <= 0 {
		t.Fatalf("CurrentRate = %v, want > 0", summary.CurrentRate)
	}
	if summary.ProjectedUtil != 100 {
		t.Fatalf("ProjectedUtil = %v, want 100 (clamped)", summary.ProjectedUtil)
	}
}

// A reset must release only that account's threshold dedupe entry. Previously
// the application wired an accountless reset and all non-default entries stuck.
func TestCodexTracker_AccountResetRearmsWebhook(t *testing.T) {
	t.Parallel()
	s := newTestCodexStore(t)
	var deliveries atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload notify.WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload.AccountID != "6" && payload.AccountID != "9" {
			t.Errorf("lost account: %q", payload.AccountID)
		}
		if payload.Event != notify.EventCritical {
			t.Errorf("event = %q", payload.Event)
		}
		deliveries.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer endpoint.Close()
	raw, _ := json.Marshal(notify.WebhookConfig{URL: endpoint.URL, Events: notify.WebhookEvents{Critical: true}})
	if err := s.SetSetting("webhook", string(raw)); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("notifications", `{"critical_threshold":95,"notify_critical":true,"channels":{"webhook":true}}`); err != nil {
		t.Fatal(err)
	}
	engine := notify.New(s, slog.Default())
	if err := engine.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := engine.ConfigureWebhook(); err != nil {
		t.Fatal(err)
	}
	tr := NewCodexTracker(s, slog.Default())
	tr.SetOnAccountReset(func(accountID int64, name string) {
		engine.Check(notify.QuotaStatus{Provider: "codex", AccountID: fmt.Sprint(accountID), QuotaKey: name, ResetOccurred: true})
	})
	now := time.Now().UTC()
	reset := now.Add(5 * time.Hour)
	poll := func(account int64, value float64, at time.Time, deadline time.Time) {
		t.Helper()
		if err := tr.Process(&api.CodexSnapshot{AccountID: account, CapturedAt: at, Quotas: []api.CodexQuota{{Name: "five_hour", Utilization: value, ResetsAt: &deadline}}}); err != nil {
			t.Fatal(err)
		}
		engine.Check(notify.QuotaStatus{Provider: "codex", AccountID: fmt.Sprint(account), QuotaKey: "five_hour", Utilization: value})
	}
	poll(6, 98, now, reset)
	poll(9, 98, now, reset)
	poll(6, 99, now.Add(time.Minute), reset)
	if got := deliveries.Load(); got != 2 {
		t.Fatalf("initial deliveries = %d, want 2", got)
	}
	poll(6, 1, reset.Add(time.Minute), reset.Add(5*time.Hour))
	poll(6, 98, reset.Add(2*time.Minute), reset.Add(5*time.Hour))
	poll(9, 99, reset.Add(2*time.Minute), reset)
	if got := deliveries.Load(); got != 3 {
		t.Fatalf("after account 6 reset deliveries = %d, want 3", got)
	}
}

package web

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

func TestCodexReadinessRequiresBothWindowsForPaidPlans(t *testing.T) {
	now := time.Now()
	short, weekly := now.Add(time.Hour), now.Add(72*time.Hour)
	snapshot := &api.CodexSnapshot{CapturedAt: now.Add(-time.Minute), PlanType: "plus", Quotas: []api.CodexQuota{
		{Name: "five_hour", Utilization: 20, ResetsAt: &short},
		{Name: "seven_day", Utilization: 100, ResetsAt: &weekly},
	}}
	got := codexReadiness(snapshot, now, 5*time.Minute, false, true)
	if got.State != "limited" || got.Binding != "seven_day" || got.ResetAt == nil || !got.ResetAt.Equal(weekly) {
		t.Fatalf("unexpected blocked readiness: %+v", got)
	}
	snapshot.Quotas[0].Utilization = 100
	if got := codexReadiness(snapshot, now, 5*time.Minute, false, true); got.Binding != "seven_day" {
		t.Fatalf("both limits exhausted: %+v", got)
	}
	snapshot.Quotas[0].Utilization = 20
	snapshot.Quotas[1].Utilization = 50
	if got := codexReadiness(snapshot, now, 5*time.Minute, false, true); got.State != "ready" || got.Binding != "seven_day" {
		t.Fatalf("unexpected ready state: %+v", got)
	}
	snapshot.Quotas = snapshot.Quotas[:1]
	if got := codexReadiness(snapshot, now, 5*time.Minute, false, true); got.State != "unknown" {
		t.Fatalf("missing weekly window: %+v", got)
	}
	if got := codexReadiness(snapshot, now.Add(6*time.Minute), 5*time.Minute, false, true); got.State != "stale" {
		t.Fatalf("aged out reading: %+v", got)
	}
	if got := codexReadiness(snapshot, now, 5*time.Minute, true, true); got.State != "reconnect" {
		t.Fatalf("paused account: %+v", got)
	}
	if got := codexReadiness(snapshot, now, 5*time.Minute, false, false); got.State != "disabled" {
		t.Fatalf("disabled polling: %+v", got)
	}
}

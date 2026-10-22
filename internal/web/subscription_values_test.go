package web

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func TestSubscriptionCycleMonthEnds(t *testing.T) {
	for _, tc := range []struct {
		now        string
		day        int
		start, end string
	}{
		{"2026-02-15", 31, "2026-01-31", "2026-02-28"},
		{"2026-02-28", 31, "2026-02-28", "2026-03-31"},
		{"2024-02-29", 31, "2024-02-29", "2024-03-31"},
		{"2026-01-02", 10, "2025-12-10", "2026-01-10"},
		{"2026-10-22", 0, "2026-10-01", "2026-11-01"},
	} {
		now, _ := time.Parse("2006-01-02", tc.now)
		start, end := subscriptionCycle(now, tc.day)
		if start.Format("2006-01-02") != tc.start || end.Format("2006-01-02") != tc.end {
			t.Fatalf("%+v: %v %v", tc, start, end)
		}
	}
}
func TestSubscriptionQuotaAllocation(t *testing.T) {
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	reset := now.Add(time.Hour)
	cad := 30.0
	value := quotaAllocation(api.CodexQuota{Name: "seven_day", Utilization: 50, ResetsAt: &reset}, 20, &cad, 30*24*time.Hour, now)
	if value == nil || math.Abs(value.UsedUSD-7.0/3) > 1e-9 || math.Abs(*value.UsedCAD-3.5) > 1e-9 || math.Abs(*value.RemainingCAD-3.5) > 1e-9 {
		t.Fatalf("wrong weekly allocation: %+v", value)
	}
	value = quotaAllocation(api.CodexQuota{Name: "five_hour", Utilization: 0}, 20, &cad, 30*24*time.Hour, now)
	if value == nil || value.UsedUSD != 0 || *value.UsedCAD != 0 || value.RemainingUSD != value.WindowUSD {
		t.Fatal("known zero was lost")
	}
	for _, q := range []api.CodexQuota{{Name: "unknown", Utilization: 10}, {Name: "five_hour", Utilization: math.NaN()}, {Name: "seven_day", Utilization: -1}, {Name: "seven_day", Utilization: 101}, {Name: "five_hour", Utilization: 20, ResetsAt: &now}} {
		if quotaAllocation(q, 20, &cad, 30*24*time.Hour, now) != nil {
			t.Fatalf("invalid or expired window was valued: %+v", q)
		}
	}
}
func TestSubscriptionValuesWithoutTokenTelemetry(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	account, err := s.CreateOrRestoreProviderAccount("codex", "quota-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	reset := now.Add(time.Hour)
	snapshot := &api.CodexSnapshot{AccountID: account.ID, CapturedAt: now, Quotas: []api.CodexQuota{{Name: "five_hour", Utilization: 50, ResetsAt: &reset}, {Name: "seven_day", Utilization: 75, ResetsAt: &reset}}}
	if _, err := s.InsertCodexSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	config := costSettings{CADPerUSD: costNumber(1.4), Subscriptions: []costSubscription{{Provider: "openai", Account: "quota-test", MonthlyUSD: costNumber(20), MonthlyCAD: costNumber(30), BillingDay: 1}}}
	raw, _ := json.Marshal(config)
	if err := s.SetSetting("costs", string(raw)); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.Costs(rr, httptest.NewRequest("GET", "/api/costs", nil))
	var result struct {
		MonthlyUSD float64             `json:"subscription_monthly_usd"`
		AnnualUSD  float64             `json:"subscription_annual_usd"`
		MonthlyCAD float64             `json:"subscription_monthly_cad"`
		AnnualCAD  float64             `json:"subscription_annual_cad"`
		Values     []subscriptionValue `json:"subscription_values"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.MonthlyUSD != 20 || result.AnnualUSD != 240 || result.MonthlyCAD != 30 || result.AnnualCAD != 360 || len(result.Values) != 1 || len(result.Values[0].Windows) != 2 {
		t.Fatal(rr.Body)
	}
	if result.Values[0].CADSource != "Entered CAD fee" {
		t.Fatal("actual CAD fee was replaced by FX conversion")
	}
	future := h.subscriptionValues(config, nil, now.Add(16*time.Minute))
	if len(future[0].Windows) != 0 || !strings.Contains(future[0].Reason, "stale") {
		t.Fatal("stale reading became current value")
	}
	config.Subscriptions[0].Account = "different-account"
	missing := h.subscriptionValues(config, nil, now)
	if len(missing[0].Windows) != 0 || missing[0].Reason == "" {
		t.Fatal("reading leaked across accounts")
	}
	config.Subscriptions[0].MonthlyCAD = nil
	converted := h.subscriptionValues(config, &costExchange{CADPerUSD: 1.5, Source: "Configured rate"}, now)
	if *converted[0].MonthlyCAD != 30 {
		t.Fatal("USD/CAD conversion incorrect")
	}
}
func TestSubscriptionCostValidation(t *testing.T) {
	config := costSettings{Subscriptions: []costSubscription{{Provider: "codex", Account: "a", MonthlyUSD: costNumber(20), BillingDay: 32}}}
	if config.validate() == nil {
		t.Fatal("invalid billing day accepted")
	}
	config.Subscriptions[0].BillingDay = 1
	config.Subscriptions[0].MonthlyCAD = costNumber(-1)
	if config.validate() == nil {
		t.Fatal("negative CAD fee accepted")
	}
	config.Subscriptions[0].MonthlyCAD = nil
	config.CADPerUSD = costNumber(0)
	if config.validate() == nil {
		t.Fatal("zero FX rate accepted")
	}
	config.CADPerUSD = costNumber(1.5)
	if config.validate() != nil {
		t.Fatal("valid subscription rejected")
	}
	config.Subscriptions = append(config.Subscriptions, costSubscription{Provider: "openai", Account: "a", MonthlyUSD: costNumber(20)})
	if config.validate() == nil {
		t.Fatal("provider aliases double-counted the same account")
	}
}

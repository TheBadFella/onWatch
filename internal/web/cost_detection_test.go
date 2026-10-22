package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func TestCostAutomaticPlansAndOverrides(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	now := time.Now().UTC()
	for _, name := range []string{"personal", "work", "custom"} {
		a, err := s.CreateOrRestoreProviderAccount("codex", name)
		if err != nil {
			t.Fatal(err)
		}
		plan := map[string]string{"personal": "plus", "work": "business", "custom": "enterprise"}[name]
		reset := now.Add(time.Hour)
		if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{AccountID: a.ID, CapturedAt: now, PlanType: plan, Quotas: []api.CodexQuota{{Name: "seven_day", Utilization: 50, ResetsAt: &reset}}}); err != nil {
			t.Fatal(err)
		}
	}
	// No saved subscriptions, fees, model rates or token telemetry.
	if err := s.SetSetting("costs", `{"cad_per_usd":1.5,"subscriptions":[],"rates":[]}`); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	h.Costs(rr, httptest.NewRequest("GET", "/api/costs", nil))
	var got struct {
		Monthly  float64                    `json:"subscription_monthly_usd"`
		Maximum  float64                    `json:"subscription_monthly_max_usd"`
		CAD      float64                    `json:"subscription_monthly_cad"`
		MaxCAD   float64                    `json:"subscription_monthly_max_cad"`
		Currency string                     `json:"display_currency"`
		Values   []subscriptionValue        `json:"subscription_values"`
		Unknown  []detectedCostSubscription `json:"unpriced_subscriptions"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Monthly != 40 || got.Maximum != 145 || got.CAD != 60 || got.MaxCAD != 217.5 || got.Currency != "CAD" || len(got.Values) != 2 || len(got.Unknown) != 1 {
		t.Fatal(rr.Body)
	}
	for _, value := range got.Values {
		if len(value.Windows) != 1 {
			t.Fatal("automatic plan has no quota value", value)
		}
	}
	if len(h.loadCostSettings().Subscriptions) != 0 {
		t.Fatal("detected prices became saved overrides")
	}
	c := costSettings{Subscriptions: []costSubscription{{Provider: "openai", Account: "personal", MonthlyUSD: costNumber(15), MonthlyCAD: costNumber(22)}}}
	effective, unknown := h.effectiveCostSettings(c)
	if len(effective.Subscriptions) != 2 || len(unknown) != 1 || *effective.Subscriptions[0].MonthlyUSD != 15 || effective.Subscriptions[0].PriceSource != "Saved price override" {
		t.Fatal(effective, unknown)
	}
	// A provider-reported plan change must update the estimate without editing settings.
	a, err := s.CreateOrRestoreProviderAccount("codex", "personal")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{AccountID: a.ID, CapturedAt: now.Add(time.Second), PlanType: "free"}); err != nil {
		t.Fatal(err)
	}
	updated, _ := h.effectiveCostSettings(costSettings{})
	for _, item := range updated.Subscriptions {
		if item.Account == "personal" && *item.MonthlyUSD != 0 {
			t.Fatal("plan change ignored")
		}
	}
}

func TestCostPlanCatalogUnknownIsNotFree(t *testing.T) {
	for _, plan := range []string{"", "enterprise", "edu", "future-plan"} {
		if _, _, _, ok := codexSubscriptionPrice(plan); ok {
			t.Fatalf("unknown plan priced: %q", plan)
		}
	}
	for _, tc := range []struct {
		plan      string
		low, high float64
	}{{"free", 0, 0}, {" PLUS ", 20, 20}, {"pro", 100, 500}, {"prolite", 100, 100}, {"promax", 500, 500}, {"team", 20, 125}} {
		low, high, _, ok := codexSubscriptionPrice(tc.plan)
		if !ok || low != tc.low || high != tc.high {
			t.Fatal(tc, low, high, ok)
		}
	}
}

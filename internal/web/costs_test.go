package web

import (
	"encoding/json"
	apiintegrations "github.com/onllm-dev/onwatch/v2/internal/api_integrations"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func costNumber(n float64) *float64 { return &n }
func TestCostPricing(t *testing.T) {
	cfg := costSettings{Rates: []costRate{{Provider: "openai", Model: "test", Input: costNumber(1), Cached: costNumber(.1), Output: costNumber(2)}}}
	rows := []store.CostUsage{{Provider: "openai", Account: "personal", Model: "test", Requests: 1, Input: 1000, Cached: 200, Output: 50}, {Provider: "openai", Model: "unknown", Requests: 2}, {Provider: "openai", Model: "test", Requests: 3, Input: 1000, UnknownCache: 3}}
	result, total, unpriced := priceCostUsage(rows, cfg)
	if math.Abs(total-.00092) > 1e-12 || unpriced != 5 || result[1].EstimatedUSD != nil || result[2].EstimatedUSD != nil {
		t.Fatalf("wrong pricing: %+v %v %v", result, total, unpriced)
	}
	cfg.Rates[0].Cached = costNumber(1)
	_, total, unpriced = priceCostUsage(rows, cfg)
	if math.Abs(total-.0021) > 1e-12 || unpriced != 2 {
		t.Fatal("cache-neutral rates should not require cache counts")
	}
	cfg.Rates = append(cfg.Rates, cfg.Rates[0])
	if cfg.validate() == nil {
		t.Fatal("duplicate rate accepted")
	}
	cfg.Rates = cfg.Rates[:1]
	cfg.Rates[0].Input = nil
	if cfg.validate() == nil {
		t.Fatal("missing price became free")
	}
	cfg.Rates[0].Input = costNumber(-1)
	if cfg.validate() == nil {
		t.Fatal("negative price accepted")
	}
}
func TestCostsSettingsAndMonthScope(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"costs":{"subscriptions":[{"provider":"openai","account":"personal","monthly_usd":20}],"rates":[{"provider":"openai","model":"test","input_per_million":1,"cached_input_per_million":0.1,"output_per_million":2}]}}`)))
	if rr.Code != 200 {
		t.Fatal(rr.Body)
	}
	now := time.Now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	for i, stamp := range []time.Time{now.Add(-time.Second), month.Add(-time.Second), now.Add(time.Hour)} {
		event, err := apiintegrations.ParseUsageEventLine([]byte(`{"ts":"`+stamp.Format(time.RFC3339)+`","integration":"cost-test","provider":"openai","account":"personal","model":"test","prompt_tokens":1000,"completion_tokens":50,"metadata":{"cached_prompt_tokens":200}}`), "/fixture/cost.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		event.RequestID = string(rune('a' + i))
		if _, err := s.InsertAPIIntegrationUsageEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	rr = httptest.NewRecorder()
	h.Costs(rr, httptest.NewRequest(http.MethodGet, "/api/costs", nil))
	var got struct {
		Monthly float64           `json:"subscription_monthly_usd"`
		API     float64           `json:"estimated_api_usd"`
		Usage   []pricedCostUsage `json:"usage"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Monthly != 20 || math.Abs(got.API-.00092) > 1e-12 || len(got.Usage) != 1 || got.Usage[0].Requests != 1 {
		t.Fatalf("wrong scoped totals: %s", rr.Body)
	}
	rr = httptest.NewRecorder()
	h.GetSettings(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if !strings.Contains(rr.Body.String(), `"monthly_usd":20`) {
		t.Fatal("settings did not round trip")
	}
}

func TestCostMonthBoundaryAndCacheMetadata(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i, metadata := range []string{`{"cached_prompt_tokens":0}`, `{}`, `{"cached_prompt_tokens":2000}`, `{"cached_prompt_tokens":"200"}`} {
		line := `{"ts":"2026-10-01T00:00:00.123Z","integration":"boundary","provider":"openai","account":"` + string(rune('a'+i)) + `","model":"test","prompt_tokens":1000,"completion_tokens":50,"metadata":` + metadata + `}`
		event, err := apiintegrations.ParseUsageEventLine([]byte(line), "/fixture/boundary.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.InsertAPIIntegrationUsageEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.QueryCostUsage(start, start.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].UnknownCache != 0 || rows[1].UnknownCache != 1 || rows[2].UnknownCache != 1 || rows[3].UnknownCache != 1 {
		t.Fatalf("month boundary or cache validation failed: %+v", rows)
	}
}

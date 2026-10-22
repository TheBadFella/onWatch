package web

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

type costSubscription struct {
	Provider      string   `json:"provider"`
	Account       string   `json:"account"`
	MonthlyUSD    *float64 `json:"monthly_usd"`
	MonthlyCAD    *float64 `json:"monthly_cad,omitempty"`
	BillingDay    int      `json:"billing_day,omitempty"`
	MonthlyMaxUSD *float64 `json:"monthly_max_usd,omitempty"`
	PriceSource   string   `json:"price_source,omitempty"`
}
type costRate struct {
	Provider string   `json:"provider"`
	Model    string   `json:"model"`
	Input    *float64 `json:"input_per_million"`
	Cached   *float64 `json:"cached_input_per_million"`
	Output   *float64 `json:"output_per_million"`
}
type costSettings struct {
	CADPerUSD     *float64           `json:"cad_per_usd,omitempty"`
	Subscriptions []costSubscription `json:"subscriptions"`
	Rates         []costRate         `json:"rates"`
}

func validCostNumber(n *float64) bool {
	return n != nil && !math.IsNaN(*n) && !math.IsInf(*n, 0) && *n >= 0 && *n <= 1000000
}
func (c costSettings) validate() error {
	if c.CADPerUSD != nil && (!validCostNumber(c.CADPerUSD) || *c.CADPerUSD <= 0 || *c.CADPerUSD > 100) {
		return fmt.Errorf("CAD per USD must be greater than 0 and at most 100")
	}
	if len(c.Subscriptions) > 100 || len(c.Rates) > 100 {
		return fmt.Errorf("at most 100 subscriptions and model rates are supported")
	}
	seen := map[string]bool{}
	for _, s := range c.Subscriptions {
		if s.MonthlyCAD != nil && !validCostNumber(s.MonthlyCAD) {
			return fmt.Errorf("monthly CAD fee must be between 0 and 1000000")
		}
		if s.BillingDay < 0 || s.BillingDay > 31 {
			return fmt.Errorf("billing day must be between 1 and 31")
		}
		key := costProvider(s.Provider) + "\x00" + s.Account
		if strings.TrimSpace(s.Provider) == "" || strings.TrimSpace(s.Account) == "" || len(s.Provider) > 64 || len(s.Account) > 256 || !validCostNumber(s.MonthlyUSD) || seen[key] {
			return fmt.Errorf("each subscription needs a unique provider/account and a monthly USD fee from 0 to 1000000")
		}
		seen[key] = true
	}
	seen = map[string]bool{}
	for _, r := range c.Rates {
		key := r.Provider + "\x00" + r.Model
		if strings.TrimSpace(r.Provider) == "" || strings.TrimSpace(r.Model) == "" || len(r.Provider) > 64 || len(r.Model) > 256 || !validCostNumber(r.Input) || !validCostNumber(r.Cached) || !validCostNumber(r.Output) || seen[key] {
			return fmt.Errorf("each rate needs a unique provider/model and input, cached input and output USD prices from 0 to 1000000")
		}
		seen[key] = true
	}
	return nil
}
func (h *Handler) loadCostSettings() costSettings {
	c := costSettings{Subscriptions: []costSubscription{}, Rates: []costRate{}}
	if h.store != nil {
		raw, _ := h.store.GetSetting("costs")
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &c)
		}
	}
	return c
}

type pricedCostUsage struct {
	store.CostUsage
	EstimatedUSD *float64 `json:"estimated_usd"`
	Reason       string   `json:"reason,omitempty"`
}

func priceCostUsage(rows []store.CostUsage, c costSettings) ([]pricedCostUsage, float64, int64) {
	rates := map[string]costRate{}
	for _, r := range c.Rates {
		rates[r.Provider+"\x00"+r.Model] = r
	}
	result := []pricedCostUsage{}
	total := 0.0
	var unpriced int64
	for _, row := range rows {
		item := pricedCostUsage{CostUsage: row}
		rate, ok := rates[row.Provider+"\x00"+row.Model]
		if !ok {
			item.Reason = "Model rate not configured"
		} else if !validCostNumber(rate.Input) || !validCostNumber(rate.Cached) || !validCostNumber(rate.Output) {
			item.Reason = "Model rate is invalid"
		} else if row.UnknownCache > 0 && *rate.Input != *rate.Cached {
			item.Reason = "Cache token counts not reported"
		} else {
			cost := (float64(row.Input-row.Cached)**rate.Input + float64(row.Cached)**rate.Cached + float64(row.Output)**rate.Output) / 1000000
			item.EstimatedUSD = &cost
			total += cost
		}
		if item.EstimatedUSD == nil {
			unpriced += row.Requests
		}
		result = append(result, item)
	}
	return result, total, unpriced
}
func (h *Handler) Costs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	rows, err := h.store.QueryCostUsage(start, now)
	if err != nil {
		respondError(w, 500, "could not load cost telemetry")
		return
	}
	truncated := len(rows) > 500
	if truncated {
		rows = rows[:500]
	}
	settings := h.loadCostSettings()
	settings, unpricedSubscriptions := h.effectiveCostSettings(settings)
	items, total, unpriced := priceCostUsage(rows, settings)
	fx := h.costExchange(r.Context(), settings, now)
	plans := h.subscriptionValues(settings, fx, now)
	monthly, annual, daily := 0.0, 0.0, 0.0
	monthlyMax, monthlyMaxCAD := 0.0, 0.0
	monthlyCAD, annualCAD, dailyCAD := 0.0, 0.0, 0.0
	cadComplete := true
	for _, plan := range plans {
		monthly += plan.MonthlyUSD
		maximum := plan.MonthlyUSD
		if plan.MonthlyMaxUSD != nil {
			maximum = *plan.MonthlyMaxUSD
		}
		monthlyMax += maximum
		annual += plan.AnnualUSD
		daily += plan.DailyUSD
		if plan.MonthlyCAD == nil {
			cadComplete = false
		} else {
			monthlyCAD += *plan.MonthlyCAD
			if plan.MonthlyMaxCAD != nil {
				monthlyMaxCAD += *plan.MonthlyMaxCAD
			} else {
				monthlyMaxCAD += *plan.MonthlyCAD
			}
			annualCAD += *plan.AnnualCAD
			dailyCAD += *plan.DailyCAD
		}
	}
	result := map[string]interface{}{"month_start": start, "as_of": now, "subscriptions": settings.Subscriptions, "subscription_values": plans, "subscription_monthly_usd": monthly, "subscription_annual_usd": annual, "subscription_daily_usd": daily, "estimated_api_usd": total, "usage": items, "unpriced_requests": unpriced, "truncated": truncated, "exchange": fx}
	result["unpriced_subscriptions"] = unpricedSubscriptions
	result["subscription_monthly_max_usd"] = monthlyMax
	result["subscription_annual_max_usd"] = monthlyMax * 12
	result["display_currency"] = "CAD"
	result["price_catalog_date"] = "2026-10-22"
	if cadComplete && len(plans) > 0 {
		result["subscription_monthly_cad"] = monthlyCAD
		result["subscription_annual_cad"] = annualCAD
		result["subscription_daily_cad"] = dailyCAD
		result["subscription_monthly_max_cad"] = monthlyMaxCAD
		result["subscription_annual_max_cad"] = monthlyMaxCAD * 12
	}
	if fx != nil {
		result["estimated_api_cad"] = total * fx.CADPerUSD
	}
	respondJSON(w, 200, result)
}

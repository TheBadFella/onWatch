package web

import (
	"math"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

type costAccount struct {
	Provider string `json:"provider"`
	Account  string `json:"account"`
}

func (h *Handler) costAccounts() []costAccount {
	result := []costAccount{}
	if h.store == nil {
		return result
	}
	for _, account := range h.detectedCostSubscriptions() {
		result = append(result, costAccount{Provider: account.Provider, Account: account.Account})
	}
	return result
}

type subscriptionValue struct {
	Provider      string       `json:"provider"`
	Account       string       `json:"account"`
	AccountID     int64        `json:"account_id,omitempty"`
	MonthlyUSD    float64      `json:"monthly_usd"`
	MonthlyMaxUSD *float64     `json:"monthly_max_usd,omitempty"`
	MonthlyMaxCAD *float64     `json:"monthly_max_cad,omitempty"`
	PriceSource   string       `json:"price_source,omitempty"`
	AnnualUSD     float64      `json:"annual_usd"`
	DailyUSD      float64      `json:"daily_usd"`
	MonthlyCAD    *float64     `json:"monthly_cad"`
	AnnualCAD     *float64     `json:"annual_cad"`
	DailyCAD      *float64     `json:"daily_cad"`
	CADSource     string       `json:"cad_source,omitempty"`
	CycleStart    time.Time    `json:"cycle_start"`
	CycleEnd      time.Time    `json:"cycle_end"`
	ObservedAt    *time.Time   `json:"observed_at,omitempty"`
	Reason        string       `json:"reason,omitempty"`
	Windows       []quotaValue `json:"windows"`
	Weekly        *quotaValue  `json:"weekly,omitempty"`
}
type quotaValue struct {
	Key          string     `json:"key"`
	UsedPercent  float64    `json:"used_percent"`
	WindowUSD    float64    `json:"window_usd"`
	UsedUSD      float64    `json:"used_usd"`
	RemainingUSD float64    `json:"remaining_usd"`
	WindowCAD    *float64   `json:"window_cad"`
	UsedCAD      *float64   `json:"used_cad"`
	RemainingCAD *float64   `json:"remaining_cad"`
	ResetsAt     *time.Time `json:"resets_at,omitempty"`
}

func subscriptionCycle(now time.Time, day int) (time.Time, time.Time) {
	if day == 0 {
		day = 1
	}
	month := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	anchor := func(base time.Time) time.Time {
		last := time.Date(base.Year(), base.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		return time.Date(base.Year(), base.Month(), min(day, last), 0, 0, 0, 0, time.UTC)
	}
	if now.Before(anchor(month)) {
		month = month.AddDate(0, -1, 0)
	}
	return anchor(month), anchor(month.AddDate(0, 1, 0))
}
func quotaAllocation(q api.CodexQuota, monthly float64, cad *float64, cycle time.Duration, now time.Time) *quotaValue {
	var duration time.Duration
	switch q.Name {
	case "five_hour":
		duration = 5 * time.Hour
	case "seven_day":
		duration = 7 * 24 * time.Hour
	default:
		return nil
	}
	if math.IsNaN(q.Utilization) || math.IsInf(q.Utilization, 0) || q.Utilization < 0 || q.Utilization > 100 || cycle <= 0 {
		return nil
	}
	if q.ResetsAt != nil && !q.ResetsAt.After(now) {
		return nil
	}
	value := quotaValue{Key: q.Name, UsedPercent: q.Utilization, ResetsAt: q.ResetsAt}
	fraction := float64(duration) / float64(cycle)
	value.WindowUSD = monthly * fraction
	value.UsedUSD = value.WindowUSD * q.Utilization / 100
	value.RemainingUSD = value.WindowUSD - value.UsedUSD
	if cad != nil {
		budget := *cad * fraction
		used := budget * q.Utilization / 100
		remaining := budget - used
		value.WindowCAD = &budget
		value.UsedCAD = &used
		value.RemainingCAD = &remaining
	}
	return &value
}
func (h *Handler) subscriptionValues(c costSettings, fx *costExchange, now time.Time) []subscriptionValue {
	result := []subscriptionValue{}
	accounts, _ := h.store.QueryActiveProviderAccounts("codex")
	for _, s := range c.Subscriptions {
		if !validCostNumber(s.MonthlyUSD) {
			continue
		}
		start, end := subscriptionCycle(now, s.BillingDay)
		item := subscriptionValue{Provider: s.Provider, Account: s.Account, MonthlyUSD: *s.MonthlyUSD, AnnualUSD: *s.MonthlyUSD * 12, CycleStart: start, CycleEnd: end, Windows: []quotaValue{}}
		item.MonthlyMaxUSD = s.MonthlyMaxUSD
		item.PriceSource = s.PriceSource
		days := end.Sub(start).Hours() / 24
		item.DailyUSD = item.MonthlyUSD / days
		if validCostNumber(s.MonthlyCAD) {
			value := *s.MonthlyCAD
			item.MonthlyCAD = &value
			item.CADSource = "Entered CAD fee"
		} else if fx != nil {
			value := item.MonthlyUSD * fx.CADPerUSD
			item.MonthlyCAD = &value
			item.CADSource = fx.Source
			if s.MonthlyMaxUSD != nil {
				high := *s.MonthlyMaxUSD * fx.CADPerUSD
				item.MonthlyMaxCAD = &high
			}
		}
		if item.MonthlyCAD != nil {
			annual := *item.MonthlyCAD * 12
			daily := *item.MonthlyCAD / days
			item.AnnualCAD = &annual
			item.DailyCAD = &daily
		}
		var quotas []api.CodexQuota
		var captured time.Time
		switch costProvider(s.Provider) {
		case "codex":
			for _, a := range accounts {
				if a.Name == s.Account {
					item.AccountID = a.ID
					if snapshot, err := h.store.QueryLatestCodex(a.ID); err == nil && snapshot != nil {
						quotas = snapshot.Quotas
						captured = snapshot.CapturedAt
					}
					break
				}
			}
		case "anthropic":
			if s.Account == "default" {
				if snapshot, err := h.store.QueryLatestAnthropic(); err == nil && snapshot != nil {
					captured = snapshot.CapturedAt
					for _, q := range snapshot.Quotas {
						quotas = append(quotas, api.CodexQuota{Name: q.Name, Utilization: q.Utilization, ResetsAt: q.ResetsAt})
					}
				}
			}
		}
		if captured.IsZero() {
			item.Reason = "No linked quota reading for this provider/account"
		} else {
			item.ObservedAt = &captured
			if snapshotStale(now, captured) {
				item.Reason = "Quota reading is stale; waiting for a successful provider poll"
			} else {
				for _, q := range quotas {
					if value := quotaAllocation(q, item.MonthlyUSD, item.MonthlyCAD, end.Sub(start), now); value != nil {
						item.Windows = append(item.Windows, *value)
					}
					if strings.HasPrefix(q.Name, "seven_day") {
						q.Name = "seven_day"
						// One weekly value per account. Overlapping model windows are not added.
						value := quotaAllocation(q, item.MonthlyUSD, item.MonthlyCAD, 52*7*24*time.Hour/12, now)
						if value != nil && (item.Weekly == nil || value.UsedPercent > item.Weekly.UsedPercent) {
							item.Weekly = value
						}
					}
				}
				if len(item.Windows) == 0 {
					item.Reason = "No current 5-hour or weekly quota window reported"
				}
			}
		}
		result = append(result, item)
	}
	return result
}

func costProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "codex":
		return "codex"
	case "claude", "anthropic":
		return "anthropic"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

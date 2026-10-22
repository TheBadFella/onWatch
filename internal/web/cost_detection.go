package web

import "strings"

// Public list prices checked 2026-10-22. These are estimates, not invoice data.
// https://chatgpt.com/pricing/ and https://help.openai.com/en/articles/8792828
// https://help.openai.com/en/articles/9793128 and /articles/6950777
type detectedCostSubscription struct {
	Provider      string   `json:"provider"`
	Account       string   `json:"account"`
	AccountID     int64    `json:"account_id,omitempty"`
	Plan          string   `json:"plan"`
	MonthlyUSD    *float64 `json:"monthly_usd"`
	MonthlyMaxUSD *float64 `json:"monthly_max_usd,omitempty"`
	Source        string   `json:"source"`
}

func codexSubscriptionPrice(plan string) (float64, float64, string, bool) {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "free":
		return 0, 0, "Free plan", true
	case "plus":
		return 20, 20, "Plus monthly list price", true
	case "prolite":
		return 100, 100, "Pro 100 monthly list price", true
	case "promax":
		return 500, 500, "Pro 500 monthly list price", true
	case "pro":
		return 200, 200, "Pro monthly reference price", true
	case "team", "business":
		return 20, 20, "Business Standard annual-plan reference price per seat", true
	default:
		return 0, 0, "No public price identified for this plan", false
	}
}

func (h *Handler) detectedCostSubscriptions() []detectedCostSubscription {
	result := []detectedCostSubscription{}
	if h.store == nil {
		return result
	}
	accounts, err := h.store.QueryActiveProviderAccounts("codex")
	if err != nil {
		return result
	}
	for _, account := range accounts {
		if len(result) >= 100 {
			break
		}
		item := detectedCostSubscription{Provider: "codex", Account: account.Name, AccountID: account.ID, Source: "Waiting for the provider to report a plan"}
		if snapshot, err := h.store.QueryLatestCodex(account.ID); err == nil && snapshot != nil {
			item.Plan = snapshot.PlanType
			low, high, source, known := codexSubscriptionPrice(item.Plan)
			item.Source = source
			if known {
				item.MonthlyUSD = &low
				if high != low {
					item.MonthlyMaxUSD = &high
				}
			}
		} else {
			// The store includes an unused default account before any connection.
			continue
		}
		result = append(result, item)
	}
	if snapshot, err := h.store.QueryLatestAnthropic(); err == nil && snapshot != nil {
		result = append(result, detectedCostSubscription{Provider: "anthropic", Account: "default", Source: "Claude quota responses do not identify the subscription tier"})
	}
	return result
}

func (h *Handler) effectiveCostSettings(c costSettings) (costSettings, []detectedCostSubscription) {
	detected := h.detectedCostSubscriptions()
	// Copy before appending so automatic estimates never become saved overrides.
	c.Subscriptions = append([]costSubscription{}, c.Subscriptions...)
	seen := map[string]bool{}
	for i := range c.Subscriptions {
		s := &c.Subscriptions[i]
		s.MonthlyMaxUSD = nil
		s.PriceSource = "Saved price override"
		seen[costProvider(s.Provider)+"\x00"+s.Account] = true
	}
	unpriced := []detectedCostSubscription{}
	for _, s := range detected {
		if seen[s.Provider+"\x00"+s.Account] {
			continue
		}
		if s.MonthlyUSD == nil {
			unpriced = append(unpriced, s)
			continue
		}
		c.Subscriptions = append(c.Subscriptions, costSubscription{Provider: s.Provider, Account: s.Account, MonthlyUSD: s.MonthlyUSD, MonthlyMaxUSD: s.MonthlyMaxUSD, PriceSource: s.Source + " (2026-10-22)"})
	}
	return c, unpriced
}

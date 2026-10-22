package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const bankOfCanadaExchangeURL = "https://www.bankofcanada.ca/valet/observations/FXUSDCAD/json?recent=1"

type costExchange struct {
	CADPerUSD float64   `json:"cad_per_usd"`
	Date      string    `json:"date,omitempty"`
	Source    string    `json:"source"`
	FetchedAt time.Time `json:"fetched_at,omitempty"`
	Stale     bool      `json:"stale"`
}

// Fixed public endpoint, bounded requests, persisted last-good rate and backoff.
// Nothing on the provider polling/authentication path depends on this lookup.
func (h *Handler) costExchange(ctx context.Context, c costSettings, now time.Time) *costExchange {
	if c.CADPerUSD != nil && validCostNumber(c.CADPerUSD) && *c.CADPerUSD > 0 {
		return &costExchange{CADPerUSD: *c.CADPerUSD, Source: "Configured rate"}
	}
	need := len(c.Rates) > 0
	for _, s := range c.Subscriptions {
		if !validCostNumber(s.MonthlyCAD) {
			need = true
			break
		}
	}
	if !need {
		return nil
	}
	h.costFXMu.Lock()
	defer h.costFXMu.Unlock()
	if h.costFX == nil {
		if raw, err := h.store.GetSetting("cost_exchange_cache"); err == nil && raw != "" {
			var cached costExchange
			if json.Unmarshal([]byte(raw), &cached) == nil && cached.CADPerUSD > 0 && cached.CADPerUSD < 100 && cached.Date != "" {
				h.costFX = &cached
			}
		}
	}
	if (h.costFX == nil || now.Sub(h.costFX.FetchedAt) >= 24*time.Hour) && (h.costFXAttempt.IsZero() || now.Sub(h.costFXAttempt) >= time.Hour) {
		h.costFXAttempt = now
		if value := h.fetchCostExchange(ctx, now); value != nil {
			h.costFX = value
			data, _ := json.Marshal(value)
			_ = h.store.SetSetting("cost_exchange_cache", string(data))
		}
	}
	if h.costFX == nil {
		return nil
	}
	result := *h.costFX
	result.Stale = now.Sub(result.FetchedAt) > 48*time.Hour
	return &result
}
func (h *Handler) fetchCostExchange(ctx context.Context, now time.Time) *costExchange {
	client := h.costFXClient
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, bankOfCanadaExchangeURL, nil)
	if err != nil {
		return nil
	}
	response, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil
	}
	var data struct {
		Observations []struct {
			Date string `json:"d"`
			Rate struct {
				Value string `json:"v"`
			} `json:"FXUSDCAD"`
		} `json:"observations"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&data) != nil || len(data.Observations) == 0 {
		return nil
	}
	latest := data.Observations[len(data.Observations)-1]
	rate, err := strconv.ParseFloat(latest.Rate.Value, 64)
	if err != nil || !validCostNumber(&rate) || rate <= 0 || rate >= 100 {
		return nil
	}
	date, err := time.Parse("2006-01-02", latest.Date)
	if err != nil || date.After(now.Add(24*time.Hour)) {
		return nil
	}
	return &costExchange{CADPerUSD: rate, Date: latest.Date, Source: "Bank of Canada", FetchedAt: now}
}

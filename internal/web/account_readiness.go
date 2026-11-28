package web

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/agent"
	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

type accountReadiness struct {
	State      string     `json:"state"`
	ObservedAt *time.Time `json:"observedAt,omitempty"`
	Binding    string     `json:"binding,omitempty"`
	ResetAt    *time.Time `json:"resetAt,omitempty"`
}

func codexReadiness(snapshot *api.CodexSnapshot, now time.Time, staleAfter time.Duration, reconnect, polling bool) accountReadiness {
	result := accountReadiness{State: "unknown"}
	if snapshot != nil {
		result.ObservedAt = &snapshot.CapturedAt
	}
	switch {
	case !polling:
		result.State = "disabled"
	case reconnect:
		result.State = "reconnect"
	case snapshot == nil:
	case now.Sub(snapshot.CapturedAt) >= staleAfter:
		result.State = "stale"
	default:
		fiveHour, weekly := false, false
		expired := false
		var binding *api.CodexQuota
		for _, quota := range snapshot.Quotas {
			name := codexNormalizedQuotaName(snapshot.PlanType, quota.Name)
			if name != "five_hour" && name != "seven_day" {
				continue
			}
			if quota.ResetsAt != nil && !quota.ResetsAt.After(now) {
				expired = true
				continue
			}
			fiveHour = fiveHour || name == "five_hour"
			weekly = weekly || name == "seven_day"
			if binding == nil || codexBindsBefore(quota, *binding) {
				quota.Name = name
				binding = &quota
			}
		}
		switch {
		case binding != nil && binding.Utilization >= 100:
			result.State = "limited"
		case expired:
			result.State = "awaiting_reset"
		case weekly && (fiveHour || codexIsFreePlan(snapshot.PlanType)):
			result.State = "ready"
		}
		if binding != nil && (result.State == "ready" || result.State == "limited") {
			result.Binding, result.ResetAt = binding.Name, binding.ResetsAt
		}
	}
	return result
}

func codexBindsBefore(quota, current api.CodexQuota) bool {
	if quota.Utilization >= 100 && current.Utilization >= 100 {
		return quota.ResetsAt == nil || (current.ResetsAt != nil && quota.ResetsAt.After(*current.ResetsAt))
	}
	return quota.Utilization > current.Utilization
}

func (h *Handler) starterStatuses(provider, account string) []store.StarterStatus {
	raw, err := h.store.GetSetting("provider_settings")
	if err != nil {
		return nil
	}
	// Other provider settings include non-string values. Decode only these toggles.
	var providers map[string]map[string]json.RawMessage
	if raw != "" && json.Unmarshal([]byte(raw), &providers) != nil {
		return nil
	}
	type starterToggle struct {
		quota, key string
		enabled    bool
	}
	quotas := []starterToggle{{"five_hour", "auto_start_5h", false}}
	if provider == "codex" {
		quotas = append(quotas, starterToggle{"seven_day", "auto_start_7d", false})
		if h.config != nil {
			quotas[0].enabled = h.config.CodexAutoStart5h
			quotas[1].enabled = h.config.CodexAutoStart7d
		}
	}
	var statuses []store.StarterStatus
	for _, q := range quotas {
		var setting string
		_ = json.Unmarshal(providers[provider][q.key], &setting)
		if setting == "off" || (setting != "on" && !q.enabled) {
			continue
		}
		status, err := h.store.GetStarterStatus(provider, account, q.quota)
		if err != nil {
			h.logger.Warn("Could not read starter status", "provider", provider, "account_id", account, "error", err)
			continue
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func (h *Handler) codexAccountReadiness(accountID int64, snapshot *api.CodexSnapshot, paused bool, now time.Time) accountReadiness {
	key := "codex:" + strconv.FormatInt(accountID, 10)
	visibility := h.providerVisibilityMap()
	polling := h.providerPollingEnabled("codex", visibility) && h.providerPollingEnabled(key, visibility)
	window := staleAfterFor(staleContext{Base: "codex", Polling: true, PollInterval: time.Duration(h.getPollIntervalSec()) * time.Second})
	return codexReadiness(snapshot, now, window, paused, polling)
}

func validStarterSettings(settings map[string]interface{}) bool {
	for _, provider := range []string{"anthropic", "codex"} {
		fields, _ := settings[provider].(map[string]interface{})
		if raw, exists := fields["starter_schedule"]; exists {
			schedule, ok := raw.(string)
			if !ok || !agent.ValidStarterSchedule(schedule) {
				return false
			}
		}
		if raw, exists := fields["starter_account_schedules"]; exists && provider == "codex" {
			schedules, ok := raw.(map[string]interface{})
			if !ok {
				return false
			}
			for account, raw := range schedules {
				id, err := strconv.ParseInt(account, 10, 64)
				schedule, ok := raw.(string)
				if err != nil || id <= 0 || !ok || !agent.ValidStarterSchedule(schedule) {
					return false
				}
			}
		}
	}
	return true
}

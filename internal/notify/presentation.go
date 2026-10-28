package notify

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func providerLabel(provider string) string {
	switch provider {
	case "codex":
		return "Codex"
	case "anthropic":
		return "Claude"
	case "zai":
		return "Z.ai"
	case "openrouter":
		return "OpenRouter"
	case "minimax":
		return "MiniMax"
	case "copilot":
		return "GitHub Copilot"
	}
	return titleCase(provider)
}

func quotaLabel(key string) string {
	switch key {
	case "five_hour":
		return "5-hour limit"
	case "seven_day":
		return "Weekly limit"
	case "seven_day_sonnet":
		return "Weekly Sonnet limit"
	case "code_review":
		return "Code review limit"
	}
	return strings.ReplaceAll(key, "_", " ")
}

func resetCauseLabel(cause string) string {
	switch cause {
	case "scheduled":
		return "Scheduled reset"
	case "banked_credit":
		return "Banked reset used"
	case "early_unknown":
		return "Early reset, cause unconfirmed"
	default:
		return "Reset detected, cause unknown"
	}
}

func (e *NotificationEngine) accountName(provider, id string) string {
	if id == "" {
		return ""
	}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil && e.store != nil {
		if acc, err := e.store.GetProviderAccountByID(n); err == nil && acc != nil && acc.Provider == provider && acc.Name != "" {
			return acc.Name
		}
		return "Account " + id
	}
	return id
}

func usageChange(before, after *float64) string {
	if before != nil && after != nil {
		return fmt.Sprintf("%.1f%% → %.1f%% used", *before, *after)
	}
	if after != nil {
		return fmt.Sprintf("%.1f%% used", *after)
	}
	return "Usage unavailable"
}

func resetMessage(status QuotaStatus) string {
	if status.Reset == nil {
		return "A new usage window was detected. The reset cause was not recorded."
	}
	return resetCauseLabel(status.Reset.Cause) + ". " + status.Reset.Evidence
}

func notificationTime(t *time.Time) string {
	if t == nil {
		return "Not reported"
	}
	return t.UTC().Format("Jan 2, 2006 at 15:04:05 UTC")
}

func remainingPercent(used float64) float64 {
	return max(0, min(100, 100-used))
}

// weeklyUsage uses the latest persisted poll for the notification's own account.
func (e *NotificationEngine) weeklyUsage(status QuotaStatus) string {
	if e.store == nil {
		return ""
	}
	format := func(used float64, reset *time.Time, observed time.Time) string {
		return fmt.Sprintf("%.1f%% used | %.1f%% remaining\nResets: %s\nObserved: %s", used, remainingPercent(used), notificationTime(reset), notificationTime(&observed))
	}
	switch status.Provider {
	case "codex":
		accountID := int64(1)
		if status.AccountID != "" {
			var err error
			accountID, err = strconv.ParseInt(status.AccountID, 10, 64)
			if err != nil || accountID <= 0 {
				return "Not reported"
			}
		}
		if snapshot, err := e.store.QueryLatestCodex(accountID); err == nil && snapshot != nil {
			for _, q := range snapshot.Quotas {
				if q.Name == "seven_day" || q.Name == "weekly" {
					return format(q.Utilization, q.ResetsAt, snapshot.CapturedAt)
				}
			}
		}
		return "Not reported"
	case "anthropic":
		if snapshot, err := e.store.QueryLatestAnthropic(); err == nil && snapshot != nil {
			for _, q := range snapshot.Quotas {
				if q.Name == "seven_day" {
					return format(q.Utilization, q.ResetsAt, snapshot.CapturedAt)
				}
			}
		}
		return "Not reported"
	}
	return ""
}

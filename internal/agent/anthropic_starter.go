package agent

import (
	"context"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/notify"
)

const anthropicStarterWindow = 5 * time.Hour
const anthropicStarterRetry = 30 * time.Minute

// SetAutoStart5hCheck reads the starter toggle on each successful API poll.
func (a *AnthropicAgent) SetAutoStart5hCheck(enabled func() bool) {
	a.autoStart5hEnabled = enabled
}

func anthropicWindowUnstarted(quotas api.AnthropicQuotaResponse, now time.Time) bool {
	window, present := quotas["five_hour"]
	if !present {
		return false
	}
	if window == nil {
		return true
	}
	if window.Utilization == nil || (window.IsEnabled != nil && !*window.IsEnabled) {
		return false
	}
	if window.ResetsAt == nil {
		return *window.Utilization == 0
	}
	reset, err := time.Parse(time.RFC3339, *window.ResetsAt)
	return err == nil && !reset.After(now)
}

func (a *AnthropicAgent) maybeStartFiveHourWindow(ctx context.Context, quotas api.AnthropicQuotaResponse, now time.Time) {
	if a.autoStart5hEnabled == nil || !a.autoStart5hEnabled() || ctx.Err() != nil || !anthropicWindowUnstarted(quotas, now) {
		return
	}
	if weekly := quotas["seven_day"]; weekly != nil && weekly.Utilization != nil && *weekly.Utilization >= 100 {
		return
	}
	// With paid usage enabled, a racing request or another binding limit could
	// turn this automatic prompt into a pay-as-you-go request.
	if extra := quotas["extra_usage"]; extra != nil && extra.IsEnabled != nil && *extra.IsEnabled {
		return
	}

	reservedUntil := now.Add(anthropicStarterWindow)
	claimed, err := a.store.ClaimAnthropicStarter(ctx, now, reservedUntil)
	if err != nil {
		a.logger.Error("Failed to reserve Anthropic starter attempt", "error", err)
		return
	}
	if !claimed {
		return
	}

	event := notify.StarterEvent{Provider: "anthropic", QuotaKey: "five_hour"}
	if err := a.startFiveHourWindow(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		event.Detail = err.Error()
		a.logger.Warn("Anthropic starter prompt failed", "error", err)
		if err := a.store.SetAnthropicStarterRetry(ctx, reservedUntil, now.Add(anthropicStarterRetry)); err != nil {
			a.logger.Error("Failed to record Anthropic starter retry", "error", err)
		}
	} else {
		event.Success = true
		a.logger.Info("Anthropic starter prompt completed")
	}
	if a.notifier != nil {
		a.notifier.SendStarterEvent(event)
	}
}

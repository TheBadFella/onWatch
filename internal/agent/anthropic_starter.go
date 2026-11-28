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
	if a.autoStart5hEnabled == nil || !a.autoStart5hEnabled() || ctx.Err() != nil {
		return
	}
	const provider, quota = "anthropic", "five_hour"
	wait := func(reason string, next time.Time) {
		recordStarterWait(a.store, a.logger, provider, "", quota, reason, next)
	}
	if !anthropicWindowUnstarted(quotas, now) {
		if window := quotas[quota]; window != nil && window.ResetsAt != nil {
			if reset, err := time.Parse(time.RFC3339, *window.ResetsAt); err == nil && reset.After(now) {
				observeStarterWindow(a.store, a.logger, provider, "", quota, now, reset, anthropicStarterWindow)
				wait("Window is active", reset)
				return
			}
		}
		wait("Waiting for quota data", time.Time{})
		return
	}
	if weekly := quotas["seven_day"]; weekly != nil && weekly.Utilization != nil && *weekly.Utilization >= 100 {
		var reset time.Time
		if weekly.ResetsAt != nil {
			reset, _ = time.Parse(time.RFC3339, *weekly.ResetsAt)
		}
		wait("Weekly quota exhausted", reset)
		return
	}
	// With paid usage enabled, a racing request or another binding limit could
	// turn this automatic prompt into a pay-as-you-go request.
	if extra := quotas["extra_usage"]; extra != nil && extra.IsEnabled != nil && *extra.IsEnabled {
		wait("Paid usage enabled", time.Time{})
		return
	}
	if allowed, next := starterSchedule(configuredStarterSchedule(a.store, provider, ""), now.In(time.Local)); !allowed {
		wait(scheduleWaitReason(next), next)
		return
	}

	reservedUntil := now.Add(anthropicStarterWindow)
	claimed, err := a.store.ClaimAnthropicStarter(ctx, now, reservedUntil)
	if err != nil {
		a.logger.Error("Failed to reserve Anthropic starter attempt", "error", err)
		return
	}
	if !claimed {
		retry, _ := a.store.AnthropicStarterRetryAt()
		wait("Starter cooldown", retry)
		return
	}

	event := notify.StarterEvent{Provider: provider, QuotaKey: quota}
	if err := a.startFiveHourWindow(ctx); err != nil {
		if ctx.Err() != nil {
			return
		}
		event.Detail = err.Error()
		a.logger.Warn("Anthropic starter prompt failed", "error", err)
		retry := now.Add(anthropicStarterRetry)
		if err := a.store.SetAnthropicStarterRetry(ctx, reservedUntil, retry); err != nil {
			a.logger.Error("Failed to record Anthropic starter retry", "error", err)
		}
		recordStarterAttempt(a.store, a.logger, provider, "", quota, "failed", now, retry)
	} else {
		event.Success = true
		a.logger.Info("Anthropic starter prompt completed")
		recordStarterAttempt(a.store, a.logger, provider, "", quota, "sent", now, reservedUntil)
	}
	if a.notifier != nil {
		a.notifier.SendStarterEvent(event)
	}
}

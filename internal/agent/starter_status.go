package agent

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func configuredStarterSchedule(db *store.Store, provider, account string) string {
	if db == nil {
		return ""
	}
	raw, err := db.GetSetting("provider_settings")
	if err != nil {
		return "unavailable"
	}
	var settings map[string]map[string]json.RawMessage
	if raw != "" && json.Unmarshal([]byte(raw), &settings) != nil {
		return "unavailable"
	}
	var schedule string
	if account != "" {
		var overrides map[string]string
		if json.Unmarshal(settings[provider]["starter_account_schedules"], &overrides) == nil {
			if value, exists := overrides[account]; exists {
				return value
			}
		}
	}
	if value := settings[provider]["starter_schedule"]; len(value) > 0 && json.Unmarshal(value, &schedule) != nil {
		return "unavailable"
	}
	return schedule
}

func scheduleWaitReason(next time.Time) string {
	if next.IsZero() {
		return "Starter schedule unavailable"
	}
	return "Outside starter hours"
}

func parseStarterSchedule(schedule string) (start, end time.Time, valid bool) {
	from, to, found := strings.Cut(schedule, "-")
	if !found || len(from) != 5 || len(to) != 5 {
		return start, end, false
	}
	start, startErr := time.Parse("15:04", from)
	end, endErr := time.Parse("15:04", to)
	return start, end, startErr == nil && endErr == nil && !start.Equal(end)
}

func ValidStarterSchedule(schedule string) bool {
	_, _, valid := parseStarterSchedule(schedule)
	return schedule == "" || valid
}

// A blank schedule allows starts at any time. Intervals may cross midnight.
func starterSchedule(schedule string, now time.Time) (allowed bool, next time.Time) {
	if schedule == "" {
		return true, time.Time{}
	}
	start, end, valid := parseStarterSchedule(schedule)
	if !valid {
		return false, time.Time{}
	}
	from, to := start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute()
	minute := now.Hour()*60 + now.Minute()
	if (from < to && minute >= from && minute < to) || (from > to && (minute >= from || minute < to)) {
		return true, time.Time{}
	}
	day := now.Day()
	if minute >= from {
		day++
	}
	return false, time.Date(now.Year(), now.Month(), day, start.Hour(), start.Minute(), 0, 0, now.Location())
}

func starterWindowObserved(at, observed, reset time.Time, window time.Duration) bool {
	started := reset.Add(-window)
	// Providers round reset timestamps. Permit a short rounding difference.
	return observed.After(at) && reset.After(observed) &&
		!started.Before(at.Add(-time.Minute)) && !started.After(at.Add(2*time.Minute)) &&
		!started.After(observed)
}

func writeStarterStatus(db *store.Store, logger *slog.Logger, provider, account, quota string, update func(*store.StarterStatus) bool) {
	if db == nil {
		return
	}
	status, err := db.GetStarterStatus(provider, account, quota)
	if err == nil && update(&status) {
		err = db.SetStarterStatus(provider, account, status)
	}
	if err != nil {
		logger.Warn("Could not save starter status", "provider", provider, "account_id", account, "quota", quota, "error", err)
	}
}

func starterTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func recordStarterWait(db *store.Store, logger *slog.Logger, provider, account, quota, reason string, next time.Time) {
	writeStarterStatus(db, logger, provider, account, quota, func(status *store.StarterStatus) bool {
		if status.Waiting == reason && ((status.NextAt == nil && next.IsZero()) || (status.NextAt != nil && status.NextAt.Equal(next))) {
			return false
		}
		status.Waiting, status.NextAt = reason, starterTime(next)
		return true
	})
}

func recordStarterAttempt(db *store.Store, logger *slog.Logger, provider, account, quota, result string, at, next time.Time) {
	writeStarterStatus(db, logger, provider, account, quota, func(status *store.StarterStatus) bool {
		*status = store.StarterStatus{Quota: quota, Result: result, AttemptedAt: starterTime(at), NextAt: starterTime(next)}
		return true
	})
}

func observeStarterWindow(db *store.Store, logger *slog.Logger, provider, account, quota string, observed, reset time.Time, window time.Duration) {
	writeStarterStatus(db, logger, provider, account, quota, func(status *store.StarterStatus) bool {
		if status.Result != "sent" || status.AttemptedAt == nil || !observed.After(*status.AttemptedAt) {
			return false
		}
		if starterWindowObserved(*status.AttemptedAt, observed, reset, window) {
			status.Result, status.ConfirmedAt = "confirmed", starterTime(observed)
			return true
		}
		return false
	})
}

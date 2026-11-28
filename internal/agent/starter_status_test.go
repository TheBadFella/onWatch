package agent

import (
	"testing"
	"time"
)

func TestStarterSchedule(t *testing.T) {
	loc := time.FixedZone("CST", -6*60*60)
	now := time.Date(2026, time.November, 28, 23, 0, 0, 0, loc)
	for _, tc := range []struct {
		schedule string
		allowed  bool
		nextHour int
	}{
		{"", true, -1},
		{"22:00-06:00", true, -1},
		{"08:00-22:00", false, 8},
		{"23:30-05:30", false, 23},
		{"invalid", false, -1},
	} {
		allowed, next := starterSchedule(tc.schedule, now)
		if allowed != tc.allowed || (tc.nextHour < 0 && !next.IsZero()) || (tc.nextHour >= 0 && (next.Hour() != tc.nextHour || !next.After(now))) {
			t.Errorf("schedule %q: got allowed=%v next=%v", tc.schedule, allowed, next)
		}
	}
}

func TestStarterConfirmation(t *testing.T) {
	at := time.Date(2026, time.November, 28, 8, 0, 0, 0, time.UTC)
	window := 5 * time.Hour
	for _, tc := range []struct {
		name     string
		observed time.Time
		reset    time.Time
		want     bool
	}{
		{"subsequent window", at.Add(time.Minute), at.Add(window + time.Second), true},
		{"concurrent poll", at, at.Add(window), false},
		{"old window", at.Add(time.Minute), at.Add(window - 5*time.Minute), false},
		{"window started much later", at.Add(time.Hour), at.Add(window + 45*time.Minute), false},
		{"still rolling as of poll", at.Add(time.Minute), at.Add(window + 2*time.Minute), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := starterWindowObserved(at, tc.observed, tc.reset, window); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

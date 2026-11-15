package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A 5-hour window renewal is useless while the same account's weekly limit is
// spent, so neither its reset nor its reset-soon alert should be sent then.
func TestShortWindowAlertsSuppressedWhileWeeklyExhausted(t *testing.T) {
	cases := []struct{ provider, account, short, weekly string }{
		{"codex", "9", "five_hour", "seven_day"},
		{"anthropic", "", "five_hour", "seven_day"},
		{"commandcode", "", "five_hour", "weekly"},
		{"antigravity", "69", "gemini-5h", "gemini-weekly"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			s := newTestStore(t)
			defer s.Close()
			e := newTestEngine(t, s)
			now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
			e.now = func() time.Time { return now }
			deliveries := make(chan WebhookPayload, 16)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var p WebhookPayload
				_ = json.NewDecoder(r.Body).Decode(&p)
				deliveries <- p
				w.WriteHeader(http.StatusNoContent)
			}))
			defer srv.Close()
			e.webhookSender = NewWebhookSender(WebhookConfig{URL: srv.URL, Events: WebhookEvents{Reset: true, ResetSoon: true}}, e.logger)
			e.cfg.Channels = NotificationChannels{Webhook: true}
			e.cfg.Types.Reset = true
			drain := func() (events []string) {
				for {
					select {
					case p := <-deliveries:
						events = append(events, p.Event+":"+p.QuotaKey)
					default:
						return
					}
				}
			}

			e.Check(QuotaStatus{Provider: tc.provider, AccountID: tc.account, QuotaKey: tc.weekly, Utilization: 100, ResetAt: now.Add(72 * time.Hour)})
			e.Check(QuotaStatus{Provider: tc.provider, AccountID: tc.account, QuotaKey: tc.short, Utilization: 40, ResetAt: now.Add(5 * time.Minute)})
			e.Check(QuotaStatus{Provider: tc.provider, AccountID: tc.account, QuotaKey: tc.short, ResetOccurred: true})
			if got := drain(); len(got) != 0 {
				t.Fatalf("short-window alerts sent while weekly exhausted: %v", got)
			}

			e.Check(QuotaStatus{Provider: tc.provider, AccountID: tc.account, QuotaKey: tc.weekly, Utilization: 60, ResetAt: now.Add(72 * time.Hour)})
			e.Check(QuotaStatus{Provider: tc.provider, AccountID: tc.account, QuotaKey: tc.short, ResetOccurred: true})
			if got := drain(); len(got) != 1 || got[0] != EventReset+":"+tc.short {
				t.Fatalf("expected one short-window reset once weekly has room: %v", got)
			}
		})
	}
}

// Another account's exhausted weekly limit must not silence this account.
func TestWeeklyExhaustionIsAccountScoped(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	e := newTestEngine(t, s)
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	e.webhookSender = NewWebhookSender(WebhookConfig{URL: srv.URL, Events: WebhookEvents{Reset: true}}, e.logger)
	e.cfg.Channels = NotificationChannels{Webhook: true}
	e.cfg.Types.Reset = true

	e.Check(QuotaStatus{Provider: "codex", AccountID: "9", QuotaKey: "seven_day", Utilization: 100})
	e.Check(QuotaStatus{Provider: "codex", AccountID: "6", QuotaKey: "five_hour", ResetOccurred: true})
	if sent != 1 {
		t.Fatalf("account 6 reset suppressed by account 9 weekly: sent=%d", sent)
	}
}

// After a restart the first reset can be detected before any weekly reading
// has been checked, so agents seed the latest stored usage without alerting.
func TestRememberUsageSeedsWeeklyExhaustionWithoutAlerting(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	e := newTestEngine(t, s)
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	e.now = func() time.Time { return now }
	sent := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	e.webhookSender = NewWebhookSender(WebhookConfig{URL: srv.URL, Events: WebhookEvents{Reset: true, Critical: true, Exhausted: true}}, e.logger)
	e.cfg.Channels = NotificationChannels{Webhook: true}
	e.cfg.Types.Reset = true
	e.cfg.Types.Critical = true

	e.RememberUsage(QuotaStatus{Provider: "codex", AccountID: "9", QuotaKey: "seven_day", Utilization: 100, ResetAt: now.Add(time.Hour)})
	e.RememberUsage(QuotaStatus{Provider: "codex", AccountID: "6", QuotaKey: "seven_day", Utilization: 100, ResetAt: now.Add(-time.Minute)}) // already reset
	if sent != 0 {
		t.Fatalf("seeding sent %d notifications", sent)
	}
	e.Check(QuotaStatus{Provider: "codex", AccountID: "9", QuotaKey: "five_hour", ResetOccurred: true})
	if sent != 0 {
		t.Fatal("reset alert sent although the seeded weekly limit is exhausted")
	}
	e.Check(QuotaStatus{Provider: "codex", AccountID: "6", QuotaKey: "five_hour", ResetOccurred: true})
	if sent != 1 {
		t.Fatalf("weekly reading whose reset time has passed still suppressed the alert: sent=%d", sent)
	}
}

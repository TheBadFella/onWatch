package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindowAlertsUseFreshUsageAndExactTimes(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	e := newTestEngine(t, s)
	now := time.Date(2026, 10, 22, 18, 0, 11, 0, time.UTC)
	e.now = func() time.Time { return now }
	var received []WebhookPayload
	deliveries := make(chan WebhookPayload, 16)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload WebhookPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		deliveries <- payload
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	e.webhookSender = NewWebhookSender(WebhookConfig{URL: srv.URL, Events: WebhookEvents{ResetSoon: true, Exhausted: true}}, e.logger)
	e.cfg.Channels = NotificationChannels{Webhook: true}
	e.cfg.Repeat = true
	check := func(status QuotaStatus) {
		e.Check(status)
		for {
			select {
			case payload := <-deliveries:
				received = append(received, payload)
			default:
				return
			}
		}
	}
	status := QuotaStatus{Provider: "codex", AccountID: "9", QuotaKey: "five_hour", Utilization: 72.3, ObservedAt: now, ResetAt: now.Add(16 * time.Minute)}
	check(status)
	if len(received) != 0 {
		t.Fatal("reminded before lead time")
	}
	status.ResetAt = now.Add(15 * time.Minute)
	check(status)
	check(status)
	if len(received) != 1 || received[0].Event != EventResetSoon {
		t.Fatalf("expected one window reminder: %+v", received)
	}
	if *received[0].Utilization != 72.3 || *received[0].Remaining < 27.69 || received[0].ObservedAt != now.Format(time.RFC3339) || received[0].ResetAt != status.ResetAt.Format(time.RFC3339) {
		t.Fatalf("reading changed: %+v", received[0])
	}
	status.Utilization = 100
	status.ObservedAt = now.Add(time.Second)
	check(status)
	if len(received) != 2 || received[1].Event != EventExhausted || *received[1].Remaining != 0 {
		t.Fatalf("missing exhausted alert: %+v", received)
	}
	status.ResetOccurred = true
	check(status)
	status.ResetOccurred = false
	status.Utilization = 9
	status.ResetAt = now.Add(-time.Second)
	check(status)
	status.ResetAt = time.Time{}
	check(status)
	if len(received) != 2 {
		t.Fatal("unknown or past deadline generated reminder")
	}
	status.ResetAt = now.Add(10 * time.Minute)
	check(status)
	if len(received) != 3 || *received[2].Utilization != 9 {
		t.Fatal("new cycle did not use fresh usage")
	}
	data, err := formatDiscordEmbed(received[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "18:15:11 UTC") || strings.Contains(string(data), ":R>") {
		t.Fatalf("reset time not exact: %s", data)
	}
	if !strings.Contains(e.buildBody(status, EventResetSoon), "18:10:11 UTC") {
		t.Fatal("email missing exact reset seconds")
	}
}

func TestWindowAlertsChannelOptIn(t *testing.T) {
	mailer := NewSMTPMailer(SMTPConfig{}, nil)
	channels := channelSet{mailer: mailer, enabled: NotificationChannels{Email: true}}
	if channels.emailEnabled(EventResetSoon) || channels.emailEnabled(EventExhausted) {
		t.Fatal("legacy SMTP silently opted into new events")
	}
	mailer.config.Events = &WebhookEvents{ResetSoon: true}
	if !channels.emailEnabled(EventResetSoon) || channels.emailEnabled(EventExhausted) {
		t.Fatal("SMTP event selection ignored")
	}
	if (WebhookEvents{Exhausted: true}).Enabled(EventResetSoon) {
		t.Fatal("webhook event selection ignored")
	}
}

func TestWindowAlertsSMTPAndWebhookAreIndependent(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	e := newTestEngine(t, s)
	count, cleanup := setupSMTPAndMailer(t, s, e)
	defer cleanup()
	e.mailer.config.Events = &WebhookEvents{ResetSoon: true}
	now := time.Now().UTC()
	e.now = func() time.Time { return now }
	var hookCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hookCount.Add(1); w.WriteHeader(204) }))
	defer server.Close()
	e.webhookSender = NewWebhookSender(WebhookConfig{URL: server.URL, Events: WebhookEvents{Exhausted: true}}, e.logger)
	e.cfg.Channels = NotificationChannels{Email: true, Webhook: true}
	e.cfg.Types = NotificationTypes{}
	status := QuotaStatus{Provider: "codex", QuotaKey: "five_hour", Utilization: 75, ResetAt: now.Add(5 * time.Minute)}
	e.Check(status)
	if count.Load() != 1 || hookCount.Load() != 0 {
		t.Fatal("window reminder ignored channel filters")
	}
	status.Utilization = 100
	e.Check(status)
	e.Check(status)
	if count.Load() != 1 || hookCount.Load() != 1 {
		t.Fatal("exhausted alert ignored channel filters or deduplication")
	}
}

func TestWindowAlertsReloadLeadTime(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	e := newTestEngine(t, s)
	minutes := 30
	storeNotificationConfig(t, s, notificationSettingsJSON{ResetSoonMinutes: &minutes})
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	if e.Config().ResetSoon != 30*time.Minute {
		t.Fatal("saved lead time not loaded")
	}
}

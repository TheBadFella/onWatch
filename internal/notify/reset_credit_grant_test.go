package notify

import (
	"strings"
	"testing"
	"time"
)

func grantTestEngine(t *testing.T) (*NotificationEngine, *webhookCapture) {
	t.Helper()
	s := newTestStore(t)
	t.Cleanup(func() { s.Close() })
	engine := newTestEngine(t, s)
	capture := newWebhookCapture(t)
	storeWebhookConfig(t, s, WebhookConfig{
		URL:            capture.srv.URL,
		TimeoutSeconds: 2,
		Events:         WebhookEvents{ResetCreditGrant: true},
	})
	if err := engine.ConfigureWebhook(); err != nil {
		t.Fatalf("ConfigureWebhook: %v", err)
	}
	engine.cfg.Channels = NotificationChannels{Webhook: true}
	return engine, capture
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

// A batch of newly granted credits produces one message that names the account
// and includes only known dates.
func TestSendResetCreditGrantNotification_DeliversBatch(t *testing.T) {
	engine, capture := grantTestEngine(t)

	granted := time.Date(2026, 10, 29, 8, 30, 0, 0, time.UTC)
	expires := time.Date(2026, 11, 5, 8, 30, 0, 0, time.UTC)
	sent := engine.SendResetCreditGrantNotification(ResetCreditGrantAlert{
		AccountID:   "7",
		AccountName: "Work Codex",
		Credits: []ResetCreditGrant{
			{CreditID: "credit-1", Title: "Full reset", GrantedAt: &granted, ExpiresAt: &expires},
			{CreditID: "credit-2", Title: "Never expires credit"},
			{CreditID: "credit-3", Title: "Unknown expiry credit", ExpiryUnknown: true},
		},
	})
	if !sent {
		t.Fatal("SendResetCreditGrantNotification = false, want a webhook delivery")
	}
	payloads := capture.all()
	if len(payloads) != 1 {
		t.Fatalf("delivered %d payloads, want 1 batch", len(payloads))
	}
	payload := payloads[0]
	if payload.Event != EventResetCreditGrant {
		t.Fatalf("event = %q, want %q", payload.Event, EventResetCreditGrant)
	}
	if payload.AccountName != "Work Codex" || payload.AccountID != "7" {
		t.Fatalf("account = %q/%q, want Work Codex/7", payload.AccountName, payload.AccountID)
	}
	if !containsAll(payload.Title, "Work Codex", "3 new", "reset credits") {
		t.Fatalf("title = %q, want account, count and noun", payload.Title)
	}
	if !containsAll(payload.Message, "Full reset", "granted 2026-10-29", "expires 2026-11-05", "Never expires credit", "Unknown expiry credit", "(expiry unknown)") {
		t.Fatalf("message missing known detail: %q", payload.Message)
	}
}

// The preference switch silences the event without delivering anything.
func TestSendResetCreditGrantNotification_RespectsPreference(t *testing.T) {
	engine, capture := grantTestEngine(t)
	engine.cfg.Types.ResetCreditGrant = false

	sent := engine.SendResetCreditGrantNotification(ResetCreditGrantAlert{
		AccountID: "1",
		Credits:   []ResetCreditGrant{{CreditID: "credit-1"}},
	})
	if sent {
		t.Fatal("SendResetCreditGrantNotification = true with the preference off")
	}
	if capture.count() != 0 {
		t.Fatalf("delivered %d payloads with the preference off, want 0", capture.count())
	}
}
func TestResetCreditGrantEmailRespectsSavedFilter(t *testing.T) {
	st := newTestStore(t)
	defer st.Close()
	engine := newTestEngine(t, st)
	count, cleanup := setupSMTPAndMailer(t, st, engine)
	defer cleanup()
	engine.mailer.config.Events = &WebhookEvents{ResetCreditGrant: false}
	alert := ResetCreditGrantAlert{AccountID: "1", Credits: []ResetCreditGrant{{CreditID: "new"}}}
	if engine.SendResetCreditGrantNotification(alert) || count.Load() != 0 {
		t.Fatal("email sent despite disabled event")
	}
	engine.mailer.config.Events.ResetCreditGrant = true
	if !engine.SendResetCreditGrantNotification(alert) || count.Load() != 1 {
		t.Fatal("enabled grant email was not delivered")
	}
}

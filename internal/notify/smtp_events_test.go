package notify

import (
	"encoding/json"
	"testing"
)

func TestSMTPEventSelection(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	e := newTestEngine(t, s)
	count, cleanup := setupSMTPAndMailer(t, s, e)
	defer cleanup()
	raw, _ := s.GetSetting("smtp")
	var cfg map[string]interface{}
	json.Unmarshal([]byte(raw), &cfg)
	cfg["events"] = map[string]bool{"auth_error": true, "starter_failure": true}
	rawBytes, _ := json.Marshal(cfg)
	s.SetSetting("smtp", string(rawBytes))
	if err := e.ConfigureSMTP(); err != nil {
		t.Fatal(err)
	}
	e.cfg.Types = NotificationTypes{Warning: true, Critical: true, Reset: true, AuthError: true}
	e.Check(QuotaStatus{Provider: "codex", QuotaKey: "five_hour", Utilization: 85})
	e.Check(QuotaStatus{Provider: "codex", QuotaKey: "weekly", Utilization: 99})
	e.Check(QuotaStatus{Provider: "codex", QuotaKey: "five_hour", ResetOccurred: true})
	if got := count.Load(); got != 0 {
		t.Fatalf("filtered quota emails = %d, want 0", got)
	}
	if !e.SendAuthErrorNotification(AuthErrorAlert{Provider: "codex", Title: "Sign in required"}) {
		t.Fatal("auth email was not sent")
	}
	e.SendStarterEvent(StarterEvent{Provider: "codex", Success: true})
	e.SendStarterEvent(StarterEvent{Provider: "codex", Success: false})
	if got := count.Load(); got != 2 {
		t.Fatalf("selected emails = %d, want 2", got)
	}
}

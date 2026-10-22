package notify

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func TestIsDiscordWebhook(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://discord.com/api/webhooks/1234/abcd", true},
		{"https://discordapp.com/api/webhooks/1234/abcd", true},
		{"HTTPS://DISCORD.COM/API/WEBHOOKS/1234/ABCD", true},
		{"https://ntfy.sh/my-topic", false},
		{"https://hooks.slack.com/services/1/2/3", false},
		{"http://localhost:8080/webhook", false},
	}

	for _, tc := range tests {
		if got := isDiscordWebhook(tc.url); got != tc.expected {
			t.Errorf("isDiscordWebhook(%q) = %v; want %v", tc.url, got, tc.expected)
		}
	}
}

func TestFormatDiscordEmbed(t *testing.T) {
	util := 82.5
	payload := WebhookPayload{
		Event:       EventWarning,
		Provider:    "codex",
		AccountID:   "badfellaxai",
		Utilization: &util,
		ResetAt:     time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		Title:       "[WARNING] Codex quota at 82.5%",
		Message:     "Account badfellaxai is approaching limit",
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}

	data, err := formatDiscordEmbed(payload)
	if err != nil {
		t.Fatalf("formatDiscordEmbed failed: %v", err)
	}

	var msg discordWebhookMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal discord message failed: %v", err)
	}

	if len(msg.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(msg.Embeds))
	}

	embed := msg.Embeds[0]
	if embed.Title != "⚠️ Codex | Account badfellaxai | Usage warning" {
		t.Errorf("title = %q; want %q", embed.Title, payload.Title)
	}
	if embed.Color != 0xf59e0b {
		t.Errorf("color = %x; want %x", embed.Color, 0xf59e0b)
	}
	if len(embed.Fields) != 2 {
		t.Errorf("expected 4 fields, got %d", len(embed.Fields))
	}
}

func TestFormatWebhookBodyRouting(t *testing.T) {
	payload := WebhookPayload{
		Event:   EventTest,
		Title:   "Test Alert",
		Message: "Hello",
	}

	orig, _ := json.Marshal(payload)

	// Discord endpoint
	discordBytes, err := formatWebhookBody("https://discord.com/api/webhooks/123/xyz", payload, orig)
	if err != nil {
		t.Fatalf("format Discord payload error: %v", err)
	}
	var msg discordWebhookMessage
	if err := json.Unmarshal(discordBytes, &msg); err != nil || len(msg.Embeds) == 0 {
		t.Errorf("expected Discord embed structure, got: %s", string(discordBytes))
	}

	// Generic endpoint
	genericBytes, err := formatWebhookBody("https://example.com/webhook", payload, orig)
	if err != nil {
		t.Fatalf("format generic payload error: %v", err)
	}
	var generic WebhookPayload
	if err := json.Unmarshal(genericBytes, &generic); err != nil || generic.Title != "Test Alert" {
		t.Errorf("expected generic WebhookPayload structure, got: %s", string(genericBytes))
	}
}

func TestResetPresentation(t *testing.T) {
	now := time.Date(2026, 10, 22, 17, 9, 0, 0, time.UTC)
	due := now.Add(3 * time.Hour)
	next := now.Add(5 * time.Hour)
	before, after := 98.0, 0.0
	event := &store.CodexResetObservation{Cause: "early_unknown", ObservedAt: now, PreviousResetAt: &due, NextResetAt: &next, BeforePercent: &before, AfterPercent: &after, Evidence: "Before the scheduled time; actor not confirmed."}
	s := newTestStore(t)
	defer s.Close()
	engine := newTestEngine(t, s)
	status := QuotaStatus{Provider: "codex", AccountID: "6", AccountName: "Personal", QuotaKey: "five_hour", Reset: event, ResetOccurred: true}
	subject, body := engine.buildSubject(status, EventReset), engine.buildBody(status, EventReset)
	if !strings.Contains(subject, "Personal") || !strings.Contains(subject, "5-hour limit") || !strings.Contains(subject, "cause unconfirmed") {
		t.Fatal(subject)
	}
	if !strings.Contains(body, "98.0% → 0.0%") || !strings.Contains(body, "Previous due time:") {
		t.Fatal(body)
	}
	payload := webhookPayloadForQuota(status, EventReset, subject, body, 0)
	data, err := formatDiscordEmbed(payload)
	if err != nil {
		t.Fatal(err)
	}
	var msg discordWebhookMessage
	if err = json.Unmarshal(data, &msg); err != nil {
		t.Fatal(err)
	}
	e := msg.Embeds[0]
	if strings.Contains(e.Description, "Provider:") || strings.Contains(e.Description, "Sent by") {
		t.Fatal("duplicated email dump")
	}
	if e.Fields[0].Name != "Personal" || !strings.Contains(e.Fields[1].Value, "<t:") {
		t.Fatalf("missing account or local dates: %+v", e)
	}
	if !strings.Contains(string(data), `"allowed_mentions":{"parse":[]}`) {
		t.Fatal("mentions not disabled")
	}
	unknown := webhookPayloadForQuota(QuotaStatus{Provider: "codex", ResetOccurred: true}, EventReset, "Reset", "", 0)
	if unknown.Utilization != nil {
		t.Fatal("unmeasured reset must not publish zero usage")
	}
}

func TestDiscordPresentationLimits(t *testing.T) {
	payload := WebhookPayload{Title: strings.Repeat("x", 300), Message: strings.Repeat("y", 5000), AccountName: strings.Repeat("z", 1500)}
	data, err := formatDiscordEmbed(payload)
	if err != nil {
		t.Fatal(err)
	}
	var msg discordWebhookMessage
	json.Unmarshal(data, &msg)
	e := msg.Embeds[0]
	if len([]rune(e.Title)) > 256 || len([]rune(e.Description)) > 1500 || len([]rune(e.Fields[0].Name)) > 256 {
		t.Fatal("oversized embed")
	}
}

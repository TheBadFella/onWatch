package notify

import (
	"encoding/json"
	"testing"
	"time"
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
	if embed.Title != payload.Title {
		t.Errorf("title = %q; want %q", embed.Title, payload.Title)
	}
	if embed.Color != 0xf59e0b {
		t.Errorf("color = %x; want %x", embed.Color, 0xf59e0b)
	}
	if len(embed.Fields) != 4 {
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

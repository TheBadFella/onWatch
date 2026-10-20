package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// isDiscordWebhook reports whether raw is a Discord webhook endpoint.
func isDiscordWebhook(urlStr string) bool {
	u := strings.ToLower(strings.TrimSpace(urlStr))
	return strings.Contains(u, "discord.com/api/webhooks/") || strings.Contains(u, "discordapp.com/api/webhooks/")
}

// discordEmbedField represents a field in a Discord embed.
type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// discordEmbed represents a rich embed in a Discord webhook payload.
type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Color       int                 `json:"color"`
	Timestamp   string              `json:"timestamp"`
	Fields      []discordEmbedField `json:"fields,omitempty"`
	Footer      *discordFooter      `json:"footer,omitempty"`
}

type discordFooter struct {
	Text string `json:"text"`
}

type discordWebhookMessage struct {
	Embeds []discordEmbed `json:"embeds"`
}

// discordColorForEvent returns a semantic decimal color for the notification event.
func discordColorForEvent(event string) int {
	switch event {
	case EventCritical, EventStarterFailure:
		return 0xef4444 // Red
	case EventWarning:
		return 0xf59e0b // Amber / Yellow
	case EventReset, EventStarterSuccess:
		return 0x10b981 // Green
	case EventAuthError:
		return 0xd97706 // Deep Orange
	case EventTest:
		return 0x8b5cf6 // Purple
	default:
		return 0x3b82f6 // Blue
	}
}

// formatDiscordEmbed serializes a WebhookPayload into Discord's native embed JSON format.
func formatDiscordEmbed(payload WebhookPayload) ([]byte, error) {
	color := discordColorForEvent(payload.Event)

	// Build prominent fields
	fields := make([]discordEmbedField, 0, 4)

	if payload.Provider != "" {
		fields = append(fields, discordEmbedField{
			Name:   "Provider",
			Value:  strings.ToUpper(payload.Provider[:1]) + payload.Provider[1:],
			Inline: true,
		})
	}

	if payload.AccountID != "" {
		fields = append(fields, discordEmbedField{
			Name:   "Account",
			Value:  payload.AccountID,
			Inline: true,
		})
	}

	if payload.Utilization != nil {
		fields = append(fields, discordEmbedField{
			Name:   "Utilization",
			Value:  fmt.Sprintf("%.1f%%", *payload.Utilization),
			Inline: true,
		})
	}

	if payload.ResetAt != "" {
		if t, err := time.Parse(time.RFC3339, payload.ResetAt); err == nil {
			fields = append(fields, discordEmbedField{
				Name:   "Resets At",
				Value:  t.UTC().Format("15:04:05 UTC"),
				Inline: true,
			})
		}
	}

	ts := payload.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}

	title := payload.Title
	if title == "" {
		title = fmt.Sprintf("[%s] Notification", strings.ToUpper(payload.Event))
	}

	msg := discordWebhookMessage{
		Embeds: []discordEmbed{
			{
				Title:       title,
				Description: payload.Message,
				Color:       color,
				Timestamp:   ts,
				Fields:      fields,
				Footer: &discordFooter{
					Text: "onWatch Quota Monitor",
				},
			},
		},
	}

	return json.Marshal(msg)
}

// formatWebhookBody formats the payload if the destination requires custom formatting (e.g. Discord).
func formatWebhookBody(endpoint string, payload WebhookPayload, original []byte) ([]byte, error) {
	if isDiscordWebhook(endpoint) {
		return formatDiscordEmbed(payload)
	}
	return original, nil
}

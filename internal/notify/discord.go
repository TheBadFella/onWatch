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
	Embeds          []discordEmbed `json:"embeds"`
	AllowedMentions struct {
		Parse []string `json:"parse"`
	} `json:"allowed_mentions"`
}

// discordColorForEvent returns a semantic decimal color for the notification event.
func discordColorForEvent(event string) int {
	switch event {
	case EventCritical, EventExhausted, EventStarterFailure:
		return 0xef4444 // Red
	case EventWarning, EventResetSoon, EventResetCreditExpiry:
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
	fields := make([]discordEmbedField, 0, 4)
	account := payload.AccountName
	if account == "" && payload.AccountID != "" {
		account = "Account " + payload.AccountID
	}
	lines := []string{}
	if payload.QuotaKey != "" && payload.Event != EventResetCreditExpiry {
		lines = append(lines, "**Limit:** "+discordText(quotaLabel(payload.QuotaKey)))
	}
	if payload.Reset != nil {
		lines = append(lines, "**Usage:** "+usageChange(payload.Reset.BeforePercent, payload.Reset.AfterPercent))
	} else if payload.Utilization != nil {
		lines = append(lines, fmt.Sprintf("**Used:** %.1f%% · **Remaining:** %.1f%%", *payload.Utilization, remainingPercent(*payload.Utilization)))
	}
	if account != "" || len(lines) > 0 {
		name := account
		if name == "" {
			name = providerLabel(payload.Provider)
		}
		fields = append(fields, discordEmbedField{Name: discordLimit(name, 256), Value: discordLimit(strings.Join(lines, "\n"), 1024)})
		if len(lines) == 0 {
			fields[len(fields)-1].Value = "Account notification"
		}
	}
	context := providerLabel(payload.Provider)
	if account != "" {
		context += " | " + account
	}
	if payload.QuotaKey != "" && payload.Event != EventResetCreditExpiry {
		context += " | " + quotaLabel(payload.QuotaKey)
	}
	title := payload.Title
	description := payload.Message
	if payload.Event == EventWarning || payload.Event == EventCritical {
		title = "⚠️ " + context + " | Usage warning"
		if payload.Event == EventCritical {
			title = "🚨 " + context + " | Limit almost reached"
		}
		description = "Review usage before continuing. The selected usage threshold has been reached."
	}
	if payload.Event == EventReset {
		title = "↻ " + context + " | Reset"
		description = "A new usage window was detected. The cause was not recorded."
		if payload.Reset != nil {
			description = resetCauseLabel(payload.Reset.Cause) + "."
			details := "**Previously due:** " + discordTime(payload.Reset.PreviousResetAt) + "\n**Next due:** " + discordTime(payload.Reset.NextResetAt)
			fields = append(fields, discordEmbedField{Name: "Reset details", Value: details})
			fields = append(fields, discordEmbedField{Name: "Evidence", Value: discordLimit(payload.Reset.Evidence, 1024)})
		}
	}
	if payload.Event == EventStarterSuccess {
		title = "▶ " + context + " | Window started"
	}
	if payload.Event == EventExhausted {
		title = "🚨 " + context + " | Quota exhausted"
		description = "Reported usage has reached 100%."
	}
	if payload.Event == EventResetSoon {
		title = "⌛ " + context + " | Window resets soon"
		description = "The provider-reported reset time is approaching."
	}
	if t, err := time.Parse(time.RFC3339, payload.ObservedAt); err == nil {
		fields = append(fields, discordEmbedField{Name: "Usage observed", Value: discordTime(&t)})
	}
	if payload.Event == EventStarterFailure {
		title = "⚠️ " + context + " | Window could not start"
	}
	if payload.Event == EventResetCreditExpiry {
		title = "⌛ " + context + " | Banked reset expires soon"
	}
	if payload.Reset == nil && payload.ResetAt != "" {
		if t, err := time.Parse(time.RFC3339, payload.ResetAt); err == nil {
			name := "Next reset"
			if payload.Event == EventResetCreditExpiry {
				name = "Expires"
			}
			fields = append(fields, discordEmbedField{Name: name, Value: discordTime(&t)})
		}
	}
	ts := payload.Timestamp
	if ts == "" {
		ts = time.Now().UTC().Format(time.RFC3339)
	}
	msg := discordWebhookMessage{Embeds: []discordEmbed{{Title: discordLimit(title, 256), Description: discordLimit(description, 1500), Color: discordColorForEvent(payload.Event), Timestamp: ts, Fields: fields, Footer: &discordFooter{Text: "OnWatch"}}}}
	msg.AllowedMentions.Parse = []string{}
	return json.Marshal(msg)
}

// Discord expands timestamps in each reader's timezone and keeps the date visible.
func discordTime(t *time.Time) string {
	if t == nil {
		return "Not reported"
	}
	return fmt.Sprintf("<t:%d:f>\n%s", t.Unix(), t.UTC().Format("2006-01-02 15:04:05 UTC"))
}

func discordLimit(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
func discordText(s string) string {
	return strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "`", "\\`", "~", "\\~").Replace(s)
}

// formatWebhookBody formats the payload if the destination requires custom formatting (e.g. Discord).
func formatWebhookBody(endpoint string, payload WebhookPayload, original []byte) ([]byte, error) {
	if isDiscordWebhook(endpoint) {
		return formatDiscordEmbed(payload)
	}
	return original, nil
}

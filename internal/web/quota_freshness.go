package web

import (
	"context"
	"fmt"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/menubar"
)

// CheckQuotaFreshness reuses the same account readings and age thresholds as
// the menubar. Manual and never-read accounts do not have a stale episode.
func (h *Handler) CheckQuotaFreshness(ctx context.Context) {
	data, err := h.menubarDataForRequest(ctx)
	if err != nil || ctx.Err() != nil {
		return
	}
	cards := append([]menubar.ProviderCard(nil), data.AllProviders...)
	h.applyMenubarStaleness(cards, data.Visibility)
	now := h.menubarNowTime()
	for _, card := range cards {
		provider := providerKeyBase(card.ID)
		if card.StaleAfterSeconds <= 0 || !h.providerPollingEnabled(provider, data.Visibility) {
			continue
		}
		at := card.StaleCheckedAt
		if at == "" {
			at = card.UpdatedAtTime
		}
		observed, err := time.Parse(time.RFC3339, at)
		if err != nil {
			continue
		}
		reconnect := card.ConnectionStatus == "reconnect" || card.ConnectionStatus == "paused"
		if !card.Stale && !(reconnect && now.Sub(observed) >= staleFloor) {
			continue
		}
		title := card.Label + " quota readings are stale"
		if reconnect {
			title = card.Label + " needs to reconnect"
		}
		message := fmt.Sprintf("The last quota reading was %s when this alert was raised.", observed.Format(time.RFC3339))
		if err := h.store.RecordQuotaFreshnessAlert(ctx, provider, card.ID, observed.UTC(), title, message); err != nil && ctx.Err() == nil {
			h.logger.Warn("Could not record quota freshness alert", "account", card.ID, "error", err)
		}
	}
}

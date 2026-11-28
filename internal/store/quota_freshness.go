package store

import (
	"context"
	"time"
)

// RecordQuotaFreshnessAlert records one dashboard alert for each last-good
// reading. Dismissing an alert does not rearm it. A new reading does.
func (s *Store) RecordQuotaFreshnessAlert(ctx context.Context, provider, card string, observed time.Time, title, message string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
		WHERE CAST(settings.value AS INTEGER) < CAST(excluded.value AS INTEGER)`,
		"quota_freshness_alert:"+card, observed.UnixNano())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count == 0 {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO system_alerts (provider, alert_type, title, message, severity, created_at, metadata)
		VALUES (?, 'quota_freshness', ?, ?, 'warning', ?, '')`,
		provider, title, message, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	return tx.Commit()
}

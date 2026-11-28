package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestQuotaFreshnessAlertOncePerReadingAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota.db")
	db, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	record := func(card string, observed time.Time) {
		t.Helper()
		if err := db.RecordQuotaFreshnessAlert(context.Background(), "codex", card, observed, card, "stale"); err != nil {
			t.Fatal(err)
		}
	}
	record("codex:1", at)
	if err := db.DismissAllSystemAlerts(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	record("codex:1", at)
	record("codex:1", at.Add(-time.Minute))
	record("codex:1", at.Add(time.Minute))
	record("codex:1", at.Add(time.Minute))
	record("codex:2", at)
	alerts, err := db.GetActiveSystemAlerts()
	if err != nil || len(alerts) != 2 {
		t.Fatalf("got %d active alerts: %v", len(alerts), err)
	}
}

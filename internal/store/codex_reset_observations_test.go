package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

func TestCodexResetObservations(t *testing.T) {
	for _, tc := range []struct {
		name, cause                                                   string
		scheduled, redeemed, baselineRedeemed, missingCredit, longGap bool
	}{
		{name: "early provider or manual unknown", cause: "early_unknown"},
		{name: "due window", cause: "scheduled", scheduled: true},
		{name: "observed banked redemption", cause: "banked_credit", redeemed: true},
		{name: "old redeemed baseline is not evidence", cause: "early_unknown", baselineRedeemed: true},
		{name: "disappearing credit is not evidence", cause: "early_unknown", missingCredit: true},
		{name: "credit observed across monitoring gap", cause: "unknown", redeemed: true, longGap: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := ":memory:"
			if tc.redeemed {
				dbPath = filepath.Join(t.TempDir(), "observations.db")
			}
			s, err := New(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			now := time.Date(2026, 10, 22, 17, 9, 1, 610000000, time.UTC)
			before := now.Add(-time.Minute)
			if tc.longGap {
				before = now.Add(-time.Hour)
			}
			oldDue := now.Add(3 * time.Hour)
			if tc.scheduled {
				oldDue = now.Add(-3 * time.Minute)
			}
			nextDue := now.Add(5 * time.Hour)
			if _, err := s.CreateCodexCycle(6, "five_hour", before, &oldDue); err != nil {
				t.Fatal(err)
			}
			if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{AccountID: 6, CapturedAt: before, Quotas: []api.CodexQuota{{Name: "five_hour", Utilization: 98, ResetsAt: &oldDue}}}); err != nil {
				t.Fatal(err)
			}
			credit := CodexResetCreditRecord{CreditID: "fixture", Status: "available", ResetType: "codex_rate_limits"}
			if tc.baselineRedeemed {
				credit.Status = "redeemed"
			}
			if err := s.SyncCodexResetCredits(6, []CodexResetCreditRecord{credit}, before); err != nil {
				t.Fatal(err)
			}
			if tc.redeemed {
				credit.Status = "redeemed"
				if err := s.SyncCodexResetCredits(6, []CodexResetCreditRecord{credit}, now); err != nil {
					t.Fatal(err)
				}
			}
			if tc.missingCredit {
				if err := s.SyncCodexResetCredits(6, nil, now); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.CloseCodexCycle(6, "five_hour", now, 98, 10); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CreateCodexCycle(6, "five_hour", now, &nextDue); err != nil {
				t.Fatal(err)
			}
			if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{AccountID: 6, CapturedAt: now, Quotas: []api.CodexQuota{{Name: "five_hour", Utilization: 0, ResetsAt: &nextDue}}}); err != nil {
				t.Fatal(err)
			}
			events, err := s.QueryCodexResetObservations(6, 8)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 {
				t.Fatalf("events = %d", len(events))
			}
			o := events[0]
			if o.Cause != tc.cause {
				t.Errorf("cause=%s want=%s", o.Cause, tc.cause)
			}
			if o.BeforePercent == nil || *o.BeforePercent != 98 || o.AfterPercent == nil || *o.AfterPercent != 0 {
				t.Fatalf("usage = %+v", o)
			}
			if !o.PreviousResetAt.Equal(oldDue) || !o.NextResetAt.Equal(nextDue) {
				t.Fatal("lost reset deadlines")
			}
			other, err := s.QueryCodexResetObservations(9, 8)
			if err != nil || len(other) != 0 {
				t.Fatal("cross-account history")
			}
			// Refreshing a redeemed row must not make an old redemption appear new.
			if err := s.SyncCodexResetCredits(6, []CodexResetCreditRecord{credit}, now.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			later, _ := s.QueryCodexResetObservations(6, 8)
			if later[0].Cause != tc.cause {
				t.Fatal("classification drifted on refresh")
			}
			if tc.redeemed {
				s.Close()
				reopened, err := New(dbPath)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				persisted, err := reopened.QueryCodexResetObservations(6, 8)
				if err != nil || len(persisted) != 1 || persisted[0].Cause != tc.cause || !persisted[0].CreditRedeemedAt.Equal(now) {
					t.Fatalf("redemption did not survive restart: %v %+v", err, persisted)
				}
			}
		})
	}
	t.Run("missing deadline", func(t *testing.T) {
		o := CodexResetObservation{}
		o.classify()
		if o.Cause != "unknown" {
			t.Fatal(o.Cause)
		}
	})
}

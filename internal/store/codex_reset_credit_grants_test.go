package store

import (
	"testing"
	"time"
)

// The first successful fetch establishes a baseline and must not replay the
// credits it already contained.
func TestEstablishCodexResetCreditGrantBaseline_SuppressesExisting(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	expiry := rcTime(t, "2026-11-05T12:00:00Z")
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available", GrantedAt: &now, ExpiresAt: expiry},
		{CreditID: "credit-2", Status: "redeemed"},
	}, now); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	established, err := s.EstablishCodexResetCreditGrantBaseline(1, now)
	if err != nil {
		t.Fatalf("Establish: %v", err)
	}
	if !established {
		t.Fatal("first baseline = false, want true")
	}
	pending, err := s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after baseline = %d, want 0: existing credits must not be replayed", len(pending))
	}

	again, err := s.EstablishCodexResetCreditGrantBaseline(1, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("Establish second: %v", err)
	}
	if again {
		t.Fatal("second baseline = true, want false")
	}
	baseline, err := s.QueryCodexResetCreditGrantBaseline(1)
	if err != nil {
		t.Fatalf("Query baseline: %v", err)
	}
	if baseline == nil || !baseline.BaselineAt.Equal(now) {
		t.Fatalf("baseline = %#v, want the first timestamp %v", baseline, now)
	}
}

// A new credit ID is detected even when the total available count did not
// change, because detection is per ID and not per count.
func TestCodexResetCreditGrant_NewIDWithUnchangedCount(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available"},
		{CreditID: "credit-2", Status: "available"},
	}, now); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if _, err := s.EstablishCodexResetCreditGrantBaseline(1, now); err != nil {
		t.Fatalf("Establish: %v", err)
	}

	// Same number of rows (two), but credit-2 is replaced by credit-3.
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available"},
		{CreditID: "credit-3", Status: "available"},
	}, now.Add(30*time.Minute)); err != nil {
		t.Fatalf("Sync second: %v", err)
	}
	pending, err := s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query pending: %v", err)
	}
	if len(pending) != 1 || pending[0].CreditID != "credit-3" {
		t.Fatalf("pending = %#v, want only credit-3", pending)
	}
}

// An empty baseline is still a baseline: a credit granted later is announced,
// while the baseline fetch itself never alerts.
func TestCodexResetCreditGrant_EmptyBaselineThenGrant(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := s.SyncCodexResetCredits(1, nil, now); err != nil {
		t.Fatalf("Sync empty: %v", err)
	}
	established, err := s.EstablishCodexResetCreditGrantBaseline(1, now)
	if err != nil {
		t.Fatalf("Establish: %v", err)
	}
	if !established {
		t.Fatal("empty baseline = false, want true")
	}
	pending, err := s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after empty baseline = %d, want 0", len(pending))
	}

	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "granted-later", Status: "available"},
	}, now.Add(time.Hour)); err != nil {
		t.Fatalf("Sync grant: %v", err)
	}
	pending, err = s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query pending after grant: %v", err)
	}
	if len(pending) != 1 || pending[0].CreditID != "granted-later" {
		t.Fatalf("pending = %#v, want granted-later", pending)
	}
}

// Claims are atomic, so repeated polls and restarts send at most one grant
// message; releasing the exact claim makes a failed delivery retryable.
func TestClaimCodexResetCreditGrantNotice_DedupesAndReleases(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available"},
	}, now); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	first := now.Add(time.Minute)
	claimed, err := s.ClaimCodexResetCreditGrantNotice(1, "credit-1", first)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim = false, want true")
	}
	claimed, err = s.ClaimCodexResetCreditGrantNotice(1, "credit-1", first.Add(time.Minute))
	if err != nil {
		t.Fatalf("Claim second: %v", err)
	}
	if claimed {
		t.Fatal("second claim = true, want false")
	}

	// A mismatched release must not clear the newer claim.
	if err := s.ReleaseCodexResetCreditGrantNotice(1, "credit-1", first.Add(time.Minute)); err != nil {
		t.Fatalf("Release mismatched: %v", err)
	}
	claimed, err = s.ClaimCodexResetCreditGrantNotice(1, "credit-1", first.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("Claim after mismatched release: %v", err)
	}
	if claimed {
		t.Fatal("claim after mismatched release = true, want false")
	}

	// The exact claim can be released for a retry.
	if err := s.ReleaseCodexResetCreditGrantNotice(1, "credit-1", first); err != nil {
		t.Fatalf("Release: %v", err)
	}
	claimed, err = s.ClaimCodexResetCreditGrantNotice(1, "credit-1", first.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("Claim after release: %v", err)
	}
	if !claimed {
		t.Fatal("claim after release = false, want true")
	}
}

// Accounts are isolated: a grant on one account is never pending on another.
func TestCodexResetCreditGrant_PerAccountIsolation(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	for _, accountID := range []int64{1, 2} {
		if _, err := s.EstablishCodexResetCreditGrantBaseline(accountID, now); err != nil {
			t.Fatalf("Establish account %d: %v", accountID, err)
		}
	}
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available"},
	}, now.Add(time.Hour)); err != nil {
		t.Fatalf("Sync account 1: %v", err)
	}

	pending1, err := s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query account 1: %v", err)
	}
	if len(pending1) != 1 {
		t.Fatalf("account 1 pending = %d, want 1", len(pending1))
	}
	pending2, err := s.QueryCodexResetCreditsNeedingGrantNotice(2, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query account 2: %v", err)
	}
	if len(pending2) != 0 {
		t.Fatalf("account 2 pending = %d, want 0", len(pending2))
	}
}

// A credit that disappeared and reappeared keeps its announcement marker, so
// restarts and transient detail responses do not replay the grant. Unsupported
// credits are excluded from grant alerts.
func TestCodexResetCreditGrant_ReappearanceAndUnsupported(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if _, err := s.EstablishCodexResetCreditGrantBaseline(1, now); err != nil {
		t.Fatalf("Establish: %v", err)
	}
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available"},
		{CreditID: "unsupported", Status: "available", UnsupportedByPlan: true},
	}, now.Add(time.Minute)); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	pending, err := s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query pending: %v", err)
	}
	if len(pending) != 1 || pending[0].CreditID != "credit-1" {
		t.Fatalf("pending = %#v, want only credit-1", pending)
	}

	claimedAt := now.Add(2 * time.Minute)
	if _, err := s.ClaimCodexResetCreditGrantNotice(1, "credit-1", claimedAt); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	// The detail response briefly omits the credit, then returns it again.
	if err := s.SyncCodexResetCredits(1, nil, now.Add(3*time.Minute)); err != nil {
		t.Fatalf("Sync gone: %v", err)
	}
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "credit-1", Status: "available"},
	}, now.Add(4*time.Minute)); err != nil {
		t.Fatalf("Sync reappear: %v", err)
	}
	pending, err = s.QueryCodexResetCreditsNeedingGrantNotice(1, time.Now().UTC())
	if err != nil {
		t.Fatalf("Query after reappearance: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending after reappearance = %d, want 0", len(pending))
	}
}

// Merging duplicate accounts preserves existing grant markers and moves the
// baseline; credits older than the baseline are suppressed, while a credit first
// seen after it stays pending.
func TestMergeCodexAccountData_PreservesGrantDedupeAndBaseline(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	target, err := s.CreateOrRestoreProviderAccount("codex", "merge-target")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	source, err := s.CreateOrRestoreProviderAccount("codex", "merge-source")
	if err != nil {
		t.Fatalf("create source: %v", err)
	}

	base := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if err := s.SyncCodexResetCredits(target.ID, []CodexResetCreditRecord{
		{CreditID: "credit-a", Status: "available"},
	}, base); err != nil {
		t.Fatalf("sync target: %v", err)
	}
	if _, err := s.EstablishCodexResetCreditGrantBaseline(target.ID, base); err != nil {
		t.Fatalf("establish baseline: %v", err)
	}

	// Source has no baseline. credit-b predates the target baseline; credit-c is
	// newer, so it must still be announced after the merge.
	if err := s.SyncCodexResetCredits(source.ID, []CodexResetCreditRecord{
		{CreditID: "credit-b", Status: "available"},
	}, base.Add(-time.Hour)); err != nil {
		t.Fatalf("sync source historical: %v", err)
	}
	if err := s.SyncCodexResetCredits(source.ID, []CodexResetCreditRecord{
		{CreditID: "credit-b", Status: "available"},
		{CreditID: "credit-c", Status: "available"},
	}, base.Add(time.Hour)); err != nil {
		t.Fatalf("sync source new: %v", err)
	}

	if err := s.MergeCodexAccountData(target.ID, source.ID); err != nil {
		t.Fatalf("merge: %v", err)
	}
	pending, err := s.QueryCodexResetCreditsNeedingGrantNotice(target.ID, base.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("query pending: %v", err)
	}
	if len(pending) != 1 || pending[0].CreditID != "credit-c" {
		t.Fatalf("pending = %#v, want only credit-c", pending)
	}

	baseline, err := s.QueryCodexResetCreditGrantBaseline(target.ID)
	if err != nil || baseline == nil || !baseline.BaselineAt.Equal(base) {
		t.Fatalf("target baseline = %#v, %v; want %v", baseline, err, base)
	}
	if sourceBaseline, err := s.QueryCodexResetCreditGrantBaseline(source.ID); err != nil || sourceBaseline != nil {
		t.Fatalf("source baseline = %#v, %v; want removed", sourceBaseline, err)
	}
	rows, err := s.QueryCodexResetCredits(target.ID)
	if err != nil {
		t.Fatalf("query credits: %v", err)
	}
	byID := map[string]CodexResetCreditRecord{}
	for _, row := range rows {
		byID[row.CreditID] = row
	}
	if byID["credit-a"].GrantNotifiedAt == nil {
		t.Fatal("credit-a grant marker lost in merge")
	}
	if byID["credit-b"].GrantNotifiedAt == nil {
		t.Fatal("credit-b older than the baseline was not suppressed")
	}
	if byID["credit-c"].GrantNotifiedAt != nil {
		t.Fatal("credit-c newer than the baseline was wrongly suppressed")
	}
}

// A credit whose known expiry has already passed is not announced as new, while
// an unknown expiry is still announced without a concrete date.
func TestCodexResetCreditGrant_SkipsKnownExpiredAndKeepsUnknownExpiry(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	if _, err := s.EstablishCodexResetCreditGrantBaseline(1, now); err != nil {
		t.Fatalf("establish: %v", err)
	}
	past := now.Add(-time.Hour)
	if err := s.SyncCodexResetCredits(1, []CodexResetCreditRecord{
		{CreditID: "expired", Status: "available", ExpiresAt: &past},
		{CreditID: "unknown", Status: "available", ExpiryUnknown: true},
	}, now.Add(time.Minute)); err != nil {
		t.Fatalf("sync: %v", err)
	}
	pending, err := s.QueryCodexResetCreditsNeedingGrantNotice(1, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(pending) != 1 || pending[0].CreditID != "unknown" {
		t.Fatalf("pending = %#v, want only the unknown-expiry credit", pending)
	}
}

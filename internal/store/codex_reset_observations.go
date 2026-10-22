package store

import (
	"database/sql"
	"fmt"
	"time"
)

// CodexResetObservation reconstructs a reset from persisted cycle and usage
// samples. Cause is evidence-based; an early reset alone cannot identify an actor.
type CodexResetObservation struct {
	AccountID          int64      `json:"accountId"`
	QuotaName          string     `json:"quotaName"`
	ObservedAt         time.Time  `json:"observedAt"`
	PreviousObservedAt *time.Time `json:"previousObservedAt,omitempty"`
	PreviousResetAt    *time.Time `json:"previousResetAt,omitempty"`
	NextResetAt        *time.Time `json:"nextResetAt,omitempty"`
	BeforePercent      *float64   `json:"beforePercent,omitempty"`
	AfterPercent       *float64   `json:"afterPercent,omitempty"`
	CreditRedeemedAt   *time.Time `json:"creditRedeemedAt,omitempty"`
	Cause              string     `json:"cause"`
	Evidence           string     `json:"evidence"`
}

func (o *CodexResetObservation) classify() {
	switch {
	case o.PreviousObservedAt == nil || o.BeforePercent == nil || o.AfterPercent == nil:
		o.Cause = "unknown"
		o.Evidence = "The usage samples around this reset are unavailable, so its cause cannot be classified."
	case o.ObservedAt.Sub(*o.PreviousObservedAt) > 5*time.Minute:
		o.Cause = "unknown"
		o.Evidence = "A monitoring gap of more than five minutes spans this reset. Timing or a credit change cannot be linked reliably to the reset."
	case o.CreditRedeemedAt != nil:
		o.Cause = "banked_credit"
		o.Evidence = "The provider reported a banked reset changing to redeemed between these usage samples. The reset was observed in the same interval."
	case o.PreviousResetAt == nil:
		o.Cause = "unknown"
		o.Evidence = "The previous scheduled reset time was not recorded, so the cause cannot be classified."
	case !o.ObservedAt.Before(o.PreviousResetAt.Add(-2 * time.Minute)):
		o.Cause = "scheduled"
		o.Evidence = "The reset was observed when the previous window was due to end. This classification is based on timing; the provider does not identify the actor."
	default:
		o.Cause = "early_unknown"
		o.Evidence = "Usage reset before the previous scheduled time. No banked redemption was confirmed in this observation interval; a provider-issued or manual reset cannot be distinguished."
	}
}

// QueryCodexResetObservations is bounded and also works for cycles recorded
// before reset reporting was added. It never backfills invented credit history.
func (s *Store) QueryCodexResetObservations(accountID int64, limit int) ([]CodexResetObservation, error) {
	if accountID == 0 {
		accountID = DefaultCodexAccountID
	}
	if limit < 1 || limit > 20 {
		limit = 8
	}
	rows, err := s.db.Query(`
 WITH cycles AS (
  SELECT c.account_id, c.quota_name, c.resets_at AS previous_reset, n.cycle_start AS observed
  FROM codex_reset_cycles c JOIN codex_reset_cycles n ON n.id = (
   SELECT id FROM codex_reset_cycles x WHERE x.account_id=c.account_id AND x.quota_name=c.quota_name AND x.id>c.id ORDER BY x.id LIMIT 1
  )
  WHERE c.account_id=? AND c.cycle_end IS NOT NULL ORDER BY c.id DESC LIMIT ?
 ), samples AS (
  SELECT cycles.*,
   (SELECT s.captured_at FROM codex_snapshots s JOIN codex_quota_values q ON q.snapshot_id=s.id WHERE s.account_id=cycles.account_id AND q.quota_name=cycles.quota_name AND s.captured_at<cycles.observed ORDER BY s.captured_at DESC LIMIT 1) AS before_at,
   (SELECT q.utilization FROM codex_snapshots s JOIN codex_quota_values q ON q.snapshot_id=s.id WHERE s.account_id=cycles.account_id AND q.quota_name=cycles.quota_name AND s.captured_at<cycles.observed ORDER BY s.captured_at DESC LIMIT 1) AS before_pct,
   (SELECT q.utilization FROM codex_snapshots s JOIN codex_quota_values q ON q.snapshot_id=s.id WHERE s.account_id=cycles.account_id AND q.quota_name=cycles.quota_name AND s.captured_at=cycles.observed LIMIT 1) AS after_pct,
   (SELECT q.resets_at FROM codex_snapshots s JOIN codex_quota_values q ON q.snapshot_id=s.id WHERE s.account_id=cycles.account_id AND q.quota_name=cycles.quota_name AND s.captured_at=cycles.observed LIMIT 1) AS next_reset
  FROM cycles
 ) SELECT account_id,quota_name,observed,previous_reset,before_at,before_pct,after_pct,next_reset,
  (SELECT MIN(redeemed_at) FROM codex_reset_credits r WHERE r.account_id=samples.account_id AND r.reset_type='codex_rate_limits' AND r.redeemed_at>=samples.before_at AND r.redeemed_at<=samples.observed AND r.unsupported_by_plan=0)
 FROM samples ORDER BY observed DESC`, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("query Codex reset observations: %w", err)
	}
	defer rows.Close()
	out := make([]CodexResetObservation, 0)
	for rows.Next() {
		var o CodexResetObservation
		var observed string
		var previous, beforeAt, next, redeemed sql.NullString
		var before, after sql.NullFloat64
		if err := rows.Scan(&o.AccountID, &o.QuotaName, &observed, &previous, &beforeAt, &before, &after, &next, &redeemed); err != nil {
			return nil, err
		}
		var err error
		o.ObservedAt, err = time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return nil, err
		}
		parse := func(v sql.NullString) *time.Time {
			if !v.Valid {
				return nil
			}
			t, err := time.Parse(time.RFC3339Nano, v.String)
			if err != nil {
				return nil
			}
			return &t
		}
		o.PreviousResetAt = parse(previous)
		o.PreviousObservedAt = parse(beforeAt)
		o.NextResetAt = parse(next)
		o.CreditRedeemedAt = parse(redeemed)
		if before.Valid {
			o.BeforePercent = &before.Float64
		}
		if after.Valid {
			o.AfterPercent = &after.Float64
		}
		o.classify()
		out = append(out, o)
	}
	return out, rows.Err()
}

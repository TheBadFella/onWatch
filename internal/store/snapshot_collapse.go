package store

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
)

// snapshotRun names a provider's snapshot table, its per-snapshot value table
// and the column scoping a run to one account ("" when there is one account).
type snapshotRun struct {
	snapshots, values, account string
}

var (
	anthropicRun   = snapshotRun{"anthropic_snapshots", "anthropic_quota_values", ""}
	antigravityRun = snapshotRun{"antigravity_snapshots", "antigravity_model_values", "account_id"}
	codexRun       = snapshotRun{"codex_snapshots", "codex_quota_values", "account_id"}
	commandCodeRun = snapshotRun{"commandcode_snapshots", "commandcode_quota_values", "account_id"}
)

// comparedColumns caches, per table, the columns that make up a reading:
// everything except row identity and capture time.
var comparedColumns sync.Map

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func columnsToCompare(q queryer, table string) (string, error) {
	cols, err := compareColumnList(q, table)
	return strings.Join(cols, ", "), err
}

func compareColumnList(q queryer, table string) ([]string, error) {
	if cols, ok := comparedColumns.Load(table); ok {
		return cols.([]string), nil
	}
	rows, err := q.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		switch name {
		case "id", "captured_at", "snapshot_id":
		default:
			cols = append(cols, `"`+name+`"`)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	comparedColumns.Store(table, cols)
	return cols, nil
}

// collapseRepeatedSnapshot keeps only the first and newest row of a run of
// identical readings. Polling once a minute, most snapshots repeat the one
// before; keeping every one made flat periods most of the database. Once the
// three newest rows for the account match, the middle one adds nothing: the
// first marks when the reading began and the newest when it was last seen, so
// charts, the latest poll time and the last reading before a change (which
// Codex reset evidence relies on) are unchanged.
func collapseRepeatedSnapshot(tx *sql.Tx, run snapshotRun, account any, newID int64) error {
	where, args := "", []any{}
	if run.account != "" {
		where, args = ` WHERE "`+run.account+`" = ?`, []any{account}
	}
	rows, err := tx.Query(`SELECT id FROM `+run.snapshots+where+` ORDER BY id DESC LIMIT 3`, args...)
	if err != nil {
		return fmt.Errorf("query recent %s: %w", run.snapshots, err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) < 3 || ids[0] != newID {
		return nil
	}
	for _, pair := range [][2]int64{{ids[0], ids[1]}, {ids[1], ids[2]}} {
		same, err := sameReading(tx, run, pair[0], pair[1])
		if err != nil || !same {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM `+run.values+` WHERE snapshot_id = ?`, ids[1]); err != nil {
		return fmt.Errorf("delete repeated %s: %w", run.values, err)
	}
	if _, err := tx.Exec(`DELETE FROM `+run.snapshots+` WHERE id = ?`, ids[1]); err != nil {
		return fmt.Errorf("delete repeated %s: %w", run.snapshots, err)
	}
	return nil
}

// sameReading compares two snapshots' stored fields and their value rows.
// EXCEPT treats NULLs as equal, which is what a repeated reading means.
func sameReading(tx *sql.Tx, run snapshotRun, a, b int64) (bool, error) {
	snapCols, err := columnsToCompare(tx, run.snapshots)
	if err != nil {
		return false, err
	}
	valueCols, err := columnsToCompare(tx, run.values)
	if err != nil {
		return false, err
	}
	snap := `SELECT ` + snapCols + ` FROM ` + run.snapshots + ` WHERE id = ?`
	vals := `SELECT ` + valueCols + ` FROM ` + run.values + ` WHERE snapshot_id = ?`
	var differs bool
	err = tx.QueryRow(`SELECT
		EXISTS(`+snap+` EXCEPT `+snap+`) OR
		EXISTS(`+vals+` EXCEPT `+vals+`) OR
		EXISTS(`+vals+` EXCEPT `+vals+`) OR
		(SELECT COUNT(*) FROM `+run.values+` WHERE snapshot_id = ?) <> (SELECT COUNT(*) FROM `+run.values+` WHERE snapshot_id = ?)`,
		a, b, a, b, b, a, a, b,
	).Scan(&differs)
	if err != nil {
		return false, fmt.Errorf("compare %s readings: %w", run.snapshots, err)
	}
	return !differs, nil
}

// snapshotRunsCollapsedKey marks that collapseExistingRuns has run, so the
// full scan happens once per database.
const snapshotRunsCollapsedKey = "snapshot_runs_collapsed_v1"

// collapseExistingRuns applies collapseRepeatedSnapshot's rule to history
// stored before it existed: inside each run of identical readings for an
// account, only the first and last rows are kept. Every value and the time it
// changed survive. It returns the number of snapshots removed.
func (s *Store) collapseExistingRuns() (int64, error) {
	if done, err := s.GetSetting(snapshotRunsCollapsedKey); err != nil || done != "" {
		return 0, err
	}
	var removed int64
	for _, run := range []snapshotRun{anthropicRun, antigravityRun, codexRun, commandCodeRun} {
		ids, err := s.repeatedRunMiddles(run)
		if err != nil {
			return removed, err
		}
		if err := s.deleteSnapshots(run, ids); err != nil {
			return removed, err
		}
		removed += int64(len(ids))
	}
	return removed, s.SetSetting(snapshotRunsCollapsedKey, "1")
}

// repeatedRunMiddles fingerprints every snapshot (its stored fields plus its
// sorted value rows) in one pass and returns the ids strictly inside runs.
func (s *Store) repeatedRunMiddles(run snapshotRun) ([]int64, error) {
	quoted := func(cols []string, alias string) string {
		parts := make([]string, len(cols))
		for i, c := range cols {
			parts[i] = "quote(" + alias + "." + c + ")"
		}
		return strings.Join(parts, " || '|' || ")
	}
	snapCols, err := compareColumnList(s.db, run.snapshots)
	if err != nil {
		return nil, err
	}
	valueCols, err := compareColumnList(s.db, run.values)
	if err != nil {
		return nil, err
	}
	account := "0"
	if run.account != "" {
		account = `quote(s."` + run.account + `")`
	}
	rows, err := s.db.Query(`SELECT s.id, ` + account + `, ` + quoted(snapCols, "s") + ` || '#' || COALESCE((
		SELECT group_concat(fp, ';') FROM (SELECT ` + quoted(valueCols, "v") + ` AS fp FROM ` + run.values + ` v WHERE v.snapshot_id = s.id ORDER BY fp)
	), '') FROM ` + run.snapshots + ` s ORDER BY s.id`)
	if err != nil {
		return nil, fmt.Errorf("fingerprint %s: %w", run.snapshots, err)
	}
	defer rows.Close()
	type seen struct {
		id     int64
		fp     string
		inside bool // previous row matched the one before it
	}
	last := map[string]*seen{}
	var middles []int64
	for rows.Next() {
		var id int64
		var acct, fp string
		if err := rows.Scan(&id, &acct, &fp); err != nil {
			return nil, err
		}
		prev := last[acct]
		if prev != nil && prev.fp == fp {
			if prev.inside {
				middles = append(middles, prev.id)
			}
			last[acct] = &seen{id: id, fp: fp, inside: true}
			continue
		}
		last[acct] = &seen{id: id, fp: fp}
	}
	return middles, rows.Err()
}

func (s *Store) deleteSnapshots(run snapshotRun, ids []int64) error {
	const batch = 500
	for start := 0; start < len(ids); start += batch {
		chunk := ids[start:min(start+batch, len(ids))]
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}
		in := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM `+run.values+` WHERE snapshot_id IN (`+in+`)`, args...); err != nil {
			tx.Rollback()
			return fmt.Errorf("delete repeated %s: %w", run.values, err)
		}
		if _, err := tx.Exec(`DELETE FROM `+run.snapshots+` WHERE id IN (`+in+`)`, args...); err != nil {
			tx.Rollback()
			return fmt.Errorf("delete repeated %s: %w", run.snapshots, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

package store

import (
	"fmt"
	"strings"
)

// anthropicStatuslineMarker is the only part of anthropic_snapshots.raw_json
// any reader uses: it tells statusline rows from API rows.
const anthropicStatuslineMarker = `"_source":"statusline"`

// anthropicStoredRawJSON keeps statusline rows' raw_json and drops the API
// response body, which nothing reads.
func anthropicStoredRawJSON(raw string) string {
	if strings.Contains(raw, anthropicStatuslineMarker) {
		return raw
	}
	return ""
}

// compactUnreadRawJSON clears provider response bodies older builds stored for
// Codex, Antigravity, Command Code and Anthropic API polls. Nothing reads them,
// and at one poll a minute they made up most of the database file. Usage values
// live in their own columns and are untouched.
func (s *Store) compactUnreadRawJSON() (int64, error) {
	stmts := []string{
		`UPDATE codex_snapshots SET raw_json = '' WHERE raw_json <> ''`,
		`UPDATE antigravity_snapshots SET raw_json = '' WHERE raw_json <> ''`,
		`UPDATE commandcode_snapshots SET raw_json = '' WHERE raw_json <> ''`,
		`UPDATE anthropic_snapshots SET raw_json = '' WHERE raw_json <> '' AND raw_json NOT LIKE '%` + anthropicStatuslineMarker + `%'`,
	}
	var cleared int64
	for _, stmt := range stmts {
		res, err := s.db.Exec(stmt)
		if err != nil {
			return cleared, fmt.Errorf("clear raw_json: %w", err)
		}
		n, _ := res.RowsAffected()
		cleared += n
	}
	return cleared, nil
}

// compactStorage runs the startup cleanups and, when either removed anything,
// one VACUUM so the file actually shrinks. Later starts find nothing to do.
func (s *Store) compactStorage() error {
	cleared, err := s.compactUnreadRawJSON()
	if err != nil {
		return err
	}
	collapsed, err := s.collapseExistingRuns()
	if err != nil {
		return err
	}
	if cleared+collapsed > 0 {
		if _, err := s.db.Exec(`VACUUM`); err != nil {
			return fmt.Errorf("vacuum after storage cleanup: %w", err)
		}
	}
	return nil
}

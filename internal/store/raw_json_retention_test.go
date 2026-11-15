package store

import (
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

func rawJSONColumn(t *testing.T, s *Store, table string) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT COALESCE(raw_json, '') FROM ` + table + ` ORDER BY id`)
	if err != nil {
		t.Fatalf("query %s: %v", table, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
	}
	return out
}

// No reader uses these provider response bodies, and at one poll a minute they
// were most of the database. Only the Anthropic statusline marker is read.
func TestSnapshotInsertsSkipUnreadRawJSON(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now().UTC()
	body := `{"big":"provider response"}`
	statusline := `{"_source":"statusline","rate_limits":{}}`

	if _, err := s.InsertCodexSnapshot(&api.CodexSnapshot{CapturedAt: now, RawJSON: body}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{CapturedAt: now, RawJSON: body}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertCommandCodeSnapshot(&api.CommandCodeSnapshot{CapturedAt: now, RawJSON: body}); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{body, statusline} {
		if _, err := s.InsertAnthropicSnapshot(&api.AnthropicSnapshot{CapturedAt: now, RawJSON: raw}); err != nil {
			t.Fatal(err)
		}
	}

	for _, table := range []string{"codex_snapshots", "antigravity_snapshots", "commandcode_snapshots"} {
		if got := rawJSONColumn(t, s, table); len(got) != 1 || got[0] != "" {
			t.Fatalf("%s stored raw_json: %q", table, got)
		}
	}
	if got := rawJSONColumn(t, s, "anthropic_snapshots"); len(got) != 2 || got[0] != "" || got[1] != statusline {
		t.Fatalf("anthropic raw_json = %q, want API body dropped and statusline kept", got)
	}
}

func TestCompactUnreadRawJSONClearsExistingRows(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	statusline := `{"_source":"statusline"}`
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO codex_snapshots (captured_at, raw_json, quota_count) VALUES (?, '{"a":1}', 0)`, []any{ts}},
		{`INSERT INTO antigravity_snapshots (captured_at, raw_json) VALUES (?, '{"a":1}')`, []any{ts}},
		{`INSERT INTO commandcode_snapshots (captured_at, raw_json) VALUES (?, '{"a":1}')`, []any{ts}},
		{`INSERT INTO anthropic_snapshots (captured_at, raw_json, quota_count) VALUES (?, '{"a":1}', 0)`, []any{ts}},
		{`INSERT INTO anthropic_snapshots (captured_at, raw_json, quota_count) VALUES (?, ?, 0)`, []any{ts, statusline}},
	} {
		if _, err := s.db.Exec(q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}

	cleared, err := s.compactUnreadRawJSON()
	if err != nil {
		t.Fatal(err)
	}
	if cleared != 4 {
		t.Fatalf("cleared %d rows, want 4", cleared)
	}
	if got := rawJSONColumn(t, s, "anthropic_snapshots"); got[0] != "" || got[1] != statusline {
		t.Fatalf("anthropic raw_json = %q", got)
	}
	if cleared, err = s.compactUnreadRawJSON(); err != nil || cleared != 0 {
		t.Fatalf("second pass cleared %d, err %v; want a no-op", cleared, err)
	}
}

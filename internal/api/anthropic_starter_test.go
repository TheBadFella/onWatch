package api

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAnthropicStarterCommandUsesPollingToken(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "wrong-key")
	t.Setenv("ANTHROPIC_BASE_URL", "https://gateway.invalid")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "wrong-account")
	t.Setenv("ENABLE_CLAUDEAI_MCP_SERVERS", "true")
	t.Setenv("OPENROUTER_API_KEY", "unrelated-key")
	cmd := anthropicStarterCommand(t.Context(), "polling-token")
	env := make(map[string]bool)
	for _, entry := range cmd.Environ() {
		env[entry] = true
	}
	for _, entry := range []string{
		"ANTHROPIC_API_KEY=wrong-key", "ANTHROPIC_BASE_URL=https://gateway.invalid",
		"CLAUDE_CODE_USE_BEDROCK=1", "CLAUDE_CODE_OAUTH_TOKEN=wrong-account",
		"ENABLE_CLAUDEAI_MCP_SERVERS=true",
		"OPENROUTER_API_KEY=unrelated-key",
	} {
		if env[entry] {
			t.Errorf("starter inherited %s", entry)
		}
	}
	if !env["CLAUDE_CODE_OAUTH_TOKEN=polling-token"] || !env["ENABLE_CLAUDEAI_MCP_SERVERS=false"] {
		t.Fatal("starter must use the polling token with cloud connectors disabled")
	}
}

func TestAnthropicStarterRunsClaudeAndChecksResult(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake Claude executable uses a Unix shell")
	}
	dir := t.TempDir()
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	path := filepath.Join(dir, "claude")
	client := NewAnthropicClient("starter-test-token", slog.Default())
	for _, result := range []struct {
		name, json string
		success    bool
	}{
		{"success", `{"type":"result","subtype":"success","is_error":false}`, true},
		{"provider error", `{"type":"result","subtype":"error","is_error":true}`, false},
	} {
		t.Run(result.name, func(t *testing.T) {
			script := "#!/bin/sh\n" +
				`[ "$CLAUDE_CODE_OAUTH_TOKEN" = "starter-test-token" ] || exit 1` + "\n" +
				`printf '%s\n' '` + result.json + "'\n"
			if err := os.WriteFile(path, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			if err := client.StartFiveHourWindow(t.Context()); (err == nil) != result.success {
				t.Fatalf("success = %v, got %v", result.success, err)
			}
		})
	}
}

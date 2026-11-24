package api

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

// StartFiveHourWindow sends one prompt through Claude Code using the same
// subscription token as the usage poll.
func (c *AnthropicClient) StartFiveHourWindow(ctx context.Context) error {
	token := c.getToken()
	if token == "" {
		return errors.New("Claude subscription token is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := anthropicStarterCommand(ctx, token)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("Claude Code is required on the onWatch daemon's PATH")
	}
	if err != nil {
		// Claude Code output can include credential or configuration details.
		return errors.New("Claude Code did not complete the starter prompt")
	}
	var result struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		IsError bool   `json:"is_error"`
	}
	if json.Unmarshal(output, &result) != nil || result.Type != "result" || result.Subtype != "success" || result.IsError {
		return errors.New("Claude Code did not return a successful starter result")
	}
	return nil
}

func anthropicStarterCommand(ctx context.Context, token string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "claude", "--print", "--safe-mode",
		"--setting-sources", "", "--no-session-persistence", "--tools", "",
		"--disallowed-tools", "mcp__*",
		"--model", "haiku", "--system-prompt", "Reply with OK.",
		"--output-format", "json", "OK?")
	cmd.WaitDelay = 2 * time.Second
	// Keep OS and network settings without passing other providers' credentials
	// or Claude settings that could select a different account or endpoint.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(key) {
		case "PATH", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT",
			"TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME",
			"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR", "NODE_EXTRA_CA_CERTS":
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "CLAUDE_CODE_OAUTH_TOKEN="+token, "DISABLE_AUTOUPDATER=1",
		"CLAUDE_CODE_MAX_RETRIES=0", "ENABLE_CLAUDEAI_MCP_SERVERS=false")
	return cmd
}

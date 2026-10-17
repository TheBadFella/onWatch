package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeAuthJSON writes a Codex-format auth.json under the sandboxed HOME and
// returns its path.
func writeAuthJSON(t *testing.T, accessToken, refreshToken, idToken, accountID string) string {
	t.Helper()

	authDir := filepath.Join(testHomeDir(t), ".codex")
	if err := os.MkdirAll(authDir, 0o700); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	raw := map[string]interface{}{
		"tokens": map[string]interface{}{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"id_token":      idToken,
			"account_id":    accountID,
		},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal auth.json: %v", err)
	}
	path := filepath.Join(authDir, "auth.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	return path
}

func readAuthRefreshToken(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	var raw struct {
		Tokens struct {
			RefreshToken string `json:"refresh_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal auth.json: %v", err)
	}
	return raw.Tokens.RefreshToken
}

func readProfileRefreshToken(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read profile: %v", err)
	}
	var profile CodexProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("unmarshal profile: %v", err)
	}
	return profile.Tokens.RefreshToken
}

// A named profile whose own token is expiring borrows the global auth.json
// credentials. When it rotates that borrowed one-time-use refresh token, the
// rotation must be written back to auth.json as well as the profile file, or
// the Codex CLI is left holding a spent token and gets logged out.
func TestCodexNamedProfile_BorrowedAuthJSONWriteBack(t *testing.T) {
	fx := newCodexManagerFixture(t)

	const account = "acct-work"
	const user = "user-work"
	validIDToken := makeCodexIDToken(t, time.Now().Add(24*time.Hour), account, user)
	authPath := writeAuthJSON(t, "sys-access", "sys-refresh", validIDToken, account)

	// The profile's own access token is opaque and its id_token has expired, so
	// the profile credentials read as expiring and the agent borrows auth.json.
	profile := CodexProfile{Name: "work", AccountID: account, UserID: user}
	profile.Tokens.AccessToken = "opaque-not-a-jwt"
	profile.Tokens.RefreshToken = "profile-refresh"
	profile.Tokens.IDToken = makeCodexIDToken(t, time.Now().Add(-1*time.Hour), account, user)
	profilePath := fx.writeProfile(t, profile)

	if err := fx.manager.startAgentForProfile(profile); err != nil {
		t.Fatalf("startAgentForProfile: %v", err)
	}
	waitUntil(t, time.Second, func() bool { return fx.instance("work") != nil }, "work profile to start")
	agent := fx.instance("work").Agent

	// The proactive path reads credentials first, which is where the borrow is
	// recorded.
	creds := agent.credsRefresh()
	if creds == nil || creds.AccessToken != "sys-access" {
		t.Fatalf("credsRefresh() = %+v, want the borrowed auth.json token", creds)
	}

	if err := agent.tokenSave("rotated-access", "rotated-refresh", validIDToken, 604800); err != nil {
		t.Fatalf("tokenSave: %v", err)
	}

	if got := readProfileRefreshToken(t, profilePath); got != "rotated-refresh" {
		t.Fatalf("profile refresh_token = %q, want rotated-refresh", got)
	}
	if got := readAuthRefreshToken(t, authPath); got != "rotated-refresh" {
		t.Fatalf("auth.json refresh_token = %q, want rotated-refresh (rotation must be written back)", got)
	}
}

// A named profile using its own valid token never borrowed from auth.json, so
// the save must leave auth.json byte-identical.
func TestCodexNamedProfile_OwnTokenLeavesAuthJSONUntouched(t *testing.T) {
	fx := newCodexManagerFixture(t)

	const account = "acct-own"
	const user = "user-own"
	ownAccess := makeCodexIDToken(t, time.Now().Add(7*24*time.Hour), account, user)
	validIDToken := makeCodexIDToken(t, time.Now().Add(24*time.Hour), account, user)
	authPath := writeAuthJSON(t, "sys-access", "sys-refresh", validIDToken, account)
	origAuth, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}

	profile := CodexProfile{Name: "own", AccountID: account, UserID: user}
	profile.Tokens.AccessToken = ownAccess
	profile.Tokens.RefreshToken = "own-refresh"
	profile.Tokens.IDToken = validIDToken
	profilePath := fx.writeProfile(t, profile)

	if err := fx.manager.startAgentForProfile(profile); err != nil {
		t.Fatalf("startAgentForProfile: %v", err)
	}
	waitUntil(t, time.Second, func() bool { return fx.instance("own") != nil }, "own profile to start")
	agent := fx.instance("own").Agent

	creds := agent.credsRefresh()
	if creds == nil || creds.AccessToken != ownAccess {
		t.Fatalf("credsRefresh() = %+v, want the profile's own token", creds)
	}

	if err := agent.tokenSave("rotated-access", "rotated-refresh", validIDToken, 604800); err != nil {
		t.Fatalf("tokenSave: %v", err)
	}

	if got := readProfileRefreshToken(t, profilePath); got != "rotated-refresh" {
		t.Fatalf("profile refresh_token = %q, want rotated-refresh", got)
	}
	afterAuth, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	if string(afterAuth) != string(origAuth) {
		t.Fatalf("auth.json changed for a profile using its own token:\ngot: %s\nwant: %s", afterAuth, origAuth)
	}
}

// Writing the rotation back to auth.json must not look like an external
// credential change to the profile scanner: the scanner only tracks the
// profile file's mtime, so its bookkeeping must stay stable across the
// write-back and no restart is triggered.
func TestCodexNamedProfile_BorrowedWriteBackKeepsScannerStable(t *testing.T) {
	fx := newCodexManagerFixture(t)

	const account = "acct-stable"
	const user = "user-stable"
	validIDToken := makeCodexIDToken(t, time.Now().Add(24*time.Hour), account, user)
	writeAuthJSON(t, "sys-access", "sys-refresh", validIDToken, account)

	profile := CodexProfile{Name: "stable", AccountID: account, UserID: user}
	profile.Tokens.AccessToken = "opaque-not-a-jwt"
	profile.Tokens.RefreshToken = "profile-refresh"
	profile.Tokens.IDToken = makeCodexIDToken(t, time.Now().Add(-1*time.Hour), account, user)
	profilePath := fx.writeProfile(t, profile)

	if err := fx.manager.startAgentForProfile(profile); err != nil {
		t.Fatalf("startAgentForProfile: %v", err)
	}
	waitUntil(t, time.Second, func() bool { return fx.instance("stable") != nil }, "stable profile to start")
	agent := fx.instance("stable").Agent

	if creds := agent.credsRefresh(); creds == nil || creds.RefreshToken != "sys-refresh" {
		t.Fatalf("credsRefresh() = %+v, want the borrowed auth.json token", creds)
	}

	fx.manager.mu.RLock()
	before := fx.manager.lastScanProfiles["stable"]
	fx.manager.mu.RUnlock()

	if err := agent.tokenSave("rotated-access", "rotated-refresh", validIDToken, 604800); err != nil {
		t.Fatalf("tokenSave: %v", err)
	}

	fx.manager.mu.RLock()
	after := fx.manager.lastScanProfiles["stable"]
	fx.manager.mu.RUnlock()

	info, err := os.Stat(profilePath)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	// The scanner bookkeeping must equal the profile file's current mtime, so
	// the next scan does not see a modification and restart the agent.
	if !after.Equal(info.ModTime()) {
		t.Fatalf("lastScanProfiles = %v, want the profile mtime %v", after, info.ModTime())
	}
	if before.IsZero() {
		t.Fatal("profile mtime was never recorded before the write-back")
	}
}

// If the CLI logged in again after the borrow, auth.json holds a newer refresh
// token and must not be overwritten by onWatch's rotation.
func TestCodexNamedProfile_BorrowedWriteBackSkippedWhenAuthJSONChanged(t *testing.T) {
	fx := newCodexManagerFixture(t)

	const account = "acct-race"
	const user = "user-race"
	validIDToken := makeCodexIDToken(t, time.Now().Add(24*time.Hour), account, user)
	authPath := writeAuthJSON(t, "sys-access", "sys-refresh", validIDToken, account)

	profile := CodexProfile{Name: "race", AccountID: account, UserID: user}
	profile.Tokens.AccessToken = "opaque-not-a-jwt"
	profile.Tokens.RefreshToken = "profile-refresh"
	profile.Tokens.IDToken = makeCodexIDToken(t, time.Now().Add(-1*time.Hour), account, user)
	profilePath := fx.writeProfile(t, profile)

	if err := fx.manager.startAgentForProfile(profile); err != nil {
		t.Fatalf("startAgentForProfile: %v", err)
	}
	waitUntil(t, time.Second, func() bool { return fx.instance("race") != nil }, "race profile to start")
	agent := fx.instance("race").Agent

	if creds := agent.credsRefresh(); creds == nil || creds.RefreshToken != "sys-refresh" {
		t.Fatalf("credsRefresh() = %+v, want the borrowed auth.json token", creds)
	}

	// The user re-authenticated with the CLI: auth.json now holds a newer token.
	writeAuthJSON(t, "cli-access", "cli-newer-refresh", validIDToken, account)

	if err := agent.tokenSave("rotated-access", "rotated-refresh", validIDToken, 604800); err != nil {
		t.Fatalf("tokenSave: %v", err)
	}

	// The profile file still gets the rotation...
	if got := readProfileRefreshToken(t, profilePath); got != "rotated-refresh" {
		t.Fatalf("profile refresh_token = %q, want rotated-refresh", got)
	}
	// ...but the CLI's newer auth.json token is preserved.
	if got := readAuthRefreshToken(t, authPath); got != "cli-newer-refresh" {
		t.Fatalf("auth.json refresh_token = %q, want cli-newer-refresh (must not overwrite a newer login)", got)
	}
}

package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestParseRawAuthData(t *testing.T) {
	// Standard auth.json format
	authJSON := `{
		"auth_mode": "chatgpt",
		"tokens": {
			"access_token": "acc_123",
			"refresh_token": "ref_456",
			"id_token": "id_789",
			"account_id": "acc-guid"
		}
	}`

	creds := parseRawAuthData(authJSON)
	if creds == nil {
		t.Fatalf("expected non-nil credentials")
	}
	if creds.AccessToken != "acc_123" {
		t.Errorf("access_token = %q; want %q", creds.AccessToken, "acc_123")
	}
	if creds.RefreshToken != "ref_456" {
		t.Errorf("refresh_token = %q; want %q", creds.RefreshToken, "ref_456")
	}
	if creds.AccountID != "acc-guid" {
		t.Errorf("account_id = %q; want %q", creds.AccountID, "acc-guid")
	}

	// Invalid input
	if got := parseRawAuthData("not-json"); got != nil {
		t.Errorf("expected nil for invalid json, got %+v", got)
	}
}

func TestCodexProfileSaveWithAuthData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "codex-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	h := &Handler{}

	payload := map[string]string{
		"name": "testprofile",
		"auth_data": `{
			"tokens": {
				"access_token": "token_a",
				"refresh_token": "token_r",
				"account_id": "acc-1"
			}
		}`,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/codex/profiles", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.CodexProfiles(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify profile file was created
	profilesDir := codexProfilesDir()
	savedPath := filepath.Join(profilesDir, "testprofile.json")
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("expected profile file at %s, got error: %v", savedPath, err)
	}
}

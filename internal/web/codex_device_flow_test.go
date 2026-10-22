package web

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type deviceAuthTransport func(*http.Request) (*http.Response, error)

func (f deviceAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCodexDeviceAuthExchangesAuthorizationCode(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	exchanges := 0
	idToken := "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"https://api.openai.com/auth":{"chatgpt_account_id":"workspace-test","chatgpt_user_id":"user-test"}}`)) + ".sig"
	http.DefaultClient = &http.Client{Transport: deviceAuthTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"authorization_code":"code-test","code_verifier":"verifier-test","code_challenge":"challenge-test"}`
		if r.URL.Path == "/oauth/token" {
			exchanges++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			for k, want := range map[string]string{"grant_type": "authorization_code", "code": "code-test", "code_verifier": "verifier-test", "redirect_uri": "https://auth.openai.com/deviceauth/callback"} {
				if r.Form.Get(k) != want {
					t.Errorf("%s = %q, want %q", k, r.Form.Get(k), want)
				}
			}
			tokens, _ := json.Marshal(map[string]string{"access_token": "access-test", "refresh_token": "refresh-test", "id_token": idToken})
			body = string(tokens)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}
	h := &Handler{}
	rr := httptest.NewRecorder()
	h.CodexDeviceAuthPoll(rr, httptest.NewRequest(http.MethodPost, "/api/codex/device/poll", strings.NewReader(`{"device_auth_id":"device-test","user_code":"user-code","profile_name":"device-test"}`)))
	if rr.Code != 200 {
		t.Fatalf("status=%d: %s", rr.Code, rr.Body.String())
	}
	if exchanges != 1 {
		t.Fatalf("token exchanges=%d, want 1", exchanges)
	}
	data, err := os.ReadFile(filepath.Join(codexProfilesDir(), "device-test.json"))
	if err != nil {
		t.Fatal(err)
	}
	var profile CodexProfile
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.Tokens.AccessToken != "access-test" || profile.Tokens.RefreshToken != "refresh-test" || profile.AccountID != "workspace-test" || profile.UserID != "user-test" {
		t.Fatal("profile missing credentials or identity")
	}
}

func TestCodexDeviceAuthFailurePreservesProfile(t *testing.T) {
	for _, tc := range []struct {
		name, pollBody, exchangeBody           string
		pollStatus, exchangeStatus, wantStatus int
	}{
		{"pending", `{}`, `{}`, 403, 200, 200},
		{"not-approved", `{}`, `{}`, 404, 200, 200},
		{"invalid-request", `{}`, `{}`, 400, 200, 400},
		{"missing-code", `{}`, `{}`, 200, 200, 502},
		{"exchange-rejected", `{"authorization_code":"code","code_verifier":"verifier"}`, `{}`, 200, 401, 502},
		{"missing-tokens", `{"authorization_code":"code","code_verifier":"verifier"}`, `{}`, 200, 200, 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			dir := codexProfilesDir()
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "existing.json")
			const original = `{"name":"existing","tokens":{"access_token":"old-access","refresh_token":"old-refresh"}}`
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			old := http.DefaultClient
			t.Cleanup(func() { http.DefaultClient = old })
			http.DefaultClient = &http.Client{Transport: deviceAuthTransport(func(r *http.Request) (*http.Response, error) {
				body, status := tc.pollBody, tc.pollStatus
				if r.URL.Path == "/oauth/token" {
					body, status = tc.exchangeBody, tc.exchangeStatus
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			rr := httptest.NewRecorder()
			(&Handler{}).CodexDeviceAuthPoll(rr, httptest.NewRequest(http.MethodPost, "/api/codex/device/poll", strings.NewReader(`{"device_auth_id":"device","user_code":"code","profile_name":"existing"}`)))
			if rr.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", rr.Code, tc.wantStatus)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != original {
				t.Fatal("failed authorization replaced existing credentials")
			}
		})
	}
}

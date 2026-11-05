package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const testCSRFToken = "b3f1c2d4e5a6978877665544332211ff"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeSource is a synthetic upstream: no network and no real agy process.
type fakeSource struct {
	mu         sync.Mutex
	calls      int
	body       []byte
	err        error
	quotaCalls int
	quotaBody  []byte
	quotaErr   error
}

func (f *fakeSource) FetchUserStatus(context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.body, nil
}

func (f *fakeSource) FetchQuotaSummary(context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.quotaCalls++
	if f.quotaErr != nil {
		return nil, f.quotaErr
	}
	return f.quotaBody, nil
}

func (f *fakeSource) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeSource) quotaCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.quotaCalls
}

const fakeUserStatus = `{"userStatus":{"email":"second@example.com","planStatus":{"availablePromptCredits":500},"cascadeModelConfigData":{"clientModelConfigs":[{"label":"Claude Sonnet","modelOrAlias":{"model":"claude-4-5-sonnet"},"quotaInfo":{"remainingFraction":0.75,"resetTime":"2026-02-24T12:00:00Z"}}]}}}`

const fakeQuotaSummary = `{"response":{"groups":[{"displayName":"Gemini models","buckets":[{"bucketId":"gemini-5h","displayName":"5h Limit","remainingFraction":0.5,"resetTime":"2026-02-24T12:00:00Z"},{"bucketId":"gemini-weekly","displayName":"Weekly Limit","remainingFraction":0.8,"resetTime":"2026-02-28T12:00:00Z"}]},{"displayName":"Claude + GPT models","buckets":[{"bucketId":"3p-5h","displayName":"5h Limit","remainingFraction":0.75,"resetTime":"2026-02-24T12:00:00Z"},{"bucketId":"3p-weekly","displayName":"Weekly Limit","remainingFraction":0.9,"resetTime":"2026-02-28T12:00:00Z"}]}]}}`

func newTestBridge(t *testing.T, src bridgeSource) (*bridge, *time.Time) {
	t.Helper()
	// Use a dry logger so tests never write to the real process output.
	b := newBridge(src, testCSRFToken, discardLogger())
	clock := time.Date(2026, 2, 24, 12, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return clock }
	return b, &clock
}

func doStatusRequest(t *testing.T, h http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	return doBridgeRequest(t, h, userStatusPath, token)
}

func doQuotaRequest(t *testing.T, h http.Handler, token string) *httptest.ResponseRecorder {
	t.Helper()
	return doBridgeRequest(t, h, quotaSummaryPath, token)
}

func doBridgeRequest(t *testing.T, h http.Handler, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"metadata":{}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connect-Protocol-Version", "1")
	if token != "" {
		req.Header.Set(csrfHeader, token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCSRFValidation(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		wantStatus int
		wantCalls  int
	}{
		{"valid token", testCSRFToken, http.StatusOK, 1},
		{"missing token", "", http.StatusUnauthorized, 0},
		{"wrong token", "not-the-token", http.StatusUnauthorized, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeSource{body: []byte(fakeUserStatus)}
			b, _ := newTestBridge(t, src)

			rec := doStatusRequest(t, b.handler(), tc.token)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := src.callCount(); got != tc.wantCalls {
				t.Fatalf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
			if rec.Code == http.StatusOK {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				if strings.TrimSpace(rec.Body.String()) != fakeUserStatus {
					t.Errorf("body = %s, want %s", rec.Body.String(), fakeUserStatus)
				}
			}
		})
	}
}

func TestResponseCacheHonors45Seconds(t *testing.T) {
	src := &fakeSource{body: []byte(fakeUserStatus)}
	b, clock := newTestBridge(t, src)
	h := b.handler()

	first := doStatusRequest(t, h, testCSRFToken)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}
	if got := src.callCount(); got != 1 {
		t.Fatalf("after first request upstream calls = %d, want 1", got)
	}

	*clock = clock.Add(44 * time.Second)
	second := doStatusRequest(t, h, testCSRFToken)
	if second.Code != http.StatusOK {
		t.Fatalf("cached status = %d, want 200", second.Code)
	}
	if got := src.callCount(); got != 1 {
		t.Fatalf("within cache window upstream calls = %d, want 1", got)
	}
	if second.Body.String() != first.Body.String() {
		t.Errorf("cached body changed: %q vs %q", second.Body.String(), first.Body.String())
	}

	*clock = clock.Add(time.Second) // exactly 45s after the first fetch
	third := doStatusRequest(t, h, testCSRFToken)
	if third.Code != http.StatusOK {
		t.Fatalf("refetch status = %d, want 200", third.Code)
	}
	if got := src.callCount(); got != 2 {
		t.Fatalf("after cache expiry upstream calls = %d, want 2", got)
	}
}

func TestQuotaSummaryCSRFValidation(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		wantStatus int
		wantCalls  int
	}{
		{"valid token", testCSRFToken, http.StatusOK, 1},
		{"missing token", "", http.StatusUnauthorized, 0},
		{"wrong token", "not-the-token", http.StatusUnauthorized, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := &fakeSource{quotaBody: []byte(fakeQuotaSummary)}
			b, _ := newTestBridge(t, src)

			rec := doQuotaRequest(t, b.handler(), tc.token)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := src.quotaCallCount(); got != tc.wantCalls {
				t.Fatalf("upstream calls = %d, want %d", got, tc.wantCalls)
			}
			if rec.Code == http.StatusOK {
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
				if strings.TrimSpace(rec.Body.String()) != fakeQuotaSummary {
					t.Errorf("body = %s, want %s", rec.Body.String(), fakeQuotaSummary)
				}
			}
		})
	}
}

func TestQuotaSummaryResponseCacheHonors45Seconds(t *testing.T) {
	src := &fakeSource{quotaBody: []byte(fakeQuotaSummary)}
	b, clock := newTestBridge(t, src)
	h := b.handler()

	first := doQuotaRequest(t, h, testCSRFToken)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", first.Code)
	}
	if got := src.quotaCallCount(); got != 1 {
		t.Fatalf("after first request upstream calls = %d, want 1", got)
	}

	*clock = clock.Add(44 * time.Second)
	second := doQuotaRequest(t, h, testCSRFToken)
	if second.Code != http.StatusOK {
		t.Fatalf("cached status = %d, want 200", second.Code)
	}
	if got := src.quotaCallCount(); got != 1 {
		t.Fatalf("within cache window upstream calls = %d, want 1", got)
	}
	if second.Body.String() != first.Body.String() {
		t.Errorf("cached body changed: %q vs %q", second.Body.String(), first.Body.String())
	}

	*clock = clock.Add(time.Second) // exactly 45s after the first fetch
	third := doQuotaRequest(t, h, testCSRFToken)
	if third.Code != http.StatusOK {
		t.Fatalf("refetch status = %d, want 200", third.Code)
	}
	if got := src.quotaCallCount(); got != 2 {
		t.Fatalf("after cache expiry upstream calls = %d, want 2", got)
	}
}

func TestQuotaSummaryRunnerErrorReturns503(t *testing.T) {
	src := &fakeSource{quotaErr: errors.New("upstream locked")}
	b, _ := newTestBridge(t, src)

	rec := doQuotaRequest(t, b.handler(), testCSRFToken)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "upstream RetrieveUserQuotaSummary failed") {
		t.Errorf("body = %q, want upstream failure message", rec.Body.String())
	}

	// A failed fetch must not be cached, so the next poll retries upstream.
	doQuotaRequest(t, b.handler(), testCSRFToken)
	if got := src.quotaCallCount(); got != 2 {
		t.Fatalf("upstream calls after failure = %d, want 2", got)
	}
}

func TestQuotaSummaryInvalidUpstreamJSONReturns503(t *testing.T) {
	src := &fakeSource{quotaBody: []byte("not json")}
	b, _ := newTestBridge(t, src)

	rec := doQuotaRequest(t, b.handler(), testCSRFToken)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestRunnerErrorReturns503(t *testing.T) {
	src := &fakeSource{err: errors.New("upstream locked")}
	b, _ := newTestBridge(t, src)

	rec := doStatusRequest(t, b.handler(), testCSRFToken)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "upstream GetUserStatus failed") {
		t.Errorf("body = %q, want upstream failure message", rec.Body.String())
	}

	// A failed fetch must not be cached, so the next poll retries upstream.
	doStatusRequest(t, b.handler(), testCSRFToken)
	if got := src.callCount(); got != 2 {
		t.Fatalf("upstream calls after failure = %d, want 2", got)
	}
}

func TestInvalidUpstreamJSONReturns503(t *testing.T) {
	src := &fakeSource{body: []byte("not json")}
	b, _ := newTestBridge(t, src)

	rec := doStatusRequest(t, b.handler(), testCSRFToken)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestHandlerRejectsNonPost(t *testing.T) {
	b, _ := newTestBridge(t, &fakeSource{body: []byte(fakeUserStatus)})

	req := httptest.NewRequest(http.MethodGet, userStatusPath, nil)
	req.Header.Set(csrfHeader, testCSRFToken)
	rec := httptest.NewRecorder()
	b.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != http.MethodPost {
		t.Errorf("Allow = %q, want POST", allow)
	}
}

func TestUnknownRouteReturns404(t *testing.T) {
	b, _ := newTestBridge(t, &fakeSource{body: []byte(fakeUserStatus)})

	req := httptest.NewRequest(http.MethodPost, "/exa.language_server_pb.LanguageServerService/Other", nil)
	rec := httptest.NewRecorder()
	b.handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestProfileDirEnv(t *testing.T) {
	linux := profileDirEnvForOS("/home/user/.onwatch/antigravity-account2", "linux")
	if linux["HOME"] != "/home/user/.onwatch/antigravity-account2" {
		t.Errorf("HOME = %q", linux["HOME"])
	}
	if linux["XDG_CONFIG_HOME"] != "/home/user/.onwatch/antigravity-account2/.config" {
		t.Errorf("XDG_CONFIG_HOME = %q", linux["XDG_CONFIG_HOME"])
	}

	darwin := profileDirEnvForOS("/Users/user/agy-account2/", "darwin")
	if darwin["HOME"] != "/Users/user/agy-account2" {
		t.Errorf("darwin HOME = %q, want trailing separator trimmed", darwin["HOME"])
	}

	windows := profileDirEnvForOS(`C:\Users\user\agy-account2`, "windows")
	if windows["USERPROFILE"] != `C:\Users\user\agy-account2` {
		t.Errorf("USERPROFILE = %q", windows["USERPROFILE"])
	}
	if windows["APPDATA"] != `C:\Users\user\agy-account2\AppData\Roaming` {
		t.Errorf("APPDATA = %q", windows["APPDATA"])
	}
	if windows["LOCALAPPDATA"] != `C:\Users\user\agy-account2\AppData\Local` {
		t.Errorf("LOCALAPPDATA = %q", windows["LOCALAPPDATA"])
	}

	if env := profileDirEnvForOS("   ", "linux"); env != nil {
		t.Errorf("empty profile dir should yield nil env, got %v", env)
	}
}

func TestGenerateCSRFToken(t *testing.T) {
	a, err := generateCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := generateCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 64 {
		t.Errorf("token length = %d, want 64 hex chars for 32 bytes", len(a))
	}
	if a == b {
		t.Error("generated tokens must differ")
	}
}

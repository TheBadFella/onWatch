package agent

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
)

// newRecoveryRefreshAgent builds a paused-able agent whose usage endpoint
// rejects the stored token and whose OAuth refresh is driven by a seam, so no
// real endpoint is touched.
func newRecoveryRefreshAgent(t *testing.T, refresh codexOAuthRefreshFunc) (*CodexAgent, *fakeClock, *atomic.Int32) {
	t.Helper()
	// The usage endpoint rejects the stale token but accepts the token a
	// successful recovery refresh installs.
	var fetchCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer fresh-access" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"plan_type":"pro","rate_limit":{"primary_window":{"used_percent":25,"reset_at":1766000000,"limit_window_seconds":18000}}}`)
	}))
	t.Cleanup(server.Close)

	st, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	logger := slog.New(slog.DiscardHandler)
	client := api.NewCodexClient("stale-access", logger, api.WithCodexBaseURL(server.URL))
	tr := tracker.NewCodexTracker(st, logger)
	ag := NewCodexAgent(client, st, tr, time.Minute, logger, nil)

	// The access token is not near expiry, so the proactive refresh path never
	// fires: only the paused-state recovery refresh is exercised. The usage
	// endpoint still 401s because the token was revoked server-side.
	creds := &api.CodexCredentials{
		AccessToken:  "stale-access",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(48 * time.Hour),
		ExpiresIn:    48 * time.Hour,
	}
	ag.SetCredentialsRefresh(func() *api.CodexCredentials { return creds })
	ag.SetTokenRefresh(func() string { return "stale-access" })
	ag.SetCredentialFingerprint(func() string { return "fingerprint-v1" })
	ag.oauthRefresh = refresh
	// Never touch a real credential file from a test.
	ag.SetTokenSave(func(accessToken, refreshToken, idToken string, expiresIn int) error { return nil })

	clock := &fakeClock{now: time.Now()}
	ag.now = clock.Now
	return ag, clock, &fetchCalls
}

func pauseViaAuthFailures(t *testing.T, ag *CodexAgent) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < maxCodexAuthFailures; i++ {
		ag.poll(ctx)
	}
	if !ag.AuthPaused() {
		t.Fatalf("expected pause after %d auth failures", maxCodexAuthFailures)
	}
}

// A transient token-endpoint outage must not strand a profile: one refresh per
// recovery window is allowed, and a later success resumes polling.
func TestCodexAgent_RecoveryRefreshTransientThenSuccess(t *testing.T) {
	t.Parallel()
	var refreshCalls atomic.Int32
	ag, clock, _ := newRecoveryRefreshAgent(t, func(ctx context.Context, refreshToken string) (*api.CodexOAuthTokenResponse, error) {
		n := refreshCalls.Add(1)
		if n == 1 {
			return nil, fmt.Errorf("%w: HTTP 503", api.ErrCodexOAuthRefreshFailed)
		}
		return &api.CodexOAuthTokenResponse{
			AccessToken:  "fresh-access",
			RefreshToken: "fresh-refresh",
			ExpiresIn:    3600,
		}, nil
	})

	pauseViaAuthFailures(t, ag)
	ctx := context.Background()

	// First recovery window: one refresh, transient failure, stays paused.
	clock.Advance(codexAuthPausedRetryInterval + time.Minute)
	ag.poll(ctx)
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls after first window = %d, want 1", got)
	}
	if !ag.AuthPaused() {
		t.Fatal("a transient refresh failure must not lift the pause")
	}

	// Second recovery window: refresh succeeds and the pause lifts.
	clock.Advance(codexAuthPausedRetryMaxInterval)
	ag.poll(ctx)
	if got := refreshCalls.Load(); got != 2 {
		t.Fatalf("refresh calls after second window = %d, want 2", got)
	}
	if ag.AuthPaused() {
		t.Fatal("a successful recovery refresh must lift the pause")
	}
}

// A reused / invalid refresh token is terminal: no further refresh may be
// attempted, or onWatch burns the credential the Codex CLI still needs.
func TestCodexAgent_RecoveryRefreshReusedTokenStopsRefreshing(t *testing.T) {
	t.Parallel()
	var refreshCalls atomic.Int32
	ag, clock, _ := newRecoveryRefreshAgent(t, func(ctx context.Context, refreshToken string) (*api.CodexOAuthTokenResponse, error) {
		refreshCalls.Add(1)
		return nil, api.ErrCodexRefreshTokenReused
	})

	pauseViaAuthFailures(t, ag)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		clock.Advance(codexAuthPausedRetryMaxInterval)
		ag.poll(ctx)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want exactly 1 (dead refresh token must not be replayed)", got)
	}
	if !ag.AuthPaused() {
		t.Fatal("expected to stay paused with a dead refresh token")
	}
}

// An invalid_grant from the token endpoint is also terminal.
func TestCodexAgent_RecoveryRefreshInvalidGrantStopsRefreshing(t *testing.T) {
	t.Parallel()
	var refreshCalls atomic.Int32
	ag, clock, _ := newRecoveryRefreshAgent(t, func(ctx context.Context, refreshToken string) (*api.CodexOAuthTokenResponse, error) {
		refreshCalls.Add(1)
		return nil, api.ErrCodexRefreshTokenInvalid
	})

	pauseViaAuthFailures(t, ag)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		clock.Advance(codexAuthPausedRetryMaxInterval)
		ag.poll(ctx)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want exactly 1 for a terminal invalid_grant", got)
	}
}

// A fingerprint change (re-authentication) unblocks refresh even after a
// terminal failure, because the new credential source is a fresh token. Here
// the new credentials still fail server-side auth, so the agent re-pauses and
// must be allowed to refresh again rather than staying permanently blocked.
func TestCodexAgent_RecoveryRefreshUnblockedByFingerprintChange(t *testing.T) {
	t.Parallel()
	var refreshCalls atomic.Int32
	ag, clock, _ := newRecoveryRefreshAgent(t, func(ctx context.Context, refreshToken string) (*api.CodexOAuthTokenResponse, error) {
		refreshCalls.Add(1)
		return nil, api.ErrCodexRefreshTokenReused
	})
	// Swap the fingerprint to simulate a fresh login after the terminal failure.
	var fingerprint atomic.Value
	fingerprint.Store("fingerprint-v1")
	ag.SetCredentialFingerprint(func() string {
		v, _ := fingerprint.Load().(string)
		return v
	})

	// First credential: terminal failure blocks further refresh.
	pauseViaAuthFailures(t, ag)
	ctx := context.Background()
	clock.Advance(codexAuthPausedRetryMaxInterval)
	ag.poll(ctx)
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	clock.Advance(codexAuthPausedRetryMaxInterval)
	ag.poll(ctx)
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("terminal token replayed: refresh calls = %d, want 1", got)
	}

	// The user re-authenticates: a new fingerprint. The new credentials also
	// fail server-side, so after re-pausing the agent may refresh again.
	fingerprint.Store("fingerprint-v2")
	ag.lastToken = "" // force a token re-read on the next poll
	for i := 0; i < maxCodexAuthFailures+1; i++ {
		ag.poll(ctx)
	}
	if !ag.AuthPaused() {
		t.Fatal("expected to re-pause on the new credentials")
	}
	clock.Advance(codexAuthPausedRetryMaxInterval)
	ag.poll(ctx)
	if got := refreshCalls.Load(); got != 2 {
		t.Fatalf("refresh calls after re-auth = %d, want 2 (terminal block must clear for new creds)", got)
	}
}

// Two agents that share the same refresh token (same fingerprint) must not both
// consume the one-time-use token within a window.
func TestCodexRefreshCoordinator_SharedFingerprintSingleClaim(t *testing.T) {
	t.Parallel()
	coord := newCodexRefreshCoordinator()
	now := time.Now()

	if !coord.claim("shared-fp", now, codexAuthPausedRetryInterval) {
		t.Fatal("first claim should succeed")
	}
	if coord.claim("shared-fp", now.Add(time.Minute), codexAuthPausedRetryInterval) {
		t.Fatal("second claim within the window must be refused")
	}
	if !coord.claim("shared-fp", now.Add(codexAuthPausedRetryInterval+time.Second), codexAuthPausedRetryInterval) {
		t.Fatal("claim after the window should succeed")
	}
	if !coord.claim("other-fp", now, codexAuthPausedRetryInterval) {
		t.Fatal("a different fingerprint is independent")
	}
}

// End to end: two profiles sharing a refresh token refresh only once.
func TestCodexAgent_RecoveryRefreshSharedAcrossProfiles(t *testing.T) {
	t.Parallel()
	shared := newCodexRefreshCoordinator()

	var refreshCalls atomic.Int32
	mkRefresh := func() codexOAuthRefreshFunc {
		return func(ctx context.Context, refreshToken string) (*api.CodexOAuthTokenResponse, error) {
			refreshCalls.Add(1)
			return nil, fmt.Errorf("%w: HTTP 503", api.ErrCodexOAuthRefreshFailed)
		}
	}

	ag1, clock1, _ := newRecoveryRefreshAgent(t, mkRefresh())
	ag2, _, _ := newRecoveryRefreshAgent(t, mkRefresh())
	ag1.refreshCoordinator = shared
	ag2.refreshCoordinator = shared
	// Same clock so both recovery windows align.
	ag2.now = clock1.Now

	pauseViaAuthFailures(t, ag1)
	pauseViaAuthFailures(t, ag2)

	clock1.Advance(codexAuthPausedRetryInterval + time.Minute)
	ag1.poll(context.Background())
	ag2.poll(context.Background())

	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("shared refresh token consumed %d times, want 1", got)
	}
}

package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// userStatusPath is the Connect-RPC endpoint onWatch polls for one
	// Antigravity account.
	userStatusPath = "/exa.language_server_pb.LanguageServerService/GetUserStatus"
	csrfHeader     = "X-Codeium-Csrf-Token"

	// bridgeCacheTTL keeps a successful upstream response in memory long
	// enough to absorb repeated polling without tripping upstream rate limits.
	bridgeCacheTTL = 45 * time.Second
)

// userStatusSource fetches the upstream GetUserStatus payload.
type userStatusSource interface {
	FetchUserStatus(ctx context.Context) ([]byte, error)
}

// bridge serves the Antigravity GetUserStatus contract from one managed agy
// profile. Responses are cached in memory so onWatch can poll frequently
// without re-launching or re-reading the upstream account.
type bridge struct {
	source    userStatusSource
	csrfToken string
	cacheTTL  time.Duration
	logger    *slog.Logger
	now       func() time.Time

	mu       sync.Mutex
	cached   []byte
	cachedAt time.Time
}

func newBridge(source userStatusSource, csrfToken string, logger *slog.Logger) *bridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &bridge{
		source:    source,
		csrfToken: strings.TrimSpace(csrfToken),
		cacheTTL:  bridgeCacheTTL,
		logger:    logger,
		now:       time.Now,
	}
}

func (b *bridge) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(userStatusPath, b.handleUserStatus)
	return mux
}

func (b *bridge) handleUserStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeBridgeError(w, http.StatusMethodNotAllowed, "invalid_argument", "method not allowed")
		return
	}
	if b.csrfToken != "" {
		provided := r.Header.Get(csrfHeader)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(b.csrfToken)) != 1 {
			writeBridgeError(w, http.StatusUnauthorized, "unauthenticated", "invalid CSRF token")
			return
		}
	}

	// Hold the lock across the fetch so concurrent polls share one upstream
	// request instead of stampeding the runner.
	b.mu.Lock()
	defer b.mu.Unlock()

	now := b.now()
	if b.cached != nil && now.Sub(b.cachedAt) < b.cacheTTL {
		writeBridgeJSON(w, http.StatusOK, b.cached)
		return
	}

	body, err := b.source.FetchUserStatus(r.Context())
	if err != nil {
		b.logger.Warn("upstream GetUserStatus failed", "error", err)
		writeBridgeError(w, http.StatusServiceUnavailable, "unavailable", "upstream GetUserStatus failed")
		return
	}
	if !json.Valid(body) {
		b.logger.Warn("upstream GetUserStatus returned invalid JSON")
		writeBridgeError(w, http.StatusServiceUnavailable, "unavailable", "upstream returned invalid JSON")
		return
	}

	b.cached = append(b.cached[:0], body...)
	b.cachedAt = now
	writeBridgeJSON(w, http.StatusOK, b.cached)
}

func writeBridgeJSON(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeBridgeError(w http.ResponseWriter, status int, code, message string) {
	payload, err := json.Marshal(map[string]string{"code": code, "message": message})
	if err != nil {
		payload = []byte(`{"code":"unknown","message":"internal error"}`)
	}
	writeBridgeJSON(w, status, payload)
}

// profileDirEnv returns the environment overrides that keep one agy profile's
// credentials separate from the default account.
func profileDirEnv(profileDir string) map[string]string {
	return profileDirEnvForOS(profileDir, runtime.GOOS)
}

func profileDirEnvForOS(profileDir, goos string) map[string]string {
	dir := strings.TrimRight(strings.TrimSpace(profileDir), `\/`)
	if dir == "" {
		return nil
	}
	if goos == "windows" {
		return map[string]string{
			"USERPROFILE":  dir,
			"APPDATA":      dir + `\AppData\Roaming`,
			"LOCALAPPDATA": dir + `\AppData\Local`,
		}
	}
	return map[string]string{
		"HOME":            dir,
		"XDG_CONFIG_HOME": dir + "/.config",
	}
}

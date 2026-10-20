package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// newGeminiImportHandler returns a Handler with a live in-memory store.
func newGeminiImportHandler(t *testing.T) *Handler {
	t.Helper()
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return NewHandler(s, nil, nil, nil, nil)
}

func TestGeminiImportToken_MissingRefreshToken(t *testing.T) {
	h := newGeminiImportHandler(t)

	body, _ := json.Marshal(map[string]string{"access_token": "tok_abc"})
	req := httptest.NewRequest(http.MethodPost, "/api/gemini/import-token", bytes.NewReader(body))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()

	h.GeminiImportToken(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGeminiImportToken_MethodNotAllowed(t *testing.T) {
	h := newGeminiImportHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gemini/import-token", nil)
	w := httptest.NewRecorder()

	h.GeminiImportToken(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestGeminiImportToken_StoresRefreshToken(t *testing.T) {
	h := newGeminiImportHandler(t)

	// Provide only a refresh token; the handler attempts a live Google refresh
	// which will fail in tests (no real network), but it still persists the
	// refresh token and returns 200.
	payload := map[string]string{
		"refresh_token": "1//test-refresh-token",
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/gemini/import-token", bytes.NewReader(body))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()

	h.GeminiImportToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", resp)
	}

	// Verify the refresh token was persisted.
	_, storedRefresh, _, err := h.store.LoadGeminiTokens()
	if err != nil {
		t.Fatalf("LoadGeminiTokens: %v", err)
	}
	if storedRefresh != "1//test-refresh-token" {
		t.Fatalf("stored refresh token mismatch: %q", storedRefresh)
	}
}

func TestGeminiImportToken_AcceptsAccessAndRefresh(t *testing.T) {
	h := newGeminiImportHandler(t)

	payload := map[string]interface{}{
		"refresh_token": "1//rt",
		"access_token":  "ya29.at",
		"expires_at":    int64(9_999_999_999_000), // far future ms
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/gemini/import-token", bytes.NewReader(body))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()

	h.GeminiImportToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	storedAccess, storedRefresh, storedExpiry, err := h.store.LoadGeminiTokens()
	if err != nil {
		t.Fatalf("LoadGeminiTokens: %v", err)
	}
	if storedAccess != "ya29.at" {
		t.Fatalf("access token mismatch: %q", storedAccess)
	}
	if storedRefresh != "1//rt" {
		t.Fatalf("refresh token mismatch: %q", storedRefresh)
	}
	if storedExpiry != int64(9_999_999_999_000) {
		t.Fatalf("expiry mismatch: %d", storedExpiry)
	}
}

func TestGeminiImportToken_AcceptsExpiryDateFormat(t *testing.T) {
	h := newGeminiImportHandler(t)

	// expiry_date is the Gemini CLI oauth_creds.json key (ms).
	payload := map[string]interface{}{
		"refresh_token": "1//rt2",
		"access_token":  "ya29.at2",
		"expiry_date":   int64(8_888_888_888_000),
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/gemini/import-token", bytes.NewReader(body))
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()

	h.GeminiImportToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	_, _, storedExpiry, err := h.store.LoadGeminiTokens()
	if err != nil {
		t.Fatalf("LoadGeminiTokens: %v", err)
	}
	if storedExpiry != int64(8_888_888_888_000) {
		t.Fatalf("expiry_date not mapped correctly: %d", storedExpiry)
	}
}

func TestGeminiImportStatus_Empty(t *testing.T) {
	h := newGeminiImportHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gemini/import-status", nil)
	w := httptest.NewRecorder()

	h.GeminiImportStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["stored"] != false {
		t.Fatalf("expected stored=false, got %v", resp["stored"])
	}
}

func TestGeminiImportStatus_AfterSave(t *testing.T) {
	h := newGeminiImportHandler(t)

	if err := h.store.SaveGeminiTokens("ya29.at", "1//rt", 0); err != nil {
		t.Fatalf("SaveGeminiTokens: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/gemini/import-status", nil)
	w := httptest.NewRecorder()

	h.GeminiImportStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["stored"] != true {
		t.Fatalf("expected stored=true, got %v", resp["stored"])
	}
	if resp["has_access"] != true {
		t.Fatalf("expected has_access=true, got %v", resp["has_access"])
	}
}

func TestGeminiClearTokens(t *testing.T) {
	h := newGeminiImportHandler(t)

	if err := h.store.SaveGeminiTokens("ya29.at", "1//rt", 0); err != nil {
		t.Fatalf("SaveGeminiTokens: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/gemini/import-token", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()

	h.GeminiClearTokens(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	storedAccess, storedRefresh, _, err := h.store.LoadGeminiTokens()
	if err != nil {
		t.Fatalf("LoadGeminiTokens: %v", err)
	}
	if storedAccess != "" || storedRefresh != "" {
		t.Fatalf("tokens not cleared: access=%q refresh=%q", storedAccess, storedRefresh)
	}
}

func TestGeminiImportStatus_MethodNotAllowed(t *testing.T) {
	h := newGeminiImportHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/gemini/import-status", nil)
	w := httptest.NewRecorder()

	h.GeminiImportStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestGeminiClearTokens_MethodNotAllowed(t *testing.T) {
	h := newGeminiImportHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/gemini/import-token", nil)
	w := httptest.NewRecorder()

	h.GeminiClearTokens(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

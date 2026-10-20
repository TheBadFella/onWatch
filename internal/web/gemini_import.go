package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

// GeminiImportTokenRoute dispatches POST (import) and DELETE (clear) on
// /api/gemini/import-token to the appropriate handler.
func (h *Handler) GeminiImportTokenRoute(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.GeminiImportToken(w, r)
	case http.MethodDelete:
		h.GeminiClearTokens(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GeminiImportToken accepts a Google OAuth refresh token (and optionally an
// access token) via POST and persists it to the DB using SaveGeminiTokens.
// The existing Gemini agent reads the DB on the next poll cycle.
//
// POST /api/gemini/import-token
// Body: {"refresh_token":"...", "access_token":"..."}  (access_token optional)
func (h *Handler) GeminiImportToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusServiceUnavailable, "storage not available")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	// Accept either a bare token JSON or a full oauth_creds.json structure.
	var req struct {
		RefreshToken string `json:"refresh_token"`
		AccessToken  string `json:"access_token"`
		ExpiresAt    int64  `json:"expires_at"`   // Unix milliseconds (optional)
		ExpiryDate   int64  `json:"expiry_date"`  // Gemini CLI format (optional)
	}
	if err := json.Unmarshal(body, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		respondError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	accessToken := strings.TrimSpace(req.AccessToken)
	expiresAt := req.ExpiresAt
	if expiresAt == 0 && req.ExpiryDate > 0 {
		expiresAt = req.ExpiryDate
	}

	// If no access token provided, attempt a refresh now so the agent has
	// a valid access token immediately. Failure is non-fatal -- the agent
	// will refresh on its own next cycle.
	if accessToken == "" {
		clientCreds := api.DetectGeminiClientCredentials()
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		if tokenResp, err := api.RefreshGeminiToken(ctx, refreshToken, clientCreds.ClientID, clientCreds.ClientSecret); err == nil {
			accessToken = tokenResp.AccessToken
			if tokenResp.ExpiresIn > 0 {
				expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second).UnixMilli()
			}
		}
		// Ignore refresh error -- we still persist the refresh token.
	}

	if err := h.store.SaveGeminiTokens(accessToken, refreshToken, expiresAt); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to save tokens: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":            true,
		"provider":      "gemini",
		"has_access":    accessToken != "",
		"expires_at_ms": expiresAt,
	})
}

// GeminiImportStatus returns whether Gemini tokens are currently stored in DB.
//
// GET /api/gemini/import-status
func (h *Handler) GeminiImportStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"stored": false})
		return
	}

	accessToken, refreshToken, expiresAtMs, err := h.store.LoadGeminiTokens()
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"stored": false})
		return
	}

	stored := accessToken != "" || refreshToken != ""
	resp := map[string]interface{}{
		"stored":          stored,
		"has_access":      accessToken != "",
		"has_refresh":     refreshToken != "",
		"expires_at_ms":   expiresAtMs,
	}

	if stored && expiresAtMs > 0 {
		expiresIn := time.Until(time.UnixMilli(expiresAtMs)).Round(time.Minute)
		resp["expires_in_minutes"] = int(expiresIn.Minutes())
	}

	respondJSON(w, http.StatusOK, resp)
}

// GeminiClearTokens removes stored Gemini tokens from the DB.
//
// DELETE /api/gemini/import-token
func (h *Handler) GeminiClearTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusServiceUnavailable, "storage not available")
		return
	}

	// Overwrite with empty to clear.
	if err := h.store.SaveGeminiTokens("", "", 0); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to clear tokens: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

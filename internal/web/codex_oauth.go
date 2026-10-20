package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

const (
	openAIDeviceAuthUserCodeURL = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	openAIDeviceAuthTokenURL    = "https://auth.openai.com/api/accounts/deviceauth/token"
)

// CodexDeviceAuthStart starts an OpenAI device code authorization flow.
func (h *Handler) CodexDeviceAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	reqBody, _ := json.Marshal(map[string]string{
		"client_id": api.CodexOAuthClientID,
	})

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIDeviceAuthUserCodeURL, bytes.NewReader(reqBody))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create request: "+err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "codex-cli/0.155.1")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to contact OpenAI: "+err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		respondError(w, resp.StatusCode, "OpenAI error: "+string(body))
		return
	}

	var data struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Interval     string `json:"interval"`
		ExpiresAt    string `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to parse OpenAI response")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"device_auth_id":   data.DeviceAuthID,
		"user_code":        data.UserCode,
		"verification_url": "https://auth.openai.com/codex/device",
		"interval":         data.Interval,
		"expires_at":       data.ExpiresAt,
	})
}

// CodexDeviceAuthPoll checks whether the user has approved the device code.
func (h *Handler) CodexDeviceAuthPoll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		ProfileName  string `json:"profile_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	name := strings.TrimSpace(req.ProfileName)
	if name == "" {
		respondError(w, http.StatusBadRequest, "profile_name is required")
		return
	}
	if !validProfileName.MatchString(name) {
		respondError(w, http.StatusBadRequest, "invalid profile name: letters, numbers, hyphens, and underscores only")
		return
	}

	pollPayload, _ := json.Marshal(map[string]string{
		"client_id":      api.CodexOAuthClientID,
		"device_auth_id": req.DeviceAuthID,
		"user_code":      req.UserCode,
	})

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIDeviceAuthTokenURL, bytes.NewReader(pollPayload))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "codex-cli/0.155.1")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		respondError(w, http.StatusBadGateway, "failed to contact OpenAI: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		var errData struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(respBytes, &errData)
		if errData.Error.Code == "deviceauth_authorization_pending" || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"status": "pending",
			})
			return
		}
		respondError(w, resp.StatusCode, "authorization failed: "+errData.Error.Message)
		return
	}

	// Successfully received tokens
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(respBytes, &tokenResp); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to parse token response")
		return
	}

	accountID := ""
	userID := api.ParseIDTokenUserID(tokenResp.IDToken)

	// Save profile to disk
	profilesDir := codexProfilesDir()
	if profilesDir == "" {
		respondError(w, http.StatusInternalServerError, "could not determine profiles directory")
		return
	}
	if err := os.MkdirAll(profilesDir, 0o700); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create profiles directory")
		return
	}

	profile := CodexProfile{
		Name:      name,
		AccountID: accountID,
		UserID:    userID,
		SavedAt:   time.Now().UTC(),
	}
	profile.Tokens.AccessToken = tokenResp.AccessToken
	profile.Tokens.RefreshToken = tokenResp.RefreshToken
	profile.Tokens.IDToken = tokenResp.IDToken

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to format profile")
		return
	}

	profilePath := filepath.Join(profilesDir, name+".json")
	if err := os.WriteFile(profilePath, data, 0o600); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to write profile file")
		return
	}

	// Register in store if available
	if h.store != nil {
		extID := codexCompositeExternalID(accountID, userID)
		if extID == "" {
			extID = name
		}
		h.store.GetOrCreateProviderAccountByExternalID("codex", name, extID)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "success",
		"message": "Profile saved and linked successfully",
		"name":    name,
	})
}

// parseRawAuthData extracts credentials if the user pasted raw auth.json or credentials JSON.
func parseRawAuthData(raw string) *api.CodexCredentials {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	if strings.HasPrefix(trimmed, "{") {
		var authFile struct {
			Tokens struct {
				AccessToken  string `json:"access_token"`
				RefreshToken string `json:"refresh_token"`
				IDToken      string `json:"id_token"`
				AccountID    string `json:"account_id"`
			} `json:"tokens"`
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			IDToken      string `json:"id_token"`
			AccountID    string `json:"account_id"`
		}
		if err := json.Unmarshal([]byte(trimmed), &authFile); err == nil {
			at := authFile.Tokens.AccessToken
			if at == "" {
				at = authFile.AccessToken
			}
			rt := authFile.Tokens.RefreshToken
			if rt == "" {
				rt = authFile.RefreshToken
			}
			it := authFile.Tokens.IDToken
			if it == "" {
				it = authFile.IDToken
			}
			accID := authFile.Tokens.AccountID
			if accID == "" {
				accID = authFile.AccountID
			}

			if at != "" || rt != "" {
				return &api.CodexCredentials{
					AccessToken:  at,
					RefreshToken: rt,
					IDToken:      it,
					AccountID:    accID,
				}
			}
		}
	}
	return nil
}

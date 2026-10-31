package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AntigravityAccounts handles Antigravity account management.
// GET    /api/antigravity/accounts        - list accounts
// POST   /api/antigravity/accounts        - create account (explicit bridge endpoint)
// PUT    /api/antigravity/accounts?id=N   - update account
// DELETE /api/antigravity/accounts?id=N   - soft-delete account
func (h *Handler) AntigravityAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.antigravityAccountsList(w, r)
	case http.MethodPost:
		h.antigravityAccountCreate(w, r)
	case http.MethodPut:
		h.antigravityAccountUpdate(w, r)
	case http.MethodDelete:
		h.antigravityAccountDelete(w, r)
	default:
		respondError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) antigravityAccountsList(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}
	accounts, err := h.store.QueryProviderAccounts("antigravity")
	if err != nil {
		h.logger.Error("failed to query Antigravity accounts", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to query accounts")
		return
	}
	result := make([]map[string]interface{}, 0, len(accounts))
	for _, acc := range accounts {
		meta := parseAntigravityAccountMetaForAPI(acc.Metadata)
		entry := map[string]interface{}{
			"id":            acc.ID,
			"name":          acc.Name,
			"createdAt":     acc.CreatedAt.Format(time.RFC3339),
			"baseUrl":       meta.BaseURL,
			"hasToken":      meta.CSRFToken != "",
			"expectedEmail": meta.ExpectedEmail,
			"source":        meta.Source,
			"legacy":        meta.BaseURL == "",
		}
		if acc.DeletedAt != nil {
			entry["deletedAt"] = acc.DeletedAt.Format(time.RFC3339)
		}
		result = append(result, entry)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": result})
}

type antigravityAccountMetaAPI struct {
	BaseURL       string `json:"base_url"`
	CSRFToken     string `json:"csrf_token"`
	ExpectedEmail string `json:"expected_email"`
	Source        string `json:"source"`
}

func parseAntigravityAccountMetaForAPI(raw string) antigravityAccountMetaAPI {
	var meta antigravityAccountMetaAPI
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &meta)
	}
	meta.BaseURL = strings.TrimSpace(meta.BaseURL)
	meta.CSRFToken = strings.TrimSpace(meta.CSRFToken)
	meta.ExpectedEmail = strings.TrimSpace(meta.ExpectedEmail)
	meta.Source = strings.TrimSpace(meta.Source)
	return meta
}

// validateAntigravityBridge checks an explicit endpoint. Loopback and private
// hosts are allowed on purpose: the supported connection method is a local
// Antigravity language server or a user-run bridge.
func validateAntigravityBridge(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("invalid bridge URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("bridge URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("bridge URL must not contain credentials, a query, or a fragment")
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("bridge URL must include a host")
	}
	return nil
}

func (h *Handler) antigravityAccountMeta(accountID int64) antigravityAccountMetaAPI {
	if h.store == nil {
		return antigravityAccountMetaAPI{}
	}
	acc, err := h.store.GetProviderAccountByID(accountID)
	if err != nil || acc == nil {
		return antigravityAccountMetaAPI{}
	}
	return parseAntigravityAccountMetaForAPI(acc.Metadata)
}

type antigravityAccountRequest struct {
	Name          string `json:"name"`
	BaseURL       string `json:"base_url"`
	CSRFToken     string `json:"csrf_token"`
	ExpectedEmail string `json:"expected_email"`
	Source        string `json:"source"`
}

func (h *Handler) antigravityAccountCreate(w http.ResponseWriter, r *http.Request) {
	var req antigravityAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "account name is required")
		return
	}
	if !validProfileName.MatchString(req.Name) {
		respondError(w, http.StatusBadRequest, "invalid account name: use only letters, numbers, hyphens, and underscores")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusInternalServerError, "store not available")
		return
	}
	if err := validateAntigravityBridge(req.BaseURL); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.BaseURL) != "" && strings.TrimSpace(req.ExpectedEmail) == "" {
		respondError(w, http.StatusBadRequest, "expected_email is required for an explicit bridge endpoint")
		return
	}
	if req.Name != "default" && strings.TrimSpace(req.BaseURL) == "" {
		respondError(w, http.StatusBadRequest, "base_url is required for a new account: agy does not expose verified profile isolation")
		return
	}
	accounts, err := h.store.QueryProviderAccounts("antigravity")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to query accounts")
		return
	}
	for _, account := range accounts {
		if account.Name == req.Name {
			respondError(w, http.StatusConflict, "account already exists; edit or restore it instead")
			return
		}
	}
	acc, err := h.store.CreateOrRestoreProviderAccount("antigravity", req.Name)
	if err != nil {
		h.logger.Error("failed to create Antigravity account", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	if err := h.store.UpdateProviderAccountMetadata(acc.ID, encodeAntigravityAccountMeta(req)); err != nil {
		h.logger.Error("failed to update Antigravity account metadata", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to save account")
		return
	}
	h.reloadAntigravityAgents()
	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "account created",
		"id":      acc.ID,
		"name":    req.Name,
	})
}

func encodeAntigravityAccountMeta(req antigravityAccountRequest) string {
	meta := antigravityAccountMetaAPI{
		BaseURL:       strings.TrimSpace(req.BaseURL),
		CSRFToken:     strings.TrimSpace(req.CSRFToken),
		ExpectedEmail: strings.TrimSpace(req.ExpectedEmail),
		Source:        strings.TrimSpace(req.Source),
	}
	raw, _ := json.Marshal(meta)
	return string(raw)
}

func (h *Handler) reloadAntigravityAgents() {
	if h.antigravityAgentMgr != nil {
		h.antigravityAgentMgr.Reload()
	}
	h.invalidateMenubarSnapshot()
}

type antigravityAccountUpdateRequest struct {
	Name          *string `json:"name"`
	BaseURL       *string `json:"base_url"`
	CSRFToken     *string `json:"csrf_token"`
	ExpectedEmail *string `json:"expected_email"`
	Source        *string `json:"source"`
	Restore       *bool   `json:"restore"`
}

func (h *Handler) antigravityAccountUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "valid account id is required")
		return
	}
	var req antigravityAccountUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusInternalServerError, "store not available")
		return
	}
	acc, err := h.store.GetProviderAccountByID(id)
	if err != nil || acc == nil || acc.Provider != "antigravity" {
		respondError(w, http.StatusNotFound, "account not found")
		return
	}

	meta := parseAntigravityAccountMetaForAPI(acc.Metadata)
	if req.BaseURL != nil {
		if err := validateAntigravityBridge(*req.BaseURL); err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		meta.BaseURL = strings.TrimSpace(*req.BaseURL)
	}
	if req.CSRFToken != nil {
		meta.CSRFToken = strings.TrimSpace(*req.CSRFToken)
	}
	if req.ExpectedEmail != nil {
		meta.ExpectedEmail = strings.TrimSpace(*req.ExpectedEmail)
	}
	if req.Source != nil {
		meta.Source = strings.TrimSpace(*req.Source)
	}
	if meta.BaseURL != "" && meta.ExpectedEmail == "" {
		respondError(w, http.StatusBadRequest, "expected_email is required for an explicit bridge endpoint")
		return
	}
	if meta.BaseURL == "" && acc.ID != h.store.DefaultAntigravityAccountID() {
		respondError(w, http.StatusBadRequest, "base_url is required for an additional account")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		name := strings.TrimSpace(*req.Name)
		if acc.Name == "default" && name != "default" {
			respondError(w, http.StatusBadRequest, "the default account cannot be renamed")
			return
		}
		if !validProfileName.MatchString(name) {
			respondError(w, http.StatusBadRequest, "invalid account name")
			return
		}
		if err := h.store.RenameProviderAccount(id, name); err != nil {
			h.logger.Error("failed to rename Antigravity account", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to rename account")
			return
		}
	}
	if req.Restore != nil && *req.Restore {
		if err := h.store.UndeleteProviderAccountByID(id); err != nil {
			h.logger.Error("failed to restore Antigravity account", "error", err)
			respondError(w, http.StatusInternalServerError, "failed to restore account")
			return
		}
	}
	raw, _ := json.Marshal(meta)
	if err := h.store.UpdateProviderAccountMetadata(id, string(raw)); err != nil {
		h.logger.Error("failed to update Antigravity account metadata", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to update account")
		return
	}
	h.reloadAntigravityAgents()
	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "account updated", "id": id})
}
func (h *Handler) antigravityAccountDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(w, http.StatusBadRequest, "valid account id is required")
		return
	}
	if h.store == nil {
		respondError(w, http.StatusInternalServerError, "store not available")
		return
	}
	acc, err := h.store.GetProviderAccountByID(id)
	if err != nil || acc == nil || acc.Provider != "antigravity" {
		respondError(w, http.StatusNotFound, "account not found")
		return
	}
	if err := h.store.MarkProviderAccountDeletedByID(id); err != nil {
		h.logger.Error("failed to delete Antigravity account", "error", err)
		respondError(w, http.StatusInternalServerError, "failed to delete account")
		return
	}
	h.reloadAntigravityAgents()
	respondJSON(w, http.StatusOK, map[string]interface{}{"message": "account deleted", "id": id})
}

// AntigravityAccountsUsage returns current usage for every active account.
func (h *Handler) AntigravityAccountsUsage(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}
	accounts, err := h.store.QueryActiveProviderAccounts("antigravity")
	if err != nil {
		h.logger.Error("failed to query Antigravity accounts", "error", err)
		respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}
	result := make([]map[string]interface{}, 0, len(accounts))
	for _, acc := range accounts {
		usage := h.buildAntigravityCurrent(acc.ID)
		usage["accountId"] = acc.ID
		usage["accountName"] = acc.Name
		result = append(result, usage)
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{"accounts": result})
}

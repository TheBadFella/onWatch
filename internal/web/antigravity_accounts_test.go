package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func antigravityTestHandler(t *testing.T, s *store.Store) *Handler {
	t.Helper()
	return NewHandler(s, nil, nil, nil, createTestConfigWithAntigravity())
}

func insertAntigravityTestSnapshot(t *testing.T, s *store.Store, accountID int64, email, modelID string, at time.Time) {
	t.Helper()
	if _, err := s.InsertAntigravitySnapshot(&api.AntigravitySnapshot{
		CapturedAt: at,
		Email:      email,
		Models: []api.AntigravityModelQuota{{
			ModelID:           modelID,
			Label:             modelID,
			RemainingFraction: 0.5,
			RemainingPercent:  50,
		}},
		Source: api.AntigravitySourceIDE,
	}, accountID); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
}

func TestHandler_AntigravityAccounts_CRUD(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	h := antigravityTestHandler(t, s)

	body := `{"name":"work","base_url":"https://127.0.0.1:42100","csrf_token":"tok","expected_email":"work@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/antigravity/accounts", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.AntigravityAccounts(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/antigravity/accounts", nil)
	rr = httptest.NewRecorder()
	h.AntigravityAccounts(rr, req)
	var list struct {
		Accounts []map[string]interface{} `json:"accounts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	var work map[string]interface{}
	for _, acc := range list.Accounts {
		if acc["name"] == "work" {
			work = acc
		}
		if _, exposed := acc["csrf_token"]; exposed {
			t.Fatalf("list exposed the CSRF token for %v", acc["name"])
		}
	}
	if work == nil {
		t.Fatal("work account missing from list")
	}
	if work["expectedEmail"] != "work@example.com" || work["baseUrl"] != "https://127.0.0.1:42100" || work["legacy"] != false {
		t.Fatalf("work account = %#v", work)
	}
	if work["hasToken"] != true {
		t.Fatalf("work hasToken = %v, want true", work["hasToken"])
	}
	id := int64(work["id"].(float64))
	// Updating the expected identity keeps the endpoint.
	update := `{"expected_email":"renamed@example.com"}`
	req = httptest.NewRequest(http.MethodPut, "/api/antigravity/accounts?id="+formatID(id), strings.NewReader(update))
	rr = httptest.NewRecorder()
	h.AntigravityAccounts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", rr.Code, rr.Body.String())
	}
	acc, err := s.GetProviderAccountByID(id)
	if err != nil || acc == nil {
		t.Fatalf("reload account: %v", err)
	}
	if !strings.Contains(acc.Metadata, "renamed@example.com") || !strings.Contains(acc.Metadata, "127.0.0.1:42100") {
		t.Fatalf("metadata = %s, want the merged identity and endpoint", acc.Metadata)
	}

	// Deleting is a soft delete.
	req = httptest.NewRequest(http.MethodDelete, "/api/antigravity/accounts?id="+formatID(id), nil)
	rr = httptest.NewRecorder()
	h.AntigravityAccounts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", rr.Code, rr.Body.String())
	}
	acc, err = s.GetProviderAccountByID(id)
	if err != nil || acc == nil || acc.DeletedAt == nil {
		t.Fatalf("account after delete = %#v, %v", acc, err)
	}
}

func TestHandler_AntigravityAccounts_Validation(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	h := antigravityTestHandler(t, s)

	cases := []struct {
		name string
		body string
	}{
		{name: "missing name", body: `{"base_url":"https://127.0.0.1:1","expected_email":"a@b.c"}`},
		{name: "invalid name", body: `{"name":"bad name!","base_url":"https://127.0.0.1:1","expected_email":"a@b.c"}`},
		{name: "missing email for bridge", body: `{"name":"work2","base_url":"https://127.0.0.1:1"}`},
		{name: "missing bridge for new account", body: `{"name":"work3"}`},
		{name: "invalid bridge scheme", body: `{"name":"work4","base_url":"ftp://127.0.0.1:1","expected_email":"a@b.c"}`},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodPost, "/api/antigravity/accounts", strings.NewReader(tc.body))
		rr := httptest.NewRecorder()
		h.AntigravityAccounts(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; body = %s", tc.name, rr.Code, rr.Body.String())
		}
	}
}
func TestHandler_AntigravityAccountsUsage_Isolated(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	h := antigravityTestHandler(t, s)

	work, err := s.CreateOrRestoreProviderAccount("antigravity", "work")
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	personal, err := s.CreateOrRestoreProviderAccount("antigravity", "personal")
	if err != nil {
		t.Fatalf("create personal: %v", err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	insertAntigravityTestSnapshot(t, s, work.ID, "work@example.com", "claude-4-5-sonnet", now)
	insertAntigravityTestSnapshot(t, s, personal.ID, "personal@example.com", "gemini-3-pro", now.Add(time.Minute))

	req := httptest.NewRequest(http.MethodGet, "/api/antigravity/accounts/usage", nil)
	rr := httptest.NewRecorder()
	h.AntigravityAccountsUsage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Accounts []map[string]interface{} `json:"accounts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byName := map[string]map[string]interface{}{}
	for _, acc := range resp.Accounts {
		name, _ := acc["accountName"].(string)
		byName[name] = acc
	}
	workUsage := byName["work"]
	personalUsage := byName["personal"]
	if workUsage == nil || personalUsage == nil {
		t.Fatalf("missing accounts in usage response: %#v", byName)
	}
	if workUsage["email"] != "work@example.com" {
		t.Fatalf("work email = %v", workUsage["email"])
	}
	if personalUsage["email"] != "personal@example.com" {
		t.Fatalf("personal email = %v", personalUsage["email"])
	}
	if got := groupRemaining(workUsage, api.AntigravityQuotaGroupClaudeGPT); got != 50 {
		t.Fatalf("work Claude+GPT remaining = %v, want 50", got)
	}
	if got := groupRemaining(personalUsage, api.AntigravityQuotaGroupGemini); got != 50 {
		t.Fatalf("personal Gemini remaining = %v, want 50", got)
	}
	if got := groupRemaining(workUsage, api.AntigravityQuotaGroupGemini); got != 100 {
		t.Fatalf("work Gemini remaining = %v, want the empty group at 100", got)
	}
}

func groupRemaining(usage map[string]interface{}, group string) float64 {
	quotas, _ := usage["quotas"].([]interface{})
	for _, entry := range quotas {
		q, _ := entry.(map[string]interface{})
		if key, _ := q["quotaGroup"].(string); key == group {
			if value, ok := q["remainingPercent"].(float64); ok {
				return value
			}
		}
	}
	return -1
}
func TestHandler_AntigravityCurrent_AccountScoped(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()
	h := antigravityTestHandler(t, s)

	work, err := s.CreateOrRestoreProviderAccount("antigravity", "work")
	if err != nil {
		t.Fatalf("create work: %v", err)
	}
	insertAntigravityTestSnapshot(t, s, work.ID, "work@example.com", "claude-4-5-sonnet", time.Now().UTC())

	req := httptest.NewRequest(http.MethodGet, "/api/current?provider=antigravity&account="+formatID(work.ID), nil)
	rr := httptest.NewRecorder()
	h.currentAntigravity(rr, req)
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["email"] != "work@example.com" {
		t.Fatalf("email = %v", resp["email"])
	}
	if resp["accountName"] != "work" {
		t.Fatalf("accountName = %v", resp["accountName"])
	}
}

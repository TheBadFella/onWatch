package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/config"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// An unknown-cap balance has no percentage, so Remaining mode must not
// invent a "100% remaining" figure from its placeholder utilization of 0.
func TestApplyDisplayModeToQuotaMap_SkipsUnknownLimit(t *testing.T) {
	t.Parallel()
	q := map[string]interface{}{"utilization": 0.0, "limitUnknown": true}
	applyDisplayModeToQuotaMap(q, "available")
	if _, ok := q["cardPercent"]; ok {
		t.Fatalf("unknown-cap quota must not get a cardPercent, got %v", q["cardPercent"])
	}
}

// Every Codex window, Review Requests included, follows the Quota Display mode.
func TestHandler_BuildCodexCurrent_CodeReviewFollowsDisplayMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode        string
		wantPercent interface{}
	}{
		{"usage", nil},
		{"available", 70.0},
	} {
		s, _ := store.New(":memory:")
		s.InsertCodexSnapshot(&api.CodexSnapshot{
			CapturedAt: time.Now().UTC(),
			PlanType:   "pro",
			Quotas:     []api.CodexQuota{{Name: "code_review", Utilization: 30.0}},
		})
		s.SetSetting("provider_settings", `{"global":{"display_mode":"`+tc.mode+`"}}`)
		h := NewHandler(s, nil, nil, nil, createTestConfigWithCodex())
		quotas := h.buildCodexCurrent(0)["quotas"].([]map[string]interface{})
		if len(quotas) != 1 {
			t.Fatalf("mode %s: expected 1 quota, got %d", tc.mode, len(quotas))
		}
		if got := quotas[0]["cardPercent"]; got != tc.wantPercent {
			t.Errorf("mode %s: code_review cardPercent = %v, want %v", tc.mode, got, tc.wantPercent)
		}
		s.Close()
	}
}

// Changing the global Quota Display mode resets per-provider overrides, so
// one switch flips every provider.
func TestHandler_UpdateSettings_GlobalDisplayModeClearsOverrides(t *testing.T) {
	t.Parallel()
	s, _ := store.New(":memory:")
	defer s.Close()
	s.SetSetting("provider_settings",
		`{"global":{"display_mode":"usage"},"codex":{"display_mode":"available","pace_mode":"5-day"}}`)
	h := NewHandler(s, nil, nil, nil, &config.Config{})

	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings",
		strings.NewReader(`{"provider_settings":{"global":{"display_mode":"available"}}}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	raw, _ := s.GetSetting("provider_settings")
	var stored map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["codex"]["display_mode"]; ok {
		t.Errorf("codex override should be cleared, got %v", stored["codex"]["display_mode"])
	}
	if stored["codex"]["pace_mode"] != "5-day" {
		t.Errorf("unrelated codex settings must survive, got %v", stored["codex"])
	}
	if got := h.getDisplayMode("codex"); got != "available" {
		t.Errorf("codex should follow the new global mode, got %q", got)
	}

	// Saving the same global value again must keep an override set afterwards.
	s.SetSetting("provider_settings",
		`{"global":{"display_mode":"available"},"codex":{"display_mode":"usage"}}`)
	rr = httptest.NewRecorder()
	h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings",
		strings.NewReader(`{"provider_settings":{"global":{"display_mode":"available"}}}`)))
	if got := h.getDisplayMode("codex"); got != "usage" {
		t.Errorf("unchanged global save must keep the codex override, got %q", got)
	}
}

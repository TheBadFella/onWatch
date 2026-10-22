package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func TestDashboardPromptPreferences(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	read := func() map[string]interface{} {
		t.Helper()
		rr := httptest.NewRecorder()
		h.GetSettings(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
		var result map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, key := range []string{"hide_connect_prompt", "hide_smtp_setup_reminder"} {
		if read()[key] != false {
			t.Fatalf("%s should default to false", key)
		}
	}
	for _, body := range []string{
		`{"hide_connect_prompt":true,"hide_smtp_setup_reminder":true}`,
		`{"timezone":"UTC"}`,
		`{"hide_connect_prompt":false}`,
	} {
		rr := httptest.NewRecorder()
		h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
		if rr.Code != http.StatusOK {
			t.Fatalf("save failed: %d %s", rr.Code, rr.Body.String())
		}
		got := read()
		if got["hide_smtp_setup_reminder"] != true || got["hide_connect_prompt"] != !strings.Contains(body, `"hide_connect_prompt":false`) {
			t.Fatalf("preferences not preserved: %v", got)
		}
	}
	for _, value := range []string{`"yes"`, `null`, `{}`} {
		rr := httptest.NewRecorder()
		h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"hide_smtp_setup_reminder":`+value+`}`)))
		if rr.Code != http.StatusBadRequest || read()["hide_smtp_setup_reminder"] != true {
			t.Fatalf("invalid preference accepted: %s", value)
		}
	}
}

func TestWindowReminderSettings(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	for _, body := range []string{`{"notifications":{"warning_threshold":80,"critical_threshold":95,"reset_soon_minutes":30}}`, `{"notifications":{"warning_threshold":80,"critical_threshold":95}}`} {
		rr := httptest.NewRecorder()
		h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
		if rr.Code != 200 {
			t.Fatalf("save failed: %s", rr.Body)
		}
		saved, _ := s.GetSetting("notifications")
		var got struct {
			Minutes int `json:"reset_soon_minutes"`
		}
		if err := json.Unmarshal([]byte(saved), &got); err != nil || got.Minutes != 30 {
			t.Fatalf("lead time not preserved: %s", saved)
		}
	}
	for _, invalid := range []string{"0", "1441", "-1"} {
		rr := httptest.NewRecorder()
		h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"notifications":{"warning_threshold":80,"critical_threshold":95,"reset_soon_minutes":`+invalid+`}}`)))
		if rr.Code != 400 {
			t.Fatalf("invalid lead time accepted: %s", invalid)
		}
	}
}

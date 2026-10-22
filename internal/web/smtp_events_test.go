package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func TestSMTPEventsPersistAndSurviveOlderClients(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessions := NewSessionStore("admin", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", s)
	h := NewHandler(s, nil, nil, sessions, createTestConfigWithSynthetic())
	for _, body := range []string{
		`{"smtp":{"host":"smtp.example.com","events":{"auth_error":true,"starter_failure":true}}}`,
		`{"smtp":{"host":"smtp.other.example.com"}}`,
		`{"smtp":{"host":"smtp.other.example.com","events":{}}}`,
	} {
		rr := httptest.NewRecorder()
		h.UpdateSettings(rr, httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(body)))
		if rr.Code != 200 {
			t.Fatalf("save status %d: %s", rr.Code, rr.Body.String())
		}
		rr = httptest.NewRecorder()
		h.GetSettings(rr, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
		var got struct {
			SMTP struct {
				Events map[string]bool `json:"events"`
			} `json:"smtp"`
		}
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		wantAuth := !strings.Contains(body, `"events":{}`)
		if got.SMTP.Events == nil || got.SMTP.Events["auth_error"] != wantAuth || got.SMTP.Events["warning"] {
			t.Fatalf("events did not round-trip: %v", got.SMTP.Events)
		}
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

func TestAnthropicStarterAfterSuccessfulPoll(t *testing.T) {
	var response atomic.Value
	response.Store(`{"five_hour":null,"seven_day":{"utilization":12}}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(response.Load().(string)))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "usage.db")
	client := api.NewAnthropicClient("test-token", slog.Default(), api.WithAnthropicBaseURL(server.URL))
	calls := 0
	openAgent := func() (*AnthropicAgent, *store.Store) {
		t.Helper()
		db, err := store.New(path)
		if err != nil {
			t.Fatal(err)
		}
		a := NewAnthropicAgent(client, db, nil, time.Minute, nil, nil)
		a.startFiveHourWindow = func(context.Context) error {
			calls++
			return nil
		}
		return a, db
	}

	a, db := openAgent()
	defer db.Close()
	ctx := context.Background()
	a.poll(ctx)
	if calls != 0 {
		t.Fatal("starter fired while off")
	}

	a.SetAutoStart5hCheck(func() bool { return true })
	for _, body := range []string{
		`{"seven_day":{"utilization":12}}`,
		`{"five_hour":{"utilization":0,"resets_at":"invalid"}}`,
		anthropicResponse(0, 12), // Zero use with a future reset is already active.
		`{"five_hour":null,"seven_day":{"utilization":100}}`,
		`{"five_hour":null,"extra_usage":{"is_enabled":true}}`,
	} {
		response.Store(body)
		a.poll(ctx)
		if calls != 0 {
			t.Fatalf("starter fired for %s", body)
		}
	}

	response.Store(`{"five_hour":{"utilization":0,"resets_at":null},"seven_day":{"utilization":12}}`)
	a.SetAPIPollingDisabled(true)
	a.poll(ctx)
	if calls != 0 {
		t.Fatal("starter fired in statusline-only mode")
	}
	a.SetAPIPollingDisabled(false)
	a.poll(ctx)
	a.poll(ctx)
	if calls != 1 {
		t.Fatalf("starter fired %d times, want 1", calls)
	}
	status, err := db.GetStarterStatus("anthropic", "", "five_hour")
	if err != nil || status.Result != "sent" || status.AttemptedAt == nil {
		t.Fatalf("expected a sent request without confirmation: %+v, %v", status, err)
	}
	latest, err := db.QueryLatestAnthropic()
	if err != nil || latest == nil || len(latest.Quotas) != 2 || latest.Quotas[0].ResetsAt != nil {
		t.Fatalf("starter should preserve the observed quota snapshot, got %+v, %v", latest, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, db := openAgent()
	defer db.Close()
	restarted.SetAutoStart5hCheck(func() bool { return true })
	restarted.poll(ctx)
	if calls != 1 {
		t.Fatal("starter fired again after restart")
	}
	observed := status.AttemptedAt.Add(time.Minute)
	reset := status.AttemptedAt.Add(anthropicStarterWindow).Format(time.RFC3339)
	var active api.AnthropicQuotaResponse
	if err := json.Unmarshal([]byte(`{"five_hour":{"utilization":1,"resets_at":"`+reset+`"}}`), &active); err != nil {
		t.Fatal(err)
	}
	restarted.maybeStartFiveHourWindow(ctx, active, observed)
	confirmed, err := db.GetStarterStatus("anthropic", "", "five_hour")
	if err != nil || confirmed.Result != "confirmed" || confirmed.ConfirmedAt == nil || !confirmed.ConfirmedAt.Equal(observed) || calls != 1 {
		t.Fatalf("expected observed window after restart: %+v, calls=%d, %v", confirmed, calls, err)
	}
}

func TestAnthropicStarterRetryAndNextWindow(t *testing.T) {
	a, _ := pollTestAgent(t, time.Minute)
	a.SetAutoStart5hCheck(func() bool { return true })
	now := time.Now().UTC().Truncate(time.Second)
	var quotas api.AnthropicQuotaResponse
	if err := json.Unmarshal([]byte(`{"five_hour":{"utilization":95,"resets_at":"2025-01-01T00:00:00Z"}}`), &quotas); err != nil {
		t.Fatal(err)
	}
	calls := 0
	a.startFiveHourWindow = func(context.Context) error {
		calls++
		if calls == 1 {
			return errors.New("test failure")
		}
		return nil
	}
	ctx := context.Background()
	a.maybeStartFiveHourWindow(ctx, quotas, now)
	a.maybeStartFiveHourWindow(ctx, quotas, now.Add(anthropicStarterRetry-time.Second))
	if calls != 1 {
		t.Fatalf("starter fired %d times before retry, want 1", calls)
	}
	retryAt := now.Add(anthropicStarterRetry)
	a.maybeStartFiveHourWindow(ctx, quotas, retryAt)
	a.maybeStartFiveHourWindow(ctx, quotas, retryAt.Add(anthropicStarterWindow-time.Second))
	if calls != 2 {
		t.Fatalf("starter fired %d times during the window, want 2", calls)
	}
	a.maybeStartFiveHourWindow(ctx, quotas, retryAt.Add(anthropicStarterWindow))
	if calls != 3 {
		t.Fatalf("starter fired %d times after expiry, want 3", calls)
	}
}

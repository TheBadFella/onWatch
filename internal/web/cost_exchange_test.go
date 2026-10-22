package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/store"
)

type costFXTransport func(*http.Request) (*http.Response, error)

func (f costFXTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCostExchangeCacheAndFailure(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	now := time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)
	calls := 0
	client := &http.Client{Transport: costFXTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != bankOfCanadaExchangeURL {
			t.Fatal("non-fixed FX endpoint")
		}
		if calls > 1 {
			return nil, errors.New("offline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"observations":[{"d":"2026-10-22","FXUSDCAD":{"v":"1.5"}}]}`)), Header: make(http.Header)}, nil
	})}
	h.costFXClient = client
	config := costSettings{Subscriptions: []costSubscription{{MonthlyUSD: costNumber(20)}}}
	first := h.costExchange(context.Background(), config, now)
	if first == nil || first.CADPerUSD != 1.5 || first.Date != "2026-10-22" {
		t.Fatal(first)
	}
	if h.costExchange(context.Background(), config, now.Add(time.Minute)) == nil || calls != 1 {
		t.Fatal("rate fetched on every request")
	}
	restarted := NewHandler(s, nil, nil, nil, createTestConfigWithSynthetic())
	restarted.costFXClient = client
	if restarted.costExchange(context.Background(), config, now.Add(time.Hour)) == nil || calls != 1 {
		t.Fatal("rate cache did not survive restart")
	}
	stale := h.costExchange(context.Background(), config, now.Add(49*time.Hour))
	if stale == nil || !stale.Stale || calls != 2 {
		t.Fatal("last good rate was lost or not marked stale")
	}
	h.costExchange(context.Background(), config, now.Add(49*time.Hour+time.Minute))
	if calls != 2 {
		t.Fatal("FX failure caused a retry storm")
	}
	config.CADPerUSD = costNumber(1.6)
	if got := h.costExchange(context.Background(), config, now); got.CADPerUSD != 1.6 || calls != 2 {
		t.Fatal("manual rate did not override automatic source")
	}
}
func TestCostExchangeRejectsInvalidRate(t *testing.T) {
	for _, value := range []string{"NaN", "0", "-1", "101", "not-a-rate"} {
		t.Run(value, func(t *testing.T) {
			h := &Handler{costFXClient: &http.Client{Transport: costFXTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"observations":[{"d":"2026-10-22","FXUSDCAD":{"v":"` + value + `"}}]}`))}, nil
			})}}
			if h.fetchCostExchange(context.Background(), time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)) != nil {
				t.Fatal("invalid rate accepted")
			}
		})
	}
}

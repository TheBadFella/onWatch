package agent

import (
	"context"
	"errors"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
)

func TestAntigravityAgent_VerifySnapshotIdentity(t *testing.T) {
	ag := &AntigravityAgent{expectedEmail: "user@example.com"}

	if err := ag.verifySnapshotIdentity(&api.AntigravitySnapshot{Email: "USER@example.com"}); err != nil {
		t.Fatalf("matching identity rejected: %v", err)
	}
	cases := []struct {
		name  string
		email string
	}{
		{name: "different account", email: "other@example.com"},
		{name: "missing identity", email: ""},
	}
	for _, tc := range cases {
		err := ag.verifySnapshotIdentity(&api.AntigravitySnapshot{Email: tc.email})
		if !errors.Is(err, ErrAntigravityIdentityMismatch) {
			t.Fatalf("%s: err = %v, want ErrAntigravityIdentityMismatch", tc.name, err)
		}
	}

	// With no expected identity, the reported value is accepted as-is.
	open := &AntigravityAgent{}
	if err := open.verifySnapshotIdentity(&api.AntigravitySnapshot{Email: "anything@example.com"}); err != nil {
		t.Fatalf("unpinned identity rejected: %v", err)
	}
}

func TestWithAntigravityAccount_SetsScopeAndIdentity(t *testing.T) {
	ag := &AntigravityAgent{}
	WithAntigravityAccount(7, " user@example.com ")(ag)
	if ag.accountID != 7 {
		t.Fatalf("accountID = %d, want 7", ag.accountID)
	}
	if ag.expectedEmail != "user@example.com" {
		t.Fatalf("expectedEmail = %q", ag.expectedEmail)
	}
}
func TestAntigravityManager_ConcurrentReloadAndShutdown(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(antigravityTestResponse())) }))
	defer server.Close()
	mgr := NewAntigravityAgentManager(st, time.Hour, 5*time.Minute, nil)
	mgr.SetLegacyClient(api.NewAntigravityClient(slog.Default(), api.WithAntigravityConnection(&api.AntigravityConnection{BaseURL: server.URL})), func() string { return api.AntigravitySourceIDE })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { mgr.Run(ctx); close(done) }()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); mgr.Reload() }()
	}
	cancel()
	wg.Wait()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung")
	}
	if len(mgr.GetRunningAccounts()) != 0 {
		t.Fatal("agents survived shutdown")
	}
}

func TestAntigravityAgent_IdentityMismatchStoresNothing(t *testing.T) {
	st, err := store.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	account, err := st.CreateOrRestoreProviderAccount("antigravity", "work")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(antigravityTestResponse())) }))
	defer server.Close()
	ag := NewAntigravityAgent(api.NewAntigravityClient(slog.Default()), st, tracker.NewAntigravityTrackerForAccount(st, slog.Default(), account.ID), time.Hour, slog.Default(), nil,
		WithAntigravityAccount(account.ID, "other@example.com"), WithAntigravityManualConfig(server.URL, "fake-token"))
	ag.poll(context.Background())
	got, err := st.QueryLatestAntigravity(account.ID)
	if err != nil || got != nil {
		t.Fatalf("mismatched identity stored: %+v, %v", got, err)
	}
	ag.expectedEmail = "test@example.com"
	ag.poll(context.Background())
	got, err = st.QueryLatestAntigravity(account.ID)
	if err != nil || got == nil || got.Email != "test@example.com" {
		t.Fatalf("matching identity not stored: %+v, %v", got, err)
	}
}

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/notify"
	"github.com/onllm-dev/onwatch/v2/internal/store"
	"github.com/onllm-dev/onwatch/v2/internal/tracker"
)

// AntigravityAccountMeta is the JSON stored in provider_accounts.metadata for
// one Antigravity account. BaseURL and CSRFToken describe an explicitly
// authenticated bridge endpoint; ExpectedEmail pins the identity that endpoint
// must report. Legacy accounts without a base URL use the global client and
// auto-detection.
type AntigravityAccountMeta struct {
	BaseURL       string `json:"base_url,omitempty"`
	CSRFToken     string `json:"csrf_token,omitempty"`
	ExpectedEmail string `json:"expected_email,omitempty"`
	Source        string `json:"source,omitempty"`
}

// ParseAntigravityAccountMeta tolerates a missing or malformed metadata blob;
// an unreadable blob yields an empty meta, which only allows the legacy default
// account to poll.
func ParseAntigravityAccountMeta(raw string) AntigravityAccountMeta {
	var meta AntigravityAccountMeta
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &meta)
	}
	meta.BaseURL = strings.TrimSpace(meta.BaseURL)
	meta.CSRFToken = strings.TrimSpace(meta.CSRFToken)
	meta.ExpectedEmail = strings.TrimSpace(meta.ExpectedEmail)
	meta.Source = strings.TrimSpace(meta.Source)
	return meta
}

// HasBridge reports whether the account has its own authenticated endpoint.
func (m AntigravityAccountMeta) HasBridge() bool { return m.BaseURL != "" }

// AntigravityAgentInstance is one running per-account agent.
type AntigravityAgentInstance struct {
	DBAccountID int64
	AccountName string
	Agent       *AntigravityAgent
	Tracker     *tracker.AntigravityTracker
	Cancel      context.CancelFunc
}

// AntigravityAgentManager runs one Antigravity agent per active provider
// account. Accounts with an explicit bridge endpoint poll it independently;
// the legacy default account falls back to the global auto-detecting client.
// Account failures are isolated: one agent returning an error never cancels
// another.
type AntigravityAgentManager struct {
	store               *store.Store
	interval            time.Duration
	idleTimeout         time.Duration
	logger              *slog.Logger
	notifier            *notify.NotificationEngine
	pollingCheck        func() bool
	accountPollingCheck func(accountID int64) bool

	legacyClient *api.AntigravityClient
	legacySource func() string

	mu        sync.RWMutex
	instances map[int64]*AntigravityAgentInstance
	wg        sync.WaitGroup
	reloadMu  sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewAntigravityAgentManager creates a manager for multi-account polling.
func NewAntigravityAgentManager(st *store.Store, interval, idleTimeout time.Duration, logger *slog.Logger) *AntigravityAgentManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &AntigravityAgentManager{
		store:       st,
		interval:    interval,
		idleTimeout: idleTimeout,
		logger:      logger,
		instances:   make(map[int64]*AntigravityAgentInstance),
	}
}

// SetLegacyClient installs the auto-detecting client (and source preference)
// used by the default account when it has no explicit bridge endpoint.
func (m *AntigravityAgentManager) SetLegacyClient(client *api.AntigravityClient, source func() string) {
	m.legacyClient = client
	m.legacySource = source
}

// SetNotifier sets the notification engine for all account agents.
func (m *AntigravityAgentManager) SetNotifier(n *notify.NotificationEngine) { m.notifier = n }

// SetPollingCheck sets the global polling check.
func (m *AntigravityAgentManager) SetPollingCheck(fn func() bool) { m.pollingCheck = fn }

// SetAccountPollingCheck sets a per-account polling check.
func (m *AntigravityAgentManager) SetAccountPollingCheck(fn func(accountID int64) bool) {
	m.accountPollingCheck = fn
}

// TrackerForAccount returns the tracker for a running account, or nil.
func (m *AntigravityAgentManager) TrackerForAccount(accountID int64) *tracker.AntigravityTracker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if inst := m.instances[accountID]; inst != nil {
		return inst.Tracker
	}
	return nil
}

// Run starts the manager, loads accounts, and blocks until the context ends.
func (m *AntigravityAgentManager) Run(ctx context.Context) error {
	m.reloadMu.Lock()
	runCtx, cancel := context.WithCancel(ctx)
	m.ctx, m.cancel = runCtx, cancel
	defer cancel()

	m.logger.Info("Antigravity agent manager started", "interval", m.interval)
	if err := m.loadAndStartAccounts(); err != nil {
		m.logger.Error("failed to load initial Antigravity accounts", "error", err)
	}

	m.reloadMu.Unlock()
	<-runCtx.Done()
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()
	m.stopAllAgents()
	return nil
}

// Reload stops all agents and restarts from the current database state. The
// web layer calls it after an account is added, changed, or removed.
func (m *AntigravityAgentManager) Reload() {
	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	if m.ctx == nil || m.ctx.Err() != nil {
		return
	}
	m.logger.Info("Antigravity agent manager reloading accounts")
	m.stopAllAgents()
	if err := m.loadAndStartAccounts(); err != nil {
		m.logger.Error("failed to reload Antigravity accounts", "error", err)
	}
}
func (m *AntigravityAgentManager) loadAndStartAccounts() error {
	accounts, err := m.store.QueryActiveProviderAccounts("antigravity")
	if err != nil {
		return fmt.Errorf("failed to query Antigravity accounts: %w", err)
	}
	started := 0
	for _, acc := range accounts {
		meta := ParseAntigravityAccountMeta(acc.Metadata)
		if err := m.startAgentForAccount(acc, meta); err != nil {
			m.logger.Warn("Antigravity account not polled",
				"account", acc.Name, "id", acc.ID, "error", err)
			continue
		}
		started++
	}
	m.logger.Info("Antigravity accounts loaded", "started", started, "total", len(accounts))
	return nil
}
func (m *AntigravityAgentManager) startAgentForAccount(acc store.ProviderAccount, meta AntigravityAccountMeta) error {
	m.mu.RLock()
	_, exists := m.instances[acc.ID]
	m.mu.RUnlock()
	if exists {
		return nil
	}

	var client *api.AntigravityClient
	var source func() string
	switch {
	case meta.HasBridge():
		if meta.ExpectedEmail == "" {
			return fmt.Errorf("bridge account requires expected_email")
		}
		conn := &api.AntigravityConnection{
			BaseURL:   meta.BaseURL,
			CSRFToken: meta.CSRFToken,
			Protocol:  "https",
		}
		client = api.NewAntigravityClient(m.logger, api.WithAntigravityConnection(conn))
	case acc.ID == m.store.DefaultAntigravityAccountID() && m.legacyClient != nil:
		client = m.legacyClient
		source = m.legacySource
	default:
		return fmt.Errorf("no explicit bridge endpoint configured (set base_url and expected_email)")
	}

	tr := tracker.NewAntigravityTrackerForAccount(m.store, m.logger, acc.ID)
	if m.notifier != nil {
		notifier := m.notifier
		accountID := acc.ID
		accountName := acc.Name
		tr.SetOnReset(func(modelID string) {
			notifier.Check(notify.QuotaStatus{
				Provider:      "antigravity",
				AccountID:     strconv.FormatInt(accountID, 10),
				AccountName:   accountName,
				QuotaKey:      modelID,
				ResetOccurred: true,
			})
		})
	}
	sm := NewSessionManager(m.store, fmt.Sprintf("antigravity:%d", acc.ID), m.idleTimeout, m.logger)
	opts := []AntigravityAgentOption{
		WithAntigravityAccount(acc.ID, meta.ExpectedEmail),
		WithAntigravityProfileLabel(acc.Name),
	}
	if meta.HasBridge() {
		opts = append(opts, WithAntigravityManualConfig(meta.BaseURL, meta.CSRFToken))
	}
	ag := NewAntigravityAgent(client, m.store, tr, m.interval, m.logger, sm, opts...)
	if m.notifier != nil {
		ag.SetNotifier(m.notifier)
	}
	if source != nil {
		ag.SetSourceCheck(source)
	}
	ag.SetPollingCheck(func() bool {
		if m.pollingCheck != nil && !m.pollingCheck() {
			return false
		}
		if m.accountPollingCheck != nil && !m.accountPollingCheck(acc.ID) {
			return false
		}
		return true
	})

	agentCtx, cancel := context.WithCancel(m.ctx)
	instance := &AntigravityAgentInstance{
		DBAccountID: acc.ID,
		AccountName: acc.Name,
		Agent:       ag,
		Tracker:     tr,
		Cancel:      cancel,
	}
	m.mu.Lock()
	m.instances[acc.ID] = instance
	m.mu.Unlock()

	m.logger.Info("starting Antigravity agent", "account", acc.Name, "id", acc.ID, "bridge", meta.HasBridge())

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				m.logger.Error("Antigravity agent panicked", "account", acc.Name, "panic", r)
			}
		}()
		if err := ag.Run(agentCtx); err != nil && agentCtx.Err() == nil {
			m.logger.Error("Antigravity agent error", "account", acc.Name, "error", err)
		}
	}()
	return nil
}

// stopAllAgents cancels every agent and waits for its goroutine to exit.
func (m *AntigravityAgentManager) stopAllAgents() {
	m.mu.Lock()
	instances := make([]*AntigravityAgentInstance, 0, len(m.instances))
	for _, inst := range m.instances {
		instances = append(instances, inst)
	}
	m.instances = make(map[int64]*AntigravityAgentInstance)
	m.mu.Unlock()

	for _, inst := range instances {
		if inst.Cancel != nil {
			inst.Cancel()
		}
	}
	m.wg.Wait()
}

// GetRunningAccounts reports the currently running account agents.
func (m *AntigravityAgentManager) GetRunningAccounts() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]map[string]interface{}, 0, len(m.instances))
	for _, inst := range m.instances {
		result = append(result, map[string]interface{}{
			"name":          inst.AccountName,
			"db_account_id": inst.DBAccountID,
		})
	}
	return result
}

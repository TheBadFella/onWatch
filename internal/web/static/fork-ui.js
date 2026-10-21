// Fork presentation only. Provider polling and authentication stay upstream.
function overviewResetText(deadline, now = Date.now()) {
  const seconds = Math.max(0, Math.ceil((deadline - now) / 1000));
  return seconds > 0 ? 'Resets in ' + formatDuration(seconds) : 'Reset due';
}

function overviewResetHTML(resetAt, seconds = 0, now = Date.now()) {
  let deadline = resetAt ? Date.parse(resetAt) : NaN;
  if (!Number.isFinite(deadline) && Number.isFinite(Number(seconds)) && Number(seconds) > 0) {
    deadline = now + Number(seconds) * 1000;
  }
  if (!Number.isFinite(deadline)) return '<span class="overview-reset-unknown">Reset time unavailable</span>';
  return `<span class="overview-reset-countdown" data-reset-deadline="${deadline}" title="${new Date(deadline).toISOString()}">${overviewResetText(deadline, now)}</span>`;
}

function providerResetHTML(entry) {
  const quotas = Array.isArray(entry.accountsGroup)
    ? entry.accountsGroup.flatMap(account => accountOverviewQuotas(entry.provider, account))
    : entry.quotas || [];
  const resets = quotas.map(quota => Date.parse(quota.resetAt || quota.resetsAt || '')).filter(Number.isFinite);
  return resets.length ? overviewResetHTML(new Date(Math.min(...resets)).toISOString()) : '';
}

function updateOverviewCountdowns(now = Date.now()) {
  document.querySelectorAll('[data-reset-deadline]').forEach(el => {
    el.textContent = overviewResetText(Number(el.dataset.resetDeadline), now);
  });
}

function setupSidebar() {
  const sidebar = document.getElementById('app-sidebar');
  const toggle = document.getElementById('sidebar-toggle');
  if (!sidebar || !toggle) return;
  const root = document.documentElement;
  const setCollapsed = (collapsed, save = true) => {
    root.classList.toggle('sidebar-collapsed', collapsed);
    toggle.setAttribute('aria-expanded', String(!collapsed));
    toggle.setAttribute('aria-label', collapsed ? 'Expand sidebar' : 'Collapse sidebar');
    toggle.title = collapsed ? 'Expand sidebar' : 'Collapse sidebar';
    if (save) {
      try { localStorage.setItem('onwatch-sidebar-collapsed', String(collapsed)); } catch (e) { /* Optional preference. */ }
    }
  };
  setCollapsed(root.classList.contains('sidebar-collapsed'), false);
  toggle.addEventListener('click', () => setCollapsed(!root.classList.contains('sidebar-collapsed')));
  document.getElementById('sidebar-backdrop')?.addEventListener('click', () => setCollapsed(true));
  document.addEventListener('keydown', event => {
    if (event.key === 'Escape' && window.matchMedia('(max-width: 760px)').matches && !root.classList.contains('sidebar-collapsed')) {
      setCollapsed(true);
      toggle.focus();
    }
  });
  const labels = { 'refresh-btn': 'Refresh data', 'notification-bell': 'Notifications', 'settings-btn': 'Settings', 'theme-toggle': 'Toggle theme', 'logout-btn': 'Sign out' };
  Object.entries(labels).forEach(([id, label]) => {
    const control = document.getElementById(id);
    if (!control) return;
    const text = document.createElement('span');
    text.className = 'sidebar-action-label';
    text.textContent = label;
    control.appendChild(text);
    control.title = label;
  });
  const tabs = document.getElementById('provider-tabs');
  if (tabs) {
    tabs.setAttribute('aria-orientation', 'vertical');
    const buttons = [...tabs.querySelectorAll('.provider-tab')];
    buttons.forEach((button, index) => {
      button.title = button.textContent.trim();
      button.tabIndex = button.classList.contains('active') ? 0 : -1;
      button.addEventListener('keydown', event => {
        let target = index;
        if (event.key === 'ArrowDown') target = (index + 1) % buttons.length;
        else if (event.key === 'ArrowUp') target = (index - 1 + buttons.length) % buttons.length;
        else if (event.key === 'Home') target = 0;
        else if (event.key === 'End') target = buttons.length - 1;
        else return;
        event.preventDefault();
        buttons.forEach((item, i) => { item.tabIndex = i === target ? 0 : -1; });
        buttons[target].focus();
      });
    });
  }
  ['codex', 'minimax'].forEach(provider => {
    document.getElementById(provider + '-profile-trigger')?.setAttribute('aria-label', 'Select ' + (provider === 'codex' ? 'Codex profile' : 'MiniMax account'));
  });
  updateOverviewCountdowns();
  setInterval(() => updateOverviewCountdowns(), 1000);
}

document.addEventListener('DOMContentLoaded', setupSidebar);

// Reuse the existing renderers when provider data changes, without redrawing
// charts merely because the server's reset countdown is a few seconds lower.
const liveDataSignatures = new Map();
function shouldRenderLiveData(key, data, force = false) {
  const signature = JSON.stringify(data, (name, value) => name === 'timeUntilResetSeconds' ? undefined : value);
  const unchanged = liveDataSignatures.get(key) === signature;
  liveDataSignatures.set(key, signature);
  return force || !unchanged;
}

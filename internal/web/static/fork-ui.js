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

function codexResetSummaryKey(credits) {
  return JSON.stringify([credits?.available_count ?? null, credits?.next_expires_at ?? null, credits?.status ?? null]);
}

function codexResetSummaryHTML(credits) {
  const known = Number.isFinite(credits?.available_count);
  const count = codexResetCreditsCount(credits);
  const value = count > 0 ? codexResetCreditsHTML(credits)
    : `<span class="reset-summary-value">${known ? 'None available' : 'Not reported'}</span>`;
  const expiry = count > 0 && Number.isFinite(Date.parse(credits?.next_expires_at))
    ? `<span class="reset-summary-expiry">Next expiry: ${escapeHTML(formatDateTime(credits.next_expires_at))}</span>` : '';
  return `<div class="reset-credit-summary" data-reset-summary="${escapeHTML(codexResetSummaryKey(credits))}"><span class="reset-summary-label">Banked resets</span>${value}${expiry}</div>`;
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
  const signature = JSON.stringify(data, (name, value) =>
    name === 'timeUntilResetSeconds' || name === 'timeUntilReset' || name === 'ageSeconds' ? undefined : value);
  const unchanged = liveDataSignatures.get(key) === signature;
  liveDataSignatures.set(key, signature);
  return force || !unchanged;
}

// Keep card nodes (and focus/listeners) alive when only usage values change.
function patchOverviewNode(current, next) {
  if (current.nodeType !== next.nodeType || current.nodeName !== next.nodeName) {
    current.replaceWith(next.cloneNode(true));
    return;
  }
  if (current.nodeType === 3) {
    if (current.nodeValue !== next.nodeValue) current.nodeValue = next.nodeValue;
    return;
  }
  if (current.nodeType !== 1) return;
  for (const attr of [...current.attributes]) {
    if (!next.hasAttribute(attr.name)) current.removeAttribute(attr.name);
  }
  for (const attr of next.attributes) {
    if (current.getAttribute(attr.name) !== attr.value) current.setAttribute(attr.name, attr.value);
  }
  const previous = [...current.childNodes];
  const incoming = [...next.childNodes];
  incoming.forEach((node, index) => {
    if (previous[index]) patchOverviewNode(previous[index], node);
    else current.appendChild(node.cloneNode(true));
  });
  previous.slice(incoming.length).forEach(node => node.remove());
}

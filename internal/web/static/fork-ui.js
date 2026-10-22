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

function accountConnectionHTML(account) {
  const needsAuth = account.paused || account.authRequired;
  const label = needsAuth ? 'Sign-in required' : account.isStale ? 'Usage out of date'
    : account.quotas?.length ? 'Usage available' : 'Waiting for first usage';
  const updated = Number.isFinite(Date.parse(account.capturedAt))
    ? `<span>Updated ${escapeHTML(formatDateTime(account.capturedAt))}</span>` : '';
  return `<div class="account-connection-info"><span class="account-connection-state" data-attention="${Boolean(needsAuth || account.isStale)}">${label}</span>${updated}</div>`;
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
    document.getElementById(provider + '-profile-trigger')?.setAttribute('aria-label', 'Select ' + (provider === 'codex' ? 'Codex account' : 'MiniMax account'));
    const trigger = document.getElementById(provider + '-profile-trigger');
    const menu = document.getElementById(provider + '-profile-menu');
    if (!trigger || !menu) return;
    trigger.addEventListener('keydown', event => {
      if (!['ArrowDown', 'ArrowUp'].includes(event.key)) return;
      event.preventDefault();
      if (trigger.getAttribute('aria-expanded') !== 'true') trigger.click();
      (menu.querySelector('[aria-selected="true"]') || menu.firstElementChild)?.focus();
    });
    menu.addEventListener('keydown', event => {
      const items = [...menu.querySelectorAll('[role="option"]')];
      const index = items.indexOf(document.activeElement);
      let next;
      if (event.key === 'ArrowDown') next = (index + 1) % items.length;
      else if (event.key === 'ArrowUp') next = (index - 1 + items.length) % items.length;
      else if (event.key === 'Home') next = 0;
      else if (event.key === 'End') next = items.length - 1;
      else if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault(); document.activeElement.click(); trigger.focus(); return;
      } else if (event.key === 'Escape') { trigger.focus(); return; }
      else return;
      event.preventDefault(); items[next]?.focus();
    });
    menu.addEventListener('focusout', event => {
      if (menu.contains(event.relatedTarget) || event.relatedTarget === trigger) return;
      menu.classList.remove('open'); trigger.setAttribute('aria-expanded', 'false');
    });
  });
  updateOverviewCountdowns();
  setInterval(() => updateOverviewCountdowns(), 1000);
}

document.addEventListener('DOMContentLoaded', setupSidebar);

document.addEventListener('DOMContentLoaded', () => {
  const form = document.querySelector('.login-form');
  if (!form) return;
  const submit = form.querySelector('[type="submit"]');
  const progress = document.getElementById('login-progress');
  const password = document.getElementById('password');
  const reset = () => { submit.disabled = false; form.removeAttribute('aria-busy'); progress.hidden = true; };
  form.addEventListener('submit', event => {
    if (submit.disabled) { event.preventDefault(); return; }
    submit.disabled = true;
    form.setAttribute('aria-busy', 'true');
    progress.hidden = false;
  });
  password?.addEventListener('keyup', event => {
    document.getElementById('login-caps-lock').hidden = !event.getModifierState('CapsLock');
  });
  window.addEventListener('pageshow', reset);
});

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

// Surface configuration gaps without exposing endpoints or credentials.
function notificationDeliverySummary(settings) {
  const channels = settings.notifications?.channels || {email: true, push: true};
  const configured = [];
  const missing = [];
  if (channels.webhook) (settings.webhook?.url ? configured : missing).push('Webhook');
  if (channels.email) (settings.smtp?.host && settings.smtp?.to ? configured : missing).push('Email');
  if (channels.push) configured.push('Browser push');
  return {attention: missing.length > 0,
    text: missing.length ? missing.join(' and ') + ' setup incomplete. Open notification settings.'
      : configured.length ? configured.join(' / ') + ' enabled. Check event filters in settings.' : 'Notification channels are off.'};
}

document.addEventListener('DOMContentLoaded', async () => {
  const center = document.querySelector('.welcome-banner');
  if (!center || typeof getSettingsData !== 'function') return;
  try {
    const summary = notificationDeliverySummary(await getSettingsData());
    const link = document.createElement('a');
    link.className = 'notification-delivery-status'; link.href = (typeof BASE_PATH === 'string' ? BASE_PATH : '') + '/settings';
    link.textContent = summary.text; link.dataset.attention = String(summary.attention);
    center.appendChild(link);
  } catch (e) { /* The existing connection status covers failed dashboard requests. */ }
});

function codexResetCauseLabel(cause) {
  return {scheduled:'Scheduled reset',banked_credit:'Banked reset used',early_unknown:'Early reset, cause unconfirmed'}[cause] || 'Reset detected, cause unknown';
}

function codexResetHistoryHTML(accounts) {
  const groups = new Map();
  accounts.forEach(account => (account.recentResets || []).forEach(reset => {
    if (!Number.isFinite(Date.parse(reset.observedAt))) return;
    const key = String(account.accountId || reset.accountId) + '|' + reset.observedAt;
    if (!groups.has(key)) groups.set(key,{key,name:account.accountName || account.name || 'Codex account',at:reset.observedAt,resets:[]});
    groups.get(key).resets.push(reset);
  }));
  const list = [...groups.values()].sort((a,b) => Date.parse(b.at)-Date.parse(a.at)).slice(0,8);
  if (!list.length) return '';
  const time = value => Number.isFinite(Date.parse(value)) ? escapeHTML(formatDateTime(value)) : 'Not reported';
  const percent = value => Number.isFinite(value) ? value.toFixed(1)+'%' : 'Not recorded';
  return '<h2>Recent resets</h2><p>Reset causes are classified from usage timing and observed credit redemptions. A provider-issued reset cannot be confirmed from timing alone.</p>' + list.map(group => {
    const order = {five_hour:0,seven_day:1,code_review:2};
    group.resets.sort((a,b) => (order[a.quotaName] ?? 3) - (order[b.quotaName] ?? 3));
    const causes = [...new Set(group.resets.map(reset => codexResetCauseLabel(reset.cause)))].join(' / ');
    const limits = group.resets.map(reset => ({five_hour:'5-hour limit',seven_day:'Weekly limit',code_review:'Code review limit'}[reset.quotaName] || reset.quotaName.replaceAll('_',' '))).join(' + ');
    return `<details class="reset-history-entry" data-reset-group="${escapeHTML(group.key)}"><summary><span><strong>${escapeHTML(group.name)}</strong><span class="reset-history-cause">${escapeHTML(causes)}</span><span>${escapeHTML(limits)}</span></span><time>${time(group.at)}</time></summary><div class="reset-history-details">` + group.resets.map(reset => {
      const label = {five_hour:'5-hour limit',seven_day:'Weekly limit',code_review:'Code review limit'}[reset.quotaName] || reset.quotaName.replaceAll('_',' ');
      return `<div><h3>${escapeHTML(label)}</h3><dl><dt>Usage before / after</dt><dd>${percent(reset.beforePercent)} → ${percent(reset.afterPercent)} used</dd><dt>Previously due</dt><dd>${time(reset.previousResetAt)}</dd><dt>Next due</dt><dd>${time(reset.nextResetAt)}</dd></dl><p>${escapeHTML(reset.evidence || 'The cause was not recorded.')}</p></div>`;
    }).join('') + '</div></details>';
  }).join('');
}

function renderCodexResetHistory(accounts) {
  const section = document.getElementById('codex-reset-history');
  if (!section) return;
  const html = codexResetHistoryHTML(accounts);
  if (section._resetHTML === html) return;
  const open = new Set([...section.querySelectorAll('details[open]')].map(el => el.dataset.resetGroup));
  section.innerHTML = html; section.hidden = !html; section._resetHTML = html;
  section.querySelectorAll('details').forEach(el => { el.open = open.has(el.dataset.resetGroup); });
}

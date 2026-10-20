/**
 * onWatch Gemini / Antigravity Google OAuth token import helper.
 *
 * Injects an "Import Google Token" section into the Antigravity and Gemini
 * provider settings modals. Works by watching for the provider-settings-modal
 * to open and checking which provider is active. No upstream files modified.
 */
(() => {
  'use strict';

  const API_BASE = document.querySelector('meta[name="base-path"]')?.getAttribute('content') || '';

  // --- Status indicator injection ---

  // Debounce flag: avoid injecting twice if the modal re-renders quickly.
  let injected = false;

  function injectGeminiImportSection(bodyEl, providerKey) {
    // Only inject for antigravity or gemini settings modals.
    if (providerKey !== 'antigravity' && providerKey !== 'gemini') return;
    if (bodyEl.querySelector('#gemini-import-section')) return;

    const section = document.createElement('div');
    section.id = 'gemini-import-section';
    section.style.cssText =
      'margin-top:20px;padding-top:16px;border-top:1px dashed var(--border,#334155);';

    section.innerHTML = `
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:10px;">
        <h4 style="margin:0;font-size:13px;font-weight:600;color:var(--text,#f8fafc);">
          Google OAuth Token
        </h4>
        <span id="gemini-import-status-badge" style="font-size:12px;color:var(--text-secondary,#94a3b8);">
          Checking...
        </span>
      </div>
      <p style="font-size:12px;color:var(--text-secondary,#94a3b8);margin:0 0 12px;">
        Paste a Google OAuth <strong>refresh token</strong> so onWatch can track
        Gemini / Antigravity quotas from a Docker container. The token is stored
        encrypted in the onWatch database and survives container restarts.
      </p>
      <div id="gemini-import-form">
        <label style="display:block;font-size:13px;font-weight:500;margin-bottom:4px;">Refresh Token</label>
        <input
          type="password"
          id="gemini-refresh-token-input"
          autocomplete="off"
          placeholder="1//0g…"
          style="width:100%;box-sizing:border-box;padding:8px 12px;border-radius:6px;
                 border:1px solid var(--border,#334155);background:var(--surface-inset,#0f172a);
                 color:var(--text,#f8fafc);font-size:13px;font-family:monospace;margin-bottom:8px;"
        >
        <label style="display:block;font-size:13px;font-weight:500;margin-bottom:4px;">Access Token <span style="font-weight:400;color:var(--text-secondary,#94a3b8)">(optional)</span></label>
        <input
          type="password"
          id="gemini-access-token-input"
          autocomplete="off"
          placeholder="ya29.a…"
          style="width:100%;box-sizing:border-box;padding:8px 12px;border-radius:6px;
                 border:1px solid var(--border,#334155);background:var(--surface-inset,#0f172a);
                 color:var(--text,#f8fafc);font-size:13px;font-family:monospace;margin-bottom:12px;"
        >
        <div style="display:flex;gap:8px;">
          <button
            id="gemini-import-save-btn"
            type="button"
            class="btn btn-primary"
            style="flex:1;padding:8px;border-radius:6px;font-weight:600;cursor:pointer;font-size:13px;"
          >
            Save Token
          </button>
          <button
            id="gemini-import-clear-btn"
            type="button"
            style="padding:8px 14px;border-radius:6px;font-weight:500;cursor:pointer;font-size:13px;
                   background:var(--surface-inset,#0f172a);border:1px solid var(--border,#334155);
                   color:var(--text-secondary,#94a3b8);"
          >
            Clear
          </button>
        </div>
      </div>
      <div id="gemini-import-feedback" style="margin-top:10px;font-size:13px;display:none;"></div>
      <details style="margin-top:14px;">
        <summary style="font-size:12px;color:var(--text-secondary,#94a3b8);cursor:pointer;">
          How to get your refresh token
        </summary>
        <div style="font-size:12px;color:var(--text-secondary,#94a3b8);margin-top:8px;line-height:1.6;">
          <p style="margin:0 0 6px;"><strong>Option 1 &ndash; oauth_creds.json file (Gemini CLI):</strong></p>
          <code style="display:block;background:var(--surface-inset,#0f172a);padding:6px 10px;border-radius:4px;margin-bottom:8px;font-size:11px;overflow-x:auto;">
            # Windows PowerShell<br>
            Get-Content "$env:USERPROFILE\.gemini\oauth_creds.json"<br><br>
            # Linux / Mac<br>
            cat ~/.gemini/oauth_creds.json
          </code>
          <p style="margin:0 0 6px;"><strong>Option 2 &ndash; Antigravity / agy CLI state (Windows):</strong></p>
          <code style="display:block;background:var(--surface-inset,#0f172a);padding:6px 10px;border-radius:4px;margin-bottom:8px;font-size:11px;overflow-x:auto;">
            # PowerShell<br>
            Get-Content "$env:APPDATA\Antigravity\credentials.json" 2&gt;$null<br>
            Get-Content "$env:APPDATA\Google\oauth2_creds.json" 2&gt;$null
          </code>
          <p style="margin:0;">
            Copy the <code>refresh_token</code> value and paste it above.
            The access token field is optional; onWatch will refresh it automatically.
          </p>
        </div>
      </details>
    `;

    bodyEl.appendChild(section);

    // Load current status badge.
    loadGeminiImportStatus();

    // Save handler.
    document.getElementById('gemini-import-save-btn').addEventListener('click', async () => {
      const rt = (document.getElementById('gemini-refresh-token-input')?.value || '').trim();
      const at = (document.getElementById('gemini-access-token-input')?.value || '').trim();
      if (!rt) {
        showGeminiImportFeedback('Refresh token is required.', true);
        return;
      }
      showGeminiImportFeedback('Saving...', false);
      try {
        const res = await authFetchOrFetch(`${API_BASE}/api/gemini/import-token`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ refresh_token: rt, access_token: at }),
        });
        if (!res.ok) {
          const err = await res.json().catch(() => ({}));
          throw new Error(err.error || res.statusText);
        }
        showGeminiImportFeedback('Token saved. Gemini quotas will update shortly.', false);
        document.getElementById('gemini-refresh-token-input').value = '';
        document.getElementById('gemini-access-token-input').value = '';
        await loadGeminiImportStatus();
      } catch (e) {
        showGeminiImportFeedback('Error: ' + e.message, true);
      }
    });

    // Clear handler.
    document.getElementById('gemini-import-clear-btn').addEventListener('click', async () => {
      if (!confirm('Remove stored Google OAuth tokens from onWatch?')) return;
      showGeminiImportFeedback('Clearing...', false);
      try {
        const res = await authFetchOrFetch(`${API_BASE}/api/gemini/import-token`, {
          method: 'DELETE',
        });
        if (!res.ok) throw new Error(res.statusText);
        showGeminiImportFeedback('Tokens cleared.', false);
        await loadGeminiImportStatus();
      } catch (e) {
        showGeminiImportFeedback('Error: ' + e.message, true);
      }
    });
  }

  function showGeminiImportFeedback(msg, isError) {
    const el = document.getElementById('gemini-import-feedback');
    if (!el) return;
    el.style.display = 'block';
    el.style.color = isError ? '#ef4444' : '#10b981';
    el.textContent = msg;
  }

  async function loadGeminiImportStatus() {
    const badge = document.getElementById('gemini-import-status-badge');
    if (!badge) return;
    try {
      const res = await authFetchOrFetch(`${API_BASE}/api/gemini/import-status`);
      if (!res.ok) throw new Error(res.statusText);
      const data = await res.json();
      if (data.stored) {
        let label = 'Token stored';
        if (data.has_access && data.expires_in_minutes != null) {
          if (data.expires_in_minutes > 0) {
            label += ` (expires in ${data.expires_in_minutes}m)`;
          } else {
            label += ' (expired, will refresh)';
          }
        } else if (data.has_refresh && !data.has_access) {
          label += ' (refresh token only)';
        }
        badge.textContent = label;
        badge.style.color = '#10b981';
      } else {
        badge.textContent = 'No token stored';
        badge.style.color = 'var(--text-secondary,#94a3b8)';
      }
    } catch {
      badge.textContent = '';
    }
  }

  // Use authFetch if available (injected by app.js), otherwise plain fetch
  // with the CSRF header that onWatch requires.
  function authFetchOrFetch(url, opts) {
    if (typeof authFetch === 'function') {
      return authFetch(url, opts);
    }
    const headers = { 'X-Requested-With': 'XMLHttpRequest', ...(opts?.headers || {}) };
    return fetch(url, { ...opts, headers });
  }

  // --- Modal watcher ---
  // onWatch renders the provider settings modal contents dynamically each time
  // it opens. We watch for the modal to become visible and for the title to
  // indicate it is the Antigravity or Gemini panel.

  function watchModal() {
    const modal = document.getElementById('provider-settings-modal');
    if (!modal) return;

    const observer = new MutationObserver(() => {
      const isOpen = !modal.hidden && modal.style.display !== 'none' &&
                     !modal.classList.contains('hidden');
      if (!isOpen) { injected = false; return; }
      if (injected) return;

      const title = document.getElementById('provider-settings-title')?.textContent || '';
      const isTarget = title.toLowerCase().includes('antigravity') ||
                       title.toLowerCase().includes('gemini');
      if (!isTarget) return;

      const bodyEl = document.getElementById('provider-settings-body');
      if (!bodyEl) return;

      injected = true;
      const providerKey = title.toLowerCase().includes('antigravity') ? 'antigravity' : 'gemini';
      injectGeminiImportSection(bodyEl, providerKey);
    });

    observer.observe(modal, { attributes: true, childList: true, subtree: false });

    // Also watch provider-settings-body for re-renders inside an open modal.
    const bodyObserver = new MutationObserver(() => {
      const modal = document.getElementById('provider-settings-modal');
      if (!modal || modal.hidden) return;

      const title = document.getElementById('provider-settings-title')?.textContent || '';
      const isTarget = title.toLowerCase().includes('antigravity') ||
                       title.toLowerCase().includes('gemini');
      if (!isTarget) { injected = false; return; }

      const bodyEl = document.getElementById('provider-settings-body');
      if (!bodyEl || bodyEl.querySelector('#gemini-import-section')) return;

      injected = false;
      const providerKey = title.toLowerCase().includes('antigravity') ? 'antigravity' : 'gemini';
      injectGeminiImportSection(bodyEl, providerKey);
    });

    const bodyEl = document.getElementById('provider-settings-body');
    if (bodyEl) {
      bodyObserver.observe(bodyEl, { childList: true });
    }
  }

  // Start watching after DOM is ready.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', watchModal);
  } else {
    watchModal();
  }
})();

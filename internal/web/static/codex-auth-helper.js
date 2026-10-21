/**
 * onWatch Codex Authentication & Profile Management Helper
 * Provides in-browser Device Auth (OAuth) and token import directly from the UI.
 */
(() => {
  const API_BASE = document.querySelector('meta[name="base-path"]')?.getAttribute('content') || '';

  // Use authFetch if available (injected by app.js), otherwise plain fetch
  // with the CSRF header that onWatch requires for state-changing requests.
  function authFetchOrFetch(url, opts) {
    if (typeof authFetch === 'function') {
      return authFetch(url, opts);
    }
    const headers = { 'X-Requested-With': 'XMLHttpRequest', ...(opts?.headers || {}) };
    return fetch(url, { ...opts, headers });
  }

  // Periodically check if the Codex settings modal is active
  setInterval(() => {
    const listEl = document.getElementById('codex-profiles-list');
    if (!listEl) return;

    // Check if we already injected our action bar
    if (document.getElementById('codex-auth-helper-actions')) return;

    const container = document.createElement('div');
    container.id = 'codex-auth-helper-actions';
    container.style.cssText = 'margin-top: 14px; padding-top: 12px; border-top: 1px dashed var(--border); display: flex; gap: 10px; align-items: center; justify-content: flex-end;';

    const addBtn = document.createElement('button');
    addBtn.type = 'button';
    addBtn.className = 'btn btn-primary';
    addBtn.style.cssText = 'font-size: 13px; padding: 6px 12px; border-radius: 6px; cursor: pointer; display: flex; align-items: center; gap: 6px;';
    addBtn.innerHTML = `
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <line x1="12" y1="5" x2="12" y2="19"></line>
        <line x1="5" y1="12" x2="19" y2="12"></line>
      </svg>
      Add Account / Re-auth
    `;
    addBtn.onclick = () => openCodexAuthModal();

    container.appendChild(addBtn);
    listEl.parentElement.appendChild(container);
  }, 500);

  function openCodexAuthModal() {
    // Remove any existing modal
    const existing = document.getElementById('codex-auth-modal-overlay');
    if (existing) existing.remove();

    const overlay = document.createElement('div');
    overlay.id = 'codex-auth-modal-overlay';
    overlay.style.cssText = 'position:fixed;inset:0;background:rgba(0,0,0,0.65);z-index:99999;display:flex;align-items:center;justify-content:center;padding:16px;backdrop-filter:blur(2px);';

    const card = document.createElement('div');
    card.style.cssText = 'background:var(--surface,#1e293b);border:1px solid var(--border,#334155);border-radius:12px;width:100%;max-width:480px;box-shadow:0 20px 25px -5px rgba(0,0,0,0.5);overflow:hidden;color:var(--text,#f8fafc);font-family:inherit;';

    card.innerHTML = `
      <div style="padding:16px 20px;border-bottom:1px solid var(--border,#334155);display:flex;justify-content:space-between;align-items:center;">
        <h3 style="margin:0;font-size:16px;font-weight:600;display:flex;align-items:center;gap:8px;">
          <span>Link Codex Account</span>
        </h3>
        <button id="codex-auth-close-btn" style="background:none;border:none;color:var(--text-secondary,#94a3b8);font-size:20px;cursor:pointer;padding:4px 8px;">&times;</button>
      </div>

      <div style="padding:20px;">
        <label style="display:block;font-size:13px;font-weight:500;margin-bottom:6px;">Profile Name</label>
        <input type="text" id="codex-auth-name-input" placeholder="e.g. badfellaxai" style="width:100%;box-sizing:border-box;padding:8px 12px;border-radius:6px;border:1px solid var(--border,#334155);background:var(--surface-inset,#0f172a);color:var(--text,#f8fafc);font-size:14px;margin-bottom:16px;">

        <div style="display:flex;gap:8px;margin-bottom:16px;border-bottom:1px solid var(--border,#334155);padding-bottom:8px;">
          <button id="codex-tab-device" style="background:none;border:none;padding:6px 12px;font-size:13px;font-weight:600;cursor:pointer;color:var(--primary,#0d9488);border-bottom:2px solid var(--primary,#0d9488);">Device Login (Direct)</button>
          <button id="codex-tab-paste" style="background:none;border:none;padding:6px 12px;font-size:13px;font-weight:500;cursor:pointer;color:var(--text-secondary,#94a3b8);">Paste auth.json</button>
        </div>

        <!-- Tab 1: Device Auth -->
        <div id="codex-panel-device">
          <p style="font-size:13px;color:var(--text-secondary,#94a3b8);margin:0 0 16px;">
            Generates an independent device code session. Authorize directly in your browser without entering passwords or modifying workstation files.
          </p>

          <div id="codex-device-start-section">
            <button id="codex-device-start-btn" class="btn btn-primary" style="width:100%;padding:10px;border-radius:6px;font-weight:600;cursor:pointer;">
              Start Device Login
            </button>
          </div>

          <div id="codex-device-waiting-section" style="display:none;text-align:center;padding:12px 0;">
            <div style="font-size:12px;color:var(--text-secondary,#94a3b8);margin-bottom:6px;">1. Open link in your browser</div>
            <a id="codex-device-url" href="https://auth.openai.com/codex/device" target="_blank" rel="noopener noreferrer" style="display:inline-block;padding:8px 16px;background:var(--surface-inset,#0f172a);border:1px solid var(--border,#334155);border-radius:6px;color:var(--primary,#0d9488);text-decoration:none;font-weight:500;font-size:13px;margin-bottom:14px;">
              Open auth.openai.com/codex/device &rarr;
            </a>

            <div style="font-size:12px;color:var(--text-secondary,#94a3b8);margin-bottom:6px;">2. Enter one-time code</div>
            <div id="codex-device-code-display" style="font-size:24px;font-weight:700;font-family:monospace;letter-spacing:2px;color:#38bdf8;padding:8px;background:var(--surface-inset,#0f172a);border-radius:6px;border:1px dashed var(--border,#334155);margin-bottom:16px;">
              ----
            </div>

            <div style="font-size:12px;color:var(--text-secondary,#94a3b8);display:flex;align-items:center;justify-content:center;gap:8px;">
              <span class="spinner" style="display:inline-block;width:12px;height:12px;border:2px solid var(--border,#334155);border-top-color:var(--primary,#0d9488);border-radius:50%;animation:spin 1s linear infinite;"></span>
              Waiting for browser authorization...
            </div>
          </div>
        </div>

        <!-- Tab 2: Paste Raw JSON -->
        <div id="codex-panel-paste" style="display:none;">
          <p style="font-size:13px;color:var(--text-secondary,#94a3b8);margin:0 0 12px;">
            Paste the contents of an <code>auth.json</code> file below:
          </p>
          <textarea id="codex-paste-textarea" rows="6" placeholder='{"auth_mode": "chatgpt", "tokens": { ... }}' style="width:100%;box-sizing:border-box;padding:8px 12px;border-radius:6px;border:1px solid var(--border,#334155);background:var(--surface-inset,#0f172a);color:var(--text,#f8fafc);font-family:monospace;font-size:12px;margin-bottom:16px;resize:vertical;"></textarea>
          <button id="codex-paste-save-btn" class="btn btn-primary" style="width:100%;padding:10px;border-radius:6px;font-weight:600;cursor:pointer;">
            Save Profile
          </button>
        </div>

        <div id="codex-auth-status" style="margin-top:14px;font-size:13px;display:none;"></div>
      </div>
    `;

    overlay.appendChild(card);
    document.body.appendChild(overlay);

    let activePollInterval = null;

    // Close logic
    const closeModal = () => {
      if (activePollInterval) clearInterval(activePollInterval);
      overlay.remove();
    };
    document.getElementById('codex-auth-close-btn').onclick = closeModal;
    overlay.onclick = (e) => {
      if (e.target === overlay) closeModal();
    };

    // Tab toggling
    const tabDevice = document.getElementById('codex-tab-device');
    const tabPaste = document.getElementById('codex-tab-paste');
    const panelDevice = document.getElementById('codex-panel-device');
    const panelPaste = document.getElementById('codex-panel-paste');

    tabDevice.onclick = () => {
      tabDevice.style.color = 'var(--primary,#0d9488)';
      tabDevice.style.borderBottom = '2px solid var(--primary,#0d9488)';
      tabPaste.style.color = 'var(--text-secondary,#94a3b8)';
      tabPaste.style.borderBottom = 'none';
      panelDevice.style.display = 'block';
      panelPaste.style.display = 'none';
    };

    tabPaste.onclick = () => {
      tabPaste.style.color = 'var(--primary,#0d9488)';
      tabPaste.style.borderBottom = '2px solid var(--primary,#0d9488)';
      tabDevice.style.color = 'var(--text-secondary,#94a3b8)';
      tabDevice.style.borderBottom = 'none';
      panelPaste.style.display = 'block';
      panelDevice.style.display = 'none';
    };

    const statusEl = document.getElementById('codex-auth-status');
    const showStatus = (msg, isErr = false) => {
      statusEl.style.display = 'block';
      statusEl.style.color = isErr ? '#ef4444' : '#10b981';
      statusEl.textContent = msg;
    };

    // Device Auth Flow
    document.getElementById('codex-device-start-btn').onclick = async () => {
      const name = document.getElementById('codex-auth-name-input').value.trim();
      if (!name) {
        showStatus('Please specify a profile name first', true);
        return;
      }

      showStatus('Connecting to OpenAI device auth...');
      try {
        const res = await authFetchOrFetch(`${API_BASE}/api/codex/device-auth/start`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();

        document.getElementById('codex-device-start-section').style.display = 'none';
        const waitSection = document.getElementById('codex-device-waiting-section');
        waitSection.style.display = 'block';
        document.getElementById('codex-device-code-display').textContent = data.user_code;

        // Start background polling
        activePollInterval = setInterval(async () => {
          try {
            const pollRes = await authFetchOrFetch(`${API_BASE}/api/codex/device-auth/poll`, {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify({
                device_auth_id: data.device_auth_id,
                user_code: data.user_code,
                profile_name: name,
              }),
            });
            const pollData = await pollRes.json();
            if (pollData.status === 'success') {
              clearInterval(activePollInterval);
              showStatus('Account successfully authorized and saved!');
              setTimeout(() => {
                closeModal();
                if (typeof openProviderSettingsModal === 'function') {
                  openProviderSettingsModal('codex');
                } else {
                  location.reload();
                }
              }, 1200);
            } else if (pollData.status !== 'pending') {
              clearInterval(activePollInterval);
              showStatus(pollData.error || 'Authorization failed', true);
            }
          } catch (e) {
            // Ignore transient network errors during poll
          }
        }, 3500);
      } catch (err) {
        showStatus('Failed to start device login: ' + err.message, true);
      }
    };

    // Paste JSON Flow
    document.getElementById('codex-paste-save-btn').onclick = async () => {
      const name = document.getElementById('codex-auth-name-input').value.trim();
      const authData = document.getElementById('codex-paste-textarea').value.trim();
      if (!name) {
        showStatus('Please specify a profile name', true);
        return;
      }
      if (!authData) {
        showStatus('Please paste your auth.json content', true);
        return;
      }

      showStatus('Saving profile...');
      try {
        const res = await authFetchOrFetch(`${API_BASE}/api/codex/profiles`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, auth_data: authData }),
        });
        if (!res.ok) {
          const errData = await res.json().catch(() => ({}));
          throw new Error(errData.error || res.statusText);
        }
        showStatus('Profile saved successfully!');
        setTimeout(() => {
          closeModal();
          if (typeof openProviderSettingsModal === 'function') {
            openProviderSettingsModal('codex');
          } else {
            location.reload();
          }
        }, 1000);
      } catch (err) {
        showStatus('Error saving profile: ' + err.message, true);
      }
    };
  }
})();

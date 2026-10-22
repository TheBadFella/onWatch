const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');

const source = fs.readFileSync(path.join(__dirname, '../../internal/web/static/app.js'), 'utf8');
const events = ['warning', 'critical', 'reset', 'auth_error', 'starter_success', 'starter_failure'];
const controlId = event => 'smtp-event-' + event.replaceAll('_', '-');

for (const [name, smtp, expected] of [
  ['legacy defaults', { host: 'smtp.example.com' }, ['warning', 'critical', 'reset', 'auth_error']],
  ['auth only', { events: { auth_error: true } }, ['auth_error']],
  ['all disabled', { events: {} }, []],
  ['starter failure only', { events: { starter_failure: true } }, ['starter_failure']],
]) {
  test('SMTP settings load and save: ' + name, async () => {
    const fields = new Map(events.map(event => [controlId(event), { checked: false }]));
    fields.set('smtp-host', { value: '' });
    const context = vm.createContext({
      document: { getElementById: id => fields.get(id) || null, querySelectorAll: () => [] },
      authFetch: async () => ({ ok: true, json: async () => ({ smtp }) }),
      setVal: (id, value) => { if (fields.has(id)) fields.get(id).value = value || ''; },
      State: {}, console,
      loadWebhookSettings: () => {}, collectWebhookSettings: () => null,
      populateProviderToggles: async () => {}, populateDashboardTabOrder: async () => {},
      populateMenubarSettings: async () => {}, syncDashboardTabOrderFromDOM: () => {},
    });
    vm.runInContext(source.slice(source.indexOf('async function loadSettings()'), source.indexOf('async function populateMenubarSettings(')), context);
    vm.runInContext(source.slice(source.indexOf('function gatherSettings()'), source.indexOf('function setupSettingsSave()')), context);
    await context.loadSettings();
    assert.deepEqual(events.filter(event => fields.get(controlId(event)).checked), expected);
    const saved = context.gatherSettings();
    assert.deepEqual(Object.entries(saved.smtp.events).filter(([, enabled]) => enabled).map(([event]) => event), expected);
  });
}

for (const failure of ['http', 'network']) {
  test('failed settings load cannot overwrite saved connection settings: ' + failure, async () => {
    const State = {settingsLoaded: true};
    const context = vm.createContext({State, document:{getElementById:()=>null},
      showSettingsFeedback:()=>{}, authFetch:async()=>{
        if (failure === 'network') throw new Error('offline');
        return {ok:false};
      }});
    vm.runInContext(source.slice(source.indexOf('async function loadSettings()'), source.indexOf('async function populateMenubarSettings(')), context);
    vm.runInContext(source.slice(source.indexOf('function gatherSettings()'), source.indexOf('function setupSettingsSave()')), context);
    await context.loadSettings();
    assert.equal(State.settingsLoaded, false);
    assert.throws(()=>context.gatherSettings(), /load your saved settings/);
  });
}

test('notification status exposes missing destinations without displaying private URLs', () => {
  const fork = fs.readFileSync(path.join(__dirname, '../../internal/web/static/fork-ui.js'), 'utf8');
  const context = vm.createContext({});
  vm.runInContext(fork.slice(fork.indexOf('function notificationDeliverySummary('), fork.indexOf("document.addEventListener('DOMContentLoaded', async")), context);
  const missing = context.notificationDeliverySummary({notifications:{channels:{webhook:true}},webhook:{events:{critical:true}}});
  assert.equal(missing.attention,true);assert.match(missing.text,/Webhook setup incomplete/);
  const ready = context.notificationDeliverySummary({notifications:{channels:{webhook:true}},webhook:{url:'https://private.example/secret-token'}});
  assert.equal(ready.attention,false);assert.doesNotMatch(ready.text,/private|secret/);
});

test('SMTP reminders respect the email channel and saved dismissal', () => {
  const fork = fs.readFileSync(path.join(__dirname, '../../internal/web/static/fork-ui.js'), 'utf8');
  const context = vm.createContext({});
  vm.runInContext(fork.slice(fork.indexOf('function notificationDeliverySummary('), fork.indexOf("document.addEventListener('DOMContentLoaded', async")), context);
  for (const settings of [
    {notifications:{channels:{email:false,webhook:true}},webhook:{url:'https://example.test'}},
    {notifications:{channels:{email:true,webhook:true}},webhook:{url:'https://example.test'},hide_smtp_setup_reminder:true},
  ]) {
    const result = context.notificationDeliverySummary(settings);
    assert.equal(result.attention, false);
    assert.equal(result.smtpReminder, false);
    assert.doesNotMatch(result.text, /Email|SMTP|incomplete/);
  }
  const missing = context.notificationDeliverySummary({notifications:{channels:{email:true}}});
  assert.equal(missing.smtpReminder, true);
  assert.match(missing.text, /Email setup incomplete/);
  const stillMissingWebhook = context.notificationDeliverySummary({notifications:{channels:{email:true,webhook:true}},hide_smtp_setup_reminder:true});
  assert.equal(stillMissingWebhook.attention, true);
  assert.match(stillMissingWebhook.text, /Webhook setup incomplete/);
});

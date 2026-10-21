const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const root = path.join(__dirname, '../..');
const source = fs.readFileSync(path.join(root, 'internal/web/static/fork-ui.js'), 'utf8');
const app = fs.readFileSync(path.join(root, 'internal/web/static/app.js'), 'utf8');
const context = vm.createContext({
  document: { addEventListener() {} },
  formatDuration: seconds => `${Math.floor(seconds / 3600)}h ${Math.floor(seconds % 3600 / 60)}m`,
  accountOverviewQuotas: (provider, account) => account.quotas,
});
vm.runInContext(source, context);
const now = Date.parse('2026-10-21T12:00:00Z');

test('countdowns use the reset timestamp and advance without another API poll', () => {
  const html = context.overviewResetHTML('2026-10-21T14:30:00Z', 1, now);
  assert.match(html, /Resets in 2h 30m/);
  assert.match(html, /data-reset-deadline="1792593000000"/);
  assert.equal(context.overviewResetText(now + 9000000, now + 60000), 'Resets in 2h 29m');
  assert.equal(context.overviewResetText(now - 1000, now), 'Reset due');
});
test('missing and malformed timestamps never invent a reset', () => {
  assert.match(context.overviewResetHTML('', 0, now), /Reset time unavailable/);
  assert.match(context.overviewResetHTML('invalid', 0, now), /Reset time unavailable/);
  assert.match(context.overviewResetHTML('', 3600, now), /Resets in 1h 0m/);
});
test('collapsed provider headers show the earliest account reset', () => {
  const html = context.providerResetHTML({provider:'codex',accountsGroup:[
    {quotas:[{resetAt:'2026-10-21T14:30:00Z'}]},
    {quotas:[{resetAt:'2026-10-21T13:00:00Z'}]},
  ]});
  assert.match(html, /2026-10-21T13:00:00.000Z/);
  assert.equal(context.providerResetHTML({quotas:[]}), '');
});
test('compact Codex cards expose available capacity and reset countdowns', () => {
  const card = vm.createContext({
    ...context, codexDisplayNames:{five_hour:'5-Hour'}, filterCodexQuotasForPlan:q=>q,
    formatCodexPlan:()=>'', codexAutoStartBadge:()=>'', codexUsageSourceNoteHTML:()=>'', escapeHTML:s=>String(s),
    formatResetTime:()=> 'Oct 21, 2:30 PM',
  });
  vm.runInContext(app.slice(app.indexOf('function accountOverviewQuotas('), app.indexOf('// Build one compact, clickable summary card per account')), card);
  const html = card.accountOverviewCardHTML('codex', {accountId:4, name:'Fixture account', quotas:[{
    name:'five_hour', utilization:34, resetsAt:'2026-10-21T14:30:00Z',
  }]},0);
  assert.match(html, /66.0% available/);
  assert.match(html, /data-reset-deadline/);
  assert.match(html, /Oct 21, 2:30 PM/);
});

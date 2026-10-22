const {test}=require('node:test');const assert=require('node:assert/strict');const vm=require('node:vm');const fs=require('node:fs');
const source=fs.readFileSync('internal/web/static/costs.js','utf8');
const context=vm.createContext({Intl,document:{addEventListener(){}}});vm.runInContext(source,context);
const plan=(id,name,used)=>({provider:'codex',account_id:id,account:name,monthly_usd:20,monthly_cad:28.04,weekly:used===null?null:{used_percent:used,used_usd:20*12/52*used/100,used_cad:28.04*12/52*used/100}});
const data={subscription_values:[plan(6,'personal',50),plan(9,'work',25)],usage:[]};
test('two Plus accounts show a compact total without ranges or explanatory blocks',()=>{
 const html=context.costDashboardHTML(data,'codex','all');
 assert.match(html,/CAD[^0-9]*56\.08/);assert.match(html,/USD[^0-9]*40\.00\/mo/);assert.match(html,/2 accounts/);assert.match(html,/CAD[^0-9]*4\.85/);
 assert.doesNotMatch(html,/range|annual|allocation|Canada|conversion|telemetry|Advanced|–/i);
});
test('selecting an account scopes both the monthly fee and weekly value',()=>{
 const html=context.costDashboardHTML(data,'codex',6);
 assert.match(html,/personal cost/);assert.match(html,/CAD[^0-9]*28\.04/);assert.match(html,/CAD[^0-9]*3\.24/);assert.match(html,/50\.0% of weekly quota/);
 assert.doesNotMatch(html,/work|56\.08|4\.85|25\.0%/);
 const second=context.costDashboardHTML(data,'codex',9);assert.match(second,/25\.0% of weekly quota/);assert.match(second,/CAD[^0-9]*1\.62/);assert.doesNotMatch(second,/50\.0%/);
});
test('unknown or stale weekly usage remains unavailable while the fee stays visible',()=>{
 const html=context.costDashboardHTML({subscription_values:[plan(6,'personal',null)]},'codex',6);
 assert.match(html,/28\.04/);assert.match(html,/Unavailable/);assert.doesNotMatch(html,/0\.0%|CAD[^0-9]*0\.00/);
 const unknown=context.costDashboardHTML({unpriced_subscriptions:[{provider:'codex',account_id:2,account:'custom'}]},'codex',2);
 assert.match(unknown,/price unavailable/);assert.doesNotMatch(unknown,/0\.00/);
});
test('missing accounts do not show a cost panel; known free plans preserve zero',()=>{
 assert.equal(context.costDashboardHTML({subscription_values:[]}), '');
 const free={...plan(1,'free',0),monthly_usd:0,monthly_cad:0,weekly:{used_percent:0,used_usd:0,used_cad:0}};
 assert.match(context.costDashboardHTML({subscription_values:[free]},'codex',1),/CAD[^0-9]*0\.00/);
});
test('token estimates stay separate, preserve zero and respect account selection',()=>{
 const api={...data,usage:[{provider:'openai',account:'personal',estimated_usd:0},{provider:'openai',account:'work',estimated_usd:3},{provider:'openai',account:'work',estimated_usd:null}]};
 const personal=context.costDashboardHTML(api,'codex',6);assert.match(personal,/<details class="cost-token-details"><summary>API token value/);assert.match(personal,/<p>USD[^0-9]*0\.00<\/p>/);assert.doesNotMatch(personal,/partial/);
 const work=context.costDashboardHTML(api,'codex',9);assert.match(work,/<p>USD[^0-9]*3\.00 \(partial\)/);
});
test('unpriced accounts mark totals partial and account labels are escaped',()=>{
 const html=context.costDashboardHTML({...data,unpriced_subscriptions:[{provider:'codex',account:'custom'}]});assert.match(html,/Monthly total \(partial\)/);assert.match(html,/2 of 3 accounts reporting/);
 const escaped=context.costDashboardHTML({subscription_values:[plan(1,'<script>',50)]},'codex',1);assert.doesNotMatch(escaped,/<script>/);assert.match(escaped,/&lt;script&gt;/);
});
test('cost comes after usage in the dashboard template and inside each account card',()=>{
 const template=fs.readFileSync('internal/web/templates/dashboard.html','utf8');assert.ok(template.indexOf('id="cost-dashboard"')>template.indexOf('id="quota-grid-codex"'));assert.ok(template.indexOf('id="cost-dashboard"')>template.indexOf('id="all-providers-container"'));
 const app=fs.readFileSync('internal/web/static/app.js','utf8');const start=app.indexOf('function accountOverviewCardHTML(');const card=app.slice(start,app.indexOf('// Build one compact, clickable summary card',start));assert.ok(card.indexOf('account-cost-slot')>card.indexOf('account-overview-quotas'));
});

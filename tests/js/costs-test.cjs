const {test}=require('node:test');const assert=require('node:assert/strict');const vm=require('node:vm');const fs=require('node:fs');
const source=fs.readFileSync('internal/web/static/costs.js','utf8');
const context=vm.createContext({Intl,document:{addEventListener(){}}});vm.runInContext(source,context);
test('cost view distinguishes absent data, free usage and partial estimates',()=>{
 const empty=context.costDashboardHTML({subscriptions:[],usage:[]});assert.match(empty,/Not available/);assert.match(empty,/No token telemetry/);
 const zero=context.costDashboardHTML({subscriptions:[],usage:[{provider:'openai',account:'a',model:'free',input_tokens:1,output_tokens:1,estimated_usd:0}],estimated_api_usd:0});assert.match(zero,/USD[^0-9]*0/);assert.doesNotMatch(zero.slice(zero.indexOf("Advanced: API-equivalent")),/Not available/);
 const partial=context.costDashboardHTML({subscriptions:[{provider:'openai',account:'<script>',monthly_usd:20}],subscription_monthly_usd:20,usage:[{provider:'openai',account:'a',model:'test',input_tokens:100,output_tokens:1,estimated_usd:1}],estimated_api_usd:1,unpriced_requests:4});assert.match(partial,/partial/);assert.match(partial,/4 requests/);assert.doesNotMatch(partial,/<script>/);assert.match(partial,/&lt;script&gt;/);
});

test('subscription and quota values do not depend on token telemetry',()=>{
 const html=context.costDashboardHTML({subscriptions:[{provider:'codex',account:'personal',monthly_usd:20}],subscription_monthly_usd:20,subscription_monthly_cad:30,subscription_annual_usd:240,subscription_annual_cad:360,usage:[],subscription_values:[{provider:'codex',account:'personal',monthly_usd:20,monthly_cad:30,daily_usd:2/3,daily_cad:1,cycle_start:'2026-04-01T00:00:00Z',cycle_end:'2026-05-01T00:00:00Z',cad_source:'Entered CAD fee',windows:[{key:'seven_day',used_percent:50,used_usd:7/3,used_cad:3.5,remaining_usd:7/3,remaining_cad:3.5}]}]});
 assert.match(html,/CAD[^0-9]*30/);assert.match(html,/USD[^0-9]*240/);assert.match(html,/CAD[^0-9]*360/);assert.match(html,/CAD[^0-9]*3.50/);assert.match(html,/Do not add/);assert.match(html,/does not prevent subscription/);
});
test('stale readings and old exchange rates remain explicit',()=>{
 const html=context.costDashboardHTML({subscriptions:[{provider:'codex',account:'a',monthly_usd:20}],subscription_monthly_usd:20,usage:[],exchange:{cad_per_usd:1.5,source:'Bank of Canada',date:'2026-10-20',stale:true},subscription_values:[{provider:'codex',account:'a',monthly_usd:20,daily_usd:1,windows:[],reason:'Quota reading is stale'}]});
 assert.match(html,/Quota reading is stale/);assert.match(html,/older cached rate/);assert.doesNotMatch(html,/undefined|NaN/);
});
test('automatic prices show CAD first, ranges and unpriced accounts',()=>{
 const html=context.costDashboardHTML({subscriptions:[{provider:'codex',account:'work',monthly_usd:20}],subscription_monthly_usd:20,subscription_monthly_max_usd:125,subscription_monthly_cad:30,subscription_monthly_max_cad:187.5,subscription_annual_usd:240,subscription_annual_max_usd:1500,subscription_annual_cad:360,subscription_annual_max_cad:2250,usage:[],unpriced_subscriptions:[{provider:'codex',account:'custom',source:'No public price identified'}],subscription_values:[{provider:'codex',account:'work',monthly_usd:20,monthly_max_usd:125,monthly_cad:30,monthly_max_cad:187.5,daily_usd:1,daily_cad:1.5,windows:[{key:'seven_day',used_percent:50,used_usd:2,used_cad:3,remaining_usd:2,remaining_cad:3}]}]});
 assert.match(html,/CAD[^0-9]*30\.00–CAD[^0-9]*187\.50<small>USD/);assert.match(html,/partial/);assert.match(html,/No public price identified/);assert.match(html,/Excluded from totals/);assert.match(html,/not Canadian checkout prices/);assert.match(html,/CAD[^0-9]*3\.00–CAD[^0-9]*18\.75/);assert.doesNotMatch(html,/NaN|undefined/);
});

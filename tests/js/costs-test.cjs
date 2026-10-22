const {test}=require('node:test');const assert=require('node:assert/strict');const vm=require('node:vm');const fs=require('node:fs');
const source=fs.readFileSync('internal/web/static/costs.js','utf8');
const context=vm.createContext({Intl,document:{addEventListener(){}}});vm.runInContext(source,context);
test('cost view distinguishes absent data, free usage and partial estimates',()=>{
 const empty=context.costDashboardHTML({subscriptions:[],usage:[]});assert.match(empty,/Not configured/);assert.match(empty,/Not available/);assert.match(empty,/No token telemetry/);
 const zero=context.costDashboardHTML({subscriptions:[],usage:[{provider:'openai',account:'a',model:'free',input_tokens:1,output_tokens:1,estimated_usd:0}],estimated_api_usd:0});assert.match(zero,/\$0/);assert.doesNotMatch(zero,/Not available/);
 const partial=context.costDashboardHTML({subscriptions:[{provider:'openai',account:'<script>',monthly_usd:20}],subscription_monthly_usd:20,usage:[{provider:'openai',account:'a',model:'test',input_tokens:100,output_tokens:1,estimated_usd:1}],estimated_api_usd:1,unpriced_requests:4});assert.match(partial,/partial/);assert.match(partial,/4 requests/);assert.doesNotMatch(partial,/<script>/);assert.match(partial,/&lt;script&gt;/);
});

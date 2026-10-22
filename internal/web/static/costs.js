// Cost estimates use explicit telemetry and user-supplied rates, never quota percentages.
function costEscape(value) { return String(value ?? '').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
let costRowID = 0;
function addCostRow(kind, data = {}) {
 const list = document.getElementById('cost-' + kind + '-rows');
 if (!list) return;
 const row = document.createElement('div'); row.className = 'cost-config-row';
 const fields = kind === 'subscription' ? [['provider','Provider','text'],['account','Account name','text'],['monthly_usd','Monthly fee (USD)','number']]
 : [['provider','Provider','text'],['model','Exact model ID','text'],['input_per_million','Input / million (USD)','number'],['cached_input_per_million','Cached input / million (USD)','number'],['output_per_million','Output / million (USD)','number']];
 row.innerHTML = fields.map(([key,label,type])=>{
  const id = 'cost-row-' + (++costRowID);
  return `<div class="settings-field"><label for="${id}">${label}</label><input id="${id}" class="settings-input" data-cost-field="${key}" type="${type}" value="${costEscape(data[key])}" ${type==='number'?'min="0" max="1000000" step="any"':'maxlength="256"'} placeholder="${key==='provider'?'openai':key==='account'?'personal':''}"></div>`;
 }).join('');
 const remove = document.createElement('button');remove.type='button';remove.className='settings-test-btn';remove.textContent='Remove '+(kind==='subscription'?'subscription':'rate');remove.addEventListener('click',()=>row.remove());row.appendChild(remove);list.appendChild(row);
}
function loadCostSettings(settings) {
 for (const [kind,key] of [['subscription','subscriptions'],['rate','rates']]) {
  const list=document.getElementById('cost-'+kind+'-rows'); if(!list)continue;
  list.replaceChildren();(settings?.[key] || []).forEach(row=>addCostRow(kind,row));
 }
}
function collectCostSettings() {
 const result={subscriptions:[],rates:[]};
 for(const [kind,key] of [['subscription','subscriptions'],['rate','rates']]) {
  document.querySelectorAll('#cost-'+kind+'-rows .cost-config-row').forEach(row=>{
   const entry={};row.querySelectorAll('[data-cost-field]').forEach(input=>{
    const value=input.value.trim();
    if(value==='')throw new Error('Complete every cost field or remove the unused row.');
    entry[input.dataset.costField]=input.type==='number'?Number(value):value;
    if(input.type==='number' && (!Number.isFinite(entry[input.dataset.costField]) || entry[input.dataset.costField]<0 || entry[input.dataset.costField]>1000000))throw new Error('Cost values must be between 0 and 1000000 USD.');
   });result[key].push(entry);
  });
 }
 return result;
}
function costDashboardHTML(data) {
 const money=value=>new Intl.NumberFormat(undefined,{style:'currency',currency:'USD',maximumFractionDigits:4}).format(value);
 const plans=data.subscriptions || [], usage=data.usage || [];
 const priced=usage.filter(row=>row.estimated_usd!==null && row.estimated_usd!==undefined);
 const estimated=priced.length ? money(data.estimated_api_usd)+(data.unpriced_requests || data.truncated?' (partial)':'') : 'Not available';
 const rows=usage.map(row=>`<tr><td>${costEscape(row.provider)} / ${costEscape(row.account)}</td><td>${costEscape(row.model)}</td><td>${costEscape((row.input_tokens+row.output_tokens).toLocaleString())}</td><td>${row.estimated_usd!==null && row.estimated_usd!==undefined?money(row.estimated_usd):costEscape(row.reason || 'Not priced')}</td></tr>`).join('');
 return `<h2>Costs <span>All accounts · month to date (UTC)</span></h2><div class="cost-summary-grid"><div><span>Monthly subscription fees</span><strong>${plans.length?money(data.subscription_monthly_usd):'Not configured'}</strong><small>Full monthly fees entered in Settings, without proration.</small></div><div><span>API-equivalent token cost</span><strong>${estimated}</strong><small>Estimate for received telemetry, separate from subscription charges.</small></div></div>`
 + (plans.length?'<details><summary>Subscription breakdown</summary><ul>'+plans.map(plan=>`<li>${costEscape(plan.provider)} / ${costEscape(plan.account)}: ${money(plan.monthly_usd)} per month</li>`).join('')+'</ul></details>':'')
 + (rows?`<details><summary>Token cost breakdown</summary><div class="cost-table-scroll"><table><thead><tr><th>Account</th><th>Model</th><th>Tokens</th><th>Estimated USD</th></tr></thead><tbody>${rows}</tbody></table></div></details>`:'<p>No token telemetry received this month. Quota readings do not contain token counts.</p>')
 + (data.unpriced_requests?`<p>${costEscape(data.unpriced_requests)} requests could not be priced. Add matching model rates and provide cache counts in telemetry.</p>`:'')
 + (data.truncated?'<p>Only the first 500 account/model groups are shown. Totals are partial.</p>':'')
 + `<a href="${typeof BASE_PATH==='string'?BASE_PATH:''}/settings">Configure subscriptions and model rates in General → Costs</a>`;
}
document.addEventListener('DOMContentLoaded',()=>{
 document.getElementById('cost-add-subscription')?.addEventListener('click',()=>addCostRow('subscription'));
 document.getElementById('cost-add-rate')?.addEventListener('click',()=>addCostRow('rate'));
 const panel=document.getElementById('cost-dashboard');if(!panel)return;
 let active=false;
 async function refresh(){
  if(document.hidden || active)return;active=true;
  try{const response=await authFetch('/api/costs');if(!response.ok)throw new Error('unavailable');const data=await response.json();const html=costDashboardHTML(data);if(panel.innerHTML!==html){const open=[...panel.querySelectorAll('details[open]')].map(el=>el.querySelector('summary').textContent);panel.innerHTML=html;panel.querySelectorAll('details').forEach(el=>{el.open=open.includes(el.querySelector('summary').textContent)})}}
  catch(e){if(!panel.children.length)panel.textContent='Cost data is unavailable. Try refreshing.';}
  finally{active=false;}
 }
 refresh();setInterval(refresh,60000);document.addEventListener('visibilitychange',()=>{if(!document.hidden)refresh()});
});

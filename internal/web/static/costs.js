// Subscription allocations are planning estimates, separate from token costs.
function costEscape(value) { return String(value ?? '').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c])); }
let costRowID = 0;
let costKnownAccounts = [];
function costProviderKey(value) {
 const provider=String(value).trim().toLowerCase();
 return ['openai','codex'].includes(provider)?'codex':['anthropic','claude'].includes(provider)?'anthropic':provider;
}
function addCostRow(kind, data = {}) {
 const list=document.getElementById('cost-'+kind+'-rows');
 if(!list)return;
 if(kind==='subscription' && !data.provider){
  const used=[...list.querySelectorAll('.cost-config-row')].map(row=>costProviderKey(row.querySelector('[data-cost-field="provider"]').value)+'|'+row.querySelector('[data-cost-field="account"]').value);
  data=costKnownAccounts.find(account=>!used.includes(costProviderKey(account.provider)+'|'+account.account)) || data;
 }
 const row=document.createElement('div');row.className='cost-config-row';
 const fields=kind==='subscription'
 ? [['provider','Provider','text'],['account','Linked account name','text'],['monthly_usd','Monthly fee (USD)','number'],['monthly_cad','Actual monthly fee (CAD, optional)','number'],['billing_day','Billing day (1–31)','number']]
 : [['provider','Provider','text'],['model','Exact model ID','text'],['input_per_million','Input / million (USD)','number'],['cached_input_per_million','Cached input / million (USD)','number'],['output_per_million','Output / million (USD)','number']];
 row.innerHTML=fields.map(([key,label,type])=>{
  const id='cost-row-'+(++costRowID),value=data[key] ?? (key==='billing_day'?1:'');
  const limits=key==='billing_day'?'min="1" max="31" step="1"':'min="0" max="1000000" step="any"';
  const list=key==='provider'?'list="cost-provider-options"':key==='account'?'list="cost-account-options"':'';
  return `<div class="settings-field"><label for="${id}">${label}</label><input id="${id}" class="settings-input" data-cost-field="${key}" type="${type}" value="${costEscape(value)}" ${type==='number'?limits:'maxlength="256"'} ${list} placeholder="${key==='provider'?'codex':key==='account'?'personal':key==='monthly_cad'?'Use automatic conversion':''}"></div>`;
 }).join('');
 const remove=document.createElement('button');remove.type='button';remove.className='settings-test-btn';remove.textContent='Remove '+(kind==='subscription'?'subscription':'rate');remove.addEventListener('click',()=>row.remove());row.appendChild(remove);list.appendChild(row);
}
function loadCostSettings(settings, accounts = []) {
 costKnownAccounts=accounts || [];
 const options=document.getElementById('cost-account-options');
 if(options)options.innerHTML=costKnownAccounts.map(account=>`<option value="${costEscape(account.account)}">${costEscape(account.provider)}</option>`).join('');
 const fx=document.getElementById('cost-cad-per-usd');if(fx)fx.value=settings?.cad_per_usd ?? '';
 for(const [kind,key] of [['subscription','subscriptions'],['rate','rates']]){
  const list=document.getElementById('cost-'+kind+'-rows');if(!list)continue;
  list.replaceChildren();(settings?.[key] || []).forEach(row=>addCostRow(kind,row));
 }
}
function collectCostSettings() {
 const result={subscriptions:[],rates:[]};
 const fx=document.getElementById('cost-cad-per-usd');
 if(fx && fx.value.trim()!==''){
  result.cad_per_usd=Number(fx.value);
  if(!Number.isFinite(result.cad_per_usd) || result.cad_per_usd<=0 || result.cad_per_usd>100)throw new Error('CAD per USD must be greater than 0 and at most 100.');
 }
 for(const [kind,key] of [['subscription','subscriptions'],['rate','rates']]){
  document.querySelectorAll('#cost-'+kind+'-rows .cost-config-row').forEach(row=>{
   const entry={};row.querySelectorAll('[data-cost-field]').forEach(input=>{
    const value=input.value.trim(),key=input.dataset.costField;
    if(value==='' && key==='monthly_cad')return;
    if(value==='')throw new Error('Complete every required cost field or remove the unused row.');
    entry[key]=input.type==='number'?Number(value):value;
    if(input.type==='number' && (!Number.isFinite(entry[key]) || entry[key]<0 || entry[key]>1000000))throw new Error('Cost values must be between 0 and 1000000.');
    if(key==='billing_day' && (!Number.isInteger(entry[key]) || entry[key]<1 || entry[key]>31))throw new Error('Billing day must be an integer between 1 and 31.');
   });result[key].push(entry);
  });
 }
 return result;
}
function costDashboardHTML(data) {
 const money=(value,currency='USD')=>new Intl.NumberFormat(undefined,{style:'currency',currency,currencyDisplay:'code',maximumFractionDigits:4}).format(value);
 const dual=(usd,cad)=>`${money(usd)}${cad!==null && cad!==undefined?`<small>${money(cad,'CAD')}</small>`:''}`;
 const date=value=>Number.isFinite(Date.parse(value))?costEscape(new Date(value).toISOString().slice(0,10)):'Unknown';
 const plans=data.subscriptions || [],values=data.subscription_values || [],usage=data.usage || [];
 const priced=usage.filter(row=>row.estimated_usd!==null && row.estimated_usd!==undefined);
 const estimated=priced.length?dual(data.estimated_api_usd,data.estimated_api_cad)+(data.unpriced_requests || data.truncated?' (partial)':''):'Not available';
 const rows=usage.map(row=>`<tr><td>${costEscape(row.provider)} / ${costEscape(row.account)}</td><td>${costEscape(row.model)}</td><td>${costEscape((row.input_tokens+row.output_tokens).toLocaleString())}</td><td>${row.estimated_usd!==null && row.estimated_usd!==undefined?money(row.estimated_usd):costEscape(row.reason || 'Not priced')}</td></tr>`).join('');
 const planRows=values.length?values.map(plan=>`<li><strong>${costEscape(plan.provider)} / ${costEscape(plan.account)}</strong>: ${dual(plan.monthly_usd,plan.monthly_cad)} per month. Daily allocation: ${dual(plan.daily_usd,plan.daily_cad)}.<small>Billing period: ${date(plan.cycle_start)} to ${date(plan.cycle_end)} UTC. CAD: ${costEscape(plan.cad_source || 'conversion unavailable')}.</small></li>`).join('')
 : plans.map(plan=>`<li>${costEscape(plan.provider)} / ${costEscape(plan.account)}: ${money(plan.monthly_usd)} per month</li>`).join('');
 const allocationRows=values.map(plan=>{
  const name=costEscape(plan.provider)+' / '+costEscape(plan.account);
  if(!plan.windows?.length)return `<tr><td>${name}</td><td colspan="3">${costEscape(plan.reason || 'Quota value unavailable')}</td></tr>`;
  return plan.windows.map(window=>`<tr><td>${name}<small>Observed: ${plan.observed_at?costEscape(new Date(plan.observed_at).toLocaleString()):'Not reported'}</small></td><td>${window.key==='five_hour'?'5-hour':'Weekly'}<small>${costEscape(window.used_percent.toFixed(1))}% used</small></td><td><span class="cost-mobile-label">Allocated fee used</span>${dual(window.used_usd,window.used_cad)}</td><td><span class="cost-mobile-label">Allocation remaining</span>${dual(window.remaining_usd,window.remaining_cad)}</td></tr>`).join('');
 }).join('');
 let html=`<h2>Subscription costs <span>All accounts · configured monthly fees and current quota readings</span></h2><div class="cost-summary-grid"><div><span>Monthly subscription fees</span><strong>${plans.length?dual(data.subscription_monthly_usd,data.subscription_monthly_cad):'Not configured'}</strong><small>The amount you enter, independent of token telemetry.</small></div><div><span>Annualized subscription fees</span><strong>${plans.length?dual(data.subscription_annual_usd ?? data.subscription_monthly_usd*12,data.subscription_annual_cad):'Not configured'}</strong><small>12 months at the current fee and conversion rate.</small></div></div>`;
 if(data.exchange){const fx=data.exchange;html+=`<p>CAD per USD: ${costEscape(fx.cad_per_usd)} · ${costEscape(fx.source)}${fx.date?' · '+costEscape(fx.date):''}${fx.stale?' · older cached rate':''}. Actual CAD fees override conversion.</p>`;}
 else if(plans.length && data.subscription_monthly_cad===undefined)html+='<p>CAD conversion is unavailable. Enter the CAD amount you pay or a CAD-per-USD rate in Settings.</p>';
 if(planRows)html+='<details><summary>Subscription breakdown</summary><ul>'+planRows+'</ul></details>';
 if(allocationRows)html+=`<details class="cost-quota-values" open><summary>Quota value estimates</summary><p>Planning allocation of the subscription fee: monthly fee × window duration ÷ billing-period duration × quota used. Five-hour and weekly limits overlap. Do not add their values. This is not API token spend or an additional charge.</p><div class="cost-table-scroll"><table class="cost-quota-table"><thead><tr><th>Account</th><th>Window</th><th>Allocated fee used</th><th>Allocation remaining</th></tr></thead><tbody>${allocationRows}</tbody></table></div></details>`;
 html+=`<details><summary>Advanced: API-equivalent token cost</summary><p>${estimated}</p>`
 +(rows?`<div class="cost-table-scroll"><table><thead><tr><th>Account</th><th>Model</th><th>Tokens</th><th>Estimated USD</th></tr></thead><tbody>${rows}</tbody></table></div>`:'<p>No token telemetry received this month. This does not prevent subscription and quota-value calculations.</p>')
 +(data.unpriced_requests?`<p>${costEscape(data.unpriced_requests)} requests could not be priced. Add matching model rates and provide cache counts in telemetry.</p>`:'')
 +(data.truncated?'<p>Only the first 500 account/model groups are shown. Token totals are partial.</p>':'')+'</details>'
 +`<a href="${typeof BASE_PATH==='string'?BASE_PATH:''}/settings">Configure subscriptions in General → Costs</a>`;
 return html;
}
document.addEventListener('DOMContentLoaded',()=>{
 document.getElementById('cost-add-subscription')?.addEventListener('click',()=>addCostRow('subscription'));
 document.getElementById('cost-add-rate')?.addEventListener('click',()=>addCostRow('rate'));
 const panel=document.getElementById('cost-dashboard');if(!panel)return;
 let active=false;
 async function refresh(){
  if(document.hidden || active)return;active=true;
  try{const response=await authFetch('/api/costs');if(!response.ok)throw new Error('unavailable');const data=await response.json();const html=costDashboardHTML(data);if(panel.innerHTML!==html){const states=new Map([...panel.querySelectorAll('details')].map(el=>[el.querySelector('summary').textContent,el.open]));panel.innerHTML=html;panel.querySelectorAll('details').forEach(el=>{const key=el.querySelector('summary').textContent;if(states.has(key))el.open=states.get(key)})}}
  catch(e){if(!panel.children.length)panel.textContent='Cost data is unavailable. Try refreshing.';}
  finally{active=false;}
 }
 refresh();setInterval(refresh,60000);document.addEventListener('visibilitychange',()=>{if(!document.hidden)refresh()});
});

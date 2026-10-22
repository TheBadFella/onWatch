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
function loadCostSettings(settings, accounts = [], detected = []) {
 costKnownAccounts=accounts || [];
 const options=document.getElementById('cost-account-options');
 if(options)options.innerHTML=costKnownAccounts.map(account=>`<option value="${costEscape(account.account)}">${costEscape(account.provider)}</option>`).join('');
 const automatic=document.getElementById('cost-detected-plans');
 if(automatic)automatic.innerHTML=detected.length?'<ul>'+detected.map(plan=>`<li><strong>${costEscape(plan.account)}</strong> · ${costEscape(plan.plan || plan.provider)}<small>${costEscape(plan.source)}${plan.monthly_usd!==null?`: USD ${costEscape(plan.monthly_usd)}${plan.monthly_max_usd?'–'+costEscape(plan.monthly_max_usd):''}/month`:''}</small></li>`).join('')+'</ul>':'<p>Plans will appear automatically after connecting a Codex account and receiving its first usage reading.</p>';
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
let costData = null;
function costMoney(value, currency='CAD') {
 return Number.isFinite(value)?new Intl.NumberFormat('en-CA',{style:'currency',currency,currencyDisplay:'code',maximumFractionDigits:2}).format(value):'Unavailable';
}
function costScope(data, provider='both', accountID=null) {
 const matches=plan=>(provider==='both' || costProviderKey(plan.provider)===costProviderKey(provider)) && (accountID===null || accountID==='all' || String(plan.account_id)===String(accountID));
 return {plans:(data.subscription_values || []).filter(matches),unknown:(data.unpriced_subscriptions || []).filter(matches)};
}
function costAmount(usd,cad,approximate=false) {
 return `<strong>${approximate?'≈ ':''}${costMoney(cad ?? usd,cad==null?'USD':'CAD')}</strong>`;
}
function costAccountSection(plan) {
 const week=plan.weekly;
 return `<section class="account-cost" aria-label="${costEscape(plan.account)} cost"><h3>Cost</h3><div class="cost-metrics"><div title="${costEscape(plan.price_source || 'Plan reference price')}"><span>Monthly plan</span>${costAmount(plan.monthly_usd,plan.monthly_cad)}<small>${costMoney(plan.monthly_usd,'USD')}/mo</small></div><div title="Subscription value based on weekly quota usage. This is not an extra charge."><span>Used this week <span aria-hidden="true">ⓘ</span></span>${week?costAmount(week.used_usd,week.used_cad,true):'<strong>Unavailable</strong>'}<small>${week?`${week.used_percent.toFixed(1)}% of weekly quota`:costEscape(plan.reason?'Waiting for fresh usage':'No weekly usage reported')}</small></div></div></section>`;
}
function costAccountHTML(provider,accountID) {
 if(!costData)return '';
 const {plans,unknown}=costScope(costData,provider,accountID);
 if(plans[0])return costAccountSection(plans[0]);
 return unknown.length?'<section class="account-cost"><h3>Cost</h3><span>Plan price unavailable</span></section>':'';
}
function costTokenHTML(data,provider,accounts,accountID) {
 const names=new Set(accounts.map(plan=>costProviderKey(plan.provider)+'|'+plan.account));
 const usage=(data.usage || []).filter(row=>(provider==='both' || costProviderKey(row.provider)===costProviderKey(provider)) && (accountID===null || accountID==='all' || names.has(costProviderKey(row.provider)+'|'+row.account)));
 if(!usage.length)return '';
 const priced=usage.filter(row=>row.estimated_usd!=null);
 return `<details class="cost-token-details"><summary>API token value</summary><p>${priced.length?costMoney(priced.reduce((sum,row)=>sum+row.estimated_usd,0),'USD'):'Unavailable'}${priced.length!==usage.length?' (partial)':''}</p><small>From received token telemetry, separate from subscription fees.</small></details>`;
}
function costDashboardHTML(data,provider='both',accountID=null) {
 const {plans,unknown}=costScope(data,provider,accountID);
 const tokens=costTokenHTML(data,provider,[...plans,...unknown],accountID);
 if(!plans.length && !unknown.length)return tokens;
 if(accountID!==null && accountID!=='all')return (plans[0]?costAccountSection(plans[0]):'<h2>Cost</h2><p>Plan price unavailable</p>')+tokens;
 const usd=plans.reduce((sum,plan)=>sum+plan.monthly_usd,0);
 const cad=plans.length && plans.every(plan=>plan.monthly_cad!=null)?plans.reduce((sum,plan)=>sum+plan.monthly_cad,0):null;
 const reporting=plans.filter(plan=>plan.weekly);
 const usedUSD=reporting.reduce((sum,plan)=>sum+plan.weekly.used_usd,0);
 const usedCAD=reporting.length && reporting.every(plan=>plan.weekly.used_cad!=null)?reporting.reduce((sum,plan)=>sum+plan.weekly.used_cad,0):null;
 const count=plans.length+unknown.length;
 let html=`<h2>Cost <span>${count} account${count===1?'':'s'}</span></h2><div class="cost-metrics"><div><span>Monthly total${unknown.length?' (partial)':''}</span>${plans.length?costAmount(usd,cad):'<strong>Unavailable</strong>'}<small>${plans.length?costMoney(usd,'USD')+'/mo':'Plan price unavailable'}</small></div><div title="Combined weekly subscription value used. This is not an extra charge."><span>Used this week <span aria-hidden="true">ⓘ</span></span>${reporting.length?costAmount(usedUSD,usedCAD,true):'<strong>Unavailable</strong>'}<small>${reporting.length} of ${count} accounts reporting</small></div></div>`;
 return html+tokens;
}
function paintCosts() {
 if(!costData)return;
 const provider=typeof getCurrentProvider==='function'?getCurrentProvider():'both';
 const accountID=provider==='codex' && typeof State!=='undefined'?State.codexAccount:null;
 const panel=document.getElementById('cost-dashboard');
 if(panel){
  const html=costDashboardHTML(costData,provider,accountID);
  panel.hidden=!html;
  if(panel.innerHTML!==html)panel.innerHTML=html;
 }
 document.querySelectorAll('.account-cost-slot').forEach(slot=>{
  const html=costAccountHTML(slot.dataset.costProvider,slot.dataset.costAccountId || null);
  if(slot.innerHTML!==html)slot.innerHTML=html;
 });
}
document.addEventListener('DOMContentLoaded',()=>{
 document.getElementById('cost-add-subscription')?.addEventListener('click',()=>addCostRow('subscription'));
 document.getElementById('cost-add-rate')?.addEventListener('click',()=>addCostRow('rate'));
 if(!document.getElementById('cost-dashboard'))return;
 let active=false;
 async function refresh(){
  paintCosts();
  if(document.hidden || active)return;
  active=true;
  try{const response=await authFetch('/api/costs');if(!response.ok)throw new Error('unavailable');costData=await response.json();paintCosts();}
  catch(e){/* Keep usage and last known subscription fees visible. */}
  finally{active=false;}
 }
 window.OnWatchCosts={refresh,paint:paintCosts};
 refresh();setInterval(refresh,60000);document.addEventListener('visibilitychange',()=>{if(!document.hidden)refresh()});
});

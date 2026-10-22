const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs'),vm=require('node:vm'),path=require('node:path');
const source=fs.readFileSync(path.join(__dirname,'../../internal/web/static/fork-ui.js'),'utf8');
function context(){const ctx=vm.createContext({escapeHTML:s=>String(s).replaceAll('&','&amp;').replaceAll('<','&lt;').replaceAll('"','&quot;'),formatDateTime:s=>s});vm.runInContext(source.slice(source.indexOf('function codexResetCauseLabel(')),ctx);return ctx;}
test('reset history groups limits by account and observation without inventing causes or zero usage',()=>{
 const c=context(),at='2026-10-22T17:09:00Z';
 const reset={observedAt:at,quotaName:'five_hour',cause:'early_unknown',beforePercent:98,afterPercent:0,previousResetAt:'2026-10-22T20:00:00Z',evidence:'Provider did not identify the actor.'};
 const html=c.codexResetHistoryHTML([{accountId:6,accountName:'<Personal>',recentResets:[reset,{...reset,quotaName:'seven_day',beforePercent:null}]}]);
 assert.equal((html.match(/<details /g)||[]).length,1);assert.match(html,/5-hour limit \+ Weekly limit/);assert.match(html,/cause unconfirmed/);assert.match(html,/Not recorded → 0.0%/);assert.match(html,/&lt;Personal>/);assert.match(html,/Previously due/);
 assert.equal(c.codexResetHistoryHTML([{recentResets:[]}]),'');
});

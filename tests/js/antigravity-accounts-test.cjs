const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const app=fs.readFileSync(path.join(__dirname,'../../internal/web/static/app.js'),'utf8');
function extract(name) {
 const start=app.indexOf('function '+name+'(');
 const end=app.indexOf('\n}',start)+2;
 return app.slice(start,end);
}
test('Antigravity account switch updates name, query and rejects late history',()=>{
 const label={textContent:''};
 const heading={textContent:''};
 const menu={querySelectorAll:()=>[]};
 const dropdown={style:{}};
 const State={antigravityAccount:2,antigravityAccounts:[{id:2,name:'Personal'},{id:3,name:'Work'}],historyRequestSeq:4,currentRange:'24h'};
 let refreshed=0;
 const c=vm.createContext({State,document:{getElementById:id=>({'antigravity-profile-label':label,'antigravity-selected-account-name':heading,'antigravity-profile-menu':menu,'antigravity-profile-dropdown':dropdown}[id])},localStorage:{setItem(){}},getCurrentProvider:()=> 'antigravity',refreshAll:()=>refreshed++});
 for (const name of ['chartAccountKey','historyRequestIsCurrent','antigravityAccountParam','saveAntigravityAccount','updateAntigravityAccountActive','updateAntigravityAccountVisibility','switchAntigravityAccount']) vm.runInContext(extract(name),c);
 assert.equal(c.historyRequestIsCurrent('antigravity',2,'24h',4),true);
 c.switchAntigravityAccount(3);
 assert.equal(label.textContent,'Work');
 assert.equal(heading.textContent,'Work');
 assert.equal(c.antigravityAccountParam(),'&account=3');
 assert.equal(c.historyRequestIsCurrent('antigravity',2,'24h',4),false);
 assert.equal(c.historyRequestIsCurrent('antigravity',3,'24h',4),true);
 assert.equal(refreshed,1);
});
test('Antigravity "All accounts" enables the overview mode and drops the account query',()=>{
 const State={antigravityAccount:'all'};
 const c=vm.createContext({State,getCurrentProvider:()=> 'antigravity'});
 for (const name of ['isAccountsOverviewMode','antigravityAccountParam']) vm.runInContext(extract(name),c);
 assert.equal(c.isAccountsOverviewMode('antigravity'),true);
 assert.equal(c.isAccountsOverviewMode('both'),false);
 assert.equal(c.antigravityAccountParam(),'');
 State.antigravityAccount=3;
 assert.equal(c.isAccountsOverviewMode('antigravity'),false);
 assert.equal(c.antigravityAccountParam(),'&account=3');
});
test('Antigravity overview quotas normalize CLI buckets and IDE model groups',()=>{
 const c=vm.createContext({});
 vm.runInContext(extract('accountOverviewQuotas'),c);
 const rows=c.accountOverviewQuotas('antigravity',{quotas:[
  {modelId:'gemini-5h',label:'Gemini 5h',usagePercent:42.5,remainingPercent:57.5,status:'warning',resetTime:'2026-11-05T12:00:00Z'},
  {quotaGroup:'claude-gpt',displayName:'Claude + GPT',usagePercent:80,status:'danger'},
  {usagePercent:10,remainingPercent:90,status:'healthy'},
 ]});
 assert.deepEqual(JSON.parse(JSON.stringify(rows[0])),{label:'Gemini 5h',quotaName:'gemini-5h',percent:42.5,status:'warning',resetAt:'2026-11-05T12:00:00Z',remaining:57.5});
 assert.deepEqual(JSON.parse(JSON.stringify(rows[1])),{label:'Claude + GPT',quotaName:'claude-gpt',percent:80,status:'danger',resetAt:null,remaining:20});
 assert.equal(rows[2].label,'Quota');
 assert.equal(rows[2].remaining,90);
});

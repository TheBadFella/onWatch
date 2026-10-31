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
 const menu={querySelectorAll:()=>[]};
 const dropdown={style:{}};
 const State={antigravityAccount:2,antigravityAccounts:[{id:2,name:'Personal'},{id:3,name:'Work'}],historyRequestSeq:4,currentRange:'24h'};
 let refreshed=0;
 const c=vm.createContext({State,document:{getElementById:id=>({'antigravity-profile-label':label,'antigravity-profile-menu':menu,'antigravity-profile-dropdown':dropdown}[id])},localStorage:{setItem(){}},getCurrentProvider:()=> 'antigravity',refreshAll:()=>refreshed++});
 for (const name of ['chartAccountKey','historyRequestIsCurrent','antigravityAccountParam','saveAntigravityAccount','updateAntigravityAccountActive','updateAntigravityAccountVisibility','switchAntigravityAccount']) vm.runInContext(extract(name),c);
 assert.equal(c.historyRequestIsCurrent('antigravity',2,'24h',4),true);
 c.switchAntigravityAccount(3);
 assert.equal(label.textContent,'Work');
 assert.equal(c.antigravityAccountParam(),'&account=3');
 assert.equal(c.historyRequestIsCurrent('antigravity',2,'24h',4),false);
 assert.equal(c.historyRequestIsCurrent('antigravity',3,'24h',4),true);
 assert.equal(refreshed,1);
});

const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const app=fs.readFileSync(path.join(__dirname,'../../internal/web/static/app.js'),'utf8');
function extract(name) {
 const start=app.indexOf('function '+name+'(');
 assert.notEqual(start,-1,'missing function '+name);
 const end=app.indexOf('\n}',start)+2;
 return app.slice(start,end);
}
function ctx(State, stored) {
 const c=vm.createContext({State,localStorage:{getItem:()=>stored,setItem(){}}});
 for (const n of ['pickDefaultAccount','applyDefaultCodexSelection','applyDefaultMiniMaxSelection']) vm.runInContext(extract(n),c);
 return c;
}

test('pickDefaultAccount opens the overview whenever several accounts are linked',()=>{
 const c=ctx({},null);
 assert.equal(c.pickDefaultAccount([{id:1},{id:2}],2),'all');
 assert.equal(c.pickDefaultAccount([{id:5}],'all'),5);
 assert.equal(c.pickDefaultAccount([],3),3);
});

test('a remembered single account no longer overrides the overview on load',()=>{
 const State={codexProfiles:[{id:1},{id:2}],codexAccount:2,minimaxAccounts:[{id:7},{id:8}],minimaxAccount:8};
 const c=ctx(State,'2');
 c.applyDefaultCodexSelection();
 c.applyDefaultMiniMaxSelection();
 assert.equal(State.codexAccount,'all');
 assert.equal(State.minimaxAccount,'all');
});

test('a single linked account is selected directly',()=>{
 const State={codexProfiles:[{id:4}],codexAccount:'all',minimaxAccounts:[{id:9}],minimaxAccount:null};
 const c=ctx(State,'all');
 c.applyDefaultCodexSelection();
 c.applyDefaultMiniMaxSelection();
 assert.equal(State.codexAccount,4);
 assert.equal(State.minimaxAccount,9);
});

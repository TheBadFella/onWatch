const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');
const path = require('node:path');
const source = fs.readFileSync(path.join(__dirname, '../../internal/web/static/codex-auth-helper.js'), 'utf8');

function fixture() {
  const nodes = new Map(), listeners = new Map(), timers = new Map(), requests = [];
  let sequence = 0, now = Date.now(), reply = async () => ({ok:true,json:async()=>({device_auth_id:'fixture-id',user_code:'TEST-CODE',interval:'7',expires_at:new Date(now+60000).toISOString()})});
  const document = {activeElement:null,getElementById:id=>nodes.get(id)||null,querySelector:()=>null,addEventListener:(name,fn)=>listeners.set(name,fn)};
  class Element {
    constructor() {this.style={};this.dataset={};this.value='';this.disabled=false;this.hidden=false;this.textContent='';this.children=[];}
    set id(value) {this._id=value;nodes.set(value,this);} get id(){return this._id;}
    set innerHTML(value) {this.html=value;for(const match of value.matchAll(/<[^>]*\bid="([^"]+)"[^>]*>/g)){const el=new Element();el.id=match[1];el.hidden=/\shidden(?:\s|>)/.test(match[0]);}}
    get innerHTML(){return this.html||'';}
    setAttribute(){} addEventListener(name,fn){this[name]=fn;}
    appendChild(child){this.children.push(child);child.parentElement=this;}
    querySelector(){return nodes.get('codex-auth-name-input');} querySelectorAll(){return [];}
    focus(){document.activeElement=this;} remove(){nodes.delete(this.id);}
  }
  document.createElement=()=>new Element();document.body=new Element();
  const context=vm.createContext({document,window:{},AbortController,Date:{now:()=>now,parse:Date.parse},
    navigator:{clipboard:{writeText:async()=>{throw new Error('clipboard denied');}}},location:{reload(){}},
    fetch:async(url,opts)=>{requests.push({url,opts});return reply(url,opts);},
    setInterval:(fn,ms)=>{timers.set(++sequence,{fn,ms});return sequence;},clearInterval:id=>timers.delete(id),
  });
  vm.runInContext(source,context);
  listeners.get('click')({target:{closest:()=>({dataset:{profileName:'personal'}})}});
  return {nodes,requests,timers,context,setReply:fn=>{reply=fn;},advance:ms=>{now+=ms;},start:()=>nodes.get('codex-device-start-btn').onclick(),poll:()=>[...timers.values()][0].fn()};
}

test('device requests use CSRF protection, preserve the name, and respect provider polling interval',async()=>{
  const f=fixture();await f.start();
  assert.equal(f.requests[0].opts.headers['X-Requested-With'],'XMLHttpRequest');
  assert.equal([...f.timers.values()][0].ms,7000);
  f.setReply(async()=>({ok:true,json:async()=>({status:'pending'})}));await f.poll();
  const body=JSON.parse(f.requests[1].opts.body);assert.equal(body.profile_name,'personal');
  assert.equal(body.device_auth_id,'fixture-id');assert.equal(f.nodes.get('codex-auth-done').hidden,true);
});

test('polls never overlap and success stops polling until the user selects Done',async()=>{
  const f=fixture();await f.start();let finish;
  f.setReply(()=>new Promise(resolve=>{finish=resolve;}));
  const pending=f.poll();await f.poll();assert.equal(f.requests.length,2);
  finish({ok:true,json:async()=>({status:'success'})});await pending;
  assert.equal(f.timers.size,0);assert.equal(f.nodes.get('codex-auth-done').hidden,false);
  assert.match(f.nodes.get('codex-auth-status').textContent,/Account connected/);
});

test('expired codes offer retry and clipboard failure never reports a successful copy',async()=>{
  const f=fixture();await f.start();await f.nodes.get('codex-device-copy-btn').onclick();
  assert.match(f.nodes.get('codex-auth-status').textContent,/Copy is unavailable/);
  assert.notEqual(f.nodes.get('codex-device-copy-btn').textContent,'Copied');
  f.advance(61000);await f.poll();assert.equal(f.timers.size,0);
  assert.equal(f.nodes.get('codex-device-start-btn').disabled,false);
  assert.match(f.nodes.get('codex-auth-status').textContent,/expired/);
});

test('closing during a request aborts it and late results cannot create a polling timer',async()=>{
  const f=fixture();let finish;f.setReply(()=>new Promise(resolve=>{finish=resolve;}));
  const pending=f.start();f.nodes.get('codex-auth-close-btn').onclick();
  assert.equal(f.requests[0].opts.signal.aborted,true);
  finish({ok:true,json:async()=>({device_auth_id:'fixture',user_code:'TEST'})});await pending;
  assert.equal(f.timers.size,0);
});

test('switching to import stops device polling, validates JSON before sending, and permits retry',async()=>{
  const f=fixture();await f.start();f.nodes.get('codex-tab-paste').onclick();
  assert.equal(f.timers.size,0);assert.equal(f.nodes.get('codex-paste-save-btn').disabled,false);
  f.nodes.get('codex-paste-textarea').value='{broken';await f.nodes.get('codex-paste-save-btn').onclick();
  assert.equal(f.requests.length,1);assert.match(f.nodes.get('codex-auth-status').textContent,/not a valid/);
  f.nodes.get('codex-paste-textarea').value='{"tokens":{"access_token":"fixture-only"}}';
  f.setReply(async()=>({ok:false,json:async()=>({error:'Fixture import failure'})}));
  await f.nodes.get('codex-paste-save-btn').onclick();assert.equal(f.nodes.get('codex-paste-save-btn').disabled,false);
  assert.equal(f.nodes.get('codex-tab-device').disabled,false);
  f.setReply(async()=>({ok:true,json:async()=>({})}));await f.nodes.get('codex-paste-save-btn').onclick();
  assert.equal(f.nodes.get('codex-paste-textarea').value,'');assert.equal(f.nodes.get('codex-auth-done').hidden,false);
});

test('network failure leaves the name editable and allows a new attempt',async()=>{
  const f=fixture();f.setReply(async()=>{throw new Error('fixture offline');});await f.start();
  assert.equal(f.nodes.get('codex-auth-name-input').readOnly,false);
  assert.equal(f.nodes.get('codex-device-start-btn').disabled,false);
  assert.match(f.nodes.get('codex-auth-status').textContent,/Failed to start/);
});

test('file import reads the selected file without sending it until Save',async()=>{
  const f=fixture();f.nodes.get('codex-tab-paste').onclick();
  const json='{"tokens":{"access_token":"fixture-only"}}';
  await f.nodes.get('codex-auth-file').onchange({target:{files:[{size:json.length,text:async()=>json}]}});
  assert.equal(f.nodes.get('codex-paste-textarea').value,json);assert.equal(f.requests.length,0);
  f.nodes.get('codex-auth-close-btn').onclick();assert.equal(f.nodes.get('codex-paste-textarea').value,'');
});

test('login progress prevents duplicate posts and resets on Back navigation',()=>{
  const ready=[],events={},formEvents={};
  const submit={disabled:false},progress={hidden:true},caps={hidden:true};
  const password={addEventListener:(name,fn)=>{events[name]=fn;}};
  const form={querySelector:()=>submit,addEventListener:(name,fn)=>{formEvents[name]=fn;},setAttribute(){},removeAttribute(){}};
  const nodes={'login-progress':progress,'login-caps-lock':caps,password};
  const context=vm.createContext({document:{addEventListener:(name,fn)=>ready.push(fn),querySelector:()=>form,getElementById:id=>nodes[id]},window:{addEventListener:(name,fn)=>{events[name]=fn;}}});
  const ui=fs.readFileSync(path.join(__dirname,'../../internal/web/static/fork-ui.js'),'utf8');vm.runInContext(ui,context);
  ready[1]();formEvents.submit({preventDefault(){assert.fail('first submit should proceed');}});
  assert.equal(submit.disabled,true);assert.equal(progress.hidden,false);
  let blocked=false;formEvents.submit({preventDefault(){blocked=true;}});assert.equal(blocked,true);
  events.pageshow();assert.equal(submit.disabled,false);assert.equal(progress.hidden,true);
  events.keyup({getModifierState:()=>true});assert.equal(caps.hidden,false);
});

test('sidebar provider navigation opens all accounts without changing card drill-down', () => {
  const app = fs.readFileSync(path.join(__dirname, '../../internal/web/static/app.js'), 'utf8');
  for (const provider of ['codex','minimax','anthropic']) {
    let click, saved = [];
    const tab = {dataset:{provider},addEventListener:(_, fn)=>{click=fn;}};
    const context = vm.createContext({document:{getElementById:()=>({querySelectorAll:()=>[tab]})},
      saveDefaultProvider:()=>{},saveCodexAccount:id=>saved.push(['codex',id]),saveMiniMaxAccount:id=>saved.push(['minimax',id]),BASE_PATH:'',window:{location:{}}});
    vm.runInContext(app.slice(app.indexOf('function setupProviderSelector()'),app.indexOf('function setupHeaderActions()')),context);
    context.setupProviderSelector();click();
    assert.deepEqual(saved,provider==='anthropic'?[]:[[provider,'all']]);
    assert.equal(context.window.location.href,'/?provider='+provider);
  }
  for (const [provider, stateName, fn] of [['codex','codexAccount','updateCodexProfileTabsVisibility'],['minimax','minimaxAccount','updateMiniMaxAccountTabsVisibility']]) {
    const dropdown = {style:{}};
    const State = {codexProfiles:[{},{}],minimaxAccounts:[{},{}],[stateName]:'all'};
    const context = vm.createContext({State,document:{getElementById:()=>dropdown},getCurrentProvider:()=>provider});
    const start=app.indexOf('function '+fn+'()');
    vm.runInContext(app.slice(start,app.indexOf('\n}',start)+2),context);
    context[fn]();assert.equal(dropdown.style.display,'none');
    State[stateName]=6;context[fn]();assert.equal(dropdown.style.display,'');
  }
});

const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const app = fs.readFileSync(path.join(__dirname,'../../internal/web/static/app.js'),'utf8');
const source = app.slice(app.indexOf('function startAutoRefresh()'), app.indexOf('// ── Pagination Helper'));
function fixture() {
  let now=0, sequence=0;
  const calls={current:0,history:0,insights:0,cycles:0,overview:0,sessions:0};
  const timers=new Map(), listeners=new Map();
  const doc={hidden:false,addEventListener:(event,fn)=>listeners.set(event,fn),removeEventListener:(event,fn)=>{if(listeners.get(event)===fn)listeners.delete(event);}};
  const context=vm.createContext({
    document:doc, State:{}, Date:{now:()=>now}, REFRESH_INTERVAL:Number(app.match(/const REFRESH_INTERVAL = (\d+)/)[1]),
    setInterval:(fn,ms)=>{timers.set(++sequence,{fn,ms});return sequence;},clearInterval:id=>timers.delete(id),
    fetchCurrent:async live=>{calls.current++; assert.equal(live,true);},
    fetchHistory:async()=>{calls.history++;},fetchDeepInsights:async()=>{calls.insights++;},
    fetchCycles:async()=>{calls.cycles++;},fetchCycleOverview:async()=>{calls.overview++;},fetchSessions:async()=>{calls.sessions++;},
    shouldShowCyclesTable:()=>true,shouldShowOverviewTable:()=>true,shouldShowSessionsTable:()=>true,
    _lazyLoaded:new Set(['.cycles-section','.cycle-overview-section','.sessions-section']),
  });
  vm.runInContext(source,context);
  context.startAutoRefresh();
  return {context,calls,timers,listeners,doc,setTime:value=>{now=value;},tick:()=>[...timers.values()][0].fn()};
}
test('usage refreshes every five seconds; history stays on its minute cadence',async()=>{
  const f=fixture();assert.equal([...f.timers.values()][0].ms,5000);
  f.setTime(5000);await f.tick();assert.equal(f.calls.current,1);assert.equal(f.calls.history,0);
  f.setTime(60000);await f.tick();assert.equal(f.calls.current,2);assert.equal(f.calls.history,1);assert.equal(f.calls.insights,1);assert.equal(f.calls.sessions,1);
  f.setTime(65000);await f.tick();assert.equal(f.calls.history,1);
});
test('hidden tabs pause and catch up when made visible',async()=>{
  const f=fixture();f.doc.hidden=true;f.setTime(180000);await f.tick();assert.equal(f.calls.current,0);
  f.doc.hidden=false;await f.listeners.get('visibilitychange')();assert.equal(f.calls.current,1);assert.equal(f.calls.history,1);
});
test('slow requests do not overlap and failures do not stop later refreshes',async()=>{
  const f=fixture();let finish;
  f.context.fetchCurrent=()=>{f.calls.current++;return new Promise(resolve=>{finish=resolve;});};
  const pending=f.tick();await f.tick();assert.equal(f.calls.current,1);finish();await pending;
  f.context.fetchCurrent=async()=>{f.calls.current++;throw new Error('fixture network error');};
  await f.tick();await f.tick();assert.equal(f.calls.current,3);
});
test('reinitialization replaces the old timer and visibility listener',()=>{
  const f=fixture();const original=f.listeners.get('visibilitychange');f.context.startAutoRefresh();
  assert.equal(f.timers.size,1);assert.equal(f.listeners.size,1);assert.notEqual(f.listeners.get('visibilitychange'),original);
});

test('unchanged snapshots avoid chart redraws while new snapshots and manual refreshes still render',()=>{
  const context=vm.createContext({document:{addEventListener(){}}});
  const ui=fs.readFileSync(path.join(__dirname,'../../internal/web/static/fork-ui.js'),'utf8');vm.runInContext(ui,context);
  const first={capturedAt:'2026-10-21T12:00:00Z',quotas:[{utilization:30,timeUntilResetSeconds:3600}]};
  assert.equal(context.shouldRenderLiveData('codex:all',first),true);
  assert.equal(context.shouldRenderLiveData('codex:all',{...first,quotas:[{utilization:30,timeUntilResetSeconds:3595}]}),false);
  assert.equal(context.shouldRenderLiveData('codex:all',{...first,capturedAt:'2026-10-21T12:01:00Z'}),true);
  assert.equal(context.shouldRenderLiveData('codex:other',first),true);
  assert.equal(context.shouldRenderLiveData('codex:other',first,true),true);
  assert.equal(context.shouldRenderLiveData('codex:other',{...first,paused:true}),true);
});

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
function load(names, extra={}) {
 const c=vm.createContext({codexDisplayNames:{},filterCodexQuotasForPlan:q=>q,...extra});
 for (const name of names) vm.runInContext(extract(name),c);
 return c;
}

test('quotaModeLabel names the shown figure and gives the other one',()=>{
 const c=load(['quotaModeLabel']);
 assert.equal(c.quotaModeLabel(false,40),'Used · 60.0% left');
 assert.equal(c.quotaModeLabel(true,40),'Remaining · 40.0% used');
 assert.equal(c.quotaModeLabel(false,140),'Used · 0.0% left');
});

const overviewNames=['quotaModeLabel','quotaUsedPercent','minimaxWindowLabel','overviewQuotaRow','accountOverviewQuotas'];

test('overview rows show used for every provider in Usage mode',()=>{
 const c=load(overviewNames);
 const ag=c.accountOverviewQuotas('antigravity',{quotas:[{modelId:'m',usagePercent:30,remainingPercent:70}]})[0];
 const mm=c.accountOverviewQuotas('minimax',{quotas:[{name:'general',usagePercent:25}]})[0];
 const cx=c.accountOverviewQuotas('codex',{quotas:[{name:'five_hour',utilization:10}]})[0];
 assert.deepEqual([ag.percent,mm.percent,cx.percent],[30,25,10]);
 assert.equal(ag.caption,'Used · 70.0% left');
 assert.equal(cx.caption,'Used · 90.0% left');
 const fractionOnly=c.accountOverviewQuotas('antigravity',{quotas:[{modelId:'m',remainingFraction:0.25}]})[0];
 assert.equal(fractionOnly.percent,75);
});

test('overview rows show remaining for every provider in Remaining mode',()=>{
 const c=load(overviewNames);
 const rem=(used)=>({cardPercent:100-used,cardLabel:'Remaining'});
 const ag=c.accountOverviewQuotas('antigravity',{quotas:[{modelId:'m',usagePercent:30,...rem(30)}]})[0];
 const mm=c.accountOverviewQuotas('minimax',{quotas:[{name:'general',usagePercent:25,...rem(25)}]})[0];
 const cx=c.accountOverviewQuotas('codex',{quotas:[{name:'five_hour',utilization:10,...rem(10)}]})[0];
 assert.deepEqual([ag.percent,mm.percent,cx.percent],[70,75,90]);
 assert.equal(ag.caption,'Remaining · 30.0% used');
 assert.equal(mm.caption,'Remaining · 25.0% used');
});

test('Command Code remaining label keeps the credit figures',()=>{
 const c=load(['commandCodeUsd','commandCodeCardLabel']);
 assert.equal(c.commandCodeCardLabel({name:'five_hour',used:1.5,limit:14,cardLabel:'Remaining'}),'12.50 / 14.00 cr left');
 assert.equal(c.commandCodeCardLabel({name:'five_hour',used:1.5,limit:14}),'1.50 / 14.00 cr used');
 assert.equal(c.commandCodeCardLabel({name:'monthly',used:2,limit:10,cardLabel:'Remaining'}),'$8.00 / $10.00 left');
 assert.equal(c.commandCodeCardLabel({name:'monthly',limitUnknown:true,remaining:3}),'$3.00 remaining');
});

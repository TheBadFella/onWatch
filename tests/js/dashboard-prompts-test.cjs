const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const source=fs.readFileSync('internal/web/static/app.js','utf8');
const fork=fs.readFileSync('internal/web/static/fork-ui.js','utf8');
for (const hidden of [true,false]) test('dashboard prompt preferences load and save: '+hidden,async()=>{
 const fields=new Map(['hide_connect_prompt','hide_smtp_setup_reminder'].map(key=>['settings-'+key.replaceAll('_','-'),{checked:!hidden}]));
 const context=vm.createContext({State:{},document:{getElementById:id=>fields.get(id)||null,querySelectorAll:()=>[]},
 authFetch:async()=>({ok:true,json:async()=>({hide_connect_prompt:hidden,hide_smtp_setup_reminder:hidden})}),
 loadWebhookSettings:()=>{},collectWebhookSettings:()=>null,populateProviderToggles:async()=>{},populateDashboardTabOrder:async()=>{},populateMenubarSettings:async()=>{},syncDashboardTabOrderFromDOM:()=>{},console});
 vm.runInContext(source.slice(source.indexOf('async function loadSettings()'),source.indexOf('async function populateMenubarSettings(')),context);
 vm.runInContext(source.slice(source.indexOf('function gatherSettings()'),source.indexOf('function setupSettingsSave()')),context);
 await context.loadSettings();
 const saved=context.gatherSettings();
 assert.equal(saved.hide_connect_prompt,hidden);assert.equal(saved.hide_smtp_setup_reminder,hidden);
});
for (const succeeds of [true,false]) test('homepage dismissal saves only the reminder preference: '+succeeds,async()=>{
 const nodes=[];let ready,change,request,invalidated=false;
 const make=()=>{const node={children:[],append(...children){this.children.push(...children)},appendChild(child){this.children.push(child)},setAttribute(){},addEventListener(event,fn){if(event==='change')change=fn},remove(){this.removed=true},dataset:{}};nodes.push(node);return node};
 const center=make();const classes=[];
 const context=vm.createContext({document:{querySelector:()=>center,createElement:make,createTextNode:text=>({text}),body:{classList:{toggle:(...args)=>classes.push(args)}},addEventListener:(event,fn)=>{ready=fn}},
 getSettingsData:async()=>({hide_connect_prompt:true,notifications:{channels:{email:true,webhook:true}}}),
 authFetch:async(url,options)=>{request={url,...options};return {ok:succeeds}},invalidateSettingsData:()=>{invalidated=true}});
 const start=fork.indexOf('function notificationDeliverySummary(');
 const end=fork.indexOf('function codexResetCauseLabel(');
 vm.runInContext(fork.slice(start,end),context);
 await ready();await change();
 assert.deepEqual(JSON.parse(request.body),{hide_smtp_setup_reminder:true});
 assert.equal(request.method,'PUT');assert.equal(invalidated,succeeds);
 assert.deepEqual(classes,[['hide-connect-prompt',true]]);
 const input=nodes.find(node=>node.type==='checkbox');
 const feedback=nodes.find(node=>node.textContent==='Could not save. Please try again.');
 if(succeeds){assert.ok(nodes.some(node=>node.textContent==='Webhook setup incomplete. Open notification settings.'));assert.ok(input.disabled)}
 else {assert.equal(input.disabled,false);assert.equal(input.checked,false);assert.ok(feedback)}
});

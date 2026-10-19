/** Browser-only UI regression harness. All fixture data stays in this test, never in the product bundle.
 * Builds to memory and intercepts the test origin; starts no HTTP server or proxy core.
 * Run from repository root: node src/web/tests/verify.mjs
 */
import { build } from 'vite';
import react from '@vitejs/plugin-react';
import { chromium } from '@playwright/test';
import { existsSync, mkdirSync } from 'node:fs';
import assert from 'node:assert/strict';
import { verifyRelayImportUi, verifyNodeGroupUi } from './relay-regressions.mjs';
import { verifyPoolCards } from './pool-regressions.mjs';

const output = await build({ configFile:false, root:'src/web', plugins:[react()], publicDir:false, build:{write:false,emptyOutDir:false}, logLevel:'warn' });
const files = new Map((Array.isArray(output) ? output[0] : output).output.map(asset => ['/' + asset.fileName,asset.type === 'chunk' ? asset.code : asset.source]));
assert(files.has('/index.html'),'Vite emits frontend index');
console.log('PASS production Vite build (in memory, no output directory writes)');
const now = '2026-01-01T00:00:00Z';
let state = {
  nodes:[], groups:[], pools:[], listeners:[], subscriptions:[], revision:0,
  settings:{testUrl:'https://example.com/generate_204',speedTestUrl:'https://example.com/download',testTimeoutMs:8000,testConcurrency:8,speedTestBytes:10485760,healthIntervalSeconds:300,portRangeStart:20000,portRangeEnd:20999},
  runtime:{available:false,running:false,applying:false,version:'',appliedRevision:0,lastError:'',startedAt:'',dirty:false,listeners:[]},
  activities:[], jobs:[]
};
let auth = true;
let rejectAuth = false;
let offline = false;
const token = 'ui-test-session-token';
const calls = [];
const errors = [];
let bootstrapCount = 0;
let listenerPayload;
let previewCount = 0;
const importMocks = {preview:null,commit:null};
let nextId = 0;
const browserPath = chromium.executablePath();
const browser = await chromium.launch(existsSync(browserPath) ? {headless:true} : {headless:true,channel:'chrome'});
const context = await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'});
const page = await context.newPage();
page.on('pageerror',error => errors.push(error.message));
await context.route('http://uniproxy.test/**',async route => {
  const request = route.request();
  const url = new URL(request.url());
  const path = url.pathname;
  if (!path.startsWith('/api/')) {
    const key = path === '/' ? '/index.html' : path;
    if (!files.has(key)) {await route.fulfill({status:404,body:''});return;}
    await route.fulfill({status:200,contentType:key.endsWith('.js') ? 'application/javascript' : key.endsWith('.css') ? 'text/css' : 'text/html',body:files.get(key)});return;
  }
  if (path === '/api/bootstrap' && offline) {await route.abort('failed');return;}
  if (rejectAuth || (auth && request.headers().authorization !== 'Bearer ' + token)) {await route.fulfill({status:401,json:{error:'请输入有效的管理员 API Token'}});return;}
  const body = request.postDataJSON();
  calls.push({path,method:request.method(),body});
  if (path === '/api/bootstrap') {bootstrapCount++;await route.fulfill({json:state});return;}
  if (path === '/api/import' && body.preview) {
    previewCount++;
    const reply = importMocks.preview ? await importMocks.preview(body) : {json:{nodes:[{name:'回归测试节点',protocol:'socks5',server:'127.0.0.1',port:1080,udp:true}],errors:[]}};
    await route.fulfill(reply);return;
  }
  let result = {};
  if (path === '/api/import' && importMocks.commit) {result = await importMocks.commit(body);}
  else if (path === '/api/import') {
    const groups = body.groupNames.map(name => ({id:'g' + (++nextId),name,color:'#0f766e',createdAt:now})); state.groups.push(...groups);
    state.nodes.push({id:'node-1',name:'回归测试节点',protocol:'socks5',server:'127.0.0.1',port:1080,udp:true,enabled:true,manual:true,customName:false,groupIds:[...body.groupIds,...groups.map(group => group.id)],sourceIds:[],status:'unknown',latencyMs:null,speedMbps:null,lastCheckedAt:'',lastError:'',createdAt:now,updatedAt:now});
    result = {added:1,updated:0,removed:0,skipped:0,errors:[],subscriptionIds:[]};
  } else if (path === '/api/groups' && request.method() === 'POST') {result = {...body,id:'g' + (++nextId),createdAt:now};state.groups.push(result);}
  else if (path.startsWith('/api/groups/') && request.method() === 'PATCH') {const group = state.groups.find(item => item.id === path.split('/').at(-1));Object.assign(group,body);result=group;}
  else if (path === '/api/pools/with-listener') {const pool={...body.pool,id:'pool-' + (++nextId),createdAt:now,updatedAt:now};const listener={...body.listener,id:'listener-' + (++nextId),name:body.listener.name || pool.name,kind:'pool',targetId:pool.id,port:body.listener.port || 20000,users:body.listener.users.map(user=>({username:user.username,hasPassword:!!user.password})),createdAt:now,updatedAt:now};state.pools.push(pool);state.listeners.push(listener);result={pool,listener};}
  else if (path === '/api/pools') {result = {...body,id:'pool-' + (++nextId),createdAt:now,updatedAt:now};state.pools.push(result);}
  else if (/^\/api\/pools\/[^/]+\/members\/[^/]+$/.test(path) && request.method()==='DELETE') {const parts=path.split('/');const pool=state.pools.find(item=>item.id===parts[3]);const id=decodeURIComponent(parts[5]);pool.nodeIds=pool.nodeIds.filter(item=>item!==id);pool.excludeNodeIds=[...new Set([...pool.excludeNodeIds,id])];result=pool;}
  else if (path === '/api/tests') {const job={id:'pool-check-' + (++nextId),kind:body.kind,status:'queued',total:body.nodeIds.length,completed:0,results:[],createdAt:now,finishedAt:''};state.jobs.unshift(job);await route.fulfill({status:202,json:job});return;}
  else if (path === '/api/nodes/batch') {for(const node of state.nodes.filter(node => body.ids.includes(node.id))) {if(body.action==='disable')node.enabled=false;if(body.action==='enable')node.enabled=true;} result={affected:body.ids.length};}
  else if (path.startsWith('/api/listeners/')) {listenerPayload=body;result={...state.listeners[0],...body,users:body.users.map(user=>({username:user.username,hasPassword:true}))};state.listeners[0]=result;}
  else if (path.startsWith('/api/pools/') && request.method() === 'PUT') {const pool=state.pools.find(item=>item.id===path.split('/').at(-1));Object.assign(pool,body);result=pool;}
  else if (path.startsWith('/api/jobs/') && path.endsWith('/cancel')) {state.jobs[0].status='cancelled';result=state.jobs[0];}
  else if (path.startsWith('/api/subscriptions/') && path.endsWith('/refresh')) {result={added:0,updated:1,removed:0,skipped:0,errors:[],subscriptionIds:['sub-1']};}
  else if (path.startsWith('/api/subscriptions/') && request.method()==='PATCH') {Object.assign(state.subscriptions[0],body);result=state.subscriptions[0];}
  else if (path === '/api/import/backup') {state={...state,...body.state};result={restored:true};}
  else if (path === '/api/settings') {state.settings={...state.settings,...body};result=state.settings;}
  else {await route.fulfill({status:400,json:{error:'Unexpected test request: ' + path}});return;}
  state.revision++;state.runtime.dirty=true;
  await route.fulfill({json:{data:result,runtime:state.runtime}});
});
async function textVisible(text) {await page.getByText(text,{exact:true}).first().waitFor({state:'visible'});}
async function clickNav(name) {await page.getByRole('navigation').getByRole('link',{name,exact:true}).click();}
async function noOverflow(label) {
  try {await page.waitForFunction(()=>document.documentElement.scrollWidth<=window.innerWidth+1,null,{timeout:2500,polling:'raf'});}
  catch {console.error('OVERFLOW',label,await page.evaluate(()=>({width:innerWidth,scroll:document.documentElement.scrollWidth,small:matchMedia('(max-width:560px)').matches,mainMargin:getComputedStyle(document.querySelector('.main-shell')).marginLeft,elements:[...document.querySelectorAll('body *')].map(element=>({tag:element.tagName,className:element.className,left:element.getBoundingClientRect().left,right:element.getBoundingClientRect().right})).filter(item=>item.left<0||item.right>innerWidth+1).slice(0,30)})));assert.fail(label+' has persistent document horizontal overflow');}
}
async function closeDialog() {await page.getByRole('button',{name:'关闭对话框'}).click();}
mkdirSync('src/web/tests/artifacts',{recursive:true});
try {
  await page.goto('http://uniproxy.test/');
  await textVisible('验证管理访问');
  await page.getByLabel('管理令牌',{exact:true}).fill(token);
  await page.getByRole('button',{name:'验证并进入',exact:true}).click();
  await page.getByRole('heading',{name:'节点管理',exact:true}).waitFor();
  assert.equal(await page.evaluate(()=>sessionStorage.getItem('uniproxy.admin-token')),token);
  assert.equal(await page.evaluate(()=>localStorage.length),0);
  assert(!page.url().includes(token),'token never reaches URL');
  await textVisible('从第一条线路开始');
  await textVisible('内核尚不可用，配置仅保存到管理服务');
  await noOverflow('Desktop empty dashboard');
  await page.screenshot({path:'src/web/tests/artifacts/empty-desktop.png',fullPage:true});
  console.log('PASS 401 token authentication, sessionStorage-only persistence, truthful empty/core state');
  const count = bootstrapCount;
  await page.waitForFunction(()=>document.querySelector('.main-footer')?.textContent?.includes('最近同步'));
  await page.waitForResponse(response=>new URL(response.url()).pathname==='/api/bootstrap',{timeout:7000});
  assert(bootstrapCount>count,'five second bootstrap polling');
  console.log('PASS bootstrap polling');
  await page.getByRole('button',{name:'导入节点',exact:true}).first().click();
  await page.getByLabel('节点内容',{exact:true}).fill('socks5://127.0.0.1:1080#回归测试节点');
  await page.getByLabel('同时新建分组',{exact:true}).fill('回归分组');
  await page.getByRole('button',{name:'预览解析',exact:true}).click();
  await page.getByRole('heading',{name:/导入预览/}).waitFor();
  assert.equal(state.nodes.length,0,'preview never mutates state');
  assert.equal(previewCount,1);
  await page.getByRole('button',{name:'确认导入',exact:true}).click();
  await textVisible('导入处理完成');
  await page.getByRole('button',{name:'完成',exact:true}).click();
  await page.getByRole('button',{name:'回归测试节点',exact:true}).waitFor();
  await page.getByLabel('选择 回归测试节点',{exact:true}).check();
  await page.getByRole('button',{name:'禁用',exact:true}).click();
  await page.getByRole('button',{name:'启用 回归测试节点',exact:true}).waitFor();
  await page.getByRole('button',{name:'启用',exact:true}).click();
  await page.getByRole('button',{name:'禁用 回归测试节点',exact:true}).waitFor();
  await page.getByLabel('搜索节点名称、协议、地址或分组').fill('no-match');
  await textVisible('没有符合条件的节点');
  await page.getByLabel('搜索节点名称、协议、地址或分组').fill('');
  console.log('PASS import preview/commit, new groups, batch enable/disable and real search');
  await clickNav('分组管理');
  await page.getByRole('button',{name:'新建分组',exact:true}).first().click();
  await page.getByLabel('分组名称',{exact:true}).fill('新分组');
  await page.getByRole('button',{name:'创建分组',exact:true}).click();
  await page.getByRole('button',{name:'管理分组 新分组 的节点',exact:true}).waitFor();
  await clickNav('代理池');
  await page.getByRole('button',{name:'新建代理池',exact:true}).first().click();
  await page.getByLabel('代理池名称',{exact:true}).fill('用户名哈希池');
  await page.getByLabel('调度策略',{exact:true}).selectOption('consistent-hashing');
  await page.getByLabel('哈希键',{exact:true}).selectOption('in-user');
  await page.getByRole('group',{name:/^包含节点/}).getByRole('checkbox',{name:'回归测试节点',exact:true}).check();
  await page.getByRole('button',{name:'添加账号',exact:true}).click();
  await page.getByLabel('用户名 1',{exact:true}).fill('pool-fixture');
  await page.getByLabel('密码 1',{exact:true}).fill('fixture-only-password');
  await page.getByRole('button',{name:'创建代理池与监听',exact:true}).click();
  await page.getByRole('article',{name:'代理池 用户名哈希池',exact:true}).waitFor();
  assert.equal(state.pools[0].hashKey,'in-user');
  assert.deepEqual(state.pools[0].nodeIds,['node-1']);
  console.log('PASS group creation and consistent hashing pool membership');
  state.listeners=[{id:'listener-1',name:'已有认证监听',kind:'node',targetId:'node-1',protocol:'mixed',host:'127.0.0.1',port:20000,udp:false,enabled:true,users:[{username:'alice',hasPassword:true}],createdAt:now,updatedAt:now}];
  await page.getByRole('button',{name:'立即同步数据',exact:true}).click();
  await page.waitForResponse(response=>new URL(response.url()).pathname==='/api/bootstrap');
  await clickNav('监听端口');
  await page.getByRole('button',{name:'编辑监听 已有认证监听',exact:true}).click();
  await page.getByLabel('监听名称',{exact:true}).fill('保留密码监听');
  await page.getByRole('button',{name:'保存监听',exact:true}).click();
  await page.getByRole('button',{name:'保留密码监听',exact:true}).waitFor();
  assert.deepEqual(listenerPayload.users,[{username:'alice'}],'unchanged password omitted, never masked placeholder');
  console.log('PASS listener password preservation');
  await clickNav('节点管理');
  while(await page.getByRole('button',{name:'关闭通知',exact:true}).count()) await page.getByRole('button',{name:'关闭通知',exact:true}).first().click();
  await page.screenshot({path:'src/web/tests/artifacts/node-fixture-desktop.png',fullPage:true});
  for(const width of [375,768,1024]) {await page.setViewportSize({width,height:900});await noOverflow('Dashboard '+width);}
  await page.setViewportSize({width:375,height:900});
  await page.screenshot({path:'src/web/tests/artifacts/node-fixture-mobile.png',fullPage:true});
  await page.getByRole('button',{name:'导入节点',exact:true}).first().click();
  await page.getByRole('dialog').waitFor();
  await noOverflow('Mobile import dialog');
  await page.screenshot({path:'src/web/tests/artifacts/import-mobile.png',fullPage:true});
  await page.keyboard.press('Escape');
  assert.equal(await page.getByRole('dialog').count(),0,'escape closes modal');
  await page.setViewportSize({width:1440,height:1000});
  await clickNav('运行与设置');
  await page.getByRole('button',{name:'检测与端口',exact:true}).click();
  await page.getByLabel('并发数量',{exact:true}).fill('12');
  await page.getByRole('button',{name:'保存设置',exact:true}).click();
  await textVisible('设置已同步');
  assert.equal(state.settings.testConcurrency,12);
  await page.getByRole('button',{name:'备份与安全',exact:true}).click();
  await page.getByRole('button',{name:'导出备份',exact:true}).click();
  await page.getByRole('dialog',{name:'导出敏感配置备份？'}).waitFor();
  await page.getByRole('button',{name:'取消',exact:true}).click();
  assert.equal(calls.filter(call=>call.path==='/api/export/backup').length,0,'cancel prevents sensitive download');
  // All four strategies retain the documented hash-key constraints.
  await clickNav('代理池');
  await page.getByRole('button',{name:'编辑代理池 用户名哈希池',exact:true}).click();
  for(const strategy of ['round-robin','url-test','sticky-sessions','consistent-hashing']) {
    await page.getByLabel('调度策略',{exact:true}).selectOption(strategy);
    assert.equal(await page.getByLabel('哈希键',{exact:true}).count(),['sticky-sessions','consistent-hashing'].includes(strategy)?1:0);
  }
  await closeDialog();
  // New username must not reuse another user's secret; HTTP forces UDP off.
  await clickNav('监听端口');
  await page.getByRole('button',{name:'编辑监听 保留密码监听',exact:true}).click();
  await page.getByLabel('用户名 1',{exact:true}).fill('bob');
  assert.equal(await page.getByLabel('密码 1',{exact:true}).getAttribute('required'),'');
  const beforeListenerCalls=calls.filter(call=>call.path.startsWith('/api/listeners/')).length;
  await page.getByRole('button',{name:'保存监听',exact:true}).click();
  assert.equal(calls.filter(call=>call.path.startsWith('/api/listeners/')).length,beforeListenerCalls,'blank password cannot create new username');
  await page.getByLabel('用户名 1',{exact:true}).fill('alice');
  await page.getByLabel('监听地址',{exact:true}).fill('0.0.0.0');
  await page.getByRole('switch',{name:'UDP 转发'}).click();
  await page.getByRole('button',{name:'保存监听',exact:true}).click();
  await textVisible('Mihomo 原生 UDP 不受用户名密码保护，UDP 仅允许回环地址。');
  assert.equal(calls.filter(call=>call.path.startsWith('/api/listeners/')).length,beforeListenerCalls,'public UDP blocked client-side');
  await page.getByLabel('监听协议',{exact:true}).selectOption('http');
  assert.equal(await page.getByRole('switch',{name:'UDP 转发'}).getAttribute('aria-checked'),'false');
  assert.equal(await page.getByRole('switch',{name:'UDP 转发'}).isDisabled(),true);
  await closeDialog();
  // File import and subscription modes send raw content only to the API.
  await clickNav('节点管理');
  await page.getByRole('button',{name:'导入节点',exact:true}).first().click();
  await page.getByRole('button',{name:'本地文件',exact:true}).click();
  await page.getByLabel('选择节点配置文件',{exact:true}).setInputFiles({name:'test-nodes.txt',mimeType:'text/plain',buffer:Buffer.from('socks5://127.0.0.1:1080')});
  await page.getByRole('button',{name:'预览解析',exact:true}).click();
  await page.getByRole('heading',{name:/导入预览/}).waitFor();
  assert.equal(calls.at(-1).body.mode,'file');
  await page.getByRole('button',{name:'订阅链接',exact:true}).click();
  await page.getByLabel('订阅 URL',{exact:true}).fill('https://example.com/subscription?token=test-only');
  await page.getByRole('button',{name:'预览解析',exact:true}).click();
  await page.getByRole('heading',{name:/导入预览/}).waitFor();
  assert.equal(calls.at(-1).body.mode,'subscriptions');
  await closeDialog();
  await verifyRelayImportUi({page,mocks:importMocks,calls,noOverflow});
  await verifyNodeGroupUi({page,state,now,noOverflow});
  await verifyPoolCards({page,state,now,calls,noOverflow});
  // Active jobs and force-refresh are explicit and cancellable.
  state.jobs=[{id:'job-1',kind:'latency',status:'running',total:1,completed:0,results:[],createdAt:now,finishedAt:''}];
  state.subscriptions=[{id:'sub-1',name:'回归订阅',url:'https://example.com/subscription',groupIds:[],enabled:true,intervalMinutes:60,lastFetchedAt:now,lastError:'',nodeCount:1,createdAt:now,updatedAt:now}];
  await Promise.all([page.waitForResponse(response=>new URL(response.url()).pathname==='/api/bootstrap'),page.getByRole('button',{name:'立即同步数据',exact:true}).click()]);
  await clickNav('运行与设置');
  await page.getByRole('button',{name:/后台任务/}).click();
  await page.getByRole('button',{name:'取消任务',exact:true}).click();
  await textVisible('已取消');
  await clickNav('订阅管理');
  await page.getByRole('button',{name:'编辑订阅 回归订阅',exact:true}).click();
  await page.getByLabel('订阅名称',{exact:true}).fill('已改名订阅');
  await page.getByRole('button',{name:'保存订阅',exact:true}).click();
  await page.getByRole('button',{name:'强制刷新订阅 已改名订阅',exact:true}).click();
  assert.equal(calls.filter(call=>call.path.endsWith('/refresh')).length,0,'force refresh awaits confirmation');
  await page.getByRole('button',{name:'确认强制刷新',exact:true}).click();
  await page.getByRole('dialog').waitFor({state:'hidden'});
  assert.equal(calls.find(call=>call.path.endsWith('/refresh')).body.force,true);
  // Restore preview reveals counts, not secrets; cancel is non-mutating.
  await clickNav('运行与设置');
  await page.getByRole('button',{name:'备份与安全',exact:true}).click();
  await page.getByLabel('选择完整配置备份',{exact:true}).setInputFiles({name:'backup.json',mimeType:'application/json',buffer:Buffer.from(JSON.stringify({schemaVersion:1,state:{nodes:[],groups:[],pools:[],listeners:[],subscriptions:[],settings:state.settings,revision:1}}))});
  await page.getByRole('button',{name:'恢复这份备份',exact:true}).click();
  await page.getByRole('dialog',{name:'覆盖当前全部配置？'}).waitFor();
  await page.getByRole('button',{name:'取消',exact:true}).click();
  assert.equal(calls.filter(call=>call.path==='/api/import/backup').length,0);
  // Failed polling retains the last confirmed state, with an explicit stale warning.
  offline=true;
  await page.getByRole('button',{name:'立即同步数据',exact:true}).click();
  await textVisible('与管理服务的连接已中断');
  offline=false;
  await page.getByRole('button',{name:'重试连接',exact:true}).click();
  await page.getByText('与管理服务的连接已中断',{exact:true}).waitFor({state:'hidden'});
  rejectAuth=true;
  await page.getByRole('button',{name:'立即同步数据',exact:true}).click();
  await textVisible('验证管理访问');
  assert.equal(await page.getByRole('navigation').count(),0,'401 clears authenticated workspace view');
  rejectAuth=false;
  await page.getByLabel('管理令牌',{exact:true}).fill(token);
  await page.getByRole('button',{name:'验证并进入',exact:true}).click();
  await page.getByRole('navigation').waitFor();
  console.log('PASS all strategy controls, new-password protection, non-loopback UDP guard, HTTP UDP-off, local-file/subscription previews, job cancel, confirmed force refresh, restore cancellation, offline stale state, expired token lock');
  assert.equal(errors.length,0,'No browser runtime errors: ' + errors.join('; '));
  console.log('PASS responsive 375/768/1024 layouts, Escape dialog, settings save and backup confirmation');
  console.log('PASS zero uncaught browser errors; screenshots src/web/tests/artifacts/ (test fixtures, not live proxy evidence)');
} catch(error) {console.error('BROWSER ERRORS',errors);await page.screenshot({path:'src/web/tests/artifacts/failure.png',fullPage:true});throw error;} finally {await context.close();await browser.close();}

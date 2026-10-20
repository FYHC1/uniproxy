/** Synthetic UI/API-contract cases only. Every request is intercepted; no server
 * or installer runs and no release package is ever downloaded by these tests. */
import assert from 'node:assert/strict';
import { expect } from '@playwright/test';

const installation = patch => ({state:'idle',stage:'',version:'1.19.32',os:'windows',arch:'amd64',path:'C:/fixture/core/mihomo.exe',installed:false,supported:true,canInstall:true,reason:'',bytesDownloaded:0,totalBytes:0,error:'',startedAt:'',finishedAt:'',...patch});
const initial = () => ({
  nodes:[],groups:[],pools:[],listeners:[],subscriptions:[],revision:7,activities:[],jobs:[],
  settings:{testUrl:'https://example.com/check',speedTestUrl:'https://example.com/download',testTimeoutMs:8000,testConcurrency:8,speedTestBytes:10485760,healthIntervalSeconds:300,portRangeStart:20000,portRangeEnd:20999},
  runtime:{available:false,running:false,applying:false,version:'',appliedRevision:0,lastError:'fixture: core is missing',startedAt:'',dirty:true,listeners:[]},
  coreInstallation:installation(),
});
const deferred = () => {let resolve;const promise=new Promise(done=>{resolve=done;});return {promise,resolve};};
const dialogFor = page => page.getByRole('dialog',{name:'安装 Mihomo 内核',exact:true});
const panelFor = page => page.getByRole('region',{name:'Mihomo 内核安装',exact:true});
const callsFor = (fixture,path) => fixture.calls.filter(call=>call.path===path);
async function harness({browser,files},patch={}) {
  const context=await browser.newContext({viewport:{width:1440,height:1000},reducedMotion:'reduce'});
  const page=await context.newPage();
  const h={context,page,state:{...initial(),...patch},calls:[],errors:[],external:[],auth:false,token:'core-fixture-token',offline:false,installReply:null,inspectReply:null,bootstrapReply:null};
  page.on('pageerror',error=>h.errors.push(error.message));
  await context.route('**/*',async route=>{
    const request=route.request();const url=new URL(request.url());const path=url.pathname;
    if(url.origin!=='http://uniproxy-core.test'){h.external.push(request.url());await route.abort('blockedbyclient');return;}
    if(!path.startsWith('/api/')){
      const key=path==='/'?'/index.html':path;
      await route.fulfill({status:files.has(key)?200:404,contentType:key.endsWith('.js')?'application/javascript':key.endsWith('.css')?'text/css':'text/html',body:files.get(key)||''});return;
    }
    const call={path,method:request.method(),body:request.postDataJSON()};h.calls.push(call);
    if(h.auth && request.headers().authorization!=='Bearer '+h.token){await route.fulfill({status:401,json:{error:'fixture: 请验证管理令牌'}});return;}
    if(path==='/api/bootstrap'){
      if(h.offline){await route.abort('failed');return;}
      const reply=h.bootstrapReply?await h.bootstrapReply(structuredClone(h.state),request):{json:structuredClone(h.state)};
      await route.fulfill(reply);return;
    }
    if(path==='/api/core/installation'){await route.fulfill({json:h.state.coreInstallation});return;}
    if(path==='/api/core/install'){
      assert.equal(call.method,'POST');assert.deepEqual(call.body,{});
      let reply;
      if(h.installReply)reply=await h.installReply();
      else {h.state.coreInstallation={...h.state.coreInstallation,state:'installing',stage:'metadata',canInstall:false,reason:'安装进行中',error:'',startedAt:'2026-01-01T00:00:00Z'};reply={status:202,json:h.state.coreInstallation};}
      if(reply.abort)await route.abort('failed');else await route.fulfill(reply);return;
    }
    if(path==='/api/core/inspect'){
      assert.equal(call.method,'POST');assert.deepEqual(call.body,{});
      const reply=h.inspectReply?await h.inspectReply():{json:{coreInstallation:h.state.coreInstallation,runtime:h.state.runtime}};
      if(reply.abort)await route.abort('failed');else await route.fulfill(reply);return;
    }
    if(path==='/api/runtime/apply'){
      h.state.runtime={...h.state.runtime,available:true,running:true,dirty:false,lastError:'',appliedRevision:h.state.revision};
      await route.fulfill({json:h.state.runtime});return;
    }
    assert.fail('Unexpected core fixture request: '+path);
  });
  h.sync=async()=>{
    const response=page.waitForResponse(res=>new URL(res.url()).pathname==='/api/bootstrap');
    await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));
    await response;
  };
  h.open=async()=>{await page.goto('http://uniproxy-core.test/');};
  h.close=async()=>{await context.unrouteAll({behavior:'wait'});await context.close();assert.deepEqual(h.errors,[],'no uncaught core UI errors');assert.deepEqual(h.external,[],'all core UI requests stay offline');};
  return h;
}
async function noOverflow(page,container) {
  await page.waitForFunction(()=>document.documentElement.scrollWidth<=innerWidth+1,null,{timeout:2500,polling:'raf'});
  const bounds=await container.evaluate(element=>({left:element.getBoundingClientRect().left,right:element.getBoundingClientRect().right,width:element.clientWidth,scroll:element.scrollWidth,view:innerWidth}));
  assert(bounds.left>=-1 && bounds.right<=bounds.view+1 && bounds.scroll<=bounds.width+1,'installer stays inside viewport without internal horizontal overflow');
}
async function operations(page) {await page.getByRole('link',{name:'运行与设置',exact:true}).click();await expect(panelFor(page)).toBeVisible();}
async function authenticate(page,token) {await page.getByLabel('管理令牌',{exact:true}).fill(token);await page.getByRole('button',{name:'验证并进入',exact:true}).click();}

export async function verifyCoreInstallUi(options) {
  {
    const h=await harness(options);h.auth=true;const {page}=h;
    try {
      await h.open();await expect(page.getByRole('heading',{name:'验证管理访问',exact:true})).toBeVisible();await expect(dialogFor(page)).toHaveCount(0);
      await authenticate(page,'wrong-token');await expect(page.getByRole('heading',{name:'验证管理访问',exact:true})).toBeVisible();await expect(dialogFor(page)).toHaveCount(0);
      await authenticate(page,h.token);const dialog=dialogFor(page);await expect(dialog).toBeVisible();
      await expect(dialog).toContainText(h.state.coreInstallation.path);await expect(dialog).toContainText('1.19.32');await expect(dialog).toContainText('不需要独立安装器');
      for(let i=0;i<8;i++){await page.keyboard.press('Tab');assert(await dialog.evaluate(element=>element.contains(document.activeElement)),'Radix traps keyboard focus');}
      await page.screenshot({path:'src/web/tests/artifacts/core-install-prompt-desktop.png',fullPage:true});
      await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await h.sync();await expect(dialog).toHaveCount(0);
      await operations(page);await expect(panelFor(page).getByRole('button',{name:'在线安装 Mihomo',exact:true})).toBeEnabled();
      await page.getByRole('button',{name:'检测与端口',exact:true}).click();await expect(panelFor(page)).toBeVisible();
      await panelFor(page).getByRole('button',{name:'安装详情',exact:true}).focus();await page.keyboard.press('Enter');await expect(dialog).toBeVisible();await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await expect(panelFor(page).getByRole('button',{name:'安装详情',exact:true})).toBeFocused();
      await page.reload();await expect(page.getByRole('heading',{name:'运行与设置',level:1,exact:true})).toBeVisible();await expect(dialog).toHaveCount(0);
      h.state.coreInstallation.path='C:/fixture/changed-destination/'+('long-directory/'.repeat(10))+'mihomo.exe';await h.sync();await expect(dialog).toBeVisible();
      for(const width of [375,390,768]){await page.setViewportSize({width,height:844});await noOverflow(page,dialog);}
      await page.setViewportSize({width:375,height:844});await page.screenshot({path:'src/web/tests/artifacts/core-install-prompt-mobile.png',fullPage:true});
      await dialog.getByRole('button',{name:'稍后处理',exact:true}).click();await expect(dialog).toHaveCount(0);
      assert.equal(callsFor(h,'/api/core/install').length,0);assert.equal(callsFor(h,'/api/core/inspect').length,0,'normal startup/poll never performs a heavyweight inspect');
      console.log('PASS core prompt authenticated-only, once per tab/path, keyboard trap/Escape/focus return, runtime/settings entries and mobile path overflow');
    } finally {await h.close();}
  }
  {
    const h=await harness(options);const {page}=h;const gate=deferred();
    try {
      await h.open();const dialog=dialogFor(page);await expect(dialog).toBeVisible();
      h.installReply=async()=>{h.state.coreInstallation={...h.state.coreInstallation,state:'installing',stage:'metadata',canInstall:false,reason:'安装进行中'};const reply={status:202,json:structuredClone(h.state.coreInstallation)};await gate.promise;return reply;};
      await dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true}).evaluate(button=>{button.click();button.click();});
      await expect(dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true})).toBeDisabled();
      await expect.poll(()=>callsFor(h,'/api/core/install').length).toBe(1);
      await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);await operations(page);
      await expect(panelFor(page).getByRole('button',{name:'在线安装 Mihomo',exact:true})).toBeDisabled();await expect(panelFor(page).getByRole('button',{name:'重新检测内核',exact:true})).toBeDisabled();await expect(page.getByRole('button',{name:'重新应用配置',exact:true})).toBeDisabled();
      await panelFor(page).getByRole('button',{name:'安装详情',exact:true}).click();await expect(dialog).toBeVisible();await expect(dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true})).toBeDisabled();
      gate.resolve();await expect(dialog.getByText('获取发布信息',{exact:true})).toBeVisible();await expect(dialog.getByRole('progressbar')).toHaveCount(0);await expect(dialog).not.toContainText('0%');
      await dialog.getByRole('button',{name:'后台继续安装',exact:true}).click();await page.getByRole('link',{name:'分组管理',exact:true}).click();
      await expect(page.getByRole('region',{name:'内核安装状态',exact:true})).toContainText('获取发布信息');await expect(dialog).toHaveCount(0);
      h.state.coreInstallation={...h.state.coreInstallation,stage:'downloading',bytesDownloaded:1048576,totalBytes:4194304};await h.sync();
      await page.getByRole('region',{name:'内核安装状态',exact:true}).getByRole('button',{name:'查看安装进度',exact:true}).click();await expect(dialog.getByRole('progressbar',{name:'安装包下载进度',exact:true})).toHaveAttribute('value','25');await expect(dialog).toContainText('1.0 MiB / 4.0 MiB · 25%');
      h.state.coreInstallation.totalBytes=0;await h.sync();await expect(dialog.getByRole('progressbar')).toHaveCount(0);await expect(dialog).toContainText('总大小未知');await expect(dialog).not.toContainText('NaN');
      h.state.coreInstallation.totalBytes=4194304;h.state.coreInstallation.bytesDownloaded=4194304;
      for(const [stage,label] of [['verifying','校验 SHA256'],['extracting','解压安装包'],['installing','写入内核文件'],['inspecting','检查内核版本']]){h.state.coreInstallation.stage=stage;await h.sync();await expect(dialog.getByText(label,{exact:true})).toBeVisible();await expect(dialog.getByRole('progressbar')).toHaveCount(0);await expect(dialog).not.toContainText('100%');}
      h.state.coreInstallation={...h.state.coreInstallation,state:'succeeded',stage:'complete',installed:true,canInstall:false,reason:'已安装',finishedAt:'2026-01-01T00:01:00Z'};h.state.runtime={...h.state.runtime,available:true,running:false,dirty:true,version:'Mihomo v1.19.32',lastError:''};await h.sync();
      await expect(dialog).toContainText('安装完成，版本校验通过');await expect(dialog).toContainText('代理尚未启动');await expect(dialog).toContainText('重新应用配置');await expect(dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true})).toHaveCount(0);
      assert.equal(callsFor(h,'/api/core/install').length,1,'duplicate surfaces share one in-flight request');assert.equal(callsFor(h,'/api/runtime/apply').length,0,'install completion never auto-applies');assert.equal(h.state.revision,7);
      await dialog.getByRole('button',{name:'稍后处理',exact:true}).click();await operations(page);await expect(page.getByRole('button',{name:'重新应用配置',exact:true})).toBeEnabled();
      await page.screenshot({path:'src/web/tests/artifacts/core-install-success.png',fullPage:true});
      await page.getByRole('button',{name:'重新应用配置',exact:true}).click();await expect.poll(()=>callsFor(h,'/api/runtime/apply').length).toBe(1);await expect(panelFor(page)).toContainText('当前代理状态以运行概况为准');
      console.log('PASS core shared lock, dismiss while submitting/downloading, navigation progress, real byte percentage only, stage transitions and explicit apply after success');
    } finally {gate.resolve();await h.close();}
  }
  {
    const h=await harness(options,{coreInstallation:installation({state:'installing',stage:'downloading',canInstall:false,bytesDownloaded:2048,totalBytes:4096})});const {page}=h;
    try {
      await h.open();await expect(page.getByRole('heading',{name:'节点管理',level:1,exact:true})).toBeVisible();await expect(dialogFor(page)).toHaveCount(0);await expect(page.getByRole('region',{name:'内核安装状态',exact:true})).toContainText('下载安装包');
      h.state.coreInstallation={...h.state.coreInstallation,state:'failed',canInstall:true,error:'fixture: SHA256 校验失败',reason:''};await h.sync();await expect(dialogFor(page),'known active job must not later auto-open a prompt').toHaveCount(0);
      await page.getByRole('button',{name:'管理内核安装',exact:true}).click();const dialog=dialogFor(page);await expect(dialog.getByRole('alert')).toContainText('SHA256 校验失败');await expect(dialog.getByRole('button',{name:'重试安装',exact:true})).toBeEnabled();
      await dialog.getByRole('button',{name:'重试安装',exact:true}).click();await expect(dialog.getByText('获取发布信息',{exact:true})).toBeVisible();assert.equal(callsFor(h,'/api/core/install').length,1);
      h.state.coreInstallation={...h.state.coreInstallation,state:'failed',canInstall:false,installed:true,error:'fixture: 检查版本失败',reason:'目标已存在不兼容文件；请手动修正后重新检测，不能覆盖。'};await h.sync();await expect(dialog.getByRole('button',{name:'重试安装',exact:true})).toHaveCount(0);await expect(dialog).toContainText('不能覆盖');
      await expect(dialog.getByRole('button',{name:'重新检测内核',exact:true})).toBeEnabled();await page.setViewportSize({width:375,height:844});await noOverflow(page,dialog);await page.screenshot({path:'src/web/tests/artifacts/core-install-failure-mobile.png',fullPage:true});
      console.log('PASS known active installation skips auto prompt, failure remains visible, retry only when allowed, incompatible file protection and mobile errors');
    } finally {await h.close();}
  }
  {
    const h=await harness(options);const {page}=h;
    try {
      await h.open();const dialog=dialogFor(page);await expect(dialog).toBeVisible();
      h.installReply=async()=>({status:409,json:{error:'fixture: 另一个操作正在进行',code:'busy'}});
      await dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true}).click();await expect(dialog.getByRole('alert')).toContainText('另一个操作正在进行');await expect(dialog.getByRole('progressbar')).toHaveCount(0);await expect(dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true})).toBeEnabled();assert.equal(callsFor(h,'/api/core/install').length,1,'409 is not auto-retried');
      h.installReply=async()=>{h.offline=true;return {abort:true};};
      await dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true}).click();await expect(dialog.getByRole('alert')).toContainText('无法连接管理服务');await expect(page.locator('.global-alert--error')).toContainText('当前显示最近一次成功同步的数据');
      await page.keyboard.press('Escape');await expect(dialog).toHaveCount(0);h.offline=false;await h.sync();await page.getByRole('button',{name:'管理内核安装',exact:true}).click();await expect(dialog.getByRole('alert')).toContainText('无法连接管理服务');
      h.installReply=async()=>({status:401,json:{error:'fixture: 安装请求的管理令牌已过期'}});
      await dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true}).click();await expect(page.getByRole('heading',{name:'验证管理访问',exact:true})).toBeVisible();await expect(dialog).toHaveCount(0);await expect(page.getByRole('navigation')).toHaveCount(0);
      await authenticate(page,h.token);await expect(page.getByRole('navigation')).toBeVisible();await expect(dialog).toHaveCount(0);await page.getByRole('button',{name:'管理内核安装',exact:true}).click();
      h.inspectReply=async()=>({status:401,json:{error:'fixture: 检测请求的管理令牌已过期'}});
      await dialog.getByRole('button',{name:'重新检测内核',exact:true}).click();await expect(page.getByRole('heading',{name:'验证管理访问',exact:true})).toBeVisible();await expect(dialog).toHaveCount(0);
      console.log('PASS core 409/offline errors persist without fabricated progress or automatic retries; install and inspect 401 lock and clear the workspace');
    } finally {await h.close();}
  }
  {
    const h=await harness(options,{coreInstallation:installation({installed:true,canInstall:false,reason:'已安装，无需重复安装'}),runtime:{...initial().runtime,available:true,version:'Mihomo v1.19.32',lastError:''}});const {page}=h;
    try {
      await h.open();await expect(page.getByRole('heading',{name:'节点管理',level:1,exact:true})).toBeVisible();await expect(dialogFor(page)).toHaveCount(0);await operations(page);await expect(panelFor(page)).toContainText('无需重复安装');await expect(panelFor(page).getByRole('button',{name:'在线安装 Mihomo',exact:true})).toHaveCount(0);
      h.state.runtime.available=false;h.state.coreInstallation={...h.state.coreInstallation,state:'failed',stage:'verifying',error:'旧安装任务校验失败',reason:'目标已有不兼容版本，请手动修正路径或版本后重新检测，不会覆盖。'};await h.sync();await expect(dialogFor(page)).toHaveCount(0);await expect(panelFor(page)).toContainText('不会覆盖');
      h.inspectReply=async()=>{h.state.runtime={...h.state.runtime,available:true,running:false,dirty:true,version:'Mihomo v1.19.32'};h.state.coreInstallation.reason='已安装';return {json:{coreInstallation:h.state.coreInstallation,runtime:h.state.runtime}};};
      await panelFor(page).getByRole('button',{name:'重新检测内核',exact:true}).click();await expect(panelFor(page)).toContainText('无需重复安装');await expect(page.getByRole('button',{name:'重新应用配置',exact:true})).toBeEnabled();assert.equal(callsFor(h,'/api/core/inspect').length,1);assert.equal(callsFor(h,'/api/core/install').length,0);assert.equal(callsFor(h,'/api/runtime/apply').length,0);assert.equal(h.state.revision,7);
      h.state.runtime.available=false;h.state.coreInstallation=installation({path:'',os:'freebsd',arch:'riscv64',supported:false,canInstall:false,reason:'当前平台不支持在线安装，请手动配置兼容内核后重新检测。'});await h.sync();const dialog=dialogFor(page);await expect(dialog).toBeVisible();await expect(dialog).toContainText('当前平台不支持在线安装');await expect(dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true})).toHaveCount(0);
      console.log('PASS detected core avoids reinstall, incompatible/unsupported destinations explain manual correction, explicit inspect discovers a manual install without applying');
    } finally {await h.close();}
  }
  for(const staleStatus of [202,401]) {
    const h=await harness(options);const {page}=h;const gate=deferred();const delivered=deferred();
    try {
      // Ignore transport cancellation to exercise the session/response guards
      // even if an old response finishes parsing after authentication changes.
      await page.addInitScript(()=>{const original=window.fetch;window.fetch=(input,init)=>original(input,{...init,signal:undefined});});
      h.installReply=async()=>{const reply=staleStatus===401?{status:401,json:{error:'obsolete unauthorized response'}}:{status:202,json:installation({state:'succeeded',stage:'complete',installed:true,canInstall:false})};await gate.promise;delivered.resolve();return reply;};
      await h.open();const dialog=dialogFor(page);await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true}).click();await expect.poll(()=>callsFor(h,'/api/core/install').length).toBe(1);
      await page.keyboard.press('Escape');await page.getByRole('button',{name:'输入管理令牌',exact:true}).click();await expect(page.getByRole('heading',{name:'验证管理访问',exact:true})).toBeVisible();
      h.state.coreInstallation=installation({path:'C:/fixture/new-session/mihomo.exe',installed:true,canInstall:false});h.state.runtime={...h.state.runtime,available:true,version:'Mihomo v1.19.32',lastError:''};
      await authenticate(page,h.token);await expect(page.getByRole('navigation')).toBeVisible();
      const oldResponse=page.waitForResponse(response=>new URL(response.url()).pathname==='/api/core/install');gate.resolve();await delivered.promise;await oldResponse;await operations(page);await h.sync();
      await expect(page.getByRole('heading',{name:'验证管理访问',exact:true})).toHaveCount(0);await expect(panelFor(page)).toContainText('C:/fixture/new-session/mihomo.exe');await expect(panelFor(page)).not.toContainText('安装完成，版本校验通过');await expect(dialog).toHaveCount(0);await expect(panelFor(page).getByRole('button',{name:'重新检测内核',exact:true})).toBeEnabled();
      console.log('PASS obsolete install '+staleStatus+' response cannot mutate or lock a newer authenticated session');
    } finally {gate.resolve();await h.close();}
  }
  {
    const h=await harness(options);const {page}=h;const gate=deferred();const started=deferred();
    try {
      await page.addInitScript(()=>{const original=window.fetch;window.fetch=(input,init)=>original(input,{...init,signal:undefined});});
      await h.open();const dialog=dialogFor(page);await expect(dialog).toBeVisible();await dialog.getByRole('button',{name:'稍后处理',exact:true}).click();
      let oldRequest;
      h.bootstrapReply=async (snapshot,request)=>{h.bootstrapReply=null;oldRequest=request;started.resolve();await gate.promise;return {json:snapshot};};
      await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));await started.promise;
      await page.getByRole('button',{name:'管理内核安装',exact:true}).click();await dialog.getByRole('button',{name:'在线安装 Mihomo',exact:true}).click();await expect(dialog.getByText('获取发布信息',{exact:true})).toBeVisible();
      h.state.coreInstallation={...h.state.coreInstallation,stage:'downloading',bytesDownloaded:3,totalBytes:4};await h.sync();await expect(dialog.getByRole('progressbar')).toHaveAttribute('value','75');
      const oldResponse=page.waitForResponse(response=>response.request()===oldRequest);gate.resolve();await oldResponse;await page.keyboard.press('Escape');await operations(page);await expect(panelFor(page).getByRole('progressbar')).toHaveAttribute('value','75');await expect(panelFor(page).getByRole('button',{name:'在线安装 Mihomo',exact:true})).toHaveCount(0);
      assert.equal(callsFor(h,'/api/core/install').length,1);console.log('PASS stale pre-install bootstrap cannot overwrite newer installation progress in the same session');
    } finally {gate.resolve();await h.close();}
  }
  {
    const h=await harness(options);const {page}=h;const gate=deferred();
    try {
      await h.open();const dialog=dialogFor(page);await expect(dialog).toBeVisible();
      h.inspectReply=async()=>{await gate.promise;return {json:{coreInstallation:h.state.coreInstallation,runtime:h.state.runtime}};};
      await dialog.getByRole('button',{name:'重新检测内核',exact:true}).evaluate(button=>{button.click();button.click();});await expect.poll(()=>callsFor(h,'/api/core/inspect').length).toBe(1);
      await page.keyboard.press('Escape');await operations(page);await expect(panelFor(page).getByRole('button',{name:'重新检测内核',exact:true})).toBeDisabled();await expect(panelFor(page).getByRole('button',{name:'在线安装 Mihomo',exact:true})).toBeDisabled();await panelFor(page).getByRole('button',{name:'安装详情',exact:true}).click();await expect(dialog.getByRole('button',{name:'重新检测内核',exact:true})).toBeDisabled();
      gate.resolve();await expect(dialog.getByRole('button',{name:'重新检测内核',exact:true})).toBeEnabled();assert.equal(callsFor(h,'/api/core/inspect').length,1);assert.equal(callsFor(h,'/api/core/install').length,0);
      console.log('PASS inspect uses the same shared lock and remains dismissible without duplicate probes');
    } finally {gate.resolve();await h.close();}
  }
  {
    const h=await harness(options);delete h.state.coreInstallation;
    try {await h.open();await expect(h.page.getByText('管理 API 数据格式不匹配，请检查前后端版本。',{exact:true})).toBeVisible();await expect(dialogFor(h.page)).toHaveCount(0);assert.equal(callsFor(h,'/api/core/install').length,0);console.log('PASS required coreInstallation contract rejects mismatched bootstrap safely');}
    finally {await h.close();}
  }
}

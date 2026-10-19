/** Synthetic browser-only fixtures. No real configuration, endpoints or credentials. */
import assert from 'node:assert/strict';
import { expect } from '@playwright/test';

const fileText = 'socks5://192.0.2.1:1080#synthetic';
const fixturePreview = {
  nodes:[
    {name:'合成中转',protocol:'socks5',server:'192.0.2.1',port:1080,udp:true},
    {name:'合成出口',protocol:'http',server:'192.0.2.2',port:8080,udp:false,dialerProxy:'合成中转',config:{password:'synthetic-secret-not-for-display'}},
  ],errors:[],totalErrors:0,errorsTruncated:false,
};
function deferred() {let resolve; const promise=new Promise(done=>{resolve=done;});return {promise,resolve};}
async function frame(page) {await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));}
async function dataTransfer(page,files) {
  return page.evaluateHandle(files=>{
    const transfer=new DataTransfer();
    for(const item of files)transfer.items.add(new File([item.bytes ? 'x'.repeat(item.bytes) : item.text ?? 'socks5://192.0.2.1:1080#synthetic'],item.name,{type:'text/plain'}));
    return transfer;
  },files);
}
async function drag(target,type,data) {
  return target.evaluate((element,{type,data})=>{
    const event=new DragEvent(type,{dataTransfer:data,bubbles:true,cancelable:true});
    element.dispatchEvent(event);
    return {prevented:event.defaultPrevented,dropEffect:data.dropEffect};
  },{type,data});
}
async function dropFiles(page,files,target=page.locator('.file-drop')) {
  const data=await dataTransfer(page,files);
  try {return await drag(target,'drop',data);} finally {await data.dispose();}
}
async function settleRead(page,name,reject=false) {
  await page.evaluate(({name,reject})=>{
    const pending=window.__syntheticReads.get(name);
    if(!pending)throw Error('Missing synthetic read '+name);
    window.__syntheticReads.delete(name);
    if(reject)pending.reject(new Error('obsolete read failure'));else pending.resolve('socks5://192.0.2.55:1080#obsolete');
  },{name,reject});
  await frame(page);
}
export async function verifyRelayImportUi({page,mocks,calls,noOverflow}) {
  mocks.preview=async()=>({json:fixturePreview});
  const open=async()=>{await page.getByRole('button',{name:'导入节点',exact:true}).first().click();await page.getByRole('button',{name:'本地文件',exact:true}).click();};
  const close=()=>page.getByRole('button',{name:'关闭对话框'}).click();
  const zone=page.getByRole('group',{name:'本地文件导入',exact:true});
  const previewButton=page.getByRole('button',{name:'预览解析',exact:true});
  const confirm=page.getByRole('button',{name:'确认导入',exact:true});
  const preview=page.locator('.preview-section');
  const parse=async()=>{await previewButton.click();await expect(preview).toBeVisible();};
  const fileInput=page.getByLabel('选择节点配置文件',{exact:true});
  const initialUrl=page.url();
  const beforeCommits=calls.filter(call=>call.path==='/api/import'&&!call.body.preview).length;
  await open();
  // The browse alternative remains a real, keyboard-accessible button.
  await frame(page);
  const browse=page.getByRole('button',{name:'浏览文件',exact:true});
  await browse.focus();await expect(browse).toBeFocused();
  const chooserEvent=page.waitForEvent('filechooser');
  await browse.press('Enter');
  const chooser=await chooserEvent;
  await chooser.setFiles({name:'keyboard.yaml',mimeType:'text/plain',buffer:Buffer.from(fileText)});
  await expect(zone.locator('strong')).toHaveText('keyboard.yaml');
  assert.equal(await fileInput.inputValue(),'','same-file retry remains possible');
  await parse();
  await expect(preview.getByText('经由：合成中转',{exact:true})).toBeVisible();
  await expect(preview.getByText(/包含 1 个中转节点/)).toBeVisible();
  assert(!(await preview.innerText()).includes('synthetic-secret-not-for-display'),'preview never renders raw config');
  await fileInput.setInputFiles([]);
  await expect(confirm).toBeEnabled();
  await expect(preview).toBeVisible();

  const valid=await dataTransfer(page,[{name:'drop.yaml',text:fileText}]);
  try {
    assert.equal((await drag(zone,'dragenter',valid)).prevented,true);
    await expect(zone).toHaveClass(/file-drop--valid/);
    const child=zone.getByRole('button');
    await drag(child,'dragenter',valid);
    await drag(zone,'dragleave',valid);
    await expect(zone).toHaveClass(/file-drop--valid/);
    await drag(child,'dragleave',valid);
    await expect(zone).not.toHaveClass(/file-drop--valid/);
    await drag(zone,'dragenter',valid);
    assert.equal((await drag(zone,'drop',valid)).prevented,true);
  } finally {await valid.dispose();}
  await expect(zone.locator('strong')).toHaveText('drop.yaml');
  await expect(preview).toHaveCount(0);
  await expect(confirm).toBeDisabled();
  await parse();
  const lastFilePreview=calls.filter(call=>call.path==='/api/import'&&call.body.preview).at(-1).body;
  assert.equal(lastFilePreview.mode,'file'); assert.equal(lastFilePreview.content,fileText);
  const outside=await dataTransfer(page,[{name:'outside.yaml'}]);
  try {
    assert.equal((await drag(page.locator('body'),'dragover',outside)).prevented,true);
    assert.equal((await drag(page.locator('body'),'drop',outside)).prevented,true);
  } finally {await outside.dispose();}
  assert.equal(page.url(),initialUrl,'file drop never navigates');
  await expect(zone.locator('strong')).toHaveText('drop.yaml');

  // Reject multiple files explicitly rather than silently reading the first file.
  const multi=await dataTransfer(page,[{name:'one.yaml'},{name:'two.yaml'}]);
  try {
    // Synthetic DataTransfer.dropEffect is browser-controlled; default cancellation and feedback are observable.
    const hover=await drag(zone,'dragenter',multi);assert.equal(hover.prevented,true);await expect(zone).toHaveClass(/file-drop--invalid/);
    await drag(zone,'drop',multi);
  } finally {await multi.dispose();}
  await expect(page.getByRole('alert')).toContainText('一次只能导入一个文件');
  await expect(confirm).toBeDisabled(); await expect(preview).toHaveCount(0); await expect(previewButton).toBeDisabled();
  // Directory entries and the File System Access fallback are both synthetic.
  for(const method of ['entry','handle']) {
    await zone.evaluate((element,method)=>{
      const data=new DataTransfer();data.items.add(new File([''],'synthetic-folder'));
      const proto=DataTransferItem.prototype;
      const originalEntry=proto.webkitGetAsEntry;
      const originalHandle=proto.getAsFileSystemHandle;
      proto.webkitGetAsEntry=()=>method==='entry'?{isDirectory:true}:null;
      proto.getAsFileSystemHandle=async()=>({kind:'directory'});
      try {element.dispatchEvent(new DragEvent('drop',{dataTransfer:data,bubbles:true,cancelable:true}));}
      finally {proto.webkitGetAsEntry=originalEntry;proto.getAsFileSystemHandle=originalHandle;}
    },method);
    await expect(page.getByRole('alert')).toContainText('不支持导入文件夹');
  }
  await dropFiles(page,[{name:'oversized.yaml',bytes:5*1024*1024+1}]);
  await expect(page.getByRole('alert')).toContainText('文件超过 5 MiB');
  await dropFiles(page,[{name:'boundary.yaml',bytes:5*1024*1024}]);
  await expect(zone.locator('strong')).toHaveText('boundary.yaml'); await expect(previewButton).toBeEnabled();
  await dropFiles(page,[{name:'empty.yaml',text:'   '}]);
  await expect(page.getByRole('alert')).toContainText('文件内容为空');
  await zone.evaluate(element=>{
    const data=new DataTransfer();data.setData('text/plain','not a file');
    element.dispatchEvent(new DragEvent('drop',{dataTransfer:data,bubbles:true,cancelable:true}));
  });
  await expect(page.getByRole('alert')).toContainText('请拖入一个本地配置文件');

  await page.evaluate(()=>{
    const original=File.prototype.text;
    window.__syntheticReads=new Map();
    window.__restoreSyntheticRead=()=>{File.prototype.text=original;};
    File.prototype.text=function(){
      if(this.name.startsWith('deferred-'))return new Promise((resolve,reject)=>window.__syntheticReads.set(this.name,{resolve,reject}));
      if(this.name==='unreadable.yaml')return Promise.reject(new Error('synthetic file read failed'));
      return original.call(this);
    };
  });
  await dropFiles(page,[{name:'unreadable.yaml'}]);
  await expect(page.getByRole('alert')).toContainText('无法读取文件');
  for(const reject of [false,true]) {
    const name=reject?'deferred-reject.yaml':'deferred-old.yaml';
    await dropFiles(page,[{name}]);
    await expect(zone).toHaveAttribute('aria-busy','true');
    await page.waitForFunction(name=>window.__syntheticReads.has(name),name);
    await fileInput.setInputFiles({name:'latest.yaml',mimeType:'text/plain',buffer:Buffer.from(fileText+'-latest')});
    await expect(zone.locator('strong')).toHaveText('latest.yaml');
    await settleRead(page,name,reject);
    await expect(zone.locator('strong')).toHaveText('latest.yaml');
    await expect(page.getByRole('alert')).toHaveCount(0);
    await parse();
    assert.equal(calls.filter(call=>call.path==='/api/import'&&call.body.preview).at(-1).body.content,fileText+'-latest');
  }
  await dropFiles(page,[{name:'deferred-mode.yaml'}]);
  await page.waitForFunction(()=>window.__syntheticReads.has('deferred-mode.yaml'));
  await page.getByRole('button',{name:'文本 / 分享链接',exact:true}).click();
  await page.getByLabel('节点内容',{exact:true}).fill(fileText+'-pasted');
  await settleRead(page,'deferred-mode.yaml');
  await expect(page.getByLabel('节点内容',{exact:true})).toHaveValue(fileText+'-pasted');
  await parse();
  assert.equal(calls.filter(call=>call.path==='/api/import'&&call.body.preview).at(-1).body.mode,'text');
  await page.getByLabel('节点内容',{exact:true}).fill(fileText+'-edited');
  await expect(preview).toHaveCount(0);await expect(confirm).toBeDisabled();
  await page.getByRole('button',{name:'本地文件',exact:true}).click();
  await dropFiles(page,[{name:'deferred-close.yaml'}]);
  await page.waitForFunction(()=>window.__syntheticReads.has('deferred-close.yaml'));
  await close();
  await open();
  await settleRead(page,'deferred-close.yaml');
  await expect(zone.locator('strong')).toHaveText('拖放一个配置文件到此处');
  await expect(previewButton).toBeDisabled();
  await page.evaluate(()=>window.__restoreSyntheticRead());

  // Late success and failure responses must not restore a stale preview after replacement.
  for(const fail of [false,true]) {
    const started=deferred();const reply=deferred();
    const obsolete=fileText+(fail?'-obsolete-error':'-obsolete-success');
    mocks.preview=async body=>{
      if(body.content===obsolete){started.resolve();return reply.promise;}
      return {json:fixturePreview};
    };
    await dropFiles(page,[{name:'obsolete.yaml',text:obsolete}]);
    await previewButton.click();await started.promise;
    await dropFiles(page,[{name:'replacement.yaml',text:fileText}]);
    await expect(zone.locator('strong')).toHaveText('replacement.yaml');
    await parse();
    const completed=page.waitForResponse(response=>response.url().endsWith('/api/import')&&response.request().postDataJSON()?.content===obsolete);
    reply.resolve(fail?{status:400,json:{error:'obsolete-preview-error'}}:{json:{nodes:[{...fixturePreview.nodes[0],name:'过期的预览'}],errors:[]}});
    await completed;await frame(page);
    await expect(preview.getByText('合成出口',{exact:true})).toBeVisible();
    await expect(page.getByText('过期的预览',{exact:true})).toHaveCount(0);
    await expect(page.getByRole('alert')).toHaveCount(0);
  }
  mocks.preview=async()=>({json:fixturePreview});
  await page.getByLabel('同时新建分组',{exact:true}).fill('合成预览分组');
  await expect(preview).toHaveCount(0);await expect(confirm).toBeDisabled();
  await parse();
  await page.getByRole('group',{name:/^所属分组/}).getByRole('checkbox').first().check();
  await expect(preview).toHaveCount(0);await expect(confirm).toBeDisabled();

  // New diagnostics keep total counts even though only 128 details are sent and shown.
  const errors=Array.from({length:128},(_,index)=>({line:index+1,message:'合成解析错误 '+(index+1)}));
  mocks.preview=async()=>({json:{...fixturePreview,errors,totalErrors:257,errorsTruncated:true}});
  await parse();
  await expect(page.getByText('有 257 项未能解析',{exact:true})).toBeVisible();
  await expect(page.locator('.import-errors li')).toHaveCount(128);
  await expect(page.getByText('错误详情已截断，仅显示 128 项（共 257 项）。',{exact:true})).toBeVisible();
  const commitStarted=deferred();const commitReply=deferred();let submitted;
  mocks.commit=async body=>{submitted=body;commitStarted.resolve();return commitReply.promise;};
  await confirm.click();await commitStarted.promise;
  await expect(page.getByRole('button',{name:'本地文件',exact:true})).toBeDisabled();
  await dropFiles(page,[{name:'ignored-during-import.yaml',text:'should not replace import'}]);
  assert.equal(submitted.content,fileText);
  commitReply.resolve({added:2,updated:0,removed:0,skipped:257,errors,totalErrors:257,errorsTruncated:true,subscriptionIds:[]});
  await expect(page.getByRole('heading',{name:'导入处理完成',exact:true})).toBeVisible();
  await expect(page.getByText('有 257 项未能解析',{exact:true})).toBeVisible();
  await expect(page.locator('.import-errors li')).toHaveCount(128);
  await page.getByRole('button',{name:'完成',exact:true}).click();
  assert.equal(calls.filter(call=>call.path==='/api/import'&&!call.body.preview).length,beforeCommits+1,'only explicit confirmation commits');

  await open();
  await dropFiles(page,[{name:'legacy.yaml'}]);
  mocks.preview=async()=>({json:{nodes:fixturePreview.nodes,errors:errors.slice(0,2)}});
  await parse();await expect(page.getByText('有 2 项未能解析',{exact:true})).toBeVisible();
  mocks.preview=async()=>({json:{nodes:fixturePreview.nodes,errors,errorsTruncated:true}});
  await parse();await expect(page.getByText('有 至少 128 项未能解析',{exact:true})).toBeVisible();
  await expect(page.getByText(/服务未提供完整错误总数/)).toBeVisible();
  await page.setViewportSize({width:375,height:900});
  const mobile=await dataTransfer(page,[{name:'mobile.yaml'}]);
  try {await drag(zone,'dragenter',mobile);await noOverflow('Mobile file drop feedback');await page.screenshot({path:'src/web/tests/artifacts/relay-import-mobile.png',fullPage:true});await drag(zone,'dragleave',mobile);}finally{await mobile.dispose();}
  await page.keyboard.press('Escape');await expect(page.getByRole('dialog')).toHaveCount(0);
  const afterClose=await dataTransfer(page,[{name:'outside-closed.yaml'}]);
  try {assert.equal((await drag(page.locator('body'),'dragover',afterClose)).prevented,false,'window drag listeners cleaned up');}finally{await afterClose.dispose();}
  await page.setViewportSize({width:1440,height:1000});
  mocks.preview=null;mocks.commit=null;
  console.log('PASS synthetic browse/drop keyboard path, navigation prevention, nested highlight, directory/multi/empty/5MiB/read errors, stale read/preview guards, invalidation, relay privacy and truthful capped diagnostics');
}

export async function verifyNodeGroupUi({page,state,now,noOverflow}) {
  const originalNodes=state.nodes;const originalGroups=state.groups;
  state.groups=[{id:'filter-a',name:'合成甲组',color:'#0f766e',createdAt:now},{id:'filter-b',name:'合成乙组',color:'#245bb2',createdAt:now},{id:'filter-x',name:'合成排除组',color:'#9f501a',createdAt:now}];
  const node=(id,name,groupIds,extra={})=>({id,name,groupIds,protocol:'socks5',server:'192.0.2.1',port:1080,udp:true,enabled:true,manual:true,customName:false,sourceIds:[],status:'unknown',latencyMs:null,speedMbps:null,lastCheckedAt:'',lastError:'',createdAt:now,updatedAt:now,...extra});
  state.nodes=[
    node('filter-ab','A 合成双组',['filter-a','filter-b'],{dialerId:'filter-a',dialerName:'B 合成甲组',config:{password:'synthetic-node-secret-not-for-display'}}),
    node('filter-a','B 合成甲组',['filter-a']),
    node('filter-b','C 合成乙组',['filter-b'],{dialerId:'missing-synthetic',dialerName:'缺失合成中转'}),
    node('filter-none','D 合成未分组',[]),node('filter-x','E 合成排除',['filter-x']),
    ...Array.from({length:27},(_,i)=>node('filter-fill-'+i,'F 合成节点 '+String(i+1).padStart(2,'0'),['filter-a'])),
  ];
  const sync=async()=>{await Promise.all([page.waitForResponse(response=>new URL(response.url()).pathname==='/api/bootstrap'),page.getByRole('button',{name:'立即同步数据',exact:true}).click()]);};
  const filter=page.locator('.node-group-filter');const summary=filter.locator('summary');
  const includes=filter.getByRole('group',{name:/^包含分组/});
  const excludes=filter.getByRole('group',{name:/^排除分组/});
  const mode=filter.getByLabel('包含匹配方式',{exact:true});
  const ungrouped=filter.getByRole('switch',{name:'仅未分组',exact:true});
  const row=name=>page.locator('.nodes-table').getByRole('button',{name,exact:true});
  const selection=page.locator('.selection-summary');
  try {
    await sync();await expect(row('A 合成双组')).toBeVisible();
    await expect(page.getByText('中转：B 合成甲组',{exact:true})).toBeVisible();
    await expect(page.getByText('中转不可用 · 缺失合成中转',{exact:true})).toBeVisible();
    await row('A 合成双组').click();
    await expect(page.getByRole('dialog').getByText('中转：B 合成甲组',{exact:true})).toBeVisible();
    assert(!(await page.getByRole('dialog').innerText()).includes('synthetic-node-secret-not-for-display'));
    await page.keyboard.press('Escape');
    state.nodes[1].enabled=false;await sync();
    await row('A 合成双组').click();await expect(page.getByRole('dialog').getByText('中转不可用：关联节点已禁用',{exact:true})).toBeVisible();await page.keyboard.press('Escape');
    state.nodes[1].enabled=true;await sync();
    await page.getByLabel('选择 A 合成双组',{exact:true}).check();
    await page.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.pagination')).toContainText('2 / 2');
    await summary.focus();await page.keyboard.press('Enter');
    await expect(filter).toHaveAttribute('open','');
    await includes.getByRole('checkbox',{name:'合成甲组',exact:true}).check();
    await expect(page.locator('.pagination')).toContainText('1 / 2');
    await includes.getByRole('checkbox',{name:'合成乙组',exact:true}).check();
    await expect(row('C 合成乙组')).toBeVisible();
    await mode.selectOption('all');
    await expect(page.locator('.nodes-table tbody tr')).toHaveCount(1);
    await expect(summary).toContainText('包含 2 · 全部');
    await excludes.getByRole('checkbox',{name:'合成甲组',exact:true}).check();
    await expect(page.getByText('没有符合条件的节点',{exact:true})).toBeVisible();
    await expect(selection).toContainText('含 1 个筛选外节点');
    await filter.getByRole('button',{name:'重置分组筛选',exact:true}).click();
    await expect(includes.getByRole('checkbox',{checked:true})).toHaveCount(0);
    await expect(excludes.getByRole('checkbox',{checked:true})).toHaveCount(0);
    await expect(mode).toHaveValue('any');await expect(ungrouped).toHaveAttribute('aria-checked','false');
    await expect(row('A 合成双组')).toBeVisible();
    await expect(selection).not.toContainText('筛选外节点');
    await excludes.getByRole('checkbox',{name:'合成乙组',exact:true}).check();
    await excludes.getByRole('checkbox',{name:'合成排除组',exact:true}).check();
    await expect(row('A 合成双组')).toHaveCount(0);await expect(row('C 合成乙组')).toHaveCount(0);await expect(row('E 合成排除')).toHaveCount(0);
    await expect(row('B 合成甲组')).toBeVisible();await expect(row('D 合成未分组')).toBeVisible();
    await expect(summary).toContainText('排除 2');await expect(selection).toContainText('含 1 个筛选外节点');
    await includes.getByRole('checkbox',{name:'合成甲组',exact:true}).check();await mode.selectOption('all');
    await ungrouped.click();
    await expect(page.locator('.nodes-table tbody tr')).toHaveCount(1);await expect(row('D 合成未分组')).toBeVisible();
    await expect(includes.getByRole('checkbox',{checked:true})).toHaveCount(0);
    await expect(includes.getByRole('checkbox').first()).toBeDisabled();await expect(mode).toBeDisabled();await expect(mode).toHaveValue('any');
    await expect(excludes.getByRole('checkbox',{checked:true})).toHaveCount(2);
    await expect(summary).toContainText('仅未分组');await expect(summary).toContainText('排除 2');
    await page.setViewportSize({width:375,height:900});await noOverflow('Mobile include/exclude group filter');
    await filter.getByRole('button',{name:'收起分组筛选',exact:true}).focus();
    const mobileBounds=await filter.locator('.filter-popover').boundingBox();
    assert(mobileBounds.y >= 0 && mobileBounds.y+mobileBounds.height <= 810,'mobile group panel stays inside viewport above navigation');
    await page.screenshot({path:'src/web/tests/artifacts/group-filter-mobile.png',fullPage:true});
    await page.keyboard.press('Escape');await expect(filter).not.toHaveAttribute('open','');await expect(summary).toBeFocused();
    await summary.click();await filter.getByRole('button',{name:'收起分组筛选',exact:true}).click();await expect(filter).not.toHaveAttribute('open','');
    await summary.click();
    await page.setViewportSize({width:1440,height:1000});
    await ungrouped.click();await expect(mode).toBeEnabled();await expect(includes.getByRole('checkbox',{checked:true})).toHaveCount(0);
    await expect(excludes.getByRole('checkbox',{checked:true})).toHaveCount(2);
    await page.getByRole('button',{name:'重置所有筛选',exact:true}).click();
    await expect(mode).toHaveValue('any');await expect(ungrouped).toHaveAttribute('aria-checked','false');
    await expect(excludes.getByRole('checkbox',{checked:true})).toHaveCount(0);await expect(summary).toContainText('全部分组');
    await page.getByRole('button',{name:'下一页',exact:true}).click();await expect(page.locator('.pagination')).toContainText('2 / 2');
    await mode.selectOption('all');await expect(page.locator('.pagination')).toContainText('1 / 2');
    await page.getByRole('button',{name:'重置所有筛选',exact:true}).click();await expect(mode).toHaveValue('any');
    await page.getByLabel('搜索节点名称、协议、地址或分组').fill('D 合成未分组');
    await ungrouped.click();await excludes.getByRole('checkbox',{name:'合成甲组',exact:true}).check();
    await page.getByRole('button',{name:'重置所有筛选',exact:true}).click();
    await expect(page.getByLabel('搜索节点名称、协议、地址或分组')).toHaveValue('');
    await expect(ungrouped).toHaveAttribute('aria-checked','false');await expect(excludes.getByRole('checkbox',{checked:true})).toHaveCount(0);
    await expect(selection).toContainText('已选 1 项');await expect(selection).not.toContainText('筛选外节点');
    await selection.getByRole('button',{name:'清空',exact:true}).click();
    await summary.click();
    console.log('PASS synthetic node include-any/all, independent exclude-any precedence, ungrouped shortcut, badges, full/group/page resets, hidden selections, relay dependency warnings and mobile filter layout');
  } finally {state.nodes=originalNodes;state.groups=originalGroups;await sync();}
}

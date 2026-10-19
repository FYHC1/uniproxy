import assert from 'node:assert/strict';
import {expect} from '@playwright/test';

// Test-only bootstrap fixtures. This harness intercepts every API call and starts no core/server.
export async function verifyPoolCards({page,state,now,calls,noOverflow}) {
  const saved = structuredClone(state);
  const node=(id,name,extra={})=>({id,name,protocol:'socks5',server:'192.0.2.10',port:1080,udp:true,enabled:true,manual:true,customName:false,groupIds:['pool-group'],sourceIds:[],status:'unknown',latencyMs:null,speedMbps:null,lastCheckedAt:'',lastError:'',createdAt:now,updatedAt:now,...extra});
  const pool={id:'pool-card-fixture',name:'卡片回归出口',strategy:'round-robin',hashKey:'',nodeIds:[],groupIds:['pool-group'],groupMode:'any',excludeNodeIds:[],excludeGroupIds:[],enabled:true,udp:false,testUrl:'https://example.com/check',intervalSeconds:300,timeoutMs:8000,toleranceMs:50,createdAt:now,updatedAt:now};
  const listener=(id,host,port)=>({id,name:'卡片入口-'+id,kind:'pool',targetId:pool.id,protocol:'mixed',host,port,udp:false,enabled:true,users:[],createdAt:now,updatedAt:now});
  state.nodes=[node('card-ok','最近成功线路',{status:'healthy',latencyMs:47,lastCheckedAt:now}),node('card-unknown','未检测线路'),node('card-failed','最近失败线路',{status:'unhealthy',lastError:'fixture upstream timeout',lastCheckedAt:now}),node('card-disabled','已禁用线路',{enabled:false}),node('card-blocked','中转阻断线路',{dialerId:'missing-fixture',dialerError:'依赖节点缺失'})];
  state.groups=[{id:'pool-group',name:'卡片演示分组',color:'#087b6c',createdAt:now}];
  state.pools=[pool,{...pool,id:'pool-empty-fixture',name:'尚未分配入口',groupIds:[]}];
  state.listeners=[listener('card-l1','127.0.0.1',20931),listener('card-l2','::1',20932)];
  state.jobs=[];
  state.runtime={available:true,running:true,applying:false,version:'1.19.32',appliedRevision:state.revision,lastError:'',startedAt:now,dirty:false,listeners:[{id:'card-l1',listening:true,error:''},{id:'card-l2',listening:true,error:''}]};
  const sync=async()=>{await Promise.all([page.waitForResponse(r=>new URL(r.url()).pathname==='/api/bootstrap'),page.getByRole('button',{name:'立即同步数据',exact:true}).click()]);};
  try {
    await sync();await page.getByRole('navigation').getByRole('link',{name:'代理池',exact:true}).click();
    const card=page.getByRole('article',{name:'代理池 卡片回归出口',exact:true});
    await expect(card).toBeVisible();await expect(card.getByText('127.0.0.1:20931',{exact:true})).toBeVisible();await expect(card.getByText('[::1]:20932',{exact:true})).toBeVisible();
    await expect(card.getByText('监听中',{exact:true})).toHaveCount(2);
    await expect(card.getByRole('listitem',{name:'池内节点 最近成功线路',exact:true})).toContainText('47 ms');
    await expect(card.getByRole('listitem',{name:'池内节点 未检测线路',exact:true})).toContainText('尚未检测');
    await expect(card.getByRole('listitem',{name:'池内节点 最近失败线路',exact:true})).toContainText('检测失败');
    await expect(card.getByRole('button',{name:'测延迟 已禁用线路',exact:true})).toBeDisabled();
    await expect(card.getByRole('button',{name:'测延迟 中转阻断线路',exact:true})).toBeDisabled();
    while(await page.getByRole('button',{name:'关闭通知',exact:true}).count()) await page.getByRole('button',{name:'关闭通知',exact:true}).first().click();
    await page.screenshot({path:'src/web/tests/artifacts/pools-desktop.png',fullPage:true});
    for (const width of [375,390,768,1024]) {await page.setViewportSize({width,height:900});await noOverflow('Pool cards '+width);}
    await page.setViewportSize({width:390,height:844});await page.screenshot({path:'src/web/tests/artifacts/pools-mobile.png',fullPage:true});
    await page.setViewportSize({width:1440,height:1000});
    const beforeTests=calls.filter(call=>call.path==='/api/tests').length;
    await card.getByRole('button',{name:'测全部延迟',exact:true}).click();
    await expect(card.getByRole('status')).toContainText('延迟检测进行中');
    const jobCall=calls.filter(call=>call.path==='/api/tests').at(-1);
    assert.deepEqual(jobCall.body,{nodeIds:['card-ok','card-unknown','card-failed'],kind:'latency'});
    assert.equal(calls.filter(call=>call.path==='/api/tests').length,beforeTests+1);
    await expect(card.getByRole('button',{name:'测全部延迟',exact:true})).toBeDisabled();
    const job=state.jobs[0];job.status='completed';job.completed=3;job.results=[{nodeId:'card-ok',ok:true,latencyMs:51,speedMbps:null,bytes:0,error:''},{nodeId:'card-unknown',ok:true,latencyMs:63,speedMbps:null,bytes:0,error:''},{nodeId:'card-failed',ok:false,latencyMs:null,speedMbps:null,bytes:0,error:'fixture timeout'}];job.finishedAt=now;
    state.nodes[0].latencyMs=51;state.nodes[1].latencyMs=63;state.nodes[1].status='healthy';state.nodes[1].lastCheckedAt=now;
    await sync();await expect(card.getByRole('status')).toContainText('延迟检测已完成');await expect(card.getByText('63 ms',{exact:true})).toBeVisible();
    await card.getByRole('button',{name:'测延迟 最近成功线路',exact:true}).click();await expect(card.getByRole('status')).toContainText('0/1');
    assert.deepEqual(calls.filter(call=>call.path==='/api/tests').at(-1).body,{nodeIds:['card-ok'],kind:'latency'});
    state.jobs[0].status='cancelled';await sync();await expect(card.getByRole('status')).toContainText('延迟检测已取消');
    await card.getByRole('button',{name:'测延迟 最近成功线路',exact:true}).click();await expect(card.getByRole('status')).toContainText('0/1');
    state.jobs=[];await sync();await expect(card.getByRole('status')).toContainText('检测任务记录已失效');await expect(card.getByRole('button',{name:'测延迟 最近成功线路',exact:true})).toBeEnabled();
    state.runtime.appliedRevision=state.revision+1;await sync();await expect(card.getByText('状态待同步',{exact:true})).toHaveCount(2);await expect(card.getByText('监听中',{exact:true})).toHaveCount(0);await expect(card.getByRole('button',{name:'测全部延迟',exact:true})).toBeDisabled();
    state.runtime.appliedRevision=state.revision;state.runtime.dirty=true;await sync();await expect(card.getByText('配置待应用',{exact:true})).toHaveCount(2);await expect(card.getByText('监听中',{exact:true})).toHaveCount(0);await expect(card.getByRole('button',{name:'测全部延迟',exact:true})).toBeDisabled();
    await card.getByRole('button',{name:'从池移除 最近成功线路',exact:true}).click();const confirm=page.getByRole('dialog');await expect(confirm).toContainText('不删除总节点');await confirm.getByRole('button',{name:'从池移除',exact:true}).click();await expect(confirm).not.toBeVisible();
    await expect(card.getByRole('listitem',{name:'池内节点 最近成功线路',exact:true})).toHaveCount(0);
    assert(state.nodes.some(item=>item.id==='card-ok'));assert(pool.excludeNodeIds.includes('card-ok'));
    assert(calls.some(call=>call.path==='/api/pools/pool-card-fixture/members/card-ok'&&call.method==='DELETE'));
    await card.getByRole('button',{name:'管理节点',exact:true}).click();const dialog=page.getByRole('dialog');await dialog.getByRole('group',{name:/^包含节点/}).getByRole('checkbox',{name:'最近成功线路',exact:true}).check();await dialog.getByRole('button',{name:'保存代理池',exact:true}).click();await expect(dialog).not.toBeVisible();await expect(card.getByRole('listitem',{name:'池内节点 最近成功线路',exact:true})).toBeVisible();
    await card.getByRole('button',{name:'管理节点',exact:true}).click();const concurrentDialog=page.getByRole('dialog');await concurrentDialog.getByRole('checkbox',{name:'最近失败线路',exact:true}).uncheck();
    state.nodes=state.nodes.filter(item=>item.id!=='card-failed');await page.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));await expect(concurrentDialog.getByText(/个引用的节点已被删除/)).toBeVisible();await concurrentDialog.getByRole('button',{name:'保存代理池',exact:true}).click();await expect(concurrentDialog).not.toBeVisible();assert(!pool.excludeNodeIds.includes('card-failed'));
    state.nodes.push(node('card-restored','替补线路'));await sync();
    const empty=page.getByRole('article',{name:'代理池 尚未分配入口',exact:true});await empty.getByRole('button',{name:'分配监听',exact:true}).click();const listenerDialog=page.getByRole('dialog');await expect(listenerDialog.getByRole('combobox',{name:'绑定目标',exact:true})).toHaveValue('pool-empty-fixture');await listenerDialog.getByRole('button',{name:'取消',exact:true}).click();
    state.nodes.push(...Array.from({length:4},(_,i)=>node('search-extra-'+i,'搜索保留-'+i)));await sync();
    const search=card.getByRole('searchbox',{name:'搜索 卡片回归出口 池内节点',exact:true});await search.fill('搜索保留-0');
    await card.getByRole('button',{name:'从池移除 搜索保留-0',exact:true}).click();await page.getByRole('dialog').getByRole('button',{name:'从池移除',exact:true}).click();await expect(page.getByRole('dialog')).not.toBeVisible();
    await expect(search).toBeVisible();await expect(card.getByText('没有匹配搜索的节点。',{exact:true})).toBeVisible();await search.fill('');await expect(card.getByRole('listitem')).toHaveCount(6);await card.getByRole('button',{name:'下一页',exact:true}).click();await expect(card.getByRole('listitem')).toHaveCount(2);await card.getByRole('button',{name:'上一页',exact:true}).click();
    state.pools=[pool];state.nodes.push(...Array.from({length:7},(_,i)=>node('compact-extra-'+i,'紧凑布局节点-'+i)));state.jobs=[];state.runtime.dirty=false;state.runtime.appliedRevision=state.revision;await sync();await page.reload();await expect(card).toBeVisible();
    await page.setViewportSize({width:1650,height:950});await noOverflow('Compact single pool');
    const bounds=await card.boundingBox();assert(bounds.width>1000 && bounds.height<750,'single pool uses workspace width and stays bounded');
    while(await page.getByRole('button',{name:'关闭通知',exact:true}).count())await page.getByRole('button',{name:'关闭通知',exact:true}).first().click();
    await page.screenshot({path:'src/web/tests/artifacts/pools-compact-single.png',fullPage:true});
    console.log('PASS pool card multiple endpoints, truthful status/latency, job requests/restart recovery, local remove/reinclude, shrink-safe search, compact pagination and responsive layouts');
  } finally {Object.assign(state,saved);await sync();}
}

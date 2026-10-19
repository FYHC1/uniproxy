import test from 'node:test';
import assert from 'node:assert/strict';
import {address,checkHttpUrl,formatDate,isIP,isLoopback,poolMembers,runtimeLabel} from '../utils.ts';
import {emptyNodeGroupFilter,matchesNodeFilter,matchesNodeGroups,nodeRelay} from '../nodeFilters.ts';
import {changePoolSelection,constrainPoolSelection,poolSelectionScope,selectedPoolNodes,withoutDeletedPoolNodes} from '../poolSelection.ts';
import {selectScope} from '../selection.ts';
import {listenerIsLive,listenerState,nodeLatencyText,poolListenerState,poolNodeMetrics} from '../poolView.ts';
import {MAX_IMPORT_BYTES,MAX_IMPORT_ERROR_DETAILS,canDropLocalFile,captureDroppedFiles,importErrorSummary,isFileTransfer,readLocalImport,validateImportContent} from '../importUtils.ts';

const nodes = [
  {id:'a',enabled:true,udp:true,groupIds:['work','asia']},
  {id:'b',enabled:true,udp:false,groupIds:['work']},
  {id:'c',enabled:false,udp:true,groupIds:['asia']},
  {id:'d',enabled:true,udp:true,groupIds:[]},
];
const base = {enabled:true,udp:false,nodeIds:[],groupIds:[],groupMode:'any',excludeNodeIds:[],excludeGroupIds:[]};
const resolve = patch => poolMembers(nodes,{...base,...patch}).map(node=>node.id);
test('empty or disabled pools do not select all nodes',()=>{
  assert.deepEqual(resolve({}),[]);
  assert.deepEqual(resolve({enabled:false,nodeIds:['a']}),[]);
});
test('any/all dynamic groups mirror canonical membership',()=>{
  assert.deepEqual(resolve({groupIds:['work','asia']}),['a','b']);
  assert.deepEqual(resolve({groupIds:['work','asia'],groupMode:'all'}),['a']);
});
test('manual includes union with groups; excludes have precedence',()=>{
  assert.deepEqual(resolve({nodeIds:['d'],groupIds:['work']}),['a','b','d']);
  assert.deepEqual(resolve({nodeIds:['a','d'],groupIds:['work'],excludeGroupIds:['asia']}),['b','d']);
  assert.deepEqual(resolve({nodeIds:['a','d'],groupIds:['work'],excludeNodeIds:['a','d']}),['b']);
});
test('disabled and UDP-incompatible nodes are filtered, unknown health is not',()=>{
  assert.deepEqual(resolve({nodeIds:['a','b','c','d'],udp:true}),['a','d']);
});
test('binding IP validation rejects names, brackets, invalid IPv4 and injected URL syntax',()=>{
  for(const host of ['127.0.0.1','0.0.0.0','::1','2001:db8::1','::ffff:127.0.0.1'])assert.equal(isIP(host),true,host);
  for(const host of ['localhost','256.1.1.1','127.01.0.1','[::1]','example.com','x:1','::1%eth0','::1]/evil',''])assert.equal(isIP(host),false,host);
});
test('loopback classification matches literal loopback addresses',()=>{
  for(const host of ['127.0.0.1','127.255.255.255','::1','0:0:0:0:0:0:0:1','::ffff:127.0.0.1','::ffff:7f00:1'])assert.equal(isLoopback(host),true,host);
  for(const host of ['0.0.0.0','::','localhost','[::1]','192.168.1.1','::ffff:c0a8:101','127.999.1.1'])assert.equal(isLoopback(host),false,host);
});
test('addresses bracket IPv6 and retain real ports',()=>{
  assert.equal(address('::1',20000),'[::1]:20000');
  assert.equal(address('127.0.0.1',1080),'127.0.0.1:1080');
});
test('check URLs reject credentials, fragments and non-HTTP protocols',()=>{
  assert.equal(checkHttpUrl('https://example.com/check?q=1'),true);
  for(const url of ['file:///etc/passwd','javascript:alert(1)','https://u:p@example.com','https://example.com/#x','invalid'])assert.equal(checkHttpUrl(url),false,url);
});
test('absent core and unknown dates are never presented as running or measured',()=>{
  assert.equal(runtimeLabel({available:false,running:false,applying:false,dirty:true}),'内核不可用');
  assert.equal(runtimeLabel({available:true,running:false,applying:false,dirty:false}),'内核未运行');
  assert.equal(formatDate(''),'尚无记录');
  assert.equal(formatDate('0001-01-01T00:00:00Z'),'尚无记录');
});

const nodeGroups = patch => nodes.filter(node => matchesNodeGroups(node,{...emptyNodeGroupFilter(),...patch})).map(node=>node.id);
test('node group filter defaults to all nodes, including disabled and ungrouped',()=>{
  assert.deepEqual(nodeGroups({}),['a','b','c','d']);
  assert.deepEqual(nodeGroups({includeMode:'all'}),['a','b','c','d']);
  const first=emptyNodeGroupFilter(); first.includeGroupIds.push('work');
  assert.deepEqual(emptyNodeGroupFilter().includeGroupIds,[],'reset state has no shared mutable arrays');
});
test('node group includes use any/all, exclude-any is independent and wins collisions',()=>{
  assert.deepEqual(nodeGroups({includeGroupIds:['work','asia']}),['a','b','c']);
  assert.deepEqual(nodeGroups({includeGroupIds:['work','asia'],includeMode:'all'}),['a']);
  assert.deepEqual(nodeGroups({excludeGroupIds:['work']}),['c','d']);
  assert.deepEqual(nodeGroups({excludeGroupIds:['work','asia']}),['d']);
  assert.deepEqual(nodeGroups({includeGroupIds:['work'],excludeGroupIds:['asia']}),['b']);
  assert.deepEqual(nodeGroups({includeGroupIds:['work'],excludeGroupIds:['work']}),[]);
  assert.deepEqual(nodeGroups({includeGroupIds:['work','asia'],includeMode:'all',excludeGroupIds:['work']}),[]);
});
test('only-ungrouped shortcut requires empty groupIds and overrides incompatible includes',()=>{
  assert.deepEqual(nodeGroups({onlyUngrouped:true}),['d']);
  assert.deepEqual(nodeGroups({onlyUngrouped:true,includeGroupIds:['work'],excludeGroupIds:['asia']}),['d']);
  assert.deepEqual(nodeGroups({includeGroupIds:['deleted']}),[]);
  assert.deepEqual(nodeGroups({excludeGroupIds:['deleted']}),['a','b','c','d']);
  assert.equal(matchesNodeGroups({groupIds:['deleted']},{...emptyNodeGroupFilter(),onlyUngrouped:true}),false);
});
test('node list uses the same predicate to combine group, search, protocol, status and UDP filters',()=>{
  const baseFilter={...emptyNodeGroupFilter(),query:'',protocol:'all',status:'all',udp:'all'};
  const fixture={...nodes[0],name:'Synthetic Relay',server:'192.0.2.1',protocol:'socks5',status:'unknown'};
  const groupList=[{id:'work',name:'Work Nodes'},{id:'asia',name:'Asia'}];
  const match=patch=>matchesNodeFilter(fixture,groupList,{...baseFilter,...patch});
  assert.equal(match({query:'  WORK ',protocol:'socks5',status:'enabled',udp:'yes',includeGroupIds:['asia'],excludeGroupIds:['other']}),true);
  for(const patch of [{query:'no-match'},{protocol:'http'},{status:'disabled'},{status:'healthy'},{udp:'no'},{excludeGroupIds:['work']}])assert.equal(match(patch),false,JSON.stringify(patch));
  assert.equal(match({status:'unknown'}),true);
  assert.equal(match({query:'192.0.2.1'}),true);
});
test('relay summaries use only public metadata and explain deleted, disabled and annotated dependencies',()=>{
  assert.equal(nodeRelay({},[]),null);
  assert.deepEqual(nodeRelay({dialerId:'up',dialerName:'Old Name'},[{id:'up',name:'Current Name',enabled:true}]),{name:'Current Name',error:''});
  assert.match(nodeRelay({dialerId:'missing'},[]).error,/不存在/);
  assert.match(nodeRelay({dialerId:'up'},[{id:'up',name:'Upstream',enabled:false}]).error,/禁用/);
  assert.equal(nodeRelay({dialerId:'up',dialerError:'Upstream dependency unavailable'},[{id:'up',name:'Upstream',enabled:true}]).error,'Upstream dependency unavailable');
});

const syntheticFile = (content='socks5://192.0.2.1:1080',patch={}) => ({name:'synthetic.yaml',size:Buffer.byteLength(content),webkitRelativePath:'',text:async()=>content,...patch});
const transfer = (files,items=files.map(file=>({kind:'file',getAsFile:()=>file,webkitGetAsEntry:()=>({isDirectory:false})})))=>({files,items,types:['Files']});
test('local file browse and drop share the read path; exact 5 MiB succeeds',async()=>{
  const file=syntheticFile();
  assert.equal((await readLocalImport({files:[file]})).content,await file.text());
  const dropped=captureDroppedFiles(transfer([file]));
  assert.equal((await readLocalImport(dropped)).file,file);
  const full=syntheticFile('x'.repeat(MAX_IMPORT_BYTES));
  assert.equal((await readLocalImport({files:[full]})).content.length,MAX_IMPORT_BYTES);
  assert.equal(canDropLocalFile(transfer([full])),true);
});
test('local import rejects empty, oversized, unreadable, multiple and directory input before preview',async()=>{
  await assert.rejects(readLocalImport({files:[]}),/一个本地配置文件/);
  await assert.rejects(readLocalImport({files:[syntheticFile(' ')]}),/内容为空/);
  await assert.rejects(readLocalImport({files:[syntheticFile('',{size:MAX_IMPORT_BYTES+1,text:()=>{throw Error('must not read');}})]}),/超过 5 MiB/);
  await assert.rejects(readLocalImport({files:[syntheticFile('',{text:async()=>{throw Error('synthetic I/O');}})]}),/无法读取文件/);
  await assert.rejects(readLocalImport({files:[syntheticFile(),syntheticFile()]}),/一次只能导入一个/);
  await assert.rejects(readLocalImport({files:[syntheticFile()],itemCount:2}),/一次只能导入一个/);
  await assert.rejects(readLocalImport({files:[syntheticFile('',{webkitRelativePath:'folder/config.yaml'})]}),/文件夹/);
  const dir=transfer([],[{kind:'file',getAsFile:()=>null,webkitGetAsEntry:()=>({isDirectory:true})}]);
  assert.equal(canDropLocalFile(dir),false);
  await assert.rejects(readLocalImport(captureDroppedFiles(dir)),/文件夹/);
  const handleDir=transfer([],[{kind:'file',getAsFile:()=>null,getAsFileSystemHandle:async()=>({kind:'directory'})}]);
  await assert.rejects(readLocalImport(captureDroppedFiles(handleDir)),/文件夹/);
});
test('drop snapshots items synchronously and handles inaccessible entries conservatively',async()=>{
  const file=syntheticFile();
  const item={kind:'file',getAsFile:()=>file,getAsFileSystemHandle:async()=>({kind:'file'})};
  const captured=captureDroppedFiles(transfer([],[item]));
  item.getAsFile=()=>null;
  assert.equal((await readLocalImport(captured)).file,file,'file remains available after drag store expires');
  const broken=transfer([file],[{kind:'file',getAsFile:()=>file,webkitGetAsEntry:()=>{throw Error('unavailable');}}]);
  await assert.rejects(readLocalImport(captureDroppedFiles(broken)),/无法读取拖放项/);
  const rejected=transfer([file],[{kind:'file',getAsFile:()=>file,getAsFileSystemHandle:()=>Promise.reject(Error('unavailable'))}]);
  await assert.rejects(readLocalImport(captureDroppedFiles(rejected)),/无法读取拖放项/);
});
test('drag checks accept protected file drags and reject text, multi, directories and oversized files',()=>{
  assert.equal(isFileTransfer(null),false);
  assert.equal(isFileTransfer({files:[],items:[],types:['text/plain']}),false);
  assert.equal(canDropLocalFile({files:[],items:[],types:['text/plain']}),false);
  assert.equal(canDropLocalFile(transfer([],[{kind:'file',getAsFile:()=>null}])),true);
  assert.equal(canDropLocalFile(transfer([syntheticFile(),syntheticFile()])),false);
  assert.equal(canDropLocalFile(transfer([syntheticFile('',{size:MAX_IMPORT_BYTES+1})])),false);
});
test('content validation measures UTF-8 bytes instead of characters',()=>{
  assert.throws(()=>validateImportContent('界'.repeat(Math.floor(MAX_IMPORT_BYTES/3)+1)),/超过 5 MiB/);
  assert.throws(()=>validateImportContent('   ','empty test message'),/empty test message/);
});
test('import totals stay honest when the detail list is capped, absent or from legacy servers',()=>{
  const errors=Array.from({length:MAX_IMPORT_ERROR_DETAILS},(_,i)=>({line:i+1,message:'synthetic issue'}));
  const capped=importErrorSummary({errors,totalErrors:300,errorsTruncated:true});
  assert.equal(capped.total,300); assert.equal(capped.countLabel,'300'); assert.equal(capped.exact,true);
  assert.equal(capped.details.length,128); assert.equal(capped.truncated,true);
  assert.equal(importErrorSummary({errors,totalErrors:300}).total,300);
  const legacy=importErrorSummary({errors:errors.slice(0,2)});
  assert.equal(legacy.countLabel,'2'); assert.equal(legacy.truncated,false);
  const unknown=importErrorSummary({errors,errorsTruncated:true});
  assert.equal(unknown.exact,false); assert.equal(unknown.countLabel,'至少 128');
  assert.equal(importErrorSummary({errors:[],totalErrors:300,errorsTruncated:true}).total,300);
  assert.equal(importErrorSummary({errors:errors.slice(0,3),totalErrors:0}).total,3);
  for(const totalErrors of [NaN,-1,1.5,Infinity])assert.equal(importErrorSummary({errors:errors.slice(0,3),totalErrors}).total,3);
  assert.equal(importErrorSummary({errors:[...errors,...errors]}).details.length,128,'UI also bounds older uncapped responses');
});

test('pool counts exclude nodes blocked by a missing or disabled relay',()=>{
 const candidates=[...nodes,{id:'relay-leaf',enabled:true,udp:true,groupIds:['work'],dialerId:'missing',dialerError:'中转节点缺失'}];
 assert.deepEqual(poolMembers(candidates,{...base,nodeIds:['relay-leaf','a']}).map(node=>node.id),['a']);
});

test('pool chooser narrows by groups any/all and UDP, no group means all candidates',()=>{
  const scope=patch=>poolSelectionScope(nodes,{...base,...patch}).map(node=>node.id);
  assert.deepEqual(scope({}),['a','b','c','d']);
  assert.deepEqual(scope({groupIds:['work']}),['a','b']);
  assert.deepEqual(scope({groupIds:['work','asia']}),['a','b','c']);
  assert.deepEqual(scope({groupIds:['work','asia'],groupMode:'all'}),['a']);
  assert.deepEqual(scope({groupIds:['work'],udp:true}),['a']);
  assert.deepEqual(scope({groupIds:['missing']}),[]);
});
test('pool checkbox state shows dynamic matches and unchecking really excludes them',()=>{
  const pool={...base,groupIds:['work']};
  assert.deepEqual(selectedPoolNodes(nodes,pool).map(node=>node.id),['a','b']);
  const deselected=changePoolSelection(pool,poolSelectionScope(nodes,pool),['b']);
  assert.deepEqual(deselected.nodeIds,[]);
  assert.deepEqual(deselected.excludeNodeIds,['a']);
  assert.deepEqual(poolMembers(nodes,deselected).map(node=>node.id),['b']);
  const restored=changePoolSelection(deselected,poolSelectionScope(nodes,deselected),['a','b']);
  assert.deepEqual(restored.excludeNodeIds,[]);
  assert.deepEqual(restored.nodeIds,[],'group nodes are not accidentally pinned to static membership');
  assert.deepEqual(poolMembers(nodes,restored).map(node=>node.id),['a','b']);
});
test('pool selection preserves hidden values and legacy group exclusions until explicit action',()=>{
  const pool={...base,groupIds:['work'],nodeIds:['d'],excludeGroupIds:['asia'],excludeNodeIds:['c']};
  const scope=poolSelectionScope(nodes,pool);
  const changed=changePoolSelection(pool,scope,['a','b']);
  assert.deepEqual(changed.nodeIds,['d']);
  assert.deepEqual(changed.excludeGroupIds,['asia']);
  assert.deepEqual(changed.excludeNodeIds,['c']);
  assert.deepEqual(selectedPoolNodes(nodes,changed).map(node=>node.id),['b','d']);
  const constrained=constrainPoolSelection(nodes,changed);
  assert.deepEqual(constrained.nodeIds,[]);
  assert.deepEqual(constrained.excludeGroupIds,['asia']);
  assert.deepEqual(constrained.excludeNodeIds,['c']);
  assert.deepEqual(pool.nodeIds,['d'],'draft updates do not mutate prior state');
});
test('manual includes are editable when no group is chosen; disabling does not erase intent',()=>{
  let pool=changePoolSelection({...base},nodes,['a','c']);
  assert.deepEqual(pool.nodeIds,['a','c']);
  assert.deepEqual(selectedPoolNodes(nodes,pool).map(node=>node.id),['a','c']);
  assert.deepEqual(poolMembers(nodes,pool).map(node=>node.id),['a']);
  pool=changePoolSelection(pool,nodes,[]);
  assert.deepEqual(pool.nodeIds,[]);
  assert.deepEqual(pool.excludeNodeIds,[]);
});
test('deleted pool refs are removed without changing existing hidden refs or group exclusions',()=>{
  const draft={...base,groupIds:['work'],nodeIds:['a','d','deleted-manual'],excludeNodeIds:['c','deleted-excluded'],excludeGroupIds:['asia']};
  const cleaned=withoutDeletedPoolNodes(nodes,draft);
  assert.deepEqual(cleaned.nodeIds,['a','d']);
  assert.deepEqual(cleaned.excludeNodeIds,['c']);
  assert.deepEqual(cleaned.excludeGroupIds,['asia']);
  assert.deepEqual(draft.nodeIds,['a','d','deleted-manual']);
});
test('dynamic pool membership still includes future group nodes after selection edits',()=>{
  const pool=changePoolSelection({...base,groupIds:['work']},nodes,['b']);
  const more=[...nodes,{id:'later',enabled:true,udp:true,groupIds:['work']}];
  assert.deepEqual(poolMembers(more,pool).map(node=>node.id),['b','later']);
});
test('bulk selection covers more than 100 filtered results and never clears other results',()=>{
  const all=Array.from({length:344},(_,i)=>'node-'+i);
  const selected=selectScope(['outside'],all,true);
  assert.equal(selected.length,345);
  assert.equal(new Set(selectScope(selected,all,true)).size,345);
  const search=all.slice(130,260);
  const removed=selectScope(selected,search,false);
  assert.equal(removed.length,215);
  assert(removed.includes('outside'));
  assert(!removed.includes('node-150'));
  assert(removed.includes('node-300'));
  assert.deepEqual(selectScope(['outside'],[],false),['outside']);
});
test('pool listener badge never claims desired address is live while dirty or applying',()=>{
  const listener={id:'l1',enabled:true};
  const runtime={available:true,running:true,dirty:false,applying:false,listeners:[{id:'l1',listening:true,error:''}]};
  assert.equal(poolListenerState(listener,runtime).label,'监听中');
  assert.equal(poolListenerState(listener,{...runtime,appliedRevision:2},1).label,'状态待同步');
  assert.equal(poolListenerState(listener,{...runtime,appliedRevision:2},2).label,'监听中');
  assert.equal(poolListenerState(listener,{...runtime,dirty:true}).label,'配置待应用');
  assert.equal(poolListenerState(listener,{...runtime,applying:true}).label,'正在应用');
  assert.equal(poolListenerState(listener,{...runtime,available:false}).label,'内核不可用');
  assert.equal(poolListenerState(listener,{...runtime,running:false}).label,'内核未运行');
  assert.equal(poolListenerState({...listener,enabled:false},runtime).label,'已停用');
  assert.equal(poolListenerState(listener,{...runtime,listeners:[]}).label,'未监听');
});
test('pool health metrics use real successful measurements without counting unknown as zero',()=>{
  assert.deepEqual(poolNodeMetrics([]),{healthy:0,unhealthy:0,unknown:0,medianMs:null,lastCheckedAt:''});
  const samples=[{status:'healthy',latencyMs:0,lastCheckedAt:'2026-10-01T01:00:00Z'},{status:'healthy',latencyMs:20,lastCheckedAt:'2026-10-01T02:00:00Z'},{status:'unhealthy',latencyMs:100,lastCheckedAt:'2026-10-01T03:00:00Z'},{status:'unknown',latencyMs:null,lastCheckedAt:''}];
  assert.deepEqual(poolNodeMetrics(samples),{healthy:2,unhealthy:1,unknown:1,medianMs:10,lastCheckedAt:'2026-10-01T03:00:00Z'});
  assert.equal(poolNodeMetrics([...samples,{status:'healthy',latencyMs:50,lastCheckedAt:''}]).medianMs,20);
});
test('pool latency distinguishes unknown, measured zero and failed last result honestly',()=>{
  assert.equal(nodeLatencyText({status:'unknown',latencyMs:null}),'尚未检测');
  assert.equal(nodeLatencyText({status:'healthy',latencyMs:42}),'42 ms');
  assert.equal(nodeLatencyText({status:'healthy',latencyMs:0}),'0 ms');
  assert.equal(nodeLatencyText({status:'unhealthy',latencyMs:42}),'检测失败');
  assert.equal(nodeLatencyText({status:'unknown',latencyMs:42}),'尚未检测');
});


test('listener state is shared by every page and never claims a stale revision is live',()=>{
  const listener={id:'l1',enabled:true};
  const runtime={available:true,running:true,dirty:false,applying:false,appliedRevision:3,listeners:[{id:'l1',listening:true,error:''}]};
  assert.equal(listenerState(listener,runtime,3).label,'监听中');
  assert.equal(listenerIsLive(listener,runtime,3),true);
  // The runtime snapshot describes a different configuration, so "live" is not knowable.
  assert.equal(listenerState(listener,runtime,4).label,'状态待同步');
  assert.equal(listenerIsLive(listener,runtime,4),false);
  assert.equal(listenerState(listener,{...runtime,dirty:true},3).label,'配置待应用');
  assert.equal(listenerIsLive(listener,{...runtime,dirty:true},3),false);
  assert.equal(listenerState({...listener,enabled:false},runtime,3).label,'已停用');
  assert.equal(listenerIsLive({...listener,enabled:false},runtime,3),false);
  assert.equal(listenerState(listener,{...runtime,listeners:[{id:'l1',listening:true,error:'bind failed'}]},3).label,'未监听');
  // The pool card keeps using the same function through its alias.
  assert.equal(poolListenerState(listener,runtime,4).label,listenerState(listener,runtime,4).label);
});

import { useEffect, useRef, useState } from 'react';
import { ArrowDown, ArrowUp, Check, Gauge, Plus, Trash } from '@phosphor-icons/react';
import type { ChainCreateRequest, ChainCreateResult, ChainDraft, ChainCheckResult, ProxyChain, TestJob } from '../shared/types';
import { useAction, useApp } from './state';
import { Badge, Button, ConfirmDialog, ErrorBox, Field, FormFooter, Modal, Note, SearchInput, Toggle, type Confirmation } from './ui';
import { CredentialsEditor, serializedUsers, type UserDraft } from './ListenerForms';
import { address, isIP, isLoopback } from './utils';
import { chainCandidates, moveChainNode, orderedChainNodes, removeChainNode, toggleChainNode } from './chainSelection';
import type { PoolNodeSort, PoolNodeStatusFilter } from './poolSelection';
import { nodeLatencyText } from './poolView';

const MAX_HOPS = 16;
const AUTO_TEST_LIMIT = 20;

export function ChainDialog({chain,onClose}:{chain?:ProxyChain;onClose:()=>void}) {
  const {data,mutate,request,refresh,notify,updatedAt} = useApp(); const state = data!; const action = useAction();
  const [draft,setDraft] = useState<ChainDraft>(() => chain ? {name:chain.name,nodeIds:[...chain.nodeIds],enabled:chain.enabled} : {name:'',nodeIds:[],enabled:true});
  const [group,setGroup] = useState('');
  const [query,setQuery] = useState('');
  const [status,setStatus] = useState<PoolNodeStatusFilter>('all');
  const [sort,setSort] = useState<PoolNodeSort>('name');
  const [selectionError,setSelectionError] = useState('');
  const [confirmation,setConfirmation] = useState<Confirmation|null>(null);
  const [testJob,setTestJob] = useState<TestJob|null>(null);
  const [testedChain,setTestedChain] = useState<ChainCheckResult|null>(null);
  const autoTested = useRef(false);
  const latestBootstrapAt = useRef(updatedAt); latestBootstrapAt.current = updatedAt;
  const [jobBootstrapAt,setJobBootstrapAt] = useState<Date|null>(null);
  const [withListener,setWithListener] = useState(!chain);
  const [listenerName,setListenerName] = useState('');
  const [protocol,setProtocol] = useState<'mixed'|'http'|'socks'>('mixed');
  const [host,setHost] = useState('127.0.0.1');
  const [port,setPort] = useState('');
  const [listenerUDP,setListenerUDP] = useState(false);
  const [users,setUsers] = useState<UserDraft[]>([]);
  const runtimeReady = state.runtime.available && state.runtime.running && !state.runtime.dirty && !state.runtime.applying && state.runtime.appliedRevision === state.revision;
  const visible = chainCandidates(state.nodes,group,query,status,sort);
  const visibleIds = new Set(visible.map(node => node.id));
  const selectedNodes = orderedChainNodes(draft,state.nodes);
  const selectedSet = new Set(draft.nodeIds);
  const hiddenSelected = draft.nodeIds.filter(id => !visibleIds.has(id));
  const allNodesUDP = selectedNodes.length >= 2 && selectedNodes.every(node => Boolean(node?.udp));
  const missingNodes = selectedNodes.filter(node => !node).length;
  const blockedNodes = selectedNodes.filter(node => node && (!node.enabled || node.dialerId || node.dialerError)).length;
  const publishedJob = testJob && state.jobs.find(item => item.id === testJob.id);
  useEffect(() => { if (publishedJob) setTestJob(publishedJob); },[publishedJob]);
  useEffect(() => {
    if (testJob && updatedAt !== jobBootstrapAt && !publishedJob) {
      setTestJob(null);
      notify('延迟检测任务记录已失效，请重新测试。','warning');
    }
  },[testJob,updatedAt,jobBootstrapAt,publishedJob,notify]);
  const activeJob = publishedJob || testJob;
  const jobRunning = activeJob?.status === 'queued' || activeJob?.status === 'running';
  const anyJobRunning = state.jobs.some(job => job.status === 'queued' || job.status === 'running');
  useEffect(() => { if (listenerUDP && !allNodesUDP) setListenerUDP(false); },[listenerUDP,allNodesUDP]);
  useEffect(() => {setTestedChain(null);},[draft.nodeIds.join('\u0000'),draft.enabled]);
  useEffect(() => {
    if (autoTested.current || !runtimeReady) return;
    const nodeIds = visible.filter(node => node.enabled && !node.dialerId && !node.dialerError).slice(0,AUTO_TEST_LIMIT).map(node => node.id);
    if (!nodeIds.length) return;
    autoTested.current = true;
    // Auto-probe only once when the runtime is ready. It never writes health back to the node registry.
    request<TestJob>('/api/tests','POST',{nodeIds,kind:'latency'}).then(job => {setJobBootstrapAt(latestBootstrapAt.current);setTestJob(job);void refresh();}).catch(() => undefined);
  },[runtimeReady,visible,request,refresh]);
  function update<K extends keyof ChainDraft>(key:K,value:ChainDraft[K]) {setDraft(previous => ({...previous,[key]:value}));setSelectionError('');}
  function add(id:string) {
    if (draft.nodeIds.includes(id)) {update('nodeIds',removeChainNode(draft.nodeIds,id));return;}
    if (draft.nodeIds.length >= MAX_HOPS) {setSelectionError('一条代理链最多 ' + MAX_HOPS + ' 跳，请先移除其他节点。');return;}
    update('nodeIds',toggleChainNode(draft.nodeIds,id));
  }
  async function testNodes(nodeIds:string[]) {
    const job = await request<TestJob>('/api/tests','POST',{nodeIds,kind:'latency'});
    setJobBootstrapAt(latestBootstrapAt.current);setTestJob(job);
    notify('延迟检测已提交，共 ' + job.total + ' 个节点。','info');
    await refresh();
  }
  async function testChain() {
    if (!chain) return;
    setTestedChain(null);
    const result = await request<ChainCheckResult>('/api/chains/' + encodeURIComponent(chain.id) + '/test','POST',{});
    if (!result.ok) throw new Error(result.error || '整链测试失败，请检查每一跳的运行状态。');
    setTestedChain(result);
    notify('整链延迟 ' + (result.latencyMs ?? '—') + ' ms。');
  }
  async function save() {
    if (!draft.name.trim()) throw new Error('请输入代理链名称。');
    if (draft.nodeIds.length < 2) throw new Error('链式代理至少需要 2 个节点，请再添加一个节点。');
    if (draft.nodeIds.length > MAX_HOPS) throw new Error('一条代理链最多 ' + MAX_HOPS + ' 跳。');
    if (draft.enabled && missingNodes) throw new Error('代理链包含已删除的节点，请移除后重试。');
    if (draft.enabled && blockedNodes) throw new Error('代理链包含已停用、已设置中转或中转异常的节点，请先修复节点状态。');
    if (new Set(draft.nodeIds).size !== draft.nodeIds.length) throw new Error('代理链不能重复使用同一节点。');
    const payload:ChainDraft = {name:draft.name.trim(),nodeIds:[...draft.nodeIds],enabled:draft.enabled};
    if (chain) {
      await mutate('/api/chains/' + encodeURIComponent(chain.id),'PUT',payload,'代理链已更新');
    } else {
      let listener:ChainCreateRequest['listener'];
      if (withListener) {
        if (!isIP(host)) throw new Error('监听地址必须是有效的 IPv4 或 IPv6 地址，不支持域名和方括号。');
        const manualPort = port.trim();
        const listenerPort = manualPort ? Number(manualPort) : 0;
        if (manualPort && (!Number.isInteger(listenerPort) || listenerPort < 1024 || listenerPort > 65535)) throw new Error('端口必须为 1024–65535 的整数；留空时自动分配。');
        const credentials = serializedUsers(users,[]);
        if (!isLoopback(host) && !credentials.length) throw new Error('非回环地址必须设置至少一组用户名和密码。');
        if (listenerUDP && (!isLoopback(host) || !allNodesUDP || protocol === 'http')) throw new Error('UDP 需要每一跳都支持 UDP、回环地址及 mixed / SOCKS5 监听。');
        listener = {name:listenerName.trim() || undefined,protocol,host:host.trim(),port:listenerPort,udp:listenerUDP,users:credentials,enabled:true};
      }
      await mutate<ChainCreateResult>('/api/chains/with-listener','POST',{chain:payload,listener},'代理链已创建');
    }
    onClose();
  }
  const savedOrder = chain ? chain.nodeIds.join('\u0000') : '';
  const orderChanged = Boolean(chain && savedOrder !== draft.nodeIds.join('\u0000'));
  return <><Modal title={chain ? '编辑代理链' : '新建代理链'} description="按顺序组合节点：前一跳承载后一跳的出站流量。链路不会修改原始节点，也不等同于代理组。" onClose={onClose} busy={action.running} wide contentClassName="chain-dialog-modal">
    <form onSubmit={event => {event.preventDefault();void action.execute(save);}}><div className="dialog-body chain-dialog"><fieldset className="form-fields chain-dialog-fields" disabled={action.running}>
      <div className="chain-dialog-heading"><Field label="代理链名称" required><input autoFocus required maxLength={100} value={draft.name} onChange={event => update('name',event.target.value)} placeholder="例如：日本入口 → 美国出口" /></Field><Toggle label="启用此代理链" checked={draft.enabled} onChange={value => update('enabled',value)} description="停用后关联监听会被拒绝转发，不会回退直连。" /></div>
      {!chain && <section className="chain-entry" aria-labelledby="chain-entry-title">
        <div className="chain-entry-head"><h3 id="chain-entry-title">入口监听</h3><Toggle label="创建" checked={withListener} onChange={setWithListener} /></div>
        {withListener && <div className="chain-entry-fields">
          <Field label="名称"><input value={listenerName} onChange={event => setListenerName(event.target.value)} maxLength={100} placeholder={draft.name || '同代理链名称'} /></Field>
          <Field label="协议"><select value={protocol} onChange={event => {const value=event.target.value as 'mixed'|'http'|'socks';setProtocol(value);if(value === 'http') setListenerUDP(false);}}><option value="mixed">Mixed · HTTP + SOCKS5</option><option value="http">HTTP</option><option value="socks">SOCKS5</option></select></Field>
          <Field label="地址" required><input required value={host} onChange={event => setHost(event.target.value)} className="mono" spellCheck={false} placeholder="127.0.0.1" /></Field>
          <Field label="端口"><input type="text" inputMode="numeric" pattern="[0-9]*" maxLength={5} value={port} onChange={event => setPort(event.target.value.replace(/\D/g,'').slice(0,5))} placeholder="自动" title={'留空时在 ' + state.settings.portRangeStart + '–' + state.settings.portRangeEnd + ' 内自动分配'} /></Field>
          <div className="chain-entry-flag" title={protocol === 'http' ? 'HTTP 监听不支持 UDP' : !allNodesUDP ? (draft.nodeIds.length < 2 ? '至少选择两个支持 UDP 的节点后可用' : '链路中的节点并非全部支持 UDP') : '仅允许回环地址，且 UDP 不受账号密码保护'}><Toggle label="UDP" checked={listenerUDP} onChange={setListenerUDP} disabled={protocol === 'http' || !allNodesUDP} /></div>
        </div>}
        {withListener && !isLoopback(host) && <Note tone="warning">非回环监听可能允许其他设备访问，必须设置认证并在主机防火墙限制来源。</Note>}
        {withListener && listenerUDP && <Note tone="warning">原生 UDP 数据包不执行用户名密码认证，仅允许回环地址。</Note>}
      </section>}
      <div className="chain-layout">
        <section className="chain-library" aria-label="节点库">
          <div className="chain-library-head"><div><h3>节点库</h3></div><Badge>{state.nodes.length} 个节点</Badge></div>
          <div className="chain-group-strip" role="group" aria-label="按分组筛选节点"><button type="button" className={!group ? 'active' : ''} onClick={() => setGroup('')}>全部</button>{state.groups.map(item => <button type="button" key={item.id} className={group === item.id ? 'active' : ''} onClick={() => setGroup(item.id)}><span className="group-dot" style={{backgroundColor:/^#[0-9a-f]{6}$/i.test(item.color) ? item.color : '#718096'}} />{item.name}</button>)}</div>
          <div className="chain-library-toolbar"><SearchInput value={query} onChange={setQuery} label="搜索节点" placeholder="搜索名称、地址或协议…" /><select aria-label="按节点状态筛选" value={status} onChange={event => setStatus(event.target.value as PoolNodeStatusFilter)}><option value="all">全部状态</option><option value="enabled">已启用</option><option value="disabled">已禁用</option><option value="healthy">检测健康</option><option value="unhealthy">检测异常</option><option value="unknown">尚未检测</option></select><select aria-label="节点排序" value={sort} onChange={event => setSort(event.target.value as PoolNodeSort)}><option value="name">按名称</option><option value="latency">按延迟</option><option value="speed">按速度</option><option value="updated">按最近更新</option></select><Button small disabled={!runtimeReady || !visible.some(node => node.enabled && !node.dialerError) || jobRunning || anyJobRunning || action.running} title={!runtimeReady ? '需要内核运行且当前配置已应用' : '测试当前筛选结果中的可测节点'} onClick={() => void action.execute(() => testNodes(visible.filter(node => node.enabled && !node.dialerError).map(node => node.id)))}><Gauge size={15} />测试可见节点</Button></div>
          {hiddenSelected.length > 0 && <p className="chain-filter-note" role="status">筛选外仍有 {hiddenSelected.length} 个已选节点保留在链路中，不会因筛选被移除。</p>}
          {selectionError && <p className="inline-field-error" role="alert">{selectionError}</p>}
          <div className="chain-node-grid">{visible.length ? visible.map(node => {const selected = selectedSet.has(node.id);const blocked = !node.enabled || Boolean(node.dialerId) || Boolean(node.dialerError);return <div className={'chain-node-card' + (selected ? ' selected' : '') + (blocked ? ' blocked' : '')} key={node.id}>
            <button type="button" className="chain-node-add" aria-pressed={selected} aria-label={(selected ? '从链路移除 ' : '加入链路 ') + node.name} onClick={() => add(node.id)}><span className="chain-node-name">{selected && <Check size={14} weight="bold" />}{node.name}</span><span className="chain-node-meta"><Badge>{node.protocol.toUpperCase()}</Badge><span className="mono">{address(node.server,node.port)}</span></span>{blocked && <span className="chain-node-blocked">{!node.enabled ? '节点已禁用' : node.dialerId ? '已设置中转' : '中转不可用'}</span>}</button>
            <button type="button" className="chain-node-latency" aria-label={'测试节点延迟 ' + node.name} disabled={!runtimeReady || blocked || jobRunning || anyJobRunning || action.running} title={!runtimeReady ? '需要内核运行且当前配置已应用' : '使用全局检测地址重新测量'} onClick={() => void action.execute(() => testNodes([node.id]))}>{nodeLatencyText(node)}</button>
          </div>;}) : <p className="chain-empty">没有匹配当前筛选的节点。</p>}</div>
          {activeJob && <div className="chain-job-progress" role="status"><span>{jobRunning ? '延迟检测进行中' : activeJob.status === 'cancelled' ? '延迟检测已取消' : '延迟检测已完成'} · {activeJob.completed}/{activeJob.total}</span><a href="#/settings">任务详情</a></div>}
        </section>
        <aside className="chain-route" aria-label="代理链配置">
          <div className="chain-route-head"><div><h3>代理链配置</h3><p>{draft.nodeIds.length} 跳 · 按自上而下顺序转发</p></div>{draft.nodeIds.length > 0 && <Button small variant="ghost" onClick={() => setConfirmation({title:'清空代理链？',description:'将移除当前链路中的全部节点，节点本身不会被删除。',label:'清空链路',danger:true,action:async () => {setDraft(previous => ({...previous,nodeIds:[]}));setTestedChain(null);setSelectionError('');}})}>清空</Button>}</div>
          {selectedNodes.length ? <ol className="chain-route-list">{selectedNodes.map((node,index) => <li className={'chain-route-hop' + (!node ? ' missing' : '')} key={(node?.id || 'missing') + '-' + index}>
            <span className="chain-hop-index">{String(index + 1).padStart(2,'0')}</span><div className="chain-hop-main"><strong>{node?.name || '节点已删除'}</strong><span>{node ? node.protocol.toUpperCase() + ' · ' + nodeLatencyText(node) : '需要移除后重新选择'}</span></div><div className="chain-hop-labels">{index === 0 && <Badge tone="blue">入口</Badge>}{index === selectedNodes.length - 1 && <Badge tone="teal">出口</Badge>}</div>
            <div className="chain-hop-actions"><button type="button" aria-label={'上移 ' + (node?.name || '节点')} disabled={index === 0} onClick={() => update('nodeIds',moveChainNode(draft.nodeIds,index,-1))}><ArrowUp size={16} /></button><button type="button" aria-label={'下移 ' + (node?.name || '节点')} disabled={index === selectedNodes.length - 1} onClick={() => update('nodeIds',moveChainNode(draft.nodeIds,index,1))}><ArrowDown size={16} /></button><button type="button" className="danger-text" aria-label={'移除 ' + (node?.name || '节点')} onClick={() => update('nodeIds',removeChainNode(draft.nodeIds,node?.id || ''))}><Trash size={16} /></button></div>
          </li>)}</ol> : <div className="chain-route-empty"><span className="chain-route-empty-icon"><Plus size={24} /></span><h4>尚未添加节点</h4><p>链式代理至少需要 2 个节点，请再添加一个节点</p></div>}
          {draft.nodeIds.length === 1 && <Note tone="warning">链式代理至少需要 2 个节点，请再添加一个节点。</Note>}
          {missingNodes > 0 && <Note tone="warning">有 {missingNodes} 个节点已被删除；保存前必须移除。</Note>}
          {blockedNodes > 0 && <Note tone="warning">有 {blockedNodes} 个节点已禁用、已设置中转或中转异常；启用链路前必须修复。</Note>}
          {chain && <div className="chain-route-test"><Button small disabled={!runtimeReady || !chain.enabled || action.running || orderChanged || draft.nodeIds.length < 2 || blockedNodes > 0 || missingNodes > 0} title={!chain.enabled ? '代理链已停用' : orderChanged ? '请先保存新的跳转顺序，整链测试只验证已保存配置' : !runtimeReady ? '需要内核运行且当前配置已应用' : '使用真实入口测试已保存的整条链路'} onClick={() => void action.execute(testChain)}><Gauge size={16} />测试已保存链路</Button>{orderChanged && <p className="field-hint">顺序或成员已变化，保存后测试的是新链路。</p>}{testedChain && <p className={testedChain.ok ? 'text-teal' : 'text-danger'} role="status">{testedChain.ok ? '整链延迟 ' + (testedChain.latencyMs ?? '—') + ' ms' : testedChain.error}</p>}</div>}
        </aside>
      </div>
      {!chain && withListener && <CredentialsEditor users={users} onChange={setUsers} />}
    </fieldset><ErrorBox message={action.error || selectionError} /></div><FormFooter onClose={onClose} busy={action.running} label={chain ? '保存代理链' : withListener ? '创建代理链与监听' : '创建代理链'}>{chain ? '保存后自动应用配置' : withListener ? '链路与入口一次校验后保存' : '仅保存链路，稍后可在监听端口页绑定入口'}</FormFooter></form>
  </Modal>{confirmation && <ConfirmDialog confirmation={confirmation} onClose={() => setConfirmation(null)} />}</>;
}

import { useEffect, useState } from 'react';
import { ArrowRight, Copy, Gauge, PencilSimple, Plus, Power, Trash } from '@phosphor-icons/react';
import type { ChainCheckResult, ProxyChain, ProxyListener } from '../shared/types';
import { useAction, useApp } from './state';
import { Badge, Button, ConfirmDialog, EmptyState, ErrorBox, IconButton, PageHeader, SearchInput, type Confirmation } from './ui';
import { ChainDialog } from './ChainDialog';
import { ListenerDialog } from './ListenerForms';
import { address, formatDate } from './utils';
import { chainBlockedReason, chainEndpoints } from './chainSelection';
import { listenerState } from './poolView';

export function ChainsPage() {
  const {data} = useApp(); const state = data!;
  const [query,setQuery] = useState('');
  const [status,setStatus] = useState<'all'|'enabled'|'disabled'|'blocked'>('all');
  const [editing,setEditing] = useState<ProxyChain|'new'|null>(null);
  const [listenerEditor,setListenerEditor] = useState<{chainId:string;listener?:ProxyListener}|null>(null);
  const chainIds = new Set(state.chains.map(chain => chain.id));
  const listeners = state.listeners.filter(listener => listener.kind === 'chain' && chainIds.has(listener.targetId));
  const ready = state.runtime.available && state.runtime.running && !state.runtime.dirty && !state.runtime.applying && state.runtime.appliedRevision === state.revision;
  const liveListeners = ready ? listeners.filter(listener => listener.enabled && state.runtime.listeners.some(item => item.id === listener.id && item.listening && !item.error)).length : null;
  const filtered = state.chains.filter(chain => {
    const blocked = chainBlockedReason(chain,state.nodes);
    const listenerText = listeners.filter(listener => listener.targetId === chain.id).map(listener => address(listener.host,listener.port)).join(' ');
    return (chain.name + ' ' + listenerText).toLowerCase().includes(query.trim().toLowerCase())
      && (status === 'all' || (status === 'enabled' ? chain.enabled : status === 'disabled' ? !chain.enabled : Boolean(blocked)));
  });
  return <><PageHeader eyebrow="ROUTING / PROXY CHAINS" title="代理链" description="按顺序串联节点，前一跳承载后一跳的流量；入口只绑定链路末跳，失败时拒绝连接。" actions={<Button variant="primary" onClick={() => setEditing('new')}><Plus size={18} />新建代理链</Button>} />
    <div className="chain-overview" role="region" aria-label="代理链概览"><div><span>代理链</span><strong>{state.chains.length}</strong><small>{state.chains.filter(chain => chain.enabled).length} 已启用</small></div><div><span>链路入口</span><strong>{listeners.length}</strong><small>{liveListeners === null ? '运行状态待确认' : liveListeners + ' 个实际监听中'}</small></div><div><span>启用但不可用</span><strong className={state.chains.filter(chain => chain.enabled && chainBlockedReason(chain,state.nodes)).length ? 'text-amber' : ''}>{state.chains.filter(chain => chain.enabled && chainBlockedReason(chain,state.nodes)).length}</strong><small>节点缺失、停用或中转异常</small></div><div><span>总跳数</span><strong>{state.chains.reduce((sum,chain) => sum + chain.nodeIds.length,0)}</strong><small>所有代理链的节点引用</small></div></div>
    <section className="panel chain-workspace" aria-label="代理链列表"><div className="panel-topline"><div className="section-title"><h2>链路与入口</h2><span className="count-pill">{filtered.length} / {state.chains.length}</span></div><div className="inline-controls"><SearchInput value={query} onChange={setQuery} label="搜索代理链" placeholder="搜索链名称或监听地址…" /><select aria-label="按代理链状态筛选" value={status} onChange={event => setStatus(event.target.value as typeof status)}><option value="all">全部状态</option><option value="enabled">已启用</option><option value="disabled">已禁用</option><option value="blocked">不可用</option></select>{(query || status !== 'all') && <Button small onClick={() => {setQuery('');setStatus('all');}}>重置筛选</Button>}</div></div>
      {filtered.length ? <div className="chain-card-grid">{filtered.map(chain => <ChainCard key={chain.id} chain={chain} onEdit={() => setEditing(chain)} onListener={listener => setListenerEditor({chainId:chain.id,listener})} />)}</div> : <EmptyState title={state.chains.length ? '没有匹配的代理链' : '把节点串成一条可验证的出口路径'} description={state.chains.length ? '调整搜索关键词或状态筛选。' : '至少选择两个节点，按顺序组成链路；可同时创建本地入口监听。任一跳失效时整条链路拒绝连接，不会缩短或回退直连。'} action={!state.chains.length && <Button onClick={() => setEditing('new')}><Plus size={16} />新建代理链</Button>} />}
    </section>
    <div className="footnote">链路按顺序使用固定跳点，不是内核代理组。整链延迟来自真实入口检测，不会改写单节点健康状态；停用或失效链路不会直连。</div>
    {editing && <ChainDialog chain={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} />}
    {listenerEditor && <ListenerDialog listener={listenerEditor.listener} initialChainId={listenerEditor.chainId} onClose={() => setListenerEditor(null)} />}
  </>;
}

function ChainCard({chain,onEdit,onListener}:{chain:ProxyChain;onEdit:()=>void;onListener:(listener?:ProxyListener)=>void}) {
  const {data,mutate,request,notify} = useApp(); const state = data!; const action = useAction();
  const [confirmation,setConfirmation] = useState<Confirmation|null>(null);
  const [result,setResult] = useState<ChainCheckResult|null>(null);
  const chainKey = chain.name + '\u0000' + chain.nodeIds.join('\u0000') + '\u0000' + String(chain.enabled);
  useEffect(() => {setResult(null);},[chainKey]);
  const listeners = state.listeners.filter(listener => listener.kind === 'chain' && listener.targetId === chain.id);
  const endpoints = chainEndpoints(chain,state.nodes);
  const blocked = chainBlockedReason(chain,state.nodes);
  const runtimeReady = state.runtime.available && state.runtime.running && !state.runtime.dirty && !state.runtime.applying && state.runtime.appliedRevision === state.revision;
  async function toggle() {
    await mutate('/api/chains/' + encodeURIComponent(chain.id),'PUT',{name:chain.name,nodeIds:chain.nodeIds,enabled:!chain.enabled},!chain.enabled ? '代理链已启用' : '代理链已停用');
  }
  async function copy() {
    const created = await mutate<ProxyChain>('/api/chains','POST',{name:chain.name + '（副本）',nodeIds:chain.nodeIds,enabled:false},'已创建停用的代理链副本，请检查后启用');
    if (!created.id) throw new Error('服务未返回代理链副本，请刷新核实。');
  }
  async function test() {
    setResult(null);
    const next = await request<ChainCheckResult>('/api/chains/' + encodeURIComponent(chain.id) + '/test','POST',{});
    setResult(next);
    if (!next.ok) throw new Error(next.error || '整链测试失败，请检查每一跳与入口运行状态。');
    notify('整链延迟 ' + (next.latencyMs ?? '—') + ' ms。');
  }
  return <article className={'chain-card' + (!chain.enabled ? ' chain-card--disabled' : '')} aria-label={'代理链 ' + chain.name}>
    <header className="chain-card-head"><div><h3>{chain.name}</h3><p>{chain.nodeIds.length} 跳 · 更新于 {formatDate(chain.updatedAt)}</p></div><div className="row-actions"><Badge dot tone={chain.enabled ? 'teal' : 'neutral'}>{chain.enabled ? '已启用' : '已禁用'}</Badge><IconButton label={'复制代理链 ' + chain.name} disabled={action.running} onClick={() => void action.execute(copy)}><Copy size={17} /></IconButton><IconButton label={'编辑代理链 ' + chain.name} onClick={onEdit}><PencilSimple size={17} /></IconButton><IconButton label={chain.enabled ? '停用代理链 ' + chain.name : '启用代理链 ' + chain.name} disabled={action.running} onClick={() => void action.execute(toggle)}><Power size={17} /></IconButton><IconButton className="danger-text" label={'删除代理链 ' + chain.name} onClick={() => setConfirmation({title:'删除「' + chain.name + '」？',description:'代理链配置将被删除，关联的链路入口监听会被停用并保留原配置。',detail:'不会删除任何节点，也不会缩短或改写其他代理链。',label:'删除代理链',danger:true,action:async () => {await mutate('/api/chains/' + encodeURIComponent(chain.id),'DELETE',undefined,'代理链已删除');}})}><Trash size={17} /></IconButton></div></header>
    <div className="chain-card-route" aria-label="链路顺序">{chain.nodeIds.map((id,index) => {const node = state.nodes.find(item => item.id === id);return <span className="chain-card-hop" key={id + '-' + index}>{index > 0 && <ArrowRight size={14} className="chain-card-arrow" />}<span className={!node ? 'text-danger' : ''}>{node?.name || '节点缺失'}</span></span>;})}</div>
    <div className="chain-card-endpoints"><div><span>入口</span><strong>{endpoints.entry}</strong></div><ArrowRight size={17} /><div><span>出口</span><strong>{endpoints.exit}</strong></div></div>
    {blocked && <p className="chain-card-warning" role="status">当前不可用：{blocked}。启用监听也不会转发。</p>}
    <section className="chain-card-listeners" aria-label="链路入口监听"><div className="chain-card-section-title"><h4>入口监听 <span>{listeners.length}</span></h4><Button small variant="ghost" onClick={() => onListener()}><Plus size={14} />{listeners.length ? '添加监听' : '分配入口'}</Button></div>
      {listeners.length ? listeners.slice(0,3).map(listener => {const status = listenerState(listener,state.runtime,state.revision);return <div className="chain-card-listener" key={listener.id}><div><span className="mono">{address(listener.host,listener.port)}</span><small>{listener.name}{listener.udp ? ' · UDP' : ' · TCP'}</small></div><div className="row-actions"><Badge dot tone={status.tone === 'teal' ? 'success' : status.tone === 'danger' ? 'danger' : status.tone === 'warning' ? 'warning' : 'neutral'}>{status.label}</Badge><IconButton label={'编辑监听 ' + listener.name} onClick={() => onListener(listener)}><PencilSimple size={15} /></IconButton></div></div>;}) : <p className="chain-inline-empty">尚未绑定入口。链路配置已保存，但不会自动开放端口。</p>}
      {listeners.length > 3 && <p className="field-hint">另有 {listeners.length - 3} 个监听，请在监听端口页查看。</p>}
    </section>
    <footer className="chain-card-footer"><div>{result ? <span className={result.ok ? 'text-teal' : 'text-danger'} role="status">{result.ok ? '整链延迟 ' + (result.latencyMs ?? '—') + ' ms' : result.error}</span> : <span className="muted">整链延迟尚未测试</span>}</div><Button small disabled={!runtimeReady || blocked !== '' || !chain.enabled || action.running} title={!runtimeReady ? '需要内核运行且当前配置已应用' : blocked || (!chain.enabled ? '代理链已停用' : '通过真实入口检测整条链路')} onClick={() => void action.execute(test)}><Gauge size={15} />测试整链</Button></footer>
    <ErrorBox message={action.error} />
    {confirmation && <ConfirmDialog confirmation={confirmation} onClose={() => setConfirmation(null)} />}
  </article>;
}

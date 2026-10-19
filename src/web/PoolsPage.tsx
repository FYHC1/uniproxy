import { useState } from 'react';
import { Plus } from '@phosphor-icons/react';
import type { ProxyListener, ProxyPool } from '../shared/types';
import { useApp } from './state';
import { Button, EmptyState, PageHeader, SearchInput } from './ui';
import { ListenerDialog } from './ListenerForms';
import { PoolDialog } from './PoolDialog';
import { PoolCard } from './PoolCard';
import { address, poolMembers, strategyLabels } from './utils';

export function PoolsPage() {
  const {data} = useApp(); const state = data!;
  const [query,setQuery] = useState('');
  const [strategy,setStrategy] = useState('all');
  const [enabled,setEnabled] = useState('all');
  const [editing,setEditing] = useState<ProxyPool | 'new' | null>(null);
  const [listenerEditor,setListenerEditor] = useState<{poolId:string;listener?:ProxyListener}|null>(null);
  const poolIds=new Set(state.pools.map(pool=>pool.id));
  const listeners=state.listeners.filter(listener=>listener.kind==='pool' && poolIds.has(listener.targetId));
  const ready=state.runtime.available && state.runtime.running && !state.runtime.dirty && !state.runtime.applying && state.runtime.appliedRevision===state.revision;
  const listening=ready ? listeners.filter(listener=>listener.enabled && state.runtime.listeners.some(item=>item.id===listener.id && item.listening && !item.error)).length : null;
  const uniqueMembers=new Set(state.pools.flatMap(pool=>poolMembers(state.nodes,pool).map(node=>node.id)));
  const emptyPools=state.pools.filter(pool=>pool.enabled && poolMembers(state.nodes,pool).length===0).length;
  const pools = state.pools.filter(pool => {
    const endpoints = state.listeners.filter(listener => listener.kind === 'pool' && listener.targetId === pool.id).map(listener => address(listener.host,listener.port)).join(' ');
    const groups=state.groups.filter(group=>pool.groupIds.includes(group.id)).map(group=>group.name).join(' ');
    return (pool.name + ' ' + endpoints + ' ' + groups).toLowerCase().includes(query.trim().toLowerCase()) && (strategy==='all' || pool.strategy===strategy) && (enabled==='all' || pool.enabled===(enabled==='enabled'));
  });
  return <><PageHeader eyebrow="ROUTING / PROXY POOLS" title="代理池" description="集中查看入口、候选出口和最近检测，直接管理每个代理池。" actions={<Button variant="primary" onClick={() => setEditing('new')}><Plus size={18} />新建代理池</Button>} />
    <div className="pool-overview" role="region" aria-label="代理池概览"><div><span>代理池</span><strong>{state.pools.length}</strong><small>{state.pools.filter(pool=>pool.enabled).length} 已启用</small></div><div><span>共享监听</span><strong>{listeners.length}</strong><small>{listening===null ? '运行状态待确认' : listening + ' 个实际监听中'}</small></div><div><span>候选出口</span><strong>{uniqueMembers.size}</strong><small>跨池去重，不代表在线</small></div><div><span>空池提醒</span><strong className={emptyPools ? 'text-amber' : ''}>{emptyPools}</strong><small>启用但无可转发成员</small></div></div>
    <section className="pool-workspace" aria-label="代理池列表"><div className="pool-workspace-toolbar"><div className="section-title"><h2>出口与监听</h2><span className="count-pill">{pools.length} / {state.pools.length}</span></div><div className="pool-filter-controls"><SearchInput value={query} onChange={setQuery} label="搜索代理池" placeholder="池名称、分组或监听地址…" /><select aria-label="按代理池策略筛选" value={strategy} onChange={event=>setStrategy(event.target.value)}><option value="all">全部策略</option>{Object.entries(strategyLabels).map(([key,label])=><option key={key} value={key}>{label}</option>)}</select><select aria-label="按代理池状态筛选" value={enabled} onChange={event=>setEnabled(event.target.value)}><option value="all">全部状态</option><option value="enabled">已启用</option><option value="disabled">已禁用</option></select>{(query || strategy!=='all' || enabled!=='all') && <Button small onClick={()=>{setQuery('');setStrategy('all');setEnabled('all');}}>重置筛选</Button>}</div></div>
      {pools.length ? <div className={'pool-grid' + (pools.length===1 ? ' pool-grid--single' : '')}>{pools.map(pool => <PoolCard key={pool.id} pool={pool} onEdit={() => setEditing(pool)} onListener={listener => setListenerEditor({poolId:pool.id,listener})} />)}</div> : <div className="panel"><EmptyState title={state.pools.length ? '未找到对应代理池' : '为出口定义一个策略'} description={state.pools.length ? '调整名称、分组、地址或策略条件。' : '选择节点或分组，并在创建时分配共享监听。支持最低延迟、轮询、一致性哈希和粘性会话。'} action={!state.pools.length && <Button onClick={() => setEditing('new')}><Plus size={16} />新建代理池</Button>} /></div>}
    </section><div className="footnote">空池拒绝连接，绝不直连。监听与健康状态分开显示；延迟是全局检测地址的最近结果，不代表实时在线。</div>
    {editing && <PoolDialog pool={editing === 'new' ? undefined : editing} onClose={() => setEditing(null)} />}
    {listenerEditor && <ListenerDialog listener={listenerEditor.listener} initialPoolId={listenerEditor.poolId} onClose={() => setListenerEditor(null)} />}
  </>;
}

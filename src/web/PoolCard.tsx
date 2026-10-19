import { useEffect, useId, useRef, useState } from 'react';
import { Copy, Gauge, PencilSimple, Plus, Trash } from '@phosphor-icons/react';
import type { ProxyListener, ProxyNode, ProxyPool, TestJob } from '../shared/types';
import { useAction, useApp } from './state';
import { Badge, Button, ConfirmDialog, ErrorBox, GroupTags, IconButton, Pagination, SearchInput, type Confirmation } from './ui';
import { address, formatDate, poolMembers, protocolLabels, strategyLabels } from './utils';
import { selectedPoolNodes } from './poolSelection';
import { nodeLatencyText, poolListenerState, poolNodeMetrics } from './poolView';

export function PoolCard({pool,onEdit,onListener}:{pool:ProxyPool;onEdit:()=>void;onListener:(listener?:ProxyListener)=>void}) {
  const {data,updatedAt,mutate,request,refresh,notify} = useApp(); const state = data!;
  const action = useAction();
  const latestBootstrapAt = useRef(updatedAt); latestBootstrapAt.current = updatedAt;
  const [confirmation,setConfirmation] = useState<Confirmation|null>(null);
  const [nodeQuery,setNodeQuery] = useState('');
  const [page,setPage] = useState(1);
  const [endpointsExpanded,setEndpointsExpanded] = useState(false);
  const pageSize = 6;
  const endpointRegionId = useId();
  const [testJob,setTestJob] = useState<TestJob|null>(null);
  const [jobNotice,setJobNotice] = useState('');
  const [jobBootstrapAt,setJobBootstrapAt] = useState<Date|null>(null);
  const [testingIds,setTestingIds] = useState<string[]>([]);
  const listeners = state.listeners.filter(listener => listener.kind === 'pool' && listener.targetId === pool.id);
  const members = selectedPoolNodes(state.nodes,pool);
  const eligible = poolMembers(state.nodes,pool);
  const eligibleIds = new Set(eligible.map(node => node.id));
  const testable = members.filter(node => node.enabled && !node.dialerError);
  const filtered = members.filter(node => (node.name + ' ' + node.server + ' ' + node.protocol).toLowerCase().includes(nodeQuery.trim().toLowerCase()));
  const currentPage = Math.min(page,Math.max(1,Math.ceil(filtered.length/pageSize)));
  const visible = filtered.slice((currentPage-1)*pageSize,currentPage*pageSize);
  const metrics = poolNodeMetrics(members);
  const runtimeReady = state.runtime.available && state.runtime.running && !state.runtime.dirty && !state.runtime.applying && state.runtime.appliedRevision === state.revision;
  const publishedJob = testJob && state.jobs.find(item => item.id === testJob.id);
  useEffect(() => {
    if (publishedJob) {setTestJob(publishedJob);return;}
    if (testJob && updatedAt !== jobBootstrapAt) {
      setTestJob(null);setTestingIds([]);
      setJobNotice('检测任务记录已失效（服务可能已重启）。请核实最近结果，必要时重新检测。');
    }
  },[publishedJob,testJob,updatedAt,jobBootstrapAt]);
  const job = publishedJob || testJob;
  const jobRunning = job?.status === 'queued' || job?.status === 'running';
  const anyJobRunning = state.jobs.some(item => item.status === 'queued' || item.status === 'running');
  const canTest = runtimeReady && !anyJobRunning && !jobRunning && !action.running;
  const testHint = !runtimeReady ? '需要内核运行且当前配置已应用' : anyJobRunning || jobRunning ? '已有检测任务，请等待完成或在运行设置中取消' : '通过真实代理检测，结果为全局节点最近检测';
  async function test(nodeIds:string[]) {
    const next = await request<TestJob>('/api/tests','POST',{nodeIds,kind:'latency'});
    setJobBootstrapAt(latestBootstrapAt.current);setJobNotice('');setTestJob(next);setTestingIds(nodeIds);
    notify('延迟检测已提交，共 ' + next.total + ' 个节点。','info');
    await refresh();
  }
  async function copy(listener:ProxyListener) {
    if (!navigator.clipboard) throw new Error('当前浏览器不支持剪贴板，请手动复制监听地址。');
    await navigator.clipboard.writeText(address(listener.host,listener.port));
    notify('监听配置地址已复制，不含认证凭据。');
  }
  function remove(node:ProxyNode) {
    setConfirmation({title:'从「' + pool.name + '」移除节点？',description:'仅将「' + node.name + '」从当前代理池移除，不删除总节点，也不影响其他池和独享监听。',detail:'即使该节点属于包含分组，也不会自动回到此池。之后可在“管理节点”中重新勾选；移除最后一个可转发成员后，新连接将被拒绝，不会直连。',label:'从池移除',danger:true,action:async () => {await mutate('/api/pools/' + encodeURIComponent(pool.id) + '/members/' + encodeURIComponent(node.id),'DELETE',undefined,'节点已从此池移除');}});
  }
  return <article className="pool-card" aria-label={'代理池 ' + pool.name}>
    <header className="pool-card-header"><div className="pool-title"><div><h3>{pool.name}</h3><span className="pool-subtitle">{eligible.length} 个可转发 · {members.length} 个已选节点</span></div></div><div className="row-actions"><Badge dot tone={pool.enabled ? 'teal' : 'neutral'}>{pool.enabled ? '已启用' : '已禁用'}</Badge><IconButton label={'编辑代理池 ' + pool.name} onClick={onEdit}><PencilSimple size={18} /></IconButton><IconButton className="danger-text" label={'删除代理池 ' + pool.name} onClick={() => setConfirmation({title:'删除「' + pool.name + '」？',description:'只删除代理池配置，不删除池内节点。',detail:'关联监听将被停用并保留原配置，需要重新绑定目标后才能转发。',label:'删除代理池',danger:true,action:async () => {await mutate('/api/pools/' + encodeURIComponent(pool.id),'DELETE',undefined,'代理池已删除');}})}><Trash size={17} /></IconButton></div></header>
    <div className="pool-strategy"><Badge tone="blue">{strategyLabels[pool.strategy]}</Badge><Badge>{pool.udp ? 'UDP 兼容成员' : 'TCP 出口'}</Badge>{pool.hashKey === 'in-user' && <Badge>按认证用户名哈希</Badge>}{pool.groupIds.length > 0 && <span className="pool-group-mode">分组{pool.groupMode === 'all' ? '交集' : '并集'}动态纳入</span>}</div>
    <div className="pool-card-columns"><div className="pool-config-column">
      {pool.groupIds.length > 0 && <div className="pool-groups"><GroupTags groups={state.groups} ids={pool.groupIds} /></div>}
      <section className="pool-endpoints" aria-label="关联监听" id={endpointRegionId}><div className="pool-section-title"><h4>监听地址 <span>{listeners.length}</span></h4><Button small variant="ghost" onClick={() => onListener()}><Plus size={14} />{listeners.length ? '添加监听' : '分配监听'}</Button></div>
        {listeners.length ? listeners.slice(0,endpointsExpanded ? listeners.length : 2).map(listener => {const status = poolListenerState(listener,state.runtime,state.revision);const detail=state.runtime.listeners.find(item=>item.id===listener.id)?.error;return <div className="pool-endpoint" key={listener.id}>
          <div className="pool-endpoint-main"><span className="pool-address mono">{address(listener.host,listener.port)}</span><div className="pool-endpoint-meta"><span>{protocolLabels[listener.protocol]}</span><span>{listener.udp ? 'TCP + UDP' : '仅 TCP'}</span><span>{listener.users.length ? listener.users.length + ' 组认证' : '无认证 · 仅本机'}</span></div><span className="pool-endpoint-name">{listener.name}</span>{detail && <p className="pool-state-note">{detail}</p>}</div>
          <div className="pool-endpoint-actions"><Badge dot tone={status.tone}>{status.label}</Badge><div className="row-actions"><IconButton label={'复制监听地址 ' + listener.name} disabled={action.running} onClick={() => void action.execute(() => copy(listener))}><Copy size={15} /></IconButton><IconButton label={'编辑监听 ' + listener.name} onClick={() => onListener(listener)}><PencilSimple size={15} /></IconButton></div></div>
        </div>;}) : <p className="pool-inline-empty">尚未分配监听。直接添加入口即可使用。</p>}
        {listeners.length>2 && <Button small variant="ghost" aria-expanded={endpointsExpanded} aria-controls={endpointRegionId} onClick={()=>setEndpointsExpanded(value=>!value)}>{endpointsExpanded ? '收起监听' : '查看全部 ' + listeners.length + ' 个监听'}</Button>}
        {state.runtime.dirty && listeners.length > 0 && <p className="pool-state-note">此处为已保存的配置地址；当前变更尚未应用到内核。</p>}
        {!state.runtime.dirty && state.runtime.appliedRevision !== state.revision && listeners.length > 0 && <p className="pool-state-note">配置列表与运行版本尚未同步，地址暂不可确认为实际监听。请刷新数据。</p>}
      </section>
      <dl className="pool-health-summary" aria-label="最近检测汇总"><div><dt>最近成功</dt><dd className="text-teal">{metrics.healthy}</dd></div><div><dt>最近失败</dt><dd className={metrics.unhealthy ? 'text-danger' : ''}>{metrics.unhealthy}</dd></div><div><dt>尚未检测</dt><dd>{metrics.unknown}</dd></div><div><dt>成功延迟中位数</dt><dd>{metrics.medianMs===null ? '—' : metrics.medianMs + ' ms'}</dd></div></dl>
      <details className="pool-config-details"><summary>策略与检测详情</summary><dl><div><dt>检查地址</dt><dd className="mono">{pool.testUrl}</dd></div><div><dt>自动检查</dt><dd>{pool.intervalSeconds} 秒 / 超时 {pool.timeoutMs} ms</dd></div><div><dt>更新时间</dt><dd>{formatDate(pool.updatedAt,true)}</dd></div><div><dt>最近节点检测</dt><dd>{formatDate(metrics.lastCheckedAt,true)}</dd></div></dl><p>卡片延迟使用全局检测地址的最近结果，健康不等于持续在线。</p></details>
    </div>
    <section className="pool-member-section" aria-label="池内节点"><div className="pool-section-title"><h4>出口节点 <span>{members.length}</span></h4><div className="row-actions"><Button small onClick={onEdit}><Plus size={14} />管理节点</Button><Button small disabled={!testable.length || !canTest} title={testHint} busy={action.running && !confirmation} onClick={() => void action.execute(() => test(testable.map(node => node.id)))}><Gauge size={15} />测全部延迟</Button></div></div>
      {(members.length > pageSize || nodeQuery) && <SearchInput value={nodeQuery} onChange={value => {setNodeQuery(value);setPage(1);}} label={'搜索 ' + pool.name + ' 池内节点'} placeholder="搜索池内节点…" />}
      {filtered.length ? <ul className="pool-node-list">{visible.map(node => {
        const blocked = !node.enabled ? '节点已禁用' : node.dialerError ? '中转不可用' : pool.udp && !node.udp ? 'UDP 不兼容' : !pool.enabled ? '代理池已禁用' : '';
        const testing = jobRunning && testingIds.includes(node.id) && !job?.results.some(result => result.nodeId === node.id);
        return <li className={'pool-node' + (!eligibleIds.has(node.id) ? ' pool-node--inactive' : '')} key={node.id} aria-label={'池内节点 ' + node.name}>
          <div className="pool-node-main"><div className="pool-node-title"><span className={'pool-health pool-health--' + node.status} aria-hidden="true" /><strong title={node.name}>{node.name}</strong></div><details className="pool-node-details"><summary><Badge>{node.protocol.toUpperCase()}</Badge><span>{blocked || '连接信息'}</span></summary><div><span className="pool-node-address mono">{address(node.server,node.port)}</span>{node.dialerId && <span className="pool-node-relay">中转：{node.dialerName || '依赖节点'}</span>}<span>最近检测：{formatDate(node.lastCheckedAt,true)}</span>{(node.lastError || node.dialerError) && <p className="text-danger">{node.lastError || node.dialerError}</p>}</div></details></div>
          <span className={'pool-node-latency ' + (node.status === 'unhealthy' ? 'text-danger' : node.status === 'healthy' ? 'text-teal' : 'muted')} title={'最近检测：' + formatDate(node.lastCheckedAt,true)}>{testing ? '检测中…' : nodeLatencyText(node)}</span>
          <div className="pool-node-actions"><IconButton label={'测延迟 ' + node.name} title={testHint} disabled={!canTest || !node.enabled || !!node.dialerError} onClick={() => void action.execute(() => test([node.id]))}><Gauge size={16} /></IconButton><IconButton label={'从池移除 ' + node.name} className="danger-text" disabled={action.running || jobRunning} onClick={() => remove(node)}><Trash size={16} /></IconButton></div>
        </li>;
      })}</ul> : <p className="pool-inline-empty">{members.length ? '没有匹配搜索的节点。' : '尚无已选节点。空池拒绝新连接，不会自动直连。'}</p>}
      {filtered.length>pageSize && <Pagination total={filtered.length} page={currentPage} pageSize={pageSize} onPage={setPage} />}
      {eligible.length === 0 && members.length > 0 && <p className="pool-state-note">当前无可转发成员；请检查池、节点启用状态或中转依赖。</p>}
      {job && <div className="pool-check-progress" role="status"><span>{jobRunning ? '延迟检测进行中' : job.status === 'cancelled' ? '延迟检测已取消' : '延迟检测已完成'} · {job.completed}/{job.total}</span><a href="#/settings">任务详情与取消</a></div>}
      {jobNotice && <p className="pool-state-note" role="status">{jobNotice}</p>}
      {!runtimeReady && <p className="pool-help-text">内核未就绪或配置待应用，暂不能测延迟。</p>}
    </section></div>
    <ErrorBox message={action.error} />
    {confirmation && <ConfirmDialog confirmation={confirmation} onClose={() => setConfirmation(null)} />}
  </article>;
}

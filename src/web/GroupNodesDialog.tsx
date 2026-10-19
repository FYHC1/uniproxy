import { useEffect, useRef, useState } from 'react';
import { Gauge, Play, Plus, Power, Trash } from '@phosphor-icons/react';
import type { Bootstrap, ProxyNode, TestJob } from '../shared/types';
import { useAction, useApp } from './state';
import { Badge, Button, Check, ChoiceList, EmptyState, ErrorBox, IconButton, Modal, Note, Pagination, SearchInput, TableRegion } from './ui';
import { address, formatDate } from './utils';
import { nodeRelay } from './nodeFilters';
import { nodeLatencyText } from './poolView';
import { selectScope } from './selection';

type Panel = { kind: 'members' | 'add' } | { kind: 'remove' | 'disable'; ids: string[]; requestedCount: number };
type BatchAction = 'add_groups' | 'remove_groups' | 'enable' | 'disable';
type TestRun = { job: TestJob; ids: string[]; bootstrapAt: Date | null };
const pageSize = 20;
const keepIds = (ids: string[], allowed: Set<string>) => ids.every(id => allowed.has(id)) ? ids : ids.filter(id => allowed.has(id));
const isRuntimeReady = (state: Bootstrap) => state.runtime.available && state.runtime.running && !state.runtime.dirty && !state.runtime.applying && state.runtime.appliedRevision === state.revision;
const hasActiveJob = (state: Bootstrap) => state.jobs.some(job => job.status === 'queued' || job.status === 'running');

export function GroupNodesDialog({ groupId, onClose }: { groupId: string; onClose: () => void }) {
  const { data, updatedAt, mutate, request, refresh, notify } = useApp();
  const state = data!;
  const latest = useRef({ state, updatedAt });
  latest.current = { state, updatedAt };
  const action = useAction();
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState('all');
  const [page, setPage] = useState(1);
  const [selected, setSelected] = useState<string[]>([]);
  const [addIds, setAddIds] = useState<string[]>([]);
  const [panel, setPanel] = useState<Panel>({ kind: 'members' });
  const [testRun, setTestRun] = useState<TestRun | null>(null);
  const [jobNotice, setJobNotice] = useState('');
  const body = useRef<HTMLDivElement>(null);
  const previousPanel = useRef(panel.kind);
  const returnAction = useRef('add');
  const group = state.groups.find(item => item.id === groupId);
  const members = group ? state.nodes.filter(node => node.groupIds.includes(groupId)).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true })) : [];
  const memberIds = new Set(members.map(node => node.id));
  const outside = group ? state.nodes.filter(node => !node.groupIds.includes(groupId)).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN', { numeric: true })) : [];
  const selectedIds = selected.filter(id => memberIds.has(id));
  const additionIds = addIds.filter(id => outside.some(node => node.id === id));
  const selectedNodes = members.filter(node => selectedIds.includes(node.id));
  const enabledCount = members.filter(node => node.enabled).length;
  const sourceCount = members.filter(node => node.sourceIds.length > 0).length;
  const search = query.trim().toLowerCase();
  const filtered = members.filter(node => [node.name, node.protocol, address(node.server, node.port)].join(' ').toLowerCase().includes(search)
    && (status === 'all' || (status === 'enabled' ? node.enabled : status === 'disabled' ? !node.enabled : node.status === status)));
  const currentPage = Math.min(page, Math.max(1, Math.ceil(filtered.length / pageSize)));
  const visible = filtered.slice((currentPage - 1) * pageSize, currentPage * pageSize);
  const hiddenSelected = selectedIds.filter(id => !filtered.some(node => node.id === id)).length;
  const testable = selectedNodes.filter(node => node.enabled && !nodeRelay(node, state.nodes)?.error);
  const pendingNodes = panel.kind === 'remove' || panel.kind === 'disable' ? members.filter(node => panel.ids.includes(node.id) && (panel.kind !== 'disable' || node.enabled)) : [];
  const publishedJob = testRun && state.jobs.find(job => job.id === testRun.job.id);
  const job = publishedJob || testRun?.job;
  const jobRunning = job?.status === 'queued' || job?.status === 'running';
  const anyJobRunning = hasActiveJob(state) || jobRunning;
  const configurationBusy = action.running || anyJobRunning;
  const runtimeReady = isRuntimeReady(state);
  const canTest = runtimeReady && !anyJobRunning && !action.running;
  const testHint = !runtimeReady ? '需要内核运行且当前配置已应用' : anyJobRunning ? '已有检测任务，请等待完成或在运行与设置中取消' : '通过实际代理检测；结果是全局节点最近检测，不代表持续在线';

  // Remove stale IDs from stored selections, not just from rendering. A node that
  // later re-enters this group must not silently regain an old selection.
  useEffect(() => {
    const currentMembers = new Set(state.nodes.filter(node => group && node.groupIds.includes(groupId)).map(node => node.id));
    const currentOutside = new Set(state.nodes.filter(node => group && !node.groupIds.includes(groupId)).map(node => node.id));
    const enabledMembers = new Set(state.nodes.filter(node => currentMembers.has(node.id) && node.enabled).map(node => node.id));
    setSelected(ids => keepIds(ids, currentMembers));
    setAddIds(ids => keepIds(ids, currentOutside));
    setPanel(previous => {
      if (previous.kind !== 'remove' && previous.kind !== 'disable') return previous;
      const ids = keepIds(previous.ids, previous.kind === 'disable' ? enabledMembers : currentMembers);
      return ids === previous.ids ? previous : { ...previous, ids };
    });
    setTestRun(previous => {
      if (!previous) return previous;
      const ids = keepIds(previous.ids, currentMembers);
      return ids === previous.ids ? previous : { ...previous, ids };
    });
  }, [state.nodes, group?.id, groupId]);

  useEffect(() => {
    if (panel.kind !== 'members') body.current?.querySelector<HTMLButtonElement>('[data-group-cancel]')?.focus();
    else if (previousPanel.current !== 'members') {
      const target = body.current?.querySelector<HTMLButtonElement>('[data-group-action="' + returnAction.current + '"]');
      if (target && !target.disabled) target.focus();
      else body.current?.querySelector<HTMLInputElement>('input[type="search"]')?.focus();
    }
    previousPanel.current = panel.kind;
  }, [panel.kind]);

  useEffect(() => {
    if (!testRun || updatedAt === testRun.bootstrapAt) return;
    if (publishedJob) {
      setTestRun(previous => previous ? { ...previous, job: publishedJob, bootstrapAt: updatedAt } : previous);
    } else {
      setTestRun(null);
      setJobNotice('检测任务记录已失效（服务可能已重启）。请核实最近结果，必要时重新检测。');
    }
  }, [publishedJob, updatedAt, testRun]);

  function showPanel(next: Panel, origin: string) {
    action.setError('');
    returnAction.current = origin;
    if (next.kind === 'add') setAddIds([]);
    setPanel(next);
  }

  function backToMembers() {
    action.setError('');
    setAddIds([]);
    setPanel({ kind: 'members' });
  }

  function currentTargets(ids: string[], adding = false): ProxyNode[] {
    const current = latest.current.state;
    if (!current.groups.some(item => item.id === groupId)) throw new Error('分组已不存在，未提交操作。');
    const requested = new Set(ids);
    return current.nodes.filter(node => requested.has(node.id) && (adding ? !node.groupIds.includes(groupId) : node.groupIds.includes(groupId)));
  }

  async function batch(kind: BatchAction, requested: string[]) {
    if (hasActiveJob(latest.current.state)) throw new Error('已有检测任务，请等待完成或取消后再修改配置。');
    const targets = currentTargets(requested, kind === 'add_groups').filter(node => kind === 'enable' ? !node.enabled : kind === 'disable' ? node.enabled : true);
    if (!targets.length) throw new Error('所选节点已变化，没有需要执行此操作的节点。');
    const ids = targets.map(node => node.id);
    const message = kind === 'add_groups' ? '已添加本组关系' : kind === 'remove_groups' ? '节点已移出本组，全局节点保留' : kind === 'enable' ? '所选节点已全局启用' : '所选节点已全局禁用';
    await mutate('/api/nodes/batch', 'POST', { ids, action: kind, ...(kind === 'add_groups' || kind === 'remove_groups' ? { groupIds: [groupId] } : {}) }, message);
    if (kind === 'remove_groups') setSelected(previous => previous.filter(id => !ids.includes(id)));
    if (kind === 'add_groups') setAddIds([]);
    setPanel({ kind: 'members' });
  }

  async function testLatency(requested: string[]) {
    const current = latest.current.state;
    if (!isRuntimeReady(current)) throw new Error('内核未就绪或配置待应用，暂不能测延迟。');
    if (hasActiveJob(current) || jobRunning) throw new Error('已有检测任务，请等待完成或取消。');
    const ids = currentTargets(requested).filter(node => node.enabled && !nodeRelay(node, current.nodes)?.error).map(node => node.id);
    if (!ids.length) throw new Error('没有可检测的所选节点，请先启用节点并恢复中转依赖。');
    const next = await request<TestJob>('/api/tests', 'POST', { nodeIds: ids, kind: 'latency' });
    setTestRun({ job: next, ids, bootstrapAt: latest.current.updatedAt });
    setJobNotice('');
    notify('延迟检测已提交，共 ' + next.total + ' 个节点。', 'info');
    await refresh();
  }

  function confirm(kind: 'remove' | 'disable', ids: string[]) {
    showPanel({ kind, ids: [...ids], requestedCount: ids.length }, kind);
  }

  return <Modal title={group ? '分组节点 · ' + group.name : '分组已不存在'} description={group ? '仅显示当前分组成员；分组关系与节点全局状态分开管理。' : '此分组已被其他操作删除。当前选择已清理，不会再提交更改。'} onClose={onClose} busy={action.running} wide>
    <div className="group-node-manager" ref={body}>
      {!group ? <><div className="dialog-body"><EmptyState compact title="分组已不存在" description="关闭后查看最新分组列表。不会继续使用旧分组或节点选择。" /></div><div className="form-footer"><Button onClick={onClose} disabled={action.running}>关闭</Button></div></> : <>
        <div className="dialog-body">
          {panel.kind === 'members' && <>
            <dl className="group-node-summary" aria-label="分组节点统计">
              <div><dt>本组节点</dt><dd>{members.length}</dd></div>
              <div><dt>已启用</dt><dd>{enabledCount}</dd><small>{members.length - enabledCount} 已禁用</small></div>
              <div><dt>检测健康</dt><dd>{members.filter(node => node.status === 'healthy').length}</dd></div>
              <div><dt>检测异常</dt><dd>{members.filter(node => node.status === 'unhealthy').length}</dd></div>
              <div><dt>尚未检测</dt><dd>{members.filter(node => node.status === 'unknown').length}</dd></div>
            </dl>
            <p className="group-node-help">健康和延迟是最近实际检测结果，不代表持续在线。未知不等于故障。</p>
            <div className="group-node-toolbar"><h3>本组节点</h3><Button data-group-action="add" variant="primary" disabled={configurationBusy} onClick={() => showPanel({ kind: 'add' }, 'add')}><Plus size={17} aria-hidden="true" />添加已有节点</Button></div>
            <fieldset className="group-node-controls" disabled={action.running}>
              <legend className="sr-only">筛选和选择本组节点</legend>
              <div className="group-node-filters"><SearchInput value={query} onChange={value => { setQuery(value); setPage(1); }} label="搜索本组节点" placeholder="搜索名称、协议或地址…" /><select aria-label="筛选节点状态" value={status} onChange={event => { setStatus(event.target.value); setPage(1); }}><option value="all">全部状态</option><option value="enabled">已启用</option><option value="disabled">已禁用</option><option value="healthy">检测健康</option><option value="unhealthy">检测异常</option><option value="unknown">尚未检测</option></select>{(query || status !== 'all') && <Button small onClick={() => { setQuery(''); setStatus('all'); setPage(1); }}>重置筛选</Button>}</div>
              <div className="group-node-selection">
                <label><Check label="选择本页节点" disabled={!visible.length} checked={visible.length > 0 && visible.every(node => selectedIds.includes(node.id))} indeterminate={visible.some(node => selectedIds.includes(node.id)) && !visible.every(node => selectedIds.includes(node.id))} onChange={checked => setSelected(previous => selectScope(previous, visible.map(node => node.id), checked))} /><span>选择本页</span></label>
                <label><Check label="选择全部筛选结果" disabled={!filtered.length} checked={filtered.length > 0 && filtered.every(node => selectedIds.includes(node.id))} indeterminate={filtered.some(node => selectedIds.includes(node.id)) && !filtered.every(node => selectedIds.includes(node.id))} onChange={checked => setSelected(previous => selectScope(previous, filtered.map(node => node.id), checked))} /><span>全部筛选结果（{filtered.length}）</span></label>
                <span className="group-selection-count" role="status">已选 {selectedIds.length} 项</span>
                {selectedIds.length > 0 && <Button small variant="ghost" onClick={() => setSelected([])}>清空选择</Button>}
                {hiddenSelected > 0 && <span className="group-selection-hidden">含 {hiddenSelected} 个筛选外节点；批量操作仍包含这些选择</span>}
              </div>
              <div className="group-node-actions">
                <Button small disabled={!selectedNodes.some(node => !node.enabled) || configurationBusy} onClick={() => void action.execute(() => batch('enable', selectedIds))}><Play size={15} aria-hidden="true" />启用所选</Button>
                <Button small data-group-action="disable" disabled={!selectedNodes.some(node => node.enabled) || configurationBusy} onClick={() => confirm('disable', selectedNodes.filter(node => node.enabled).map(node => node.id))}><Power size={15} aria-hidden="true" />禁用所选</Button>
                <Button small disabled={!testable.length || !canTest} title={testHint} onClick={() => void action.execute(() => testLatency(selectedIds))}><Gauge size={15} aria-hidden="true" />检测所选延迟</Button>
                <Button small data-group-action="remove" variant="danger" disabled={!selectedIds.length || configurationBusy} onClick={() => confirm('remove', selectedIds)}><Trash size={15} aria-hidden="true" />从本组移除</Button>
              </div>
            </fieldset>
            <p className="group-global-warning">启用/禁用影响节点的所有分组、代理池、独享监听和中转依赖，是全局操作；从本组移除只改变当前分组关系。</p>
            {sourceCount > 0 && <Note><strong>{sourceCount} 个节点关联订阅</strong><p>订阅刷新或修改可能把节点重新加入本组。需要长期调整时，请修改对应订阅的分组；这里不判断当前分组关系来自哪个订阅。</p></Note>}
            {testable.length < selectedIds.length && <p className="group-node-help">延迟检测将跳过 {selectedIds.length - testable.length} 个已禁用或中转不可用的所选节点。</p>}
            {!runtimeReady && <p className="group-node-help">内核未就绪或配置待应用，暂不能测延迟。</p>}
            {anyJobRunning && <p className="group-node-help">检测任务运行期间暂停配置更改，可在“运行与设置”中查看或取消任务。</p>}
            {job && <p className="group-job-status" role="status">{jobRunning ? '延迟检测进行中' : job.status === 'cancelled' ? '延迟检测已取消' : '延迟检测已完成'} · {job.completed}/{job.total}<Button small variant="ghost" onClick={() => { onClose(); window.location.hash = '/settings'; }}>任务详情与取消</Button></p>}
            {jobNotice && <p className="group-node-help" role="status">{jobNotice}</p>}
            {filtered.length ? <TableRegion label="本组节点列表"><table className="data-table group-nodes-table"><thead><tr><th scope="col" className="group-node-select"><span className="sr-only">选择</span></th><th scope="col">节点 / 服务器</th><th scope="col">全局状态</th><th scope="col">最近延迟</th><th scope="col" className="align-right">操作</th></tr></thead><tbody>{visible.map(node => {
              const relay = nodeRelay(node, state.nodes);
              const testing = jobRunning && testRun?.ids.includes(node.id) && !job?.results.some(result => result.nodeId === node.id);
              return <tr key={node.id} aria-label={'本组节点 ' + node.name} className={selectedIds.includes(node.id) ? 'selected-row' : undefined}>
                <td className="group-node-select"><label className="group-node-check"><Check label={'选择节点 ' + node.name} checked={selectedIds.includes(node.id)} disabled={action.running} onChange={checked => setSelected(previous => selectScope(previous, [node.id], checked))} /></label></td>
                <td className="group-node-identity"><strong>{node.name}</strong><span className="cell-sub mono">{address(node.server, node.port)}</span><span className="cell-sub">{node.protocol.toUpperCase()}{node.udp && ' · UDP 配置兼容'}</span>{relay && <span className={'cell-sub' + (relay.error ? ' text-danger' : '')}>{relay.error ? '中转不可用：' + relay.error : '中转：' + relay.name}</span>}</td>
                <td className="group-node-status"><Badge tone={node.enabled ? 'teal' : 'neutral'}>{node.enabled ? '已启用' : '已禁用'}</Badge></td>
                <td className="group-node-latency"><span className={node.status === 'unhealthy' ? 'text-danger' : node.status === 'healthy' ? 'text-teal' : 'muted'} title={node.lastError || undefined}>{nodeLatencyText(node)}</span><span className="cell-sub">{formatDate(node.lastCheckedAt)}</span>{testing && <span className="cell-sub" role="status">检测中…</span>}</td>
                <td className="group-node-row-actions"><div className="row-actions"><IconButton label={'测延迟 ' + node.name} title={!node.enabled ? '节点已禁用' : relay?.error || testHint} disabled={!node.enabled || !!relay?.error || !canTest} onClick={() => void action.execute(() => testLatency([node.id]))}><Gauge size={17} aria-hidden="true" /></IconButton><IconButton className="danger-text" label={'从本组移除 ' + node.name} disabled={configurationBusy} onClick={() => confirm('remove', [node.id])}><Trash size={17} aria-hidden="true" /></IconButton></div></td>
              </tr>;
            })}</tbody></table></TableRegion> : <EmptyState compact title={members.length ? '没有符合条件的本组节点' : '这个分组还没有节点'} description={members.length ? '调整搜索或状态筛选。筛选外选择会保留，可点“清空选择”重置。' : '点击“添加已有节点”从全局节点中选择，不会复制节点或更改其他分组。'} />}
            <Pagination total={filtered.length} page={currentPage} pageSize={pageSize} onPage={setPage} />
          </>}
          {panel.kind === 'add' && <section className="group-add-panel" aria-label="添加已有节点">
            <h3>添加已有节点</h3><p className="group-node-help">只增加当前分组的手动关联，保留其他分组和订阅关系。不会复制节点，也不改变订阅节点的来源保留规则。</p>
            <fieldset className="group-node-controls" disabled={configurationBusy}><ChoiceList label="可添加节点" searchable bulk options={outside.map(node => ({ id: node.id, name: node.name, detail: node.protocol.toUpperCase() + ' · ' + address(node.server, node.port) + (!node.enabled ? ' · 已禁用' : '') + (node.groupIds.length ? ' · 已在其他分组' : ' · 未分组') }))} value={additionIds} onChange={setAddIds} empty="暂无组外节点，可先在节点管理中导入。" /></fieldset>
            <p className="group-node-help">全选覆盖当前搜索结果的所有项目，包含尚未显示的项目；搜索外选择保留。</p>
            <div className="group-panel-actions"><Button data-group-cancel disabled={action.running} onClick={backToMembers}>返回本组节点</Button><Button variant="primary" disabled={!additionIds.length || configurationBusy} busy={action.running} onClick={() => void action.execute(() => batch('add_groups', additionIds))}>添加到本组</Button></div>
          </section>}
          {(panel.kind === 'remove' || panel.kind === 'disable') && <section className="group-confirm-panel" role="region" aria-label={panel.kind === 'remove' ? '确认移出本组' : '确认全局禁用'}>
            <h3>{panel.kind === 'remove' ? '从本组移除所选节点？' : '全局禁用所选节点？'}</h3>
            <p>当前将处理 <strong>{pendingNodes.length}</strong> 个节点：{pendingNodes.slice(0, 5).map(node => node.name).join('、')}{pendingNodes.length > 5 && ' 等'}。</p>
            {panel.kind === 'remove' ? <Note tone="warning"><strong>仅移除「{group.name}」的分组关系，不删除全局节点，也不移除其他分组。</strong><p>引用本组的代理池会重新计算动态成员；手动纳入或其他分组仍可保留节点。旧排除规则不再匹配移出的节点，可能扩大对应池范围。空池拒绝连接，不会直连。</p><p>若节点关联订阅，订阅刷新或修改可能将节点重新加入本组；需要长期调整时，请修改对应订阅的分组。</p></Note> : <Note tone="warning"><strong>这是全局禁用，不是仅在本组停用。</strong><p>所选节点在其他分组、所有代理池和独享监听中都将停止接受新连接；依赖这些节点的中转线路也会不可用。已有连接不会主动断开。若只是调整分类，请返回并使用“从本组移除”。</p></Note>}
            {pendingNodes.length < panel.requestedCount && <p className="group-node-help" role="status">部分节点已被删除、移出本组或状态已变更，已清理失效选择；仅处理仍符合条件的节点。</p>}
            <div className="group-panel-actions"><Button data-group-cancel disabled={action.running} onClick={backToMembers}>取消</Button><Button variant="danger" disabled={!pendingNodes.length || configurationBusy} busy={action.running} onClick={() => void action.execute(() => batch(panel.kind === 'remove' ? 'remove_groups' : 'disable', pendingNodes.map(node => node.id)))}>{panel.kind === 'remove' ? '确认移出本组' : '确认全局禁用'}</Button></div>
          </section>}
          <ErrorBox message={action.error} />
        </div>
        {panel.kind === 'members' && <div className="form-footer"><span className="form-footer-note">更改自动保存与应用，以实际内核状态为准</span><Button onClick={onClose} disabled={action.running}>关闭</Button></div>}
      </>}
    </div>
  </Modal>;
}

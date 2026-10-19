import { useState } from 'react';
import { ArrowRight, FolderSimple, PencilSimple, Plus, Trash } from '@phosphor-icons/react';
import type { Group } from '../shared/types';
import { useAction, useApp } from './state';
import { Button, ConfirmDialog, EmptyState, ErrorBox, Field, FormFooter, IconButton, Modal, PageHeader, SearchInput, TableRegion, safeColor } from './ui';
import { GroupNodesDialog } from './GroupNodesDialog';
import { formatDate } from './utils';
import './groupStyles.css';

const colors = ['#0f766e', '#416ca1', '#8057a3', '#a87722', '#b15e52', '#637259'];
type GroupPanel = { kind: 'manage' | 'edit' | 'delete'; id: string } | { kind: 'new' } | null;

export function GroupsPage() {
  const { data, mutate } = useApp();
  const state = data!;
  const [query, setQuery] = useState('');
  const [panel, setPanel] = useState<GroupPanel>(null);
  const groups = state.groups.filter(group => group.name.toLowerCase().includes(query.trim().toLowerCase()));
  const activeGroup = panel && panel.kind !== 'new' ? state.groups.find(group => group.id === panel.id) : undefined;
  const groupedCount = state.nodes.filter(node => node.groupIds.length > 0).length;
  const closePanel = () => setPanel(null);

  return <>
    <PageHeader eyebrow="ASSETS / ORGANIZATION" title="分组管理" description="点击分组名称或节点数量，直接管理这个分组的节点。" actions={<Button variant="primary" onClick={() => setPanel({ kind: 'new' })}><Plus size={18} aria-hidden="true" />新建分组</Button>} />
    <div className="context-banner group-context">
      <FolderSimple size={27} weight="duotone" aria-hidden="true" />
      <div><strong>一个节点，可以属于多个分组</strong><p>已分组 {groupedCount} 个节点 · 未分组 {state.nodes.length - groupedCount} 个。移出分组不会删除全局节点；启用或禁用会影响节点的所有用途。</p></div>
      <span className="context-number">{state.groups.length}<small>个分组</small></span>
    </div>
    <section className="panel groups-panel">
      <div className="panel-topline"><div className="section-title"><h2>全部分组</h2><span className="count-pill">{state.groups.length}</span></div><SearchInput value={query} onChange={setQuery} label="搜索分组" placeholder="搜索分组名称…" /></div>
      {groups.length ? <TableRegion label="分组列表"><table className="data-table groups-table">
        <thead><tr><th scope="col">分组名称</th><th scope="col">关联节点</th><th scope="col">成员来源引用</th><th scope="col">旧排除规则引用</th><th scope="col">订阅来源</th><th scope="col">创建时间</th><th scope="col" className="align-right">操作</th></tr></thead>
        <tbody>{groups.map(group => {
          const members = state.nodes.filter(node => node.groupIds.includes(group.id));
          const enabledCount = members.filter(node => node.enabled).length;
          const healthyCount = members.filter(node => node.status === 'healthy').length;
          const includeCount = state.pools.filter(pool => pool.groupIds.includes(group.id)).length;
          const exclusionCount = state.pools.filter(pool => pool.excludeGroupIds.includes(group.id)).length;
          return <tr key={group.id}>
            <td data-label="分组名称"><div className="group-name-cell"><span className="group-folder" style={{ color: safeColor(group.color) }}><FolderSimple size={23} weight="duotone" aria-hidden="true" /></span><button type="button" className="node-name group-manager-link" aria-label={'管理分组 ' + group.name + ' 的节点'} onClick={() => setPanel({ kind: 'manage', id: group.id })}>{group.name}<ArrowRight size={14} aria-hidden="true" /></button></div></td>
            <td data-label="关联节点"><button type="button" className="group-count-link" aria-label={'管理分组 ' + group.name + ' 的 ' + members.length + ' 个节点'} onClick={() => setPanel({ kind: 'manage', id: group.id })}><strong className="metric-number">{members.length}</strong> 个节点</button><span className="cell-sub">{enabledCount} 已启用 · {healthyCount} 检测健康</span></td>
            <td data-label="成员来源引用" className="group-pool-includes">{includeCount} 个池</td>
            <td data-label="旧排除规则引用" className="group-pool-exclusions">{exclusionCount} 个池{exclusionCount > 0 && <span className="cell-sub">排除此组，非成员来源</span>}</td>
            <td data-label="订阅来源">{state.subscriptions.filter(source => source.groupIds.includes(group.id)).length} 个订阅</td>
            <td data-label="创建时间" className="muted">{formatDate(group.createdAt)}</td>
            <td data-label="操作"><div className="row-actions"><IconButton label={'编辑分组 ' + group.name} onClick={() => setPanel({ kind: 'edit', id: group.id })}><PencilSimple size={17} aria-hidden="true" /></IconButton><IconButton label={'删除分组 ' + group.name} onClick={() => setPanel({ kind: 'delete', id: group.id })}><Trash size={17} aria-hidden="true" /></IconButton></div></td>
          </tr>;
        })}</tbody>
      </table></TableRegion> : <EmptyState title={state.groups.length ? '没有匹配的分组' : '用分组整理你的节点'} description={state.groups.length ? '尝试其他分组名称，已有分组不会被更改。' : '创建一个分组，然后添加已有节点。同一节点可以保留在多个分组中。'} action={!state.groups.length && <Button onClick={() => setPanel({ kind: 'new' })}><Plus size={17} aria-hidden="true" />新建分组</Button>} />}
    </section>
    <p className="group-page-hint">“成员来源引用”只统计代理池的包含分组；旧排除规则单独统计。检测健康表示最近结果，不代表持续在线。</p>
    {panel?.kind === 'manage' && <GroupNodesDialog key={panel.id} groupId={panel.id} onClose={closePanel} />}
    {panel?.kind === 'new' && <GroupDialog onClose={closePanel} />}
    {panel?.kind === 'edit' && activeGroup && <GroupDialog key={activeGroup.id} group={activeGroup} onClose={closePanel} />}
    {panel?.kind === 'delete' && activeGroup && <ConfirmDialog onClose={closePanel} confirmation={{
      title: '删除「' + activeGroup.name + '」？',
      description: '仅删除分组和它的关联关系，不删除组内节点。引用此组的代理池和订阅将更新分组规则。',
      detail: '此操作不能撤销。清除包含或排除引用会重新计算代理池成员；全部匹配（交集）或旧排除规则的范围可能扩大。空池拒绝连接，不会回退直连。',
      label: '删除分组', danger: true,
      action: async () => { await mutate('/api/groups/' + encodeURIComponent(activeGroup.id), 'DELETE', undefined, '分组已删除'); },
    }} />}
    {panel && panel.kind !== 'new' && panel.kind !== 'manage' && !activeGroup && <Modal title="分组已不存在" description="此分组已被其他操作删除。不会再提交更改。" onClose={closePanel}><div className="dialog-body"><Button onClick={closePanel}>关闭</Button></div></Modal>}
  </>;
}

function GroupDialog({ group, onClose }: { group?: Group; onClose: () => void }) {
  const { mutate } = useApp();
  const action = useAction();
  const [name, setName] = useState(group?.name || '');
  const [color, setColor] = useState(group?.color || colors[0]);
  return <Modal title={group ? '编辑分组' : '新建分组'} description="名称用于查找和引用，颜色帮助快速区分。" busy={action.running} onClose={onClose}>
    <form onSubmit={event => { event.preventDefault(); void action.execute(async () => {
      if (!name.trim()) throw new Error('请输入分组名称。');
      await mutate('/api/groups' + (group ? '/' + encodeURIComponent(group.id) : ''), group ? 'PATCH' : 'POST', { name: name.trim(), color }, group ? '分组已更新' : '分组已创建');
      onClose();
    }); }}>
      <div className="dialog-body"><fieldset className="form-fields" disabled={action.running}>
        <Field label="分组名称" required><input autoFocus value={name} onChange={event => setName(event.target.value)} placeholder="例如：日常办公" required maxLength={100} /></Field>
        <fieldset className="color-field"><legend>标识颜色</legend><div className="color-options">{colors.map((value, index) => <button key={value} type="button" className={'color-swatch' + (color === value ? ' selected' : '')} style={{ backgroundColor: value }} aria-label={['青绿', '蓝色', '紫色', '琥珀', '赤陶', '橄榄'][index]} aria-pressed={color === value} onClick={() => setColor(value)} />)}<label className="custom-color">自定义<input type="color" aria-label="自定义分组颜色" value={safeColor(color)} onChange={event => setColor(event.target.value)} /></label></div></fieldset>
      </fieldset><ErrorBox message={action.error} /></div>
      <FormFooter busy={action.running} onClose={onClose} label={group ? '保存分组' : '创建分组'} />
    </form>
  </Modal>;
}

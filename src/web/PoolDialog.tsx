import { useState } from 'react';
import type { PoolCreateRequest, PoolCreateResult, PoolDraft, ProxyListener, ProxyPool } from '../shared/types';
import { useAction, useApp } from './state';
import { Button, ChoiceList, ErrorBox, Field, FormFooter, GroupPicker, Modal, Note, Toggle } from './ui';
import { CredentialsEditor, serializedUsers, type UserDraft } from './ListenerForms';
import { address, checkHttpUrl, isIP, isLoopback, poolMembers, strategyLabels } from './utils';
import { changePoolSelection, constrainPoolSelection, excludedByLegacyGroup, poolSelectionScope, selectedPoolNodes, withoutDeletedPoolNodes } from './poolSelection';

const strategyHelp:Record<ProxyPool['strategy'],string> = {
  'url-test':'根据代理探测选择较低延迟的节点，并用容差减少频繁切换。',
  'round-robin':'按轮询顺序分配新连接，分散出口负载。',
  'consistent-hashing':'以哈希键稳定分配连接，相同键尽量使用同一出口。',
  'sticky-sessions':'为同一会话维持相对稳定的出口；原生缓存不是永久绑定。',
};

export function PoolDialog({pool,onClose}:{pool?:ProxyPool;onClose:()=>void}) {
  const {data,mutate} = useApp(); const state = data!; const action = useAction();
  const [draft,setDraft] = useState<PoolDraft>(() => pool ? {
    name:pool.name,strategy:pool.strategy,hashKey:pool.hashKey,nodeIds:pool.nodeIds,groupIds:pool.groupIds,
    groupMode:pool.groupMode,excludeNodeIds:pool.excludeNodeIds,excludeGroupIds:pool.excludeGroupIds,
    enabled:pool.enabled,udp:pool.udp,testUrl:pool.testUrl,intervalSeconds:pool.intervalSeconds,
    timeoutMs:pool.timeoutMs,toleranceMs:pool.toleranceMs,
  } : {name:'',strategy:'url-test',hashKey:'',nodeIds:[],groupIds:[],groupMode:'any',excludeNodeIds:[],excludeGroupIds:[],enabled:true,udp:false,testUrl:state.settings.testUrl,intervalSeconds:300,timeoutMs:8000,toleranceMs:50});
  const [scopeNotice,setScopeNotice] = useState('');
  const [host,setHost] = useState('127.0.0.1');
  const [automaticPort,setAutomaticPort] = useState(true);
  const [port,setPort] = useState(state.settings.portRangeStart);
  const [protocol,setProtocol] = useState<ProxyListener['protocol']>('mixed');
  const [listenerUdp,setListenerUdp] = useState(false);
  const [users,setUsers] = useState<UserDraft[]>([]);
  const canHash = draft.strategy === 'consistent-hashing' || draft.strategy === 'sticky-sessions';
  const scope = poolSelectionScope(state.nodes,draft);
  const selected = selectedPoolNodes(scope,draft).map(node => node.id);
  const scopeIds = new Set(scope.map(node => node.id));
  const existingIds = new Set(state.nodes.map(node => node.id));
  const deletedReferenceCount = new Set([...draft.nodeIds,...draft.excludeNodeIds].filter(id => !existingIds.has(id))).size;
  const outside = draft.nodeIds.filter(id => !scopeIds.has(id));
  const members = poolMembers(state.nodes,draft);
  const nodes = scope.map(node => ({id:node.id,name:node.name,disabled:excludedByLegacyGroup(node,draft),detail:
    node.protocol.toUpperCase() + ' · ' + address(node.server,node.port)
    + (!node.enabled ? ' · 已禁用' : '') + (node.dialerError ? ' · 中转不可用' : '')
    + (node.udp ? ' · UDP' : '') + (excludedByLegacyGroup(node,draft) ? ' · 受旧分组排除规则限制' : '')}));
  function update<K extends keyof PoolDraft>(key:K,value:PoolDraft[K]) {setDraft(previous => ({...previous,[key]:value}));}
  function updateScope(patch:Partial<Pick<PoolDraft,'groupIds'|'groupMode'|'udp'>>) {
    const next = constrainPoolSelection(state.nodes,{...draft,...patch});
    const removed = draft.nodeIds.length - next.nodeIds.length;
    setScopeNotice(removed ? '已从本次草稿移除 ' + removed + ' 个范围外手动节点；取消对话框不会保存此更改。' : '');
    setDraft(next);
    if (patch.udp === false) setListenerUdp(false);
  }
  async function save() {
    if (!draft.name.trim()) throw new Error('请输入代理池名称。');
    if (!checkHttpUrl(draft.testUrl)) throw new Error('健康检查 URL 必须是完整的 HTTP(S) 地址。');
    const savedPool = {...withoutDeletedPoolNodes(state.nodes,draft),name:draft.name.trim(),hashKey:canHash ? draft.hashKey : ''} satisfies PoolDraft;
    if (pool) {
      await mutate('/api/pools/' + encodeURIComponent(pool.id),'PUT',savedPool,'代理池已更新');
    } else {
      if (!isIP(host)) throw new Error('监听地址必须是有效的 IPv4 或 IPv6 地址，不支持域名和方括号。');
      if (!automaticPort && (!Number.isInteger(port) || port < 1024 || port > 65535)) throw new Error('端口必须为 1024–65535 的整数。');
      const credentials = serializedUsers(users,[]);
      if (!isLoopback(host) && !credentials.length) throw new Error('非回环地址必须设置至少一组用户名和密码。');
      if (listenerUdp && (!isLoopback(host) || !draft.udp || protocol === 'http')) throw new Error('UDP 需要兼容代理池、回环地址及 mixed / SOCKS5 监听。');
      if (savedPool.hashKey === 'in-user' && (!credentials.length || listenerUdp)) throw new Error('按用户名哈希需要配置认证账号，且不能启用 UDP。');
      const body:PoolCreateRequest = {pool:savedPool,listener:{host:host.trim(),port:automaticPort ? 0 : port,protocol,udp:listenerUdp,users:credentials,enabled:true}};
      await mutate<PoolCreateResult>('/api/pools/with-listener','POST',body,'代理池与监听已一并创建');
    }
    onClose();
  }
  return <Modal title={pool ? '编辑代理池' : '新建代理池'} description={pool ? '调整成员和策略。移除节点只影响此池，关联监听保持原配置。' : '选择出口节点并分配共享监听，一次校验后统一保存与应用。'} onClose={onClose} busy={action.running} wide>
    <form onSubmit={event => {event.preventDefault();void action.execute(save);}}><div className="dialog-body pool-dialog"><fieldset className="form-fields" disabled={action.running}>
      <div className="form-grid"><Field label="代理池名称" required><input autoFocus required maxLength={100} value={draft.name} onChange={event => update('name',event.target.value)} placeholder="例如：日常出口" /></Field>
        <Field label="调度策略"><select value={draft.strategy} onChange={event => {const strategy = event.target.value as ProxyPool['strategy'];setDraft(previous => ({...previous,strategy,hashKey:strategy === 'url-test' || strategy === 'round-robin' ? '' : previous.hashKey}));}}>{Object.entries(strategyLabels).map(([key,label]) => <option key={key} value={key}>{label} · {key}</option>)}</select></Field></div>
      <p className="field-hint">{strategyHelp[draft.strategy]}</p>
      {canHash && <Field label="哈希键" hint="按用户名哈希需要监听配置认证账号；UDP 不支持此模式。"><select value={draft.hashKey} onChange={event => {const value=event.target.value as ''|'in-user';update('hashKey',value);if(value==='in-user')setListenerUdp(false);}}><option value="">内核默认</option><option value="in-user">认证用户名 · in-user</option></select></Field>}
      <div className="form-section-heading"><span>01</span><h3>选择成员</h3></div>
      <div className="form-grid"><GroupPicker groups={state.groups} value={draft.groupIds} onChange={value => updateScope({groupIds:value})} label="包含分组" bulk />
        <div className="form-fields"><Field label="分组匹配关系"><select value={draft.groupMode} onChange={event => updateScope({groupMode:event.target.value as 'any'|'all'})}><option value="any">任意一个分组（并集）</option><option value="all">全部所选分组（交集）</option></select></Field><Toggle label="仅选择 UDP 兼容节点" checked={draft.udp} onChange={value => updateScope({udp:value})} description="仅表示配置兼容，不代表已经验证 UDP 连通。" /></div></div>
      <p className="field-hint pool-selection-hint">{draft.groupIds.length ? '仅显示所选分组' + (draft.groupMode === 'all' ? '交集' : '并集') + '内的节点。分组成员默认勾选，取消勾选会从本池排除；订阅新增的组内节点仍会自动纳入。' : '未选择分组时可从所有节点中手动勾选。选择分组后，下方列表将限定在分组范围内。'}</p>
      {scopeNotice && <p className="scope-notice" role="status">{scopeNotice}</p>}
      {deletedReferenceCount > 0 && <Note>有 {deletedReferenceCount} 个引用的节点已被删除，保存时会清理失效引用；仍存在的范围外节点不受影响。</Note>}
      {outside.length > 0 && <Note tone="warning">有 {outside.length} 个旧手动节点在当前范围外，单纯修改其他设置会保留。修改分组或兼容范围时会清理这些选择。<Button small onClick={() => updateScope({})}>仅保留当前范围</Button></Note>}
      {draft.excludeGroupIds.length > 0 && <Note tone="warning"><strong>旧分组排除规则仍生效</strong><p>{draft.excludeGroupIds.map(id => state.groups.find(group => group.id === id)?.name || '已删除分组').join('、')}。为避免编辑后意外扩大出口范围，旧规则会保留；新建代理池不再设置排除分组。</p><Button small onClick={() => update('excludeGroupIds',[])}>清除旧规则（保存后生效）</Button></Note>}
      <ChoiceList label="包含节点" searchable bulk options={nodes} value={selected} onChange={ids => setDraft(previous => changePoolSelection(previous,scope,ids))} empty={state.nodes.length ? '当前分组或 UDP 条件下没有节点，请调整条件。' : '暂无节点，请先导入。'} />
      <p className="field-hint">已勾选表示成员配置；禁用、中转异常及 UDP 不兼容节点不会转发。取消勾选不删除总节点。{draft.excludeNodeIds.length > 0 && ' 本池保留 ' + draft.excludeNodeIds.length + ' 个节点排除记录，重新勾选可恢复。'}</p>
      <div className="member-preview" aria-live="polite"><div><strong>{members.length}</strong><span>个当前可转发成员</span></div><p>{members.length ? members.slice(0,5).map(node => node.name).join('、') + (members.length > 5 ? ' 等' : '') : '当前没有可转发成员；连接将被拒绝，不会自动直连。'}</p></div>
      {!pool && <><div className="form-section-heading"><span>02</span><h3>分配监听端口</h3></div>
        <Note>监听与代理池一并创建，校验失败不会留下半成品。默认仅供本机使用；未启用 UDP 时只监听 TCP。</Note>
        <div className="form-grid"><Field label="监听协议"><select value={protocol} onChange={event => {const value=event.target.value as ProxyListener['protocol'];setProtocol(value);if(value==='http')setListenerUdp(false);}}><option value="mixed">Mixed · HTTP + SOCKS5</option><option value="http">HTTP</option><option value="socks">SOCKS5</option></select></Field><Toggle label="UDP 转发" checked={listenerUdp} onChange={setListenerUdp} disabled={protocol==='http' || !draft.udp || draft.hashKey==='in-user'} description={protocol==='http' ? 'HTTP 监听不支持 UDP' : draft.hashKey==='in-user' ? '按用户名哈希不能启用 UDP' : !draft.udp ? '请先开启“仅选择 UDP 兼容节点”' : '仅允许回环地址，不受用户名密码保护'} /></div>
        <Field label="监听地址" required hint="IPv4 / IPv6 地址。默认回环地址，不会向局域网或公网开放。"><input required value={host} onChange={event => setHost(event.target.value)} className="mono" spellCheck={false} /></Field>
        <Toggle label="自动分配空闲端口" checked={automaticPort} onChange={setAutomaticPort} description={'在 ' + state.settings.portRangeStart + '–' + state.settings.portRangeEnd + ' 范围内选择，跳过已有监听和占用端口。'} />
        {!automaticPort && <Field label="监听端口" required><input type="number" required min={1024} max={65535} value={port} onChange={event => setPort(Number(event.target.value))} /></Field>}
        {!isLoopback(host) && <Note tone="warning">非回环监听可能允许其他设备访问，必须设置认证并在主机防火墙限制来源。普通代理认证不提供链路加密。</Note>}
        {listenerUdp && <Note tone="warning">原生 UDP 数据包不执行用户名密码认证，仅允许回环地址；同机其他进程仍可发送 UDP。</Note>}
        <CredentialsEditor users={users} onChange={setUsers} />
      </>}
      <details className="pool-advanced"><summary>健康检查与运行设置</summary><div className="form-fields">
        <Field label="检查 URL" required><input type="url" value={draft.testUrl} onChange={event => update('testUrl',event.target.value)} required className="mono" /></Field>
        <div className="form-grid form-grid--three"><Field label="间隔（秒）"><input type="number" min={10} max={86400} required value={draft.intervalSeconds} onChange={event => update('intervalSeconds',Number(event.target.value))} /></Field><Field label="超时（毫秒）"><input type="number" min={500} max={30000} required value={draft.timeoutMs} onChange={event => update('timeoutMs',Number(event.target.value))} /></Field><Field label="切换容差（毫秒）" hint="主要用于最低延迟策略。"><input type="number" min={0} max={10000} required value={draft.toleranceMs} onChange={event => update('toleranceMs',Number(event.target.value))} /></Field></div>
        <Toggle label="启用代理池" checked={draft.enabled} onChange={value => update('enabled',value)} description="关闭后该池不提供候选出口；已有监听配置会保留。" />
      </div></details>
    </fieldset><ErrorBox message={action.error} /></div><FormFooter onClose={onClose} busy={action.running} label={pool ? '保存代理池' : '创建代理池与监听'}>{pool ? '保存后自动应用配置' : '一次保存池与监听，实际状态以应用结果为准'}</FormFooter></form>
  </Modal>;
}

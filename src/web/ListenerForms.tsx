import { useState } from 'react';
import { Key, Plus, Trash } from '@phosphor-icons/react';
import type { Credential, ProxyListener } from '../shared/types';
import { useAction, useApp } from './state';
import { Button, ErrorBox, Field, FormFooter, IconButton, Modal, Note, Toggle } from './ui';
import { isIP, isLoopback, poolMembers } from './utils';

export type UserDraft = {key:string; username:string; password:string};
let credentialRow = 0;
function nextCredentialKey() { return 'credential-' + (++credentialRow); }
function draftUsers(users:Credential[]):UserDraft[] { return users.map(user => ({key:nextCredentialKey(),username:user.username,password:''})); }
export function serializedUsers(users:UserDraft[], original:Credential[]):Credential[] {
  const seen = new Set<string>();
  return users.map(user => {
    const username = user.username.trim();
    if (!username) throw new Error('每组认证凭据都需要用户名，请补全或移除空白项。');
    if (new TextEncoder().encode(username).length > 255 || new TextEncoder().encode(user.password).length > 255) throw new Error('用户名和密码不能超过 255 字节。');
    if (username.includes(':') || /[\x00-\x1f\x7f]/.test(username + user.password)) throw new Error('凭据不能包含控制字符，用户名不能包含冒号。');
    if (seen.has(username)) throw new Error('认证用户名不能重复：' + username);
    seen.add(username);
    const preserved = original.some(item => item.username === username && item.hasPassword);
    if (!user.password && !preserved) throw new Error('新用户名「' + username + '」需要设置密码。');
    return {username, ...(user.password ? {password:user.password} : {})};
  });
}
export function CredentialsEditor({users,onChange,original = []}:{users:UserDraft[];onChange:(users:UserDraft[])=>void;original?:Credential[]}) {
  return <section className="credentials-section"><div className="section-heading"><div><h3><Key size={17} />访问认证</h3><p>可添加多组账号；非回环地址必须启用认证。</p></div><Button small onClick={() => onChange([...users,{key:nextCredentialKey(),username:'',password:''}])}><Plus size={15} />添加账号</Button></div>{users.length ? <div className="credentials-list">{users.map((user,index) => { const preserved = original.some(item => item.username === user.username.trim() && item.hasPassword); return <div className="credential-row" key={user.key}><Field label={'用户名 ' + (index + 1)} required><input value={user.username} onChange={event => onChange(users.map(item => item.key === user.key ? {...item,username:event.target.value} : item))} autoComplete="off" maxLength={128} required /></Field><Field label={'密码 ' + (index + 1)} required={!preserved} hint={preserved ? '留空保留此用户名的原密码' : '新账号必须设置密码'}><input type="password" value={user.password} onChange={event => onChange(users.map(item => item.key === user.key ? {...item,password:event.target.value} : item))} placeholder={preserved ? '保持现有密码' : '设置访问密码'} autoComplete="new-password" required={!preserved} maxLength={255} /></Field><IconButton label={'移除第 ' + (index + 1) + ' 组认证'} onClick={() => onChange(users.filter(item => item.key !== user.key))}><Trash size={17} /></IconButton></div>; })}</div> : <div className="quiet-empty">未设置账号。仅允许绑定回环地址，避免形成开放代理。</div>}</section>;
}
export function ProtocolFields({protocol,udp,setProtocol,setUdp}:{protocol:ProxyListener['protocol'];udp:boolean;setProtocol:(value:ProxyListener['protocol'])=>void;setUdp:(value:boolean)=>void}) {
  return <div className="form-grid"><Field label="监听协议"><select value={protocol} onChange={event => { const value = event.target.value as ProxyListener['protocol']; setProtocol(value); if (value === 'http') setUdp(false); }}><option value="mixed">Mixed · HTTP + SOCKS5</option><option value="http">HTTP</option><option value="socks">SOCKS5</option></select></Field><Toggle label="UDP 转发" checked={udp} onChange={setUdp} disabled={protocol === 'http'} description={protocol === 'http' ? 'HTTP 监听不支持 UDP' : '需要 UDP 兼容目标，且只能绑定回环地址'} /></div>;
}
export function ListenerDialog({listener,onClose,initialNodeId,initialPoolId}:{listener?:ProxyListener;onClose:()=>void;initialNodeId?:string;initialPoolId?:string}) {
  const {data,mutate} = useApp(); const state = data!;
  const [name,setName] = useState(listener?.name || (initialPoolId ? (state.pools.find(pool => pool.id === initialPoolId)?.name || '代理池') + ' · 共享入口' : ''));
  const [kind,setKind] = useState<ProxyListener['kind']>(listener?.kind || (initialPoolId ? 'pool' : initialNodeId ? 'node' : state.pools.length ? 'pool' : 'node'));
  const [targetId,setTargetId] = useState(listener?.targetId || initialPoolId || initialNodeId || '');
  const [protocol,setProtocol] = useState<ProxyListener['protocol']>(listener?.protocol || 'mixed');
  const [host,setHost] = useState(listener?.host || '127.0.0.1');
  const [port,setPort] = useState(listener?.port || state.settings.portRangeStart);
  const [udp,setUdp] = useState(listener?.udp || false);
  const [enabled,setEnabled] = useState(listener?.enabled ?? true);
  const [users,setUsers] = useState<UserDraft[]>(draftUsers(listener?.users || []));
  const action = useAction();
  const targets = kind === 'node' ? state.nodes : state.pools;
  const target = targets.find(item => item.id === targetId);
  const emptyPool = kind === 'pool' && target && poolMembers(state.nodes,state.pools.find(p => p.id === target.id)!).length === 0;
  async function save() { await action.execute(async () => {
    if (!name.trim()) throw new Error('请输入监听名称。');
    if (!target && (enabled || !targetId)) throw new Error('请选择有效目标；失效目标仅可保存为禁用状态。');
    if (udp && target && !target.udp) throw new Error('当前目标不支持 UDP，请关闭 UDP 或更换目标。');
    const credentials = serializedUsers(users,listener?.users || []);
    if (!isLoopback(host) && !credentials.length) throw new Error('非回环地址必须设置至少一组用户名和密码。');
    if (udp && !isLoopback(host)) throw new Error('Mihomo 原生 UDP 不受用户名密码保护，UDP 仅允许回环地址。');
    if (kind === 'pool') { const pool = state.pools.find(item => item.id === targetId); if (pool?.hashKey === 'in-user') { if (udp) throw new Error('UDP 不支持认证用户名哈希，请关闭 UDP。'); if (enabled && !credentials.length) throw new Error('按用户名哈希的代理池需要监听配置认证凭据。'); } }
    if (!isIP(host)) throw new Error('监听地址必须是有效的 IPv4 或 IPv6 地址，不支持域名和方括号。');
    if (!Number.isInteger(port) || port < 1024 || port > 65535) throw new Error('端口必须为 1024–65535 的整数。');
    await mutate('/api/listeners' + (listener ? '/' + encodeURIComponent(listener.id) : ''),listener ? 'PUT' : 'POST',{name:name.trim(),kind,targetId,protocol,host:host.trim(),port,udp,enabled,users:credentials},listener ? '监听配置已保存' : '监听配置已创建');
    onClose();
  }); }
  return <Modal title={listener ? '编辑监听端口' : '新建监听端口'} description="将一个本地入口绑定到节点或代理池。保存后由内核验证并应用。" onClose={onClose} busy={action.running} wide><form onSubmit={event => {event.preventDefault();void save();}}><div className="dialog-body"><fieldset disabled={action.running} className="form-fields"><Field label="监听名称" required><input value={name} onChange={event => setName(event.target.value)} required maxLength={100} placeholder="例如：办公应用入口" autoFocus /></Field><div className="form-grid"><Field label="目标类型"><select value={kind} onChange={event => {setKind(event.target.value as ProxyListener['kind']);setTargetId('');}}><option value="pool">代理池</option><option value="node">独立节点</option></select></Field><Field label="绑定目标" required><select required value={targetId} onChange={event => setTargetId(event.target.value)}><option value="">选择{kind === 'pool' ? '代理池' : '节点'}</option>{targetId && !target && <option value={targetId}>目标已失效 · 请重新选择</option>}{targets.map(item => <option key={item.id} value={item.id}>{item.name}{!item.enabled ? '（已禁用）' : ''}{item.udp ? ' · UDP' : ''}</option>)}</select></Field></div>{target && !target.enabled && <Note tone="warning">目标当前已禁用。保存监听不会自动启用目标，流量将被拒绝。</Note>}{emptyPool && <Note tone="warning">该代理池当前没有可用成员，连接将被拒绝，不会自动直连。</Note>}<ProtocolFields protocol={protocol} udp={udp} setProtocol={setProtocol} setUdp={setUdp} /><div className="form-grid"><Field label="监听地址" required hint="填写 IPv4 / IPv6 地址，不支持域名。默认只允许本机连接。"><input value={host} onChange={event => setHost(event.target.value)} required spellCheck={false} className="mono" placeholder="127.0.0.1" /></Field><Field label="端口" required><input type="number" value={port} onChange={event => setPort(Number(event.target.value))} min={1024} max={65535} required /></Field></div>{!isLoopback(host) && <Note tone="warning">此地址可能允许其他设备访问。必须设置认证，并在主机防火墙限制来源；普通 HTTP / SOCKS 认证本身不提供链路加密。</Note>}{udp && <Note tone="warning">原生 UDP 不受用户名密码保护，仅允许回环地址；不支持按认证用户名哈希。</Note>}<CredentialsEditor users={users} onChange={setUsers} original={listener?.users} /><Toggle label="启用此监听" checked={enabled} onChange={setEnabled} description="配置启用不等于实际监听，请以运行状态为准。" /></fieldset><ErrorBox message={action.error} /></div><FormFooter onClose={onClose} busy={action.running} label={listener ? '保存监听' : '创建监听'} /></form></Modal>;
}
export function BatchListenerDialog({nodeIds,onClose}:{nodeIds:string[];onClose:()=>void}) {
  const {data,mutate} = useApp();
  const [protocol,setProtocol] = useState<ProxyListener['protocol']>('mixed');
  const [host,setHost] = useState('127.0.0.1');
  const [udp,setUdp] = useState(false);
  const [users,setUsers] = useState<UserDraft[]>([]);
  const action = useAction();
  const nodes = data!.nodes.filter(node => nodeIds.includes(node.id));
  return <Modal title="批量分配独立端口" description={'为已选 ' + nodes.length + ' 个节点分别创建独立监听，整批校验后一次应用。'} onClose={onClose} busy={action.running} wide><form onSubmit={event => {event.preventDefault();void action.execute(async () => {
    if (!nodes.length || nodes.length !== nodeIds.length) throw new Error('选中的节点发生变化，请关闭对话框重新选择。');
    if (udp && nodes.some(node => !node.udp)) throw new Error('部分选中节点不支持 UDP，请关闭 UDP 或重新选择。');
    if (udp && !isLoopback(host)) throw new Error('Mihomo 原生 UDP 不受用户名密码保护，UDP 仅允许回环地址。');
    if (!isIP(host)) throw new Error('监听地址必须是有效的 IPv4 或 IPv6 地址。');
    const credentials = serializedUsers(users,[]);
    if (!isLoopback(host) && !credentials.length) throw new Error('非回环地址必须设置访问认证。');
    const listeners = await mutate<ProxyListener[]>('/api/listeners/batch','POST',{nodeIds,protocol,host:host.trim(),udp,users:credentials},'独立监听配置已创建');
    if (!listeners.length) throw new Error('服务未返回已创建的监听，请刷新核实。');
    onClose();
  });}}><div className="dialog-body"><fieldset disabled={action.running} className="form-fields"><Note>服务将在 <strong className="mono">{data!.settings.portRangeStart}–{data!.settings.portRangeEnd}</strong> 内自动分配空闲端口。不会修改节点原始地址，也不会覆盖已有监听。</Note><ProtocolFields protocol={protocol} udp={udp} setProtocol={setProtocol} setUdp={setUdp} /><Field label="监听地址" required><input value={host} onChange={event => setHost(event.target.value)} required className="mono" /></Field>{udp && <Note tone="warning">原生 UDP 不受用户名密码保护，仅允许绑定回环地址。</Note>}<CredentialsEditor users={users} onChange={setUsers} />{nodes.some(node => !node.enabled) && <Note tone="warning">选中列表包含禁用节点，其监听不会转发流量。请先启用对应节点。</Note>}{!isLoopback(host) && <Note tone="warning">非回环监听会暴露到网络，请配置认证与防火墙访问控制。</Note>}</fieldset><ErrorBox message={action.error} /></div><FormFooter onClose={onClose} busy={action.running} label={'创建 ' + nodes.length + ' 个独立监听'} /></form></Modal>;
}

import type { ProxyNode, ProxyPool, RuntimeStatus } from '../shared/types';
export const strategyLabels: Record<ProxyPool['strategy'],string> = { 'url-test':'最低延迟', 'round-robin':'轮询均衡', 'consistent-hashing':'一致性哈希', 'sticky-sessions':'粘性会话' };
export const protocolLabels: Record<string,string> = { mixed:'HTTP / SOCKS5', http:'HTTP', socks:'SOCKS5' };
export const protocolOptions = ['vless','vmess','trojan','ss','hysteria','hysteria2','tuic','ssh','socks5','http'] as const;
export function formatDate(value: string, full = false): string {
  if (!value || value.startsWith('0001-')) return '尚无记录';
  const date = new Date(value); if (Number.isNaN(date.getTime())) return '时间未知';
  return date.toLocaleString('zh-CN', full ? {year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',second:'2-digit',hour12:false} : {month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false});
}
export function address(host:string, port:number):string { return (host.includes(':') && !host.startsWith('[') ? '[' + host + ']' : host) + ':' + port; }
export function poolMembers(nodes:ProxyNode[], pool:Pick<ProxyPool,'enabled'|'udp'|'nodeIds'|'groupIds'|'groupMode'|'excludeNodeIds'|'excludeGroupIds'>):ProxyNode[] {
  if (!pool.enabled) return [];
  return nodes.filter(node => node.enabled && !node.dialerError && (!pool.udp || node.udp) && !pool.excludeNodeIds.includes(node.id) && !node.groupIds.some(id => pool.excludeGroupIds.includes(id)) && (pool.nodeIds.includes(node.id) || (pool.groupIds.length > 0 && (pool.groupMode === 'all' ? pool.groupIds.every(id => node.groupIds.includes(id)) : pool.groupIds.some(id => node.groupIds.includes(id))))));
}
export function runtimeLabel(runtime:RuntimeStatus):string {
  if (runtime.applying) return '正在应用';
  if (!runtime.available) return '内核不可用';
  if (!runtime.running) return '内核未运行';
  if (runtime.dirty) return '配置待应用';
  return '内核运行中';
}
export function runtimeTone(runtime:RuntimeStatus):'success'|'warning'|'neutral' { return runtime.running && !runtime.dirty && !runtime.applying ? 'success' : runtime.dirty || runtime.lastError ? 'warning' : 'neutral'; }
export function isIP(host:string):boolean {
  const value = host.trim();
  if (/^(?:\d{1,3}\.){3}\d{1,3}$/.test(value)) return value.split('.').every(part => Number(part) <= 255 && String(Number(part)) === part);
  if (!value.includes(':') || !/^[a-fA-F0-9:.]+$/.test(value)) return false;
  try { return new URL('http://[' + value + ']/').hostname.startsWith('['); } catch { return false; }
}
export function isLoopback(host:string):boolean {
  const value = host.trim().toLowerCase();
  if (!isIP(value)) return false;
  if (value.startsWith('127.')) return true;
  if (!value.includes(':')) return false;
  const normalized = new URL('http://[' + value + ']/').hostname.slice(1,-1);
  return normalized === '::1' || /^::ffff:7f[0-9a-f]{2}:[0-9a-f]{1,4}$/.test(normalized);
}
export function safeFilename(prefix:string, extension:string) { return prefix + '-' + new Date().toISOString().slice(0,10) + '.' + extension; }
export function checkHttpUrl(value:string):boolean { try { const url = new URL(value); return ['http:','https:'].includes(url.protocol) && !url.username && !url.password && !url.hash; } catch { return false; } }

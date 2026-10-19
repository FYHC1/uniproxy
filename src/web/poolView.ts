import type { ProxyListener, ProxyNode, RuntimeStatus } from '../shared/types';

export function poolNodeMetrics(nodes:ProxyNode[]) {
  const measured=nodes.filter(node=>node.status==='healthy' && node.latencyMs!==null && Number.isFinite(node.latencyMs)).map(node=>node.latencyMs!).sort((a,b)=>a-b);
  const middle=Math.floor(measured.length/2);
  return {healthy:nodes.filter(node=>node.status==='healthy').length,unhealthy:nodes.filter(node=>node.status==='unhealthy').length,unknown:nodes.filter(node=>node.status==='unknown').length,
    medianMs:measured.length ? Math.round(measured.length%2 ? measured[middle] : (measured[middle-1]+measured[middle])/2) : null,
    lastCheckedAt:nodes.map(node=>node.lastCheckedAt).filter(value=>!!value && !value.startsWith('0001-')).sort().at(-1) || ''};
}


export type ListenerStateTone = 'teal'|'warning'|'neutral'|'danger';

/** Runtime listener state that never claims a saved-but-unapplied address is live.
 * A stale applied revision or pending change means the previous runtime snapshot
 * describes different configuration, so "监听中" would be a false claim.
 */
export function listenerState(listener:Pick<ProxyListener,'id'|'enabled'>, runtime:RuntimeStatus, revision?:number):{label:string;tone:ListenerStateTone} {
  if (runtime.applying) return {label:'正在应用',tone:'warning'};
  if (runtime.dirty) return {label:'配置待应用',tone:'warning'};
  if (revision !== undefined && runtime.appliedRevision !== revision) return {label:'状态待同步',tone:'warning'};
  if (!listener.enabled) return {label:'已停用',tone:'neutral'};
  if (!runtime.available) return {label:'内核不可用',tone:'warning'};
  if (!runtime.running) return {label:'内核未运行',tone:'neutral'};
  const status = runtime.listeners.find(item => item.id === listener.id);
  if (status?.listening && !status.error) return {label:'监听中',tone:'teal'};
  return {label:'未监听',tone:status?.error ? 'danger' : 'neutral'};
}

/** True only when this listener is confirmed listening for the applied revision. */
export function listenerIsLive(listener:Pick<ProxyListener,'id'|'enabled'>, runtime:RuntimeStatus, revision:number):boolean {
  return listenerState(listener,runtime,revision).label === '监听中';
}

export const poolListenerState = listenerState;

export function nodeLatencyText(node:ProxyNode):string {
  if (node.status === 'unhealthy') return '检测失败';
  if (node.latencyMs !== null && node.status === 'healthy') return node.latencyMs + ' ms';
  return '尚未检测';
}

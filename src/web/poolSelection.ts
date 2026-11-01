import type { ProxyNode, ProxyPool } from '../shared/types';

export type PoolSelection = Pick<ProxyPool, 'nodeIds'|'groupIds'|'groupMode'|'excludeNodeIds'|'excludeGroupIds'|'udp'>;

export function matchesPoolGroups(node:ProxyNode, pool:Pick<ProxyPool,'groupIds'|'groupMode'>):boolean {
  return pool.groupIds.length > 0 && (pool.groupMode === 'all'
    ? pool.groupIds.every(id => node.groupIds.includes(id))
    : pool.groupIds.some(id => node.groupIds.includes(id)));
}

export function excludedByLegacyGroup(node:ProxyNode, pool:Pick<ProxyPool,'excludeGroupIds'>):boolean {
  return node.groupIds.some(id => pool.excludeGroupIds.includes(id));
}

// Selection reflects saved intent, not health/enable state. Runtime membership still uses poolMembers.
export function selectedPoolNodes(nodes:ProxyNode[], pool:PoolSelection):ProxyNode[] {
  return nodes.filter(node => (pool.nodeIds.includes(node.id) || matchesPoolGroups(node,pool))
    && !pool.excludeNodeIds.includes(node.id) && !excludedByLegacyGroup(node,pool));
}

export function poolSelectionScope(nodes:ProxyNode[], pool:PoolSelection):ProxyNode[] {
  return nodes.filter(node => (!pool.groupIds.length || matchesPoolGroups(node,pool)) && (!pool.udp || node.udp));
}

// Drop references only after authoritative bootstrap confirms the node was deleted, never for UI filters.
export function withoutDeletedPoolNodes<T extends PoolSelection>(nodes:ProxyNode[], pool:T):T {
  const existing = new Set(nodes.map(node => node.id));
  return {...pool,nodeIds:pool.nodeIds.filter(id => existing.has(id)),excludeNodeIds:pool.excludeNodeIds.filter(id => existing.has(id))};
}

// Only explicit user changes to group/UDP scope prune manual inclusions; opening an old pool never does.
export function constrainPoolSelection<T extends PoolSelection>(nodes:ProxyNode[], pool:T):T {
  const allowed = new Set(poolSelectionScope(nodes,pool).map(node => node.id));
  return {...pool,nodeIds:pool.nodeIds.filter(id => allowed.has(id))};
}

// Presentation-only helpers: filtering and sorting must never rewrite saved intent.
export type PoolNodeStatusFilter = 'all'|'enabled'|'disabled'|'healthy'|'unhealthy'|'unknown';

export function matchesPoolNodeStatus(node:Pick<ProxyNode,'enabled'|'status'>, filter:PoolNodeStatusFilter):boolean {
  if (filter === 'all') return true;
  if (filter === 'enabled') return node.enabled;
  if (filter === 'disabled') return !node.enabled;
  return node.status === filter;
}

export type PoolNodeSort = 'name'|'latency'|'speed'|'updated';

export function sortPoolNodes<T extends Pick<ProxyNode,'name'|'latencyMs'|'speedMbps'|'updatedAt'>>(nodes:T[], sort:PoolNodeSort):T[] {
  const ordered = [...nodes];
  if (sort === 'latency') return ordered.sort((left,right) => (left.latencyMs ?? Infinity) - (right.latencyMs ?? Infinity));
  if (sort === 'speed') return ordered.sort((left,right) => (right.speedMbps ?? -1) - (left.speedMbps ?? -1));
  if (sort === 'updated') return ordered.sort((left,right) => new Date(right.updatedAt).getTime() - new Date(left.updatedAt).getTime());
  return ordered.sort((left,right) => left.name.localeCompare(right.name,'zh-CN',{numeric:true}));
}

// Group matches remain dynamic. Unchecking a match creates an exclusion rather than a static snapshot.
export function changePoolSelection<T extends PoolSelection>(pool:T, scope:ProxyNode[], selectedIds:string[]):T {
  const selected = new Set(selectedIds);
  const manual = new Set(pool.nodeIds);
  const excluded = new Set(pool.excludeNodeIds);
  for (const node of scope) {
    if (excludedByLegacyGroup(node,pool)) continue;
    if (selected.has(node.id)) {
      excluded.delete(node.id);
      if (!matchesPoolGroups(node,pool)) manual.add(node.id);
    } else {
      manual.delete(node.id);
      if (matchesPoolGroups(node,pool)) excluded.add(node.id);
    }
  }
  return {...pool,nodeIds:[...manual],excludeNodeIds:[...excluded]};
}

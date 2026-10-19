import type { Group, ProxyNode } from '../shared/types';

export interface NodeGroupFilter {
  includeGroupIds: string[];
  excludeGroupIds: string[];
  includeMode: 'any' | 'all';
  onlyUngrouped: boolean;
}
export const emptyNodeGroupFilter = (): NodeGroupFilter => ({includeGroupIds:[],excludeGroupIds:[],includeMode:'any',onlyUngrouped:false});

/** Exclusions always win. No includes means all nodes, not only grouped nodes.
 * The ungrouped shortcut ignores includes; the UI clears and disables them.
 */
export function matchesNodeGroups(node: Pick<ProxyNode,'groupIds'>, filter: NodeGroupFilter): boolean {
  if (node.groupIds.some(id => filter.excludeGroupIds.includes(id))) return false;
  if (filter.onlyUngrouped) return node.groupIds.length === 0;
  if (!filter.includeGroupIds.length) return true;
  return filter.includeMode === 'all'
    ? filter.includeGroupIds.every(id => node.groupIds.includes(id))
    : filter.includeGroupIds.some(id => node.groupIds.includes(id));
}

export interface NodeFilter extends NodeGroupFilter {
  query: string;
  protocol: string;
  status: string;
  udp: string;
}
export function matchesNodeFilter(node: ProxyNode, groups: Group[], filter: NodeFilter): boolean {
  const haystack = [node.name,node.server,node.protocol,...node.groupIds.map(id => groups.find(group => group.id === id)?.name || '')].join(' ').toLowerCase();
  return haystack.includes(filter.query.trim().toLowerCase())
    && (filter.protocol === 'all' || node.protocol === filter.protocol)
    && (filter.status === 'all' || (filter.status === 'enabled' ? node.enabled : filter.status === 'disabled' ? !node.enabled : node.status === filter.status))
    && (filter.udp === 'all' || node.udp === (filter.udp === 'yes'))
    && matchesNodeGroups(node,filter);
}

export function nodeRelay(node: Pick<ProxyNode,'dialerId'|'dialerName'|'dialerError'>, nodes: ProxyNode[]) {
  if (!node.dialerId && !node.dialerName && !node.dialerError) return null;
  const upstream = nodes.find(item => item.id === node.dialerId);
  return {
    name: upstream?.name || node.dialerName || '关联节点',
    error: node.dialerError || (node.dialerId && !upstream ? '关联节点已删除或不存在' : upstream && !upstream.enabled ? '关联节点已禁用' : ''),
  };
}

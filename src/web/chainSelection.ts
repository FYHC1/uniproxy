import type {ProxyChain, ProxyNode} from '../shared/types';
import {matchesPoolNodeStatus, sortPoolNodes, type PoolNodeSort, type PoolNodeStatusFilter} from './poolSelection.ts';

// The picker can filter a large node library without rewriting the saved chain order.
export function chainCandidates(nodes:ProxyNode[], group:string, query:string, status:PoolNodeStatusFilter, sort:PoolNodeSort):ProxyNode[] {
  const needle = query.trim().toLocaleLowerCase();
  const visible = nodes.filter(node =>
    (!group || node.groupIds.includes(group)) &&
    matchesPoolNodeStatus(node,status) &&
    (!needle || node.name.toLocaleLowerCase().includes(needle) || node.server.toLocaleLowerCase().includes(needle) || node.protocol.toLocaleLowerCase().includes(needle)));
  return sortPoolNodes(visible,sort);
}

export function toggleChainNode(ids:string[],id:string):string[] {
  return ids.includes(id) ? ids.filter(value => value !== id) : [...ids,id];
}

export function moveChainNode(ids:string[],index:number,delta:number):string[] {
  const target = index + delta;
  if (index < 0 || index >= ids.length || target < 0 || target >= ids.length) return [...ids];
  const next = [...ids];
  [next[index],next[target]] = [next[target],next[index]];
  return next;
}

export function removeChainNode(ids:string[],id:string):string[] {
  return ids.filter(value => value !== id);
}

export function orderedChainNodes(chain:Pick<ProxyChain,'nodeIds'>,nodes:ProxyNode[]):Array<ProxyNode|undefined> {
  const byID = new Map(nodes.map(node => [node.id,node]));
  return chain.nodeIds.map(id => byID.get(id));
}

export function chainEndpoints(chain:Pick<ProxyChain,'nodeIds'>,nodes:ProxyNode[]):{entry:string;exit:string;missing:number} {
  const ordered = orderedChainNodes(chain,nodes);
  return {
    entry: ordered[0]?.name ?? '节点缺失',
    exit: ordered.length ? (ordered[ordered.length-1]?.name ?? '节点缺失') : '尚未组成',
    missing: ordered.filter(node => !node).length,
  };
}

// A chain can enter a UDP listener only when every hop advertises UDP.
export function chainSupportsUDP(chain:Pick<ProxyChain,'nodeIds'>,nodes:ProxyNode[]):boolean {
  const byID = new Map(nodes.map(node => [node.id,node]));
  return chain.nodeIds.length >= 2 && chain.nodeIds.every(id => byID.get(id)?.udp === true);
}

export function chainBlockedReason(chain:Pick<ProxyChain,'nodeIds'>,nodes:ProxyNode[]):string {
  if (chain.nodeIds.length < 2) return '至少需要 2 个节点';
  const byID = new Map(nodes.map(node => [node.id,node]));
  if (chain.nodeIds.some(id => !byID.has(id))) return '包含已被删除的节点';
  if (chain.nodeIds.some(id => !byID.get(id)?.enabled)) return '包含已停用的节点';
  if (chain.nodeIds.some(id => Boolean(byID.get(id)?.dialerId))) return '包含已设置中转的节点';
  if (chain.nodeIds.some(id => Boolean(byID.get(id)?.dialerError))) return '包含中转异常的节点';
  return '';
}

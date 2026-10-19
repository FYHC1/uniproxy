export type Protocol = 'vless'|'vmess'|'trojan'|'ss'|'hysteria'|'hysteria2'|'tuic'|'ssh'|'socks5'|'http';
export interface ProxyNode { id:string; name:string; dialerId?:string; dialerName?:string; dialerError?:string; protocol:Protocol; server:string; port:number; udp:boolean; enabled:boolean; manual:boolean; customName:boolean; groupIds:string[]; sourceIds:string[]; status:'unknown'|'healthy'|'unhealthy'; latencyMs:number|null; speedMbps:number|null; lastCheckedAt:string; lastError:string; createdAt:string; updatedAt:string; }
export interface Group {id:string; name:string; color:string; createdAt:string}
export interface ProxyPool {id:string;name:string;strategy:'url-test'|'round-robin'|'consistent-hashing'|'sticky-sessions';hashKey:''|'in-user';nodeIds:string[];groupIds:string[];groupMode:'any'|'all';excludeNodeIds:string[];excludeGroupIds:string[];enabled:boolean;udp:boolean;testUrl:string;intervalSeconds:number;timeoutMs:number;toleranceMs:number;createdAt:string;updatedAt:string}
export interface Credential {username:string;password?:string;hasPassword?:boolean}
export interface ProxyListener {id:string;name:string;kind:'pool'|'node';targetId:string;protocol:'mixed'|'http'|'socks';host:string;port:number;udp:boolean;enabled:boolean;users:Credential[];createdAt:string;updatedAt:string}
export type PoolDraft = Omit<ProxyPool,'id'|'createdAt'|'updatedAt'>;
export type PoolListenerInput = Partial<Pick<ProxyListener,'name'|'protocol'|'host'|'port'|'udp'|'enabled'|'users'>>;
export interface PoolCreateRequest {pool:PoolDraft;listener:PoolListenerInput}
export interface PoolCreateResult {pool:ProxyPool;listener:ProxyListener}
export interface Subscription {id:string;name:string;url:string;groupIds:string[];enabled:boolean;intervalMinutes:number;lastFetchedAt:string;lastError:string;nodeCount:number;createdAt:string;updatedAt:string}
export interface Settings {testUrl:string;speedTestUrl:string;testTimeoutMs:number;testConcurrency:number;speedTestBytes:number;healthIntervalSeconds:number;portRangeStart:number;portRangeEnd:number}
export interface RuntimeStatus {available:boolean;running:boolean;applying:boolean;version:string;appliedRevision:number;lastError:string;startedAt:string;dirty:boolean;listeners:{id:string;listening:boolean;error:string}[]}
export interface CheckResult {nodeId:string;ok:boolean;latencyMs:number|null;speedMbps:number|null;bytes:number;error:string}
export interface TestJob {id:string;kind:'latency'|'speed';status:'queued'|'running'|'completed'|'cancelled';total:number;completed:number;results:CheckResult[];createdAt:string;finishedAt:string}
export interface Activity {id:string;level:'info'|'warning'|'error';message:string;createdAt:string}
export interface Bootstrap {nodes:ProxyNode[];groups:Group[];pools:ProxyPool[];listeners:ProxyListener[];subscriptions:Subscription[];settings:Settings;revision:number;runtime:RuntimeStatus;activities:Activity[];jobs:TestJob[]}
export interface ImportIssue {line:number;message:string}
export interface ImportResult {added:number;updated:number;removed:number;skipped:number;errors:ImportIssue[];totalErrors?:number;errorsTruncated?:boolean;subscriptionIds:string[]}
export interface ImportRequest {mode:'text'|'subscriptions'|'file';content:string;groupIds:string[];groupNames?:string[];name?:string;intervalMinutes?:number;preview?:boolean}
export interface Mutation<T> {data:T;runtime:RuntimeStatus}
export type PublicNode = ProxyNode;
export type PublicListener = ProxyListener;

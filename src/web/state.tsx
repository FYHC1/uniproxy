import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import type { Bootstrap, Mutation, RuntimeStatus } from '../shared/types';
import { api, ApiError, downloadFile, errorMessage, readToken, storeToken } from './api';

type Tone = 'success' | 'warning' | 'error' | 'info';
export interface Notice { id: number; text: string; tone: Tone }
interface AppState {
  data: Bootstrap | null; loading: boolean; refreshing: boolean; connectionError: string; updatedAt: Date | null;
  authRequired: boolean; authError: string; notices: Notice[]; hasToken: boolean;
  authenticate: (token: string) => Promise<void>; forgetToken: () => void;
  refresh: () => Promise<void>; request: <T>(path: string, method?: string, body?: unknown) => Promise<T>;
  mutate: <T>(path: string, method: string, body?: unknown, success?: string) => Promise<T>;
  runtimeAction: (action: 'apply' | 'stop') => Promise<void>;
  download: (path: string, filename: string) => Promise<void>;
  notify: (text: string, tone?: Tone) => void; dismiss: (id: number) => void;
}
const Context = createContext<AppState | null>(null);
export function AppProvider({ children }: { children: ReactNode }) {
  const [data, setData] = useState<Bootstrap | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [connectionError, setConnectionError] = useState('');
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null);
  const [authRequired, setAuthRequired] = useState(false);
  const [authError, setAuthError] = useState('');
  const [hasToken, setHasToken] = useState(Boolean(readToken()));
  const [notices, setNotices] = useState<Notice[]>([]);
  const controller = useRef<AbortController | null>(null);
  const mounted = useRef(true);
  const writes = useRef(0);
  const sessionEpoch = useRef(0);
  const noticeId = useRef(0);
  const timers = useRef(new Set<number>());
  const notify = useCallback((text: string, tone: Tone = 'success') => {
    const id = ++noticeId.current;
    setNotices(list => [...list.slice(-1), { id, text, tone }]);
    const timer = window.setTimeout(() => { setNotices(list => list.filter(n => n.id !== id)); timers.current.delete(timer); }, tone === 'success' ? 6500 : 11000);
    timers.current.add(timer);
  }, []);
  const acceptRuntime = useCallback((runtime: RuntimeStatus) => {
    if (mounted.current) setData(old => old ? { ...old, runtime } : old);
  }, []);
  const handleError = useCallback((error: unknown) => {
    if (error instanceof ApiError) {
      if (error.runtime) acceptRuntime(error.runtime);
      if (error.status === 401) {
        sessionEpoch.current++;
        controller.current?.abort();
        setAuthRequired(true); setAuthError(error.message); setData(null); setUpdatedAt(null);
      }
    }
  }, [acceptRuntime]);
  const request = useCallback(async <T,>(path: string, method = 'GET', body?: unknown): Promise<T> => {
    const epoch = sessionEpoch.current;
    try { const result = await api<T>(path, method, body); if (epoch !== sessionEpoch.current) throw new Error('管理会话已变更，请在当前会话中核实操作结果。'); return result; }
    catch (error) { if (epoch === sessionEpoch.current) handleError(error); throw error; }
  }, [handleError]);
  const load = useCallback(async (force = false) => {
    if (!force && (writes.current || (controller.current && !controller.current.signal.aborted))) return;
    controller.current?.abort();
    const epoch = sessionEpoch.current;
    const current = new AbortController(); controller.current = current;
    setRefreshing(true);
    try {
      const next = await api<Bootstrap>('/api/bootstrap', 'GET', undefined, current.signal);
      if (current.signal.aborted || !mounted.current || epoch !== sessionEpoch.current) return;
      if (!next || !['nodes','groups','pools','listeners','subscriptions','activities','jobs'].every(key => Array.isArray(next[key as keyof Bootstrap])) || !next.runtime || !Array.isArray(next.runtime.listeners) || !next.settings) throw new Error('管理 API 数据格式不匹配，请检查前后端版本。');
      setData(next); setUpdatedAt(new Date()); setConnectionError(''); setAuthRequired(false); setAuthError('');
    } catch (error) {
      if (current.signal.aborted || !mounted.current || epoch !== sessionEpoch.current) return;
      handleError(error);
      if (!(error instanceof ApiError && error.status === 401)) setConnectionError(errorMessage(error));
    } finally {
      if (controller.current === current && mounted.current) { controller.current = null; setLoading(false); setRefreshing(false); }
    }
  }, [handleError]);
  const refresh = useCallback(() => load(true), [load]);
  useEffect(() => {
    mounted.current = true;
    if (!authRequired) void load();
    const poll = window.setInterval(() => { if (!authRequired && !document.hidden && !writes.current) void load(); }, 5000);
    const onVisible = () => { if (!document.hidden && !authRequired) void load(); };
    document.addEventListener('visibilitychange', onVisible);
    return () => { mounted.current = false; controller.current?.abort(); window.clearInterval(poll); document.removeEventListener('visibilitychange', onVisible); };
  }, [authRequired, load]);
  useEffect(() => () => { timers.current.forEach(window.clearTimeout); timers.current.clear(); }, []);
  const mutate = useCallback(async <T,>(path: string, method: string, body?: unknown, success = '更改已保存'): Promise<T> => {
    writes.current++; controller.current?.abort();
    try {
      const result = await request<Mutation<T>>(path, method, body);
      if (!result || !result.runtime || !Object.prototype.hasOwnProperty.call(result, 'data')) throw new Error('服务返回了不完整的保存结果，请刷新核实更改。');
      acceptRuntime(result.runtime);
      if (success) notify(result.runtime.dirty ? success + '，但尚未应用到内核。' : success, result.runtime.dirty ? 'warning' : 'success');
      await load(true);
      return result.data;
    } finally { writes.current--; }
  }, [request, acceptRuntime, notify, load]);
  const runtimeAction = useCallback(async (action: 'apply' | 'stop') => {
    writes.current++; controller.current?.abort();
    try {
      const runtime = await request<RuntimeStatus>('/api/runtime/' + action, 'POST', {});
      acceptRuntime(runtime);
      notify(action === 'stop' ? '已请求停止内核，请以实时状态为准。' : runtime.dirty ? '配置仍待应用，请查看内核错误。' : '配置已应用到内核。', runtime.dirty ? 'warning' : 'success');
      await load(true);
    } finally { writes.current--; }
  }, [request, acceptRuntime, notify, load]);
  const authenticate = useCallback(async (token: string) => {
    storeToken(token.trim()); sessionEpoch.current++; setHasToken(Boolean(token.trim())); setAuthError('');
    await load(true);
  }, [load]);
  const forgetToken = useCallback(() => {
    try { storeToken(''); } catch (error) { notify(errorMessage(error), 'error'); return; }
    sessionEpoch.current++; controller.current?.abort(); setHasToken(false); setAuthRequired(true); setAuthError(''); setData(null); setUpdatedAt(null);
  }, [notify]);
  const download = useCallback(async (path: string, filename: string) => {
    try { await downloadFile(path, filename); }
    catch (error) { handleError(error); throw error; }
  }, [handleError]);
  return <Context.Provider value={{ data, loading, refreshing, connectionError, updatedAt, authRequired, authError, hasToken, notices, authenticate, forgetToken, refresh, request, mutate, runtimeAction, download, notify, dismiss: id => setNotices(list => list.filter(n => n.id !== id)) }}>{children}</Context.Provider>;
}
export function useApp() { const context = useContext(Context); if (!context) throw new Error('AppProvider is missing'); return context; }
export function useAction() {
  const [running, setRunning] = useState(false);
  const [error, setError] = useState('');
  const lock = useRef(false);
  const execute = async (action: () => Promise<void>): Promise<boolean> => {
    if (lock.current) return false;
    lock.current = true; setRunning(true); setError('');
    try { await action(); return true; }
    catch (cause) { setError(errorMessage(cause)); return false; }
    finally { lock.current = false; setRunning(false); }
  };
  return { running, error, setError, execute };
}

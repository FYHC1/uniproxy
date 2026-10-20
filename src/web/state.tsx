import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react';
import type { Bootstrap, CoreInstallation, Mutation, RuntimeStatus } from '../shared/types';
import { api, ApiError, downloadFile, errorMessage, readToken, storeToken } from './api';
import { isCoreInstallation, shouldPromptForCore } from './CoreInstallModel';

type Tone = 'success' | 'warning' | 'error' | 'info';
type CoreAction = 'install' | 'inspect';
export interface Notice { id: number; text: string; tone: Tone }
interface AppState {
  data: Bootstrap | null; loading: boolean; refreshing: boolean; connectionError: string; updatedAt: Date | null;
  authRequired: boolean; authError: string; notices: Notice[]; hasToken: boolean;
  authenticate: (token: string) => Promise<void>; forgetToken: () => void;
  refresh: () => Promise<void>; request: <T>(path: string, method?: string, body?: unknown) => Promise<T>;
  mutate: <T>(path: string, method: string, body?: unknown, success?: string) => Promise<T>;
  runtimeAction: (action: 'apply' | 'stop') => Promise<void>;
  coreInstallOpen: boolean; coreAction: CoreAction | null; coreError: string;
  openCoreInstaller: () => void; closeCoreInstaller: () => void;
  startCoreInstall: () => Promise<void>; inspectCore: () => Promise<void>;
  download: (path: string, filename: string) => Promise<void>;
  notify: (text: string, tone?: Tone) => void; dismiss: (id: number) => void;
}
const Context = createContext<AppState | null>(null);
const CORE_PROMPT_KEY = 'uniproxy.core-install.prompt:';
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
  const [coreInstallOpen, setCoreInstallOpen] = useState(false);
  const [coreAction, setCoreAction] = useState<CoreAction | null>(null);
  const [coreError, setCoreError] = useState('');
  const dataRef = useRef<Bootstrap | null>(null);
  const controller = useRef<AbortController | null>(null);
  // One synchronous lock survives dialog dismissal and navigation. Aborting the
  // HTTP request on logout does not cancel an accepted server-side download.
  const coreOperation = useRef<{ controller: AbortController } | null>(null);
  const promptedPaths = useRef(new Set<string>());
  const mounted = useRef(true);
  const writes = useRef(0);
  const sessionEpoch = useRef(0);
  const responseEpoch = useRef(0);
  const noticeId = useRef(0);
  const timers = useRef(new Set<number>());
  const notify = useCallback((text: string, tone: Tone = 'success') => {
    const id = ++noticeId.current;
    setNotices(list => [...list.slice(-1), { id, text, tone }]);
    const timer = window.setTimeout(() => { setNotices(list => list.filter(n => n.id !== id)); timers.current.delete(timer); }, tone === 'success' ? 6500 : 11000);
    timers.current.add(timer);
  }, []);
  const acceptRuntime = useCallback((runtime: RuntimeStatus) => {
    if (mounted.current && dataRef.current) {
      dataRef.current = { ...dataRef.current, runtime };
      setData(dataRef.current);
    }
  }, []);
  const clearSession = useCallback(() => {
    sessionEpoch.current++; responseEpoch.current++;
    controller.current?.abort(); coreOperation.current?.controller.abort();
    dataRef.current = null; setData(null); setUpdatedAt(null);
    setCoreInstallOpen(false); setCoreError(''); setCoreAction(null); setNotices([]);
  }, []);
  const handleError = useCallback((error: unknown) => {
    if (error instanceof ApiError) {
      if (error.runtime) acceptRuntime(error.runtime);
      if (error.status === 401) {
        clearSession(); setAuthRequired(true); setAuthError(error.message);
      }
    }
  }, [acceptRuntime, clearSession]);
  const request = useCallback(async <T,>(path: string, method = 'GET', body?: unknown): Promise<T> => {
    const epoch = sessionEpoch.current;
    try {
      const result = await api<T>(path, method, body);
      if (!mounted.current || epoch !== sessionEpoch.current) throw new Error('管理会话已变更，请在当前会话中核实操作结果。');
      return result;
    } catch (error) { if (mounted.current && epoch === sessionEpoch.current) handleError(error); throw error; }
  }, [handleError]);
  const load = useCallback(async (force = false) => {
    if (!force && (writes.current || (controller.current && !controller.current.signal.aborted))) return;
    controller.current?.abort();
    const epoch = sessionEpoch.current; const response = responseEpoch.current;
    const current = new AbortController(); controller.current = current;
    setRefreshing(true);
    try {
      const next = await api<Bootstrap>('/api/bootstrap', 'GET', undefined, current.signal);
      if (current.signal.aborted || !mounted.current || epoch !== sessionEpoch.current || response !== responseEpoch.current) return;
      if (!next || !['nodes','groups','pools','listeners','subscriptions','activities','jobs'].every(key => Array.isArray(next[key as keyof Bootstrap])) || !next.runtime || !Array.isArray(next.runtime.listeners) || !next.settings || !isCoreInstallation(next.coreInstallation)) throw new Error('管理 API 数据格式不匹配，请检查前后端版本。');
      dataRef.current = next; setData(next); setUpdatedAt(new Date()); setConnectionError(''); setAuthRequired(false); setAuthError('');
    } catch (error) {
      if (current.signal.aborted || !mounted.current || epoch !== sessionEpoch.current || response !== responseEpoch.current) return;
      handleError(error);
      if (!(error instanceof ApiError && error.status === 401)) setConnectionError(errorMessage(error));
    } finally {
      if (controller.current === current && mounted.current) { controller.current = null; setLoading(false); setRefreshing(false); }
    }
  }, [handleError]);
  const refresh = useCallback(() => load(true), [load]);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false; sessionEpoch.current++;
      controller.current?.abort(); coreOperation.current?.controller.abort();
      timers.current.forEach(window.clearTimeout); timers.current.clear();
    };
  }, []);
  useEffect(() => {
    if (!authRequired) void load();
    // Bootstrap already contains cached installation progress. Normal polling
    // never invokes the heavyweight /core/inspect version probe.
    const poll = window.setInterval(() => { if (!authRequired && !document.hidden && !writes.current) void load(); }, 5000);
    const onVisible = () => { if (!document.hidden && !authRequired) void load(); };
    document.addEventListener('visibilitychange', onVisible);
    return () => { window.clearInterval(poll); document.removeEventListener('visibilitychange', onVisible); };
  }, [authRequired, load]);
  const rememberCorePrompt = useCallback((path: string) => {
    promptedPaths.current.add(path);
    try { sessionStorage.setItem(CORE_PROMPT_KEY + path, '1'); } catch { /* In-memory fallback still prevents repeated prompts. */ }
  }, []);
  useEffect(() => {
    if (!data || authRequired || connectionError || coreAction) return;
    const installation = data.coreInstallation;
    try { if (sessionStorage.getItem(CORE_PROMPT_KEY + installation.path)) promptedPaths.current.add(installation.path); } catch { /* Session storage is optional for this preference. */ }
    // A known background job must not open a surprise modal, even when that job
    // later fails. Its progress/failure remains available from the global entry.
    if (installation.state === 'installing') { rememberCorePrompt(installation.path); return; }
    if (shouldPromptForCore(installation, data.runtime, promptedPaths.current)) {
      rememberCorePrompt(installation.path); setCoreInstallOpen(true);
    }
  }, [data, authRequired, connectionError, coreAction, rememberCorePrompt]);
  const openCoreInstaller = useCallback(() => {
    if (!dataRef.current || authRequired) return;
    rememberCorePrompt(dataRef.current.coreInstallation.path); setCoreInstallOpen(true);
  }, [authRequired, rememberCorePrompt]);
  const closeCoreInstaller = useCallback(() => setCoreInstallOpen(false), []);
  const operateCore = useCallback(async (action: CoreAction) => {
    if (coreOperation.current || !dataRef.current || authRequired) return;
    const currentData = dataRef.current;
    const installation = currentData.coreInstallation;
    if (writes.current || currentData.runtime.applying || installation.state === 'installing') {
      setCoreError('内核安装或其他操作正在进行，请等待完成后再试。'); return;
    }
    if (action === 'install' && (!installation.canInstall || !installation.supported || installation.installed || currentData.runtime.running)) {
      setCoreError(installation.reason || '当前不能在线安装；已有内核文件不会被覆盖，请核实安装状态。'); return;
    }
    const operation = { controller: new AbortController() };
    const epoch = sessionEpoch.current;
    const active = () => mounted.current && epoch === sessionEpoch.current && !operation.controller.signal.aborted;
    coreOperation.current = operation; writes.current++; responseEpoch.current++; controller.current?.abort();
    rememberCorePrompt(installation.path); setCoreAction(action); setCoreError('');
    try {
      const result = await api<CoreInstallation | { coreInstallation: CoreInstallation; runtime: RuntimeStatus }>('/api/core/' + action, 'POST', {}, operation.controller.signal);
      if (!active()) return;
      const next = action === 'install' ? result as CoreInstallation : (result as { coreInstallation: CoreInstallation }).coreInstallation;
      const runtime = action === 'inspect' ? (result as { runtime: RuntimeStatus }).runtime : undefined;
      if (!isCoreInstallation(next) || (action === 'inspect' && (!runtime || !Array.isArray(runtime.listeners)))) throw new Error('服务返回了不完整的内核安装状态，请同步数据后核实。');
      // Supersede every bootstrap that began before this authoritative response.
      responseEpoch.current++; controller.current?.abort();
      if (dataRef.current) {
        dataRef.current = { ...dataRef.current, coreInstallation: next, ...(runtime ? { runtime } : {}) };
        setData(dataRef.current);
      }
      if (action === 'inspect') notify(runtime?.available ? '已检测到可用内核；不会自动启动代理，请按需重新应用配置。' : next.reason || '尚未检测到可用内核，请核实安装路径与版本。', runtime?.available ? 'success' : 'warning');
    } catch (error) {
      if (active()) {
        handleError(error);
        if (active()) {
          setCoreError(errorMessage(error));
          if (!(error instanceof ApiError)) setConnectionError(errorMessage(error));
        }
      }
    } finally {
      // A 409 may mean another tab already started installation. Reconcile the
      // real status without inventing progress or silently retrying the POST.
      if (active()) { responseEpoch.current++; controller.current?.abort(); await load(true); }
      writes.current--;
      if (coreOperation.current === operation) { coreOperation.current = null; if (active()) setCoreAction(null); }
    }
  }, [authRequired, handleError, load, notify, rememberCorePrompt]);
  const startCoreInstall = useCallback(() => operateCore('install'), [operateCore]);
  const inspectCore = useCallback(() => operateCore('inspect'), [operateCore]);
  const mutate = useCallback(async <T,>(path: string, method: string, body?: unknown, success = '更改已保存'): Promise<T> => {
    writes.current++; responseEpoch.current++; controller.current?.abort();
    try {
      const result = await request<Mutation<T>>(path, method, body);
      if (!result || !result.runtime || !Object.prototype.hasOwnProperty.call(result, 'data')) throw new Error('服务返回了不完整的保存结果，请刷新核实更改。');
      responseEpoch.current++; controller.current?.abort(); acceptRuntime(result.runtime);
      if (success) notify(result.runtime.dirty ? success + '，但尚未应用到内核。' : success, result.runtime.dirty ? 'warning' : 'success');
      await load(true);
      return result.data;
    } finally { writes.current--; }
  }, [request, acceptRuntime, notify, load]);
  const runtimeAction = useCallback(async (action: 'apply' | 'stop') => {
    if (coreOperation.current || dataRef.current?.coreInstallation.state === 'installing') throw new Error('内核安装或检测期间不能应用或停止代理，请等待完成。');
    if (action === 'apply' && !dataRef.current?.runtime.available) { openCoreInstaller(); return; }
    writes.current++; responseEpoch.current++; controller.current?.abort();
    try {
      const runtime = await request<RuntimeStatus>('/api/runtime/' + action, 'POST', {});
      responseEpoch.current++; controller.current?.abort(); acceptRuntime(runtime);
      notify(action === 'stop' ? '已请求停止内核，请以实时状态为准。' : runtime.dirty ? '配置仍待应用，请查看内核错误。' : '配置已应用到内核。', runtime.dirty ? 'warning' : 'success');
      await load(true);
    } finally { writes.current--; }
  }, [request, acceptRuntime, notify, load, openCoreInstaller]);
  const authenticate = useCallback(async (token: string) => {
    storeToken(token.trim()); clearSession(); setHasToken(Boolean(token.trim())); setAuthError(''); setConnectionError('');
    await load(true);
  }, [clearSession, load]);
  const forgetToken = useCallback(() => {
    try { storeToken(''); } catch (error) { notify(errorMessage(error), 'error'); return; }
    clearSession(); setHasToken(false); setAuthRequired(true); setAuthError(''); setConnectionError('');
  }, [clearSession, notify]);
  const download = useCallback(async (path: string, filename: string) => {
    const epoch = sessionEpoch.current;
    try { await downloadFile(path, filename); }
    catch (error) { if (mounted.current && epoch === sessionEpoch.current) handleError(error); throw error; }
  }, [handleError]);
  return <Context.Provider value={{ data, loading, refreshing, connectionError, updatedAt, authRequired, authError, hasToken, notices, authenticate, forgetToken, refresh, request, mutate, runtimeAction, coreInstallOpen, coreAction, coreError, openCoreInstaller, closeCoreInstaller, startCoreInstall, inspectCore, download, notify, dismiss: id => setNotices(list => list.filter(n => n.id !== id)) }}>{children}</Context.Provider>;
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

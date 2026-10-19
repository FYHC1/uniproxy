import type { RuntimeStatus } from '../shared/types';

const TOKEN_KEY = 'uniproxy.admin-token';
export function readToken(): string {
  try { return sessionStorage.getItem(TOKEN_KEY) ?? ''; } catch { return ''; }
}
export function storeToken(token: string): void {
  try { if (token) sessionStorage.setItem(TOKEN_KEY, token); else sessionStorage.removeItem(TOKEN_KEY); }
  catch { throw new Error('浏览器禁止会话存储，请允许 sessionStorage 后重试。令牌不会写入地址栏。'); }
}
export class ApiError extends Error {
  constructor(message: string, public status: number, public code?: string, public runtime?: RuntimeStatus) {
    super(message); this.name = 'ApiError';
  }
}
async function response(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  const token = readToken();
  if (token) headers.set('Authorization', 'Bearer ' + token);
  if (init.body) headers.set('Content-Type', 'application/json');
  let res: Response;
  try { res = await fetch(path, { ...init, headers, credentials: 'same-origin', cache: 'no-store' }); }
  catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error;
    throw new Error('无法连接管理服务。请确认 UniProxy 服务可用后重试。');
  }
  if (!res.ok) {
    const body = await res.json().catch(() => null) as {error?: string; code?: string; runtime?: RuntimeStatus} | null;
    throw new ApiError(body?.error || (res.status === 401 ? '管理令牌无效或已过期，请重新验证。' : '请求失败（HTTP ' + res.status + '）'), res.status, body?.code, body?.runtime);
  }
  return res;
}
export async function api<T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> {
  const res = await response(path, { method, body: body === undefined ? undefined : JSON.stringify(body), signal });
  if (res.status === 204) return undefined as T;
  try { return await res.json() as T; }
  catch { throw new Error('服务返回了无法识别的响应。请检查 API 路由与服务版本。'); }
}
export async function downloadFile(path: string, filename: string): Promise<void> {
  const res = await response(path);
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url; link.download = filename; document.body.appendChild(link); link.click(); link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export function errorMessage(error: unknown): string { return error instanceof Error ? error.message : '操作未完成，请重试。'; }

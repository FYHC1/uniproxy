import { randomUUID } from 'node:crypto';
import { test as base, expect, type APIRequestContext, type Locator, type Page } from '@playwright/test';
import type { Bootstrap, Group, ImportResult, Mutation, PoolCreateResult, PoolDraft, PoolListenerInput, ProxyListener, ProxyNode, ProxyPool } from '../../src/shared/types';

export { expect };

export interface FixtureProxy {
  name: string;
  type: 'socks5' | 'http';
  server: string;
  port: number;
  udp: boolean;
  username?: string;
  password?: string;
  'dialer-proxy'?: string;
}

export function byName<T extends { name: string }>(items: T[], name: string): T {
  const matches = items.filter(item => item.name === name);
  expect(matches, 'Find the test-owned object by its unique name').toHaveLength(1);
  return matches[0];
}

// These fixtures use only the temporary, missing-core testserver. Never point
// them at an existing installation, restore a shared backup, or contact a proxy.
export class BrowserFixtures {
  readonly prefix = 'E2E-' + randomUUID().slice(0, 12);
  private readonly addressNamespace = randomUUID().replaceAll('-', '').slice(0, 20);
  private addressIndex = 0;

  constructor(readonly request: APIRequestContext) {}

  name(label: string): string {
    return this.prefix + '-' + label;
  }

  proxy(label: string, options: Partial<Omit<FixtureProxy, 'name'>> = {}): FixtureProxy {
    const parts = this.addressNamespace.match(/.{4}/g)!;
    const server = '2001:db8:' + parts.join(':') + ':' + (++this.addressIndex).toString(16);
    return { name: this.name(label), type: 'socks5', server, port: 1080, udp: options.type !== 'http', ...options };
  }

  async bootstrap(): Promise<Bootstrap> {
    const response = await this.request.get('/api/bootstrap');
    expect(response.ok(), 'Read the isolated API bootstrap').toBe(true);
    return response.json() as Promise<Bootstrap>;
  }

  async importNodes(proxies: FixtureProxy[], options: { groupIds?: string[]; groupNames?: string[] } = {}): Promise<ProxyNode[]> {
    for (const proxy of proxies) {
      expect(proxy.name.startsWith(this.prefix + '-')).toBe(true);
      expect(proxy.server, 'Fixtures must use RFC 5737 or RFC 3849 documentation addresses').toMatch(/^(?:192[.]0[.]2[.][0-9]{1,3}|198[.]51[.]100[.][0-9]{1,3}|203[.]0[.]113[.][0-9]{1,3}|2001:db8:[0-9a-f:]+)$/i);
    }
    const result = await this.mutation<ImportResult>('/api/import', {
      mode: 'text', content: JSON.stringify({ proxies }),
      groupIds: options.groupIds ?? [], groupNames: options.groupNames ?? [],
    });
    expect(result.data.errors).toEqual([]);
    expect(result.data.totalErrors ?? 0).toBe(0);
    const state = await this.bootstrap();
    return proxies.map(proxy => byName(state.nodes, proxy.name));
  }

  async group(label: string): Promise<Group> {
    return (await this.mutation<Group>('/api/groups', { name: this.name(label) })).data;
  }

  poolDraft(label: string, overrides: Partial<PoolDraft> = {}): PoolDraft {
    return {
      name: this.name(label), strategy: 'url-test', hashKey: '', nodeIds: [], groupIds: [],
      groupMode: 'any', excludeNodeIds: [], excludeGroupIds: [], enabled: true, udp: false,
      testUrl: 'http://192.0.2.254/generate_204', intervalSeconds: 300, timeoutMs: 8000, toleranceMs: 50,
      ...overrides,
    };
  }

  async pool(label: string, overrides: Partial<PoolDraft> = {}): Promise<ProxyPool> {
    return (await this.mutation<ProxyPool>('/api/pools', this.poolDraft(label, overrides))).data;
  }

  async poolWithListener(label: string, overrides: Partial<PoolDraft> = {}, listener: PoolListenerInput = {}): Promise<PoolCreateResult> {
    return (await this.mutation<PoolCreateResult>('/api/pools/with-listener', {
      pool: this.poolDraft(label, overrides), listener,
    })).data;
  }

  // For the old listener endpoint only: the absent core never binds this saved
  // configuration. Atomic-create tests obtain ports through its real allocator.
  async unusedConfiguredPort(): Promise<number> {
    const state = await this.bootstrap();
    const used = new Set(state.listeners.map(listener => listener.port));
    used.add(19090);
    for (let port = state.settings.portRangeEnd; port >= state.settings.portRangeStart; port--) {
      if (!used.has(port)) return port;
    }
    throw new Error('No unused configured port remains in the isolated fixture range');
  }

  async listener(label: string, targetId: string, overrides: Partial<Omit<ProxyListener, 'id' | 'createdAt' | 'updatedAt' | 'name' | 'targetId'>> = {}): Promise<ProxyListener> {
    const port = overrides.port ?? await this.unusedConfiguredPort();
    return (await this.mutation<ProxyListener>('/api/listeners', {
      name: this.name(label), kind: 'pool', targetId, protocol: 'mixed', host: '127.0.0.1',
      port, udp: false, enabled: true, users: [], ...overrides,
    })).data;
  }

  async patchNode(nodeId: string, patch: Partial<Pick<ProxyNode, 'enabled' | 'name' | 'groupIds'>>): Promise<void> {
    const response = await this.request.patch('/api/nodes/' + encodeURIComponent(nodeId), { data: patch });
    expect(response.ok(), 'Patch an isolated fixture node').toBe(true);
  }

  async deleteListener(listenerId: string): Promise<void> {
    const response = await this.request.delete('/api/listeners/' + encodeURIComponent(listenerId));
    expect(response.ok(), 'Remove this test-owned listener configuration').toBe(true);
  }

  private async mutation<T>(path: string, data: unknown): Promise<Mutation<T>> {
    const response = await this.request.post(path, { data });
    expect(response.ok(), 'Fixture mutation ' + path + ' returned HTTP ' + response.status()).toBe(true);
    const result = await response.json() as Mutation<T>;
    expect(result.runtime.available).toBe(false);
    expect(result.runtime.running).toBe(false);
    expect(result.runtime.dirty).toBe(true);
    return result;
  }

  async dispose(): Promise<void> {
    const state = await this.bootstrap();
    const owned = (item: { name: string }) => item.name.startsWith(this.prefix + '-');
    for (const listener of state.listeners.filter(owned)) await this.deleteListener(listener.id);
    for (const pool of state.pools.filter(owned)) {
      const response = await this.request.delete('/api/pools/' + encodeURIComponent(pool.id));
      expect(response.ok(), 'Remove this test-owned pool').toBe(true);
    }
    const nodeIds = state.nodes.filter(owned).map(node => node.id);
    if (nodeIds.length) {
      const response = await this.request.post('/api/nodes/batch', { data: { ids: nodeIds, action: 'delete' } });
      expect(response.ok(), 'Remove only this test-owned node batch').toBe(true);
    }
    for (const group of state.groups.filter(owned)) {
      const response = await this.request.delete('/api/groups/' + encodeURIComponent(group.id));
      expect(response.ok(), 'Remove this test-owned group').toBe(true);
    }
  }
}

// Other feature tests dismiss the real startup prompt rather than changing
// bootstrap availability or removing assertions about a genuinely missing core.
export async function dismissCoreInstallPrompt(page: Page): Promise<void> {
  const dialog = page.getByRole('dialog', { name: '安装 Mihomo 内核', exact: true });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: '稍后处理', exact: true }).click();
  await expect(dialog).not.toBeVisible();
}

export async function gotoWorkspace(page: Page, target: string): Promise<void> {
  const alreadyPrompted = await page.evaluate(() => Object.keys(sessionStorage).some(key => key.startsWith('uniproxy.core-install.prompt:'))).catch(() => false);
  await page.goto(target);
  if (!alreadyPrompted) await dismissCoreInstallPrompt(page);
  else await expect(page.getByRole('navigation')).toBeVisible();
}

export const test = base.extend<{ fixtures: BrowserFixtures; pageErrors: void; offlineCoreGuard: void }>({
  fixtures: [async ({ request, baseURL }, use) => {
    expect(baseURL, 'Browser writes are restricted to cmd/testserver on port 19090').toBe('http://127.0.0.1:19090');
    const fixtures = new BrowserFixtures(request);
    const state = await fixtures.bootstrap();
    expect(state.runtime.available, 'Do not run fixtures against a real core').toBe(false);
    expect(state.runtime.running).toBe(false);
    try {
      await use(fixtures);
    } finally {
      await fixtures.dispose();
    }
  }, { auto: true }],
  offlineCoreGuard: [async ({ context }, use) => {
    const unexpected: string[] = [];
    await context.route('**/api/core/install', async route => {
      unexpected.push(route.request().method());
      await route.abort('blockedbyclient');
    });
    await use();
    expect(unexpected, 'Real API browser tests must never request a network core installation').toEqual([]);
  }, { auto: true }],
  pageErrors: [async ({ page }, use) => {
    const errors: string[] = [];
    page.on('pageerror', error => errors.push(error.message));
    await use();
    expect(errors, 'No unhandled browser errors').toEqual([]);
  }, { auto: true }],
});

export function poolCard(page: Page, name: string): Locator {
  return page.getByRole('article', { name: '代理池 ' + name, exact: true });
}

export function nodeChoices(dialog: Locator): Locator {
  return dialog.getByRole('group', { name: /^包含节点/ });
}

export function groupChoices(dialog: Locator): Locator {
  return dialog.getByRole('group', { name: /^包含分组/ });
}

export async function newPoolDialog(page: Page): Promise<Locator> {
  await gotoWorkspace(page, '/#/pools');
  // The empty state also has a create button; always use the page action.
  await page.locator('.page-heading').getByRole('button', { name: '新建代理池', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '新建代理池', exact: true });
  await expect(dialog).toBeVisible();
  return dialog;
}

export async function editPoolDialog(page: Page, name: string): Promise<Locator> {
  await poolCard(page, name).getByRole('button', { name: '管理节点', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '编辑代理池', exact: true });
  await expect(dialog).toBeVisible();
  return dialog;
}

export async function submitNewPool(page: Page, dialog: Locator): Promise<Mutation<PoolCreateResult>> {
  const responsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname === '/api/pools/with-listener' && response.request().method() === 'POST');
  await dialog.getByRole('button', { name: '创建代理池与监听', exact: true }).click();
  const response = await responsePromise;
  expect(response.ok()).toBe(true);
  const result = await response.json() as Mutation<PoolCreateResult>;
  await expect(dialog).not.toBeVisible();
  return result;
}

export async function savePoolDialog(dialog: Locator): Promise<void> {
  await dialog.getByRole('button', { name: '保存代理池', exact: true }).click();
  await expect(dialog).not.toBeVisible();
}

import type { APIRequestContext, Page } from '@playwright/test';
import type { Settings } from '../../src/shared/types';
import { expect, gotoWorkspace, test } from './fixtures';

interface InterfaceEntry { name: string; up: boolean; loopback: boolean; addresses: string[] }

async function liveAdapters(request: APIRequestContext): Promise<InterfaceEntry[]> {
  const response = await request.get('/api/interfaces');
  expect(response.ok(), 'Read the read-only host interface inventory').toBe(true);
  const body = await response.json() as { interfaces: InterfaceEntry[] };
  return body.interfaces.filter(item => item.up && !item.loopback);
}

async function settingsTab(page: Page): Promise<void> {
  await gotoWorkspace(page, '/#/settings');
  await page.getByRole('button', { name: '检测与端口', exact: true }).click();
}

// The selector must mirror the API inventory exactly: only adapters that are
// currently up and not loopback, with automatic routing still the default.
test('global outbound selector mirrors the live, non-loopback interface inventory', async ({ page, fixtures }) => {
  const adapters = await liveAdapters(fixtures.request);
  await settingsTab(page);
  const select = page.getByLabel('全局出站网卡', { exact: true });
  await expect(select).toBeVisible();
  await expect(select.locator('option').first()).toHaveText('自动（系统默认路由）');
  // The inventory is fetched after mount, so wait for it instead of racing it.
  await expect(select.locator('option')).toHaveCount(1 + adapters.length);
  const values = await select.locator('option').evaluateAll(nodes => nodes.map(node => (node as HTMLOptionElement).value));
  // The console renders the inventory in locale order, so compare membership
  // rather than re-implementing that collation in the test process.
  expect(values[0]).toBe('');
  expect([...values].sort()).toEqual(['', ...adapters.map(item => item.name)].sort());
  expect(values).toHaveLength(1 + adapters.length);
  for (const adapter of adapters) {
    const option = select.locator('option', { hasText: adapter.name });
    await expect(option).toHaveCount(1);
    if (adapter.addresses.length) await expect(option).toContainText(adapter.addresses[0]);
  }
  // Down/loopback adapters and routes, DNS or MAC details must never leak here.
  const body = await (await fixtures.request.get('/api/interfaces')).json() as { interfaces: Record<string, unknown>[] };
  for (const entry of body.interfaces) expect(Object.keys(entry).sort()).toEqual(['addresses', 'loopback', 'name', 'up']);
  const text = await page.content();
  expect(text).not.toContain('fe80:');
  expect(text).not.toContain('169.254.');
  await expect(page.getByText('网卡不可用时不会回退直连。', { exact: false })).toBeVisible();
});

test('binding a live adapter persists and reverting to automatic clears it', async ({ page, fixtures }) => {
  const adapters = await liveAdapters(fixtures.request);
  test.skip(adapters.length === 0, 'This host reports no up non-loopback adapter');
  const before = (await fixtures.bootstrap()).settings;
  // The isolated server is shared by every spec, so never assume a clean pin:
  // start from automatic routing and restore it even when an assertion fails.
  await fixtures.request.patch('/api/settings', { data: { outboundInterface: '' } });
  const adapter = adapters[0];
  try {
    await settingsTab(page);
    const select = page.getByLabel('全局出站网卡', { exact: true });
    await select.selectOption(adapter.name);
    await page.getByRole('button', { name: '保存设置', exact: true }).click();
    await expect(page.getByText('设置已同步', { exact: true })).toBeVisible();
    const pinned = (await fixtures.bootstrap()).settings;
    expect(pinned.outboundInterface).toBe(adapter.name);
    // The pinned adapter stays selectable and keeps its address annotation.
    await expect(select).toHaveValue(adapter.name);
    await expect(select.locator('option', { hasText: adapter.name })).toHaveCount(1);

    await select.selectOption('');
    await page.getByRole('button', { name: '保存设置', exact: true }).click();
    await expect(page.getByText('设置已同步', { exact: true })).toBeVisible();
    const cleared = (await fixtures.bootstrap()).settings;
    expect(cleared.outboundInterface).toBe('');
    expect(cleared).toEqual({ ...before, outboundInterface: '' } as Settings);
  } finally {
    await fixtures.request.patch('/api/settings', { data: { outboundInterface: '' } });
  }
});

test('an adapter that does not exist is rejected without touching saved settings', async ({ page, fixtures }) => {
  const adapters = await liveAdapters(fixtures.request);
  const before = (await fixtures.bootstrap()).settings;
  const response = await fixtures.request.patch('/api/settings', { data: { outboundInterface: 'uniproxy-e2e-missing-adapter' } });
  expect(response.status()).toBe(400);
  const body = await response.json() as { error: string };
  expect(body.error).toContain('全局出站网卡');
  expect((await fixtures.bootstrap()).settings).toEqual(before);
  // The selector still offers automatic routing and the real adapters only.
  await settingsTab(page);
  const missingSelect = page.getByLabel('全局出站网卡', { exact: true });
  await expect(missingSelect.locator('option')).toHaveCount(1 + adapters.length);
  const values = await missingSelect.locator('option').evaluateAll(nodes => nodes.map(node => (node as HTMLOptionElement).value));
  expect(values).not.toContain('uniproxy-e2e-missing-adapter');
  expect(values[0]).toBe('');
});

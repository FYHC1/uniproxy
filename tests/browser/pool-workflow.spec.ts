import type { Locator, Page } from '@playwright/test';
import { byName, editPoolDialog, expect, newPoolDialog, nodeChoices, poolCard, savePoolDialog, submitNewPool, test, gotoWorkspace } from './fixtures';

async function expectNoHorizontalOverflow(page: Page, container: Locator): Promise<void> {
  const dimensions = await page.evaluate(() => ({ body: document.body.scrollWidth, root: document.documentElement.scrollWidth, view: window.innerWidth }));
  expect(dimensions.body).toBeLessThanOrEqual(dimensions.view + 1);
  expect(dimensions.root).toBeLessThanOrEqual(dimensions.view + 1);
  const bounds = await container.evaluate(element => ({ width: element.clientWidth, scroll: element.scrollWidth, left: element.getBoundingClientRect().left, right: element.getBoundingClientRect().right }));
  expect(bounds.scroll).toBeLessThanOrEqual(bounds.width + 1);
  expect(bounds.left).toBeGreaterThanOrEqual(-1);
  expect(bounds.right).toBeLessThanOrEqual(dimensions.view + 1);
}

test('invalid pool-listener creation keeps both collections and revision unchanged', async ({ page, fixtures }) => {
  const [node] = await fixtures.importNodes([fixtures.proxy('validation-node')]);
  const occupied = await fixtures.poolWithListener('occupied-port', { nodeIds: [node.id] });
  const before = await fixtures.bootstrap();
  const dialog = await newPoolDialog(page);
  const name = fixtures.name('validation-pool');
  await dialog.getByLabel('代理池名称', { exact: true }).fill(name);
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill(node.name);
  await nodeChoices(dialog).getByRole('checkbox', { name: node.name, exact: true }).check();
  let createRequests = 0;
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/pools/with-listener' && request.method() === 'POST') createRequests++; });
  await dialog.getByLabel('监听地址', { exact: true }).fill('192.0.2.50');
  await dialog.getByRole('button', { name: '创建代理池与监听', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('非回环地址必须设置至少一组用户名和密码');
  expect(createRequests).toBe(0);
  await dialog.getByLabel('监听地址', { exact: true }).fill('127.0.0.1');
  await dialog.getByRole('switch', { name: '自动分配空闲端口', exact: true }).click();
  await dialog.getByRole('spinbutton', { name: '监听端口', exact: true }).fill(String(occupied.listener.port));
  const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === '/api/pools/with-listener' && response.request().method() === 'POST');
  await dialog.getByRole('button', { name: '创建代理池与监听', exact: true }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(400);
  expect(response.request().postDataJSON()).toMatchObject({ pool: { name }, listener: { port: occupied.listener.port, host: '127.0.0.1' } });
  await expect(dialog.getByRole('alert')).toContainText(/端口.*使用|端口.*占用/);
  await expect(dialog).toBeVisible();
  const failed = await fixtures.bootstrap();
  expect(failed.revision).toBe(before.revision);
  expect(failed.pools).toEqual(before.pools);
  expect(failed.listeners).toEqual(before.listeners);
  expect(failed.pools.filter(pool => pool.name === name)).toEqual([]);
  expect(failed.listeners.filter(listener => listener.name === name)).toEqual([]);
  await dialog.getByRole('switch', { name: '自动分配空闲端口', exact: true }).click();
  const result = await submitNewPool(page, dialog);
  expect(result.data.listener.port).not.toBe(occupied.listener.port);
  const after = await fixtures.bootstrap();
  expect(after.revision).toBe(before.revision + 1);
  expect(after.pools.filter(pool => pool.name === name)).toHaveLength(1);
  expect(after.listeners.filter(listener => listener.targetId === result.data.pool.id)).toHaveLength(1);
});

test('manual listener port creation uses the entered port and loopback UDP controls', async ({ page, fixtures }) => {
  const [node] = await fixtures.importNodes([fixtures.proxy('manual-port-node')]);
  // Obtain a real free port from the allocator, then release only this test's
  // saved listener. No socket/server is started by the browser test.
  const reservation = await fixtures.poolWithListener('manual-port-reservation', { nodeIds: [node.id] }, { udp: false });
  await fixtures.deleteListener(reservation.listener.id);
  const dialog = await newPoolDialog(page);
  await dialog.getByLabel('代理池名称', { exact: true }).fill(fixtures.name('manual-port-pool'));
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill(node.name);
  await nodeChoices(dialog).getByRole('checkbox', { name: node.name, exact: true }).check();
  await dialog.getByRole('switch', { name: '仅选择 UDP 兼容节点', exact: true }).click();
  await expect(dialog.getByRole('switch', { name: 'UDP 转发', exact: true })).toBeEnabled();
  await dialog.getByRole('switch', { name: 'UDP 转发', exact: true }).click();
  await expect(dialog.getByText(/原生 UDP 数据包不执行用户名密码认证/)).toBeVisible();
  await dialog.getByRole('combobox', { name: '监听协议', exact: true }).selectOption('http');
  await expect(dialog.getByRole('switch', { name: 'UDP 转发', exact: true })).not.toBeChecked();
  await expect(dialog.getByRole('switch', { name: 'UDP 转发', exact: true })).toBeDisabled();
  await dialog.getByRole('combobox', { name: '监听协议', exact: true }).selectOption('socks');
  await dialog.getByRole('switch', { name: '自动分配空闲端口', exact: true }).click();
  await dialog.getByRole('spinbutton', { name: '监听端口', exact: true }).fill(String(reservation.listener.port));
  const result = await submitNewPool(page, dialog);
  expect(result.data.listener).toMatchObject({ port: reservation.listener.port, protocol: 'socks', udp: false, host: '127.0.0.1' });
  expect(result.runtime).toMatchObject({ running: false, dirty: true });
  await expect(poolCard(page, result.data.pool.name).getByRole('region', { name: '关联监听', exact: true })).toContainText('127.0.0.1:' + reservation.listener.port);
});

test('card removal is pool-local, confirms before DELETE, and allows re-inclusion of a dynamic member', async ({ page, fixtures }) => {
  const group = await fixtures.group('remove-group');
  const [node] = await fixtures.importNodes([fixtures.proxy('remove-member')], { groupIds: [group.id] });
  const source = await fixtures.poolWithListener('remove-source', { groupIds: [group.id], nodeIds: [node.id] });
  const other = await fixtures.poolWithListener('remove-other', { groupIds: [group.id] });
  const dedicated = await fixtures.listener('remove-dedicated', node.id, { kind: 'node' });
  await gotoWorkspace(page, '/#/pools');
  const before = await fixtures.bootstrap();
  const removed: string[] = [];
  page.on('request', request => { if (request.method() === 'DELETE') removed.push(new URL(request.url()).pathname); });
  const card = poolCard(page, source.pool.name);
  const member = card.getByRole('listitem', { name: '池内节点 ' + node.name, exact: true });
  await member.getByRole('button', { name: '从池移除 ' + node.name, exact: true }).click();
  let confirmation = page.getByRole('dialog', { name: '从「' + source.pool.name + '」移除节点？', exact: true });
  await expect(confirmation).toContainText('不删除总节点');
  expect(removed).toEqual([]);
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  await expect(member).toBeVisible();
  await member.getByRole('button', { name: '从池移除 ' + node.name, exact: true }).click();
  confirmation = page.getByRole('dialog', { name: '从「' + source.pool.name + '」移除节点？', exact: true });
  const expectedPath = '/api/pools/' + source.pool.id + '/members/' + node.id;
  const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === expectedPath && response.request().method() === 'DELETE');
  await confirmation.getByRole('button', { name: '从池移除', exact: true }).click();
  expect((await responsePromise).ok()).toBe(true);
  await expect(confirmation).not.toBeVisible();
  await expect(member).toHaveCount(0);
  await expect(card).toContainText('空池拒绝新连接，不会自动直连');
  expect(removed).toEqual([expectedPath]);
  const after = await fixtures.bootstrap();
  const saved = byName(after.pools, source.pool.name);
  expect(saved.nodeIds).not.toContain(node.id);
  expect(saved.excludeNodeIds).toContain(node.id);
  expect(byName(after.nodes, node.name)).toEqual(byName(before.nodes, node.name));
  expect(byName(after.pools, other.pool.name)).toEqual(byName(before.pools, other.pool.name));
  expect(after.listeners).toEqual(before.listeners);
  expect(byName(after.listeners, dedicated.name).targetId).toBe(node.id);
  await expect(poolCard(page, other.pool.name).getByRole('listitem', { name: '池内节点 ' + node.name, exact: true })).toBeVisible();
  const dialog = await editPoolDialog(page, source.pool.name);
  const checkbox = nodeChoices(dialog).getByRole('checkbox', { name: node.name, exact: true });
  await expect(checkbox).not.toBeChecked();
  await checkbox.check();
  await savePoolDialog(dialog);
  const reincluded = byName((await fixtures.bootstrap()).pools, source.pool.name);
  expect(reincluded.excludeNodeIds).not.toContain(node.id);
  expect(reincluded.nodeIds).toEqual([]);
  expect(reincluded.groupIds).toEqual([group.id]);
  await expect(member).toBeVisible();
});

test('pool-only API stays compatible and cards allocate or add listeners on the same page', async ({ page, fixtures }) => {
  const [node] = await fixtures.importNodes([fixtures.proxy('same-page-node')]);
  const before = await fixtures.bootstrap();
  const pool = await fixtures.pool('pool-only', { nodeIds: [node.id] });
  expect((await fixtures.bootstrap()).listeners).toEqual(before.listeners);
  await gotoWorkspace(page, '/#/pools');
  const card = poolCard(page, pool.name);
  await expect(card.getByRole('region', { name: '关联监听', exact: true })).toContainText('尚未分配监听');
  for (const [index, action] of ['分配监听', '添加监听'].entries()) {
    await card.getByRole('button', { name: action, exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '新建监听端口', exact: true });
    await expect(dialog.getByRole('combobox', { name: '目标类型', exact: true })).toHaveValue('pool');
    await expect(dialog.getByRole('combobox', { name: '绑定目标', exact: true })).toHaveValue(pool.id);
    const listenerName = fixtures.name('same-page-listener-' + index);
    await dialog.getByLabel('监听名称', { exact: true }).fill(listenerName);
    await dialog.getByRole('spinbutton', { name: '端口', exact: true }).fill(String(await fixtures.unusedConfiguredPort()));
    await dialog.getByRole('button', { name: '创建监听', exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(page).toHaveURL(/#\/pools$/);
    await expect(card.getByRole('region', { name: '关联监听', exact: true }).getByText(listenerName, { exact: true })).toBeVisible();
  }
  expect((await fixtures.bootstrap()).listeners.filter(listener => listener.kind === 'pool' && listener.targetId === pool.id)).toHaveLength(2);
});

for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
  test('pool cards show real configured addresses, unknown health and disabled tests at width ' + viewport.width, async ({ page, fixtures }, testInfo) => {
    await page.setViewportSize(viewport);
    const relayProxy = fixtures.proxy('relay');
    const leafProxy = fixtures.proxy('blocked-leaf', { 'dialer-proxy': relayProxy.name });
    const [unknown, disabled, relay, leaf] = await fixtures.importNodes([
      fixtures.proxy('long-unknown-' + '出口'.repeat(22)), fixtures.proxy('disabled-node'), relayProxy, leafProxy,
    ]);
    await fixtures.patchNode(disabled.id, { enabled: false });
    await fixtures.patchNode(relay.id, { enabled: false });
    const created = await fixtures.poolWithListener('responsive-pool-' + '出口'.repeat(18), { nodeIds: [unknown.id, disabled.id, leaf.id] });
    const ipv6 = await fixtures.listener('ipv6-loopback', created.pool.id, { host: '::1', protocol: 'socks' });
    const poolName = created.pool.name;
    const testRequests: string[] = [];
    page.on('request', request => { if (new URL(request.url()).pathname === '/api/tests') testRequests.push(request.method()); });
    await gotoWorkspace(page, '/#/pools');
    await page.getByRole('searchbox', { name: '搜索代理池', exact: true }).fill(poolName);
    const card = poolCard(page, poolName);
    await expect(card).toBeVisible();
    await expect(card).toContainText('1 个可转发 · 3 个已选节点');
    const endpoints = card.getByRole('region', { name: '关联监听', exact: true });
    await expect(endpoints.getByText('127.0.0.1:' + created.listener.port, { exact: true })).toBeVisible();
    await expect(endpoints.getByText('[::1]:' + ipv6.port, { exact: true })).toBeVisible();
    await expect(endpoints.getByText('配置待应用', { exact: true })).toHaveCount(2);
    await expect(endpoints.getByText('监听中', { exact: true })).toHaveCount(0);
    await expect(endpoints).toContainText('已保存的配置地址');
    await expect(card.getByRole('button', { name: '测全部延迟', exact: true })).toBeDisabled();
    for (const node of [unknown, disabled, leaf]) {
      const item = card.getByRole('listitem', { name: '池内节点 ' + node.name, exact: true });
      await expect(item).toContainText('尚未检测');
      await item.locator('.pool-node-details summary').click();
      await expect(item.getByText('[' + node.server + ']:' + node.port,{exact:true})).toBeVisible();
      await expect(item.getByRole('button', { name: '测延迟 ' + node.name, exact: true })).toBeDisabled();
      await expect(item.getByRole('button', { name: '从池移除 ' + node.name, exact: true })).toBeEnabled();
      await expect(item.getByText(/[0-9]+(?:[.][0-9]+)?\s*ms/)).toHaveCount(0);
    }
    await expect(card.getByRole('listitem', { name: '池内节点 ' + disabled.name, exact: true })).toContainText('节点已禁用');
    await expect(card.getByRole('listitem', { name: '池内节点 ' + leaf.name, exact: true })).toContainText('中转不可用');
    await expectNoHorizontalOverflow(page, card);
    await page.screenshot({ path: testInfo.outputPath('pool-card-' + viewport.width + '.png'), fullPage: true });
    const dialog = await editPoolDialog(page, poolName);
    await expect(dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true })).toBeVisible();
    await expect(nodeChoices(dialog).getByRole('checkbox', { name: disabled.name, exact: true })).toBeChecked();
    await expect(nodeChoices(dialog).getByRole('checkbox', { name: leaf.name, exact: true })).toBeChecked();
    await expectNoHorizontalOverflow(page, dialog);
    await dialog.getByRole('button', { name: '取消', exact: true }).click();
    expect(testRequests).toEqual([]);
  });
}

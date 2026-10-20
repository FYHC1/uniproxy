import type { Mutation, ProxyListener } from '../../src/shared/types';
import { byName, expect, newPoolDialog, nodeChoices, poolCard, submitNewPool, test, gotoWorkspace } from './fixtures';

test('missing-core console is honest and every page is accessible', async ({ page }) => {
  await gotoWorkspace(page, '/');
  await expect(page.getByRole('link', { name: 'UniProxy 节点管理', exact: true })).toBeVisible();
  await expect(page.getByText('内核尚不可用，配置仅保存到管理服务', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: '管理内核安装', exact: true })).toBeEnabled();
  await expect(page.getByRole('button', { name: '重新应用', exact: true })).toHaveCount(0);
  for (const name of ['节点管理', '分组管理', '代理池', '监听端口', '订阅管理', '运行与设置']) {
    await page.getByRole('link', { name, exact: true }).click();
    await expect(page.getByRole('heading', { name, level: 1, exact: true })).toBeVisible();
  }
});

test('offline import and UI reflect persisted nodes without exposing credentials', async ({ page, fixtures }) => {
  const password = fixtures.name('generated-upstream-password');
  const groupName = fixtures.name('browser-group');
  const [node] = await fixtures.importNodes([fixtures.proxy('Browser-Smoke', { type: 'http', username: 'browser-test', password })], { groupNames: [groupName] });
  await gotoWorkspace(page, '/#/nodes');
  await page.getByRole('searchbox', { name: '搜索节点名称、协议、地址或分组' }).fill(node.name);
  const row = page.getByRole('row').filter({ has: page.getByRole('button', { name: node.name, exact: true }) });
  await expect(row).toBeVisible();
  await expect(row.getByText(groupName, { exact: true })).toBeVisible();
  expect(await page.content()).not.toContain(password);
  const saved = byName((await fixtures.bootstrap()).nodes, node.name);
  expect(saved).not.toHaveProperty('config');
  expect(saved.groupIds).toHaveLength(1);
});

test('create a unique group through the responsive console', async ({ page, fixtures }, testInfo) => {
  await gotoWorkspace(page, '/#/groups');
  await page.locator('.page-heading').getByRole('button', { name: '新建分组', exact: true }).click();
  const dialog = page.getByRole('dialog');
  const name = fixtures.name('UI-Group');
  await dialog.getByLabel('分组名称', { exact: true }).fill(name);
  await dialog.getByRole('button', { name: /创建分组|保存/ }).click();
  await expect(dialog).not.toBeVisible();
  await expect(page.getByText(name, { exact: true })).toBeVisible();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({ path: testInfo.outputPath('mobile-console.png'), fullPage: true });
  const dimensions = await page.evaluate(() => ({ body: document.body.scrollWidth, view: window.innerWidth }));
  expect(dimensions.body).toBeLessThanOrEqual(dimensions.view + 1);
});

test('import dialog previews and imports documentation-address nodes offline', async ({ page, fixtures }, testInfo) => {
  const proxy = fixtures.proxy('UI-SOCKS');
  await gotoWorkspace(page, '/#/nodes');
  await page.locator('.page-heading').getByRole('button', { name: '导入节点', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByLabel('节点内容', { exact: false }).fill(JSON.stringify({ proxies: [proxy] }));
  await dialog.getByRole('button', { name: '预览解析', exact: true }).click();
  await expect(dialog.getByText(proxy.name, { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '确认导入', exact: true }).click();
  await expect(dialog.getByText('导入处理完成', { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  await page.getByRole('searchbox', { name: '搜索节点名称、协议、地址或分组' }).fill(proxy.name);
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: proxy.name, exact: true }) })).toBeVisible();
  await page.screenshot({ path: testInfo.outputPath('desktop-console.png'), fullPage: true });
});

test('create a native strategy pool and its authenticated mixed listener atomically', async ({ page, fixtures }) => {
  const [node] = await fixtures.importNodes([fixtures.proxy('Pool-Node')]);
  const before = await fixtures.bootstrap();
  const dialog = await newPoolDialog(page);
  const name = fixtures.name('UI-Pool');
  await dialog.getByLabel('代理池名称', { exact: true }).fill(name);
  await dialog.getByRole('combobox', { name: '调度策略', exact: true }).selectOption('sticky-sessions');
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill(node.name);
  await nodeChoices(dialog).getByRole('checkbox', { name: node.name, exact: true }).check();
  await expect(dialog.getByRole('switch', { name: '自动分配空闲端口', exact: true })).toBeChecked();
  await expect(dialog.getByRole('spinbutton', { name: '监听端口', exact: true })).toHaveCount(0);
  await expect(dialog.getByLabel('监听地址', { exact: true })).toHaveValue('127.0.0.1');
  await expect(dialog.getByRole('combobox', { name: '监听协议', exact: true })).toHaveValue('mixed');
  await dialog.getByRole('button', { name: '添加账号', exact: true }).click();
  await dialog.getByLabel('用户名 1', { exact: true }).fill('pool-task');
  const password = fixtures.name('generated-pool-password');
  await dialog.getByLabel('密码 1', { exact: true }).fill(password);
  const created = await submitNewPool(page, dialog);
  expect(created.runtime).toMatchObject({ available: false, running: false, dirty: true });
  expect(created.runtime.lastError).not.toBe('');
  const after = await fixtures.bootstrap();
  expect(after.revision).toBe(before.revision + 1);
  const pool = byName(after.pools, name);
  expect(pool.strategy).toBe('sticky-sessions');
  expect(pool.nodeIds).toEqual([node.id]);
  const related = after.listeners.filter(listener => listener.kind === 'pool' && listener.targetId === pool.id);
  expect(related).toHaveLength(1);
  const [listener] = related;
  expect(listener).toMatchObject({ id: created.data.listener.id, host: '127.0.0.1', protocol: 'mixed', udp: false, enabled: true });
  expect(listener.port).toBeGreaterThanOrEqual(after.settings.portRangeStart);
  expect(listener.port).toBeLessThanOrEqual(after.settings.portRangeEnd);
  expect(before.listeners.map(item => item.port)).not.toContain(listener.port);
  expect(listener.users).toEqual([{ username: 'pool-task', hasPassword: true }]);
  const card = poolCard(page, name);
  await expect(card).toBeVisible();
  await expect(card.getByRole('region', { name: '关联监听', exact: true })).toContainText('127.0.0.1:' + listener.port);
  await expect(card.getByText('配置待应用', { exact: true })).toBeVisible();
  await expect(card.getByRole('listitem', { name: '池内节点 ' + node.name, exact: true })).toContainText('尚未检测');
  expect(await page.content()).not.toContain(password);
});

test('separate listener creation and blank-password editing preserve authentication', async ({ page, fixtures }) => {
  const [node] = await fixtures.importNodes([fixtures.proxy('auth-node')]);
  const pool = await fixtures.pool('auth-pool', { nodeIds: [node.id] });
  await gotoWorkspace(page, '/#/listeners');
  await page.locator('.page-heading').getByRole('button', { name: '新建监听', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: '新建监听端口', exact: true });
  const name = fixtures.name('auth-listener');
  await dialog.getByLabel('监听名称', { exact: true }).fill(name);
  await dialog.getByRole('combobox', { name: '目标类型', exact: true }).selectOption('pool');
  await dialog.getByRole('combobox', { name: '绑定目标', exact: true }).selectOption(pool.id);
  await dialog.getByRole('spinbutton', { name: '端口', exact: true }).fill(String(await fixtures.unusedConfiguredPort()));
  await dialog.getByRole('button', { name: '添加账号', exact: true }).click();
  await dialog.getByLabel('用户名 1', { exact: true }).fill('listener-task');
  const password = fixtures.name('generated-listener-password');
  await dialog.getByLabel('密码 1', { exact: true }).fill(password);
  await dialog.getByRole('button', { name: '创建监听', exact: true }).click();
  await expect(dialog).not.toBeVisible();
  const row = page.getByRole('row').filter({ has: page.getByRole('button', { name, exact: true }) });
  await expect(row).toBeVisible();
  const original = byName((await fixtures.bootstrap()).listeners, name);
  expect(original.users).toEqual([{ username: 'listener-task', hasPassword: true }]);
  await row.getByRole('button', { name, exact: true }).click();
  dialog = page.getByRole('dialog', { name: '编辑监听端口', exact: true });
  await expect(dialog.getByLabel('密码 1', { exact: true })).toHaveValue('');
  await expect(dialog.getByText('留空保留此用户名的原密码', { exact: true })).toBeVisible();
  await dialog.getByRole('combobox', { name: '监听协议', exact: true }).selectOption('socks');
  const responsePromise = page.waitForResponse(response =>
    new URL(response.url()).pathname === '/api/listeners/' + original.id && response.request().method() === 'PUT');
  await dialog.getByRole('button', { name: '保存监听', exact: true }).click();
  const response = await responsePromise;
  expect(response.ok()).toBe(true);
  expect(response.request().postDataJSON().users).toEqual([{ username: 'listener-task' }]);
  const saved = await response.json() as Mutation<ProxyListener>;
  expect(saved.data.users).toEqual([{ username: 'listener-task', hasPassword: true }]);
  await expect(dialog).not.toBeVisible();
  expect(byName((await fixtures.bootstrap()).listeners, name)).toMatchObject({ id: original.id, protocol: 'socks', users: original.users });
  expect(await page.content()).not.toContain(password);
});

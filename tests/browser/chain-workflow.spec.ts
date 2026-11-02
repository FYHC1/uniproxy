import type { Locator, Page } from '@playwright/test';
import { expect, gotoWorkspace, test } from './fixtures';

async function openNewChain(page: Page): Promise<Locator> {
  await gotoWorkspace(page, '/#/chains');
  await page.locator('.page-heading').getByRole('button', { name: '新建代理链', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '新建代理链', exact: true });
  await expect(dialog).toBeVisible();
  return dialog;
}

async function addHop(dialog: Locator, name: string): Promise<void> {
  await dialog.getByRole('button', { name: '加入链路 ' + name, exact: true }).click();
  await expect(dialog.getByRole('button', { name: '从链路移除 ' + name, exact: true })).toBeVisible();
}

async function expectNoOverflow(page: Page, container: Locator): Promise<void> {
  const dimensions = await page.evaluate(() => ({ body: document.body.scrollWidth, root: document.documentElement.scrollWidth, view: window.innerWidth }));
  expect(dimensions.body).toBeLessThanOrEqual(dimensions.view + 1);
  expect(dimensions.root).toBeLessThanOrEqual(dimensions.view + 1);
  const bounds = await container.evaluate(element => ({ width: element.clientWidth, scroll: element.scrollWidth, left: element.getBoundingClientRect().left, right: element.getBoundingClientRect().right }));
  expect(bounds.scroll).toBeLessThanOrEqual(bounds.width + 1);
  expect(bounds.left).toBeGreaterThanOrEqual(-1);
  expect(bounds.right).toBeLessThanOrEqual(dimensions.view + 1);
}

test('create an ordered chain with an atomic listener, inspect it, then delete without shortening', async ({ page, fixtures }) => {
  const [entry, middle, exit] = await fixtures.importNodes([
    fixtures.proxy('chain-entry'), fixtures.proxy('chain-middle'), fixtures.proxy('chain-exit'),
  ]);
  const name = fixtures.name('ordered-chain');
  const dialog = await openNewChain(page);
  await dialog.getByLabel('代理链名称', { exact: true }).fill(name);
  await addHop(dialog, entry.name);
  await addHop(dialog, middle.name);
  await addHop(dialog, exit.name);
  let order = await dialog.locator('.chain-route-hop .chain-hop-main strong').allTextContents();
  expect(order).toEqual([entry.name, middle.name, exit.name]);
  await dialog.getByRole('button', { name: '上移 ' + middle.name, exact: true }).click();
  await dialog.getByRole('button', { name: '上移 ' + exit.name, exact: true }).click();
  order = await dialog.locator('.chain-route-hop .chain-hop-main strong').allTextContents();
  expect(order).toEqual([middle.name, exit.name, entry.name]);
  await expect(dialog.getByText('入口', { exact: true })).toBeVisible();
  await expect(dialog.getByText('出口', { exact: true })).toBeVisible();
  const requestPromise = page.waitForResponse(response => new URL(response.url()).pathname === '/api/chains/with-listener' && response.request().method() === 'POST');
  await dialog.getByRole('button', { name: '创建代理链与监听', exact: true }).click();
  const response = await requestPromise;
  expect(response.status()).toBe(200);
  const payload = response.request().postDataJSON();
  expect(payload.chain).toMatchObject({ name, nodeIds: [middle.id, exit.id, entry.id], enabled: true });
  expect(payload.listener).toMatchObject({ protocol: 'mixed', host: '127.0.0.1', port: 0, udp: false, enabled: true, users: [] });
  await expect(dialog).not.toBeVisible();
  const card = page.getByRole('article', { name: '代理链 ' + name, exact: true });
  await expect(card).toBeVisible();
  await expect(card).toContainText('3 跳');
  await expect(card.getByText(middle.name, { exact: true }).first()).toBeVisible();
  await expect(card.getByText(entry.name, { exact: true }).first()).toBeVisible();
  await expect(card.getByRole('button', { name: '测试整链', exact: true })).toBeDisabled();
  const state = await fixtures.bootstrap();
  const chain = state.chains.find(item => item.name === name);
  expect(chain?.nodeIds).toEqual([middle.id, exit.id, entry.id]);
  const listener = state.listeners.find(item => item.kind === 'chain' && item.targetId === chain?.id);
  expect(listener).toMatchObject({ name, protocol: 'mixed', host: '127.0.0.1', udp: false, enabled: true });
  await card.getByRole('button', { name: '编辑监听 ' + name, exact: true }).click();
  const listenerDialog = page.getByRole('dialog', { name: '编辑监听端口', exact: true });
  await expect(listenerDialog).toBeVisible();
  await expect(listenerDialog.getByLabel('目标类型', { exact: true })).toHaveValue('chain');
  await expect(listenerDialog.getByLabel('绑定目标', { exact: true })).toHaveValue(chain!.id);
  await listenerDialog.getByRole('button', { name: '关闭对话框', exact: true }).click();
  await card.getByRole('button', { name: '删除代理链 ' + name, exact: true }).click();
  const confirmation = page.getByRole('dialog', { name: '删除「' + name + '」？', exact: true });
  await expect(confirmation).toContainText('关联的链路入口监听会被停用');
  const deletePromise = page.waitForResponse(response => new URL(response.url()).pathname === '/api/chains/' + chain!.id && response.request().method() === 'DELETE');
  await confirmation.getByRole('button', { name: '删除代理链', exact: true }).click();
  expect((await deletePromise).status()).toBe(200);
  await expect(card).not.toBeVisible();
  const after = await fixtures.bootstrap();
  expect(after.chains.some(item => item.id === chain!.id)).toBe(false);
  const retained = after.listeners.find(item => item.id === listener!.id);
  expect(retained?.enabled).toBe(false);
  expect(retained?.targetId).toBe(chain!.id);
});

test('a disabled listener can be saved when its chain is unavailable', async ({ page, fixtures }) => {
  const [entry, exit] = await fixtures.importNodes([fixtures.proxy('blocked-entry'), fixtures.proxy('blocked-exit')]);
  const created = await fixtures.chainWithListener('blocked-chain', [entry.id, exit.id], {name: fixtures.name('blocked-listener')}, {enabled: false});
  const listener = created.listener;
  if (!listener) throw new Error('chain fixture did not return its listener');
  await fixtures.patchNode(entry.id, {enabled: false});
  await gotoWorkspace(page, '/#/listeners');
  await page.getByRole('button', {name: '编辑监听 ' + listener.name, exact: true}).click();
  const dialog = page.getByRole('dialog', {name: '编辑监听端口', exact: true});
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText(/该代理链当前不可用/)).toBeVisible();
  await dialog.getByRole('switch', {name: '启用此监听', exact: true}).click();
  const save = page.waitForResponse(response => new URL(response.url()).pathname === '/api/listeners/' + listener.id && response.request().method() === 'PUT');
  await dialog.getByRole('button', {name: '保存监听', exact: true}).click();
  expect((await save).status()).toBe(200);
  await expect(dialog).not.toBeVisible();
  const state = await fixtures.bootstrap();
  expect(state.listeners.find(item => item.id === listener.id)?.enabled).toBe(false);
});

test('chain editor rejects a one-hop save, preserves hidden selections and reports unavailable hops', async ({ page, fixtures }) => {
  const [first, second, disabled] = await fixtures.importNodes([
    fixtures.proxy('chain-first'), fixtures.proxy('chain-second'), fixtures.proxy('chain-disabled'),
  ]);
  await fixtures.patchNode(disabled.id, { enabled: false });
  const dialog = await openNewChain(page);
  await dialog.getByLabel('代理链名称', { exact: true }).fill(fixtures.name('validation-chain'));
  await addHop(dialog, first.name);
  let createCalls = 0;
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/chains/with-listener' && request.method() === 'POST') createCalls++; });
  await dialog.getByRole('button', { name: '创建代理链与监听', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('链式代理至少需要 2 个节点，请再添加一个节点');
  expect(createCalls).toBe(0);
  await expect(dialog).toBeVisible();
  await addHop(dialog, second.name);
  await dialog.getByRole('searchbox', { name: '搜索节点', exact: true }).fill(first.name);
  await expect(dialog.getByText('筛选外仍有 1 个已选节点保留在链路中，不会因筛选被移除。', { exact: true })).toBeVisible();
  await expect(dialog.locator('.chain-route-hop')).toHaveCount(2);
  await dialog.getByRole('searchbox', { name: '搜索节点', exact: true }).fill('');
  await addHop(dialog, disabled.name);
  await expect(dialog.getByText('有 1 个节点已禁用、已设置中转或中转异常；启用链路前必须修复。', { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '保存代理链', exact: true }).count();
  // UI uses the create label until the chain exists; the invalid hop must keep the request local.
  await dialog.getByRole('button', { name: '创建代理链与监听', exact: true }).click();
  await expect(dialog.getByRole('alert')).toContainText('代理链包含已停用、已设置中转或中转异常的节点');
  expect(createCalls).toBe(0);
  await dialog.getByRole('button', { name: '移除 ' + disabled.name, exact: true }).click();
  await expect(dialog.locator('.chain-route-hop')).toHaveCount(2);
});

test('clearing a chain requires explicit confirmation', async ({ page, fixtures }) => {
  const [first, second] = await fixtures.importNodes([fixtures.proxy('clear-first'), fixtures.proxy('clear-second')]);
  const dialog = await openNewChain(page);
  await addHop(dialog, first.name);
  await addHop(dialog, second.name);
  await dialog.getByRole('button', { name: '清空', exact: true }).click();
  const confirmation = page.getByRole('dialog', { name: '清空代理链？', exact: true });
  await expect(confirmation).toContainText('将移除当前链路中的全部节点');
  const route = page.locator('.chain-dialog-modal .chain-route-hop');
  await expect(route).toHaveCount(2);
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  await expect(route).toHaveCount(2);
  await dialog.getByRole('button', { name: '清空', exact: true }).click();
  await page.getByRole('dialog', { name: '清空代理链？', exact: true }).getByRole('button', { name: '清空链路', exact: true }).click();
  await expect(dialog.locator('.chain-route-hop')).toHaveCount(0);
  await expect(dialog.getByText('链式代理至少需要 2 个节点，请再添加一个节点', { exact: true }).first()).toBeVisible();
});

test('saved chain cards copy as disabled routes and listener page offers chain targets', async ({ page, fixtures }) => {
  const [entry, exit] = await fixtures.importNodes([fixtures.proxy('card-entry'), fixtures.proxy('card-exit')]);
  const chain = await fixtures.chain('card-chain', [entry.id, exit.id]);
  await gotoWorkspace(page, '/#/chains');
  const card = page.getByRole('article', { name: '代理链 ' + chain.name, exact: true });
  await expect(card).toBeVisible();
  await card.getByRole('button', { name: '复制代理链 ' + chain.name, exact: true }).click();
  const copyName = chain.name + '（副本）';
  const copy = page.getByRole('article', { name: '代理链 ' + copyName, exact: true });
  await expect(copy).toBeVisible();
  await expect(copy.getByText('已禁用', { exact: true })).toBeVisible();
  await copy.getByRole('button', { name: '启用代理链 ' + copyName, exact: true }).click();
  await expect(copy.getByText('已启用', { exact: true })).toBeVisible();
  await gotoWorkspace(page, '/#/listeners');
  await page.locator('.page-heading').getByRole('button', { name: '新建监听', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '新建监听端口', exact: true });
  await expect(dialog).toBeVisible();
  await dialog.getByLabel('目标类型', { exact: true }).selectOption('chain');
  await expect(dialog.getByLabel('绑定目标', { exact: true }).locator('option')).toContainText([chain.name, copyName]);
  await dialog.getByLabel('绑定目标', { exact: true }).selectOption(chain.id);
  await dialog.getByLabel('监听名称', { exact: true }).fill(fixtures.name('chain-entry'));
  await expect(dialog.getByLabel('监听名称', { exact: true })).not.toHaveValue('');
  await dialog.getByRole('button', { name: '关闭对话框', exact: true }).click();
  await page.setViewportSize({ width: 390, height: 844 });
  await expectNoOverflow(page, page.locator('.main-content'));
});

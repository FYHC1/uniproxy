import type { Locator, Page } from '@playwright/test';
import type { Bootstrap, Group } from '../../src/shared/types';
import { byName, expect, poolCard, test, gotoWorkspace } from './fixtures';

async function openManager(page: Page, group: Group): Promise<Locator> {
  await gotoWorkspace(page, '/#/groups');
  await page.getByRole('button', { name: '管理分组 ' + group.name + ' 的节点', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '分组节点 · ' + group.name, exact: true });
  await expect(dialog).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  return dialog;
}

function member(dialog: Locator, name: string): Locator {
  return dialog.getByRole('row', { name: '本组节点 ' + name, exact: true });
}

async function submitBatch(page: Page, button: Locator, action: string, ids: string[], groupIds?: string[]): Promise<void> {
  const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === '/api/nodes/batch' && response.request().method() === 'POST');
  await button.click();
  const response = await responsePromise;
  expect(response.ok()).toBe(true);
  const body = response.request().postDataJSON() as { action: string; ids: string[]; groupIds?: string[] };
  expect(body.action).toBe(action);
  expect([...body.ids].sort()).toEqual([...ids].sort());
  if (groupIds) expect(body.groupIds).toEqual(groupIds);
  else expect(body).not.toHaveProperty('groupIds');
}

// Trigger the existing visibility refresh without remounting the dialog or
// replacing application state. API writes stay in the isolated test fixture.
async function syncBootstrap(page: Page): Promise<void> {
  const next = page.waitForResponse(response => new URL(response.url()).pathname === '/api/bootstrap' && response.request().method() === 'GET');
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  expect((await next).ok()).toBe(true);
}

async function noHorizontalOverflow(page: Page, container: Locator): Promise<void> {
  const view = await page.evaluate(() => ({ width: innerWidth, body: document.body.scrollWidth, root: document.documentElement.scrollWidth }));
  expect(view.body).toBeLessThanOrEqual(view.width + 1);
  expect(view.root).toBeLessThanOrEqual(view.width + 1);
  const bounds = await container.evaluate(element => ({ width: element.clientWidth, scroll: element.scrollWidth, left: element.getBoundingClientRect().left, right: element.getBoundingClientRect().right }));
  expect(bounds.scroll).toBeLessThanOrEqual(bounds.width + 1);
  expect(bounds.left).toBeGreaterThanOrEqual(-1);
  expect(bounds.right).toBeLessThanOrEqual(view.width + 1);
}

test('group name and count open only its members; pencil renames and inclusion/exclusion references stay distinct', async ({ page, fixtures }) => {
  const group = await fixtures.group('direct-manager');
  const other = await fixtures.group('other-group');
  const [enabled, disabled] = await fixtures.importNodes([fixtures.proxy('member-enabled'), fixtures.proxy('member-disabled')], { groupIds: [group.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('outside')], { groupIds: [other.id] });
  await fixtures.patchNode(disabled.id, { enabled: false });
  await fixtures.pool('included', { groupIds: [group.id] });
  await fixtures.pool('legacy-excluded', { groupIds: [other.id], excludeGroupIds: [group.id] });
  let dialog = await openManager(page, group);
  await expect(member(dialog, enabled.name)).toBeVisible();
  await expect(member(dialog, disabled.name)).toContainText('已禁用');
  await expect(member(dialog, outside.name)).toHaveCount(0);
  await expect(dialog.getByLabel('分组名称', { exact: true })).toHaveCount(0);
  await expect(dialog.locator('.group-node-summary')).toContainText('本组节点2');
  await expect(dialog.locator('.group-node-summary')).toContainText('已启用1');
  await expect(dialog.locator('.group-node-summary')).toContainText('尚未检测2');
  await expect(dialog.locator('form')).toHaveCount(0);
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  const row = page.getByRole('row').filter({ has: page.getByRole('button', { name: '管理分组 ' + group.name + ' 的节点', exact: true }) });
  await expect(row.locator('.group-pool-includes')).toHaveText('1 个池');
  await expect(row.locator('.group-pool-exclusions')).toContainText('1 个池');
  await expect(row.locator('.group-pool-exclusions')).toContainText('排除此组，非成员来源');
  await row.getByRole('button', { name: '管理分组 ' + group.name + ' 的 2 个节点', exact: true }).click();
  dialog = page.getByRole('dialog', { name: '分组节点 · ' + group.name, exact: true });
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  await row.getByRole('button', { name: '编辑分组 ' + group.name, exact: true }).click();
  const rename = page.getByRole('dialog', { name: '编辑分组', exact: true });
  const renamed = fixtures.name('renamed');
  await rename.getByLabel('分组名称', { exact: true }).fill(renamed);
  await rename.getByRole('button', { name: '保存分组', exact: true }).click();
  await expect(rename).not.toBeVisible();
  expect(byName((await fixtures.bootstrap()).groups, renamed).id).toBe(group.id);
  await page.getByRole('button', { name: '管理分组 ' + renamed + ' 的节点', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '分组节点 · ' + renamed, exact: true })).toBeVisible();
});

test('adding existing nodes uses bulk results beyond 100, retains search-hidden choices and other groups', async ({ page, fixtures }) => {
  const group = await fixtures.group('add-target');
  const other = await fixtures.group('add-other');
  const [existing] = await fixtures.importNodes([fixtures.proxy('existing-member')], { groupIds: [group.id] });
  const ungrouped = await fixtures.importNodes(Array.from({ length: 101 }, (_, index) => fixtures.proxy('add-hit-' + String(index).padStart(3, '0'))));
  const grouped = await fixtures.importNodes([fixtures.proxy('add-hit-grouped'), fixtures.proxy('keep-search-hidden')], { groupIds: [other.id] });
  const dialog = await openManager(page, group);
  await dialog.getByRole('button', { name: '添加已有节点', exact: true }).click();
  const choices = dialog.getByRole('group', { name: /^可添加节点/ });
  const search = choices.getByRole('searchbox', { name: '搜索可添加节点', exact: true });
  await expect(choices.getByRole('checkbox', { name: existing.name, exact: true })).toHaveCount(0);
  await search.fill(grouped[1].name);
  await choices.getByRole('checkbox', { name: grouped[1].name, exact: true }).check();
  await search.fill(fixtures.name('add-hit-'));
  await expect(choices.getByRole('checkbox')).toHaveCount(101);
  await expect(choices.getByRole('button', { name: '再显示 100 项（共 102 项）', exact: true })).toBeVisible();
  await choices.getByRole('checkbox', { name: '全选可添加节点当前结果', exact: true }).check();
  await expect(choices.locator('.selection-count')).toHaveText('103 已选');
  await choices.getByRole('button', { name: '再显示 100 项（共 102 项）', exact: true }).click();
  await expect(choices.getByRole('checkbox', { name: ungrouped[100].name, exact: true })).toBeChecked();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await expect(dialog.locator('form')).toHaveCount(0);
  await submitBatch(page, dialog.getByRole('button', { name: '添加到本组', exact: true }), 'add_groups', [...ungrouped, ...grouped].map(node => node.id), [group.id]);
  await expect(dialog.getByRole('heading', { name: '本组节点', exact: true })).toBeVisible();
  const after = await fixtures.bootstrap();
  for (const node of [...ungrouped, ...grouped]) {
    const saved = byName(after.nodes, node.name);
    expect(saved.groupIds).toContain(group.id);
    expect(saved.manual).toBe(node.manual);
    expect(saved.sourceIds).toEqual(node.sourceIds);
  }
  for (const node of grouped) expect(byName(after.nodes, node.name).groupIds).toEqual(expect.arrayContaining([group.id, other.id]));
  expect(after.nodes.filter(node => node.groupIds.includes(group.id))).toHaveLength(104);
  await dialog.getByRole('searchbox', { name: '搜索本组节点', exact: true }).fill(grouped[1].name);
  await expect(member(dialog, grouped[1].name)).toBeVisible();
});

test('remove confirmation can cancel, then removes only the current group without global deletion or other-group/pool loss', async ({ page, fixtures }) => {
  const group = await fixtures.group('remove-current');
  const other = await fixtures.group('remove-other');
  const [shared] = await fixtures.importNodes([fixtures.proxy('shared-node')], { groupIds: [group.id, other.id] });
  const [retained] = await fixtures.importNodes([fixtures.proxy('retained-node')], { groupIds: [group.id] });
  const dynamic = await fixtures.pool('dynamic-current', { groupIds: [group.id] });
  const otherPool = await fixtures.pool('dynamic-other', { groupIds: [other.id] });
  const dedicated = await fixtures.listener('dedicated-kept', shared.id, { kind: 'node' });
  const before = await fixtures.bootstrap();
  const requests: { method: string; path: string; body: unknown }[] = [];
  page.on('request', request => { if (request.method() !== 'GET') requests.push({ method: request.method(), path: new URL(request.url()).pathname, body: request.postDataJSON() }); });
  const dialog = await openManager(page, group);
  await member(dialog, shared.name).getByRole('button', { name: '从本组移除 ' + shared.name, exact: true }).click();
  let confirmation = dialog.getByRole('region', { name: '确认移出本组', exact: true });
  await expect(confirmation).toContainText('不删除全局节点，也不移除其他分组');
  await expect(confirmation).toContainText('订阅刷新或修改');
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await expect(confirmation.getByRole('button', { name: '取消', exact: true })).toBeFocused();
  expect(requests).toEqual([]);
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  expect((await fixtures.bootstrap()).revision).toBe(before.revision);
  await expect(member(dialog, shared.name)).toBeVisible();
  await dialog.getByRole('checkbox', { name: '选择节点 ' + shared.name, exact: true }).check();
  await dialog.getByRole('button', { name: '从本组移除', exact: true }).click();
  confirmation = dialog.getByRole('region', { name: '确认移出本组', exact: true });
  await submitBatch(page, confirmation.getByRole('button', { name: '确认移出本组', exact: true }), 'remove_groups', [shared.id], [group.id]);
  await expect(confirmation).not.toBeVisible();
  await expect(member(dialog, shared.name)).toHaveCount(0);
  await expect(member(dialog, retained.name)).toBeVisible();
  const after = await fixtures.bootstrap();
  expect(byName(after.nodes, shared.name).groupIds).toEqual([other.id]);
  expect(byName(after.nodes, shared.name).enabled).toBe(true);
  expect(byName(after.nodes, shared.name).manual).toBe(true);
  expect(after.pools).toEqual(before.pools);
  expect(after.listeners).toEqual(before.listeners);
  expect(byName(after.listeners, dedicated.name).targetId).toBe(shared.id);
  expect(requests).toHaveLength(1);
  expect(requests[0]).toMatchObject({ method: 'POST', path: '/api/nodes/batch', body: { action: 'remove_groups' } });
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await gotoWorkspace(page, '/#/pools');
  await expect(poolCard(page, dynamic.name)).toContainText('1 个已选节点');
  await expect(poolCard(page, dynamic.name).getByRole('listitem', { name: '池内节点 ' + shared.name, exact: true })).toHaveCount(0);
  await expect(poolCard(page, otherPool.name).getByRole('listitem', { name: '池内节点 ' + shared.name, exact: true })).toBeVisible();
});

test('batch disable warns globally, cancels safely and locks Escape while pending; enable preserves all memberships', async ({ page, fixtures }) => {
  const group = await fixtures.group('global-state');
  const other = await fixtures.group('global-other');
  const [shared, alreadyDisabled] = await fixtures.importNodes([fixtures.proxy('global-shared'), fixtures.proxy('global-disabled')], { groupIds: [group.id, other.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('global-outside')], { groupIds: [other.id] });
  await fixtures.patchNode(alreadyDisabled.id, { enabled: false });
  const pool = await fixtures.pool('global-pool', { groupIds: [other.id] });
  const listener = await fixtures.listener('global-dedicated', shared.id, { kind: 'node' });
  const before = await fixtures.bootstrap();
  const dialog = await openManager(page, group);
  await expect(dialog).toContainText('启用/禁用影响节点的所有分组、代理池、独享监听和中转依赖');
  await dialog.getByRole('checkbox', { name: '选择本页节点', exact: true }).check();
  await dialog.getByRole('button', { name: '禁用所选', exact: true }).click();
  let confirmation = dialog.getByRole('region', { name: '确认全局禁用', exact: true });
  await expect(confirmation).toContainText('这是全局禁用，不是仅在本组停用');
  await expect(confirmation).toContainText('中转线路也会不可用');
  await confirmation.getByRole('button', { name: '取消', exact: true }).click();
  expect((await fixtures.bootstrap()).revision).toBe(before.revision);
  await dialog.getByRole('button', { name: '禁用所选', exact: true }).click();
  confirmation = dialog.getByRole('region', { name: '确认全局禁用', exact: true });
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  await page.route('**/api/nodes/batch', async route => {
    // Hold only delivery of the real isolated API response. No fake success.
    const response = await route.fetch();
    await gate;
    await route.fulfill({ response });
  }, { times: 1 });
  const submitted = page.waitForResponse(response => new URL(response.url()).pathname === '/api/nodes/batch' && response.request().method() === 'POST');
  try {
    await confirmation.getByRole('button', { name: '确认全局禁用', exact: true }).click();
    await expect(confirmation.getByRole('button', { name: '取消', exact: true })).toBeDisabled();
    await expect(dialog.getByRole('button', { name: '关闭对话框', exact: true })).toBeDisabled();
    await page.keyboard.press('Escape');
    await expect(dialog).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(1);
  } finally { release(); }
  const disabledResponse = await submitted;
  expect(disabledResponse.ok()).toBe(true);
  expect(disabledResponse.request().postDataJSON()).toEqual({ action: 'disable', ids: [shared.id] });
  await expect(confirmation).not.toBeVisible();
  const disabledState = await fixtures.bootstrap();
  expect(byName(disabledState.nodes, shared.name).enabled).toBe(false);
  expect(byName(disabledState.nodes, outside.name).enabled).toBe(true);
  expect(byName(disabledState.pools, pool.name)).toEqual(byName(before.pools, pool.name));
  expect(byName(disabledState.listeners, listener.name)).toEqual(byName(before.listeners, listener.name));
  await submitBatch(page, dialog.getByRole('button', { name: '启用所选', exact: true }), 'enable', [shared.id, alreadyDisabled.id]);
  await expect(member(dialog, shared.name)).toContainText('已启用');
  const after = await fixtures.bootstrap();
  for (const node of [shared, alreadyDisabled]) {
    expect(byName(after.nodes, node.name).enabled).toBe(true);
    expect(byName(after.nodes, node.name).groupIds).toEqual(expect.arrayContaining([group.id, other.id]));
  }
});

test('page selection stays local while full filtered selection spans pages and preserves search-hidden choices', async ({ page, fixtures }) => {
  const group = await fixtures.group('large-selection');
  const nodes = await fixtures.importNodes(Array.from({ length: 52 }, (_, index) => fixtures.proxy((index < 45 ? 'filter-hit-' : 'filter-other-') + String(index).padStart(3, '0'))), { groupIds: [group.id] });
  const dialog = await openManager(page, group);
  const search = dialog.getByRole('searchbox', { name: '搜索本组节点', exact: true });
  const allFiltered = dialog.getByRole('checkbox', { name: '选择全部筛选结果', exact: true });
  const selection = dialog.locator('.group-selection-count');
  await search.fill(fixtures.name('filter-hit-'));
  await expect(dialog.getByRole('region', { name: '本组节点列表', exact: true }).getByRole('row', { name: /^本组节点 / })).toHaveCount(20);
  await dialog.getByRole('checkbox', { name: '选择本页节点', exact: true }).check();
  await expect(selection).toHaveText('已选 20 项');
  await expect(allFiltered).toBeChecked({ indeterminate: true });
  await dialog.getByRole('button', { name: '下一页', exact: true }).click();
  await expect(dialog.getByRole('checkbox', { name: '选择节点 ' + nodes[20].name, exact: true })).not.toBeChecked();
  await allFiltered.check();
  await expect(selection).toHaveText('已选 45 项');
  await dialog.getByRole('button', { name: '下一页', exact: true }).click();
  await expect(dialog.getByRole('checkbox', { name: '选择节点 ' + nodes[44].name, exact: true })).toBeChecked();
  await search.fill(fixtures.name('filter-other-'));
  await expect(dialog.getByRole('button', { name: '上一页', exact: true })).toBeDisabled();
  await expect(dialog.locator('.group-selection-hidden')).toContainText('45 个筛选外节点');
  await allFiltered.check();
  await expect(selection).toHaveText('已选 52 项');
  await allFiltered.uncheck();
  await expect(selection).toHaveText('已选 45 项');
  await search.fill(fixtures.name('no-results'));
  await expect(allFiltered).toBeDisabled();
  await expect(selection).toHaveText('已选 45 项');
  await search.fill('');
  await dialog.getByRole('button', { name: '禁用所选', exact: true }).click();
  await submitBatch(page, dialog.getByRole('button', { name: '确认全局禁用', exact: true }), 'disable', nodes.slice(0, 45).map(node => node.id));
  await expect(dialog.getByRole('region', { name: '确认全局禁用', exact: true })).not.toBeVisible();
  const after = await fixtures.bootstrap();
  for (const node of nodes.slice(0, 45)) expect(byName(after.nodes, node.name).enabled).toBe(false);
  for (const node of nodes.slice(45)) expect(byName(after.nodes, node.name).enabled).toBe(true);
  await dialog.getByRole('combobox', { name: '筛选节点状态', exact: true }).selectOption('enabled');
  await expect(dialog.getByRole('region', { name: '本组节点列表', exact: true }).getByRole('row', { name: /^本组节点 / })).toHaveCount(7);
  await expect(dialog.locator('.group-selection-hidden')).toContainText('45 个筛选外节点');
  await dialog.getByRole('button', { name: '清空选择', exact: true }).click();
  await expect(selection).toHaveText('已选 0 项');
});

test('390px manager and inline panels avoid overflow and source warnings do not invent provenance', async ({ page, fixtures }, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const group = await fixtures.group('mobile-' + '出口'.repeat(22));
  const [node] = await fixtures.importNodes([fixtures.proxy('source-label-' + '很长的节点名称'.repeat(12))], { groupIds: [group.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('outside-mobile-' + '很长'.repeat(25))]);
  // UI-only public projection: no source URL, subscription fetch, backup import,
  // or source attribution writes. sourceGroups are intentionally unavailable.
  await page.route('**/api/bootstrap', async route => {
    const response = await route.fetch();
    const state = await response.json() as Bootstrap;
    byName(state.nodes, node.name).sourceIds = ['fixture-public-source-id'];
    await route.fulfill({ response, json: state });
  });
  const dialog = await openManager(page, group);
  await expect(dialog).toContainText('1 个节点关联订阅');
  await expect(dialog).toContainText('订阅刷新或修改可能把节点重新加入本组');
  await expect(dialog).toContainText('不判断当前分组关系来自哪个订阅');
  await expect(dialog.getByText('fixture-public-source-id', { exact: true })).toHaveCount(0);
  await expect(member(dialog, node.name)).toContainText('尚未检测');
  await noHorizontalOverflow(page, dialog);
  await dialog.getByRole('button', { name: '添加已有节点', exact: true }).click();
  const choices = dialog.getByRole('group', { name: /^可添加节点/ });
  await expect(choices.getByRole('checkbox', { name: outside.name, exact: true })).toBeVisible();
  await noHorizontalOverflow(page, dialog);
  await dialog.getByRole('button', { name: '返回本组节点', exact: true }).click();
  await member(dialog, node.name).getByRole('button', { name: '从本组移除 ' + node.name, exact: true }).click();
  await expect(dialog.getByRole('region', { name: '确认移出本组', exact: true })).toContainText('订阅刷新或修改');
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await expect(dialog.locator('form')).toHaveCount(0);
  await noHorizontalOverflow(page, dialog);
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await page.screenshot({ path: testInfo.outputPath('group-manager-390.png'), fullPage: true });
  await dialog.getByRole('button', { name: '关闭', exact: true }).click();
  await noHorizontalOverflow(page, page.locator('.groups-panel'));
  expect(byName((await fixtures.bootstrap()).nodes, node.name).sourceIds).toEqual([]);
});

test('latency controls require applied runtime and send real requests; UI-only recent-result projection never implies connectivity', async ({ page, fixtures }) => {
  const group = await fixtures.group('latency');
  const relayProxy = fixtures.proxy('latency-relay');
  const [healthy, failed, disabled, relay, blocked] = await fixtures.importNodes([
    fixtures.proxy('latency-healthy'), fixtures.proxy('latency-failed'), fixtures.proxy('latency-disabled'), relayProxy,
    fixtures.proxy('latency-blocked', { 'dialer-proxy': relayProxy.name }),
  ], { groupIds: [group.id] });
  await fixtures.patchNode(disabled.id, { enabled: false });
  await fixtures.patchNode(relay.id, { enabled: false });
  let projection: 'real' | 'mismatched' | 'ready' = 'real';
  await page.route('**/api/bootstrap', async route => {
    const response = await route.fetch();
    const state = await response.json() as Bootstrap;
    if (projection !== 'real') {
      // Exercise frontend gates/rendering only; the server remains missing-core
      // and /api/tests is NOT mocked. These values are not real measurements.
      state.runtime = { ...state.runtime, available: true, running: true, applying: false, dirty: false, lastError: '', appliedRevision: state.revision - (projection === 'mismatched' ? 1 : 0) };
      Object.assign(byName(state.nodes, healthy.name), { status: 'healthy', latencyMs: 17, lastCheckedAt: '2026-01-01T00:00:00Z' });
      Object.assign(byName(state.nodes, failed.name), { status: 'unhealthy', latencyMs: 99, lastCheckedAt: '2026-01-01T00:00:00Z', lastError: 'fixture failure' });
    }
    await route.fulfill({ response, json: state });
  });
  const requests: unknown[] = [];
  page.on('request', request => { if (new URL(request.url()).pathname === '/api/tests') requests.push(request.postDataJSON()); });
  const dialog = await openManager(page, group);
  await expect(member(dialog, healthy.name)).toContainText('尚未检测');
  await expect(member(dialog, healthy.name).getByRole('button', { name: '测延迟 ' + healthy.name, exact: true })).toBeDisabled();
  await dialog.getByRole('checkbox', { name: '选择全部筛选结果', exact: true }).check();
  await expect(dialog.getByRole('button', { name: '检测所选延迟', exact: true })).toBeDisabled();
  expect(requests).toEqual([]);
  projection = 'mismatched';
  await syncBootstrap(page);
  await expect(member(dialog, healthy.name)).toContainText('17 ms');
  await expect(member(dialog, failed.name)).toContainText('检测失败');
  await expect(member(dialog, failed.name)).not.toContainText('99 ms');
  await expect(dialog.getByRole('button', { name: '检测所选延迟', exact: true })).toBeDisabled();
  projection = 'ready';
  await syncBootstrap(page);
  await expect(member(dialog, disabled.name).getByRole('button', { name: '测延迟 ' + disabled.name, exact: true })).toBeDisabled();
  await expect(member(dialog, blocked.name).getByRole('button', { name: '测延迟 ' + blocked.name, exact: true })).toBeDisabled();
  await expect(member(dialog, healthy.name).getByRole('button', { name: '测延迟 ' + healthy.name, exact: true })).toBeEnabled();
  const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === '/api/tests' && response.request().method() === 'POST');
  await dialog.getByRole('button', { name: '检测所选延迟', exact: true }).click();
  const response = await responsePromise;
  expect(response.status()).toBe(409);
  expect(response.request().postDataJSON().kind).toBe('latency');
  expect(response.request().postDataJSON().nodeIds.sort()).toEqual([healthy.id, failed.id].sort());
  await expect(dialog.getByRole('alert')).toContainText('代理内核未运行');
  expect(requests).toHaveLength(1);
  const actual = await fixtures.bootstrap();
  expect(actual.runtime.running).toBe(false);
  expect(actual.jobs).toEqual([]);
  expect(byName(actual.nodes, healthy.name).latencyMs).toBeNull();
});

test('latest bootstrap removes stale member/add/confirmation selections and missing groups allow closing only', async ({ page, fixtures }) => {
  const group = await fixtures.group('live-group');
  const other = await fixtures.group('live-other');
  const [returning, removed] = await fixtures.importNodes([fixtures.proxy('live-returning'), fixtures.proxy('live-removed')], { groupIds: [group.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('live-outside')]);
  let dialog = await openManager(page, group);
  await dialog.getByRole('checkbox', { name: '选择全部筛选结果', exact: true }).check();
  await expect(dialog.locator('.group-selection-count')).toHaveText('已选 2 项');
  await fixtures.patchNode(returning.id, { groupIds: [other.id] });
  await syncBootstrap(page);
  await expect(dialog.locator('.group-selection-count')).toHaveText('已选 1 项');
  await expect(member(dialog, returning.name)).toHaveCount(0);
  await fixtures.patchNode(returning.id, { groupIds: [group.id, other.id] });
  await syncBootstrap(page);
  await expect(dialog.getByRole('checkbox', { name: '选择节点 ' + returning.name, exact: true })).not.toBeChecked();
  await dialog.getByRole('button', { name: '从本组移除', exact: true }).click();
  const deletion = await fixtures.request.post('/api/nodes/batch', { data: { action: 'delete', ids: [removed.id] } });
  expect(deletion.ok()).toBe(true);
  await syncBootstrap(page);
  await expect(dialog.getByRole('button', { name: '确认移出本组', exact: true })).toBeDisabled();
  await expect(dialog.getByRole('region', { name: '确认移出本组', exact: true })).toContainText('已清理失效选择');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  await expect(dialog.locator('.group-selection-count')).toHaveText('已选 0 项');
  await dialog.getByRole('button', { name: '添加已有节点', exact: true }).click();
  await dialog.getByRole('checkbox', { name: outside.name, exact: true }).check();
  const outsideDeletion = await fixtures.request.post('/api/nodes/batch', { data: { action: 'delete', ids: [outside.id] } });
  expect(outsideDeletion.ok()).toBe(true);
  await syncBootstrap(page);
  await expect(dialog.getByRole('button', { name: '添加到本组', exact: true })).toBeDisabled();
  await expect(dialog.locator('.selection-count')).toHaveText('0 已选');
  await dialog.getByRole('button', { name: '返回本组节点', exact: true }).click();
  const renamed = fixtures.name('live-renamed');
  expect((await fixtures.request.patch('/api/groups/' + group.id, { data: { name: renamed } })).ok()).toBe(true);
  await syncBootstrap(page);
  dialog = page.getByRole('dialog', { name: '分组节点 · ' + renamed, exact: true });
  await expect(dialog).toBeVisible();
  await member(dialog, returning.name).getByRole('button', { name: '从本组移除 ' + returning.name, exact: true }).click();
  expect((await fixtures.request.delete('/api/groups/' + group.id)).ok()).toBe(true);
  await syncBootstrap(page);
  const missing = page.getByRole('dialog', { name: '分组已不存在', exact: true });
  await expect(missing).toBeVisible();
  await expect(missing.getByRole('button', { name: '确认移出本组', exact: true })).toHaveCount(0);
  await expect(missing.getByRole('button', { name: '添加已有节点', exact: true })).toHaveCount(0);
  await expect(missing.getByRole('button')).toHaveCount(2);
  await missing.getByRole('button', { name: '关闭', exact: true }).click();
  await expect(missing).not.toBeVisible();
});

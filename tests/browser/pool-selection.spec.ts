import type { Bootstrap } from '../../src/shared/types';
import { byName, editPoolDialog, expect, groupChoices, newPoolDialog, nodeChoices, poolCard, savePoolDialog, submitNewPool, test, gotoWorkspace } from './fixtures';

test('group-scoped bulk selection covers more than 100 results and preserves search-hidden choices', async ({ page, fixtures }) => {
  const group = await fixtures.group('large-scope');
  const proxies = Array.from({ length: 125 }, (_, index) => fixtures.proxy((index < 110 ? 'search-hit-' : 'other-') + String(index).padStart(3, '0')));
  const nodes = await fixtures.importNodes(proxies, { groupIds: [group.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('outside-scope')]);
  let dialog = await newPoolDialog(page);
  const name = fixtures.name('large-pool');
  await dialog.getByLabel('代理池名称', { exact: true }).fill(name);
  await groupChoices(dialog).getByRole('checkbox', { name: group.name, exact: true }).check();
  let choices = nodeChoices(dialog);
  let bulk = choices.getByRole('checkbox', { name: '全选包含节点当前结果', exact: true });
  await expect(choices.getByRole('checkbox')).toHaveCount(101);
  await expect(choices.locator('.selection-count')).toHaveText('125 已选');
  await expect(choices.getByRole('checkbox', { name: outside.name, exact: true })).toHaveCount(0);
  await expect(choices.getByRole('button', { name: '再显示 100 项（共 125 项）', exact: true })).toBeVisible();
  await expect(bulk).toBeChecked();
  // The bulk box edits membership immediately; there is no separate staging step.
  await bulk.uncheck();
  await expect(choices.locator('.selection-count')).toHaveText('0 已选');
  await expect(bulk).not.toBeChecked();
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill(fixtures.name('search-hit-'));
  await expect(choices.getByRole('button', { name: '再显示 100 项（共 110 项）', exact: true })).toBeVisible();
  await bulk.check();
  await expect(choices.locator('.selection-count')).toHaveText('110 已选');
  await choices.getByRole('button', { name: '再显示 100 项（共 110 项）', exact: true }).click();
  await expect(choices.getByRole('checkbox', { name: nodes[109].name, exact: true })).toBeChecked();
  const created = await submitNewPool(page, dialog);
  expect(created.data.pool.nodeIds).toEqual([]);
  expect(created.data.pool.groupIds).toEqual([group.id]);
  expect([...created.data.pool.excludeNodeIds].sort()).toEqual(nodes.slice(110).map(node => node.id).sort());
  await expect(poolCard(page, name)).toContainText('110 个已选节点');

  dialog = await editPoolDialog(page, name);
  choices = nodeChoices(dialog);
  bulk = choices.getByRole('checkbox', { name: '全选包含节点当前结果', exact: true });
  await expect(bulk).not.toBeChecked();
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill(fixtures.name('other-'));
  await bulk.check();
  await expect(choices.locator('.selection-count')).toHaveText('125 已选');
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill(fixtures.name('search-hit-'));
  await expect(bulk).toBeChecked();
  await bulk.uncheck();
  await expect(choices.locator('.selection-count')).toHaveText('15 已选');
  await dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true }).fill('');
  await expect(bulk).not.toBeChecked();
  await bulk.check();
  await expect(choices.locator('.selection-count')).toHaveText('125 已选');
  await savePoolDialog(dialog);
  const saved = byName((await fixtures.bootstrap()).pools, name);
  expect(saved.nodeIds).toEqual([]);
  expect(saved.excludeNodeIds).toEqual([]);
  const [newMember] = await fixtures.importNodes([fixtures.proxy('later-dynamic-member')], { groupIds: [group.id] });
  await page.reload();
  const card = poolCard(page, name);
  await expect(card).toContainText('126 个已选节点');
  await card.getByRole('searchbox', { name: '搜索 ' + name + ' 池内节点', exact: true }).fill(newMember.name);
  await expect(card.getByRole('listitem', { name: '池内节点 ' + newMember.name, exact: true })).toBeVisible();
});

test('no selected groups exposes grouped and ungrouped nodes with full search selection', async ({ page, fixtures }) => {
  const group = await fixtures.group('manual-group');
  const ungrouped = await fixtures.importNodes(Array.from({ length: 110 }, (_, index) => fixtures.proxy('needle-' + String(index).padStart(3, '0'))));
  const grouped = await fixtures.importNodes(Array.from({ length: 5 }, (_, index) => fixtures.proxy('needle-grouped-' + index)), { groupIds: [group.id] });
  const [retained] = await fixtures.importNodes([fixtures.proxy('retain-outside-search')]);
  const dialog = await newPoolDialog(page);
  const name = fixtures.name('manual-pool');
  await dialog.getByLabel('代理池名称', { exact: true }).fill(name);
  const choices = nodeChoices(dialog);
  const search = dialog.getByRole('searchbox', { name: '搜索包含节点', exact: true });
  const bulk = choices.getByRole('checkbox', { name: '全选包含节点当前结果', exact: true });
  await expect(search).toBeVisible();
  await expect(groupChoices(dialog).locator('.selection-count')).toHaveText('0 已选');
  await search.fill(fixtures.name('needle-'));
  await expect(choices.getByRole('checkbox')).toHaveCount(101);
  await expect(choices.getByRole('button', { name: '再显示 100 项（共 115 项）', exact: true })).toBeVisible();
  await bulk.check();
  await expect(choices.locator('.selection-count')).toHaveText('115 已选');
  await search.fill(grouped[0].name);
  await expect(choices.getByRole('checkbox', { name: grouped[0].name, exact: true })).toBeChecked();
  await search.fill(ungrouped[109].name);
  await expect(choices.getByRole('checkbox', { name: ungrouped[109].name, exact: true })).toBeChecked();
  await search.fill(retained.name);
  await choices.getByRole('checkbox', { name: retained.name, exact: true }).check();
  await expect(choices.locator('.selection-count')).toHaveText('116 已选');
  await search.fill(fixtures.name('needle-'));
  await expect(bulk).toBeChecked();
  await bulk.uncheck();
  await expect(choices.locator('.selection-count')).toHaveText('1 已选');
  await search.fill(fixtures.name('no-such-result'));
  await expect(search).toBeVisible();
  await expect(bulk).toBeDisabled();
  await expect(choices.getByText('没有匹配当前搜索的项目。', { exact: true })).toBeVisible();
  const result = await submitNewPool(page, dialog);
  expect(result.data.pool).toMatchObject({ nodeIds: [retained.id], groupIds: [], excludeNodeIds: [], excludeGroupIds: [] });
  await expect(poolCard(page, name).getByRole('listitem')).toHaveCount(1);
  await expect(poolCard(page, name).getByRole('listitem', { name: '池内节点 ' + retained.name, exact: true })).toBeVisible();
});

test('group bulk, any/all matching and UDP restrict the candidate scope without snapshotting membership', async ({ page, fixtures }) => {
  const a = await fixtures.group('A');
  const b = await fixtures.group('B');
  const [onlyA] = await fixtures.importNodes([fixtures.proxy('only-A')], { groupIds: [a.id] });
  const [shared, tcpOnly] = await fixtures.importNodes([fixtures.proxy('both-UDP'), fixtures.proxy('both-TCP', { type: 'http' })], { groupIds: [a.id, b.id] });
  const [onlyB] = await fixtures.importNodes([fixtures.proxy('only-B')], { groupIds: [b.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('ungrouped')]);
  const dialog = await newPoolDialog(page);
  await dialog.getByLabel('代理池名称', { exact: true }).fill(fixtures.name('scope-pool'));
  const groups = groupChoices(dialog);
  const groupBulk = groups.getByRole('checkbox', { name: '全选包含分组当前结果', exact: true });
  await groupBulk.check();
  await expect(groups.getByRole('checkbox', { name: a.name, exact: true })).toBeChecked();
  await expect(groups.getByRole('checkbox', { name: b.name, exact: true })).toBeChecked();
  await groupBulk.uncheck();
  await expect(groups.locator('.selection-count')).toHaveText('0 已选');
  await groups.getByRole('checkbox', { name: a.name, exact: true }).check();
  await groups.getByRole('checkbox', { name: b.name, exact: true }).check();
  const choices = nodeChoices(dialog);
  for (const node of [onlyA, shared, tcpOnly, onlyB]) await expect(choices.getByRole('checkbox', { name: node.name, exact: true })).toBeChecked();
  await expect(choices.getByRole('checkbox', { name: outside.name, exact: true })).toHaveCount(0);
  await expect(choices.locator('.selection-count')).toHaveText('4 已选');
  await dialog.getByRole('combobox', { name: '分组匹配关系', exact: true }).selectOption('all');
  await expect(choices.getByRole('checkbox')).toHaveCount(3);
  await expect(choices.getByRole('checkbox', { name: onlyA.name, exact: true })).toHaveCount(0);
  await expect(choices.getByRole('checkbox', { name: onlyB.name, exact: true })).toHaveCount(0);
  await dialog.getByRole('switch', { name: '仅选择 UDP 兼容节点', exact: true }).click();
  await expect(choices.getByRole('checkbox')).toHaveCount(2);
  await expect(choices.getByRole('checkbox', { name: tcpOnly.name, exact: true })).toHaveCount(0);
  await expect(choices.getByRole('checkbox', { name: shared.name, exact: true })).toBeChecked();
  await expect(dialog.getByRole('switch', { name: 'UDP 转发', exact: true })).toBeEnabled();
  await dialog.getByRole('combobox', { name: '监听协议', exact: true }).selectOption('http');
  await expect(dialog.getByRole('switch', { name: 'UDP 转发', exact: true })).toBeDisabled();
  const result = await submitNewPool(page, dialog);
  expect(result.data.pool).toMatchObject({ groupMode: 'all', udp: true, nodeIds: [], excludeNodeIds: [] });
  expect([...result.data.pool.groupIds].sort()).toEqual([a.id, b.id].sort());
  expect(result.data.listener).toMatchObject({ protocol: 'http', udp: false });
  await expect(poolCard(page, result.data.pool.name)).toContainText('1 个可转发');
});

test('only explicit group, mode or UDP scope changes prune old manual inclusions', async ({ page, fixtures }) => {
  const a = await fixtures.group('prune-A');
  const b = await fixtures.group('prune-B');
  const [shared] = await fixtures.importNodes([fixtures.proxy('prune-shared')], { groupIds: [a.id, b.id] });
  const [tcpOnly] = await fixtures.importNodes([fixtures.proxy('prune-TCP', { type: 'http' })], { groupIds: [a.id] });
  const [onlyB] = await fixtures.importNodes([fixtures.proxy('prune-only-B')], { groupIds: [b.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('prune-outside')]);
  const allIds = [shared.id, tcpOnly.id, onlyB.id, outside.id];
  const pool = await fixtures.pool('pruning-pool', { groupIds: [a.id], nodeIds: allIds });
  await gotoWorkspace(page, '/#/pools');
  let dialog = await editPoolDialog(page, pool.name);
  await expect(dialog.getByText(/有 2 个旧手动节点在当前范围外/)).toBeVisible();
  await dialog.getByRole('combobox', { name: '调度策略', exact: true }).selectOption('round-robin');
  await savePoolDialog(dialog);
  expect(byName((await fixtures.bootstrap()).pools, pool.name).nodeIds.sort()).toEqual([...allIds].sort());
  dialog = await editPoolDialog(page, pool.name);
  await groupChoices(dialog).getByRole('checkbox', { name: b.name, exact: true }).check();
  await expect(dialog.getByRole('status')).toContainText('1 个范围外手动节点');
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  expect(byName((await fixtures.bootstrap()).pools, pool.name).nodeIds.sort()).toEqual([...allIds].sort());
  dialog = await editPoolDialog(page, pool.name);
  await groupChoices(dialog).getByRole('checkbox', { name: b.name, exact: true }).check();
  await dialog.getByRole('combobox', { name: '分组匹配关系', exact: true }).selectOption('all');
  await expect(nodeChoices(dialog).getByRole('checkbox')).toHaveCount(2);
  await expect(dialog.getByRole('status')).toContainText('2 个范围外手动节点');
  await savePoolDialog(dialog);
  expect(byName((await fixtures.bootstrap()).pools, pool.name).nodeIds).toEqual([shared.id]);

  const udpPool = await fixtures.pool('udp-pruning-pool', { nodeIds: [shared.id, tcpOnly.id] });
  await page.reload();
  dialog = await editPoolDialog(page, udpPool.name);
  await dialog.getByRole('switch', { name: '仅选择 UDP 兼容节点', exact: true }).click();
  await expect(dialog.getByRole('status')).toContainText('1 个范围外手动节点');
  await savePoolDialog(dialog);
  expect(byName((await fixtures.bootstrap()).pools, udpPool.name)).toMatchObject({ udp: true, nodeIds: [shared.id] });
});

test('legacy excluded groups survive ordinary editing and clear only after an explicit save', async ({ page, fixtures }) => {
  const include = await fixtures.group('legacy-include');
  const excluded = await fixtures.group('legacy-excluded');
  const [normal] = await fixtures.importNodes([fixtures.proxy('legacy-normal')], { groupIds: [include.id] });
  const [blocked] = await fixtures.importNodes([fixtures.proxy('legacy-blocked')], { groupIds: [include.id, excluded.id] });
  const [outside] = await fixtures.importNodes([fixtures.proxy('legacy-manual-outside')]);
  const pool = await fixtures.pool('legacy-pool', { groupIds: [include.id], excludeGroupIds: [excluded.id], nodeIds: [outside.id] });
  await gotoWorkspace(page, '/#/pools');
  let dialog = await editPoolDialog(page, pool.name);
  await expect(dialog.getByRole('group', { name: /^排除分组/ })).toHaveCount(0);
  await expect(dialog.getByText('旧分组排除规则仍生效', { exact: true })).toBeVisible();
  await expect(nodeChoices(dialog).getByRole('checkbox', { name: blocked.name, exact: true })).toBeDisabled();
  await expect(nodeChoices(dialog).getByRole('checkbox', { name: normal.name, exact: true })).toBeChecked();
  await dialog.getByRole('combobox', { name: '调度策略', exact: true }).selectOption('consistent-hashing');
  await savePoolDialog(dialog);
  expect(byName((await fixtures.bootstrap()).pools, pool.name)).toMatchObject({ excludeGroupIds: [excluded.id], nodeIds: [outside.id] });
  dialog = await editPoolDialog(page, pool.name);
  await dialog.getByRole('button', { name: '清除旧规则（保存后生效）', exact: true }).click();
  await expect(nodeChoices(dialog).getByRole('checkbox', { name: blocked.name, exact: true })).toBeEnabled();
  await expect(nodeChoices(dialog).getByRole('checkbox', { name: blocked.name, exact: true })).toBeChecked();
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  expect(byName((await fixtures.bootstrap()).pools, pool.name).excludeGroupIds).toEqual([excluded.id]);
  dialog = await editPoolDialog(page, pool.name);
  await dialog.getByRole('button', { name: '清除旧规则（保存后生效）', exact: true }).click();
  await savePoolDialog(dialog);
  expect(byName((await fixtures.bootstrap()).pools, pool.name)).toMatchObject({ excludeGroupIds: [], nodeIds: [outside.id] });
  await expect(poolCard(page, pool.name).getByRole('listitem', { name: '池内节点 ' + blocked.name, exact: true })).toBeVisible();
});

test('an open editor drops deleted node references without losing existing hidden or legacy rules', async ({ page, request, fixtures }) => {
  const include = await fixtures.group('stale-include');
  const excluded = await fixtures.group('stale-legacy-exclude');
  const [deleted, retained] = await fixtures.importNodes([fixtures.proxy('stale-deleted'), fixtures.proxy('stale-retained')], { groupIds: [include.id] });
  const [hiddenManual, hiddenExcluded] = await fixtures.importNodes([fixtures.proxy('stale-manual'), fixtures.proxy('stale-excluded')]);
  const pool = await fixtures.pool('stale-pool', { groupIds: [include.id], nodeIds: [hiddenManual.id], excludeNodeIds: [hiddenExcluded.id], excludeGroupIds: [excluded.id] });
  await gotoWorkspace(page, '/#/pools');
  const dialog = await editPoolDialog(page, pool.name);
  await nodeChoices(dialog).getByRole('checkbox', { name: deleted.name, exact: true }).uncheck();
  const response = await request.post('/api/nodes/batch', { data: { ids: [deleted.id], action: 'delete' } });
  expect(response.ok()).toBe(true);
  // Refresh real bootstrap while the modal remains open; do not synthesize API state.
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await expect(dialog.getByText(/有 1 个引用的节点已被删除/)).toBeVisible({ timeout: 10000 });
  await expect(nodeChoices(dialog).getByRole('checkbox', { name: deleted.name, exact: true })).toHaveCount(0);
  await expect(nodeChoices(dialog).getByRole('checkbox', { name: retained.name, exact: true })).toBeChecked();
  const saveResponse = page.waitForResponse(result => new URL(result.url()).pathname === '/api/pools/' + pool.id && result.request().method() === 'PUT');
  await savePoolDialog(dialog);
  expect((await saveResponse).ok()).toBe(true);
  const saved = byName((await fixtures.bootstrap()).pools, pool.name);
  expect(saved).toMatchObject({ nodeIds: [hiddenManual.id], excludeNodeIds: [hiddenExcluded.id], excludeGroupIds: [excluded.id], groupIds: [include.id] });
  expect([...saved.nodeIds, ...saved.excludeNodeIds]).not.toContain(deleted.id);
});

test('node status filtering and sorting are display-only and keep hidden members on save', async ({ page, fixtures }) => {
  const [zeta, alpha] = await fixtures.importNodes([fixtures.proxy('view-zeta'), fixtures.proxy('view-alpha')]);
  await fixtures.patchNode(alpha.id, { enabled: false });
  const name = fixtures.name('filtered-view-pool');
  const dialog = await newPoolDialog(page);
  await dialog.getByLabel('代理池名称', { exact: true }).fill(name);
  const choices = nodeChoices(dialog);
  // Default name order is numeric-locale order, not fixture insertion order.
  await expect(choices.locator('.choice-item').nth(0)).toContainText(alpha.name);
  await expect(choices.locator('.choice-item').nth(1)).toContainText(zeta.name);
  const alphaBox = choices.getByRole('checkbox', { name: alpha.name, exact: true });
  const zetaBox = choices.getByRole('checkbox', { name: zeta.name, exact: true });
  await zetaBox.check();
  await alphaBox.check();
  await expect(choices.locator('.selection-count')).toHaveText('2 已选');
  await choices.getByRole('combobox', { name: '按节点状态筛选', exact: true }).selectOption('disabled');
  await expect(zetaBox).toHaveCount(0);
  await expect(alphaBox).toBeVisible();
  await expect(choices.locator('.selection-count')).toHaveText('2 已选');
  const testButton = choices.getByRole('button', { name: '测延迟', exact: true });
  await expect(testButton).toBeDisabled();
  await expect(testButton).toHaveAttribute('title', '需要内核运行且当前配置已应用。');
  const created = await submitNewPool(page, dialog);
  expect([...created.data.pool.nodeIds].sort()).toEqual([alpha.id, zeta.id].sort());
  await expect(poolCard(page, name)).toContainText('1 个可转发 · 2 个已选节点');
});

test('the latency action submits only enabled selected nodes once the applied runtime is ready', async ({ page, fixtures }) => {
  const group = await fixtures.group('pool-latency');
  const [healthy, failed, disabled] = await fixtures.importNodes([
    fixtures.proxy('pool-latency-healthy'), fixtures.proxy('pool-latency-failed'), fixtures.proxy('pool-latency-disabled'),
  ], { groupIds: [group.id] });
  await fixtures.patchNode(disabled.id, { enabled: false });
  // Dismiss the genuine missing-core prompt before projecting a ready runtime.
  await gotoWorkspace(page, '/#/pools');
  await page.route('**/api/bootstrap', async route => {
    const response = await route.fetch();
    const state = await response.json() as Bootstrap;
    // Runtime readiness is projected for this isolated UI test; /api/tests itself remains the real missing-core endpoint.
    state.runtime = { ...state.runtime, available: true, running: true, applying: false, dirty: false, lastError: '', appliedRevision: state.revision };
    await route.fulfill({ response, json: state });
  });
  await page.reload();
  const dialog = await newPoolDialog(page);
  await dialog.getByLabel('代理池名称', { exact: true }).fill(fixtures.name('latency-pool'));
  await groupChoices(dialog).getByRole('checkbox', { name: group.name, exact: true }).check();
  const choices = nodeChoices(dialog);
  await expect(choices.locator('.selection-count')).toHaveText('3 已选');
  const testButton = choices.getByRole('button', { name: '测延迟', exact: true });
  await expect(testButton).toBeEnabled();
  await expect(testButton).toHaveAttribute('title', '测试 2 个已启用节点的延迟；1 个已选节点不可测。');
  const responsePromise = page.waitForResponse(response => new URL(response.url()).pathname === '/api/tests' && response.request().method() === 'POST');
  await testButton.click();
  const response = await responsePromise;
  expect(response.status()).toBe(409);
  const body = response.request().postDataJSON() as { kind: string; nodeIds: string[] };
  expect(body.kind).toBe('latency');
  expect([...body.nodeIds].sort()).toEqual([healthy.id, failed.id].sort());
  await expect(dialog.getByRole('alert')).toContainText('代理内核未运行');
});

test('the group picker flows horizontally and shows four complete rows before scrolling', async ({ page, fixtures }) => {
  const groups = [];
  for (let index = 0; index < 13; index++) groups.push(await fixtures.group('scroll-group-' + String(index).padStart(2, '0')));
  await fixtures.importNodes([fixtures.proxy('scroll-node')], { groupIds: [groups[0].id] });
  const dialog = await newPoolDialog(page);
  const list = groupChoices(dialog).locator('.choice-list');
  await expect(list.locator('.choice-item')).toHaveCount(13);
  const metrics = await list.evaluate(element => {
    const style = getComputedStyle(element);
    return {
      clientHeight: element.clientHeight,
      scrollHeight: element.scrollHeight,
      overflowY: style.overflowY,
      columns: style.gridTemplateColumns.split(' ').length,
      rowGap: parseFloat(style.rowGap),
      verticalPadding: parseFloat(style.paddingTop) + parseFloat(style.paddingBottom),
    };
  });
  expect(metrics.overflowY).toBe('auto');
  expect(metrics.columns).toBeGreaterThanOrEqual(3);
  expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight);
  const rowHeight = await list.locator('.choice-item').first().evaluate(element => element.getBoundingClientRect().height);
  expect(rowHeight).toBeGreaterThan(30);
  expect(rowHeight).toBeLessThan(40);
  // Thirteen groups occupy five rows in three columns; the fixed window shows four complete rows and scrolls to the fifth.
  const visibleRows = (metrics.clientHeight - metrics.verticalPadding + metrics.rowGap) / (rowHeight + metrics.rowGap);
  expect(visibleRows).toBeGreaterThanOrEqual(3.8);
  expect(visibleRows).toBeLessThan(4.2);
  const lastRowVisibleAfterScroll = await list.evaluate(element => {
    element.scrollTop = element.scrollHeight;
    const box = element.getBoundingClientRect();
    const last = element.querySelector('.choice-item:last-child')!.getBoundingClientRect();
    return last.top >= box.top - 1 && last.bottom <= box.bottom + 1;
  });
  expect(lastRowVisibleAfterScroll).toBe(true);
});

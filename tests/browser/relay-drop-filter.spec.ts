import { byName, expect, test, gotoWorkspace } from './fixtures';

test('dragged local Clash file previews and imports relay dependencies', async ({ page, fixtures }) => {
  const relay = fixtures.proxy('Drop-Relay');
  const leaf = fixtures.proxy('Drop-Leaf', { 'dialer-proxy': relay.name });
  // JSON is valid YAML and stays entirely in the offline local-file importer.
  const content = JSON.stringify({ proxies: [leaf, relay] });
  await gotoWorkspace(page, '/#/nodes');
  await page.locator('.page-heading').getByRole('button', { name: '导入节点', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('button', { name: '本地文件', exact: true }).click();
  const transfer = await page.evaluateHandle(text => {
    const data = new DataTransfer();
    data.items.add(new File([text], 'relay-drop.yaml', { type: 'application/yaml' }));
    return data;
  }, content);
  const zone = dialog.getByRole('group', { name: '本地文件导入', exact: true });
  await zone.dispatchEvent('dragenter', { dataTransfer: transfer });
  await expect(zone).toHaveClass(/file-drop--valid/);
  await zone.dispatchEvent('drop', { dataTransfer: transfer });
  await transfer.dispose();
  await expect(dialog.getByText('relay-drop.yaml', { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '预览解析', exact: true }).click();
  await expect(dialog.getByText('经由：' + relay.name, { exact: true })).toBeVisible();
  await expect(dialog.getByText(/不会回退为直连/)).toBeVisible();
  await dialog.getByRole('button', { name: '确认导入', exact: true }).click();
  await expect(dialog.getByText('导入处理完成', { exact: true })).toBeVisible();
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
  const boot = await fixtures.bootstrap();
  const savedLeaf = byName(boot.nodes, leaf.name);
  const savedRelay = byName(boot.nodes, relay.name);
  expect(savedLeaf.dialerId).toBe(savedRelay.id);
  expect(savedLeaf).not.toHaveProperty('config');
  await page.getByRole('searchbox', { name: '搜索节点名称、协议、地址或分组', exact: true }).fill(leaf.name);
  await expect(page.getByRole('row').filter({ has: page.getByRole('button', { name: leaf.name, exact: true }) })).toContainText(relay.name);
});

test('exclude groups retains ungrouped nodes and explicit ungrouped filter resets', async ({ page, fixtures }) => {
  const included = fixtures.proxy('Filter-Included', { type: 'http' });
  const excluded = fixtures.proxy('Filter-Excluded', { type: 'http' });
  const ungrouped = fixtures.proxy('Filter-Ungrouped', { type: 'http' });
  const a = fixtures.name('Filter-A');
  const b = fixtures.name('Filter-B');
  await fixtures.importNodes([included], { groupNames: [a] });
  await fixtures.importNodes([excluded], { groupNames: [b] });
  await fixtures.importNodes([ungrouped]);
  await gotoWorkspace(page, '/#/nodes');
  await page.getByRole('searchbox', { name: '搜索节点名称、协议、地址或分组', exact: true }).fill(fixtures.prefix);
  await page.locator('.node-group-filter summary').click();
  const filter = page.locator('.node-group-filter');
  await filter.getByRole('group', { name: /^排除分组/ }).getByRole('checkbox', { name: b, exact: true }).check();
  const row = (name: string) => page.getByRole('row').filter({ has: page.getByRole('button', { name, exact: true }) });
  await expect(row(excluded.name)).toHaveCount(0);
  await expect(row(ungrouped.name)).toBeVisible();
  await expect(row(included.name)).toBeVisible();
  await filter.getByRole('switch', { name: '仅未分组', exact: false }).click();
  await expect(row(included.name)).toHaveCount(0);
  await expect(row(ungrouped.name)).toBeVisible();
  await filter.getByRole('button', { name: /清空|重置/ }).click();
  await expect(row(excluded.name)).toBeVisible();
  await expect(row(included.name)).toBeVisible();
});

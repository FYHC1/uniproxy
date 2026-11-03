import { expect, test, gotoWorkspace } from './fixtures';

test('the node batch action bar stays visible before and after selecting nodes', async ({ page, fixtures }) => {
  await fixtures.importNodes([fixtures.proxy('batch-one'), fixtures.proxy('batch-two')]);
  await gotoWorkspace(page, '/#/nodes');
  const bar = page.locator('.batch-bar');
  await expect(bar).toBeVisible();
  await expect(bar.getByText('选择节点以批量操作', { exact: true })).toBeVisible();
  await expect(bar.getByRole('button', { name: '启用', exact: true })).toBeDisabled();
  await bar.getByLabel('选择本页所有节点').check();
  await expect(bar.getByText('已选 2 项', { exact: true })).toBeVisible();
  await expect(bar.getByRole('button', { name: '启用', exact: true })).toBeEnabled();
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(bar).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1)).toBe(true);
});

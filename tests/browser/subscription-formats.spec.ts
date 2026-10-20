import { expect, test, gotoWorkspace } from './fixtures';

// These tests only use the missing-core server's fresh temporary database.
// All endpoints are documentation addresses. No external subscription is fetched.
test('plaintext TLS template defaults preview and commit without exposing credentials', async ({ page, fixtures }) => {
  const name = fixtures.name('tls-placeholder');
  const source = JSON.stringify({ proxies: [{ name, type: 'vmess', server: '192.0.2.71', port: 443, uuid: '11111111-1111-4111-8111-111111111111', tls: false, 'skip-cert-verify': false }] });
  const before = await fixtures.bootstrap();
  await gotoWorkspace(page, '/#/nodes');
  await page.locator('.page-heading').getByRole('button', { name: '导入节点', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '导入节点', exact: true });
  await expect(dialog.getByText(/未明确声明 UDP 时仅导入 TCP/)).toBeVisible();
  await dialog.getByLabel('节点内容', { exact: true }).fill(source);
  await dialog.getByRole('button', { name: '预览解析', exact: true }).click();
  await expect(dialog.locator('.preview-section')).toContainText('1 个节点');
  await expect(dialog.locator('.import-errors')).toHaveCount(0);
  expect(await dialog.locator('.preview-section').textContent()).not.toContain('11111111-1111-4111-8111-111111111111');
  expect((await fixtures.bootstrap()).revision).toBe(before.revision);
  await dialog.getByRole('button', { name: '确认导入', exact: true }).click();
  await expect(dialog.getByText('导入处理完成', { exact: true })).toBeVisible();
  const saved = await fixtures.bootstrap();
  expect(saved.nodes.filter(node => node.name === name)).toHaveLength(1);
  expect(saved.runtime.running).toBe(false);
  await dialog.getByRole('button', { name: '完成', exact: true }).click();
});

test('client static profile file previews nodes but never imports global policy', async ({ page, fixtures }) => {
  const name = fixtures.name('surge-static');
  const credential = 'synthetic-ui-password-canary';
  const content = '[General]\nhttp-listen = 0.0.0.0:9999\n[Proxy]\nDIRECT = direct\n' + name + ' = trojan, 192.0.2.72, 443, password=' + credential + ',sni=front.example.invalid,skip-cert-verify=false\n[Rule]\nFINAL,DIRECT\n';
  const before = await fixtures.bootstrap();
  await gotoWorkspace(page, '/#/nodes');
  await page.locator('.page-heading').getByRole('button', { name: '导入节点', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '导入节点', exact: true });
  await dialog.getByRole('button', { name: '订阅链接', exact: true }).click();
  await expect(dialog.getByText(/未明确声明 UDP 时仅导入 TCP/)).toBeVisible();
  await dialog.getByRole('button', { name: '本地文件', exact: true }).click();
  await expect(dialog.getByText(/未明确声明 UDP 时仅导入 TCP/)).toBeVisible();
  await dialog.getByLabel('选择节点配置文件', { exact: true }).setInputFiles({ name: 'synthetic-profile.conf', mimeType: 'text/plain', buffer: Buffer.from(content) });
  await dialog.getByRole('button', { name: '预览解析', exact: true }).click();
  await expect(dialog.locator('.preview-section')).toContainText('1 个节点');
  await expect(dialog.locator('.import-errors')).toHaveCount(0);
  expect(await dialog.locator('.preview-section').textContent()).not.toContain(credential);
  expect((await fixtures.bootstrap()).revision).toBe(before.revision);
  await dialog.getByRole('button', { name: '确认导入', exact: true }).click();
  await expect(dialog.getByText('导入处理完成', { exact: true })).toBeVisible();
  const after = await fixtures.bootstrap();
  expect(after.nodes.filter(node => node.name === name)).toHaveLength(1);
  expect(after.listeners).toEqual(before.listeners);
  expect(after.pools).toEqual(before.pools);
  expect(after.subscriptions).toEqual(before.subscriptions);
  expect(after.runtime.running).toBe(false);
});

test('real TLS contradictions still block import instead of being normalized away', async ({ page, fixtures }) => {
  const before = await fixtures.bootstrap();
  await gotoWorkspace(page, '/#/nodes');
  await page.locator('.page-heading').getByRole('button', { name: '导入节点', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '导入节点', exact: true });
  await dialog.getByLabel('节点内容', { exact: true }).fill(JSON.stringify({ proxies: [{ name: fixtures.name('invalid-tls'), type: 'vmess', server: '192.0.2.73', port: 443, uuid: '11111111-1111-4111-8111-111111111111', tls: false, 'skip-cert-verify': true }] }));
  await dialog.getByRole('button', { name: '预览解析', exact: true }).click();
  await expect(dialog.locator('.import-errors')).toContainText('TLS options require TLS to be enabled');
  await expect(dialog.getByRole('button', { name: '确认导入', exact: true })).toBeDisabled();
  expect((await fixtures.bootstrap()).revision).toBe(before.revision);
});

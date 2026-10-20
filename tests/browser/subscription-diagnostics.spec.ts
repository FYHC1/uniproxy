import { expect, test, gotoWorkspace } from './fixtures';

test('subscription preview explains direct download and distinguishes transport failure without saving',async({page,fixtures})=>{
 const before=await fixtures.bootstrap();
 await gotoWorkspace(page, '/#/nodes');await page.locator('.page-heading').getByRole('button',{name:'导入节点',exact:true}).click();
 const dialog=page.getByRole('dialog');await dialog.getByRole('button',{name:'订阅链接',exact:true}).click();
 await expect(dialog.getByText(/预览会由管理服务直连下载/)).toBeVisible();
 await expect(dialog.getByText(/不会发送订阅路径或令牌/)).toBeVisible();
 const canary=fixtures.name('private-url-canary');
 // Literal loopback is rejected before any socket dial; this is not an external subscription request.
 await dialog.getByLabel('订阅 URL',{exact:true}).fill('http://127.0.0.1:1/'+canary+'?token='+canary);
 await dialog.getByRole('button',{name:'预览解析',exact:true}).click();
 await expect(dialog.locator('.import-errors')).toContainText('获取或解析失败');
 await expect(dialog.locator('.import-errors')).toContainText('第 1 个订阅');
 await expect(dialog.locator('.import-errors')).toContainText(/内网|保留地址/);
 expect(await dialog.locator('.import-errors').textContent()).not.toContain(canary);
 await expect(dialog.getByText(/零节点不等于订阅格式一定不支持/)).toBeVisible();
 await expect(dialog.getByRole('button',{name:'确认导入',exact:true})).toBeDisabled();
 const after=await fixtures.bootstrap();expect(after.nodes).toEqual(before.nodes);expect(after.subscriptions).toEqual(before.subscriptions);expect(after.revision).toBe(before.revision);
 await dialog.getByRole('button',{name:'文本 / 分享链接',exact:true}).click();await expect(dialog.locator('.import-errors')).toHaveCount(0);
});

test('literal Fake-IP URL is still blocked instead of treating safety fallback as permission',async({page,fixtures})=>{
 await gotoWorkspace(page, '/#/subscriptions');await page.getByRole('button',{name:'添加订阅',exact:true}).first().click();
 const dialog=page.getByRole('dialog');const before=await fixtures.bootstrap();
 await dialog.getByLabel('订阅 URL',{exact:true}).fill('https://198.18.0.1/example-token?key=test-only');
 await dialog.getByRole('button',{name:'预览解析',exact:true}).click();
 await expect(dialog.locator('.import-errors')).toContainText(/保留地址|Fake-IP/);
 await expect(dialog.getByRole('button',{name:'确认导入',exact:true})).toBeDisabled();
 expect(await dialog.locator('.import-errors').textContent()).not.toContain('example-token');
 expect((await fixtures.bootstrap()).revision).toBe(before.revision);
});

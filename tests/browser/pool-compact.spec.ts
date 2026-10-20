import { byName, expect, poolCard, test, gotoWorkspace } from './fixtures';

test('single pool fills the workspace width and large membership stays paginated and compact',async({page,fixtures})=>{
 const nodes=await fixtures.importNodes(Array.from({length:150},(_,i)=>fixtures.proxy('compact-'+String(i).padStart(3,'0'))));
 const created=await fixtures.poolWithListener('compact-pool',{nodeIds:nodes.map(n=>n.id),strategy:'round-robin'});
 await page.setViewportSize({width:1650,height:1000});await gotoWorkspace(page, '/#/pools');
 await page.getByRole('searchbox',{name:'搜索代理池',exact:true}).fill(created.pool.name);
 const card=poolCard(page,created.pool.name);
 await expect(card).toContainText('150 个已选节点');
 await expect(card.getByRole('listitem')).toHaveCount(6);
 const list=page.getByRole('region',{name:'代理池列表',exact:true});
 const dimensions=await card.evaluate(e=>({width:e.getBoundingClientRect().width,height:e.getBoundingClientRect().height}));
 const available=await list.evaluate(e=>e.clientWidth);
 expect(dimensions.width).toBeGreaterThan(available*.9);
 expect(dimensions.height).toBeLessThan(750);
 await card.getByRole('button',{name:'下一页',exact:true}).click();
 await expect(card.getByRole('listitem',{name:'池内节点 '+nodes[6].name,exact:true})).toBeVisible();
 await card.getByRole('searchbox',{name:'搜索 '+created.pool.name+' 池内节点',exact:true}).fill(nodes[149].name);
 await expect(card.getByRole('listitem')).toHaveCount(1);
 const last=card.getByRole('listitem',{name:'池内节点 '+nodes[149].name,exact:true});
 await last.locator('summary').click();await expect(last.getByText('['+nodes[149].server+']:1080',{exact:true})).toBeVisible();
 await expect(last).toContainText('最近检测：尚无记录');
 await expect(card.getByRole('definition').filter({hasText:'150'})).toHaveCount(1);
 await card.locator('.pool-config-details summary').click();await expect(card.getByText('http://192.0.2.254/generate_204',{exact:true})).toBeVisible();
 await page.getByRole('combobox',{name:'按代理池策略筛选',exact:true}).selectOption('url-test');await expect(card).toHaveCount(0);
 await page.getByRole('button',{name:'重置筛选',exact:true}).click();await expect(card).toBeVisible();
 expect(byName((await fixtures.bootstrap()).pools,created.pool.name).nodeIds).toHaveLength(150);
});

test('many pool listeners collapse explicitly and remain accessible without growing node rows',async({page,fixtures})=>{
 const [node]=await fixtures.importNodes([fixtures.proxy('endpoint-node')]);
 const created=await fixtures.poolWithListener('endpoint-pool',{nodeIds:[node.id]});
 const second=await fixtures.listener('endpoint-second',created.pool.id);
 const third=await fixtures.listener('endpoint-third',created.pool.id,{host:'::1'});
 await gotoWorkspace(page, '/#/pools');const card=poolCard(page,created.pool.name);
 await expect(card.getByText(third.name,{exact:true})).toHaveCount(0);
 await card.getByRole('button',{name:'查看全部 3 个监听',exact:true}).click();
 await expect(card.getByText(third.name,{exact:true})).toBeVisible();await expect(card.getByText(second.name,{exact:true})).toBeVisible();
 await card.getByRole('button',{name:'收起监听',exact:true}).click();await expect(card.getByText(third.name,{exact:true})).toHaveCount(0);
 await page.setViewportSize({width:390,height:844});await expect(card).toBeVisible();
 await expect.poll(()=>page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1)).toBe(true);
});

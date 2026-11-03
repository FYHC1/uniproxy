# 新建/编辑代理池布局与节点批量栏验收（M28）

**状态：2026-11-03 本地实现并通过验证；未提交、未发布、未部署。**

## 目标与范围

用户要求继续优化代理池编辑体验：新建或编辑代理池窗口的元素排版更紧凑；监听区上移，先于成员选择；包含分组改为可滚动且固定显示约四个分组；包含节点区域更宽、更高、更易读；节点管理页的批量操作栏保持常驻。用户随后明确撤回“添加/移除暂存按钮”，最终交互仍为即时勾选、状态筛选、排序与真实延迟检测。范围为前端布局和浏览器回归，不改变代理池/监听 API、模型、SQLite schema、Mihomo 或代理安全策略。

## 实现

- [PoolDialog.tsx](<../src/web/PoolDialog.tsx>) 第 94–106 行：只在新建代理池时显示监听面板，放在名称/策略之后、成员选择之前；协议、地址、端口、自动分配与 UDP 合并为单行四列，UDP 相关说明留在面板内。第 107–124 行：成员选择成为独立区域，分组选择器与匹配关系/仅 UDP 兼容节点左右分栏；节点选择器占满整行，卡片直接显示协议、UDP 标记、健康/延迟与地址。编辑模式保留原监听配置，不在弹窗内重复编辑监听。
- [PoolDialog.tsx](<../src/web/PoolDialog.tsx>) 第 89 行：新建弹窗不再显示重复说明；编辑弹窗保留“调整成员和策略。移除节点只影响此池，关联监听保持原配置。”作为唯一范围说明。
- [poolStyles.css](<../src/web/poolStyles.css>) 第 15–33 行：监听面板四列网格、成员面板分隔线、分组/选项两列布局；分组列表固定 `max-height:177px`，按 36px 行高、5px 行距与 16px 内边距正好显示四整行，更多分组在列表内滚动；节点列表 `max-height:420px`，节点卡片最小高度 50px，健康状态独立成胶囊，避免长名称挤压。
- [poolStyles.css](<../src/web/poolStyles.css>) 第 163–178 行：≤900px 监听网格降为三列、分组选项单列；≤760px 监听两列、分组两列、节点列表 340px；≤560px 监听单列、分组单列、节点列表 300px，并保持无水平溢出。
- [ui.tsx](<../src/web/ui.tsx>) 第 52 行为 `ChoiceList` 增加受控 `columns` 与 `renderOption`；第 66 行保留批量栏“全选当前结果”常驻；第 67 行通过 `choice-list--columns` 让列数由属性驱动；第 69 行 `GroupPicker` 透传 `columns`。[styles.css](<../src/web/styles.css>) 第 315 行新增通用 `.choice-list--columns` 网格规则，不再硬编码某个弹窗的三列。
- 节点管理批量栏保持 [NodesPage.tsx](<../src/web/NodesPage.tsx>) 第 45 行的常驻渲染：未选择节点时显示“选择节点以批量操作”，操作按钮禁用；选择后显示选中数量与可用操作，不引入浮动或临时遮罩。

## 验证结果

- `pnpm run build:web`：Vite 7.3.7，4655 modules；最终资源 `internal/webui/assets/dist/assets/index-D2d1U0qR.css` 110.00kB / gzip 22.85kB、`index-bdmjANDs.js` 556.35kB / gzip 161.96kB；仅有 500kB chunk 体积提示，不是构建失败；脚本内含严格 `tsc --noEmit`。
- `pnpm test:ui`：42/42 通过，fixture 全部 PASS。
- `pnpm test:e2e`：52/52 通过（1.3m），覆盖全部页面、代理池/代理链/分组/订阅/设置/导入/监听和 390px 响应式流程。
- 定向验证：`pnpm exec playwright test tests/browser/pool-selection.spec.ts tests/browser/pool-workflow.spec.ts` 17/17 通过（30.6s）。其中 [pool-selection.spec.ts](<../tests/browser/pool-selection.spec.ts>) 第 293 行用 13 个分组断言桌面三列、四整行可见、第五行需滚动；[pool-workflow.spec.ts](<../tests/browser/pool-workflow.spec.ts>) 第 14 行断言监听区位于成员区上方、弹窗宽度≥1000、分组≥3列、节点列表高度≥400，第 35 行断言编辑弹窗同样保持四行分组与宽节点区。
- [nodes-batch.spec.ts](<../tests/browser/nodes-batch.spec.ts>) 通过：未选择时批量栏可见且按钮禁用，选择 2 个节点后显示“已选 2 项”且操作可用，390px 无页面水平溢出。
- 视觉检查：`.local/ui-theme/pool-dialog-four-groups-desktop.png`（1060×1040）与 `pool-dialog-four-groups-mobile.png`（370×1013），确认桌面四行分组、13 个分组时第 13 个在列表内滚动、节点卡片与健康标签不重叠、390px 单列无水平溢出。临时视觉 spec 已删除。

## 边界

- 只改前端布局、组件属性与浏览器测试；未改代理池/监听 API、数据模型、数据库 schema、Mihomo 版本、代理/证书/SSRF 安全策略或用户数据。
- 浏览器测试使用隔离 fixture（内核不可用）和 headless Microsoft Edge；未覆盖 Safari、Firefox、真实移动设备和全部系统缩放。
- 未运行真实节点或真实代理连通测试；本验收不证明订阅节点可用性、吞吐或长时间稳定性。
- 本轮未提交、未发布、未部署；未生成或替换任何运行程序。

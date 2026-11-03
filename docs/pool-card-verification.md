# 代理池卡片紧凑化与出口节点双列验收（M29）

**状态：2026-11-03 10:00（Asia/Shanghai）本地实现并通过验证；未提交、未发布、未部署。**

## 目标与范围

用户要求继续优化代理池卡片的空间利用率：出口节点卡片更紧凑、桌面可分两列展示，从而提高同屏节点数量；同时遵循“避免不必要的二级标签和说明，让布局来传达说明”的界面原则。范围为代理池卡片的前端结构、样式与浏览器回归，不改变代理池/监听 API、数据模型、SQLite schema、Mihomo 或代理安全策略。

## 实现

- [PoolCard.tsx](<../src/web/PoolCard.tsx>) 第 18 行把每页节点数从 6 提到 10；第 84–90 行把节点卡重构为紧凑两行结构：首行是健康状态、名称、延迟/状态、测延迟与移除按钮，次行只保留协议 Badge（及必要的禁用原因），展开后才显示地址、中转依赖、最近检测与错误。删除重复的“连接信息”二级标签，用卡片层级和展开行为表达信息关系。
- [poolStyles.css](<../src/web/poolStyles.css>) 第 121–143 行把成员区设为 `container-name:pool-member; container-type:inline-size`，节点列表改为两列 grid、6px 间距、最大高度 380px 并在内部滚动；`.pool-node-head/.pool-node-name/.pool-node-latency/.pool-node-actions` 收紧到单行，节点操作按钮桌面 24×24；`@container pool-member (max-width:430px)` 降为单列。
- [poolStyles.css](<../src/web/poolStyles.css>) 第 199 行在 `pointer:coarse` 下把节点操作按钮恢复为 44×44，保留触控可点性；[theme.css](<../src/web/theme.css>) 第 357–379 行把池卡片内边距收紧到 14px、节点列表高度统一为 380px，≤560px 为 320px。
- 搜索框占满节点区宽度；分页仍为每页 10 个，两列时正好 5 行，卡片高度不因新增节点无限拉长。

## 验证结果

- `pnpm run build:web`：Vite 7.3.7，4655 modules；最终资源 `internal/webui/assets/dist/assets/index-xZdLmcBI.css` 110.64kB / gzip 22.97kB、`index-D-RTtYM2.js` 556.38kB / gzip 161.96kB；仅有 500kB chunk 体积提示，不是构建失败；脚本内含严格 `tsc --noEmit`。
- `pnpm test:ui`：42/42 通过，fixture 全部 PASS。
- `pnpm test:e2e`：52/52 通过（1.4m），包含 `pool-compact` 的 2/2 两列/单列断言。
- 定向验证：`pnpm exec playwright test tests/browser/pool-compact.spec.ts` 2/2 通过（6.5s）；`pnpm exec playwright test tests/browser/pool-selection.spec.ts tests/browser/pool-workflow.spec.ts` 17/17 通过（29.3s）。[pool-compact.spec.ts](<../tests/browser/pool-compact.spec.ts>) 第 3–33 行断言每页 10 项、第二页从 nodes[10] 开始、首两卡同高、第三卡下移、卡片高度小于 75px，以及 525px 两列、390px 单列；[pool-regressions.mjs](<../src/web/tests/pool-regressions.mjs>) 的搜索/分页数据同步为 10+4。
- 视觉检查：`.local/pool-preview/pool-page-525.png` 为两列且卡片约 497px 宽，`.local/pool-preview/pool-page-390.png` 为单列；`src/web/tests/artifacts/pools-desktop.png`、`pools-compact-single.png`、`pools-mobile.png` 已检查，无水平溢出。截图和 fixture 只证明布局，不代表真实代理节点在线。

## 边界

- 只改前端组件、样式和浏览器测试；未改代理池/监听 API、模型、SQLite schema、Mihomo 版本或代理/证书/SSRF 安全策略。
- 浏览器验证使用隔离 fixture 与 headless Microsoft Edge；未覆盖 Safari、Firefox、真实移动设备、全部系统缩放或真实节点连通/吞吐。
- 本轮未提交、未发布、未部署；未生成或替换任何运行程序，未触碰用户数据目录。

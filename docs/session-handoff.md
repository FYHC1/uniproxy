# UniProxy 会话接续快照

**最后核对：2026-11-03 10:00（Asia/Shanghai）。v0.2.7 已正式发布并设 Latest（未部署）；M25 代理链已随 v0.2.7 发布。M26「新建代理链弹窗布局调整」已提交并推送 main（`607dc8b589836aa0a58782c41b8f5559617257d1`，main CI 41487587509 成功），包含入口监听移至代理链名称下方、节点库上方，访问认证在底部，节点库 3×3，端口空白自动/手工可填；严格 TS、生产 Web 构建、链浏览器 6/6 通过；未发布、未部署。M27「全局界面视觉刷新」已在本地完成：theme.css 亮色覆盖层、布局表达层级、42/42 UI 与 49/49 浏览器验证、0.2.8-ui-preview.1 本地预览；未提交、未发布、未部署。M28「新建/编辑代理池布局与节点批量栏」已在本地完成：监听区上移、分组固定四行内滚、节点区加宽加高、批量栏常驻，42/42 UI 与 52/52 浏览器验证通过；未提交、未发布、未部署。M29「代理池卡片紧凑化与出口节点双列」已在本地完成：每页 10 个节点、节点卡两层结构、容器查询双列/单列、触控按钮 44px，42/42 UI 与 52/52 浏览器验证通过；未提交、未发布、未部署。2026-11-03 10:00 复核：uniproxy PID18324（临时 go-build 路径，09:30:08 启动）与内核 mihomo PID24836（09:30:09 启动）正在运行，监听 127.0.0.1:9090 与 20006；本轮未启动、停止或替换该实例，操作前仍须重新核对；M27 预览 PID3976 已在 19091 验收后停止，临时数据已清理。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.7](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.7>)，四平台程序与 SHA256SUMS；[中文说明](<releases/v0.2.7.md>)已同步到 Release body（2019 字符，GitHub API 实测逐字一致）。
- M26 已提交：M26「新建代理链弹窗布局调整」已完成入口监听上移、访问认证置底、节点库 3×3、端口空白自动/手工填写；严格 TS、生产 Web 构建、链浏览器 6/6 通过，已提交并推送 main（`607dc8b589836aa0a58782c41b8f5559617257d1`，main CI 41487587509 成功），未发布/部署。见[计划 M26](<development-plan.md>)与[需求 R31](<requirements.md>)。
- M27 本地完成：全局界面视觉刷新新增 `src/web/theme.css` 最后加载，统一暖纸亮色主题、紧凑布局、隐藏重复说明和 390px 底部导航；严格 TS、生产 Web 构建、`pnpm test:ui` 42/42、`pnpm test:e2e` 49/49 通过；本地 Windows 预览 `0.2.8-ui-preview.1`，未提交/发布/部署。见[验收](<ui-refresh-verification.md>)、[计划 M27](<development-plan.md>)、[需求 R32](<requirements.md>)、[决策 D30](<decisions.md>)。
- M28 本地完成：新建/编辑代理池弹窗监听区上移、分组固定四行内滚、节点区加宽加高；节点管理批量栏常驻且未选择时操作禁用。严格 TS、生产 Web 构建、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52 通过；未提交/发布/部署。见[验收](<pool-layout-verification.md>)、[计划 M28](<development-plan.md>)、[需求 R33](<requirements.md>)、[决策 D31](<decisions.md>)。
- M29 本地完成：代理池卡片每页 10 个节点，节点卡压成健康/名称/延迟/操作首行与协议徽章次行，地址/中转/检测/错误收进展开区；成员区容器查询在 >430px 两列、≤430px 单列，列表 380px 内滚，触控按钮 44px。`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.4m）、聚焦 pool 17/17、pool-compact 2/2 通过；未提交/发布/部署。见[验收](<pool-card-verification.md>)、[计划 M29](<development-plan.md>)、[需求 R34](<requirements.md>)、[决策 D32](<decisions.md>)。
- M25 代理链已随 v0.2.7 发布：独立「代理链」页面、双栏节点选择器、2–16 跳合成出站串接、chain 监听、整链检测与原子创建；发布前 Go 全量、42/42 UI 工具、48/48 隔离浏览器、真实两跳回环链路与 Windows 候选程序验收均通过；v0.2.7 不部署。详见[代理链验收](<chain-verification.md>)与[v0.2.7 发布验收](<release-0.2.7-verification.md>)。
- 发布源码 [14cc9f570b44422b9dfc1deec94b3556749a1a05](<https://github.com/FYHC1/uniproxy/commit/14cc9f570b44422b9dfc1deec94b3556749a1a05>)（M25 代理链，49 文件），annotated tag 对象 `568f8639935eef3cbdd9090b19ff4d2b0dde53da`，解引用为该提交。
- main CI [41456237334](<https://github.com/FYHC1/uniproxy/actions/runs/41456237334>)、标签 CI [41456604601](<https://github.com/FYHC1/uniproxy/actions/runs/41456604601>)、Release [41456604526](<https://github.com/FYHC1/uniproxy/actions/runs/41456604526>) 全部成功；2026-11-02 21:52:52 +08 发布并设 Latest。
- v0.2.7 **只发布、未部署**：未停止或替换任何运行实例，未触碰 `C:\ProgramData\UniProxy` 数据目录、数据库或主密钥。当前用户运行的是本地预览 `0.2.6-chain-preview.1`。
- 当前栈仍为 Go + React/TS + SQLite 加密 payload + 外置 Mihomo1.19.32；M24 新增 VLESS XHTTP、HTTPUpgrade→WS、VMess KCP、ECH、`vcn`/`pcs`、VLESS Encryption 与 URI 别名；M25 新增 2–16 跳代理链合成出站与 chain 监听；无 schema 迁移、无内核版本变化。
- 历史：v0.2.6 于 2026-11-02 16:00:55 发布（M24，未部署）；v0.2.5 于 2026-11-02 02:27:40 发布（M22/M23，未部署）；v0.2.4 于 2026-11-01 12:18:53 发布并部署替换本机实例，停机备份在 `C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`；v0.2.3 及更早标签与资产不变。
- 2026-11-01 在 WSL2/Fedora 44 amd64 使用官方 Linux v0.2.4 与固定 Mihomo 1.19.32 完成实机运行验收，并用本地 0.2.5 候选完成 M22 修复复验（首次 apply、回环 HTTP/SOCKS、`kill -9` 清理、重启恢复、正常停止）；正式 v0.2.4 资产不含该修复。

## 当前运行服务

- 2026-11-03 10:00 实测：uniproxy PID18324（`C:\Users\h2147\AppData\Local\Temp\go-build2592475721\b001\exe\uniproxy.exe`，09:30:08 启动）与内核 mihomo PID24836（`C:\ProgramData\UniProxy\core\mihomo.exe`，09:30:09 启动）正在运行，监听 `127.0.0.1:9090` 与 20006；来源未被本轮确认，保留不动。此前 00:36 记录的无实例状态已过期。本轮未停止、替换、读取或修改用户数据库。
- v0.2.7 未部署；发布只创建 GitHub Release 与附件。M26 仅为 UI 改动，已提交并推送 main，未生成发布资产。
- 部署前置条件（若后续获授权）：确认无实例与子内核在运行，或先让其正常退出并确认内核退出 → 需要备份时停机后复制 → 替换主程序并沿用原数据目录/启动参数 → health 与 /api/core/inspect 复核；禁止在运行中复制或覆盖 SQLite 数据库，禁止混用不同库的 WAL/SHM。

## 新建代理链弹窗布局（M26，2026-11-03，已提交/未发布/未部署）

- 用户要求（m08966/m09131）：入口监听从原折叠区移到代理链名称下方、节点库上方；访问认证留最下面；去掉不必要的二级标签和说明；端口留空自动分配，也可手工填写；节点库左侧至少显示三排三列节点，尽量显示更多。
- 实现：`src/web/ChainDialog.tsx` 入口监听为单行五列（名称/协议/地址/端口/UDP），块头仅“入口监听 + 创建”开关；`CredentialsEditor` 位于 `chain-layout` 之后、fieldset 末尾；删除自动分配开关及冗余说明，仅保留非回环与 UDP 安全警告。端口状态改为字符串，空白提交 `port: 0`，手工仅接受 1024–65535 整数。
- 样式：`src/web/chainStyles.css` 新增 `.chain-entry` 面板和响应式五列/两列；`.chain-node-grid` 桌面固定三列，配合紧凑卡片与弹窗间距可在 1440×1000 内容纳 3×3 共 9 个节点，访问认证与 footer 仍可见；窄屏两列/单列。
- 测试：`tests/browser/chain-workflow.spec.ts` 新增入口监听/认证位置、9 节点 3×3、空白端口提交 0、手工端口透传、非法端口本地拦截断言；`pnpm exec playwright test tests/browser/chain-workflow.spec.ts` 6/6 通过（12.8s）。`pnpm exec tsc --noEmit --noUnusedLocals --noUnusedParameters` 与 `pnpm run build:web` 通过（最终 CSS 84.99kB/gzip17.13，JS 556.44kB/gzip162.16，仅有 Vite chunk 体积提示）。
- 视觉：`.local/chain-preview/chain-dialog-new.png`（1440×1000，3×3 九节点）与 `chain-dialog-new-390.png`（390×844，无水平溢出）已检查；临时视觉 spec 交付前删除。未改动 API/模型/schema/Mihomo/安全策略，已提交并推送 main（`607dc8b589836aa0a58782c41b8f5559617257d1`，main CI 41487587509 成功），未发布、未部署。见[计划 M26](<development-plan.md>)、[需求 R31](<requirements.md>)、[决策 D29](<decisions.md>)。

## 全局界面视觉刷新（M27，2026-11-03 本地完成，未提交/未发布/未部署）

- 用户要求整体优化 Web UI：避免不必要二级标签和说明，用布局表达层级，界面紧凑、简洁明了、亮色主题并提升完成度；范围是全部管理控制台页面、共享组件、弹窗、认证与通知，不改业务 API/模型/schema/Mihomo/安全策略。
- 实现：`src/web/theme.css` 在 `src/web/main.tsx` 第 6–9 行最后加载，覆盖原有视觉变量与组件：暖纸亮色画布、深森林墨文字、青绿/酸橙强调、serif 大标题、深色首张统计卡；统一侧栏/顶栏/页面标题/统计/表格/按钮/表单/弹窗/池与链卡片/设置/空态/认证/通知。隐藏重复 eyebrow、页面与区块 description、subtle-meta、footnote、分组提示、context banner、侧栏说明、面包屑和页脚辅助文案；保留安全提示、真实状态、空态与错误。
- 密度与响应式：压缩表格行高、卡片/弹窗间距；活动列表 396px 内滚；390px 底部导航改为 62px 七列单行，页面无水平溢出。`src/web/App.tsx` 登录错误仅在用户提交或点击本机免令牌后显示，避免初始 401 红色错误噪声。
- 验证：严格 TS、`pnpm run build:web`（Vite 7.3.7，4655 modules；CSS 106.42kB/gzip22.24，JS 556.48kB/gzip162.18，仅 chunk 提示）、`pnpm test:ui` 42/42、`pnpm test:e2e` 49/49（1.2m）通过；视觉截图在 `.local/ui-theme/`（nodes/groups/listeners/subscriptions/pools/chains/settings/nodes-mobile/nodes-selected/import/pool/chain/auth）。
- 本地预览：`bin/updates/ui-refresh/uniproxy-windows-amd64.exe`，14,227,456 bytes，SHA256 `54BFEBCD087DA9D05EF395785F4CC77BD54CF45AB0B2A4338D7BC36CFCFC38B8`，身份 `0.2.8-ui-preview.1 · commit local-ui-ref · built 2026-11-02T18:36:12Z`；隔离 19091 启动与 `/api/health` 通过。
- 边界：仅前端样式和登录错误显示时机；截图/浏览器测试使用隔离 fixture 与 headless Edge 1440/390，未覆盖 Safari/Firefox、真实移动设备和全部缩放组合；未提交、未发布、未部署。详见[验收](<ui-refresh-verification.md>)。

## 新建/编辑代理池布局与节点批量栏（M28，2026-11-03 本地完成，未提交/未发布/未部署）

- 用户要求继续压缩新建/编辑代理池弹窗：监听区上移，包含分组固定约四行并可滚动，包含节点区域更宽更高，减少二级说明；节点管理批量栏保持可见。用户随后撤回“添加/移除暂存按钮”，即时勾选、筛选、排序和真实延迟检测保持不变。
- 实现：新建时将监听面板放在名称/策略之后、成员选择之前，协议/地址/端口/自动分配/UDP 单行四列；成员区上方左侧分组选择、右侧匹配方式/仅 UDP 兼容节点，下方节点列表全宽，卡片显示协议/UDP/健康/地址。编辑模式不重复编辑监听。
- 样式：`src/web/poolStyles.css` 分组列表 `min-height:78px; max-height:177px`（36px 行高、5px 间隔、16px 内边距正好四整行），节点列表最高 420px；桌面弹窗 1060px，900/760/560px 逐级降级。`src/web/ui.tsx` 的 `ChoiceList` 增加 `columns`/`renderOption`，`src/web/styles.css` 增加通用 `.choice-list--columns`；`src/web/PoolDialog.tsx` 第 89 行新建说明为空、第 94–124 行为监听/成员两区。
- 节点批量栏：`src/web/NodesPage.tsx` 第 45 行保持常驻，未选择时显示“选择节点以批量操作”并禁用动作；新增 `tests/browser/nodes-batch.spec.ts` 锁定可见性、选择 2 个节点后的“已选 2 项”和 390px 无水平溢出。
- 测试：`tests/browser/pool-selection.spec.ts` 第 293 行 13 组断言三列、四整行、滚动；`tests/browser/pool-workflow.spec.ts` 第 14/35 行覆盖新建与编辑弹窗监听区位置、弹窗≥1000px、节点区≥400px、四行分组与编辑说明。`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.3m）、聚焦 17/17（30.6s）通过。
- 构建：`pnpm run build:web` Vite7.3.7，4655 modules；CSS `index-D2d1U0qR.css` 110.00kB/gzip22.85，JS `index-bdmjANDs.js` 556.35kB/gzip161.96，仅 chunk 提示。视觉截图 `.local/ui-theme/pool-dialog-four-groups-desktop.png`（1060×1040）与 `pool-dialog-four-groups-mobile.png`（370×1013）已检查，临时 spec 已删除。
- 边界：仅前端布局、组件属性与浏览器测试；未改代理池/监听 API、模型、schema、Mihomo 或安全策略；未覆盖真实节点连通、Safari/Firefox、真实移动设备或全部缩放。详见[验收](<pool-layout-verification.md>)、[需求 R33](<requirements.md>)、[决策 D31](<decisions.md>)。

## 代理池卡片紧凑化与出口节点双列（M29，2026-11-03 本地完成，未提交/未发布/未部署）

- 用户要求代理池卡片 UI 布局优化、提高空间利用率：出口节点卡片设计更紧凑、桌面两列展示，提高同屏节点数量；并按“避免不必要的二级标签和说明，让布局来传达说明”的原则精简节点卡。
- 实现：`src/web/PoolCard.tsx:18` 每页 10 个节点；节点卡首行为健康/名称/延迟/测延迟/移除，次行仅协议 Badge，地址、中转依赖、最近检测和错误收进 `details` 展开区，删除重复“连接信息”。`src/web/poolStyles.css:121-143` 用 `container-type:inline-size` 容器查询，成员区 >430px 两列、≤430px 单列；列表 380px 内滚，桌面按钮 24×24，`pointer:coarse` 恢复 44×44。`src/web/theme.css` 收紧池卡片内边距并统一列表高度。
- 测试：`tests/browser/pool-compact.spec.ts` 覆盖每页 10、第二页 nodes[10]、两列/单列、首两卡同高与第三卡下移；`src/web/tests/pool-regressions.mjs` 同步 10+4 分页数据。生产 Web 构建（Vite7.3.7，4655 modules；CSS 110.64kB/gzip22.97，JS 556.38kB/gzip161.96）、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.4m）、聚焦 pool 17/17（29.3s）、pool-compact 2/2（6.5s）通过。
- 视觉：`.local/pool-preview/pool-page-525.png` 两列、`pool-page-390.png` 单列；`src/web/tests/artifacts/pools-desktop.png`、`pools-compact-single.png`、`pools-mobile.png` 已检查。
- 边界：仅前端组件、样式和浏览器测试；未改代理池/监听 API、模型、schema、Mihomo 或安全策略；未覆盖真实节点连通、Safari/Firefox、真实移动设备或全部缩放。详见[验收](<pool-card-verification.md>)、[需求 R34](<requirements.md>)、[决策 D32](<decisions.md>)。

## 代理链（M25，已随 v0.2.7 发布，不含部署）

- 需求 R30：一条入口按固定顺序经过 2–16 个节点，形成可绑定监听的整链；不是代理组，不复制节点，不增加 DIRECT 回退。
- 模型/存储：`Chain{ID,Name,NodeIDs,Enabled,CreatedAt,UpdatedAt}`、`State.Chains`、`Listener.Kind=chain`、`CheckResult.ChainID`；`store.Normalize` 为旧状态补空链，无 SQLite schema 迁移。
- API：`GET/POST /api/chains`、`PUT/DELETE /api/chains/{id}`、`POST /api/chains/with-listener`（链和入口原子创建，复用 `poolListenerInput` 的校验/默认值/自动端口）、`POST /api/chains/{id}/test`。
- 编译：每个启用链生成 `c-<chainID>-<index>` 有序副本，第二跳起 `dialer-proxy` 指向上一 tag，入口只绑定末跳；任一跳缺失/禁用/中转异常或跳数越界时整链不生成、入口 REJECT，绝不缩短或直连。删除被引用节点会停用整链与关联入口，保留 NodeIDs 原顺序。
- 检测：`Manager.CheckChain` 读取 live `chainTags` 并调用内核末跳真实 delay 接口，不写节点健康；`verifyProxies` 通过 `chainHops` 核对每一跳暴露；真实两跳回环 SOCKS + 本地 HTTP origin 已证明每跳收到连接。
- 前端：独立「代理链」页与双栏创建窗口（分组/状态筛选、名称/延迟排序、最多 20 个可见节点自动测一次、单节点重测、上移/下移/移除、入口/出口标记、<2 跳提示、清空确认、可折叠入口监听）；列表支持搜索、状态、复制停用副本、启停、编辑、删除与整链测试。
- 发布前验证（2026-11-02）：`go test -mod=readonly -p 1 ./... -count=1`（显式本机内核）全通过（app 13.855s、core 15.748s、integration 9.269s）；`go vet`、gofmt、diff check 通过；两个 importer fuzz 各 10s 通过（500,855 / 115,976 execs）；严格 tsc、生产 Web 构建通过；`pnpm test:ui` 42/42；`pnpm test:e2e` 48/48（1.2m，chain-workflow 5/5）。
- 独立审查修复：链监听 UDP 校验只在链路和监听均启用时生效，节点缺失与不支持 UDP 分明；整链检测失败返回 HTTP 200 CheckResult；重复 hop/nativeProxy 错误 fail-closed；`chainHops` 核对每一跳；编辑/启停/重测清理旧结果、自动测速等待 runtimeReady、停用监听可保存失效链。新增删除节点 UDP 事务、失败检测、重复 hop 与停用监听浏览器回归。
- 边界：只使用本机回环链路测试；未验证真实公网远端、Docker、race、容量/并发或长期运行；已随 v0.2.7 发布，不部署，运行中的用户预览实例未被触碰。
- 本地预览构建（2026-11-02 19:56）：`bin/updates/chain-preview/uniproxy-windows-amd64.exe`，14,205,440 字节，SHA256 `5c8705f9451b9cceb1a7d62df738b3a3c04b96b1268d88c16a950cfced67e148`，身份 `0.2.6-chain-preview.1 · commit local-chain- · built 2026-11-02T11:56:52Z`；隔离 19091 启动、`/api/health`、嵌入首页 200 通过；该 exe 的 TestBuilt 集成测试全部 PASS（1.518s）；启动/验收/重建命令见 `bin/updates/chain-preview/使用说明.md`。本地预览，不是发布资产。
- v0.2.7 本地候选（2026-11-02 13:33Z）：`bin/updates/release-0.2.7/uniproxy-windows-amd64.exe`，14,205,440 字节，SHA256 `1d76d7d45720c08aef32e1378339809527b9a9dcbda603f64b951e473514392f`，身份 `0.2.7-pre.1`；TestBuilt 全部 PASS（4.045s），真实控制台 WM_CLOSE 两轮与失败窗口保留通过；仅本地验收，不是正式资产。

## 9router JP-US 链排障（2026-11-02 20:52，运行配置修复，无源码改动）

- 用户报 9router 代理池 20006 测试失败：`fetch failed: Client network socket disconnected before secure TLS connection was established (ECONNRESET)`。链 `JP-US`（id `dd5fcb318cdbf1df41049571091eaae1`）初始第一跳为台湾家宽01 `2b670bc2c08040972c94aa667d9a4873`、第二跳美国原生01 `f61b5adb38c97e04dae864f786765d87`；链测试 3 次均为 `context deadline exceeded`。
- 隔离复现（独立 Mihomo 1.19.32、独立端口 21006/21090、临时目录 `.local/chain-debug`，已停止并删除）：美国直连 735ms；日本→美国链 1967ms 成功；台湾→美国链连续 8s 超时；去掉 `flow: xtls-rprx-vision` 仍失败。结论是第一跳台湾节点无法承载到第二跳的嵌套 TLS/Reality 流量，不是 UniProxy 链编译错误。
- 修复：`PUT /api/chains/dd5fcb318cdbf1df41049571091eaae1` 将第一跳改为日本原生01 `f2b682f19308efc6dcd13c63e80a67af`（第二跳不变）；appliedRevision 60，4 个监听均为 listening。链测试 3 次 `ok:true`，延迟 1404/1882/1444ms；通过 20006 的 HTTP 与 SOCKS5 均返回 204。
- 9router 端：`JP-US1` 池测试于 2026-11-02 20:49:07 成功，状态由 `error/inactive` 变为 `active`，池总数 11、Active 11；此前错误已消失。
- 边界：只改运行中的链配置，不改源码/API/schema/内核版本；原第一跳 `2b670bc2c08040972c94aa667d9a4873` 可回切。未停止或替换用户主服务，未改用户数据库。

## 菜鸟 25 节点延迟诊断（2026-11-01，仅观察，未改服务）
- 用户文件 `C:\CustomFiles\CodeProjects\Project\networkTools\work\cainiao-pc\cainiao-pc-clash-25nodes.yaml` 导入后 25 节点全部 unhealthy；`/api/export/nodes` 的 TLS/WS/Host/skip-cert-verify 与源文件一致，排除导入丢字段。
- 主机当前 Clash Verge TUN 运行：Meta 适配器默认路由 NextHop `198.18.0.2`（metric 0），以太网默认路由 `192.168.2.1`；默认路由 curl 源地址为 `198.18.0.1`，香港/美国 TLS 超时，台湾返回 Microsoft Edge 400。
- 绑定以太网 `192.168.2.3` 后三个代表节点均 TLS 成功、WS Upgrade 返回 101；同版本 Mihomo 1.19.32 对照：同一节点 `interface-name: 以太网` 延迟 521ms，不绑定为 Timeout。源文件 25 节点临时绑定测试 22/25 成功（约 371–938ms），香港10、美国04、台湾02 仍 Timeout（节点侧）。
- 结论：当前全失败的主因是 Clash Verge TUN 接管了 UniProxy 内核出站默认路由，不是 UniProxy 导入/节点配置错误；关闭 TUN 或给节点入口网段加直连路由后可恢复。本次未停止/替换 PID16568/28844，未修改用户数据。

## v0.2.4 发布与部署事实

- 版本/文档提交 `72da273`（8 文件，54 新增/17 删除）；发布前生产 Web、严格 TS、39/39 UI 工具、40/40 隔离浏览器、显式本机内核的 Go 全量/vet 通过。
- 正式 Windows 程序版本 `UniProxy 0.2.4 · commit 72da273b5b81 · built 2026-11-01T04:17:59Z`；全部 TestBuilt 子测通过（4.093 秒），真实控制台 WM_CLOSE 两轮退出 0/重启及独立失败窗口退出 1 通过。
- 部署顺序：`POST /api/runtime/stop` 停内核 → 确认仅含该实例的控制台并发送受控中断 → 服务/内核退出，WAL/SHM 清理完成 → 停机备份并校验 → 启动官方 v0.2.4 → health/bootstrap 核对。
- 详细验收见[发布与部署验收](<release-0.2.4-verification.md>)。

## WSL 实机运行验收（2026-11-01，M21）

- 使用官方 Linux amd64 v0.2.4（SHA256 `c56a73ae…`）和固定 Mihomo 1.19.32（解压后 `7a0d59da…`），在 Fedora Linux 44 WSL2 x86_64 临时目录运行；仅用自建回环测试源和节点。
- 通过：health/UI+CSP、固定内核起停、回环 HTTP/SOCKS 真实转发（`UNIPROXY-WSL-E2E-OK`）、runtime stop、SIGTERM 后 WAL/SHM 清理、同数据目录重启自动应用、healthcheck、端口冲突/公网绑定保护、管理员 Token 401/200。
- 初次发现：WSL 冷启动首次 apply 可能触发 750 ms 私有认证 probe 超时，重试可恢复；`kill -9` 主程序会遗留 Mihomo 内核（当时 processguard 仅 Windows），手动 SIGINT 可清理，重启后数据与流量正常。两项均已由 M22 在本地修复并复验；正式 v0.2.4 资产不含该修复。
- 边界：仅 WSL2/Fedora/amd64；Linux arm64、裸机/其他发行版、Docker、systemd、UDP、并发/容量/吞吐和真实远端节点未测。详见[WSL 实机验收](<wsl-runtime-verification.md>)。

## Linux 修复（M22，随 v0.2.5 发布）

- 探针：私有认证 HTTP/SOCKS/UDP 检查保留，外层改为 3 秒有界重试（每次 1 秒、间隔 100ms）；`/proxies` 原生出站暴露检查同样有界重试。到 deadline 仍失败时停止内核，不降低认证标准。
- 子进程：`processguard.PrepareCommand` 在 Linux 且 `SysProcAttr == nil` 时设置 `Pdeathsig=SIGTERM`；版本探测、配置校验、正式启动三处统一调用。Windows 继续用 Job Object，其他平台 no-op；这是 best-effort，非 Windows Job 等价。
- 验证：新增 `verifyProxies` 空映射后重试、1200ms 延迟绑定/父 deadline 与 Linux 父进程强杀回归；Windows/WSL 定向及全量 Go、vet、gofmt、diff 通过。最终 Linux 候选 `.local/linux-fix/uniproxy-linux-amd64`（13,656,224 字节，SHA256 `8ff1916249b5cd6486cb3921b11aa253f1490b4110c4630b20f19460b057f2dc`）在 WSL2/Fedora 44 实跑：首次 apply 直接 HTTP 200、回环 HTTP/SOCKS 通过、`kill -9` 后内核退出且端口释放、同目录重启恢复、最终正常停止。
- 边界：仅源码/候选和 WSL2 amd64 验证；已提交 main（`ede12bb`）并随 v0.2.5 发布。Linux arm64、裸机/其他发行版、Docker、systemd、容量与真实远端节点未测。详见[WSL 实机运行与修复验收](<wsl-runtime-verification.md>)。

## 全局出站网卡（M23，随 v0.2.5 发布）

- 需求 R28：管理员可选用物理以太网、WLAN 等本机网卡出站，解决 Clash Verge TUN 经 `Meta` 适配器接管默认路由导致 UniProxy 检测全超时的环境问题。
- 实现：`Settings.OutboundInterface`（空=自动）+ 编译时非空写 Mihomo 顶层 `interface-name`；`net.InterfaceByName` 要求存在/已启用/非回环并拒绝首尾空白；新增只读 `GET /api/interfaces`；设置页「03 全局出站网络」选择器（含刷新、不可用标注、非 TUN 说明）。节点级 `interface-name` 仍被 `safeNativeValue` 拒绝。
- 关键实测：Mihomo 不校验网卡存在性（`-t` 对不存在网卡仍报成功），非法网卡只在拨号时失败且日志伪装成 DNS 解析失败，且**不回退直连**——因此只能在管理层拒绝。
- 真实出站：用 `core.Build` 生成配置起临时 Mihomo，auto/以太网/WLAN/Meta 请求均 204；`Get-NetTCPConnection -OwningProcess` 显示以太网与 WLAN 变体的出站源地址各自不同，证明顶层绑定生效。
- 验证：`go test -mod=readonly -p 1 ./... -count=1`（显式本机内核）全通过；`go vet`、gofmt 干净；39/39 UI 工具；43/43 隔离浏览器（含新增 `tests/browser/outbound-interface.spec.ts`）；`src/web/tests/verify.mjs` fixture 全绿。
- 边界：只约束自有内核出站，不创建虚拟网卡、不改系统路由/DNS，不等价 TUN 分流；Linux/macOS 网卡命名与真实远端节点连通未在本轮验证。已提交 main（`a4dd45d`）并随 v0.2.5 发布。

## 正式下载副本（v0.2.7）

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.7/uniproxy-windows-amd64.exe>) | 13,328,896 | `9eff0c7de7cb4b9c869fdbf697abeb7a0a7f236430d98c1244779cf430d41cbb` |
| [Windows arm64](<../bin/releases/v0.2.7/uniproxy-windows-arm64.exe>) | 12,412,928 | `792f4083d87a3fc8e64d36c2165beb9ce23230d0d3326509f612f9890a9fa1db` |
| [Linux amd64](<../bin/releases/v0.2.7/uniproxy-linux-amd64>) | 12,972,216 | `1d6b937714ec60aeef58d2a6b172c21d4b104a4ee10599963b2b2cfbf4e8e1f2` |
| [Linux arm64](<../bin/releases/v0.2.7/uniproxy-linux-arm64>) | 12,320,952 | `fd549e9daa6ee72f0f906924561da87a7cca3942aa75815147f2e8db4ddb2aa4` |
| [SHA256SUMS](<../bin/releases/v0.2.7/SHA256SUMS.txt>) | 360 | `cc23e3ba0b45fb8e2949be9a86b012e7c3fcdc27fd5f25113f2f6c6264ec5739` |

副本位于 `bin/releases/v0.2.7/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致；正式 Windows 程序版本为 `UniProxy 0.2.7 · commit 14cc9f570b44 · built 2026-11-02T13:51:54Z`。

## 历史正式下载副本（v0.2.6）

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.6/uniproxy-windows-amd64.exe>) | 13,244,416 | `3757fd9218d20eceaf1078930fe1dbc9931f9619d237603292f822188205190c` |
| [Windows arm64](<../bin/releases/v0.2.6/uniproxy-windows-arm64.exe>) | 12,326,400 | `f0a72a685ffab56739585f897aa68f0543f5e775ea6df59909cc06eb62ebc26f` |
| [Linux amd64](<../bin/releases/v0.2.6/uniproxy-linux-amd64>) | 12,890,296 | `0dd697146b6f8dc1c83e36c850c736417b29e523b22c25766804771bb0f4f2fb` |
| [Linux arm64](<../bin/releases/v0.2.6/uniproxy-linux-arm64>) | 12,189,880 | `f2fdca77f67c6b833b472b398e4e0c9cd574227c9e3b452cdb3abc972a805d56` |
| [SHA256SUMS](<../bin/releases/v0.2.6/SHA256SUMS.txt>) | 360 | `a2f2cb8328f41b3afb096d4ecb3256d0d05119e1a861b6b034fd6dcb7f93b43a` |

副本位于 `bin/releases/v0.2.6/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致；正式 Windows 程序版本为 `UniProxy 0.2.6 · commit dba132cb379f · built 2026-11-02T08:00:13Z`。

## 结束状态与下一步

- v0.2.7 已发布并设 Latest，**未部署**；本轮未停止或替换任何运行实例，未触碰用户数据目录、数据库或主密钥。
- 当前工作树 HEAD=`f2f462c`（M26 文档提交），M27/M28/M29 改动未提交：19 个已修改文件 + 5 个未跟踪文件（`src/web/theme.css`、`docs/ui-refresh-verification.md`、`tests/browser/nodes-batch.spec.ts`、`docs/pool-layout-verification.md`、`docs/pool-card-verification.md`）；M25/M26 已发布/提交，M27/M28/M29 不部署。
- M27 本地预览 `0.2.8-ui-preview.1`（PID3976）曾在 `127.0.0.1:19091` 用临时数据 `.local/ui-preview-data` 验收；验收后已停止并删除临时数据。2026-11-03 10:00 复核时 9090/20006 由 PID18324/PID24836 监听，19090/19091 无监听；该实例未触碰。9router 相关观察不在本轮修改范围。
- v0.2.7 发布流程已完成：main CI、tag CI、Release 四平台构建均成功，正式资产与 Windows 程序已核验；发布不等于部署。若后续获部署授权：先确认没有任何 UniProxy 实例运行，或先让其正常退出并确认内核退出；再替换可执行文件并沿用原数据目录与启动参数；禁止在运行中复制或覆盖数据库，禁止混用不同库的 WAL/SHM。
- 用户侧真实节点连通/日常使用需另行观察；发布与程序验收不等于节点可用或吞吐达标。不要因会话恢复自动启动/停止服务。
- Linux amd64 已在 WSL2/Fedora 44 用 0.2.5 候选复验（M22 修复）；v0.2.6 的 Linux 资产仅构建与摘要核对，未实机运行；Linux arm64 仅构建与摘要核对。Docker、race、容量/吞吐与真实远端连通未在本轮验证。
- 常规测试清空 UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD 与 UNIPROXY_TEST_APP_PATH/UNIPROXY_TEST_CORE_PATH。既有 Docker 忽略未排除 .local，不构建/上传本机 Docker 上下文。
- 前端产物：M25 本地验证使用 `pnpm run build:web` 生成生产包（`assets/index-D4D2OA-E.js` 556.99kB / gzip 162.15kB、`assets/index-Dj_1XGal.css` 83.99kB / gzip 16.98kB，仅非失败 chunk 提示）；dist 被 .gitignore 忽略，正式资产由 CI 重新构建。

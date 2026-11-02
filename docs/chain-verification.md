# 代理链验收（M25 / R30）

最后更新：2026-11-02（Asia/Shanghai）。状态：**已随 v0.2.7 发布（不含部署）**。本记录不是发布资产证明，也不代表运行实例已升级。

## 1. 范围与用户界面

- 目标是让入口按固定顺序经过多个上游节点，支持 2–16 跳，并把整条链作为一个可绑定的监听目标；不修改原节点、不把链伪装成代理组、不增加 DIRECT 回退。
- 左侧导航新增独立「代理链」页（位于「代理池」后、「监听端口」前）。页面提供搜索、状态筛选、链卡片（名称、启停、跳数、入口→出口、监听数、整链延迟）、编辑、复制为停用副本、启停、删除与整链测试。
- 新建/编辑窗口为双栏：左侧节点库顶部横向分组筛选，带搜索、状态筛选、名称/延迟排序和“测试可见节点”；节点卡显示名称、协议、延迟、server:port；点击卡片按顺序加入右侧链路。打开时对最多 20 个可见可测节点自动发起一次真实延迟检测，点击单个延迟可重测，复用 `POST /api/tests` 与任务状态，不伪造延迟。
- 右侧固定宽度，展示序号、节点名、协议、延迟、上移/下移/移除；首跳标“入口”、末跳标“出口”。少于 2 跳显示“链式代理至少需要 2 个节点，请再添加一个节点”；排序/筛选不改变链路顺序，筛选外已选节点保留并提示数量；清空必须二次确认。
- 底部可折叠「入口监听」：开关、名称、mixed/http/socks、地址、端口（自动分配或显式）、UDP、访问认证；新链可在同一事务创建链和监听，编辑链只更新链本身。监听页目标类型增加“代理链”，显示目标缺失/停用状态。

## 2. 模型、校验与 API

- 模型 `internal/model/types.go`：`Chain{ID,Name,NodeIDs,Enabled,CreatedAt,UpdatedAt}` 及 `State.Chains`；`CheckResult.ChainID`；`ProxyListener.Kind` 扩展为 `node|pool|chain`。存储归一化会为旧状态补空 `chains`，无 SQLite schema 迁移。
- API `internal/app/chains.go`、`internal/app/http.go`：`GET/POST /api/chains`、`PUT/DELETE /api/chains/{id}`、`POST /api/chains/with-listener`、`POST /api/chains/{id}/test`。`POST /api/chains/with-listener` 的 listener 复用池监听的 `poolListenerInput` 校验、默认值和自动端口分配；kind/targetId 由服务端固定，失败不保存半成品。
- 校验 `internal/app/validate.go`：链 2–16 跳、同链节点唯一、节点存在；启用链要求每跳启用、无 `DialerID` 且无 RelayErrors；禁用链保留已删除节点的 NodeIDs 以便修复。chain 监听目标必须存在；UDP 要求每一跳节点支持 UDP。端口、地址、非回环认证和冲突检查与现有监听一致。
- 删除语义 `internal/app/app.go`：删除被链引用的节点时停用包含它的整链及关联 chain 监听，不缩短 NodeIDs；监听配置保留，避免端口静默换绑。

## 3. 编译与真实检测

- `internal/core/compiler.go`：每条启用链按 `c-<chainID>-<index>` 生成有序 outbound 副本；第一跳正常，第二跳起写入 `dialer-proxy` 指向上一跳 tag；监听只绑定末跳。原节点配置和 tag 不被修改。
- 全有或全无：任一跳缺失、禁用、中转阻断、跳数不在 2–16、重复跳或 nativeProxy 构造失败时，整链不生成、不产生 partial `c-*`，绑定入口编译为 REJECT 并写 runtime.error；绝不缩短链或回退直连。`compiledConfig.chainTags` 记录 chainID→末跳 tag，`chainHops` 另存全部 hop tag，`internal/core/controller.go` 的 `verifyProxies` 会逐一核对每一跳的真实暴露，避免只验证末跳。
- `internal/core/checks.go`：抽出 `checkProxyTag`，新增 `Manager.CheckChain(ctx,chainID,testURL,timeout)`，读取 live chainTags 后调用内核末跳 delay 接口，返回 `CheckResult{chainId,ok,latencyMs}`，不写节点健康。API 的 `testChain` 使用管理员设置的检测 URL/超时。
- 真实链路测试 `internal/core/chain_real_test.go`：两个本机回环 SOCKS 上游 + 本地 HTTP origin，验证入口实际访问 origin、第一跳和第二跳都收到连接，证明 `dialer-proxy` 串接生效；不连接任何真实远端节点。

## 4. 实际验证结果

- Go 全量：`go test -mod=readonly -p 1 ./... -count=1`（显式 `UNIPROXY_TEST_CORE_PATH=C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe`）全部通过（app 9.322s、core 9.002s、integration 3.927s）；app/core/integration 包括链 API、编译失败闭合、真实两跳回环链路。
- 前端：严格 `tsc --noEmit --noUnusedLocals --noUnusedParameters` 通过；`pnpm run build:web` 通过（Vite 7.3.7，`index-D4D2OA-E.js` 556.99kB / gzip 162.15kB、`index-Dj_1XGal.css` 83.99kB / gzip 16.98kB，仅非失败 chunk 提示）；`pnpm test:ui` 的 42/42 工具单测和全部 fixture 场景通过，零未捕获浏览器错误。
- 浏览器：全量 `pnpm test:e2e` 48/48 通过（1.1m）；`tests/browser/chain-workflow.spec.ts` 5/5 通过——有序链+原子监听、编辑监听目标、删除保留停用监听、单跳拒绝/筛选外保留/停用节点拒绝、复制停用副本、监听页 chain 目标、清空确认、停用监听可保存失效链。
- 独立只读审查后补强并复验：删除节点事务只在链路和监听均启用时校验 chain UDP，节点缺失与不支持 UDP 分别报错；整链检测失败以 HTTP 200 CheckResult 返回，不再映射为 500；重复 hop/nativeProxy 错误整链 fail-closed；编辑、启停或重测时清理旧结果；自动测速只在 runtimeReady 后对可见可测节点发起一次；已停用监听允许保存失效链路。新增 `TestDeletingChainNodeDisablesWholeRouteAndUDPEntry`、`TestChainTestFailureIsReturnedAsCheckResult`、重复 hop 与停用监听浏览器回归。
- 视觉检查：列表页（两链、统计、筛选）与创建窗口（双栏、节点卡、三跳顺序、入口监听展开）在隔离 fixture 中布局正常；截图仅本地 `.local/chain-preview/`，不进入源码或用户数据。
- 真实内核测试仅用本机回环数据；没有访问用户订阅、节点凭据、数据库或运行实例。

## 5. 边界与未验证项

- 链的 TCP 入口可绑定；UDP chain 入口仍只允许回环，并要求每一跳节点支持 UDP。固定内核的 UDP 认证限制不变。
- 不承诺真实公网远端节点的可用性、延迟或吞吐；本机回环测试只证明转发路径。
- Linux arm64、Docker、race、容量/并发、长期运行和跨平台实跑未在本轮执行；浏览器 fixture 不替代真实内核链路证据。
- 已随 v0.2.7 发布；正式资产与 Windows 程序验收见[发布记录](<release-0.2.7-verification.md>)。本版不部署，运行实例保持原状。

## 6. 关键文件

- 模型/校验：[types.go](<../internal/model/types.go>)、[validate.go](<../internal/app/validate.go>)、[chains.go](<../internal/app/chains.go>)、[app.go](<../internal/app/app.go>)、[http.go](<../internal/app/http.go>)
- 编译/检测：[compiler.go](<../internal/core/compiler.go>)、[controller.go](<../internal/core/controller.go>)、[checks.go](<../internal/core/checks.go>)
- 前端：[ChainsPage.tsx](<../src/web/ChainsPage.tsx>)、[ChainDialog.tsx](<../src/web/ChainDialog.tsx>)、[chainSelection.ts](<../src/web/chainSelection.ts>)、[chainStyles.css](<../src/web/chainStyles.css>)、[ListenerForms.tsx](<../src/web/ListenerForms.tsx>)
- 测试：[chain_api_test.go](<../internal/app/chain_api_test.go>)、[chain_delete_test.go](<../internal/app/chain_delete_test.go>)、[compiler_chain_test.go](<../internal/core/compiler_chain_test.go>)、[chain_real_test.go](<../internal/core/chain_real_test.go>)、[chain-workflow.spec.ts](<../tests/browser/chain-workflow.spec.ts>)

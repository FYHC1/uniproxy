# 测试与验收

本文列出测试方式；具体已执行结果在交付说明中记录，不把计划当作已通过。

## 自动化

- go test ./...：解析、状态、加密持久化、订阅协调、安全边界、编译器与运行管理。
- go vet ./...：Go 静态检查。
- pnpm run build:web：TypeScript 全量检查及生产构建。
- pnpm test:e2e：启动隔离测试服务并执行浏览器功能流（不写用户数据）。
- pnpm test:ui：前端工具单元测试+内存构建fixture浏览器测试，不启动服务器。
- UNIPROXY_TEST_CORE_PATH：指向已验证的 Mihomo 后执行真实内核测试；没设置时这些测试跳过而非伪造结果。
- 真实多引擎 sidecar E2E（本地）：设置 `UNIPROXY_E2E_REAL=1`、`UNIPROXY_E2E_XRAY`、`UNIPROXY_E2E_MIHOMO` 后运行 `go test ./internal/e2e -run TestRealAppProcessThreeProtocols -count=1 -v -timeout 5m`；未设置显式跳过。测试构建临时 `uniproxy`、使用临时数据目录与随机管理端口，启动真实 Mihomo/Xray，不读取用户数据。
- 真实远端订阅 E2E：设置 `UNIPROXY_E2E_REMOTE_SUB`（例如 `http://210.109.205.7:20080/links-xray.txt`）、`UNIPROXY_E2E_XRAY`、`UNIPROXY_E2E_MIHOMO` 后运行 `go test ./internal/e2e -run '^TestRealRemoteSubscription$' -count=1 -v -timeout 5m`；可选 `UNIPROXY_E2E_REMOTE_TARGET`、`UNIPROXY_E2E_REMOTE_NODES`、`UNIPROXY_E2E_REMOTE_NAMES`。测试会实际抓取远端订阅、启动本地 app/Mihomo/Xray 并访问远端目标，不读取用户数据库或凭据；未设置 `UNIPROXY_E2E_REMOTE_SUB` 时显式跳过。

## Windows 启动、关闭与同目录重启

- [CLI/启动测试](<../cmd/uniproxy/main_test.go>)与[诊断测试](<../cmd/uniproxy/diagnostics_test.go>)覆盖参数脱敏、绑定端口失败不创建数据、错误路径、日志有界/回退/无覆盖和同目录取消后再启。
- [损坏回归](<../internal/store/corruption_test.go>)只截断合成数据库，断言错误分类及原库/密钥不被清空；[短暂锁](<../internal/store/restart_test.go>)验证busy_timeout必须先于WAL初始化。
- [Job测试](<../internal/processguard/subprocess_windows_test.go>)主测试进程不加入Job，只启动隔离helper；验证强杀/正常退出/嵌套Job清理子孙，原始inherit-all创建验证句柄不继承。
- [构建后Windows测试](<../tests/integration/startup_windows_test.go>)需显式 APP_PATH/CORE_PATH；三轮强杀主程序而不先Stop内核，同一临时配置/端口再启，验证子进程退出与版本不丢；错误日志写隔离cache，不污染用户目录。
- [真实控制台脚本](<../scripts/test-windows-console.py>)需Windows+Python标准库：用独立隐藏控制台和临时数据启动候选，以精确已创建PID附加核对控制台身份，然后发WM_CLOSE（不同于强杀/CTRL_BREAK）。两轮关闭/重启及无参数错误窗口保留；不向用户控制台发信号。命令：`python scripts/test-windows-console.py <uniproxy.exe> <mihomo.exe>`。只证明本机传统控制台路径，未覆盖所有Terminal/ConPTY宿主。
- 用户数据库诊断/恢复不是常规测试。必须另行授权，先保留原证据，在副本检查完整性/解密，恢复时全部停机；不得复制运行中的主库或删除WAL来让测试通过。

## 在线内核安装验证

- [下载器测试](<../internal/coreinstall/install_test.go>)以注入 transport/摘要验证四平台归档、来源/TLS重定向边界、校验先于解压、有界大小/取消清理、无覆盖文件发布竞态；默认不联网。
- [内核安装生命周期测试](<../internal/core/installation_test.go>)与[管理 API 测试](<../internal/app/core_install_test.go>)覆盖自动发现、重复/运行保护、真实字节状态、安装不自动应用、关闭取消等待、鉴权/严格空对象与重新检测。
- 浏览器隔离服务对安装 POST 固定返回 409，客户端也默认拦截；真正在线安装只能在单独、明确的 opt-in 集成测试中执行。
- [真实下载 API 测试](<../tests/integration/core_install_test.go>)仅在 `UNIPROXY_TEST_CORE_DOWNLOAD=1` 时访问官方发布并安装到临时目录，经真实版本检测后显式应用空配置、停止清理。不读取订阅/用户配置，不使用公网代理节点；常规测试该变量保持为空。

~~~powershell
$env:UNIPROXY_TEST_CORE_DOWNLOAD = "1"
go test ./tests/integration -run '^TestOnlineCoreDownloadThroughManagementAPI$' -count=1 -v
# 当前 shell 后续普通测试前清除此 opt-in
$env:UNIPROXY_TEST_CORE_DOWNLOAD = ""
~~~

## 订阅格式兼容与授权样例离线验证

- [TLS回归](<../internal/importer/tls_compat_test.go>)覆盖false/空SNI占位、显式安全冲突、URI/VMess/sing-box/插件、稳定指纹和中转；[客户端文本测试](<../internal/importer/client_formats_test.go>)覆盖有界静态段/引用拒绝/参数与引号；[URI元数据测试](<../internal/importer/subscription_uri_test.go>)覆盖STATUS与同值SNI别名。
- [API回归](<../internal/app/subscription_compat_test.go>)采用注入HTTP transport和临时SQLite/fakeCore，验证7节点预览/保存/刷新、无秘密响应、强制错误刷新也不清理。
- [集成验证](<../tests/integration/subscription_formats_test.go>)的合成方言用例需显式CORE_PATH进行Mihomo `-t`；不启动上游节点。私有样例仅在用户授权后通过 `UNIPROXY_TEST_SUBSCRIPTION_MANIFEST` 指向仓库外JSON清单（File绝对路径、Nodes/Errors预期数、可选ErrorContains），执行 `go test ./tests/integration -run "^TestPrivateSubscriptionFilesOffline$" -count=1 -v`。文件读取有界，测试不下载URL、不打印文件路径/内容/凭据，不启动节点；有CORE_PATH时仅对零错误样例做配置校验。常规测试该变量必须为空。[真实回环TLS观察](<../tests/integration/subscription_tls_test.go>)仅本机ClientHello，服务端故意中止握手，断言显式SNI未被WS Host替换且测速不能假成功。[候选程序验收](<../tests/integration/subscription_binary_test.go>)在临时data/cache、显式不存在core下直接验证暂存编译程序的6种格式/结构文档预览与保存及活动冲突拒绝。
- [浏览器用例](<../tests/browser/subscription-formats.spec.ts>)只用临时缺失core服务器和合成文档地址：TLS占位导入、静态CONF预览/保存不采用全局策略、真实TLS冲突阻止导入。具体本轮结果见[验收](<subscription-format-verification.md>)。

## 全局出站网卡验证

- 单元：[outbound_interface_test.go](<../internal/app/outbound_interface_test.go>) 覆盖空值/不存在/未启用/回环/首尾空白/可用网卡、PATCH 往返与拒绝、未知字段、`GET /api/interfaces` 结构与链路本地过滤；[compiler_interface_test.go](<../internal/core/compiler_interface_test.go>) 覆盖空值不写 key、非空写顶层、去空白、节点级 `interface-name` 仍拒绝。
- 真实内核：[interface_real_test.go](<../internal/core/interface_real_test.go>) 需 `UNIPROXY_TEST_CORE_PATH`，证明 Mihomo `-t` 对未知网卡也接受，故必须管理层校验。
- 真实出站（手工，未纳入 CI）：用 `core.Build` 生成配置改写端口后启动临时 Mihomo，比对 auto/以太网/WLAN/Meta 的 HTTP 结果，并用 `Get-NetTCPConnection -OwningProcess` 核对出站源地址随 `interface-name` 变化；不存在的网卡应表现为拨号失败而非直连回退。
- 浏览器：[outbound-interface.spec.ts](<../tests/browser/outbound-interface.spec.ts>) 在隔离 19090 服务上断言下拉选项与 `GET /api/interfaces` 的启用非回环网卡逐项一致、只暴露 name/up/loopback/addresses、保存真实网卡后 bootstrap 持久化并可改回自动、不存在网卡 PATCH 返回 400 且不写库；用例自行清理 pin，不依赖其他 spec 的残留状态。[verify.mjs](<../src/web/tests/verify.mjs>) 的 fixture 覆盖网卡消失后的「当前不可用」标注与陈旧值不被静默改写。
- 该设置只改变自有内核的出站源网卡，不是 TUN，不创建虚拟网卡，不改系统路由/DNS；浏览器用例只验证交互与持久化，不验证真实网卡出站效果，不能替代上述手工核对。

## 代理链验证

- 模型/API：[chain_api_test.go](<../internal/app/chain_api_test.go>)、[chain_delete_test.go](<../internal/app/chain_delete_test.go>) 与 [chain_test.go](<../internal/app/chain_test.go>) 覆盖 2–16 跳、重复/缺失/禁用/中转错误、原子创建链+监听、默认端口分配、非法/冲突入口、整链检测、编辑/删除、删除节点停用整链且保留原顺序，以及带 UDP 入口的删除事务与检测失败返回 HTTP 200 CheckResult；所有 API 响应不返回凭据。
- 编译：[compiler_chain_test.go](<../internal/core/compiler_chain_test.go>) 断言 `c-<chainID>-<index>` 顺序、第二跳 `dialer-proxy`、原节点不变、监听末跳、无 partial 链、失败入口 REJECT 与 UDP 逐跳检查。
- 真实内核：[chain_real_test.go](<../internal/core/chain_real_test.go>) 需 `UNIPROXY_TEST_CORE_PATH`，用两个回环 SOCKS 上游和本地 HTTP origin 验证 `CheckChain` 成功且两跳都收到真实连接；只证明本机链路，不证明公网节点可用性。
- 前端/浏览器：[utils.test.mjs](<../src/web/tests/utils.test.mjs>) 覆盖链候选/顺序纯函数；[chain-workflow.spec.ts](<../tests/browser/chain-workflow.spec.ts>) 的 5 例覆盖有序链+原子监听、单跳拒绝、筛选外保留、复制停用副本、监听页 chain 目标、清空确认，以及停用监听保存失效链。fixture 的延迟必须来自真实检测请求，没有内核时显示未检测，不能伪造延迟。
- 已知边界：链只支持自有内核的 TCP 入口；UDP 要求每一跳 UDP；真实公网远端链路、跨平台实跑、Docker、race 和容量未在本轮验证。
## 全局界面视觉刷新（M27）

- 视觉实现：[theme.css](<../src/web/theme.css>) 在 [main.tsx](<../src/web/main.tsx>) 最后加载，覆盖原有视觉变量与组件样式；[App.tsx](<../src/web/App.tsx>) 的登录错误仅在用户提交或尝试本机免令牌后显示。不改业务 API、模型、schema、Mihomo 或安全策略。
- 构建与静态检查：`pnpm run build:web`（Vite 7.3.7，4655 modules；CSS 106.42kB/gzip22.24，JS 556.48kB/gzip162.18，仅 500kB chunk 提示）、严格 `tsc --noEmit --noUnusedLocals --noUnusedParameters` 通过。
- 交互与响应式：`pnpm test:ui` 42/42（认证、空态、真实状态、375/768/1024 等 fixture）；`pnpm test:e2e` 49/49（1.2m，覆盖全部页面、弹窗、链/池/分组/订阅/设置/导入/监听与 390px 流程）。
- 视觉检查：headless Microsoft Edge 1440×1000 与 390×844 截图，包含 nodes/groups/listeners/subscriptions/pools/chains/settings-runtime/settings-detection/settings-backup/nodes-mobile/nodes-selected/import-dialog/pool-dialog/chain-dialog/chain-dialog-mobile/auth；检查依据为布局层级、信息密度、对比度、无水平溢出和关键操作可见性，不替代真实代理连通测试。
- 本地预览程序：`bin/updates/ui-refresh/uniproxy-windows-amd64.exe`，14,227,456 bytes，SHA256 `54BFEBCD087DA9D05EF395785F4CC77BD54CF45AB0B2A4338D7BC36CFCFC38B8`，身份 `0.2.8-ui-preview.1`；隔离 19091 启动后 `/api/health` 返回 `{"ok":true,"version":"0.2.8-ui-preview.1"}`。
- 边界：未覆盖 Safari/Firefox、真实移动设备和全部缩放组合；截图与 fixture 使用隔离数据/内核不可用服务，不证明代理连通或性能；未提交/发布/部署。详见[验收](<ui-refresh-verification.md>)。

## 代理池布局与节点批量栏（M28）

- 布局回归：[pool-selection.spec.ts](<../tests/browser/pool-selection.spec.ts>) 以 13 个分组断言桌面三列、滚动区可见约四整行、第五行需滚动、滚动后末项可见；[pool-workflow.spec.ts](<../tests/browser/pool-workflow.spec.ts>) 覆盖新建和编辑弹窗的监听区位于成员区上方、弹窗≥1000px、分组≥3列、节点列表≥400px、编辑说明可见。
- 批量栏：[nodes-batch.spec.ts](<../tests/browser/nodes-batch.spec.ts>) 断言未选择时整行可见且按钮禁用、全选 2 个节点后显示“已选 2 项”且操作可用、390px 无页面水平溢出。
- 执行：`pnpm run build:web`（Vite 7.3.7，4655 modules；CSS 110.00kB/gzip22.85，JS 556.35kB/gzip161.96，仅 chunk 提示）、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.3m）、聚焦弹窗 17/17（30.6s）通过。
- 边界：仅前端布局和浏览器回归，不改代理池/监听 API、模型、schema、Mihomo 或安全策略；headless Edge 与隔离 fixture 不证明真实节点连通、跨浏览器或全部缩放；未提交/发布/部署。详见[验收](<pool-layout-verification.md>)。

## 代理池卡片紧凑化与出口节点双列（M29）

- [pool-compact.spec.ts](<../tests/browser/pool-compact.spec.ts>) 断言每页 10 个节点、第二页从 nodes[10] 开始、首两卡同高/第三卡下移/卡片高度<75px，并在 525px 两列、390px 单列；[pool-regressions.mjs](<../src/web/tests/pool-regressions.mjs>) 同步搜索 10+4 分页数据。
- 执行：`pnpm run build:web`（Vite 7.3.7，4655 modules；CSS 110.64kB/gzip22.97，JS 556.38kB/gzip161.96，仅 chunk 提示）、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.4m）、聚焦 pool 17/17（29.3s）、pool-compact 2/2（6.5s）通过。
- 边界：只改前端组件、样式与浏览器测试；headless Edge 与隔离 fixture 不证明真实节点连通、跨浏览器或全部缩放；未提交/发布/部署。详见[验收](<pool-card-verification.md>)。

## 必测功能

1. 各协议链接/Clash/sing-box 配置导入，非法参数及不支持变体明确拒绝，错误不泄漏密钥。
2. 批量导入重复节点去重，多分组静态/动态成员、并集/交集/排除。
3. mixed同时支持HTTP与SOCKS5；单协议不接受另一种协议；TCP CONNECT与SOCKS5 UDP真实本地回显。
4. 池原生四策略、一致哈希认证键、URLTest检测、空池/禁用节点/错误凭据fail-closed。
5. 订阅网络失败不删节点；正常移除清理引用；多来源/人工节点不误删；异常删减需确认。
6. 配置校验失败不提交，应用失败显示待生效并保留运行版；端口冲突；正常reload已有TCP隧道继续传输。
7. 管理Token、Host/Origin校验、订阅SSRF/大小限制、备份安全、API响应凭据脱敏。
8. 重启恢复配置、取消检测任务、并发限制、数据加密round-trip。

## 负载验证（需在目标硬件执行）

用500个节点配置验证导入/列表/分组与编译耗时，再分别增加并发连接和UDP流。观察CPU、RSS、句柄、错误率、p95延迟及吞吐。内核文件数不是吞吐指标；不得将本地回显测试作为公网多协议/带宽性能结论。

真实上游协议互通和公网吞吐需要用户提供受控测试节点。测试不访问或修改浏览器系统代理，不修改防火墙或用户全局网络配置。

## 多引擎 sidecar 真实流程验证（2026-11-04）

- Windows 11 amd64：Xray 26.10.31 + Mihomo v1.19.32，`TestRealAppProcessThreeProtocols` PASS 2.74s；VLESS Encryption、VMess XHTTP、Reality 三路经真实 app → Mihomo → sidecar SOCKS5 → Xray 得到 `real-e2e-ok`，同 dataDir 重启恢复，强杀后 4 个占用端口释放。
- WSL2 Fedora 44 Linux amd64：同一真实进程测试 PASS 2.12s，四项日志一致；`TestPhase0XrayPOC` PASS 0.38s；`go test ./... -count=1 -p 1` 全通过。
- kr-vps 真实远端订阅 E2E（2026-11-04 16:40–16:41）：Alpine Linux 3.24.2 容器 `210.109.205.7`，仅公网 21443 映射到容器 Xray；`TestRealRemoteSubscription` 使用 `http://210.109.205.7:20080/links-xray.txt` 与目标 `http://210.109.205.7:20080/ok-remote.txt`，VLESS Encryption PASS 10.23s、VMess XHTTP PASS 6.12s、Reality PASS 5.66s；每协议首次导入/强制刷新/应用重启三次均返回 `remote-e2e-ok`，sidecar 状态为 `engine=xray version=26.10.31 running=true`。
- 前端全量回归（2026-11-04 16:49）：`pnpm typecheck`、`pnpm test:ui`（44/44 单测 + 全部浏览器回归）、`pnpm build:web`（Vite 7.3.7，4656 modules；CSS 112.77kB、JS 566.93kB，仅 chunk 提示）、`pnpm test:e2e` 52/52（1.3m）通过。
- sidecar 安装器：`internal/sidecarinstall` 单测覆盖四平台资产/摘要、白名单解压、无覆盖发布与失败清理；2026-11-04 临时 `TestLiveGitHubInstall` 使用真实 `Xray-windows-64.zip` 16,623,726 bytes（SHA256 与 windows-amd64 pin 一致），在注入 `gh api` 真实 release metadata 的条件下完整走生产 `install(...)`，PASS 4.60s。匿名 GitHub API 配额耗尽时直接 API 请求返回 403（Remaining=0，Reset 17:57:56），该边界仍存在；Linux 实机安装未验证。
- 边界：Windows/WSL 只验证本机回环合成上游；kr-vps 仅单端口单节点顺序协议，未验证三协议并发、多端口映射、UDP、公网带宽、容量/长期运行、其他发行版或裸机；真实 GitHub 资产下载已用 Windows amd64 官方 ZIP 验证，匿名 API 配额耗尽时 403，Linux 实机安装仍待验证。详见[真实流程验证](<sidecar-real-process-verification.md>)。
## 已有执行记录（历史，不是每轮自动重测）

首版证据见[历史验收](<verification.md>)，中转修复证据见[修复验收](<relay-fix-verification.md>)。20:31文档整理轮没有重跑产品测试；随后代理池优化轮已重新执行完整相关验证，见[代理池优化验收](<pool-workflow-verification.md>)。

- Windows Go单元/集成测试、go vet、TypeScript及Vite生产构建。
- 固定Mihomo真实HTTP/SOCKS/mixed认证链路、UDP数据报、真实URLTest与限量下载、四种原生池策略路由、正常reload/端口交换保留TCP长连接、空池/禁用/删除目标拒绝新流量。
- API导入→自动应用→实际上游转发→检测测速→禁用断新流量的全链路。
- 各协议代表配置（含Reality、WS、gRPC）经过真实内核 -t 校验；不等于已测试远程协议握手。
- 后端安全复核修复6项中等级问题，并加入来源归属、旧版本检测丢弃、刷新与备份恢复竞态、重定向Referer、批量端口非法地址、超过12MiB备份恢复回归。
- 首版Playwright为5项，修复后增加拖拽/排除筛选至7项；连接真实Go API和临时SQLite，但测试服务故意不安装内核，不能当真实代理链路证据。前端工具测试由9项增至21项，另有内存构建的fixture浏览器回归。
- UniProxy及安装器Linux amd64/arm64交叉编译成功；没有相应Linux主机实跑证据。Docker CLI不可用，未实际构建/启动容器。
- Race detector受当前CGO=0且无C编译器限制，未执行；普通并发单元测试不替代race。
- 导入解析与SSH私钥解析分别通过30秒fuzz测试。

## 代理池优化轮（2026-10-19 22:23）

- 严格TS、30项工具测试、fixture浏览器回归、20项真实隔离API浏览器流程全部通过；125项选择/110项搜索全选、旧排除、动态纳入及并发删除草稿均覆盖。
- Go全量使用固定真实core与本轮Windows暂存程序，通过后执行go vet全量通过。新[本机工作流](<../tests/integration/pool_workflow_test.go>)验证原子创建认证入口、真实测延迟、移除最后节点拒绝及同端口重新纳入。
- [API事务测试](<../internal/app/pools_test.go>)、[端口测试](<../internal/app/pool_ports_test.go>)及[成员操作测试](<../internal/app/pool_members_test.go>)覆盖原子保存/失败/待应用、手动停止、端口和认证安全、幂等移除及关联对象不变。
- 私有导入变量保持空，本轮未测用户原始文件或真实远端节点；Linux两架构仅交叉构建，Docker/race/吞吐限制仍未验证。

## 订阅与分组体验轮（2026-10-19 23:20）

- 严格TS、31项工具、fixture浏览器和32项真实Go/API浏览器全部通过。
- safehttp23函数与子场景（19新增）覆盖Fake-IP/DoH/TLS/固定拨号/取消/重定向/大小/HTML及fec0拒绝；app新增14函数覆盖订阅安全错误传播/保留来源、分组归属与全局状态。全部为注入或本机数据，无默认远端调用。
- Windows新版暂存、Linux两架构cross-build通过；显式core/APP路径的Go全量与vet通过。
- 独立裸域名诊断一次：系统Fake-IP、公共DoH、验证TLS，以及新客户端根HEAD HTTP200；未请求含令牌路径/正文。全量测试时裸域名及私有文件变量均为空，对应测试跳过。
- 细节和未测范围见[本轮验收](<subscription-groups-verification.md>)。

## 入库与全面复查轮（2026-10-20 01:10）

- 首次将源码推送到私有 GitHub 仓库；提交前扫描确认无凭据、私钥、订阅链接与个人绝对路径，构建产物/数据目录不入库。
- 复查方式：全部 Go 源码与测试、全部前端页面与纯函数、契约/API/架构/决策文档、CI/Docker/compose；再用暂存二进制与临时数据目录做 60 余条真实 HTTP API 边界探测。
- 通过：Go全量 -p1 -count1、go vet、gofmt、严格TS（含noUnused）、32项工具测试、32项API浏览器、fixture UI、真实本机内核链路、构建后进程/CLI/manifest测试、Windows暂存+Linux两架构。
- 修复7项缺陷并补回归：长名称导致批量独立端口整批失败、预览与保存名称口径不一致、订阅元数据下载后才校验、监听页/节点页待同步误标“监听中”、活动记录按字节截断产生非法UTF-8、fixture脚本写死浏览器路径、文档含个人绝对路径。
- 未执行 race（无C工具链）、Linux/Docker实跑、容量压测；未访问真实订阅或真实节点。细节见[复查验收](<repository-review-verification.md>)。

## 内置安装轮（2026-10-20 11:09）

- 最终Go全量显式Mihomo/暂存程序、vet/gofmt、严格TS、36工具、fixture UI、34真实API浏览器通过。
- 下载器40函数/97子用例，92.0%覆盖，2个OS symlink子用例因宿主权限skip；本轮真实临时官网下载、摘要/版本检测、显式空配置启停通过。
- 四平台仅交叉构建暂存，未部署/提交/发布。Linux/Docker/Windowsarm64实跑、race、私有订阅/公网节点/容量压测未做。[完整验收与失败调查](<core-install-verification.md>)。

## WSL 实机运行轮（2026-11-01 17:02）

- 在 Fedora Linux 44 (WSL2) x86_64 上运行官方 Linux amd64 v0.2.4 程序（SHA256 c56a73ae7b6bb1985bf10750489b3e7642cadb1a52a544721e51b0ae306cecae）和固定 Mihomo 1.19.32（解压后 SHA256 7a0d59da2e678d56c899a3db996a2ad8963286c4f3634b0451435db248f13fa1），使用临时空数据和自建回环上游。
- 通过：健康/UI+CSP、固定内核起停、真实 HTTP/SOCKS5 转发（UNIPROXY-WSL-E2E-OK）、runtime stop、SIGTERM 后 WAL/SHM 清理、同数据目录重启自动应用、healthcheck、端口冲突与公网绑定保护、管理员 Token 401/200。
- 初次发现：WSL 冷启动首次 apply 可能触发 750 ms 私有认证 probe 超时，重试恢复；`kill -9` 主程序会遗留 Mihomo 内核（当时 processguard 仅 Windows），手动 SIGINT 可清理，重启后数据恢复正常。两项均已由 M22 在本地修复并复验，正式 v0.2.4 资产不含该修复。
- 边界：仅 WSL2/Fedora/amd64；未覆盖 arm64 实机、裸机、Docker、systemd、UDP、并发/容量/吞吐、真实远端节点。详见[WSL 实机验收](<wsl-runtime-verification.md>)。

## Linux 进程保护与探针重试修复轮（2026-11-01 17:43）

- [controller_verify_test.go](<../internal/core/controller_verify_test.go>) 覆盖 `/proxies` 先返回空映射后暴露原生出站的 `verifyProxies` 重试、1,200 ms 延迟绑定的 SOCKS listener（旧 750 ms 单次检查必失败）及父 deadline 到期；[processguard_linux_test.go](<../internal/processguard/processguard_linux_test.go>) 覆盖 `Pdeathsig = SIGTERM` 与 helper 父进程 `SIGKILL` 后子进程消失。
- Windows `go test -mod=readonly ./internal/core -count=1` 通过（12.506s），全量中 processguard 1.835s；`go test ./internal/core -run TestVerifyListener -count=1 -v` 通过（1.22s/0.05s）；WSL Fedora 定向测试通过。
- Windows 全量 `go test -mod=readonly -p 1 ./... -count=1` 通过（app 11.310s、core 12.106s、coreinstall 1.261s、integration 6.994s）；WSL `go test -mod=readonly ./internal/core ./internal/processguard -count=1` 通过（core 6.123s、processguard 0.010s）；`go vet ./...`、`gofmt -l internal cmd tests`、`git diff --check` 通过。
- 最终本地 Linux 候选 `.local/linux-fix/uniproxy-linux-amd64`（13,656,224 字节，SHA256 `8ff1916249b5cd6486cb3921b11aa253f1490b4110c4630b20f19460b057f2dc`）在 WSL2/Fedora 44 实跑：首次 apply HTTP 200，回环 HTTP/SOCKS 通过，`kill -9` 后内核退出、端口释放，重启恢复，最终正常停止。该候选仅本地验证；同一修复已提交 `ede12bb` 并随 v0.2.5 发布（不含部署）。

具体测试源、命令和限制可复查，不以虚拟节点/虚构延迟填充生产界面。

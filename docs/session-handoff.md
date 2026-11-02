# UniProxy 会话接续快照

**最后核对：2026-11-02 21:40（Asia/Shanghai）。v0.2.6 为当前正式版（未部署）；M25 代理链已完成独立审查修复、发布前全量验证与候选程序验收，版本定为 v0.2.7，正在进行提交与 Release 发布流程（不含部署）。本机当前用户自行启动的是本地预览 0.2.6-chain-preview.1：主程序 PID24580（`bin/updates/chain-preview/uniproxy-windows-amd64.exe`，20:11:00 启动）+ 内核 PID14272（`C:\ProgramData\UniProxy\core\mihomo.exe`，20:11:01，父进程 24580），127.0.0.1:9090 与 20006 链监听归该实例；本轮为 9router 排障只改运行中的链配置，未停止、未替换、未读取或修改用户数据库。操作前仍须重新核对当前进程。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.6](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.6>)，四平台程序与 SHA256SUMS；[中文说明](<releases/v0.2.6.md>)已同步到 Release body（2315 字符，GitHub API 实测一致）。
- 当前工作树基于 `e66d4e8` 的 v0.2.6 文档提交，包含 M25 代理链实现及 v0.2.7 发布文档：独立「代理链」页面、双栏节点选择器、2–16 跳合成出站串接、chain 监听、整链检测与原子创建；Go 全量、42/42 UI 工具、48/48 隔离浏览器、真实两跳回环链路与 Windows 候选程序验收通过；正在按 v0.2.7 提交/发布，不部署。详见[代理链验收](<chain-verification.md>)与[v0.2.7 发布验收](<release-0.2.7-verification.md>)。
- 发布源码 [dba132cb379fe113b897b787fef3a94ab91b8bef](<https://github.com/FYHC1/uniproxy/commit/dba132cb379fe113b897b787fef3a94ab91b8bef>)（M24 现代 VLESS/VMess 协议兼容），annotated tag 对象 `24346228a70db7c5e4530c2744389bed87db441a`，解引用为该提交。
- main CI [41409672766](<https://github.com/FYHC1/uniproxy/actions/runs/41409672766>)、标签 CI [41409873967](<https://github.com/FYHC1/uniproxy/actions/runs/41409873967>)、Release [41409873947](<https://github.com/FYHC1/uniproxy/actions/runs/41409873947>) 全部成功；2026-11-02 16:00:55 +08 发布并设 Latest。
- v0.2.6 **只发布、未部署**：未停止或替换任何运行实例，未触碰 `C:\ProgramData\UniProxy` 数据目录、数据库或主密钥。当前用户运行的是本地预览 `0.2.6-chain-preview.1`；v0.2.7 同样只发布、不部署。
- 当前栈仍为 Go + React/TS + SQLite 加密 payload + 外置 Mihomo1.19.32；M24 新增 VLESS XHTTP（Xray extra/download/reuse）、HTTPUpgrade→WS、VMess KCP、ECH、`vcn`/`pcs`、VLESS Encryption 与 URI 别名；VMess XHTTP/`spx`/`pqv`/`fm`/`flow=xtls-rprx-vision-udp443` 等无固定内核等价字段明确拒绝；无 schema 迁移、无内核版本变化。
- 历史：v0.2.5 于 2026-11-02 02:27:40 发布（M22/M23，未部署）；v0.2.4 于 2026-11-01 12:18:53 发布并部署替换本机实例，停机备份在 `C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`；v0.2.3 及更早标签与资产不变。
- 2026-11-01 在 WSL2/Fedora 44 amd64 使用官方 Linux v0.2.4 与固定 Mihomo 1.19.32 完成实机运行验收，并用本地 0.2.5 候选完成 M22 修复复验（首次 apply、回环 HTTP/SOCKS、`kill -9` 清理、重启恢复、正常停止）；正式 v0.2.4 资产不含该修复。

## 当前运行服务

- 2026-11-02 20:52 实测：主程序 PID24580（`C:\CustomFiles\CodeProjects\Github\uniproxy\bin\updates\chain-preview\uniproxy-windows-amd64.exe`，20:11:00 启动，无参数，版本 `0.2.6-chain-preview.1`）+ 内核 PID14272（`C:\ProgramData\UniProxy\core\mihomo.exe`，20:11:01，父进程 24580）；`127.0.0.1:9090` 归主程序，20006 归内核，19090 未监听。该实例由用户自行启动，不是本轮启动或测试残留；本轮未停止、未替换、未读取或修改用户数据库。
- v0.2.6 未部署；发布只创建 GitHub Release 与附件。
- 部署前置条件（若后续获授权）：确认无实例与子内核在运行，或先让其正常退出并确认内核退出 → 需要备份时停机后复制 → 替换主程序并沿用原数据目录/启动参数 → health 与 /api/core/inspect 复核；禁止在运行中复制或覆盖 SQLite 数据库，禁止混用不同库的 WAL/SHM。

## 代理链（M25，计划随 v0.2.7 发布，不含部署）

- 需求 R30：一条入口按固定顺序经过 2–16 个节点，形成可绑定监听的整链；不是代理组，不复制节点，不增加 DIRECT 回退。
- 模型/存储：`Chain{ID,Name,NodeIDs,Enabled,CreatedAt,UpdatedAt}`、`State.Chains`、`Listener.Kind=chain`、`CheckResult.ChainID`；`store.Normalize` 为旧状态补空链，无 SQLite schema 迁移。
- API：`GET/POST /api/chains`、`PUT/DELETE /api/chains/{id}`、`POST /api/chains/with-listener`（链和入口原子创建，复用 `poolListenerInput` 的校验/默认值/自动端口）、`POST /api/chains/{id}/test`。
- 编译：每个启用链生成 `c-<chainID>-<index>` 有序副本，第二跳起 `dialer-proxy` 指向上一 tag，入口只绑定末跳；任一跳缺失/禁用/中转异常或跳数越界时整链不生成、入口 REJECT，绝不缩短或直连。删除被引用节点会停用整链与关联入口，保留 NodeIDs 原顺序。
- 检测：`Manager.CheckChain` 读取 live `chainTags` 并调用内核末跳真实 delay 接口，不写节点健康；`verifyProxies` 通过 `chainHops` 核对每一跳暴露；真实两跳回环 SOCKS + 本地 HTTP origin 已证明每跳收到连接。
- 前端：独立「代理链」页与双栏创建窗口（分组/状态筛选、名称/延迟排序、最多 20 个可见节点自动测一次、单节点重测、上移/下移/移除、入口/出口标记、<2 跳提示、清空确认、可折叠入口监听）；列表支持搜索、状态、复制停用副本、启停、编辑、删除与整链测试。
- 发布前验证（2026-11-02）：`go test -mod=readonly -p 1 ./... -count=1`（显式本机内核）全通过（app 13.855s、core 15.748s、integration 9.269s）；`go vet`、gofmt、diff check 通过；两个 importer fuzz 各 10s 通过（500,855 / 115,976 execs）；严格 tsc、生产 Web 构建通过；`pnpm test:ui` 42/42；`pnpm test:e2e` 48/48（1.2m，chain-workflow 5/5）。
- 独立审查修复：链监听 UDP 校验只在链路和监听均启用时生效，节点缺失与不支持 UDP 分明；整链检测失败返回 HTTP 200 CheckResult；重复 hop/nativeProxy 错误 fail-closed；`chainHops` 核对每一跳；编辑/启停/重测清理旧结果、自动测速等待 runtimeReady、停用监听可保存失效链。新增删除节点 UDP 事务、失败检测、重复 hop 与停用监听浏览器回归。
- 边界：只使用本机回环链路测试；未验证真实公网远端、Docker、race、容量/并发或长期运行；计划随 v0.2.7 发布，不部署，运行中的用户预览实例未被触碰。
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

## 正式下载副本（v0.2.6）

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.6/uniproxy-windows-amd64.exe>) | 13,244,416 | `3757fd9218d20eceaf1078930fe1dbc9931f9619d237603292f822188205190c` |
| [Windows arm64](<../bin/releases/v0.2.6/uniproxy-windows-arm64.exe>) | 12,326,400 | `f0a72a685ffab56739585f897aa68f0543f5e775ea6df59909cc06eb62ebc26f` |
| [Linux amd64](<../bin/releases/v0.2.6/uniproxy-linux-amd64>) | 12,890,296 | `0dd697146b6f8dc1c83e36c850c736417b29e523b22c25766804771bb0f4f2fb` |
| [Linux arm64](<../bin/releases/v0.2.6/uniproxy-linux-arm64>) | 12,189,880 | `f2fdca77f67c6b833b472b398e4e0c9cd574227c9e3b452cdb3abc972a805d56` |
| [SHA256SUMS](<../bin/releases/v0.2.6/SHA256SUMS.txt>) | 360 | `a2f2cb8328f41b3afb096d4ecb3256d0d05119e1a861b6b034fd6dcb7f93b43a` |

副本位于 `bin/releases/v0.2.6/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致；正式 Windows 程序版本为 `UniProxy 0.2.6 · commit dba132cb379f · built 2026-11-02T08:00:13Z`。

## 结束状态与下一步

- v0.2.6 为当前正式版；v0.2.7 发布流程进行中。两者均**未部署**，本轮未停止或替换任何运行实例，未触碰用户数据目录、数据库或主密钥。
- 当前工作树基于 HEAD=origin/main=`e66d4e82c4e7f6d97ee95956c53324502ed2cd13`，M25 与 v0.2.7 文档改动处于发布提交准备中；M25 不部署。
- 当前用户自行启动的本地预览 0.2.6-chain-preview.1（PID24580 + 内核 PID14272）仍在运行，`127.0.0.1:9090` 归主程序、20006 链监听归内核，19090 未监听；这是用户既有实例，不是本轮启动或测试残留。9router 主进程 PID30460/子进程 4448 监听 20128，`JP-US1` 池已 active。
- v0.2.7 发布流程：提交/推送 main → 等两平台 CI 绿 → 打 annotated tag → 等 tag CI 与 release.yml → 核验四平台资产；发布不等于部署。若后续获部署授权：先确认没有任何 UniProxy 实例运行，或先让其正常退出并确认内核退出；再替换可执行文件并沿用原数据目录与启动参数；禁止在运行中复制或覆盖数据库，禁止混用不同库的 WAL/SHM。
- 用户侧真实节点连通/日常使用需另行观察；发布与程序验收不等于节点可用或吞吐达标。不要因会话恢复自动启动/停止服务。
- Linux amd64 已在 WSL2/Fedora 44 用 0.2.5 候选复验（M22 修复）；v0.2.6 的 Linux 资产仅构建与摘要核对，未实机运行；Linux arm64 仅构建与摘要核对。Docker、race、容量/吞吐与真实远端连通未在本轮验证。
- 常规测试清空 UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD 与 UNIPROXY_TEST_APP_PATH/UNIPROXY_TEST_CORE_PATH。既有 Docker 忽略未排除 .local，不构建/上传本机 Docker 上下文。
- 前端产物：M25 本地验证使用 `pnpm run build:web` 生成生产包（`assets/index-D4D2OA-E.js` 556.99kB / gzip 162.15kB、`assets/index-Dj_1XGal.css` 83.99kB / gzip 16.98kB，仅非失败 chunk 提示）；dist 被 .gitignore 忽略，正式资产由 CI 重新构建。

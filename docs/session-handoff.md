# UniProxy 会话接续快照

**最后核对：2026-11-01 21:40（Asia/Shanghai）。v0.2.4 已正式发布并部署；当前运行服务为主程序 PID22076（`bin/releases/v0.2.4/uniproxy-windows-amd64.exe`，18:55:04 启动）+ 内核 PID5536（`C:\ProgramData\UniProxy\core\mihomo.exe`，18:55:05），9090 归 22076，19090 无监听。此前记录的主程序 PID16568/内核 PID28844 已退出，属用户自行重启，本轮未改动该实例。另于 17:02 在 WSL2/Fedora 44 amd64 完成官方 Linux 程序实机验收，随后完成 Linux 修复（M22）与全局出站网卡选择（M23）的本地实现与验证；两者均未提交/发布/部署。操作前仍须重新核对当前进程。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.4](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.4>)，四平台程序与 SHA256SUMS；[中文说明](<releases/v0.2.4.md>)已同步。
- 发布源码 [72da273b5b8119de9a360b979ec33956ad0e4d23](<https://github.com/FYHC1/uniproxy/commit/72da273b5b8119de9a360b979ec33956ad0e4d23>)，annotated tag 对象 `05037b0804d4efab7aac0ba87e5ec778ea54caff`，解引用为该提交。
- main CI [41272387153](<https://github.com/FYHC1/uniproxy/actions/runs/41272387153>)、标签 CI [41272518085](<https://github.com/FYHC1/uniproxy/actions/runs/41272518085>)、Release [41272518132](<https://github.com/FYHC1/uniproxy/actions/runs/41272518132>) 全部成功；2026-11-01 12:18:53 发布并设 Latest。
- 已按用户授权部署替换：旧 v0.2.3 实例 PID10920/内核 28120 已优雅退出；原数据目录 `C:\ProgramData\UniProxy` 保留。停机后备份到 `C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`，5 文件 62,960,192 字节，源/备份逐文件 SHA256 一致。
- 当前栈仍为 Go + React/TS + SQLite 加密 payload + 外置 Mihomo1.19.32；M19/v0.2.4 仅前端交互、样式、测试、版本/文档和部署，无 schema/API/内核/安全策略变化。
- 2026-11-01 另在 WSL2/Fedora 44 amd64 使用官方 Linux v0.2.4 与固定 Mihomo 1.19.32 完成实机运行验收，覆盖回环 HTTP/SOCKS 真实转发、持久化重启、认证和异常终止观察；Windows 服务未受影响。
- 2026-11-01 17:43 已为 WSL 发现的两项 Linux 问题完成本地修复与复验：私有认证探针改为3秒有界重试，Linux 子进程增加 best-effort `Pdeathsig=SIGTERM`；仅本地源码/候选，未提交、未发布、未部署，正式 v0.2.4 资产不含该修复。

## 当前运行服务

- 主程序 PID22076：`C:\CustomFiles\CodeProjects\Github\uniproxy\bin\releases\v0.2.4\uniproxy-windows-amd64.exe`，2026-11-01 18:55:04 启动，无参数、使用默认数据目录（用户自行重启，非本轮部署）。
- 内核 PID5536：`C:\ProgramData\UniProxy\core\mihomo.exe`，1.19.32；父进程 22076。
- 127.0.0.1:9090 归 22076；19090 无监听（该端口仅用于隔离浏览器测试）。
- 21:40 复核进程与端口如上；部署时的 PID16568/28844 已退出。本轮 M22/M23 的本地改动**不在**该运行程序内，需重新构建并另行获得授权后才能部署。
- 未发现桌面/开始菜单快捷方式或 Run 注册表项指向旧 v0.2.3 程序。旧实例 PID10920/28120 已不存在。

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

## Linux 修复（M22，本地完成，未发布/部署）

- 探针：私有认证 HTTP/SOCKS/UDP 检查保留，外层改为 3 秒有界重试（每次 1 秒、间隔 100ms）；`/proxies` 原生出站暴露检查同样有界重试。到 deadline 仍失败时停止内核，不降低认证标准。
- 子进程：`processguard.PrepareCommand` 在 Linux 且 `SysProcAttr == nil` 时设置 `Pdeathsig=SIGTERM`；版本探测、配置校验、正式启动三处统一调用。Windows 继续用 Job Object，其他平台 no-op；这是 best-effort，非 Windows Job 等价。
- 验证：新增 `verifyProxies` 空映射后重试、1200ms 延迟绑定/父 deadline 与 Linux 父进程强杀回归；Windows/WSL 定向及全量 Go、vet、gofmt、diff 通过。最终 Linux 候选 `.local/linux-fix/uniproxy-linux-amd64`（13,656,224 字节，SHA256 `8ff1916249b5cd6486cb3921b11aa253f1490b4110c4630b20f19460b057f2dc`）在 WSL2/Fedora 44 实跑：首次 apply 直接 HTTP 200、回环 HTTP/SOCKS 通过、`kill -9` 后内核退出且端口释放、同目录重启恢复、最终正常停止。
- 边界：仅本地源码/候选和 WSL2 amd64；未提交、未推送、未发布、未部署。Linux arm64、裸机/其他发行版、Docker、systemd、容量与真实远端节点未测。详见[WSL 实机运行与修复验收](<wsl-runtime-verification.md>)。

## 全局出站网卡（M23，本地完成，未发布/部署）

- 需求 R28：管理员可选用物理以太网、WLAN 等本机网卡出站，解决 Clash Verge TUN 经 `Meta` 适配器接管默认路由导致 UniProxy 检测全超时的环境问题。
- 实现：`Settings.OutboundInterface`（空=自动）+ 编译时非空写 Mihomo 顶层 `interface-name`；`net.InterfaceByName` 要求存在/已启用/非回环并拒绝首尾空白；新增只读 `GET /api/interfaces`；设置页「03 全局出站网络」选择器（含刷新、不可用标注、非 TUN 说明）。节点级 `interface-name` 仍被 `safeNativeValue` 拒绝。
- 关键实测：Mihomo 不校验网卡存在性（`-t` 对不存在网卡仍报成功），非法网卡只在拨号时失败且日志伪装成 DNS 解析失败，且**不回退直连**——因此只能在管理层拒绝。
- 真实出站：用 `core.Build` 生成配置起临时 Mihomo，auto/以太网/WLAN/Meta 请求均 204；`Get-NetTCPConnection -OwningProcess` 显示以太网与 WLAN 变体的出站源地址各自不同，证明顶层绑定生效。
- 验证：`go test -mod=readonly -p 1 ./... -count=1`（显式本机内核）全通过；`go vet`、gofmt 干净；39/39 UI 工具；43/43 隔离浏览器（含新增 `tests/browser/outbound-interface.spec.ts`）；`src/web/tests/verify.mjs` fixture 全绿。
- 边界：只约束自有内核出站，不创建虚拟网卡、不改系统路由/DNS，不等价 TUN 分流；Linux/macOS 网卡命名与真实远端节点连通未在本轮验证；未提交、未推送、未发布、未部署。

## 正式下载副本

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.4/uniproxy-windows-amd64.exe>) | 13126144 | 4d57d1fe98165f9c720ac2adcbbf00599ccab359017250a4bbf1588b57ae5a9c |
| [Windows arm64](<../bin/releases/v0.2.4/uniproxy-windows-arm64.exe>) | 12220416 | 16a91ae40710ad85d5429323686a3c9c883dacc379957010b8bce80bc021748b |
| [Linux amd64](<../bin/releases/v0.2.4/uniproxy-linux-amd64>) | 12771512 | c56a73ae7b6bb1985bf10750489b3e7642cadb1a52a544721e51b0ae306cecae |
| [Linux arm64](<../bin/releases/v0.2.4/uniproxy-linux-arm64>) | 12124344 | d30f5023887ff713ba4de45eec03c4a72a1bff9dfe5f84573689dbb49a2568f7 |
| [SHA256SUMS](<../bin/releases/v0.2.4/SHA256SUMS.txt>) | 360 | 3f2e0c2aeaf2140aa9e558192943c21d66c6222fd9d5cfe56e0efd149dd40c01 |

## 结束状态与下一步

- v0.2.4 已发布并部署，旧实例已替换；不移动 v0.2.4 标签。此前发布/部署结果文档已作为独立文档提交（[skip ci]）并推送 main。
- 本次 WSL 实机验收、M22 Linux 修复与 M23 全局出站网卡的源码、测试和文档均为本地未提交变更；用户本轮只要求实现并测试网卡选择，未要求提交、推送、发布或部署。M22 与 M23 改动同处工作树，提交时必须按文件分开，不要混入同一提交。
- 用户侧真实节点连通/日常使用需另行观察；部署成功不等于节点可用或吞吐达标。不要因会话恢复自动重启/停止当前服务。
- Linux amd64 已在 WSL2/Fedora 44 实机运行并通过 M22 修复后候选复验（见[WSL 实机运行与修复验收](<wsl-runtime-verification.md>)）；Linux arm64 仍仅构建与摘要校验，未实机运行。Docker、race、容量/吞吐与真实远端连通未在本轮新增验证。
- 常规测试清空 UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD。既有 Docker 忽略未排除 .local，不构建/上传本机 Docker 上下文。
- 下一步（若用户要求）：分别按文件拆分提交 M22（Linux 稳定性）与 M23（全局出站网卡），再走既有发布流程（先 main CI 绿，再打 annotated tag）；部署需另行授权，且必须先正常退出运行实例再替换可执行文件。
- 前端产物：`pnpm run build:web` 已重建 `internal/webui/assets/dist`（`assets/index-D4iDTw75.js` 525508B + `assets/index-BM9nabfr.css` 71736B，2026-11-01 21:37:17；仅有非失败的 chunk 体积提示）；dist 已被 .gitignore 忽略，嵌入前端的 Go 程序未重建，运行中的 v0.2.4 不含 M23 界面。

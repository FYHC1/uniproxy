# UniProxy 会话接续快照

**最后核对：2026-11-04 19:38（Asia/Shanghai；发布与用户实例信息为 11:43 观察，本轮未重新核对）。v0.3.0 发布准备完成，开发分支待提交、合并、打标签与 CI；v0.2.9 已于 2026-11-03 16:22:39 +08 正式发布并设 Latest，源码提交 `6ff15a8c2a0fb44e3dbc14c4e024b1f7c6b4d54a`，annotated tag 对象 `4d05181f557445f5f6325bd57e3821d138c981a5`。**只发布、未部署**，未停止或替换任何运行实例，未触碰用户数据目录、数据库或主密钥。当前用户实例仍为 uniproxy PID18324（临时 go-build 路径，09:30:08 启动）与内核 mihomo PID24836（09:30:09 启动），监听 `127.0.0.1:9090` 与 20006；操作前仍须重新核对。**

## 2026-11-04 — 合并前代码复审与修复（开发分支）

- 用户要求：对 `feat/multi-engine-sidecars` 做代码 review，完成后合并到 `main` 并发布新 release；不部署、不触碰用户实例/数据库。
- 三路只读审查覆盖 core/store/app、sidecar/installer、importer/UI；最终复查未发现未闭环 P1。已修复 P1：`internal/core/compiler.go` 原生节点指向 Xray 中转时 `dialer-proxy` 使用节点 tag 而非 `x-<ID>` 代理 tag（Mihomo 引用不存在出站）。已修复 P2：`PATCH /api/nodes/{id}` 响应未清除 sidecar 端点凭据；sidecar 安装失败未复位 `awaitingCoreApply`；`core.Apply` 失败后 sidecar 未回滚；手动停止后失败应用可能重启旧 sidecar；sidecar 崩溃只写状态不重启且节点未标记不可用；disabled 节点重复端口导致保存/启动失败；外部 PATH Xray 被误报固定版本且阻止在线安装；HTTP 出站仍生成 SOCKS UDP；Trojan flow 被固定 Xray 26.10.31 拒绝但导入仍接受；`xray -test`/版本探测子进程缺少进程树保护；`App.Start` 准备失败后仍应用配置会覆盖错误。
- 修复落盘：`internal/app/app.go` 增加 `appliedNodes`/`rollbackSidecar`（core 未运行时停止 sidecar）与启动准备错误短路；`internal/app/http.go` 统一节点公开投影并在 runtime stop 清空 appliedNodes；`internal/app/sidecar_install.go` 失败复位；`internal/sidecar/process.go` 崩溃 bounded backoff 自动重启（最多 3 次）+ bootstrap 节点 `unhealthy`；`internal/sidecar/endpoint.go` 先保留 enabled 端点、再修复 disabled 冲突；`internal/sidecar/install.go` 区分托管/外部版本；`internal/sidecar/xray.go` 对 HTTP 关闭 UDP 并拒绝 Trojan flow；`internal/core/compiler.go` HTTP sidecar 出站 `udp=false`；`internal/importer/xray_uri.go` 拒绝 Trojan flow；`internal/sidecar/process.go`、`install.go` 使用 `processguard.PrepareCommand`；并补对应回归测试。
- 验证（2026-11-04 19:38）：`go vet ./...` 无输出；`go test ./... -count=1 -p 1` 全通过（app 11.747s、core 6.714s、sidecar 1.754s、sidecarinstall 0.198s、importer 0.709s、store 0.347s、cmd/uniproxy 0.128s）；`pnpm typecheck`、`pnpm test:ui` 44/44（全部浏览器回归 PASS）、`pnpm build:web`（Vite 7.3.7，4656 modules）、`pnpm test:e2e` 52/52（1.3m）通过。Go 格式化核验：修改的 19 个 Go 文件按 LF 规范化后 `gofmt -d` 无差异（`GOFMT_BAD=0`），工作树 `gofmt -l` 的 CRLF 显示仅为 `core.autocrlf=true`，不作为格式缺陷。
- 状态：27 个修改文件，+776/−56（复审修复 + v0.3.0 发布准备）；版本文件、README、CHANGELOG、发布说明与验收记录已同步；HEAD 仍为 `39a5955`，待提交、推送、合并、打标签、CI 后回填资产；未部署。

## 2026-11-04 — 多引擎 sidecar 提交（开发分支）

- 用户要求提交当前工作树，并询问发布 release 前是否需要合并主分支。
- 已提交 `00e3350` `feat: add multi-engine sidecar support`：71 files changed, 7993 insertions(+), 242 deletions(-)；提交前删除无代码引用的临时 `internal/sidecar/debug-generated.json`。
- 分支 `feat/multi-engine-sidecars` 相对 `main`（`eb7675c`）领先 1 个功能提交；未推送、未发布、未部署。文档记录随后以独立提交补齐。
- 发布流程核对：`.github/workflows/release.yml` 由 `v*.*.*` tag push 或 workflow_dispatch 触发；手动 dispatch 会在 `GITHUB_SHA` 创建 tag。正式发布前应先把 `feat/multi-engine-sidecars` 合并到 `main`，在 `main` 上验证后建 tag，不能从功能分支直接发。

## 2026-11-04 — 真实 GitHub 网络下载验证（开发分支，未提交）

- 用户要求：验证侧车 Xray 安装器的真实 GitHub 网络下载。
- 固定资产：官方 `Xray-windows-64.zip` 16,623,726 bytes，SHA256 `2e04a9995a6f2ad1b28c0bb25c708ecb2695e43a1c9c7889c4b6ac2b48cf7a6a` 与 `internal/sidecarinstall` 内置 windows-amd64 pin 一致；`gh release download` 得到的 ZIP 摘要一致，展开后 xray.exe 34,555,392、geoip.dat 3,312,784、geosite.dat 7,554,929、wintun.dll 427,552、LICENSE 16,725、LICENSE-Wintun 5,431 bytes；`xray.exe version` 输出 `Xray 26.10.31 (Xray, Penetrates Everything.) a4a3dff (go1.27.3 windows/amd64) A unified platform for anti-censorship.`。
- 生产安装路径：临时 `internal/sidecarinstall/live_github_temp_test.go` 的 `TestLiveGitHubInstall` 通过 `gh api` 获取真实 release metadata，仅注入 `api.github.com` 测试响应；资产字节由真实 GitHub CDN 下载，`install(...)` 完整走 metadata/downloading/verifying/extracting/installing，PASS 4.60s（下载 16,623,726 bytes、3.784s；发布目录含 xray.exe/geoip.dat/geosite.dat/wintun.dll/LICENSE/LICENSE-Wintun，版本检测包含 26.10.31）。
- 匿名 API 边界：本机共享出口的 GitHub 匿名 API 配额耗尽，直接 `Install` 返回 `GitHub 返回 HTTP 403（访问受限或请求过于频繁）；请稍后重试，并检查服务器能否直连 GitHub`；响应头 `X-RateLimit-Limit: 60`、`Remaining: 0`、`Used: 60`、`Reset: 1793786276`（2026-11-04 17:57:56 +08）。安装器仍保持 API metadata/digest 校验，不因验证接受 token 或放宽检查。
- 验证（2026-11-04 17:03）：删除临时测试后 `go vet ./...` 无输出、`go test ./... -count=1 -p 1` 全通过（app 11.143s、core 6.822s、sidecar 0.874s、sidecarinstall 0.204s、importer 0.830s、store 0.361s、e2e 0.029s、integration 0.030s）。
- 清理：临时测试与 `.local/final-go-*.log` 已删除；未修改生产代码、未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未发布、未部署。

## 2026-11-04 — 工作树复审与 kr-vps 真实远端订阅 E2E（开发分支，未提交）

- 用户要求：审阅当前工作树 diff，并用 ssh 工具中的 kr-vps 的 21443 端口部署协议测试和真实订阅验收。
- 复审范围：59 个 modified + 17 个 untracked（40 个 tracked 文件 1045 insertions/244 deletions 的核心 diff，加 `internal/sidecar`、`internal/sidecarinstall`、`internal/e2e`、Xray importer 与前端安装面板）；逐项检查 model/compiler/validate/app/relay/sidecar/sidecarinstall/importer/前端和测试。
- 修复 4 项真实缺陷：1）`internal/importer/xray_vmess.go` 把 VMess cipher `security=none` 误当 transport security 覆盖 `tls=tls`，拒绝合法 XHTTP 节点；2）`internal/sidecar/install.go` 在文件已发布后版本探测失败时未清理安装目录，导致 `CanInstall=false` 无法重试；3）`internal/store/store.go` 归一化外层 `Node.Engine` 与嵌入 `ParsedNode.Engine`，并在 `internal/app/relay.go` 重算 Xray 指纹前同步 engine，修复备份恢复后指纹漂移；4）`internal/app/relay.go` 的 `exportNativeNodes` 递归排除依赖 Xray sidecar 的原生节点，避免导出悬空 `dialer-proxy`。
- 回归补强：`TestXrayVMessXHTTPNoneCipherWithTLS`、`TestXrayManagerVersionProbeFailureIsRetryable`、`TestNormalizeSyncsParsedNodeEngineAfterJSONRoundTrip`、`TestBackupRestoreKeepsXrayFingerprint`，并扩展 `TestExportNativeNodesOmitsXraySidecarNodes` 覆盖 leaf/chain-leaf 依赖。
- 远端 fixture：kr-vps 为 Alpine Linux 3.24.2 x86_64 容器（210.109.205.7）；只有公网 21443 真正映射到容器 Xray，21444/21445 未转发。Xray 26.10.31 a4a3dff linux/amd64 使用官方 `Xray-linux-64.zip`（16,692,690 bytes，SHA256 与内置 linux-amd64 pin 一致）。订阅 `http://210.109.205.7:20080/links-xray.txt`，远端正文本目标 `http://210.109.205.7:20080/ok-remote.txt`（`remote-e2e-ok`）；远端 freedom 禁止回环目标，必须用公网地址。
- 新增 `internal/e2e/real_remote_test.go`：`TestRealRemoteSubscription` 通过 `UNIPROXY_E2E_REMOTE_SUB` 启用，`UNIPROXY_E2E_REMOTE_NODES` 支持单协议（默认 3），覆盖真实 HTTP 订阅导入、EngineXray 调度、sidecar 启动、mixed listener、远端目标读取、强制刷新和 app 重启后复验。
- 远端结果（2026-11-04 16:40–16:41）：VLESS Encryption PASS 10.23s、VMess XHTTP PASS 6.12s、Reality PASS 5.66s；每个协议首次导入/强制刷新/应用重启三次均返回 `remote-e2e-ok`。VLESS Encryption ticket 约 600 秒过期，每次测试前需重新生成；当前远端 21443 保留 Reality fixture。
- 验证（2026-11-04 16:43）：删除临时 debug 测试后 `go vet ./...` 无输出、`go test ./... -count=1 -p 1` 全通过（app 11.412s、core 6.753s、sidecar 0.938s、sidecarinstall 0.235s、importer 0.909s、store 0.364s）。前端于 16:49 完整重跑：`pnpm typecheck`、`pnpm test:ui`（44/44 单测 + 全部浏览器回归）、`pnpm build:web`（Vite 7.3.7，4656 modules；CSS 112.77kB、JS 566.93kB，仅 chunk 提示）、`pnpm test:e2e` 52/52（1.3m）全部通过。
- 边界：kr-vps 仅单端口单节点顺序协议，未验证三协议并发、多端口映射、UDP、公网带宽、容量/长期运行、其他发行版或裸机；未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未发布、未部署。
- 决策：新增 D38，记录真实远端 E2E 采用单端口顺序协议、不把测试用端口复用放入产品。
- 下一步：如需保留远端测试环境，可直接访问 `http://210.109.205.7:20080/links-xray.txt` 当前的 Reality 订阅；后续改协议需重新部署 21443 并重建链接。

## 2026-11-04 — Xray 在线安装器与设置页接入（开发分支，未提交）

- 用户要求：继续 handoff 后续批次，完成固定版本 Xray 在线安装器、多文件 Xray ZIP 安全解压、sidecar 自动发现、app/API 安装状态与设置页安装/重试；不提交、不发布、不部署。
- 新增 `internal/sidecarinstall`：固定 Xray 26.10.31；仅支持 windows/linux × amd64/arm64，四平台官方资产名与内置 SHA256 pin；只允许 GitHub API/官方 release 资产域名，GitHub digest 必须与内置 pin 一致；下载/解压/单项/总量均有上限；ZIP 仅接受平坦白名单文件（Windows 必含 xray.exe/geoip.dat/geosite.dat/wintun.dll，Linux 必含 xray/geoip.dat/geosite.dat，可选 LICENSE/LICENSE-Wintun），拒绝路径穿越、符号链接、加密项和大小写重复；先校验后解压，目标已存在直接拒绝，逐文件硬链接无覆盖发布，失败清理已发布文件和空目录；错误文案不泄露敏感信息。
- `internal/sidecar` 接入安装管理：可执行文件发现顺序为显式 `BinaryPath` → 安装目录 `WorkDir/xray/26.10.31/xray(.exe)` → PATH；`InstallationStatus` 纯文件发现 idle/installing/succeeded/failed，`PrepareInstall` 用 `opMu.TryLock` 预留，`RunInstall` 回写进度并在完成后执行 `xray.exe version`，实际版本必须等于固定版本；安装中 `Start/Apply` 返回“Xray sidecar 正在安装，请等待完成后再应用配置”，并发安装返回“Xray sidecar 正在执行其他操作，请稍后重试”。
- app/API：`GET /api/sidecars` 增加 `installation` 状态；新增 `POST /api/sidecars/install`（仅接受空对象，202）；安装期间 runtime/apply/stop 和内核安装均返回 409；安装完成后配置只保存、需用户手动重新应用；失败把中文原因写入 `applyError`，不伪造成功或运行状态。
- 设置页：新增 `SidecarInstall` 面板，显示“Xray 可执行文件”、固定版本/平台/目标路径、在线安装/重试、下载与解压进度、成功/失败状态；安装中每 5 秒刷新 `GET /api/sidecars`。
- 验证（2026-11-04 14:49）：`pnpm typecheck`、`pnpm test:ui`（44/44 单测 + 全部浏览器回归，含安装面板断言）、`pnpm build:web`、`pnpm test:e2e` 52/52（1.4m）通过；删除临时安装测试后 `go vet ./...` 无输出、`go test ./... -count=1 -p 1` 全通过（app 12.020s、core 6.778s、sidecar 0.861s、sidecarinstall 0.235s）。真实安装级验证用本地官方 `Xray-windows-64.zip`（16623726 bytes，SHA256 与内置 windows-amd64 pin 一致）模拟 GitHub metadata/asset，`TestLiveInstallLocalOfficialArchive` PASS 2.10s；发布目录含 xray.exe/geoip.dat/geosite.dat/wintun.dll/LICENSE/LICENSE-Wintun，版本输出 `Xray 26.10.31 (Xray, Penetrates Everything.) a4a3dff (go1.27.3 windows/amd64)`；临时测试文件已删除。实际 GitHub 网络下载未在本轮验证（后于 2026-11-04 17:04 小节完成真实 GitHub 资产下载与生产安装路径验证），Linux 资产只做单元测试与 pin/解压逻辑验证，未做真实 Linux 安装。
- 截图：`src/web/tests/artifacts/settings-sidecar-desktop.png`（1440x2673）已人工查看，04 多引擎 Sidecar 面板无内容重叠；底部固定条覆盖属截图视口正常表现。
- 边界：未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未推送、未发布、未部署。安装器代码与 Windows 本地官方 ZIP 安装级验证完成，真实 GitHub 下载链路已于 17:04 小节验证，Linux 实机安装仍需后续验证。
- 下一步：审阅安装器 diff；如需真实网络安装验收，在隔离临时数据目录执行，并保留版本/摘要/失败现场；随后按 handoff 汇报已完成/待做。

## 2026-11-04 — Importer Xray-only 与真实订阅验收（开发分支，未提交）

- 用户要求：补 `internal/importer` 的 Xray-only 识别并保留真实配置，再用 leidahub VLESS 订阅做真实 Xray sidecar 远端验收；不提交、不发布、不部署，临时资源最后清理。
- importer：`model.ParsedNode` 增加 `Engine`，`addParsed` 复制到 `Node.Engine`，`Fingerprint` 加入 `engine,omitempty`（旧 Mihomo 指纹不变）。新增 `internal/importer/xray_uri.go`（fm/pqv/cs/spx、VLESS encryption、flow=xtls-rprx-vision-udp443、XHTTP、Reality 扩展、TLS cipher suites）和 `internal/importer/xray_vmess.go`（VMess JSON XHTTP）；`uri.go`/`vmess.go` 接入；合法 Xray-only 字段保留，坏 fm/cs 严格失败或预览降级，VLESS 明文公网 IP 回退 Mihomo 预览。
- 测试：新增 `internal/importer/xray_uri_test.go`，覆盖 finalmask、Reality 扩展、cipher suites、VMess XHTTP authority/JSON、安全字段映射和坏样本降级；`go test ./internal/importer` 通过。
- 真实订阅：`C:\Users\h2147\AppData\Local\Temp\leida-vless.txt` 解码 1394 条 VLESS；`ParsePreview` 得到 1383 个可显示节点（strict 914 = Mihomo 888 + Xray 26；degraded 469 = Mihomo 418 + Xray 51；错误 11）。77 个 Xray 节点逐节点 `xray -test` 全部通过，合并配置 `Configuration OK`。
- 真实远端：临时探针启动 77 个真实 Xray 节点，用原始 SOCKS5 CONNECT + `GET http://api.ipify.org/` 验证，当前 38/77 可达（并发 8、单节点超时 10s）；失败多为 Cloudflare ws/tls `unexpected EOF`。结果只是单次快照。
- 验证（2026-11-04 14:18）：删除两个临时探针后 `go test ./... -count=1 -p 1` 全通过（app 12.117s、core 9.812s、sidecar 2.081s、importer 1.335s）；`go vet ./...` 无输出。未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未推送、未发布、未部署。
- 下一步：安装器/多文件 Xray ZIP 安全解压已在后续小节完成；继续按 handoff 汇报已完成/待做。

## 2026-11-04 — 多引擎 sidecar 真实流程验证（第 1–3 层，开发分支，未提交）

- 用户要求：执行真实流程第 1–3 步，即 Windows 11 amd64 本机真实进程闭环、三协议本地端到端、Linux amd64 用 WSL 实跑；不包含真实远端 VPS/测试订阅、安装器与部署。
- 新增 `internal/e2e/real_process_test.go`：用真实 `uniproxy` 子进程 + 真实 Mihomo/Xray 二进制，覆盖三个 Xray 节点（VLESS Encryption、VMess XHTTP、Reality）经 Mihomo mixed listener → sidecar SOCKS5 → 本地 Xray 服务端的完整链路；校验 `GET /api/sidecars`/`GET /api/runtime`、`POST /api/runtime/stop`、同 dataDir 重启恢复及强杀后 4 个占用端口全部释放。
- Windows 11 实跑（Xray 26.10.31、Mihomo v1.19.32）：`TestRealAppProcessThreeProtocols` PASS 2.74s；三协议与重启均返回 `real-e2e-ok`，日志确认“abrupt application exit released 4 owned ports”。Phase 0 POC 与 `BuildXrayConfigAcceptedByXray` 同时 PASS（32300/32301/32302）。
- WSL2 Fedora 44（Linux amd64，Go 1.26.10）：同一真实进程测试 PASS 2.12s，四项日志一致；`TestPhase0XrayPOC` PASS 0.38s；`go test ./... -count=1 -p 1` 全通过（app 2.390s、core 6.091s、sidecar 0.583s）。
- 全量 Windows Go 测试：`go test ./... -count=1 -p 1` 全通过（app 9.014s、core 6.633s、sidecar 0.820s）。
- 下一步：第 4 层真实远端 VPS/测试订阅需要用户提供受控节点与订阅；第 5 层安装器已完成，可在独立测试数据目录复跑真实安装与端到端链路。
- 边界：真实远端 VPS、真实订阅、安装器、多文件 Xray ZIP 安全解压与跨平台非 WSL 裸机未验证；未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未推送、未发布、未部署。详见[真实流程验证](<sidecar-real-process-verification.md>)。

## 2026-11-04 — 双 sidecar 第四批产品化（端口范围、预检与脱敏，开发分支，未提交）

- 用户要求：继续 handoff 下一批，先完成 Phase 2 可独立验收的端口范围持久化/校验/bind 预检、200 节点上限中文 UX 和 sidecar 凭据/日志脱敏；安装器留后续；不提交、不发布、不部署。
- 设置与存储：`Settings.SidecarPortRangeStart/SidecarPortRangeEnd` 默认 30000/30999；旧 state 缺失或为 0 时由 `store.Normalize` 补默认值，不影响已有显式范围。
- sidecar：新增 `XrayManager.ConfigurePortRange`（校验后热替换分配器）、`PortRange` 和 `Preflight`（跳过当前 Xray 进程已持有的回环端口，逐节点检查 TCP/UDP 绑定，重复/占用返回明确中文）；`Start/Apply` 超 200 节点统一返回中文上限错误；`Logs()` 与 `xray -test` 输出同时脱敏用户名和密码。
- app：`Sidecar` 接口增加 `ConfigurePortRange`/`Preflight`；新增 `prepareSidecar` 统一执行范围校验、端点分配和预检，`mutate`、`Start`、`applyLocked` 共用；`GET /api/sidecars` 增加 `portRangeStart/portRangeEnd/maxNodes`；`PATCH /api/settings` 白名单接受两个新字段。
- 校验：sidecar 范围必须在 1024–65535 且起点不大于终点；启用时不得与独立监听范围重叠、不得包含管理端口或已有监听端口；启用 Xray 节点超过 200 个直接拒绝。
- UI：设置页 04 面板新增 Sidecar 起止端口输入、独立范围提示和 `启用节点 / 200` 展示，达到上限时显示拆分提示；保存前做范围与重叠校验。
- 验证（2026-11-04 11:11）：变更 Go 文件 `gofmt` 检查无输出，`go vet ./...`、`go test ./... -count=1 -p 1` 全通过；`pnpm typecheck`、`pnpm test:ui`（44/44 单测 + 全部浏览器回归）、`pnpm build:web`、`pnpm test:e2e` 52/52 通过；设置页 1440/375 截图无横向溢出。
- 边界：本批未做安装器、多文件 Xray ZIP 安全解压、跨平台实跑、真实远端节点与真实订阅；跨平台实跑已由上方真实流程验证补充；未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未推送、未发布、未部署。

## 2026-11-04 — 双 sidecar 第三批 UI 接入（开发分支，未提交）

- 用户要求：继续 handoff 下一批，完成 UI 接入并汇报已完成/待做；不提交、不发布、不部署。
- 设置页：运行与设置 → 检测与端口新增“04 多引擎 Sidecar”，读取 `GET /api/sidecars`，显示实际进程状态、版本、启用节点数和功能开关；切换 `enableSidecar` 只在保存后生效；节点数大于 0 且关闭开关、或已有节点但进程未运行时显示明确警告；进程错误原样展示，不伪造运行状态。
- 节点页：新增“引擎”列，显示 `Mihomo`/`Xray`/`sing-box` 徽章；Xray 行显示实际 sidecar 状态（运行中/未运行/异常），编辑弹窗补充引擎、Sidecar 状态、版本与错误。
- 导入预览：协议列显示引擎徽章；导入器对合法 Xray-only 字段生成 `EngineXray` 节点，Mihomo 节点保持默认 `Mihomo`，降级预览节点只展示不导入。
- 测试：`src/web/tests/utils.test.mjs` 增加 engine/sidecar 状态辅助函数用例（44/44）；`verify.mjs` 增加 Xray 节点徽章/编辑弹窗、设置开关保存、导入预览徽章及 375/768/1024 设置页无横向溢出断言；`core-install-regressions.mjs` 补充 `enableSidecar:false` 与 `/api/sidecars` fixture。
- 验证（2026-11-04 10:30）：`pnpm typecheck`、`pnpm test:ui`（44/44 单测 + 全部浏览器回归）、`pnpm build:web`、`pnpm test:e2e` 52/52、`go vet ./...`、`go test ./... -count=1 -p 1` 全通过。
- 边界：本批只做 UI 展示和开关，不新增后端 API、不做安装器/端口范围预检/跨平台实跑；真实 Xray 远端节点、真实订阅和用户实例仍未验证；未提交、未推送、未发布、未部署。

## 2026-11-04 — 双 sidecar 第一批实现（开发分支，未提交）

- 用户要求：按[交接](<multi-engine-sidecar-handoff.md>)第一批顺序实现模型字段、`internal/sidecar`、编译器本地 SOCKS 出站、链位置校验和 3 个离线 POC；不提交、不发布、不部署。
- 模型：`internal/model/types.go` 增加 `Node.Engine`（空=mihomo）、`Node.SidecarEndpoint`、`SidecarStatus`；`internal/model/engine.go` 集中链位置校验；`internal/store/store.go` 归一空引擎。
- `internal/sidecar`：`ports.go` 做 127.0.0.1 端口分配（1024–65535，建议 30000–30999）；`xray.go` 生成每节点一个 SOCKS inbound、同 tag outbound 的多节点配置并校验真实远端只来自 Node.Server/Port；`process.go` 单 Xray 进程启动/停止、`xray run -test -c`、SOCKS 健康检查和随机本地凭据，单进程最多 200 节点。
- 编译器：`internal/core/compiler.go` 把 sidecar 节点编译为 `x-<NodeID>` 的 Mihomo `socks5` 出站（server 127.0.0.1、端口取 SidecarEndpoint），真实远端地址不进入 Mihomo YAML；`nodeTags` 保持检查/探针按实际 tag 查找；sidecar 节点禁止 DialerID，pool/listener 使用 `proxyTag`。
- 链位置：`internal/app/validate.go` 与编译器双重校验，sidecar 只允许单独出站或链首，链中间/链尾返回“sidecar 节点只允许位于链首”。
- Phase 0 POC：`internal/sidecar/poc_test.go` 的 `TestPhase0XrayPOC` 用 Xray 26.10.31 覆盖 VLESS Encryption（ML-KEM-768）、VMess XHTTP、Reality（mldsa65Seed/Verify + 大证书目标）三套配置，经本地 SOCKS 实际转发 `phase0-ok`，三节点全通过。
- 验证（2026-11-04 02:25）：变更文件 `gofmt` 干净、`go vet ./...`、`go test ./... -count=1 -p 1` 全通过（含 `internal/sidecar`）；`pnpm typecheck`、`pnpm test:ui` 42/42、`pnpm build:web`、`pnpm test:e2e` 52/52 通过；POC 单测 PASS（vless-enc 32300、vmess-xhttp 32301、reality 32302）。
- 边界：第一批 1–5 与第二批最小 app/API 集成已完成；UI 开关、节点引擎徽章和状态页展示尚未接入（功能设置默认关闭）；未接真实订阅、未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未推送、未发布、未部署。

## 2026-11-04 — 双 sidecar 第二批最小 app/API 集成（开发分支，未提交）

- 用户要求：完成[交接](<multi-engine-sidecar-handoff.md>)第 6 项最小 app/API 集成，功能开关默认关闭，UI 暂不展示；不提交、不发布、不部署。
- 设置与类型：`internal/model/types.go` 新增 `Settings.EnableSidecar`（默认 false，保持现有 Mihomo-only 路径）；`src/shared/types.ts` 同步 `enableSidecar` 及节点 `engine`/`sidecarEndpoint`/`sidecarStatus` 公开投影。
- sidecar 生命周期：`internal/sidecar/endpoint.go` 的 `AssignEndpoints` 保留已有端点、为缺失端点分配 127.0.0.1 随机端口和随机凭据；`internal/sidecar/process.go` 的 `Apply` 按配置哈希复用单进程，配置变化时先停旧进程再启动新进程，失败时尝试恢复上一版；`NewXrayManager` 会创建 0700 工作目录，生产 `dataDir/sidecar` 不存在时也可直接启动；启动顺序为 sidecar 就绪后再应用 Mihomo，停止顺序为先停 Mihomo 再停 sidecar。
- app 集成：`internal/app/app.go` 增加可选 `Sidecar` 注入（`Options.Sidecar`，nil 时完全保持原路径）；`mutate` 在保存前分配并持久化端点；`Start`/自动应用先校验功能开关和启用节点，再启动 sidecar，最后 `core.Apply`；sidecar 启动失败不会进入 Mihomo 应用，错误明确写入 `applyError`，不直连回退。
- API 与脱敏：新增 `GET /api/sidecars -> {enabled,status,nodeCount}`；`PATCH /api/settings` 白名单增加 `enableSidecar`；`bootstrap` 清除 `SidecarStatus` 和端点用户名/密码，只保留 host/port，节点状态按当前管理器的实时状态注入；备份恢复对 EngineXray 节点跳过 Mihomo importer 解析但仍重算指纹；`GET /api/export/nodes` 只导出 Mihomo 原生节点，Xray sidecar 节点通过完整备份迁移。
- 校验：EngineXray 仅允许 vless/vmess/trojan/ss/socks5/http，禁止 DialerID；启用 Xray 节点必须有 127.0.0.1 端点，端口全局唯一；功能开关关闭时保存或应用启用 Xray 节点会明确报错；链位置仍只允许单独出站或链首。
- 验证（2026-11-04 08:45）：`gofmt`、`go build ./...`、`go vet ./...`、`go test ./... -count=1 -p 1` 全通过；`pnpm typecheck`、`pnpm test:ui` 42/42、`pnpm build:web`、`pnpm test:e2e` 52/52 通过；真实 Xray 26.10.31 的 `TestPhase0XrayPOC` 三样本和 `BuildXrayConfigAcceptedByXray` 均 PASS（vless-enc 32300、vmess-xhttp 32301、reality 32302）。
- 边界：仅最小 app/API 集成；UI 开关/徽章/状态页、真实订阅和真实远端节点未验证；未启动/停止用户实例、未触碰用户数据库/主密钥；未提交、未推送、未发布、未部署。

## 2026-11-03 16:22 — v0.2.9 协议适配三档（已发布，不含部署）

- 用户要求按三档实现：严格 native 映射、宽松解析预览/degraded、真实可用性验证；实现见 [协议适配三档验证](<protocol-adaptation-tiers.md>)。
- 严格 `Parse`：真实订阅 1064 条中 697 成功（65.5%），`TotalErrors` 367；宽松 `ParsePreview`：697 严格 + 365 降级 = 1062 可展示（99.8%），2 条未恢复。降级节点只展示，永不进入 `addParsed`、SQLite、编译器或运行配置。
- 发布：提交 `6ff15a8c2a0fb44e3dbc14c4e024b1f7c6b4d54a`（29 files，841 insertions / 85 deletions）；tag `v0.2.9` 对象 `4d05181f557445f5f6325bd57e3821d138c981a5`；main CI `41602628999`、tag CI `41602671353`、Release `41602671127` 全部成功。
- Release：[v0.2.9](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.9>)，2026-11-03T08:22:39Z 发布；四平台资产已下载到 `bin/releases/v0.2.9` 并逐项与 `SHA256SUMS.txt` 一致。正式 Windows 程序 `UniProxy 0.2.9 · commit 6ff15a8c2a0f · built 2026-11-03T08:21:55Z`，TestBuilt 全部 PASS（3.275s），真实控制台两轮 WM_CLOSE exit0 + standalone error exit1/report saved。
- 验证：`gofmt`、`go vet ./...`、串行 `go test ./... -count=1`、`pnpm typecheck`、`pnpm test:ui` 42/42、`pnpm build:web`、`pnpm test:e2e` 52/52 通过；真实订阅仅本机临时解析，样本与 live 测试已删除。
- 发布不部署；用户运行实例未触碰。

## v0.2.8 历史摘要

- 正式发布：[UniProxy v0.2.8](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.8>)，非 draft、非 prerelease、Latest；发布时间 2026-11-03 10:28:28 +08（UTC 2026-11-03T02:28:28Z）。
- 源码提交 `76f3c59d0adf42bd54a9abb6e167c9328f876bb9`（31 文件，1172 新增 / 120 删除）；annotated tag `v0.2.8`，tag 对象 `c9567f228657d63601dbf5d89dc8e4b7e08faebf`。
- main CI [41565978602](<https://github.com/FYHC1/uniproxy/actions/runs/41565978602>)（Ubuntu 42s、Windows 1m38s）、标签 CI [41566227166](<https://github.com/FYHC1/uniproxy/actions/runs/41566227166>)（Ubuntu 38s、Windows 1m39s）、Release [41566227152](<https://github.com/FYHC1/uniproxy/actions/runs/41566227152>) 四平台 build 与 release job 全部成功。
- 正式中文说明与本地 [docs/releases/v0.2.8.md](<releases/v0.2.8.md>) 逐字一致（1911 字符）；正式资产四平台程序与 SHA256SUMS 已下载核验，见下文。
- 正式 Windows 程序 `UniProxy 0.2.8 · commit 76f3c59d0adf · built 2026-11-03T02:27:52Z`；全部 TestBuilt 通过（4.253s），真实控制台 WM_CLOSE 两轮退出 0 并重启、独立失败窗口保留后退出 1 均通过。
- 本版内容：M26 新建代理链弹窗布局；M27 全局暖纸亮色视觉刷新；M28 新建/编辑代理池布局与节点批量栏；M29 代理池卡片紧凑化与出口节点双列。纯前端，无 API/模型/SQLite schema/Mihomo 内核变更。
- **v0.2.8 只创建 GitHub Release 与附件，不部署**；当前用户实例未被触碰。若后续获部署授权，先正常退出旧实例并确认内核退出，再替换程序；禁止运行中复制或覆盖 SQLite 数据库。
- 历史 v0.2.7 标签仍为 `14cc9f570b44422b9dfc1deec94b3556749a1a05`，Release 5 个资产未变；v0.2.6 及更早标签与资产未触碰。

## 当前运行服务

- 2026-11-03 10:33 实测：uniproxy PID18324（`C:\Users\h2147\AppData\Local\Temp\go-build2592475721\b001\exe\uniproxy.exe`，09:30:08 启动）与内核 mihomo PID24836（`C:\ProgramData\UniProxy\core\mihomo.exe`，09:30:09 启动）正在运行；`127.0.0.1:9090` 归 18324，`127.0.0.1:20006` 归 24836，19090 无监听。来源未被本轮确认，保留不动；本轮未停止、替换、读取或修改用户数据库。
- 部署前置条件（若后续获授权）：确认无实例与子内核运行，或先让其正常退出并确认内核退出 → 需要备份时停机后复制 → 替换主程序并沿用原数据目录/启动参数 → health 与 `/api/core/inspect` 复核；禁止运行中复制或覆盖 SQLite 数据库，禁止混用不同库的 WAL/SHM。

## v0.2.8 正式下载副本

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.8/uniproxy-windows-amd64.exe>) | 13,355,008 | `faff52411584a2aa44092775aa4282b22e8b28efcc889163822bbd52eb63a190` |
| [Windows arm64](<../bin/releases/v0.2.8/uniproxy-windows-arm64.exe>) | 12,439,040 | `6a729707e0034dc5cbf50f5f85a870cfb44ed0f0bcaec82acaa7fabf2fe09fe0` |
| [Linux amd64](<../bin/releases/v0.2.8/uniproxy-linux-amd64>) | 12,996,792 | `4445a859aabe425f47061ca1ebd873f7596e33a48d2a9ce2b91d5359621ccc9e` |
| [Linux arm64](<../bin/releases/v0.2.8/uniproxy-linux-arm64>) | 12,386,488 | `20b956356edacd0f0adb0146a841f95dc440a1ca3a64b399e8209d070b3e106b` |
| [SHA256SUMS](<../bin/releases/v0.2.8/SHA256SUMS.txt>) | 360 | `88f7e9276434c57aac65afa42c7fae8afbdcfcdf0d6e1f362bc116de34389c1d` |

副本位于 `bin/releases/v0.2.8/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致。

## v0.2.8 发布验收（M26–M29）

- M26 新建代理链弹窗：入口监听移到名称下方、节点库上方，认证置底；入口监听单行五列；节点库桌面 3×3；端口留空提交 0、手工限 1024–65535。
- M27 全局视觉：新增最后加载的 [theme.css](<../src/web/theme.css>)，暖纸亮色、深森林墨、青绿/酸橙强调、serif 标题；用布局表达层级，隐藏重复说明，压缩表格/卡片/弹窗；390px 底部导航七列单行无溢出。
- M28 代理池编辑：监听区上移，分组固定四行内滚，节点区加宽加高，批量栏常驻；即时勾选、筛选、排序与真实延迟检测保持。
- M29 代理池卡片：每页 10 节点，节点卡首行健康/名称/延迟/操作，次行协议徽章，地址/中转/检测/错误收进展开区；成员区 >430px 两列、≤430px 单列，列表 380px 内滚，触控按钮 44px。
- 本地验证：Go 全量（显式本机内核）、gofmt/vet/diff check、严格 TS、生产 Web 构建、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52、两个 importer fuzz、TestBuilt 与真实控制台均通过。完整命令与耗时见 [v0.2.8 发布验收](<release-0.2.8-verification.md>)。
- 验证边界：Linux/ARM64 资产仅构建与摘要核对，未在本轮实机运行；Docker、race、容量/吞吐和真实远端节点连通未测；fixture UI 不等于真实代理验证。

## 相关历史与边界

- M25 代理链已随 v0.2.7 发布：2–16 跳有序合成出站、`dialer-proxy` 串接、chain 监听、整链检测与原子创建；任一跳缺失/禁用/中转异常整链 REJECT，不缩短、不回退直连。详见[代理链验收](<chain-verification.md>)。
- 2026-11-02 9router JP-US 链排障为运行配置修复：第一跳由台湾家宽改为日本原生后链测试与 20006 HTTP/SOCKS 恢复；无源码改动。
- 更早历史：v0.2.4 于 2026-11-01 12:18:53 发布并部署替换本机实例（停机备份 `C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`）；v0.2.5/v0.2.6/v0.2.7 均只发布未部署；v0.2.0–v0.2.3 标签与资产不变。
- 当前栈仍为 Go + React/TypeScript + SQLite 加密配置 payload + 外置 Mihomo 1.19.32；无 schema 迁移、无内核升级。
- 常规测试清空 `UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD` 与 `UNIPROXY_TEST_APP_PATH/UNIPROXY_TEST_CORE_PATH`。既有 Docker 忽略未排除 `.local`，不构建/上传本机 Docker 上下文。

## 历史正式下载副本（v0.2.7）

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

- v0.2.8 已发布并设 Latest，**未部署**；工作树在源码提交 `76f3c59` 后仅保留本轮发布验收文档回写，随本次收尾文档提交（`[skip ci]`）推送。
- 当前用户实例 PID18324/PID24836 保持运行，未触碰；不要因会话恢复自动启动、停止或替换服务。
- 本地候选 `bin/updates/release-0.2.8/uniproxy-windows-amd64.exe` 仅为发布前验收构建，不是正式资产；正式资产以下载副本与 GitHub Release 为准。
- 用户侧真实节点连通、日常使用与吞吐需另行观察；发布与程序验收不等于节点可用或吞吐达标。
- Linux amd64 实机运行验收停留在 v0.2.5 候选（M22 修复）；v0.2.6–v0.2.8 的 Linux 资产仅构建与摘要核对。Linux arm64、裸机/Docker、race、容量/吞吐与真实远端节点未在本轮验证。

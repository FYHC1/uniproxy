# 多引擎 sidecar 开发交接（新对话入口）

- 日期：2026-11-04
- 分支：`feat/multi-engine-sidecars`
- 工作目录：`C:\CustomFiles\CodeProjects\Github\uniproxy-sidecar`
- 基线：`main` = `eb7675c`；v0.2.9 tag = `6ff15a8`
- 状态：第一批 1–5、第 6 项最小 app/API 集成、UI 接入、第四批端口范围/预检/脱敏、真实流程第 1–3 层（Windows/WSL）、importer Xray-only 识别与真实订阅验收、固定版本 Xray 在线安装器/多文件 ZIP 安全解压/设置页安装状态、工作树复审与 kr-vps 真实远端订阅 E2E（三协议 PASS），以及真实 GitHub 资产下载与生产安装路径验证均已实现；不提交、不发布、不部署。
- 详细方案：[multi-engine-sidecar-requirements.md](<multi-engine-sidecar-requirements.md>)

## 1. 目标

Mihomo 保持主核，Xray 作为 Xray-only 节点的本地 SOCKS sidecar；sing-box sidecar 暂缓，只有确实需要 AnyTLS/Snell/ShadowTLS/Naive/Tor 等 R01 之外协议时再加。

不要做三个对等主核，不要把 Xray/sing-box 做成第二个公共入口。

## 2. 当前基线

- 当前发布 v0.2.9；源码提交 `6ff15a8`，tag `v0.2.9`，main 之后文档提交 `eb7675c`。
- 现有固定内核 Mihomo 1.19.32；`internal/core` 生产 1,915 行/测试 2,543 行，`internal/coreinstall` 780/1,355，`internal/importer` 5,224/3,421，`internal/app` 2,709/3,918，`internal/model` 334，`src/web` 2,077。
- 当前真实订阅 1064 条：严格 697、预览 1062；剩余 2 条是无效 UUID/未知导出器参数，换 sidecar 不会自动修复。

## 3. 第一批任务（1–6、UI 接入与真实流程验证已实现）

1. 在 `internal/model/types.go` 增加：
   - `Node.Engine`（默认 `mihomo`）
   - `Node.SidecarEndpoint`
   - `SidecarStatus`（引擎、版本、运行状态、错误）
2. 新建 `internal/sidecar`：
   - Xray 配置生成（每个节点一个本地 SOCKS inbound）
   - `xray run -test -c`
   - 单进程启动/停止/健康检查
   - 端口分配（建议 30000–30999）
3. `internal/core/compiler.go`：
   - sidecar 节点生成 Mihomo `socks5` 出站 `x-<NodeID>`
   - 真实远端地址保留在 `Node.Server`/`Node.Port`，sidecar 配置使用真实地址；Mihomo 只使用 `127.0.0.1:<port>`
4. 链位置校验：
   - 本地 sidecar 只允许单独出站或链首
   - 链中间/链尾直接报错“sidecar 节点只允许位于链首”
5. 先用 3 个离线样本做 POC：VLESS Encryption、VMess XHTTP、Reality 扩展；不接真实订阅。
6. 最小 app/API 集成已完成（2026-11-04）；设置开关、节点引擎徽章、导入预览和错误提示已接入（2026-11-04），功能开关默认关闭；sidecar 专属端口范围、保存/应用前 bind 预检、200 节点上限和日志脱敏已完成（2026-11-04 第四批）；Windows 11 与 WSL2 Fedora 44 的真实 app/Mihomo/Xray 三协议端到端、重启与端口释放验证已完成（2026-11-04）；kr-vps:21443 的真实订阅 E2E 三协议（VLESS Encryption、VMess XHTTP、Reality）顺序验收通过（2026-11-04，见 [sidecar-real-process-verification.md](<sidecar-real-process-verification.md>)）。
7. importer Xray-only 识别已完成（2026-11-04）：`ParsedNode.Engine`、`xray_uri.go`、`xray_vmess.go` 覆盖 VLESS Encryption、VLESS/VMess XHTTP、Reality 扩展、FinalMask、TLS cipher suites 和 flow=xtls-rprx-vision-udp443；1394 条真实 VLESS 订阅导入后 77 个 Xray 节点全部通过 `xray -test`，真实远端 SOCKS5 验证当前 38/77 可达。
8. Xray 在线安装器已完成（2026-11-04）：`internal/sidecarinstall` 固定 26.10.31、四平台官方资产名与 SHA256、多文件 ZIP 平坦白名单安全解压、无覆盖发布；`internal/sidecar` 自动发现安装目录并暴露安装状态；`POST /api/sidecars/install` 与设置页安装/重试/进度已接入；Windows 本地官方 ZIP 安装级验证 PASS，真实 GitHub 资产下载与生产安装路径 PASS（官方 ZIP 16,623,726 bytes、SHA256 与内置 windows-amd64 pin 一致，4.60s）；匿名 GitHub API 配额耗尽时仍会返回 403，Linux 实机安装待验证。

## 4. 关键代码入口

- 模型：`internal/model/types.go`
- 核心接口：`internal/app/app.go:20-29`
- Mihomo 编译器：`internal/core/compiler.go:340-351`（document map）、`:442-483`（nativeProxy）
- Apply/验证：`internal/core/manager.go:150-250`
- 控制器/健康：`internal/core/controller.go`、`internal/core/checks.go`
- 安装：`internal/coreinstall`、`internal/core/installation.go`、`internal/sidecarinstall`、`internal/sidecar/install.go`
- 导入：`internal/importer`、`internal/app/imports.go`
- 前端：`src/shared/types.ts`、`src/web/ImportDialog.tsx`、`src/web/SettingsPage.tsx`
- E2E：`internal/e2e/real_process_test.go`、`internal/e2e/real_remote_test.go`
- 决策：`docs/decisions.md` D01/D27/D34/D35/D36/D37/D38
- 需求：`docs/requirements.md` R01/R05

## 5. 必须遵守的边界

- sidecar 只监听 `127.0.0.1`，随机用户名密码，不暴露公网。
- 一个引擎一个进程，不为每个节点起进程；第一阶段最多 200 个 sidecar 节点。
- sidecar 配置中的真实节点凭据只存在加密 state/临时内存，不进日志、预览或仓库。
- sidecar 失败只影响其节点；相关池返回失败/REJECT，不直连回退。
- 本地 sidecar 不得出现在链中间或链尾；远程 sidecar/整链封装属于后续阶段。
- 不修改、读取或复制用户数据库、主密钥、原始订阅、代理凭据；测试只用临时数据。
- 不启动、停止、替换用户正在运行的 UniProxy/Mihomo 实例。
- 不在 `main` 上直接改共享模型；所有工作在 `feat/multi-engine-sidecars` 分支。

## 6. 第一轮验收

- 旧 state 中 `Engine` 为空 → 默认 mihomo，行为不变。
- 3 个 Xray-only 配置通过 `xray -test`，经本地 SOCKS 实际转发。
- 现有 listen/pool/chain 回归通过；sidecar 节点单独/链首可用；链中间/链尾保存报错。
- sidecar 崩溃不影响原生 Mihomo 节点。
- `gofmt`、`go vet ./...`、`go test ./...`、`pnpm typecheck`、`pnpm test:ui`、`pnpm build:web`、`pnpm test:e2e` 通过。

## 7. 备注

- Xray v26.10.31 实测：`xray vlessenc` 生成 ML-KEM-768 1184 字节 key；`xray run -test` 接受 VLESS Encryption + flow=udp443 + VMess XHTTP 配置。
- Xray 没有 Hysteria2/TUIC/SSH、Clash HTTP API 和 SIGHUP；sing-box 有 Hysteria2/TUIC/SSH，但缺 XHTTP/VLESS Encryption/VMess KCP。因此主核保持 Mihomo。
- 详细能力矩阵、阶段计划、测试矩阵和风险见[需求分析](<multi-engine-sidecar-requirements.md>)。

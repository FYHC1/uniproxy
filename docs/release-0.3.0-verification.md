# UniProxy v0.3.0 发布验收

更新日期：2026-11-04
发布阶段状态：**已发布（2026-11-04 19:43:33 +08；GitHub Release 与四平台资产已上线）；只发布，不部署。**

## 1. 范围

本版为 v0.2.9 之后的多引擎 sidecar 版本，汇总 M31 第一批到第四批：

- Mihomo 1.19.32 保持主核；Xray-only 节点由本地单进程 Xray 26.10.31 sidecar 承载，每个节点一个 127.0.0.1 SOCKS5 inbound，Mihomo 编译为 `x-<NodeID>` 的 socks5 出站。
- sidecar 节点只允许单独出站或链首；链中间/链尾保存和编译均报“sidecar 节点只允许位于链首”。
- 新增固定版本 Xray 在线安装器、端口范围与 bind 预检、200 节点上限、日志/API 凭据脱敏、崩溃有界重启与节点不可用标记。
- 新增 Xray-only URI 识别：VLESS Encryption、VLESS/VMess XHTTP、Reality 扩展、FinalMask、TLS cipher suites；固定 Xray 26.10.31 已移除的 Trojan flow 在导入和配置生成阶段拒绝。
- 设置页、节点页与导入预览接入引擎徽章、sidecar 状态、安装面板和端口范围。
- 版本与文档：`internal/version/version.go`=0.3.0、`internal/version/version_test.go`=0.3.0、`package.json`=0.3.0；README/CHANGELOG/开发计划/接续快照同步；新增 `docs/releases/v0.3.0.md` 与本记录。
- 不包含：部署、停止/替换运行实例、SQLite schema 迁移、Mihomo 内核升级、sing-box sidecar、把本地 sidecar 作为公共入口。

## 2. 发布前本地验证

### 2.1 静态检查

- 修改的 19 个 Go 文件按 LF 规范化后 `gofmt -d`：无差异（`GOFMT_BAD=0`）。
- `go vet ./...`：通过（`VET_EXIT=0`）。
- `git diff --check`：通过（`DIFF_CHECK_EXIT=0`）。

### 2.2 Go 全量测试

命令：`go test ./... -count=1 -p 1`。

结果：全部通过（`TEST_EXIT=0`；app 11.747s、core 6.714s、sidecar 1.754s、sidecarinstall 0.198s、importer 0.709s、store 0.347s、cmd/uniproxy 0.128s）。

### 2.3 前端与浏览器

- `pnpm typecheck`：通过。
- `pnpm test:ui`：44/44 通过（含全部浏览器回归）。
- `pnpm build:web`：通过（Vite 7.3.7，4656 modules）。
- `pnpm test:e2e`：52/52 通过（1.3m）。

### 2.4 真实流程与真实订阅

- Windows 11 amd64：真实 `uniproxy` 子进程 + Mihomo 1.19.32 + Xray 26.10.31，VLESS Encryption、VMess XHTTP、Reality 三协议经 Mihomo → sidecar SOCKS5 → 本地 Xray 端到端 PASS；stop、同 dataDir 重启恢复与强杀后 4 个端口释放 PASS。
- WSL2 Fedora 44 amd64：同一真实进程测试 PASS 2.12s，Phase 0 POC PASS 0.38s，全量 Go 测试通过。
- kr-vps 真实远端订阅（2026-11-04 16:40–16:41）：VLESS Encryption PASS 10.23s、VMess XHTTP PASS 6.12s、Reality PASS 5.66s；每协议首次导入/强制刷新/应用重启三次均返回 `remote-e2e-ok`。
- Xray 官方资产：`Xray-windows-64.zip` 16,623,726 bytes，SHA256 `2e04a9995a6f2ad1b28c0bb25c708ecb2695e43a1c9c7889c4b6ac2b48cf7a6a` 与内置 windows-amd64 pin 一致；真实 GitHub CDN 下载与生产安装路径 PASS 4.60s；`xray.exe version` 为 26.10.31 a4a3dff。
- 边界：真实验收为单端口顺序协议与单次网络快照，不代表并发、UDP、公网带宽、容量或长期运行；Linux 裸机安装仍待验证。

## 3. CI 与正式资产

### 3.1 源码与 CI

- 源码提交：`e73644a5e70738a87145714050734446528f39d7`（`release: prepare v0.3.0 multi-engine sidecar`）。
- main CI：[run 41822810042](<https://github.com/FYHC1/uniproxy/actions/runs/41822810042>) 成功（Windows、Ubuntu 双平台均通过）。
- annotated tag `v0.3.0`：tag 对象 `dc64ffb82ef7024ed52008773f5c34812543612d`，指向提交 `e73644a5e70738a87145714050734446528f39d7`。
- 标签 CI：[run 41823088724](<https://github.com/FYHC1/uniproxy/actions/runs/41823088724>) 成功；Release workflow：[run 41823088760](<https://github.com/FYHC1/uniproxy/actions/runs/41823088760>) 成功。

### 3.2 正式发布

- 发布时间：2026-11-04 19:43:33 +08。
- Release：[UniProxy v0.3.0](<https://github.com/FYHC1/uniproxy/releases/tag/v0.3.0>)，已设为 Latest，非草稿、非预发布。
- 正式资产与 SHA256：

| 资产 | 字节 | SHA256 |
|---|---:|---|
| `uniproxy-linux-amd64` | 13,361,336 | `49fad3029921739246e5102858a66adb9ed0b5d6f8f0895d4925899f0b888536` |
| `uniproxy-linux-arm64` | 12,648,632 | `db5b3bab8c0d91d1a5604085261cf442ef82061ab88ca09c9fd2b87d83e31f7e` |
| `uniproxy-windows-amd64.exe` | 13,729,792 | `09d8f26de474cd300b5e85ba2c0b967316c23daead50d088d5b5bd6bdb0a8be3` |
| `uniproxy-windows-arm64.exe` | 12,787,712 | `b8eb4433acb823ad048b582a8b27705491a5301149f8615f6e6b6f04999e974d` |
| `SHA256SUMS.txt` | 360 | `4da04d736b1361e54d7a55a762fc7296f3452765f476392a3fb0e841b6fde485` |

下载的 Windows amd64 资产经 `Get-FileHash` 复核为 `09d8f26d...8a8be3`，与 `SHA256SUMS.txt` 及 Release 元数据 digest 一致。

### 3.3 正式 Windows 程序验收

- `-version`：`UniProxy 0.3.0 · commit e73644a5e707 · built 2026-11-04T11:42:50Z`。
- `TestBuilt`：对下载的 `uniproxy-windows-amd64.exe` 设置 `UNIPROXY_TEST_APP_PATH` 运行 `go test ./tests/integration -run TestBuiltCLIVersionAndPublicBindGuard -count=1 -v`，PASS 0.16s（version、syntax、unexpected-positional、public-without-token 四个子测试全部通过）。
- 真实 Windows 控制台：同一正式资产以临时 `-data` 目录和 `-port 19093` 启动，`GET /api/health` 返回 `{"ok":true,"version":"0.3.0"}`，同端口 `-healthcheck` 退出码 0；验证后已停止临时进程，未触碰用户运行实例。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。

# UniProxy v0.3.0 发布验收

更新日期：2026-11-04
发布阶段状态：**发布准备中（本地验证完成；待提交、合并、标签、CI 与正式资产回填）；只发布，不部署。**

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

- 源码提交：待发布后回填。
- main CI：待发布后回填。
- annotated tag `v0.3.0`：待发布后回填。
- 标签 CI / Release workflow：待发布后回填。

### 3.2 正式发布

- 发布时间：待发布后回填。
- Release：待发布后回填。
- 正式资产与 SHA256：待发布后回填。

### 3.3 正式 Windows 程序验收

- `-version`：待发布后回填。
- `TestBuilt`：待发布后回填。
- 真实 Windows 控制台：待发布后回填。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。

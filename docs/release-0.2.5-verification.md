# UniProxy v0.2.5 发布验收

更新：2026-11-02（Asia/Shanghai）。本记录覆盖 M22（Linux 稳定性修复）与 M23（全局出站网卡）合并发布的源码、验证与正式资产。发布阶段状态：**已于 2026-11-02 02:27:40（Asia/Shanghai）正式发布，未部署**。

## 1. 范围

- 版本：源码默认版本 `0.2.5`；发布构建由 [release.yml](<../.github/workflows/release.yml>) 注入 tag 版本、提交与构建时间。
- 变更：[processguard](<../internal/processguard/processguard.go>)（Linux `Pdeathsig`）、[controller.go](<../internal/core/controller.go>)（探针与 `/proxies` 有界重试）、`Settings.OutboundInterface` 与顶层 `interface-name` 编译、`GET /api/interfaces`、设置页「03 全局出站网络」。
- 不包含：订阅解析语义、节点身份、中转依赖、数据库 schema、内核版本（仍固定 Mihomo 1.19.32）、系统代理/防火墙改动。

## 2. 发布前本地验证

### 静态检查与单元/集成测试

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：无输出。
- `go test -mod=readonly -p 1 ./... -count=1`（显式 `UNIPROXY_TEST_CORE_PATH` 指向本机 Mihomo 1.19.32）：全部 ok，退 0。cmd/uniproxy 1.064s、app 10.768s、core 13.729s、coreinstall 1.077s、importer 1.120s、processguard 2.042s、store 0.937s、tests/integration 8.297s。
- `npx tsc --noEmit --noUnusedLocals --noUnusedParameters`：退 0。
- `pnpm run build:web`：退 0，Vite 7.3.7，4650 modules，`index-D4iDTw75.js` 525.51 kB（gzip 155.08 kB）、`index-BM9nabfr.css` 71.74 kB；仅有非失败的 >500 kB 提示。
- `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./...`：通过。
- `pnpm test:ui`：39/39 前端工具单测通过，生产 Vite 内存构建与全部 fixture 场景 PASS，零未捕获浏览器错误。
- `pnpm test:e2e`：43/43 隔离真实 API 浏览器用例通过（60.0s）。

### 浏览器用例顺序修正

首次跑全量 E2E 时 [outbound-interface.spec.ts](<../tests/browser/outbound-interface.spec.ts>) 的网卡清单断言失败：接口清单按 Go `sort.Strings`（字节序）返回，而选择器按 `localeCompare`（本地化排序）渲染，测试用 API 顺序做全等比较。产品行为正确（两侧都只列已启用非回环网卡），是测试断言把展示顺序当契约。改为比较集合成员、首个选项仍断言「自动」，重跑该文件 3/3、全量 43/43 通过。

### Windows 候选程序验收

- 候选：`bin/updates/release-0.2.5/uniproxy-windows-amd64.exe`，14,020,096 字节，SHA256 `8766f1a8c41317f57947208c5cc0bd7f05630ca8f73329aa3cec1c50c8304e7a`；`-version` 为 `UniProxy 0.2.5 · commit local-2025-v · built 2026-11-01T17:37:21Z`。**本地验证构建，不是发布资产。**
- `UNIPROXY_TEST_APP_PATH` 指向该候选、`UNIPROXY_TEST_CORE_PATH` 指向本机内核，`go test ./tests/integration -run 'TestBuilt|TestWindowsParentLifetime|TestStartup'` 通过（3.772s）：CLI 版本/语法/公网绑定保护、嵌入 UI、三轮强杀重启与子进程随退、启动失败隔离报告、healthcheck 不堆日志、6 格式订阅预览/保存。
- 真实控制台脚本 `python scripts/test-windows-console.py`：两轮 WM_CLOSE 均退出 0 且可重启；独立失败窗口保留并落盘报告后退出 1。

### WSL2 实机复验（Linux amd64）

- 环境：WSL2 / Fedora 44，内核 6.18.54.1-microsoft-standard-WSL2。候选 `.local/v025-wsl/uniproxy-linux-amd64`，13,668,512 字节，SHA256 `d34569410516ec547d8cdf742cbecaf43322f84ec99797d4fa822ad534899fb7`，`-version` 为 `UniProxy 0.2.5 · commit local-v025-w · built 2026-11-01T17:44:41Z`。内核为官方 Mihomo 1.19.32 linux-amd64-compatible，解压后 SHA256 `7a0d59da2e678d56c899a3db996a2ad8963286c4f3634b0451435db248f13fa1`。
- 启动与认证：`-host 127.0.0.1 -port 19090 -data <临时目录>`，health 50ms 返回 `{"ok":true,"version":"0.2.5"}`，`/api/core/inspect` 检出 1.19.32。
- 网卡清单：`GET /api/interfaces` 返回 6 个接口，字段恰为 `addresses/loopback/name/up`；`lo` 标 loopback，链路口址（fe80::/10、169.254/16）未出现。
- 网卡校验：`lo` 被拒（回环）、不存在的名称被拒、含首尾空白的名称被拒，三次都返回 400 且已保存设置仍是空字符串；合法网卡 `br-ecc835eaff5f` 返回 200 并持久化为 `outboundInterface`。
- 应用与流量：设好网卡后首次 `POST /api/runtime/apply` 直接 200（running true、appliedRevision 1），无需重试；导入本机回环 HTTP 节点并创建 mixed 监听 28183，提交即 appliedRevision 3、listening true，HTTP 与 SOCKS5 均返回 `UNIPROXY-WSL-E2E-OK`。
- 强杀与恢复：监听生效时 `kill -9` 主程序，2 秒后内核消失、28183 释放；同一数据目录重启 50ms 健康，设置/节点/监听保留，runtime running true、dirty false、appliedRevision 3，HTTP 流量恢复。
- 正常停止：`POST /api/runtime/stop` 后内核退出，SIGTERM 后 100ms 主程序退出，无遗留内核或端口，数据目录仅剩 `core`、`master.key`、`uniproxy.db`（无残留 WAL/SHM）。
- 原始内核对照：直接以 `interface-name` 起 Mihomo，绑定现存网卡时出站 socket 源地址为 `172.18.0.1%br-ecc835eaff5f`；绑定不存在的网卡时直接失败、不回退直连。两例对公网 IP 字面量都没拿到 HTTP 响应，说明本机 WSL 出站本身不可用，因此该对照只证明「绑定生效/不生效」，不作为连通性结论。

## 3. CI 与正式资产

### 推送与工作流

- 源码提交：`ede12bb`（M22 Linux 稳定性）、`a4dd45d`（M23 全局出站网卡）、`1e1f144`（v0.2.5 版本与发布材料），已推送 origin main。
- main CI [41336207092](<https://github.com/FYHC1/uniproxy/actions/runs/41336207092>)：Ubuntu 37s、Windows 1m52s，全部 success（仅 Node20 action 与 ubuntu-latest 迁移提示）。
- annotated tag `v0.2.5`，tag 对象 `271ced2d6db943ae3205c190254c2cb3cbf2c619`，解引用为 `1e1f14417c677fdcc9a95858e03daeda41f1c3f8`。
- 标签 CI [41336634581](<https://github.com/FYHC1/uniproxy/actions/runs/41336634581>)：success；Release [41336634537](<https://github.com/FYHC1/uniproxy/actions/runs/41336634537>)：四平台 build 与 release job 全部 success。
- Release 于 2026-11-02 02:27:40（Asia/Shanghai）发布，非 draft、非 prerelease，并用 `gh release edit --notes-file docs/releases/v0.2.5.md --latest` 应用中文说明；远端 body 与本地文件一致（2336 字符）。

### 正式资产（GitHub API 与下载副本双重核对）

| 文件 | 字节 | SHA256 |
|---|---:|---|
| uniproxy-windows-amd64.exe | 13142528 | `570cd2e6aae76ab07f6b3c7d8ac72a04a4fb62ff0801a66ac88cecccb0320161` |
| uniproxy-windows-arm64.exe | 12235776 | `36d89fbe1592152c58ee1a9e9f19059f866f9d026ba16ba22b88b8140ce49344` |
| uniproxy-linux-amd64 | 12787896 | `a794bcabaea240037588657111667a1d5e6adcc1d44b26cf0b7c99c022458e70` |
| uniproxy-linux-arm64 | 12124344 | `defea76d92163dcfdceefc9af8d0eced3702fc1c9df2149d8f00b20fd1e954e2` |
| SHA256SUMS.txt | 360 | `c904ad372f69549fd9701820686818229ddfc5f59f2d2ef91015506eda80d2b2` |

下载副本存于 `bin/releases/v0.2.5/`（gitignored），五个文件本地 SHA256 与 GitHub digest、`SHA256SUMS.txt` 逐项一致。

### 正式 Windows 程序验收

- `-version`：`UniProxy 0.2.5 · commit 1e1f14417c67 · built 2026-11-01T18:26:55Z`。
- `UNIPROXY_TEST_APP_PATH` 指向正式资产、`UNIPROXY_TEST_CORE_PATH` 指向本机 Mihomo 1.19.32，`go test ./tests/integration -run 'TestBuilt|TestWindowsParentLifetime|TestStartup'` 通过（3.868s）：CLI 版本/语法/公网绑定保护、嵌入 UI、三轮强杀重启与子进程随退、启动失败隔离报告、healthcheck 不堆日志、6 格式订阅预览/保存。
- `python scripts/test-windows-console.py <exe> <mihomo>`：两轮 WM_CLOSE 退出 0 且可重启；独立失败窗口保留并落盘报告后退出 1。
- 历史 tag/Release 未变：v0.2.0 `a9d3fc8`、v0.2.1 `d8f394e`、v0.2.2 `22849c9`、v0.2.3 `5ed4cdf`、v0.2.4 `72da273`；v0.2.5 为唯一 Latest。

## 4. 部署边界

本版**不部署**：未停止、未替换任何运行中的 UniProxy 实例，未修改用户数据目录或数据库。

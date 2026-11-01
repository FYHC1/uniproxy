# UniProxy v0.2.5 发布验收

更新：2026-11-02（Asia/Shanghai）。本记录覆盖 M22（Linux 稳定性修复）与 M23（全局出站网卡）合并发布的源码、验证与正式资产。发布阶段状态：**发布前验证中**。

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

- 待发布后填写：main CI、tag CI、Release run ID 与结论。
- 待发布后填写：四平台资产字节数与 SHA256。

## 4. 部署边界

本版**不部署**：未停止、未替换任何运行中的 UniProxy 实例，未修改用户数据目录或数据库。

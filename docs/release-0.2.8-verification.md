# UniProxy v0.2.8 发布验收

更新日期：2026-11-03
发布阶段状态：**发布流程准备中；本记录在正式发布后回填 CI、Release 与资产结论。发布不等于部署。**

## 1. 范围

本版为 v0.2.7 之后的前端界面版本，汇总 M26–M29：

- M26 新建代理链弹窗布局：入口监听移到名称下方、节点库上方，认证留底部；入口监听单行五列，节点库桌面三列三排。
- M27 全局界面视觉刷新：最后加载的 `theme.css` 统一页面、弹窗、认证和通知视觉；用布局表达层级，压缩表格、卡片和弹窗间距。
- M28 新建/编辑代理池布局与节点批量栏：监听区上移、分组固定四行内滚、节点区加宽加高、批量栏常驻。
- M29 代理池卡片紧凑化与出口节点双列：每页 10 个节点，节点卡两层结构，容器查询在大于 430px 时两列、窄屏单列。
- 版本与文档：`internal/version/version.go`=0.2.8、`internal/version/version_test.go`=0.2.8、`package.json`=0.2.8；README/CHANGELOG/requirements/decisions/development-plan/session-handoff 同步；新增 `docs/releases/v0.2.8.md` 与本记录。
- 不包含：部署、停止/替换运行实例、数据库 schema 迁移、Mihomo 内核升级、API/模型变更、真实远端节点连通性或吞吐测试。

## 2. 发布前本地验证

### 2.1 静态检查

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：通过。
- `git diff --check`：通过（提交前复核）。

### 2.2 Go 全量测试

命令（显式使用本机 `bin/mihomo.exe`）：

```powershell
$env:UNIPROXY_TEST_CORE_PATH='C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe'
go test -mod=readonly -p 1 ./... -count=1
```

结果：全部通过。

| 包 | 耗时 |
|---|---:|
| cmd/uniproxy | 0.943s |
| internal/app | 11.970s |
| internal/appdata | 0.308s |
| internal/core | 16.112s |
| internal/coreinstall | 1.138s |
| internal/importer | 1.304s |
| internal/model | 0.289s |
| internal/processguard | 2.251s |
| internal/safehttp | 0.754s |
| internal/store | 1.081s |
| internal/version | 0.275s |
| tests/integration | 8.753s |

### 2.3 Fuzz

- `go test ./internal/importer -run '^$' -fuzz=FuzzClientFormatsNeverPanic -fuzztime=10s`：PASS，11.775s，464,578 次执行，新增 54 个 interesting。
- `go test ./internal/importer -run '^$' -fuzz=FuzzParseNeverPanics -fuzztime=10s`：PASS，12.030s，154,855 次执行，新增 59 个 interesting。

### 2.4 前端与浏览器

- 严格 TypeScript：`npx tsc --noEmit --noUnusedLocals --noUnusedParameters` 通过。
- 生产构建：`pnpm run build:web` 通过，Vite 7.3.7，4655 modules；`index-D-RTtYM2.js` 556.38kB（gzip 161.96kB）、`index-xZdLmcBI.css` 110.64kB（gzip 22.97kB）；仅非失败 chunk 体积提示。
- `pnpm test:ui`：42/42 工具单测通过；生产内存构建与全部 fixture 场景通过，零未捕获浏览器错误。
- `pnpm test:e2e`：52/52 通过（1.6m），使用隔离测试 API 与临时数据；`pool-compact`、`pool-selection`、`pool-workflow`、`chain-workflow` 等流程全部通过。

### 2.5 Windows 候选程序（本地验证构建，非发布资产）

- 路径：`bin/updates/release-0.2.8/uniproxy-windows-amd64.exe`。
- 大小：14,231,552 字节。
- SHA256：`701b122eb0aac4850efb5c15440bf5f5a3e299ac084d723db7845c73d83cb1f6`。
- `-version`：`UniProxy 0.2.8-pre.1 · commit local-releas · built 2026-11-03T02:20:10Z`。

`go test ./tests/integration -run '^TestBuilt' -count=1 -v`（APP_PATH 指该候选，CORE_PATH 指本机内核）：

- `TestBuiltCLIVersionAndPublicBindGuard` 通过（version/syntax/unexpected positional/public-without-token）。
- `TestBuiltServiceWithEmbeddedUI` 通过。
- `TestBuiltWindowsAbruptExitAndRestart` 3 轮通过（子进程随主进程退出）。
- `TestBuiltWindowsStartupFailureWritesIsolatedReport` 通过。
- `TestBuiltWindowsHealthcheckFailuresDoNotAccumulateLogs` 通过。
- `TestBuiltSubscriptionCompatibility` 通过。
- 包级结果：`ok ... 4.617s`。

### 2.6 真实 Windows 控制台

真实 Windows 控制台脚本 `python scripts/test-windows-console.py <候选> bin/mihomo.exe`：

```json
{"case":"actual-console-WM_CLOSE","round":1,"exit":0,"running_before_close":true}
{"case":"actual-console-WM_CLOSE","round":2,"exit":0,"running_before_close":true}
{"case":"standalone-error-console-retained","exit_after_close":1,"report_saved":true}
```

### 2.7 跨平台编译

以下目标 `CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' ./cmd/uniproxy` 全部通过：

| 目标 | 字节 | SHA256 |
|---|---:|---|
| windows/arm64 | 13,137,408 | `cd94bd2a5ae6180c5b31f730b8530d7d60dc3325f6c2525dd2c63cbb5196b4fe` |
| linux/amd64 | 13,881,504 | `d184b854805a5ae65625e0abfe0e38b2bca8d17028cba0103fb5108ed80202ca` |
| linux/arm64 | 13,041,824 | `05802a395d513e7706360f601f28c10832c1731a4bc21adfa4498a9f0793a46b` |

## 3. CI 与正式资产

待正式发布后回填：源码提交、main CI、annotated tag、tag CI、Release workflow、四平台资产大小与 SHA256、正式 Windows 程序验收。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。操作前应重新核对当时的用户服务 PID 与端口；本记录不把历史 PID 观察当作当前状态。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。不得把测试候选当作正式发布资产。

# UniProxy v0.2.7 发布验收

更新日期：2026-11-02
发布阶段状态：**发布前验证完成；CI 与正式资产待回填**

## 1. 范围

本版为 v0.2.6 之后的补丁版本，对应 M25“链式代理”。代码范围：

- 新增链数据模型、存储归一化、2–16 跳校验、合成出站编译、整链检测与链/入口 API。
- 新增前端「代理链」页面、双栏创建/编辑窗口、监听目标类型扩展及对应样式与纯函数。
- 新增 Go 单元/集成测试、真实两跳回环内核测试、UI 工具测试和 5 例隔离浏览器流程。
- 版本与文档：`internal/version/version.go`=0.2.7、`internal/version/version_test.go`=0.2.7、`package.json`=0.2.7；README/CHANGELOG/requirements/decisions/development-plan/session-handoff 同步；新增 `docs/releases/v0.2.7.md` 与本记录。
- 不包含：部署、停止/替换运行实例、数据库 schema 迁移、Mihomo 内核升级、真实远端节点连通性或吞吐测试。

## 2. 发布前本地验证

### 2.1 静态检查

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：通过。
- `git diff --check`：通过（提交前复核）。
- 文件编码：涉及源码与文档为 UTF-8 无 BOM、LF、结尾换行。

### 2.2 Go 全量测试

命令（显式使用本机 `bin/mihomo.exe`）：

```powershell
$env:UNIPROXY_TEST_CORE_PATH='C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe'
go test -mod=readonly -p 1 ./... -count=1
```

结果：全部通过。

| 包 | 耗时 |
|---|---:|
| cmd/uniproxy | 0.830s |
| internal/app | 13.855s |
| internal/appdata | 0.337s |
| internal/core | 15.748s |
| internal/coreinstall | 1.109s |
| internal/importer | 1.172s |
| internal/model | 0.287s |
| internal/processguard | 2.216s |
| internal/safehttp | 0.619s |
| internal/store | 0.907s |
| internal/version | 0.268s |
| tests/integration | 9.269s |

`internal/core` 中的 `TestRealChainRoutesThroughEverySyntheticHop` 使用两个本机回环 SOCKS 上游与本地 HTTP origin，确认两跳都收到真实连接；未连接任何远端节点。

### 2.3 Fuzz

- `go test ./internal/importer -run '^$' -fuzz=FuzzClientFormatsNeverPanic -fuzztime=10s`：PASS，11.848s，500,855 次执行，新增 71 个 interesting。
- `go test ./internal/importer -run '^$' -fuzz=FuzzParseNeverPanics -fuzztime=10s`：PASS，11.869s，115,976 次执行，新增 43 个 interesting。

### 2.4 前端与浏览器

- 严格 TypeScript：`npx tsc --noEmit --noUnusedLocals --noUnusedParameters` 通过。
- 生产构建：`pnpm run build:web` 通过，Vite 7.3.7，4654 modules；`index-D4D2OA-E.js` 556.99kB（gzip 162.15kB）、`index-Dj_1XGal.css` 83.99kB（gzip 16.98kB）；仅非失败 chunk 体积提示。
- `pnpm test:ui`：42/42 工具单测通过；生产内存构建与全部 fixture 场景通过，零未捕获浏览器错误。
- `pnpm test:e2e`：48/48 通过（1.2m），使用隔离测试 API 与临时数据；`chain-workflow.spec.ts` 5/5 通过。

### 2.5 Windows 候选程序（本地验证构建，非发布资产）

- 路径：`bin/updates/release-0.2.7/uniproxy-windows-amd64.exe`。
- 大小：14,205,440 字节。
- SHA256：`1d76d7d45720c08aef32e1378339809527b9a9dcbda603f64b951e473514392f`。
- `-version`：`UniProxy 0.2.7-pre.1 · commit local-releas · built 2026-11-02T13:33:06Z`。
- 构建时带本地嵌入前端资源；候选仅用于程序验收，不是 GitHub Release 资产。

`go test ./tests/integration -run '^TestBuilt' -count=1 -v`（APP_PATH 指该候选，CORE_PATH 指本机内核）：

- `TestBuiltCLIVersionAndPublicBindGuard` 通过（version/syntax/unexpected positional/public-without-token）。
- `TestBuiltServiceWithEmbeddedUI` 通过。
- `TestBuiltWindowsAbruptExitAndRestart` 3 轮通过（子进程随主进程退出）。
- `TestBuiltWindowsStartupFailureWritesIsolatedReport` 通过。
- `TestBuiltWindowsHealthcheckFailuresDoNotAccumulateLogs` 通过。
- `TestBuiltSubscriptionCompatibility` 通过（6 种格式预览/保存/安全拒绝）。
- 包级结果：`ok ... 4.045s`。

真实 Windows 控制台脚本 `python scripts/test-windows-console.py <候选> bin/mihomo.exe`：

```json
{"case":"actual-console-WM_CLOSE","round":1,"exit":0,"running_before_close":true}
{"case":"actual-console-WM_CLOSE","round":2,"exit":0,"running_before_close":true}
{"case":"standalone-error-console-retained","exit_after_close":1,"report_saved":true}
```

### 2.6 跨平台编译

以下目标 `CGO_ENABLED=0 go build ./cmd/uniproxy` 全部通过：

| 目标 | 字节 | SHA256 |
|---|---:|---|
| windows/arm64 | 13,111,296 | `49c3e427f9641f4d3b78e90f15d26875633c769805c1954f48fcb76da799d191` |
| linux/amd64 | 13,852,832 | `7048164504aff90c4c51622db1da4e5234259cfdf17f5b3e2674166e27df20b6` |
| linux/arm64 | 13,041,824 | `075a3274c053db17f419a2671d1eb821a8b3b14370ea816b710091d80773df74` |

## 3. CI 与正式资产

### 3.1 提交流与 CI

- 源码提交：待回填。
- main CI：待回填。
- annotated tag `v0.2.7`：待回填。
- tag CI：待回填。
- Release workflow：待回填。

### 3.2 Release 与正式资产

- `v0.2.7` Release：待回填。

| 文件 | 字节 | SHA256 |
|---|---:|---|
| uniproxy-windows-amd64.exe | 待回填 | 待回填 |
| uniproxy-windows-arm64.exe | 待回填 | 待回填 |
| uniproxy-linux-amd64 | 待回填 | 待回填 |
| uniproxy-linux-arm64 | 待回填 | 待回填 |
| SHA256SUMS.txt | 待回填 | 待回填 |

- 正式 Windows x64 程序验收：待回填。
- 历史 `v0.2.0`–`v0.2.6` 标签对象与解引用提交保持不变（发布后复核）。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。发布验收时本机仍运行用户自行启动的本地预览 `0.2.6-chain-preview.1`：主程序 PID 24580（`bin/updates/chain-preview/uniproxy-windows-amd64.exe`，2026-11-02 20:11:00 启动）+ 内核 PID 14272（`C:\ProgramData\UniProxy\core\mihomo.exe`，20:11:01，父进程 24580），`127.0.0.1:9090` 归主程序、20006 归内核；本轮未停止、未替换、未改其数据目录。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。不得把测试候选当作正式发布资产。

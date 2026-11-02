# UniProxy v0.2.7 发布验收

更新日期：2026-11-02
发布阶段状态：**2026-11-02 21:52:52（Asia/Shanghai）已正式发布并设 Latest；不含部署**

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

- 源码提交：[`14cc9f570b44422b9dfc1deec94b3556749a1a05`](<https://github.com/FYHC1/uniproxy/commit/14cc9f570b44422b9dfc1deec94b3556749a1a05>)（`feat: add chained proxies for v0.2.7`，49 文件，+1919/−80）。
- main CI [41456237334](<https://github.com/FYHC1/uniproxy/actions/runs/41456237334>)：成功；Ubuntu 47s、Windows 1m52s，均完成 `pnpm run build:web`、`go test ./...`、`go vet ./...`、`go build ./cmd/uniproxy`。
- annotated tag `v0.2.7`：tag 对象 `568f8639935eef3cbdd9090b19ff4d2b0dde53da`，解引用为 `14cc9f570b44422b9dfc1deec94b3556749a1a05`。
- tag CI [41456604601](<https://github.com/FYHC1/uniproxy/actions/runs/41456604601>)：成功；Ubuntu 39s、Windows 1m49s。
- Release workflow [41456604526](<https://github.com/FYHC1/uniproxy/actions/runs/41456604526>)：成功；linux-amd64、linux-arm64、windows-amd64、windows-arm64 四个构建 job 与 release job 全部完成。

### 3.2 Release 与正式资产

- `v0.2.7` Release：<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.7>，发布于 2026-11-02 21:52:52（Asia/Shanghai，UTC 13:52:52），非 draft、非 prerelease、已设 Latest；中文说明通过 `gh release edit --notes-file` 应用，与 [docs/releases/v0.2.7.md](<releases/v0.2.7.md>) 正文逐字一致（2019 字符）。

| 文件 | 字节 | SHA256 |
|---|---:|---|
| uniproxy-windows-amd64.exe | 13,328,896 | `9eff0c7de7cb4b9c869fdbf697abeb7a0a7f236430d98c1244779cf430d41cbb` |
| uniproxy-windows-arm64.exe | 12,412,928 | `792f4083d87a3fc8e64d36c2165beb9ce23230d0d3326509f612f9890a9fa1db` |
| uniproxy-linux-amd64 | 12,972,216 | `1d6b937714ec60aeef58d2a6b172c21d4b104a4ee10599963b2b2cfbf4e8e1f2` |
| uniproxy-linux-arm64 | 12,320,952 | `fd549e9daa6ee72f0f906924561da87a7cca3942aa75815147f2e8db4ddb2aa4` |
| SHA256SUMS.txt | 360 | `cc23e3ba0b45fb8e2949be9a86b012e7c3fcdc27fd5f25113f2f6c6264ec5739` |

- 正式 Windows x64 程序：`UniProxy 0.2.7 · commit 14cc9f570b44 · built 2026-11-02T13:51:54Z`；官方 `TestBuilt*` 全部 PASS（4.385s）；真实控制台 WM_CLOSE 两轮退出 0 并重启、独立失败窗口保留报告后退出 1，均通过。
- 五个正式资产已下载到 `bin/releases/v0.2.7/`（gitignored），大小与 SHA256 和 GitHub digest、`SHA256SUMS.txt` 逐项一致。
- 历史 `v0.2.6` tag 对象仍为 `24346228a70db7c5e4530c2744389bed87db441a`（解引用 `dba132cb379fe113b897b787fef3a94ab91b8bef`）；`v0.2.0`–`v0.2.5` 标签与 Release 未改动。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。发布验收时本机仍运行用户自行启动的本地预览 `0.2.6-chain-preview.1`：主程序 PID 24580（`bin/updates/chain-preview/uniproxy-windows-amd64.exe`，2026-11-02 20:11:00 启动）+ 内核 PID 14272（`C:\ProgramData\UniProxy\core\mihomo.exe`，20:11:01，父进程 24580），`127.0.0.1:9090` 归主程序、20006 归内核；本轮未停止、未替换、未改其数据目录。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。不得把测试候选当作正式发布资产。

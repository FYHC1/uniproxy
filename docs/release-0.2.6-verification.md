# UniProxy v0.2.6 发布验收

更新日期：2026-11-02
发布阶段状态：**发布前验证完成；CI 与正式资产待回填**

## 1. 范围

本版为 v0.2.5 之后的补丁版本，对应 M24“现代 VLESS/VMess 协议兼容”。代码范围：

- 新增 `internal/importer/modern.go`、`internal/importer/modern_test.go`。
- 修改 `internal/importer/{uri.go,transport.go,native.go,vmess.go,security_test.go,uri_test.go,README.md}`。
- 支持 VLESS XHTTP（含 Xray `extra`/`downloadSettings`/`xmux`/reuse 选项）、HTTPUpgrade→WS `v2ray-http-upgrade`、VMess KCP/mKCP、ECH、`vcn`/`pcs`、VLESS Encryption、`raw`/`tcp` 与 URI 大小写归一。
- 明确拒绝 VMess XHTTP、Reality `spx`（非空且非 `/`）、`pqv`、`fm`、`flow=xtls-rprx-vision-udp443` 及 XHTTP 无固定内核等价字段。
- 版本与文档：`internal/version/version.go`=0.2.6，`package.json`=0.2.6，README/CHANGELOG/requirements/decisions/development-plan/session-handoff 同步，新增 `docs/releases/v0.2.6.md`。
- 不包含：部署、停止/替换运行实例、数据库 schema 迁移、Mihomo 内核升级、真实远端节点连通性或吞吐测试。

## 2. 发布前本地验证

### 2.1 静态检查

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：通过。
- `git diff --check`：通过（提交前复核）。
- 文件编码：涉及源码与文档为 UTF-8 无 BOM、LF、结尾换行。

### 2.2 Go 全量测试

命令（显式使用本机 `bin/mihomo.exe`，61,276,672 字节，2026-10-19 18:05:07）：

```powershell
$env:UNIPROXY_TEST_CORE_PATH='C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe'
go test -mod=readonly -p 1 ./... -count=1
```

结果：全部通过。

| 包 | 耗时 |
|---|---:|
| cmd/uniproxy | 0.118s |
| internal/app | 8.615s |
| internal/appdata | 0.021s |
| internal/core | 8.736s |
| internal/coreinstall | 0.539s |
| internal/importer | 0.709s |
| internal/model | 0.017s |
| internal/processguard | 0.913s |
| internal/safehttp | 0.155s |
| internal/store | 0.433s |
| internal/version | 0.017s |
| tests/integration | 3.939s |

### 2.3 Fuzz

`go test ./internal/importer -run '^$' -fuzz '^FuzzParseNeverPanics$' -fuzztime 15s -count=1`：

- 基线 551 个种子全部加载，16 workers，196,887 次执行，新增 58 个 interesting，PASS（16.211s）。

### 2.4 前端与浏览器

- 严格 TypeScript：`npx tsc --noEmit --noUnusedLocals --noUnusedParameters` 通过。
- 生产构建：`pnpm run build:web` 通过，Vite 7.3.7，4650 modules；`index-D4iDTw75.js` 525.51kB（gzip 155.08kB）、`index-BM9nabfr.css` 71.74kB（gzip 15.02kB）；仅非失败 chunk 体积提示。
- `pnpm test:ui`：39/39 工具单测通过；生产内存构建与全部 fixture 场景通过，零未捕获浏览器错误。
- `pnpm test:e2e`：43/43 通过（58.6s），使用隔离测试 API 与临时数据。

### 2.5 Windows 候选程序（本地验证构建，非发布资产）

- 路径：`bin/updates/release-0.2.6/uniproxy-windows-amd64.exe`。
- 大小：20,568,064 字节。
- SHA256：`CDA18FA4E0E58D04197B37B83AED04DBB993FE3C33F1F4B26407DE17A64FFD6A`。
- `-version`：`UniProxy 0.2.6 · commit local-2026-v · built 2026-11-02T07:51:26Z`。
- 构建时带本地嵌入前端资源；候选仅用于程序验收，不是 GitHub Release 资产。

`go test ./tests/integration -run 'TestBuilt|TestWindowsParentLifetime|TestStartup' -count=1 -v`（APP_PATH 指该候选，CORE_PATH 指本机内核）：

- `TestBuiltCLIVersionAndPublicBindGuard` 通过（version/syntax/unexpected positional/public-without-token）。
- `TestBuiltServiceWithEmbeddedUI` 通过。
- `TestBuiltWindowsAbruptExitAndRestart` 3 轮通过（子进程随主进程退出）。
- `TestBuiltWindowsStartupFailureWritesIsolatedReport` 通过。
- `TestBuiltWindowsHealthcheckFailuresDoNotAccumulateLogs` 通过。
- `TestBuiltSubscriptionCompatibility` 通过（6 种格式预览/保存/安全拒绝）。
- 包级结果：`ok ... 1.584s`。

真实 Windows 控制台脚本 `python scripts/test-windows-console.py <候选> bin/mihomo.exe`：

```json
{"case":"actual-console-WM_CLOSE","round":1,"exit":0,"running_before_close":true}
{"case":"actual-console-WM_CLOSE","round":2,"exit":0,"running_before_close":true}
{"case":"standalone-error-console-retained","exit_after_close":1,"report_saved":true}
```

### 2.6 跨平台编译

以下目标 `CGO_ENABLED=0 go build ./...` 全部通过：

- linux/amd64
- linux/arm64
- windows/arm64

### 2.7 现代协议重点复核

- `mihomo -t` 不校验 transport 名称：未知 network 也会退出 0 并走默认 TCP；因此测试断言 native map、URI/native 边界和拒绝分支，而不是仅依赖 `-t`。
- 合成 VLESS XHTTP + VMess KCP + VLESS ECH 配置在固定 Mihomo v1.19.32 下 `-t` 通过（历史 M24 验证）。
- 不把 VMess XHTTP 映射到不存在的选项；不把 HTTPUpgrade 写成不存在的 network；不把 `flow=xtls-rprx-vision-udp443` 截断成 vision 后静默接受。

## 3. CI 与正式资产

待发布后回填：

- main CI run id、Ubuntu/Windows 结果。
- tag CI run id 与四平台构建结果。
- Release 发布时间、Latest 状态、四个主程序与 SHA256SUMS 的字节数/SHA256。
- 正式 Windows 程序 `-version` 与程序验收结果。
- 历史 v0.2.0–v0.2.5 标签与资产保持不变。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。发布前的本机检查未发现 UniProxy 运行实例；不得把测试候选当作正式发布资产。

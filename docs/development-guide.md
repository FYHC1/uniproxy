# UniProxy 开发指南

更新：2026-10-19。先读[接续快照](<session-handoff.md>)和[需求基线](<requirements.md>)；本指南是工程操作说明，不授权改变运行中用户服务。

## 1. 工具链与源码定位

- Go 最低 1.24，来自 [go.mod](<../go.mod>)；纯 Go SQLite，不要求产品构建启用 CGO。
- Node.js ≥22.18，推荐24；pnpm11.28.5，使用现有锁文件。构建/测试需要 Node，发布后只需 UniProxy 和独立内核。
- 当前使用 React/TypeScript/Vite，Go 标准库 HTTP，modernc SQLite，yaml.v3；不是 Node API 服务，不要重建旧 Fastify/sing-box 草案。
- Mihomo 固定1.19.32；安装器验证 GitHub 官方资产摘要与内置固定 SHA256，运行管理器拒绝未验证版本。
- 命令从项目根目录执行。最低版本与本机曾执行的版本不是一回事；当前快照见[接续记录](<session-handoff.md>)。

| 模块 | 主要源码 | 责任 |
|---|---|---|
| 服务入口 | [main.go](<../cmd/uniproxy/main.go>) | 参数/环境、IP/Token、数据路径、Web/API、启动与退出 |
| 内置内核安装 | [下载器](<../internal/coreinstall/install.go>)、[安装生命周期](<../internal/core/installation.go>)、[安装 API](<../internal/app/core_install.go>) | 固定版本、来源/摘要/归档安全、原子无覆盖安装与后台状态 |
| 数据模型 | [types.go](<../internal/model/types.go>)、[relay.go](<../internal/model/relay.go>) | 规范类型、动态池成员、稳定依赖图/阻断检查 |
| 解析 | [parser.go](<../internal/importer/parser.go>)、[dialer.go](<../internal/importer/dialer.go>) | 有界离线输入、协议标准化、同文件依赖/递归指纹 |
| 状态协调 | [app.go](<../internal/app/app.go>)、[http.go](<../internal/app/http.go>) | 序列化变更、公开投影、鉴权、接口与备份 |
| 导入/订阅 | [imports.go](<../internal/app/imports.go>)、[relay.go](<../internal/app/relay.go>) | 多来源去重、分组归属、删除保护、图恢复/导出 |
| 检测任务 | [jobs.go](<../internal/app/jobs.go>) | 异步、限并发、取消、版本一致性、不写过期结果 |
| 持久化 | [store.go](<../internal/store/store.go>) | SQLite WAL、AES-GCM 配置快照、活动记录、规范空数组 |
| 下载安全 | [fetch.go](<../internal/safehttp/fetch.go>)、[client.go](<../internal/safehttp/client.go>)、[doh.go](<../internal/safehttp/doh.go>)、[errors.go](<../internal/safehttp/errors.go>) | URL/公网DNS固定拨号、严格Fake-IP公共DoH复核、TLS/重定向/大小保护、无秘密分类诊断 |
| 内核 | [compiler.go](<../internal/core/compiler.go>)、[manager.go](<../internal/core/manager.go>) | 稳定标签、原生策略、失效即拒绝、进程/验证/重载/回滚 |
| 控制与探测 | [controller.go](<../internal/core/controller.go>)、[checks.go](<../internal/core/checks.go>) | 私有认证API、入口验证、实际延迟与下载 |
| 嵌入前端 | [webui.go](<../internal/webui/webui.go>)、[Vite配置](<../vite.config.ts>) | 静态构建嵌入Go、CSP、缺少构建时503 |
| 前端状态/API | [state.tsx](<../src/web/state.tsx>)、[api.ts](<../src/web/api.ts>)、[共享类型](<../src/shared/types.ts>) | 会话Token、轮询、错误/dirty、旧响应保护 |
| 导入/筛选UI | [ImportDialog.tsx](<../src/web/ImportDialog.tsx>)、[importUtils.ts](<../src/web/importUtils.ts>)、[nodeFilters.ts](<../src/web/nodeFilters.ts>) | 文件读取版本、拖放安全、包含/排除/未分组语义 |
| 代理池工作流 | [pools.go](<../internal/app/pools.go>)、[ports.go](<../internal/app/ports.go>) | 池+监听单次事务、自动/手动空闲端口、原子池内移除 |
| 代理池UI | [PoolDialog.tsx](<../src/web/PoolDialog.tsx>)、[PoolCard.tsx](<../src/web/PoolCard.tsx>)、[poolSelection.ts](<../src/web/poolSelection.ts>) | 分组范围/勾选映射、全选、旧排除兼容、紧凑监听/真实检测卡片和6条分页 |
| 分组节点管理 | [GroupsPage.tsx](<../src/web/GroupsPage.tsx>)、[GroupNodesDialog.tsx](<../src/web/GroupNodesDialog.tsx>) | 点击组名/数量管理组内20条分页，添加关系/移出关系与全局启禁分开，失效选择清理 |

## 2. 首次安装与开发

~~~sh
pnpm install --frozen-lockfile
~~~

启动管理服务后在 WebUI 缺失提示或“运行与设置”中在线安装，无独立安装器。内核默认安装到当前平台数据目录的 core 子目录；自动检测包含显式路径、数据目录、旧 bin、程序同目录与 PATH。已有文件及运行中的内核不会覆盖。下载由服务主机直连官方发布服务，进度与错误可轮询；安装成功后需显式应用，详见[内核指南](<../internal/core/README.md>)。

仅在默认9090/5173空闲且数据目录是开发用时：

~~~sh
pnpm dev
~~~

[脚本](<../package.json>)启动 Go API 和 Vite；Go代码变化需要重启Go，Vite只热更新其开发前端。Vite固定将 /api 转到127.0.0.1:9090；单独把后端改端口并不会自动修改它。运行中的生产程序与开发Vite不是同一个界面实例。

订阅模式的“预览”会先下载远端URL，只是不写数据库；解析器及文本/文件预览才是离线。启动真实应用会立即尝试应用已存配置，并启动订阅/健康调度；所以不能为了文档检查或查看界面随意用用户数据启动新实例。

### 数据目录规则

**默认数据目录按平台选择，显式相对 -data 路径仍相对于进程工作目录。** 旧版 data 目录自动沿用规则见下文。源代码见[服务入口](<../cmd/uniproxy/main.go>)。显式参数/环境优先；原生服务不自动载入 [.env.example](<../.env.example>)。

开发和测试不得直接复用用户数据库或在同一数据目录启动两个服务。使用绝对 -data/-core 最清楚；手工调试另选端口，勿抢占9090或测试19090。

### 数据目录（平台标准位置）

数据目录不再默认取相对工作目录的 `./data`，由[解析器](<../internal/appdata/appdata.go>)按平台决定：

| 平台 | 默认 | 回退 |
|---|---|---|
| Windows | `%ProgramData%\UniProxy` | `%LOCALAPPDATA%\UniProxy` |
| Linux | `/var/lib/uniproxy` | `$XDG_DATA_HOME/uniproxy` 或 `~/.local/share/uniproxy` |
| macOS | `~/Library/Application Support/UniProxy` | 同左 |

优先级：显式 `-data`/`UNIPROXY_DATA_DIR` → 已存在的标准目录 → 已存在的旧版 `data`（工作目录优先，其次可执行文件目录，日志会提示迁移）→ 新建标准目录 → 不可写时回退用户目录。旧目录被采用时**不会**在新位置建空库，避免升级后看起来数据丢失。

迁移：停服 → 复制 `uniproxy.db`、`-wal`、`-shm`、`master.key`、`core/` → `-data` 指向新目录启动确认 → 删除旧目录。主密钥必须同行。

Docker 通过 `UNIPROXY_DATA_DIR=/data` 指向唯一挂载卷。

### 版本与发布

[internal/version](<../internal/version/version.go>) 是唯一版本来源；发布构建用 `-ldflags -X` 注入 `Version/Commit/Date`，当前源码未注入构建报告 0.2.1 且省略提交/时间；未发布暂存应显式注入开发标识，不能只凭版本字符串认定发布身份。`-version` 与 `/api/health` 报告同一值。

[发布工作流](<../.github/workflows/release.yml>)在推送 `v*.*.*` 标签或手动触发时，构建 linux/windows × amd64/arm64 四个 UniProxy 二进制（在线安装内置，不再构建独立安装器），生成 `SHA256SUMS.txt` 并创建 Release。**只有真实推送的标签才算发布**，不要手写版本号冒充。

## 3. 构建

### 没有活动同名程序时

~~~sh
pnpm build
~~~

顺序：TypeScript → Vite → [平台构建脚本](<../scripts/build-binaries.mjs>)。Vite输出 internal/webui/assets/dist，随后 Go编译将其嵌入。只运行go build可能嵌入旧前端；只刷新浏览器不会更新旧exe。

### 有运行中服务时，使用暂存构建

~~~powershell
pnpm run build:web
if ($LASTEXITCODE -ne 0) { throw 'Web build failed' }
go build -trimpath -o bin/updates/uniproxy.exe ./cmd/uniproxy
if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
~~~

这不是热更新。不能运行普通 pnpm build 去覆盖被用户进程占用的exe；也不能在还运行依赖该exe的测试时重编译它。Windows会报文件被占用。暂存路径只是本地约定，没有自动升级器。

### Linux构建

在Linux原生构建或在独立shell中设置 GOOS=linux、GOARCH=amd64/arm64、CGO_ENABLED=0，并把 -o 指向单独目录。不要把这些环境变量留在后续Windows测试shell里。已有[构建脚本](<../scripts/build.sh>)及 [Windows脚本](<../scripts/build.ps1>)可参考，但它们也会写常规bin路径。

## 4. 分层测试

### 快速逻辑/静态

~~~sh
go test ./...
go vet ./...
pnpm typecheck
node node_modules/typescript/bin/tsc --noEmit --noUnusedLocals --noUnusedParameters
node --test src/web/tests/utils.test.mjs
~~~

没有以下显式环境变量时，真实内核/构建程序/私有文件测试会 skip；不能把skip说成已经验证。

### 真实内核和最终可执行产物

~~~powershell
$env:UNIPROXY_TEST_CORE_PATH = (Resolve-Path bin/mihomo.exe).Path
$env:UNIPROXY_TEST_APP_PATH = (Resolve-Path bin/updates/uniproxy.exe).Path
# 请先完成上述暂存构建；执行期间不要覆盖该exe。
go test -p 1 -count=1 ./...
if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
go vet ./...
~~~

如果使用默认安装目录，调整内核路径。-p 1降低多包并发启动内核造成的资源/端口波动，不是规避失败；仍须调查任何非零退出。

- [真实内核测试](<../internal/core/integration_test.go>)：HTTP/SOCKS/mixed、UDP、调度、认证、下载、重载TCP、端口交换。
- [中转测试](<../internal/core/relay_test.go>)：真实两跳代理，证明中转与出口都经过；禁用中转不得直接拨出口。
- [API全链路](<../tests/integration/api_test.go>)、[代理池工作流](<../tests/integration/pool_workflow_test.go>)、[可执行进程测试](<../tests/integration/process_test.go>)：临时数据与受控本机服务，不动用户实例。池工作流验证一次创建、认证转发、真实延迟、移除最后节点拒绝和同端口重新纳入。
- [API图回归](<../internal/app/relay_test.go>)：解析/ID/来源/导出/备份/删除恢复/隐私。

### 浏览器

~~~sh
pnpm run build:web
pnpm test:e2e
pnpm test:ui
~~~

- E2E用[隔离测试服务器](<../cmd/testserver/main.go>)19090及临时数据库；不会使用真实节点。Windows默认Edge。[配置](<../playwright.config.ts>)不复用已存在服务器。
- UI fixture脚本内存构建、拦截测试域名，无产品假数据；它不能替代真实Go/内核验证。
- 两种浏览器验证不要复制启动，不共享活动测试端口。当前源码为34个E2E、36个工具测试；本次是否实际通过以[接续记录](<session-handoff.md>)和带日期验收为准。

### 私有文件离线验证

仅在用户授权后设置 UNIPROXY_TEST_IMPORT_FILE 为其绝对文件路径，执行：

~~~sh
go test ./internal/app ./tests/integration -run TestPrivateClash -count=1 -v
~~~

[应用验证](<../internal/app/private_import_test.go>)和[配置验证](<../tests/integration/local_import_test.go>)只在临时状态测试，不复制源文件或输出凭据。若同时设置内核路径，可做 -t 校验，不会启动该文件的远端节点。不要在命令或测试夹具里固化用户账号；原文件的定位只存于[本机接续快照](<session-handoff.md>)。

### 可选的订阅主机传输诊断

[主机测试](<../tests/integration/subscription_host_test.go>)默认跳过。只有用户要求排查其公共订阅主机时，才设置 UNIPROXY_TEST_SUBSCRIPTION_HOST 为裸域名（不能是完整URL或令牌）；测试用产品默认安全客户端对HTTPS根路径发HEAD，不跟随重定向、不读取响应正文，仅验证安全解析/拨号/TLS和HTTP响应。它不能证明私有订阅内容能解析，更不能证明节点连通。全量日常回归时保持此变量为空，避免隐式公网请求。

### Fuzz与race

[解析器说明](<../internal/importer/README.md>)列出FuzzParseNeverPanics、FuzzSSHKeysNeverPanic命令。race需要可用C工具链；当前宿主CGO关闭且无gcc/clang，不曾完成race。勿修改系统安全设置来强行运行被阻止的测试exe。

## 5. 必须理解的不变量

### 状态和并发

- 变更走 App.mutate：克隆 → 业务校验 → 内核可用时校验 → revision递增/SQLite提交 → 自动apply → 公共结果。
- 校验错误不提交；apply错误可能已保存期望版本但保留旧运行版，必须看dirty和lastError。
- 手动Stop在当前进程内暂停自动apply；显式Apply或重启应用恢复尝试。正常热更新不等于UDP/粘性缓存无损。
- 全局配置变更互斥；健康结果不推进配置revision。检查需匹配运行revision；中途变更不得写旧健康。
- 订阅网络下载在锁外；提交要核对源URL/UpdatedAt，恢复备份时更新源代数，旧请求不得覆盖新源。
- 活动/任务是管理信息：配置快照持久化，任务在内存，当前手动停止标志也在内存。

### 中转字段

| 字段 | 阶段/处理 |
|---|---|
| ParsedNode.DialerProxy | 文档内的原始引用名，可用于导入预览；不是可直接透传的内核配置 |
| ParsedNode.DialerFingerprint | 解析时的上游递归指纹，json忽略；用于两遍导入映射 |
| Node.DialerID | 存储稳定边；上游删除时保留，确保拒绝而不是失去依赖 |
| Node.DialerIdentity | 最后验证的上游身份指纹，用于删除后的备份/恢复/重导入一致性；不能被公共注解函数清空 |
| Node.DialerName / DialerError | 公共说明；健康未知不是依赖错误 |

原始 Config 不含 dialer-proxy；仅[编译器](<../internal/core/compiler.go>)在校验图后注入 n-ID。直接节点指纹必须兼容旧版本；链指纹包含上游配置身份，不包含显示名。导出名字要避开规范化内置策略以及所有缺失引用碰撞。恢复备份重算指纹，不相信随意填入的节点Fingerprint。

### 安全

- 公开节点投影不泄漏Config/Fingerprint/DialerIdentity/来源私密字段，更新接口的响应同样脱敏。
- API配置导出/备份是显式明文秘密；SQLite仅state payload加密，活动表不加密，因此日志不得带秘密。
- 内核候选与启动配置从stdin传入，重载/回滚内存传入认证回环API；不要重新引入runtime.yaml。
- 私有控制器和逐节点探针不对外暴露；环境CLASH_*/MIHOMO_*覆盖被清除。
- 订阅安全拨号逐地址校验、禁止默认内网、重定向重新校验且删除Referer，不能直接换成无约束http.Get。
- 维持回环UDP、不直连回退、来源删除保护、严格不支持参数报错；详见[决策记录](<decisions.md>)。

## 6. 部署与回滚（先获准停机）

1. 确认准确exe路径、PID/子进程、管理端口、进程工作目录/绝对数据目录和内核路径，不使用过期PID。
2. 准备匹配的备份。优先受控导出或停机后一致拷贝整个数据目录；SQLite WAL运行期间不能只复制主db文件。备份包括主密钥，且仅私有保存。
3. 正常退出应用并确认它的内核退出。不要盲杀所有mihomo；异常退出可能留孤儿进程，先核对归属。
4. 校验暂存文件哈希，保留旧exe到明确备份路径，再替换；任何覆盖/回滚前先确认目标。
5. 用原数据目录启动；前端嵌入必须重启+刷新。检查bootstrap/runtime是否运行、版本一致、端口绑定正常；不要把仅health200当完整验收。
6. 升级后更新[接续记录](<session-handoff.md>)的运行构建/哈希/时间。发布构建报告注入的版本+提交，未注入的本地构建显示开发版；仍以哈希核对具体文件。
7. 涉及新数据字段时，**旧exe可能忽略并在写回时丢失字段**（尤其DialerID/Identity）。回滚必须恢复匹配旧版的备份，而不是直接让旧程序写新版数据。

## 7. 常见陷阱

- “128项解析失败”来自旧扫描上限和不支持中转，不是GFW/网络结论。
- 原生延迟API把四舍五入0ms视为失败；回环fixture应实际延迟几毫秒，不伪造延迟结果。
- 测试的空闲端口要同时可绑定TCP/UDP；只探测TCP不足，旧fixture已修复。
- Windows曾有临时test.exe执行拒绝或内核首次检查超时；查真实证据、确保任务不重复，不能禁用安全软件或假报通过。
- 当前项目已有 Git 仓库；仍应检查 git status 再修改。不自动提交、推送或发布，新请求需明确授权。
- 在有活动用户实例时，文档变更不需要启动服务或全量构建。只跑与改动相关的校验。

## 8. 每次收尾回写

更新[计划](<development-plan.md>)、[接续快照](<session-handoff.md>)、[变更记录](<../CHANGELOG.md>)；新增决策写[decisions.md](<decisions.md>)，API/模型变化同步契约/示例/前端类型。记录实际执行命令、通过/失败/跳过、剩余任务、运行构建是否更新。后台任务须收集或停止；不得把旧测试记录当新结果。

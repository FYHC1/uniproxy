# 首次入库与全面复查修复验收

**日期：2026-10-20（Asia/Shanghai）。范围：源码仓库初始化、上传 GitHub、全量代码复查与缺陷修复。本轮未部署，未停止用户服务，未读取用户数据库/主密钥/原始订阅。**

## 1. 版本控制与上传

| 项目 | 结果 |
|---|---|
| 仓库 | 私有 GitHub 仓库 `FYHC1/uniproxy` |
| 初始提交 | `7829f25 chore: import UniProxy source`（157 个文件） |
| 未入库 | `node_modules`、`bin`、`data`、`test-results`、`playwright-report`、`.local`、`.test-core` |
| 入库前扫描 | 无 GitHub token、私钥、真实订阅链接、真实节点凭据、个人绝对路径 |

扫描方式：对全部候选文本文件匹配 UUID、代理 URI scheme、订阅路径、密码/令牌赋值、PEM 私钥、GitHub token 前缀、Windows 绝对路径；命中项逐条人工确认均为测试夹具（`example.com`、`192.0.2.x`、`2001:db8::` 等文档地址或显式 canary 字符串）。`bin/mihomo.exe`、`bin/data`、`data/` 与 `.local/state/gh` 未进入版本库。

## 2. 复查方法

1. 逐文件阅读 `internal/app`、`internal/core`、`internal/importer`、`internal/model`、`internal/safehttp`、`internal/store`、`cmd/*` 全部源码及测试。
2. 阅读前端全部页面、对话框、纯函数、样式与测试脚本，以及接口契约、API 文档、架构、决策记录、CI、Dockerfile、compose。
3. 运行既有全部测试与静态检查。
4. 用**暂存二进制 + 临时数据目录**（非用户数据）对真实 HTTP API 做 60 余条边界探测，包含非法字段、越权 Host/Origin、端口冲突、公网监听、长名称、备份恢复、订阅元数据等。

### 已执行并通过的检查

| 检查 | 结果 |
|---|---|
| `go test -p 1 -count=1 ./...` | 通过（含 `tests/integration` 默认跳过真实内核项） |
| `go vet ./...` / `gofmt -l` | 通过 / 无差异 |
| `tsc --noEmit --noUnusedLocals --noUnusedParameters` | 通过 |
| `node --test src/web/tests/utils.test.mjs` | **32/32**（原 31，新增 1） |
| `node src/web/tests/verify.mjs`（fixture UI） | 通过，零未捕获错误 |
| Playwright 真实 API 浏览器套件 | **32/32**，55.8 秒 |
| 真实本机 Mihomo 链路（`UNIPROXY_TEST_CORE_PATH`） | HTTP/mixed 转发、URLTest、限速测速、禁用即拒、池原子创建、空池拒绝、同端口重新纳入、13 协议配置校验全部通过 |
| 构建后的进程/CLI/manifest 测试（`UNIPROXY_TEST_APP_PATH`） | 通过 |
| Windows 暂存构建 + Linux amd64/arm64 交叉构建 | 通过 |

### 复查确认无问题的部分（抽样）

- Host 伪造、`Sec-Fetch-Site: cross-site`、跨站 Origin 均 403；非 JSON 写请求 415；未知字段 400。
- 管理端口占用、非回环无认证、非回环 UDP、重复监听端口、通配地址冲突均 400。
- 备份恢复整库替换、删除代理池后监听保留且禁用、监听密码留空保留原密码、改名用户缺密码被拒。
- 订阅预览对回环地址返回明确内网拦截说明且不写入状态。
- 静态资源路径穿越尝试（`/../`、`%2e%2e`）未返回仓库文件。

## 3. 发现并修复的缺陷（共 9 项）

### 3.1 批量分配独立端口对接近名称上限的节点整批失败

`POST /api/listeners/batch` 用 `"<节点名> · 独享"` 生成监听名，而监听名上限 160 字符、节点名上限同为 160。名称超过 155 字符的节点会让**整批**返回 400，且错误信息是“监听名称、IP 或端口无效（端口范围 1024–65535）”，把名称问题指向了端口。

修复：新增 `derivedName`，为后缀预留空间，超出时按字符边界截断并加“…”；错误信息拆分到真实字段。

### 3.2 预览与实际保存的校验口径不一致

导入器允许节点标签到 1024 字符，状态校验只允许 160。结果是预览显示 1 个可导入节点、点“确认导入”才失败，且提示“节点名称、服务器或端口无效”。

修复：导入器对超过 160 字符的标签按字符边界截断并加“…”，使预览展示的就是将保存的值；超过 1024 字符或含控制字符仍判为无效输入（保留原有“不静默接受畸形输入”的边界）。空白标签回退为协议+地址，避免预览通过后保存失败。

### 3.3 订阅元数据校验时机过晚

长订阅名/分组名/非法刷新间隔此前只在下一次 `mutate` 的状态校验里暴露，即下载完成后才失败。

修复：在发起任何网络请求之前校验，且只在订阅模式生效（文本/文件导入忽略这些字段）。

### 3.4 监听页与节点页可能把待同步地址显示为“监听中”

池卡片已用 `appliedRevision` 比较避免误报，但监听页与节点页仍只看 `runtime.running && listening`。当配置版本与运行版本不一致时，旧地址会被标为“监听中”。

修复：抽出共享 `listenerState`/`listenerIsLive`，池卡片、监听页、节点页统一使用；待同步显示“状态待同步”，监听页顶部“实际监听中”计数也只统计已确认版本。

### 3.5 活动记录按字节截断产生非法 UTF-8

`Activity` 用 `message[:2000]` 截断，多字节字符被切开后 UI 显示替换字符。

修复：按字符边界截断，并补回归测试。

### 3.6 浏览器测试脚本写死用户级安装路径

`verify.mjs` 回退到 `channel:'chrome'`，Playwright 只查用户级路径；本机 Chrome/Edge 装在 `Program Files` 时会直接启动失败。

修复：优先 bundled，其次 `PLAYWRIGHT_CHROME_PATH`，再探测 Windows `Program Files` 与常见 Linux 路径。

### 3.7 测试端口辅助函数在 Windows 上间歇失败

`poolTestPortRange` 与 `freePort` 先向系统申请临时端口，再验证相邻端口或 UDP 同端口是否可用。Windows 的动态端口范围与 Hyper-V/WinNAT 保留段重叠，随机起点经常没有可用邻居，导致 `go test ./...` 偶发失败并报“could not obtain isolated TCP/UDP test ports”。这是测试基础设施缺陷，不是产品逻辑失败。

修复：改为在固定低段（21000–48000）顺序扫描可同时绑定 TCP 与 UDP 的端口，去掉随机性；连续两次全量 `go test -p 1 -count=1 ./...` 均通过。

### 3.8 文档泄露个人绝对路径

接续快照中包含本机工作区绝对路径与一个私有文件绝对路径，已随首次提交上传。

修复：改为不含个人路径的描述；复查 `docs`、README、CHANGELOG、AGENTS 已无 Windows 绝对路径。

### 3.9 测试基础设施并发不稳定（核心版本探测超时）

`go test ./...` 并行运行各包时，核心包的 mock 进程首次 exec 可能因杀毒扫描/磁盘争用超过 5 秒，被 `inspectBinary` 判为“版本探测超时”，导致 `TestLifecycleValidateApplyReloadStop` 间歇失败。

修复：版本探测预算从 5 秒提高到 20 秒（仍为有界预算），并把 app 与 core 的测试端口段设为互不重叠、随机起点，避免并行包测试抢同一端口。修复后连续多次并行全量测试通过。

## 4. 新增回归测试

| 位置 | 覆盖 |
|---|---|
| `internal/importer/document_test.go` | 超长标签按字符边界截断保留节点、短标签逐字节保留、1025 字符仍拒绝 |
| `internal/app/validate_test.go` | 派生名不越界、名称上限节点可批量建监听、错误信息点名字段、订阅元数据下载前拒绝、截断不破坏多字节字符 |
| `internal/store/store_test.go` | 活动记录截断保持合法 UTF-8 |
| `src/web/tests/utils.test.mjs` | 共享监听状态在各页面一致、待同步/待应用/禁用/监听错误均不显示“监听中” |
| `internal/app/pools_test.go`、`internal/core/manager_test.go` | 端口辅助函数改用固定低段顺序扫描，消除Windows保留端口段导致的间歇失败 |

## 5. 构建产物（暂存，未部署）

| 产物 | 字节 | 时间 | SHA256 |
|---|---|---|---|
| `bin/updates/uniproxy.exe` | 19785216 | 2026-10-20 01:42:34 | `510CAE7FE157938A8F5060AE5B66363E73A73EFF9CE90F3F63CABF20CB147317` |
| `bin/updates/linux-amd64/uniproxy` | 19335940 | 01:42:37 | `0C901F935D2346B201931336EF2EE7EF9771436D0F9BAA91C791C45F78E658F7` |
| `bin/updates/linux-arm64/uniproxy` | 18211567 | 01:42:39 | `229771A0D1AB43CF2882BE6EAA9F502B9C28A8250352394E6FF9CA56C7826BFA` |

当前运行中的 `bin/uniproxy.exe` 仍是 `22596106…`（19779584 字节，2026-10-19 23:16:57），**本轮未替换、未停止服务**。用户进程 PID 5248（父 25700，2026-10-19 23:25:57 启动），子内核 PID 32296。

## 6. 限制

- 未执行 race detector（当前环境无 C 工具链）；未做 Linux/Docker 实跑与吞吐压测。
- 未访问任何真实订阅路径或真实节点；真实内核验证使用本机回环夹具上游。
- 前端生产包仍为单 chunk 509.25 kB（gzip 150.33 kB），触发 Vite 500 kB 提示；未通过调高阈值掩盖，代码分包仍是后续优化项。
- 本轮复查是静态审查加行为探测，不构成全面安全担保。

测试端口辅助函数修复后，连续两次全量 Go 测试通过；该缺陷仅影响测试可靠性，不影响产品运行行为。

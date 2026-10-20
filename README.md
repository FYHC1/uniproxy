# UniProxy

面向个人浏览器、工具和自动化程序的自托管聚合代理平台。Go 管理服务 + React 控制台 + SQLite + 独立 Mihomo 代理内核。

## 开发文档与接续

[文档总目录](<docs/README.md>) · [开发计划](<docs/development-plan.md>) · [开发指南](<docs/development-guide.md>) · [会话接续快照](<docs/session-handoff.md>) · [需求基线](<docs/requirements.md>) · [决策记录](<docs/decisions.md>) · [变更记录](<CHANGELOG.md>)

新会话/上下文压缩后，先读根目录 [AGENTS.md](<AGENTS.md>) 与接续快照。源码已完成和运行程序已升级是不同状态；有正在使用的实例时，不要直接全量构建覆盖原exe或改用另一数据目录。

## 功能

- 导入 VLESS / VMess / Trojan / Shadowsocks / Hysteria / Hysteria2 / TUIC / SSH / SOCKS5 / HTTP(S) 节点。
- 粘贴节点链接、Base64 订阅、批量订阅 URL、本地 Clash YAML/JSON 和 sing-box JSON。自定义多个分组，预览识别结果；不支持的参数明确报错，不悄悄降级。
- 节点多分组、搜索筛选（包含任一/全部、排除分组、仅未分组）、批量启禁/删除/加减分组、连通性检测、限流下载测速、批量独享端口。点击分组可直接分页管理组内节点、添加已有节点、移出本组和全局启禁/检测。
- 订阅下载支持严格Fake-IP DNS兼容与分类诊断：只对全Fake-IP域名做公共加密DNS复核，保留公网地址/TLS/重定向安全校验，不放行内网。
- 本地导入支持点击选择和拖入单个文件；相同5MiB限制，拒绝文件夹/多文件，拖入后需预览确认。
- 代理池 URL Test、轮询、一致性哈希、原生粘性会话；包含分组限定候选、任一/全部动态成员、全选搜索结果及池内节点移除/重新纳入。
- 创建代理池时同时分配共享监听（自动空闲端口或手填）；紧凑卡片显示监听、真实统计和最近延迟，节点6条分页、详细信息按需展开，单池宽屏双栏；可单个/整池测延迟。
- mixed（HTTP + SOCKS5）、HTTP-only、SOCKS5-only 入口；同一节点可独享端口并参与多个池。
- 本机 SOCKS5 UDP；仅选择具有 UDP 能力的节点。
- 订阅定时/手动刷新、多来源去重，完整成功刷新清理消失节点；失败/部分解析不删旧节点，异常大幅删减保护。
- 自动校验并应用配置，端口冲突检测，运行状态与保存状态分离，失败保留待应用版本；正常修改不主动切断已有 TCP。
- 管理 Token、代理认证、订阅 SSRF 防护、敏感列表脱敏、加密 SQLite、备份/恢复。

> 第一版的安全边界：Mihomo v1.19.32 原生 SOCKS UDP 不执行用户名/密码认证，也不携带 InUser。因此只允许回环 UDP 监听；同机其他进程仍可发送UDP。不支持“对外 UDP + 用户名认证”或“UDP 按用户名哈希”。TCP 对外监听有认证支持。公网管理必须配置 Token 并使用 HTTPS。

## 下载发布版

当前正式版本：[UniProxy v0.2.1](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.1>)。提供Windows/Linux × amd64/arm64四个主程序和SHA256SUMS；启动后在WebUI安装Mihomo，无需独立安装器。私有仓库下载需要已获访问权限的GitHub账号。

## 快速开始（源码）

需要 Go 1.24+、Node.js 22.18+（建议24）、pnpm 11.28.5。运行时仅需编译后的 UniProxy 和 Mihomo，不需要 Node/Python。

在仓库根目录执行：

~~~sh
pnpm install --frozen-lockfile
pnpm build
~~~

不需要另行下载或运行独立安装器。先启动 UniProxy，再从 WebUI 安装所需的 Mihomo 内核；没有内核也可以打开管理面板、保存配置。

Windows：

~~~powershell
.\bin\uniproxy.exe
~~~

Linux：

~~~sh
./bin/uniproxy
~~~

打开 **http://127.0.0.1:9090**。首次没有节点；如果未检测到 Mihomo，管理面板会弹出“安装 Mihomo 内核”提示。点击在线安装，等待完成并通过版本检测，再点击“重新应用配置”启动代理。导入后新建代理池时可一并配置共享监听，或为节点创建独享入口。服务不会更改系统/浏览器代理或防火墙。

### Mihomo 内核在线安装

- **自动检测**：显式 `-core` / `UNIPROXY_CORE_PATH` 优先；未指定时依次检查数据目录的 `core/mihomo[.exe]`、工作目录的 `bin/mihomo[.exe]`、UniProxy 可执行文件同目录、系统 PATH。可用内核必须是 v1.19.32，已有内核无需重新安装。
- **随时进入**：“运行与设置” → “运行概况”或“检测与端口”都保留在线安装、安装状态和“重新检测内核”入口；关闭首次提示不会妨碍继续管理配置。
- **真实进度**：下载、SHA256 校验、解压、安装和版本检测在服务端后台执行；面板每 5 秒同步。关闭窗口或切换页面不取消任务；失败显示原因，未写入目标时可以重试。
- **目标位置**：默认使用当前数据目录下的 `core` 子目录，随数据卷持久化。显式指定缺失的内核文件路径时安装到该路径；仅指定 PATH 命令名则需要先改为完整文件路径或取消覆盖。
- **安全与版本**：仅 Windows/Linux × amd64/arm64 在线安装固定 v1.19.32；从官方发布下载，经 HTTPS、重定向域名、公网 DNS/TLS 和内置 SHA256 双重摘要校验后才解压。不会执行安装脚本、自动升级或覆盖已有文件，不会替换运行中的内核。
- **安装不等于代理运行**：安装完成后不自动应用配置，期间及之后的配置变更仅保存；必须显式“重新应用配置”。此暂停与安装任务记录在当前进程内，重启管理服务仍按原有启动语义尝试应用已保存配置。

下载由 **UniProxy 所在主机**直连官方发布服务，不使用浏览器或系统 HTTP 代理。如果服务器无法访问，面板会报告网络原因；可以手动放置匹配版本的 Mihomo 后点击“重新检测内核”。已有损坏/不兼容文件需先在主机上确认路径并手动处理，界面不会静默覆盖。安装目录须可写且支持硬链接（常见 NTFS、ext4 等），不支持时安全失败而不是降级覆盖。

### 开发

~~~sh
pnpm dev
~~~

Go API 默认9090，Vite 默认5173，开发界面访问 http://127.0.0.1:5173。修改 Go 后需重启 Go 子进程；Vite 负责前端热更新。生产界面必须 pnpm build 后重启 UniProxy，生产二进制嵌入前端产物。

## 数据目录

配置数据库、主密钥与内核工作文件默认放在**平台标准位置**，不再依赖启动时的工作目录：

| 平台 | 默认目录 |
|---|---|
| Windows | `%ProgramData%\UniProxy`（通常是 `C:\ProgramData\UniProxy`） |
| Linux | `/var/lib/uniproxy` |
| macOS | `~/Library/Application Support/UniProxy` |

启动时会按顺序决定位置，并在日志中打印实际目录和选择原因：

1. `-data` 参数或 `UNIPROXY_DATA_DIR`（显式指定优先，目录不存在会创建）；
2. 已存在的平台标准目录；
3. 已存在的旧版目录（进程工作目录下的 `data`，其次可执行文件同目录的 `data`）——升级不会因为换了默认路径而指向空库；
4. 新建平台标准目录；若该目录不可写（例如普通用户没有 `/var/lib` 或 `ProgramData` 写权限），回退到用户目录 `%LOCALAPPDATA%\UniProxy`、`$XDG_DATA_HOME/uniproxy` 或 `~/.local/share/uniproxy`，并在日志中说明。

若出现“检测到旧版…数据目录并继续使用”的提示，说明仍在用旧位置。迁移步骤：停止服务 → 把整个目录（`uniproxy.db`、`uniproxy.db-wal`、`uniproxy.db-shm`、`master.key`、`core/`）复制到新目录 → 用 `-data` 指向新目录启动一次确认数据在 → 再删除旧目录。**主密钥必须与数据库一起迁移**，换库不换钥会无法解密。

Docker 使用 `/data` 卷，这是容器内唯一挂载点，通过 `UNIPROXY_DATA_DIR=/data` 显式设置。

## 发布与版本

版本号只有一处来源：[internal/version](<internal/version/version.go>)。发布构建用 `-ldflags` 注入版本、提交与构建时间。当前源码默认版本为 0.2.1，未注入时不显示提交和时间；未发布构建应显式注入开发标识，不要仅凭版本号认定它是发布资产：

~~~sh
go build -trimpath \
  -ldflags "-X github.com/uniproxy/uniproxy/internal/version.Version=0.2.1 \
            -X github.com/uniproxy/uniproxy/internal/version.Commit=$(git rev-parse --short=12 HEAD) \
            -X github.com/uniproxy/uniproxy/internal/version.Date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/uniproxy ./cmd/uniproxy
~~~

`uniproxy -version` 和 `/api/health` 都报告同一版本，可用来确认正在运行的进程对应哪个发布。

发布由 [release workflow](<.github/workflows/release.yml>) 自动完成：推送 `v0.2.1` 形式的标签，或在 Actions 里手动触发并填写版本号。工作流会构建 linux/windows 的 amd64+arm64 四个 UniProxy 二进制（内置在线安装功能，不再附带独立安装器），生成 `SHA256SUMS.txt`，创建 Release 并附加产物。标签必须是真实推送的版本，未发布的构建不应冒充已发布版本。

## 配置

程序从进程环境读取设置，**不会自动载入 .env**；Docker Compose 则支持 .env 变量插值。

| 环境变量 | 默认/用途 |
|---|---|
| UNIPROXY_HOST | 127.0.0.1，管理服务 IP |
| UNIPROXY_PORT | 9090，管理端口 |
| UNIPROXY_DATA_DIR | 平台标准数据目录（见下），可显式覆盖 |
| UNIPROXY_CORE_PATH | 自动查找 data/core、bin 或可执行文件同目录的 Mihomo |
| UNIPROXY_ADMIN_TOKEN | 本机可选；非回环监听必须至少24字符 |
| UNIPROXY_ALLOW_PRIVATE_SUBSCRIPTIONS | false；显式 true 才允许受信任的内网订阅 |
| UNIPROXY_ALLOWED_ORIGINS | 额外允许的精确浏览器 Origin，以逗号分隔，供反向代理使用 |

命令参数：-host、-port、-data、-core、-version、-healthcheck。代理监听地址/端口、凭据、UDP 在控制台管理，与管理服务的端口分开。

Linux对外部署示例（真实值请自行设置，不要使用示例密码）：

~~~sh
export UNIPROXY_HOST=0.0.0.0
export UNIPROXY_ADMIN_TOKEN='replace-with-at-least-24-random-characters'
./bin/uniproxy
~~~

入口为 0.0.0.0 等非回环地址时必须设置代理用户名/密码，HTTP 和 SOCKS5 认证本身不是加密传输，请通过可信网络或额外加密隧道接入。

## Docker

~~~sh
# 在 .env 中设置 UNIPROXY_ADMIN_TOKEN=足够长的随机值
# 或在当前 shell 导出该环境变量
docker compose up -d --build
~~~

默认将管理9090和TCP代理20000–20049映射到宿主回环地址。容器内新建入口应绑定0.0.0.0并配置代理认证。端口范围要与 compose 映射保持一致；在Web里添加端口不会修改Docker映射。

**UDP部署限制**：bridge模式只提供对宿主暴露的认证TCP入口。原生SOCKS UDP仅回环，Linux可使用 docker compose -f compose.host.yaml up -d --build（host网络）并在控制台绑定127.0.0.1；Windows需要本机UDP时建议原生运行。两份compose方案二选一，不能同时使用同一数据卷。不能把认证TCP误当成同端口UDP也已认证。

镜像用非root用户、无额外 capabilities，数据卷持久化 /data。首次启动在 WebUI 在线安装内核到 /data/core，重建镜像不丢失已安装内核；镜像构建本身不下载或预置 Mihomo。现有旧卷可能仍需首次安装。无需 privileged、TUN或NET_ADMIN。

## 使用 API

管理接口采用同源 JSON REST。启用Token时使用 Authorization: Bearer；Token不要放URL。不要把管理Token当代理密码。

~~~sh
curl -H "Authorization: Bearer $UNIPROXY_ADMIN_TOKEN" http://127.0.0.1:9090/api/bootstrap

curl -X POST http://127.0.0.1:9090/api/import \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $UNIPROXY_ADMIN_TOKEN" \
  -d '{"mode":"text","content":"socks5://user:password@127.0.0.1:1080?udp=true#my-node","groupIds":[],"groupNames":["本地"]}'
~~~

配置修改通常返回 {data,runtime}；**HTTP200表示保存完成，不等于代理已生效**。自动化调用方必须检查 runtime.running、runtime.dirty 和 runtime.lastError。检测任务返回202+任务ID；查询 /api/jobs/:id。详细字段见 [API与模块契约](docs/implementation-contract.md)。

主要接口：

| 接口 | 用途 |
|---|---|
| GET /api/bootstrap | 全部公开状态（不含节点凭据） |
| POST /api/import | 导入/预览 |
| PATCH /api/nodes/:id、POST /api/nodes/batch | 节点管理 |
| POST/PATCH/DELETE /api/groups | 分组 |
| POST/PUT/DELETE /api/pools | 代理池 |
| POST/PUT/DELETE /api/listeners、POST /api/listeners/batch | 入口与批量端口 |
| POST /api/subscriptions/:id/refresh | 订阅刷新；force:true确认异常删减 |
| POST /api/tests | latency/speed异步任务 |
| GET /api/core/installation | 内核检测与后台安装状态 |
| POST /api/core/install（空 JSON 对象） | 后台安装固定内核，返回 202；不自动启动代理 |
| POST /api/core/inspect（空 JSON 对象） | 重新检测手动放置的内核，不应用配置 |
| POST /api/runtime/apply、/api/runtime/stop | 应用/停止 |
| GET /api/export/nodes、/api/export/backup | 含敏感信息的显式导出 |
| POST /api/import/backup | 恢复备份 |

## 运行语义与限制

- 轮询按连接/流调度，不是每个HTTP请求换IP；Keep-Alive、HTTP/2和CONNECT会复用出口。
- 原生粘性缓存10分钟，有限容量；节点故障、淘汰、成员变化、重载、重启可改变后续映射。没有跨重启持久化会话。
- in-user使用预先注册的入口用户名，不支持任意动态 session-xxx 用户名。
- 默认不回落直连。空池、禁用/删除目标拒绝流量。失效独享入口保留配置但自动停用，端口不会悄悄换绑节点。
- 普通修改不主动关闭已有TCP；原生reload可能重置UDP映射/粘性缓存。更改监听端口/认证、手动停止、内核升级或故障恢复重启可能中断连接。
- 配置校验不是上游实测成功。连通性通过实际代理请求检测；下载测速消耗流量，默认手动触发、并发最多2、单节点默认10MiB。
- 列表健康是全局检测URL的最近结果；池自身按各自URL/间隔由内核调度健康检查。
- 手动停止后，配置修改仍可保存但不自动重启，直到点击“重新应用”；重启UniProxy后会尝试恢复期望配置。代理内核崩溃后显示停止/错误，首版通过“重新应用”恢复；不静默无限重启。
- Clash文件内 dialer-proxy 可引用同文件的具体节点，保留中转链，支持前向引用与稳定ID去重；缺失、重名、循环、超过16层或引用策略组/内置DIRECT等会报错。禁用/删除中转会阻断下游新流量，不直连回退。源文件的全局端口、控制器和路由不会导入。
- 不接受本地私钥路径、脚本、外部provider、sing-box detour等危险/尚未映射字段。SSH可使用内联私钥，并建议配置host-key校验；未提供host-key时不验证SSH主机身份。具体兼容范围见导入说明。
- 没有真实上游凭据时只能验证语法转换与本地可控链路，不能声称所有公网节点可用。

## 数据安全与备份

配置快照上限32 MiB，备份恢复请求上限40 MiB，单次节点导入5 MiB。数据库是AES-256-GCM加密配置快照，master.key位于同一私有数据目录。备份数据库必须同时保留密钥；丢失密钥后不可恢复。日志不保存节点密码。**加密不防止已取得同账户/主机权限的攻击者**。

内核必须在内存中使用明文凭据，但候选校验/启动通过stdin传入、重载通过认证回环API传入，**不持久化明文内核YAML**。主密钥、进程内存和导出的JSON/YAML备份都应视作秘密。Unix使用0700/0600权限；Windows请保护账户与目录ACL。Web导出备份是明文，导出前有提示，不要提交Git或共享到公共平台。

## 测试

~~~sh
go test ./...
go vet ./...
pnpm run build:web
pnpm test:e2e
~~~

真实内核测试（先安装）：

~~~powershell
$env:UNIPROXY_TEST_CORE_PATH = (Resolve-Path bin/mihomo.exe).Path
go test -v ./internal/core ./tests/integration
~~~

若安装在平台数据目录的 core 子目录，替换上面的路径。Linux用 export UNIPROXY_TEST_CORE_PATH=/absolute/path/to/mihomo。未设置环境变量时，真实内核测试会明确跳过。

浏览器测试在隔离临时数据目录启动19090服务；Windows默认使用已安装的Edge，其他平台可运行 pnpm exec playwright install chromium。不要同时运行另一份19090测试服务。

[架构与安全边界](docs/architecture.md) · [API示例](docs/api.md) · [测试说明](docs/testing.md) · [导入兼容范围](internal/importer/README.md) · [Clash中转与导入筛选修复](docs/relay-fix-verification.md)

## 第三方许可证

Mihomo遵循GPL-3.0，打包分发时须遵守其许可证与相应源码提供义务；独立进程不代表免除内核分发义务。本项目尚未替项目所有者确定UniProxy自身开源许可证。详见 [第三方说明](THIRD_PARTY_NOTICES.md)。

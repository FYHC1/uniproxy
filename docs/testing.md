# 测试与验收

本文列出测试方式；具体已执行结果在交付说明中记录，不把计划当作已通过。

## 自动化

- go test ./...：解析、状态、加密持久化、订阅协调、安全边界、编译器与运行管理。
- go vet ./...：Go 静态检查。
- pnpm run build:web：TypeScript 全量检查及生产构建。
- pnpm test:e2e：启动隔离测试服务并执行浏览器功能流（不写用户数据）。
- pnpm test:ui：前端工具单元测试+内存构建fixture浏览器测试，不启动服务器。
- UNIPROXY_TEST_CORE_PATH：指向已验证的 Mihomo 后执行真实内核测试；没设置时这些测试跳过而非伪造结果。

## 在线内核安装验证

- [下载器测试](<../internal/coreinstall/install_test.go>)以注入 transport/摘要验证四平台归档、来源/TLS重定向边界、校验先于解压、有界大小/取消清理、无覆盖文件发布竞态；默认不联网。
- [内核安装生命周期测试](<../internal/core/installation_test.go>)与[管理 API 测试](<../internal/app/core_install_test.go>)覆盖自动发现、重复/运行保护、真实字节状态、安装不自动应用、关闭取消等待、鉴权/严格空对象与重新检测。
- 浏览器隔离服务对安装 POST 固定返回 409，客户端也默认拦截；真正在线安装只能在单独、明确的 opt-in 集成测试中执行。
- [真实下载 API 测试](<../tests/integration/core_install_test.go>)仅在 `UNIPROXY_TEST_CORE_DOWNLOAD=1` 时访问官方发布并安装到临时目录，经真实版本检测后显式应用空配置、停止清理。不读取订阅/用户配置，不使用公网代理节点；常规测试该变量保持为空。

~~~powershell
$env:UNIPROXY_TEST_CORE_DOWNLOAD = "1"
go test ./tests/integration -run '^TestOnlineCoreDownloadThroughManagementAPI$' -count=1 -v
# 当前 shell 后续普通测试前清除此 opt-in
$env:UNIPROXY_TEST_CORE_DOWNLOAD = ""
~~~

## 必测功能

1. 各协议链接/Clash/sing-box 配置导入，非法参数及不支持变体明确拒绝，错误不泄漏密钥。
2. 批量导入重复节点去重，多分组静态/动态成员、并集/交集/排除。
3. mixed同时支持HTTP与SOCKS5；单协议不接受另一种协议；TCP CONNECT与SOCKS5 UDP真实本地回显。
4. 池原生四策略、一致哈希认证键、URLTest检测、空池/禁用节点/错误凭据fail-closed。
5. 订阅网络失败不删节点；正常移除清理引用；多来源/人工节点不误删；异常删减需确认。
6. 配置校验失败不提交，应用失败显示待生效并保留运行版；端口冲突；正常reload已有TCP隧道继续传输。
7. 管理Token、Host/Origin校验、订阅SSRF/大小限制、备份安全、API响应凭据脱敏。
8. 重启恢复配置、取消检测任务、并发限制、数据加密round-trip。

## 负载验证（需在目标硬件执行）

用500个节点配置验证导入/列表/分组与编译耗时，再分别增加并发连接和UDP流。观察CPU、RSS、句柄、错误率、p95延迟及吞吐。内核文件数不是吞吐指标；不得将本地回显测试作为公网多协议/带宽性能结论。

真实上游协议互通和公网吞吐需要用户提供受控测试节点。测试不访问或修改浏览器系统代理，不修改防火墙或用户全局网络配置。

## 已有执行记录（历史，不是每轮自动重测）

首版证据见[历史验收](<verification.md>)，中转修复证据见[修复验收](<relay-fix-verification.md>)。20:31文档整理轮没有重跑产品测试；随后代理池优化轮已重新执行完整相关验证，见[代理池优化验收](<pool-workflow-verification.md>)。

- Windows Go单元/集成测试、go vet、TypeScript及Vite生产构建。
- 固定Mihomo真实HTTP/SOCKS/mixed认证链路、UDP数据报、真实URLTest与限量下载、四种原生池策略路由、正常reload/端口交换保留TCP长连接、空池/禁用/删除目标拒绝新流量。
- API导入→自动应用→实际上游转发→检测测速→禁用断新流量的全链路。
- 各协议代表配置（含Reality、WS、gRPC）经过真实内核 -t 校验；不等于已测试远程协议握手。
- 后端安全复核修复6项中等级问题，并加入来源归属、旧版本检测丢弃、刷新与备份恢复竞态、重定向Referer、批量端口非法地址、超过12MiB备份恢复回归。
- 首版Playwright为5项，修复后增加拖拽/排除筛选至7项；连接真实Go API和临时SQLite，但测试服务故意不安装内核，不能当真实代理链路证据。前端工具测试由9项增至21项，另有内存构建的fixture浏览器回归。
- UniProxy及安装器Linux amd64/arm64交叉编译成功；没有相应Linux主机实跑证据。Docker CLI不可用，未实际构建/启动容器。
- Race detector受当前CGO=0且无C编译器限制，未执行；普通并发单元测试不替代race。
- 导入解析与SSH私钥解析分别通过30秒fuzz测试。

## 代理池优化轮（2026-10-19 22:23）

- 严格TS、30项工具测试、fixture浏览器回归、20项真实隔离API浏览器流程全部通过；125项选择/110项搜索全选、旧排除、动态纳入及并发删除草稿均覆盖。
- Go全量使用固定真实core与本轮Windows暂存程序，通过后执行go vet全量通过。新[本机工作流](<../tests/integration/pool_workflow_test.go>)验证原子创建认证入口、真实测延迟、移除最后节点拒绝及同端口重新纳入。
- [API事务测试](<../internal/app/pools_test.go>)、[端口测试](<../internal/app/pool_ports_test.go>)及[成员操作测试](<../internal/app/pool_members_test.go>)覆盖原子保存/失败/待应用、手动停止、端口和认证安全、幂等移除及关联对象不变。
- 私有导入变量保持空，本轮未测用户原始文件或真实远端节点；Linux两架构仅交叉构建，Docker/race/吞吐限制仍未验证。

## 订阅与分组体验轮（2026-10-19 23:20）

- 严格TS、31项工具、fixture浏览器和32项真实Go/API浏览器全部通过。
- safehttp23函数与子场景（19新增）覆盖Fake-IP/DoH/TLS/固定拨号/取消/重定向/大小/HTML及fec0拒绝；app新增14函数覆盖订阅安全错误传播/保留来源、分组归属与全局状态。全部为注入或本机数据，无默认远端调用。
- Windows新版暂存、Linux两架构cross-build通过；显式core/APP路径的Go全量与vet通过。
- 独立裸域名诊断一次：系统Fake-IP、公共DoH、验证TLS，以及新客户端根HEAD HTTP200；未请求含令牌路径/正文。全量测试时裸域名及私有文件变量均为空，对应测试跳过。
- 细节和未测范围见[本轮验收](<subscription-groups-verification.md>)。

## 入库与全面复查轮（2026-10-20 01:10）

- 首次将源码推送到私有 GitHub 仓库；提交前扫描确认无凭据、私钥、订阅链接与个人绝对路径，构建产物/数据目录不入库。
- 复查方式：全部 Go 源码与测试、全部前端页面与纯函数、契约/API/架构/决策文档、CI/Docker/compose；再用暂存二进制与临时数据目录做 60 余条真实 HTTP API 边界探测。
- 通过：Go全量 -p1 -count1、go vet、gofmt、严格TS（含noUnused）、32项工具测试、32项API浏览器、fixture UI、真实本机内核链路、构建后进程/CLI/manifest测试、Windows暂存+Linux两架构。
- 修复7项缺陷并补回归：长名称导致批量独立端口整批失败、预览与保存名称口径不一致、订阅元数据下载后才校验、监听页/节点页待同步误标“监听中”、活动记录按字节截断产生非法UTF-8、fixture脚本写死浏览器路径、文档含个人绝对路径。
- 未执行 race（无C工具链）、Linux/Docker实跑、容量压测；未访问真实订阅或真实节点。细节见[复查验收](<repository-review-verification.md>)。

## 内置安装轮（2026-10-20 11:09）

- 最终Go全量显式Mihomo/暂存程序、vet/gofmt、严格TS、36工具、fixture UI、34真实API浏览器通过。
- 下载器40函数/97子用例，92.0%覆盖，2个OS symlink子用例因宿主权限skip；本轮真实临时官网下载、摘要/版本检测、显式空配置启停通过。
- 四平台仅交叉构建暂存，未部署/提交/发布。Linux/Docker/Windowsarm64实跑、race、私有订阅/公网节点/容量压测未做。[完整验收与失败调查](<core-install-verification.md>)。

具体测试源、命令和限制可复查，不以虚拟节点/虚构延迟填充生产界面。

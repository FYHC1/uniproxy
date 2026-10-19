# UniProxy 会话接续快照

**最后核对：2026-10-20 01:10（Asia/Shanghai）。M11首次入库GitHub + 全面复查修复已完成并重新暂存；用户运行进程仍为2026-10-19 23:25启动的旧构建，未部署、未停止服务、未改动DB/主密钥/原始节点文件。**

## 30秒摘要

- 当前栈：Go + React/TypeScript + SQLite配置payload加密 + 外置Mihomo1.19.32；不是Node/sing-box早期草案。
- M0–M11完成。M11：仓库初始化并推送私有GitHub仓库，全面复查后修复7项缺陷。详情见[复查验收](<repository-review-verification.md>)、[M10验收](<subscription-groups-verification.md>)和[计划](<development-plan.md>)，不要重做旧任务。
- **当前用户运行的是2026-10-19 23:25:57启动的旧构建（PID 5248，SHA256 22596106…），M10/M11新版均未部署。** 本轮未替用户升级，仅只读核对进程与文件。
- M11修复：长节点名导致批量端口整批失败、预览/保存名称口径不一致、订阅元数据下载后才校验、监听页/节点页把待同步地址显示为监听中、活动记录按字节截断产生乱码、测试脚本写死浏览器路径、文档含个人绝对路径。
- M11实际通过：**32 API浏览器、32工具、Go全量 -p1、vet、gofmt、严格TS、fixture UI、真实本机内核链路、构建后进程/CLI/manifest、Windows暂存+Linux两架构**。未请求私有订阅路径/正文或真实节点；race/Docker/Linux实跑/压测未做。
- 下一步仅在用户批准后部署最新版/用户验收，或接受新的明确任务；不要擅自停服务、删除节点或换数据目录。

先读：[需求](<requirements.md>) → [决策](<decisions.md>) → [开发指南](<development-guide.md>)。本文PID/哈希有时效性，未来操作前重新核对。

## 1. 当前工作区与用户实例

| 项目 | 2026-10-20 01:10观察 |
|---|---|
| 工作区 | 本仓库根目录（会话启动时由运行环境提供；文档不记录个人绝对路径） |
| 版本控制 | 已初始化 git 并推送私有 GitHub 仓库；初始提交 `7829f25`；本轮修复改动另行提交 |
| 用户应用 | PID5248，启动2026-10-19 23:25:57，[bin/uniproxy.exe](<../bin/uniproxy.exe>)，无CLI参数 |
| 子内核 | PID32296，父5248，启动23:26:02，[bin/mihomo.exe](<../bin/mihomo.exe>)，-f -从stdin读配置 |
| 数据线索 | 子内核-d指向[bin/data/core](<../bin/data/core/>)，用户数据预计[bin/data](<../bin/data/>)；没有读取DB/密钥，升级前仍需核对启动环境/CWD |
| 其他目录 | 根[data](<../data/>)来自更早测试，不能与用户数据混用或自行清理 |
| 管理端口 | 默认9090；本轮未请求用户运行API，未把默认值当监听事实 |
| 测试端口 | API浏览器仅临时19090，测试结束不保留服务；真实core测试均临时状态/本机受控上游 |

默认-data=./data相对于进程CWD，不是exe目录。不能在旧实例运行时直接启动暂存程序；启动真实数据会自动apply/订阅刷新/检测，不能当只读检查。不得复用这里PID作为未来kill命令。

### 构建身份（SHA256）

| 产物 | 字节数 / mtime | SHA256 |
|---|---|---|
| [当前运行文件](<../bin/uniproxy.exe>) | 19779584 / 2026-10-19 23:16:57 | 22596106E243811EC3FCB81A5C39A982643D731D9782CBAC8EDC49F1F1DC81AB |
| [Windows M11新版](<../bin/updates/uniproxy.exe>) | 19785216 / 2026-10-20 01:42:34 | 510CAE7FE157938A8F5060AE5B66363E73A73EFF9CE90F3F63CABF20CB147317 |
| [Linux amd64 M11](<../bin/updates/linux-amd64/uniproxy>) | 19335940 / 01:42:37 | 0C901F935D2346B201931336EF2EE7EF9771436D0F9BAA91C791C45F78E658F7 |
| [Linux arm64 M11](<../bin/updates/linux-arm64/uniproxy>) | 18211567 / 01:42:39 | 229771A0D1AB43CF2882BE6EAA9F502B9C28A8250352394E6FF9CA56C7826BFA |

仍报告0.1.0，不可用health/version字符串辨认补丁；health200只是存活，不代表core/config ready。哈希不是签名或自动升级授权。

历史20:31：应用PID27412、核心14788，当时运行文件19565056字节/SHA256 B2F45BFD28154D156A7F91F84678FFE983B3D07ADFC4279097743B3B2B405778，暂存为4A8FC2F8…；这些均不作为当前运行事实。

## 2. 当前关键实现语义（M9保留，M10补充，M11修订）

### 代理池选择

[对话框](<../src/web/PoolDialog.tsx>)用[纯选择函数](<../src/web/poolSelection.ts>)：

- 未选组候选为所有节点；已选组候选是any并集/all交集，再按池UDP兼容过滤。
- 组成员默认勾选，取消变excludeNodeIds；重新勾选清除排除，组命中不额外静态pin。动态订阅新增仍自动纳入。
- 全选包含搜索结果所有分页，保留搜索之外选择；禁用旧group-excluded项不能通过全选绕过。
- 显式改变组/模式/UDP清理范围外手动引用并提示，取消对话框不保存；仅打开/改名称不改旧成员。
- 旧excludeGroupIds继续生效并展示兼容说明，没有排除组选择器；新池为空，可明确清除旧规则后保存。节点列表排除组功能不变。
- 最新bootstrap确认节点已删除时，保存清理nodeIds/excludeNodeIds中真正失效引用；不能因范围/搜索隐藏而清掉有效引用。

### 池与监听事务 / 移除

[API](<../internal/app/pools.go>)增加：

~~~text
POST /api/pools/with-listener
{pool:<Pool editable fields>,listener:{name?,protocol?,host?,port?,udp?,enabled?,users?}}
=> {data:{pool,listener(public)},runtime}

DELETE /api/pools/{id}/members/{nodeId}
=> {data:Pool,runtime}
~~~

组合创建一次mutate/校验/revision/保存/应用，不是两个前端请求。服务器分配身份与绑定，拒绝注入；默认监听名=池名，回环mixed TCP。port0/省略自动分配；手填允许设置范围外1024–65535。统一[端口分配器](<../internal/app/ports.go>)跳过管理和已有配置（含禁用）并探测TCP/所需UDP。没有永久预留；应用失败可能保存两者但dirty，不能补偿删除。

成员DELETE在当前服务器快照删除手动引用并加节点排除，旧group exclusions不变；不改总节点、分组来源、中转、其他池/监听。空池REJECT。旧池POST/PUT/DELETE行为与schema1保留。

### 卡片与检测

[卡片](<../src/web/PoolCard.tsx>)展示全部关联监听、复制/编辑/追加入口，成员最近实际延迟和禁用/中转状态，单/整池检测、确认移除。dirty/applying只显示配置地址，不冒充已生效。检测使用全局Settings.TestURL而非池自动健康URL；不模拟指标。

服务重启导致任务记录丢失时有失效说明并解除忙碌；成员降至分页阈值以下但仍有搜索词时保留可清除搜索框。外部节点删除不会让编辑草稿陷入不可保存。

### M10：订阅Fake-IP传输修复

系统DNS对用户截图主机返回198.18.0.230，原安全策略拒绝且隐藏原因。新[客户端](<../internal/safehttp/client.go>)/[DoH](<../internal/safehttp/doh.go>)仅全Fake-IP域名触发固定公共加密DNS的A/AAAA复核，验证全部公网IP后固定拨号、校验原域名TLS；literalFakeIP/普通私网/混合答案不放行。未使用系统HTTP代理、未改网络设置。

[安全错误](<../internal/safehttp/errors.go>)保留类别/HTTP码不保留原url.Error等秘密。HTML、下载、配置解析分别说明；订阅源不完整不导入/不删除，空200仍原确认规则。显式allowPrivate模式保留旧行为，不推荐用来解决Fake-IP。

本轮有界域名证据：系统DNS、固定公共DoH、原域名TLS1.3、[新客户端根HEAD](<../tests/integration/subscription_host_test.go>)返回200+校验TLS。**没访问私有订阅路径、没读正文，不能称用户完整订阅导入/远端节点已验证。** 主机诊断环境变量默认空。

### M11：显示名称与运行状态一致性

显示名称（分组/节点/池/监听/订阅）统一1–160字符且禁控制字符，但该上限只约束“保存的显示名”：[导入器](<../internal/importer/native.go>)对超过160字符的上游标签按字符边界截断加“…”，超过1024字符或含控制字符仍判无效输入，空白标签回退协议+地址。因此预览展示的就是保存结果，不再出现“预览通过、确认导入失败”。

服务端派生名称必须为后缀预留空间：[批量独立监听](<../internal/app/http.go>)用[derivedName](<../internal/app/validate.go>)生成“<节点名> · 独享”，接近上限时截断而不是让整批400。名称类校验错误点名字段（节点/监听/分组/订阅名称、监听地址、监听端口、订阅URL、间隔），不再把名称问题伪装成端口问题。订阅名称/分组名/刷新间隔在下载前校验，且只在订阅模式生效。

监听运行状态由[listenerState](<../src/web/poolView.ts>)统一判定，池卡片、监听页、节点页共用；`appliedRevision`与`revision`不一致或dirty/applying时不得显示“监听中”。活动记录按字符边界截断，禁止产生非法UTF-8。

### M10：紧凑池与分组管理

[池页](<../src/web/PoolsPage.tsx>)用真实统计+策略/状态过滤；[卡片](<../src/web/PoolCard.tsx>)单池宽屏双栏、6节点分页/限高、2入口后明确展开、节点details、真实成功失败未知/中位延迟，revision mismatch显示状态待同步而不是旧地址在线。

[分组页](<../src/web/GroupsPage.tsx>)组名/数量打开[节点管理](<../src/web/GroupNodesDialog.tsx>)：20条分页、组范围筛选/全选、批量添加、仅移出本组、全局启禁确认、实际延迟任务。使用旧batch API，不删除总节点/覆盖所有组。订阅刷新或修改可重新分组，人工加组不改Manual保留标志。组页分列包含与旧排除引用，删除组/移出旧排除组可能扩大池范围需提示。最新数据清理失效选中ID，缺失组只可关闭。

## 3. 之前完成的背景（历史事实）

首版包含多协议导入、分组/订阅、四策略池、独享/共享监听、回环UDP、实际检测/限量下载、自动应用回滚、认证、加密配置、备份恢复，六个Web页面；没有独立流量分析模块。

前轮Clash中转/拖入/节点过滤已完成，不能误开旧待办：

- 前轮授权离线验证的私有Clash文件（路径不写入仓库）共328定义、178中转、325唯一节点；3别名等价。**这是历史授权离线测试事实，本轮未读取或重测该文件，也没使用真实凭据联网。**
- 旧失败是拒绝dialer-proxy和128详情 cap提前停止扫描，不是GFW判断；解析继续扫描并统计totalErrors。
- 稳定DialerID/DialerIdentity保留上下游身份；缺失/禁用中转阻断，不得删dialer字段直连。原生Config不得直接透传不可信dialer-proxy。
- 导出保留安全占位避免builtin/缺失引用捕获；公共response副本才清除私有身份字段；来源归属/备份恢复不误删共享节点。

[首版历史验收](<verification.md>)、[中转修复验收](<relay-fix-verification.md>)保留当时5/9、7/21测试数字，不是本轮重复测量。

## 4. 本轮验证与限制

完整命令、缺陷清单与边界：[M11复查验收](<repository-review-verification.md>)；[M10验收](<subscription-groups-verification.md>)、[M9验收](<pool-workflow-verification.md>)作为历史保留。

- 严格TS（含noUnused）、生产Vite（build:web，未覆盖运行exe）、Windows暂存构建、Linux amd64/arm64交叉编译通过。
- node工具32/32；内存构建fixture浏览器通过（含375/390/768/1024，零未捕获错误）。
- 真实API浏览器32/32，55.8秒；[fixture隔离器](<../tests/browser/fixtures.ts>)限制19090/missing core、唯一名称、文档地址、仅清理测试归属对象。
- Go全量 -p1 -count1 通过，go vet与gofmt通过；真实内核链路用显式UNIPROXY_TEST_CORE_PATH（本地bin/mihomo.exe），构建后进程/CLI/manifest用UNIPROXY_TEST_APP_PATH指向暂存exe；私有文件与裸主机变量为空。
- 真实本机链路验证 HTTP/mixed转发、URLTest、限速测速、禁用即拒、池原子创建/空池拒绝/同端口重新纳入、13协议配置校验。
- 复查发现并修复7项：长名称批量端口整批失败、预览/保存名称口径不一致、订阅元数据下载后才校验、监听页/节点页待同步误标监听中、活动记录字节截断乱码、测试脚本写死浏览器路径、文档个人绝对路径。均有回归或行为复测。
- 未做：私有订阅正文、真实公网节点握手/吞吐、Linux/Docker实跑、race、容量压测。

Vite单chunk 509.25kB/gzip 150.33kB仍触发500kB非失败警告，未修改阈值掩盖；分包是后续优化。当前运行文件与暂存构建哈希不同，说明部署未发生。

## 5. 不可破坏的安全/兼容边界

- 正式数据/密钥不作为测试夹具，不读取或输出私有配置。所有新功能测试只临时数据。
- 配置保存≠应用；手动Stop暂停auto apply，失败保留dirty/lastError；校验失败不提交。
- 失败即拒绝，不直连回退；固定Mihomo1.19.32；回环UDP且禁UDP in-user，不把TCP认证当UDP认证。
- SQLite仅payload加密，WAL在线不可只拷主db；配置明文只内存/stdin，剔除CLASH_/MIHOMO_环境覆盖。
- 源节点多来源归属不提升成全部人工固定；任务/刷新有revision/代数校验。
- 普通pnpm build覆盖原bin exe，有活动服务时只暂存；前端嵌入Go，刷新旧程序不能生效。

## 6. 明确下一步及接续

1. 本次复查与修复完成；最新源码已构建到暂存目录，未执行部署。用户若希望马上使用，先确认允许停机、准确CWD/数据目录和一致备份，再正常退出旧服务、替换原位置、原数据启动、刷新Web。
2. 若继续开发，按[计划](<development-plan.md>)或新用户要求限定范围，不重做已完成功能；先读[根入口](<../AGENTS.md>)及本快照。
3. 本轮无并行写任务、无后台任务；用户服务不是残留，不清理它。仓库已推送私有GitHub，修复改动仍需提交推送。
4. 收尾继续更新计划/本快照/变更/决策与实际测试状态，核对产物与运行差异；不宣称部署或未经实测的平台成功。

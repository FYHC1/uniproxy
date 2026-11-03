# UniProxy 开发计划与任务状态

最后更新：2026-11-03 10:35（Asia/Shanghai）。负责人：项目维护者/当前执行开发者；不依赖已经结束的子代理或聊天任务板。

**当前结论：M0–M30完成，M30 协议适配三档已随 v0.2.9 于2026-11-03 16:22:39正式发布并设Latest（不含部署）；M26–M29 已随 v0.2.8 于2026-11-03 10:28:28正式发布并设Latest（不含部署）。v0.2.4（M19代理池编辑体验）已于2026-11-01 12:18:53正式发布并设Latest并部署替换本机实例；2026-11-01 17:02 在 WSL2/Fedora 44 amd64 完成官方 Linux v0.2.4 程序实机运行验收（M21），随后完成 Linux 稳定性修复（M22）与全局出站网卡选择（M23）的本地实现与复验。M22 提交 `ede12bb`、M23 提交 `a4dd45d`，v0.2.5 已于2026-11-02 02:27:40正式发布并设Latest（不含部署）；历史v0.2.3/v0.2.4标签与资产不变。M24 现代 VLESS/VMess 协议兼容提交 `dba132c`，v0.2.6 已于2026-11-02 16:00:55正式发布并设Latest（不含部署）；发布验收时本机仍运行用户自行启动的 v0.2.5（主程序 PID29920 + 内核 PID11688），本轮未停止/替换；历史 v0.2.5 及更早标签与资产不变。M25 代理链已在本机实现并通过 Go/API/真实内核/浏览器验证，已随 v0.2.7 发布（不含部署）；M26 新建代理链弹窗布局调整已在本机完成并通过严格 TS/生产 Web 构建/链浏览器 6/6，已提交并推送 main（`607dc8b589836aa0a58782c41b8f5559617257d1`，main CI 41487587509 成功）；M27/M28/M29 随后完成并随 v0.2.8 发布。**

相关：[需求基线](<requirements.md>) · [开发指南](<development-guide.md>) · [当前运行/构建快照](<session-handoff.md>)。

## 1. 状态约定

- 已完成：有源码及列出的验证记录；不暗示已部署。
- 已构建待部署：可执行产物存在，但现有用户进程仍运行旧文件。
- 待确认：实施前需要需求/操作授权或测试环境。
- 待验证：实现/构建存在，但没有相应平台或场景的实测结果。
- 候选：可能有价值，但不是已承诺范围，不得自动开工。

## 2. 已执行里程碑

| 里程碑 | 交付与验收 | 状态 |
|---|---|---|
| M0 需求讨论 | 个人+自动化、三平台、单管理员、多组、多策略、UDP、自动应用；见 R01–R11 | 已确认 |
| M1 工程基座 | 从未实现业务的 Node/sing-box 草案改为 Go + React + SQLite + Mihomo；两面分离 | 已完成 |
| M2 配置导入 | 全部目标协议家族、URI/Base64/Clash/sing-box、严格安全映射、去重；单元/fuzz | 已完成 |
| M3 内核运行管理 | 固定内核校验、认证私有控制器/探针、池/入口、TCP/UDP、热更新回滚、校验安装器 | 已完成；真实本机链路通过 |
| M4 管理/API/存储 | 加密 SQLite、节点多来源/多分组、订阅协调、任务/取消、备份恢复、安全请求校验 | 已完成 |
| M5 中文控制台 | 六个页面：节点、分组、池、入口、订阅、运行设置；认证/真实空态/错误/批量操作 | 已完成 |
| M6 首版集成 | Go/vet/TS/build、真实内核、5 个浏览器流程、9 个 UI 工具测试、Linux 构建 | 已完成；历史口径见首版验收 |
| M7 反馈修复 | R12–R14：安全中转图、错误截断修复、拖入文件、排除/未分组、备份恢复链身份 | 已完成并构建待部署 |
| M8 文档固化 | R15：索引/需求/计划/指南/决策/接续/变更记录，归档旧协作备注；源码核对与本地链接/围栏检查通过 | 已完成（仅文档） |
| M9 代理池交互优化 | R16–R18：分组限定与全选、一并创建监听、卡片及真实检测/池内移除；保持旧 API/动态组/安全边界 | 已完成并构建待部署；[验收](<pool-workflow-verification.md>) |
| M10 订阅与管理体验 | R19–R21：安全Fake-IP DNS兼容/诊断、紧凑池卡片、分组节点管理 | 已完成并构建待部署；[验收](<subscription-groups-verification.md>) |
| M11 入库与全面复查 | 源码初始化并推送私有GitHub仓库；全量代码复查；修复名称上限派生/预览口径、订阅元数据校验时机、监听状态误报、活动记录UTF-8截断、测试浏览器探测与文档绝对路径 | 已完成并构建待部署；[验收](<repository-review-verification.md>) |
| M12 数据目录与发布 | R22：平台标准数据目录+旧目录沿用、版本单一来源与 `-ldflags` 注入、标签驱动四平台发布工作流 | 已完成并**发布 v0.2.0**（四平台+校验和已复核）；[验收](<data-release-verification.md>) |
| M13 内置内核安装 | R23：自动检测/缺失弹窗、运行与设置在线安装、后台进度/重检、固定摘要/无覆盖/并发保护、取消独立安装器分发 | 已完成并随v0.2.1发布；[功能验收](<core-install-verification.md>)；未部署到用户实例 |
| M14 v0.2.1发布 | 用户授权提交/推送/tag/Release；版本同步、四平台构建、资产/校验和及Windows发布程序核验 | 已发布并验证；[发布验收](<release-0.2.1-verification.md>) |
| M15 Windows重启闪退 | R24：实际malformed诊断、授权离线恢复、错误窗口/持久日志、早期取消、Windows自有子进程保护 | 修复验收通过；[验收](<windows-startup-verification.md>) |
| M16 v0.2.2发布 | 用户授权发布M15修复；版本/说明、两平台CI、四平台资产与Windows程序核验 | 已正式发布并设Latest；[发布验收](<release-0.2.2-verification.md>) |
| M17 订阅格式兼容 | R25：TLS 默认值误拒修复、公开参考客户端源码调研、多格式订阅隔离验证及回归 | 开发完成并暂存，最终只读复查通过；[验收](<subscription-format-verification.md>)；后续发布由M18记录 |
| M18 v0.2.3发布 | 用户授权发布M17；版本/中文说明、main与标签CI、四平台正式资产校验 | 已正式发布并设Latest，正式Windows程序验收通过；[发布验收](<release-0.2.3-verification.md>)；未部署或访问用户数据 |

M7 后测试口径：**7 项真实隔离 API 浏览器流程、21 项 UI 工具测试**，另有 fixture 浏览器回归；Go 全量/内核中转/指定文件离线验证通过。历史覆盖率不是每次修改后的实时指标。详见[测试说明](<testing.md>)、[中转修复验收](<relay-fix-verification.md>)。

M9后口径：**20项真实隔离API浏览器、30项工具测试**，另有fixture交互与真实本机池创建/移除链路；[验收记录](<pool-workflow-verification.md>)列明命令及限制。22:21核对当前运行文件已为前轮中转修复，不再是20:31文档轮的首版；本轮代理池新版仍需获准部署。

M11后口径：**32项真实API浏览器、32项工具**（新增共享监听状态用例），并新增Go侧名称/派生名/UTF-8回归函数；真实本机内核链路与构建后进程测试通过。[验收](<repository-review-verification.md>)。

M10后口径：**32项真实API浏览器、31项工具**，safehttp23测试函数、app新增14函数及矩阵，Go全量/真实本机核心/构建通过。域名根HEAD返回200验证Fake-IP传输修复，不代表私有订阅全文已测；[最新验收](<subscription-groups-verification.md>)记录事实。M9运行文件已于22:30启动，本次M10仍只暂存。

## M13 — WebUI 内置 Mihomo 安装（2026-10-20，已完成，待部署）

- 用户 R23：不再依赖独立安装器；自动检测缺失、首次管理提示、运行与设置页在线安装。
- 实施：固定版本安全下载复用为内部包；后台安装及可轮询状态；并发/进程/目标文件保护；成功后显式应用，不自动开启代理。
- 验证：最终Go全量+真实本地Mihomo+暂存APP/vet/gofmt通过；下载安全40函数/97子用例（OS symlink2skip）、manager10/API7、TS/36工具、fixture UI/34 API浏览器、真实临时官网下载/校验/显式空配置启停通过。Windows/Linux四平台仅暂存，[完整验收](<core-install-verification.md>)。
- 10:44 复核：9090 有用户启动的发布版实例，本轮不得停止/替换；未读取正式数据库/主密钥。本轮不提交、不推送、不发布。

## M14 — v0.2.1 发布（2026-10-20 15:29，已完成）

- 按用户授权将M13功能与0.2.1版本提交推送为d8f394e，main CI成功后推v0.2.1标签；仓库保持私有，无强推/重写历史tag。
- main与标签Windows/Linux CI全部通过，Release四平台build及release job全部成功；15:25:50正式发布并设Latest，中文说明已补全。
- 四个主程序和SHA256SUMS.txt精确名单/大小/摘要通过，Windows发布文件-version准确匹配tag commit，临时CLI/内核/health/嵌入UI+CSP测试通过。
- 本轮重跑Go全量/vet、36工具/严格TS、生产Web、候选CLI/嵌入服务、34项API浏览器（54.3秒）通过；详见[发布验收](<release-0.2.1-verification.md>)。
- 未部署；15:29未观察到用户应用/内核进程或9090监听，非本轮停止；没有读取数据库/主密钥。

## M15 — Windows 启动闪退与数据库恢复（2026-10-20）

- 实际错误为SQLite malformed(11)。现库头179页、实际4页、无WAL且配置溢出页缺失；用户确认在应用运行时覆盖数据库。合成数据复现旧活动WAL在关闭时将被覆盖的大库截断至4页。
- 用户逐次授权只读诊断/验证备份/离线恢复且暂不启动；选择较新的完整旧库，密钥匹配、完整性/解密/模型/离线编译通过。原子替换仅恢复DB并保留损坏证据/密钥，不改Mihomo/系统网络，不启动原配置；公开文档省略用户配置数量。
- 代码：CLI错误不再直接退出丢失提示；独立Windows双击错误窗口等待Enter、独立有界日志；开库前路径和端口诊断，损坏不清空；busy_timeout先于WAL；提前信号/Close取消；Windows生命周期Job无句柄继承、嵌套/强制退出保护。
- 已通过Go全量（显式本地Mihomo）、vet、新增存储/CLI、实际候选三轮强杀重启/核心退出、真实WM_CLOSE两轮重启及失败窗口保留；四平台最终构建和独立审查结果见验收。只读旧调查2项和processguard共享任务14完成；收尾前收集Lead验证任务。
- 最终候选08:28:42Z四平台构建、Windows实际Go/vet/三轮强杀/两轮WM_CLOSE重启通过；健康检查日志累积和CLI退出码兼容审查项已修复。一次旧真实URL-test fixture亚毫秒503经真实3ms响应延时修复，20遍与最终全量通过。
- 本轮无提交/推送/发布，v0.2.1标签和发布文件未变；新产物显式0.2.1-fix.1本地标识，未替换下载exe。16:22用户自行启动旧bin程序PID31404、核心25652，9090版本0.1.0；助手保留不动，不代表修复候选已部署。

## M16 — v0.2.2 正式发布（2026-10-20，已发布）

- 用户明确要求为启动失败诊断和Windows子进程保护发布新Release，采用补丁版本0.2.2；提交/推送/新标签获准，不含部署。
- 保留M15功能与测试，版本单一来源和中文说明同步；只提交源码/安全文档，不包含本机诊断、DB/key/备份、EXE或临时文件。
- 提交22849c9推main，Windows/Linux CI成功后推v0.2.2；标签CI和四平台Release全部成功，16:49:21发布并设Latest。所有资产与清单/GitHub digest匹配；正式Windows程序CLI/嵌入UI/3轮强杀重启/2轮WM_CLOSE及错误窗口全部通过。见[发布验收](<release-0.2.2-verification.md>)。
- 16:51用户旧PID31404/内核25652和9090监听保持；本轮没有部署/启动或私有数据操作。后续文档提交只回写发布结果，不移动标签。

## M17 — 订阅格式兼容（2026-10-20）

- 原图订阅下载成功后7个VMess，其中4个tls:false+skip-cert-verify:false触发旧presence检查；修复false/空SNI无操作默认值，不改变实际TLS或证书验证。原源7/7解析和内核配置校验通过。
- 新增QX/Surge/Surfboard/Loon静态文本适配，支持同值SNI别名、STATUS展示头，保留URI/Base64/Clash/sing-box；不导入全局脚本/规则/动态provider或策略组policy-path。
- 安全review修正WS Host/SNI歧义、验证名/绕过冲突、缺失插件Host和YAML块/显式键被误认问题。UDP未显式开启则TCP-only，明确不是完整复制相同语法各客户端默认行为。
- 9份授权只读响应：7份有效过离线core-t（6份各12、原源7），2份各5个非法SNI应拒绝；用户原始凭据不入仓库，完成后本轮11个临时下载/清单副本已清理。
- 本地Go全量/vet、24适配测试/新TLS与API回归、37隔离浏览器+5末次复测、36工具/严格TS、20/30秒fuzz、9组core-t/3组回环ClientHello、Windows候选CLI/实际导入/强杀重启/日志验证通过。候选在ignored更新目录，不部署、不提交/推送/发布；详见[验收](<subscription-format-verification.md>)。

## M18 — v0.2.3 正式发布（2026-10-21，已完成）

- 用户明确要求推送并发布，采用补丁0.2.3；范围为M17修复、回归、版本及安全可公开文档，不扩大解析功能。
- 01:29复核仓库公开、main与origin均28021bf，v0.2.3不存在；当前用户主程序PID6244与其内核23716、9090保持，本轮不停止或替换。
- 版本/说明已同步；两项只读源码/隐私及流程审查无已知阻塞。生产Web、严格TS/36工具、Go全量显式APP/CORE、vet/gofmt/diff、37项隔离浏览器和完整UI fixture通过；未重下载私有样例。
- 提交5ed4cdf（33文件）已推main，main CI39467680808两平台成功后推annotated v0.2.3；标签CI39468279998和四平台Release39468280079全部成功。01:42:43发布、中文说明逐字匹配并设Latest，4程序+SHA256SUMS全部与清单/远端digest匹配。
- 下载的正式Windows版本0.2.3/5ed4cdfd5a40/17:41:59Z，所有TestBuilt通过3.955秒（实际导入、CLI、3轮强杀重启/核心退出、嵌入UI、日志）；WM_CLOSE两轮退出0并重启、独立失败窗口保留/关闭后退出1通过。
- 01:45观察原用户6244/23716及9090未变，19090无残留。历史tag与Release/asset ID不变；后续仅提交发布结果文档，不移动标签，不部署。
- 全部新进度见[发布验收](<release-0.2.3-verification.md>)，不得把前轮私有样例测试当成本轮重跑。

## M19 — 编辑代理池体验（2026-10-22，已完成，已发布并部署）

- 用户要求收紧编辑代理池弹窗排版；包含分组固定显示四行并可滚动；包含节点先要求“添加/移除”暂存，后明确撤回，改为测试、排序、状态筛选。范围为前端交互、样式与浏览器回归，无API、数据模型、内核或安全策略变化。
- 实现：节点恢复即时勾选并仍由changePoolSelection统一推导nodeIds/excludeNodeIds；新增状态筛选与名称/延迟/速度/更新时间排序，均只影响显示并保留隐藏成员；全选覆盖当前筛选结果；“测延迟”复用真实/api/tests，仅提交已启用无中转错误的已选节点，要求内核运行且配置已应用；分组列表167px内滚；弹窗间距、行高与提示块压缩。
- 验证：39/39工具单测、严格TS、生产Web构建、40/40隔离浏览器（含筛选/排序仅显示、真实检测请求体、四行滚动）、完整test:ui与verify.mjs fixture通过；Go全量未因本轮纯前端改动重跑。未跑真实节点连通；2026-11-01功能提交8c047c0已推送main，随后作为v0.2.4发布并部署。
- 10-22 18:08观察用户服务为PID17124+内核22212、9090保持，本轮未触碰。提交/推送、发布与部署均已获授权并完成。

## M20 — v0.2.4 发布与部署（2026-11-01，已完成）

- 用户明确要求发布新 Release 并部署替换运行中的服务；采用补丁版 0.2.4，范围仅包含已提交的 M19 前端改动、版本/文档、发布与本机部署。
- 版本/文档提交 72da273；main CI41272387153、标签CI41272518085、Release41272518132 全部成功；v0.2.4 于12:18:53发布并设Latest，4程序+SHA256SUMS与清单/GitHub digest一致。
- 正式Windows程序0.2.4/72da273b5b81/built 2026-11-01T04:17:59Z；全部TestBuilt4.093秒、真实控制台WM_CLOSE两轮退出0/重启及失败窗口退出1通过。
- 部署：12:20前核对旧服务PID10920/内核28120、9090与32节点等基线；调用runtime/stop停内核后以受控控制台中断优雅退出，确认WAL/SHM清理完成；停机备份C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4（5文件62,960,192字节）逐文件SHA256一致。
- 12:23:29启动官方v0.2.4：PID16568/新内核28844，9090归16568，health0.2.4，32节点/3分组/3池/6监听，内核1.19.32且dirty=false；16:32复核仍运行。详情见[发布与部署验收](<release-0.2.4-verification.md>)。

## M21 — WSL 实机运行验收（2026-11-01，已完成）

- 用户要求“使用wsl来做实际运行测试”；目标是补 Linux 实机运行缺口，不触碰 Windows 运行服务、用户数据或外部节点。
- 环境：Fedora Linux 44 (WSL2)，内核 6.18.54.1-microsoft-standard-WSL2，x86_64，16 CPU/7.4GiB；官方 Linux amd64 v0.2.4 程序（SHA256 c56a73ae…）+ 固定 Mihomo 1.19.32 官方 linux-amd64-compatible（归档 SHA256 ba3ce607…，解压后 7a0d59da…）。
- 实跑结果：健康/UI+CSP、固定内核起停、自建回环 HTTP 与 SOCKS5 真实转发、runtime stop 与端口释放、SIGTERM 后 WAL/SHM 清理、同数据目录重启自动应用、healthcheck、端口冲突/公网绑定保护、管理员 Token 401/200 均通过。测试源和节点均为本机回环合成数据。
- 初次验收发现：WSL 冷启动首次 apply 可能因 750ms 私有认证 probe 超时失败，重试恢复；`kill -9` 主程序会遗留 Mihomo 内核（当时 processguard 仅 Windows），手动 SIGINT 可清理，重启可从 WAL 恢复数据与流量。该两项已由 M22 在本地修复并复验，正式 v0.2.4 资产不含该修复。
- 边界：仅 WSL2/Fedora/amd64；Linux arm64、裸机/其他发行版、Docker、systemd、UDP、并发/容量/吞吐和真实远端节点未测。详见[WSL 实机验收](<wsl-runtime-verification.md>)。

## M22 — Linux 实机问题修复（2026-11-01 完成，随 v0.2.5 发布）

- 用户要求“修复Linux测试中遇到的问题”；范围为 M21 发现的两项：750ms 私有认证 probe 偶发超时、Linux `kill -9` 后 Mihomo 子进程遗留。
- 实现：[processguard](<../internal/processguard/processguard.go>)新增 `PrepareCommand`，Linux 在 `SysProcAttr == nil` 时设置 `Pdeathsig = syscall.SIGTERM`（best-effort，非 Windows Job 等价）；[manager.go](<../internal/core/manager.go>)在版本探测、配置校验、启动三处调用。[controller.go](<../internal/core/controller.go>)把单次 750ms 探针检查改为 3 秒有界重试（1 秒尝试/100ms 间隔），并对 `/proxies` 原生出站暴露检查增加同样的有界重试；超期仍失败并停止内核。
- 测试：新增 [controller_verify_test.go](<../internal/core/controller_verify_test.go>) 的 `verifyProxies` 空映射后重试、1200ms 延迟绑定/父 deadline 用例和 [processguard_linux_test.go](<../internal/processguard/processguard_linux_test.go>) 的 SIGTERM/父进程强杀用例；Windows/WSL 定向与全量 Go、vet、gofmt、diff 检查通过。
- 实跑：最终本地构建 `.local/linux-fix/uniproxy-linux-amd64`（13,656,224 字节，SHA256 `8ff1916249b5cd6486cb3921b11aa253f1490b4110c4630b20f19460b057f2dc`）在 WSL2/Fedora 44 使用固定 Mihomo 1.19.32：首次 apply 直接 HTTP 200，无 probe 重试；回环 HTTP/SOCKS 通过；`kill -9` 后 `CORE_AFTER_KILL9_ALIVE=no`、端口释放，同目录重启恢复，最终正常停止 `FINAL_CORE=NONE`。
- 边界：仅源码/候选和 WSL2 amd64 验证；已提交 main（`ede12bb`）并随 v0.2.5 发布，正式 v0.2.4 资产不含修复；Linux arm64/裸机/Docker/systemd/容量与真实远端节点未测。详见[WSL 实机运行与修复验收](<wsl-runtime-verification.md>)。

## M23 — 全局出站网卡选择（2026-11-01 完成，随 v0.2.5 发布）

- 用户要求“测试全局出站接口方案，提供选择物理网卡和 wlan 等网卡的方式”。范围为管理员全局设置 + 只读网卡枚举 + 设置页选择器，解决 Clash Verge TUN 经 `Meta` 适配器接管默认路由、导致 UniProxy 检测全部超时的环境问题。
- 模型/编译：[types.go](<../internal/model/types.go>) 新增 `Settings.OutboundInterface`（空=自动）；[compiler.go](<../internal/core/compiler.go>) 在非空时写入顶层 `interface-name`，节点级 `interface-name` 仍由 `safeNativeValue` 拒绝。
- 校验/接口：[validate.go](<../internal/app/validate.go>) 用 `net.InterfaceByName` 要求存在、`FlagUp`、非 `FlagLoopback`，并拒绝首尾空白；[interfaces.go](<../internal/app/interfaces.go>) 提供只读 `GET /api/interfaces`（name/up/loopback/addresses，过滤链路本地，不含网关/MAC/DNS）；`PATCH /api/settings` 白名单新增字段。
- 前端：设置页新增「03 全局出站网络」，默认「自动（系统默认路由）」，列出启用且非回环网卡，支持刷新；当前值不可用时保留并标注，说明不创建虚拟网卡、不改系统路由。
- 测试：`internal/app/outbound_interface_test.go`（校验矩阵、PATCH 往返与拒绝、接口结构）、`internal/core/compiler_interface_test.go`（空值不输出/非空输出/去空白/节点级仍拒绝）、`internal/core/interface_real_test.go`（真实 Mihomo 对未知网卡也不报错，证明必须管理层校验）、`tests/browser/outbound-interface.spec.ts`（下拉选项与只读枚举逐项一致、真实网卡保存后持久化并可改回自动、不存在网卡 PATCH 400 且不落库）全部通过；fixture UI 的 `verify.mjs` 也覆盖了网卡消失后的「当前不可用」标注与陈旧值不被静默改写。
- 真实出站实测：用 `core.Build` 生成配置并改写端口后起临时 Mihomo，auto/以太网/WLAN/Meta 均 204；`Get-NetTCPConnection` 观察到以太网与 WLAN 出站源地址分别是各自网卡地址，证明绑定生效；不存在的网卡启动成功但拨号失败（`curl 35/000`，日志伪装成 DNS 失败），不回退直连。
- 边界：已提交 main（`a4dd45d`）并随 v0.2.5 发布；正式 v0.2.4 资产不含该功能；只约束自有内核出站，不处理系统路由/DNS，不等价于 TUN 分流；Linux/macOS 网卡名与真实远端节点连通未在本轮验证。
- 发布与部署：v0.2.5 已发布但未部署；2026-11-02 16:01 发布验收时本机仍运行用户自行启动的 v0.2.5（主程序 PID29920 + 内核 PID11688），本轮未停止/替换，部署需另行授权。详见[v0.2.5 发布验收](<release-0.2.5-verification.md>)。

## M24 — 现代 VLESS/VMess 协议兼容（2026-11-02 完成，已随 v0.2.6 发布）

- 用户反馈“社区用户称 uniproxy 无法识别最新版的 VLESS 和 VMess 协议”，要求深入检查并修复。范围限定为导入器解析/规范化和测试；不改 API、数据库 schema、内核版本或运行服务。
- 调研：对照 v2rayN master 的 `BaseFmt`/`VmessFmt`/`V2rayOutboundService` 与固定 Mihomo 1.19.32 出站源码；确认 `mihomo -t` 不校验 transport 名称，未知 network 会静默走 TCP，因此不能只靠 `-t`。
- 实现：新增 [modern.go](<../internal/importer/modern.go>) 与 `modern_test.go`，扩展 [uri.go](<../internal/importer/uri.go>)、[transport.go](<../internal/importer/transport.go>)、[native.go](<../internal/importer/native.go>)、[vmess.go](<../internal/importer/vmess.go>)：VLESS XHTTP（Xray extra/download/xmux）、HTTPUpgrade→WS `v2ray-http-upgrade`、VMess KCP、ECH、`vcn`/`pcs`、VLESS Encryption、URI 别名与大小写归一；VMess JSON `insecure`/`raw` 兼容。
- 明确拒绝：VMess XHTTP（VMess 适配器无 XHTTPOpts）、`spx`（非空且非 `/`）、`pqv`、`fm`、`flow=xtls-rprx-vision-udp443`（固定内核截断为 Vision）、`noSSEHeader`/`scMaxBufferedPosts`/`scStreamUpServerSecs`/`serverMaxHeaderBytes`/`h2Params` 等无固定内核等价字段；不静默降级、不改变 TLS/证书校验。
- 验证：`go test -mod=readonly -p 1 ./... -count=1`（显式 `UNIPROXY_TEST_CORE_PATH=C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe`）全通过（cmd0.120s、app9.121s、core8.805s、coreinstall0.482s、importer0.751s、integration4.069s 等）；`go vet ./...`、`gofmt -l internal cmd tests`、`FuzzParseNeverPanics` 15s/246481 execs 通过；`bin/mihomo.exe -t -f` 对 VLESS XHTTP+VMess KCP+VLESS ECH 合成配置报成功。
- 发布：源码提交 `dba132cb379fe113b897b787fef3a94ab91b8bef`；main CI `41409672766`、tag CI `41409873967`、Release workflow `41409873947` 全部成功；`v0.2.6` 于 2026-11-02 16:00:55 +08 发布并设 Latest，四平台资产与 SHA256SUMS 已核验。
- 边界：已随 v0.2.6 发布（不含部署）；用户运行实例未触碰；未做真实远端节点连通/吞吐测试；固定 Mihomo 无等价字段按错误处理。详见[现代协议验证](<modern-protocol-verification.md>)。

## M25 — 链式代理（2026-11-02 完成并随 v0.2.7 发布，不含部署）

- 用户批准的设计：独立「代理链」页；创建窗口左侧分组节点库、右侧有序代理链配置；打开时对可见可测节点自动测一次延迟，点击延迟可重测；入口监听可折叠并在同一事务创建。范围包含模型/存储/校验、编译器、检测、API、前端与测试，不改变节点协议、订阅解析、内核版本或系统网络。
- 模型/API：新增 `Chain{ID,Name,NodeIDs,Enabled,CreatedAt,UpdatedAt}` 与 `State.Chains`；新增 `GET/POST /api/chains`、`PUT/DELETE /api/chains/{id}`、`POST /api/chains/with-listener`、`POST /api/chains/{id}/test`；监听 `kind` 增加 `chain`，目标为链 ID；入口复用池监听的协议/地址/端口/认证/UDP 校验和自动端口分配。
- 编译/运行：每条链生成 `c-<chainID>-<index>` 合成 outbound 副本，第二跳起设置 `dialer-proxy` 指向前一跳，监听只绑定末跳；任一跳缺失/禁用/中转异常、跳数越界、重复 hop 或 nativeProxy 构造失败时整链不生成并让入口 REJECT，不缩短不回退直连。`compiledConfig.chainTags` 记录链→末跳 tag，`chainHops` 记录全部 hop tag；`verifyProxies` 逐一检查每一跳真实暴露；`Manager.CheckChain` 调用内核 delay 接口测试整链，不写节点健康。
- 删除/校验：删除节点会停用包含该节点的整链及其 chain 监听，保留 NodeIDs 便于修复；启用链要求 2–16 跳、节点唯一且存在、每跳启用且无 DialerID/RelayErrors；UDP 入口要求每跳节点支持 UDP。
- 前端：新增 [ChainsPage](<../src/web/ChainsPage.tsx>) 与 [ChainDialog](<../src/web/ChainDialog.tsx>)，独立导航「代理链」；双栏节点库/链路配置、分组筛选/状态筛选/排序/搜索/真实延迟、链路顺序上移/下移/移除、入口/出口标签、少于 2 跳提示、清空确认、创建/编辑/复制/启停/删除/整链测试；监听页目标类型增加代理链。
- 测试：新增 `internal/app/chain_*_test.go`、`internal/core/compiler_chain_test.go`、`internal/core/chain_real_test.go`、`src/web/chainSelection.ts` 纯函数测试、`tests/browser/chain-workflow.spec.ts`（5 例）。真实内核测试用两个回环 SOCKS 上游与本地 HTTP origin，确认两跳都收到连接；独立审查后补上删除节点/UDP 事务、整链检测失败 HTTP 200、重复 hop fail-closed、停用监听保存失效链等回归；本机全量 Go、严格 TS、生产 Web 构建、UI 工具（42/42）、链浏览器 5/5 与全量浏览器 48/48 通过。
- 本地预览构建（2026-11-02 19:56）：`bin/updates/chain-preview/uniproxy-windows-amd64.exe`，14,205,440 字节，SHA256 `5c8705f9451b9cceb1a7d62df738b3a3c04b96b1268d88c16a950cfced67e148`，身份 `0.2.6-chain-preview.1 · commit local-chain- · built 2026-11-02T11:56:52Z`；隔离 19091 启动/health/嵌入首页通过，针对该 exe 的 TestBuilt 集成测试全部 PASS（1.518s）；说明见 `bin/updates/chain-preview/使用说明.md`。仍为本地预览，不是发布资产。
- 边界：代理链只支持本应用自有内核的 TCP 入口；UDP 要求每一跳节点支持；不承诺真实公网远端链路性能/可用性；未做多平台实跑、Docker、race 或容量测试。本版已随 v0.2.7 发布，不部署，用户运行实例未触碰。
- 运行排障（2026-11-02 20:52）：9router 代理池 20006 的 `JP-US` 链曾报 ECONNRESET；隔离进程验证为第一跳台湾节点无法承载到第二跳的嵌套 TLS/Reality（美国直连 735ms、日本→美国 1967ms 成功、台湾→美国 8s 超时，去掉 Vision 仍失败），非编译器问题。通过 API 将第一跳改为日本原生01 后，链测试 3/3 通过（1404/1882/1444ms），20006 HTTP/SOCKS 204，9router `JP-US1` 20:49:07 测试成功并 active。只改运行配置，未改源码/数据库。
- 发布（2026-11-02 21:52:52 +08）：源码提交 `14cc9f570b44422b9dfc1deec94b3556749a1a05`，annotated tag `v0.2.7`（对象 `568f8639935eef3cbdd9090b19ff4d2b0dde53da`）；main CI `41456237334`、tag CI `41456604601`、Release `41456604526` 全部成功；四平台资产与 SHA256SUMS 已核验；正式 Windows 程序 `UniProxy 0.2.7 · commit 14cc9f570b44 · built 2026-11-02T13:51:54Z`，TestBuilt 4.385s 与真实控制台通过。发布不含部署，用户预览实例未触碰。

## M26 — 新建代理链弹窗布局调整（2026-11-03 完成，已随 v0.2.8 发布，不含部署）

- 用户要求：把入口监听从原折叠区移到代理链名称下方、节点库上方；访问认证保留最下面；去掉不必要的二级标签和说明；端口留空自动分配，也可手工填写；节点库左侧至少显示三排三列节点并尽量多显示。
- 实现：`src/web/ChainDialog.tsx` 删除 `automaticPort` 状态，端口改为字符串输入；保存时空白提交 `port: 0`，手工端口校验 1024–65535 并给出就近错误。入口监听改为单行五列（名称/协议/地址/端口/UDP），块头只保留“入口监听 + 创建”开关；`CredentialsEditor` 移到 `chain-layout` 之后、fieldset 末尾；删除节点库“点击卡片按顺序加入右侧链路”等冗余提示。
- 样式：`src/web/chainStyles.css` 新增入口监听面板、五列网格和窄屏降级；`.chain-node-grid` 桌面固定三列，配合卡片/网格尺寸可在 1440×1000 弹窗内显示 3×3 共 9 个节点；弹窗头部、正文、字段间距进一步压缩，访问认证仍可见，底部 footer 保留。
- 测试：`tests/browser/chain-workflow.spec.ts` 增加入口监听位于节点库上方、认证位于底部、9 节点 3×3、空白端口提交 0、手工端口透传、非法端口本地拦截等断言；原有 6 个链工作流用例全部保留。
- 验证：`pnpm exec tsc --noEmit --noUnusedLocals --noUnusedParameters` 通过；`pnpm run build:web` 通过（Vite 7.3.7，4654 modules；最终 CSS 84.99kB/gzip17.13，JS 556.44kB/gzip162.16，仅有 chunk 体积提示）；`pnpm exec playwright test tests/browser/chain-workflow.spec.ts` 6/6 通过（12.8s）；临时视觉截图 `.local/chain-preview/chain-dialog-new.png` 与 `chain-dialog-new-390.png` 用于检查，临时 spec 交付前删除。
- 边界：仅前端布局和交互，不改监听/代理链 API、模型、数据库 schema、Mihomo 版本或安全策略；已提交并推送 main（`607dc8b589836aa0a58782c41b8f5559617257d1`，main CI 41487587509 成功），用户运行实例未触碰，并已随 v0.2.8 发布（不含部署）。详见 [CHANGELOG](<../CHANGELOG.md>)、[需求 R31](<requirements.md>)、[决策 D29](<decisions.md>)。

## M27 — 全局界面视觉刷新（2026-11-03 完成，已随 v0.2.8 发布，不含部署）

- 用户要求整体优化 UniProxy Web UI：避免不必要的二级标签和说明，让布局表达层级，界面更紧凑、简洁明了，亮色主题并提升完成度；参考外部项目截图但以 UniProxy 自身风格为主。范围为全部管理控制台页面、共享组件、弹窗、认证与通知，不改变业务 API/模型/数据库 schema/Mihomo 版本/代理安全策略。
- 实现：新增 [theme.css](<../src/web/theme.css>)，在 [main.tsx](<../src/web/main.tsx>) 第 6–9 行最后加载；采用暖纸亮色画布、深森林墨文字、青绿/酸橙强调、serif 大标题与深色首张统计卡，统一侧栏、顶栏、页面标题、统计、表格、按钮、表单、表格、弹窗、池/链卡片、设置、空态、认证与通知。隐藏重复的 eyebrow、页面/区块 description、subtle-meta、footnote、分组提示、context banner、侧栏说明、面包屑和页脚辅助文案；保留安全提示、真实状态、空态和错误。
- 密度与响应式：压缩表格行高、卡片/弹窗间距；活动列表 396px 内滚；390px 底部导航改为 62px 七列单行布局，页面无水平溢出；[App.tsx](<../src/web/App.tsx>) 的登录错误仅在用户尝试后显示，避免首次打开显示红色 401。
- 验证：严格 TypeScript、生产 Web 构建（Vite 7.3.7，4655 modules；CSS 106.42kB/gzip22.24，JS 556.48kB/gzip162.18，仅 chunk 提示）、pnpm test:ui 42/42、pnpm test:e2e 49/49（1.2m）通过；视觉截图见 .local/ui-theme/（nodes/groups/listeners/subscriptions/pools/chains/settings/nodes-mobile/import/pool/chain/auth 等），本地 Windows 预览 bin/updates/ui-refresh/uniproxy-windows-amd64.exe（14,227,456 bytes，SHA256 54BFEBCD087DA9D05EF395785F4CC77BD54CF45AB0B2A4338D7BC36CFCFC38B8，0.2.8-ui-preview.1）。
- 边界：仅前端样式和登录错误显示时机；未改 API/模型/schema/Mihomo/安全策略。截图与浏览器测试使用隔离 fixture 和 headless Edge 1440/390；未覆盖 Safari/Firefox、真实移动设备和全部缩放组合。已随 v0.2.8 发布（不含部署），用户运行实例未触碰。详见[验收](<ui-refresh-verification.md>)、[需求 R32](<requirements.md>)、[决策 D30](<decisions.md>)。

## M28 — 新建/编辑代理池布局与节点批量栏（2026-11-03 完成，已随 v0.2.8 发布，不含部署）

- 用户要求继续压缩新建/编辑代理池弹窗：监听区上移，包含分组固定约四行并可滚动，包含节点区域更宽更高，减少二级说明；节点管理批量操作栏保持可见。用户随后撤回“添加/移除暂存按钮”，最终交互维持即时勾选、状态筛选、名称/延迟/速度/更新时间排序与真实延迟检测。
- 实现：只在新建设置中把监听面板放到名称/策略之后、成员选择之前，协议/地址/端口/自动分配/UDP 单行四列；成员区改为分组选择与匹配方式左右分栏、节点列表占满整行并显示协议/UDP/健康/地址。编辑模式保留原监听配置，不在弹窗内重复编辑。
- 样式与组件：`src/web/poolStyles.css` 分组列表固定 177px 内滚（36px 行高、5px 间隔、16px 内边距正好四行），节点列表最高 420px，桌面弹窗 1060px，900/760/560px 逐级降级；`src/web/ui.tsx` 的 `ChoiceList` 增加受控 `columns` 与 `renderOption`；`src/web/styles.css` 增加通用 `.choice-list--columns`。节点管理批量栏保持常驻，未选择时按钮禁用。
- 测试：13 组/9 节点覆盖新建与编辑弹窗、四整行分组滚动、监听区在成员区上方、节点区≥400px、390px 无溢出；新增 `tests/browser/nodes-batch.spec.ts` 锁定批量栏常驻/禁用/选中状态。`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.3m）、生产 Web 构建（Vite 7.3.7，4655 modules；CSS 110.00kB/gzip22.85，JS 556.35kB/gzip161.96，仅 chunk 提示）通过；聚焦 17/17（30.6s）。
- 边界：仅前端布局、组件属性和浏览器测试；未改代理池/监听 API、模型、SQLite schema、Mihomo 或代理安全策略。浏览器使用隔离 fixture/headless Edge，未覆盖 Safari/Firefox、真实移动设备、全部缩放或真实节点连通；已随 v0.2.8 发布（不含部署）。详见[验收](<pool-layout-verification.md>)、[需求 R33](<requirements.md>)、[决策 D31](<decisions.md>)。

## M29 — 代理池卡片紧凑化与出口节点双列（2026-11-03 完成，已随 v0.2.8 发布，不含部署）

- 用户要求代理池卡片 UI 布局优化、提高空间利用率：出口节点卡片设计更紧凑，桌面分两列展示，提高同屏节点数量；并按“避免不必要的二级标签和说明，让布局来传达说明”的原则精简节点卡。
- 实现：`src/web/PoolCard.tsx:18` 每页 10 个节点；节点卡首行为健康、名称、延迟/状态、测延迟与移除，次行仅协议 Badge，地址/中转/最近检测/错误收进可展开详情，删除“连接信息”重复标签。`src/web/poolStyles.css:121-143` 使用 `container-type:inline-size` 的成员区容器查询，宽度>430px 两列、≤430px 单列；列表 380px 内滚，桌面操作按钮 24×24，`pointer:coarse` 恢复 44×44。`src/web/theme.css` 收紧池卡片内边距并统一列表高度。
- 测试：`tests/browser/pool-compact.spec.ts` 覆盖每页 10、第二页 nodes[10]、两列/单列、首两卡同高、第三卡下移；`src/web/tests/pool-regressions.mjs` 同步 10+4 分页数据。生产 Web 构建（Vite7.3.7，4655 modules；CSS 110.64kB/gzip22.97，JS 556.38kB/gzip161.96）、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52（1.4m）、聚焦 pool 17/17（29.3s）、pool-compact 2/2（6.5s）通过。
- 边界：仅前端组件、样式和浏览器测试；未改代理池/监听 API、模型、SQLite schema、Mihomo 或安全策略。浏览器使用隔离 fixture/headless Edge，未覆盖 Safari/Firefox、真实移动设备、全部缩放或真实节点连通；已随 v0.2.8 发布（不含部署）。详见[验收](<pool-card-verification.md>)、[需求 R34](<requirements.md>)、[决策 D32](<decisions.md>)。

## M30 — 协议适配三档（2026-11-03 完成，已随 v0.2.9 发布，不含部署）

- 用户要求：按严格 native 映射、宽松解析预览/degraded、真实可用性验证三档实现协议适配。
- 实现：`internal/model/types.go` 增加 `ParsedNode.Degraded/DegradedReasons`、`ParseResult.Degraded`、`ImportResult.Degraded`；`internal/importer/parser.go` 新增 `ParsePreview`，严格 `Parse` 不变；新增 `internal/importer/preview.go` 做 URI 级预览归一；`internal/app/imports.go` 使用 `ParsePreview` 并分离预览/提交；`src/shared/types.ts`、`src/web/ImportDialog.tsx` 增加“可导入/仅预览”分区和降级原因表。
- 严格修复：`security=false`、`packetEncoding=none`、`type=none`、`extra=null`、空 `fp`、`fp=ios`、WS `headerType=none`、TCP path/host 占位、VLESS Encryption 1184 字节 key。
- 真实订阅：1064 条；严格 697（65.5%）；宽松预览 697+365=1062（99.8%），2 条未恢复（1 invalid UUID、1 unknown exporter combination）；预览错误仍如实显示，不强行吞掉。
- 验证：`gofmt`、`go vet ./...`、`go test ./... -count=1`、`pnpm typecheck`、`pnpm test:ui` 42/42、`pnpm build:web`、`pnpm test:e2e` 52/52（1.2m）通过；新增 `internal/importer/preview_test.go`、`internal/app/preview_import_test.go`。
- 边界：预览不等于可应用；`Degraded` 永不进入 `addParsed`/SQLite/编译器；订阅提交仍 `TotalErrors == 0`；真实节点连通性/吞吐/握手未验证。详见[验证](<protocol-adaptation-tiers.md>)、[需求 R35](<requirements.md>)、[决策 D33](<decisions.md>)。
- 发布（2026-11-03 16:22:39 +08）：提交 `6ff15a8c2a0fb44e3dbc14c4e024b1f7c6b4d54a`，annotated tag `v0.2.9` 对象 `4d05181f557445f5f6325bd57e3821d138c981a5`；main CI `41602628999`、tag CI `41602671353`、Release `41602671127` 均成功；四平台资产与正式 Windows 程序验收完成；只发布，不含部署。

## 运行观察 — 菜鸟 25 节点延迟（2026-11-01，仅诊断）

- 用户文件 `work/cainiao-pc/cainiao-pc-clash-25nodes.yaml` 导入后 25 节点全部 unhealthy；导出字段与源文件一致，TLS/WS 参数未被丢弃。
- 主机 Clash Verge TUN 默认路由经 `198.18.0.2`/Meta，UniProxy Mihomo 出站继承该路由；绑定物理以太网 `192.168.2.3` 后代表节点 TLS/WS 正常，同版本 Mihomo 对照绑定 `interface-name: 以太网` 延迟 521ms、默认路由 Timeout。
- 源文件 25 节点临时绑定测试 22/25 成功（约 371–938ms），香港10、美国04、台湾02 仍 Timeout；当前全失败主因是 Clash Verge TUN 路由冲突，不是 UniProxy 导入错误。服务 PID16568/内核28844 未改动。

## 3. 按优先级排列的剩余工作

以下是建议顺序，不是用户已授权的新增开发任务。

| ID | 优先级/状态 | 工作与依赖 | 完成标准 |
|---|---|---|---|
| P0-01 | 已完成 / 2026-11-01 | v0.2.4已部署替换原v0.2.3实例；新主程序PID16568（bin/releases/v0.2.4/uniproxy-windows-amd64.exe）/内核28844，原数据目录保留，停机备份已校验 | 16:32复核health0.2.4、32节点/3分组/3池/6监听、内核运行；后续用户侧真实节点检测另记 |
| P0-02 | 高 / 待授权实测 | 升级后用户侧重导入原文件；如需实际节点连通测试，另确认网络请求和流量范围 | 导入依赖不丢失、原节点去重；可用/不可用按真实检测记录，不把离线校验当连通 |
| P1-01 | 中 / 部分完成（WSL2 amd64） | Linux amd64 已在 WSL2 实机验证启动、认证 TCP、回环 HTTP/SOCKS、持久化重启、停机恢复；仍待裸机/其他发行版、Linux arm64、Docker bridge/host、host 回环 UDP、端口映射和系统服务验证 | 已有记录见[WSL 实机验收](<wsl-runtime-verification.md>)；其余范围需对应主机/Docker 再做 |
| P1-02 | 中 / 待验证 | Go race 检测，需 C 工具链/受支持环境 | 并发应用、检测取消、订阅刷新/恢复通过 race；不得用普通单测替代 |
| P1-03 | 中 / 待验证 | 目标硬件容量测试，使用受控节点/流量预算 | 500 节点管理、实际连接并发、UDP 流量、p95/CPU/RSS/错误率/长期运行报告 |
| P1-04 | 中 / 部分完成 | 发布流程已实现并跑通（v0.2.0 已发布）；仍需项目所有者确定许可证 | 项目所有者确定许可证；符合内核 GPL 分发义务；版本注入已可区分构建，发布需真实推送标签 |
| P1-05 | 中 / 候选 | CI 增加 UI 浏览器及固定内核集成验证 | 保留现有 Windows/Linux job；额外测试在 CI 真实运行，下载内核校验摘要、不使用用户密钥 |
| P2-01 | 低 / 修复已发布，待部署 | M22 已为 Linux 增加 `PrepareCommand`/`Pdeathsig` best-effort 保护并在 WSL2 复验，随 v0.2.5 发布；v0.2.4 资产无此修复。自动重启/Windows常驻/优雅更新仍候选，Linux 仅 best-effort 非 Job 等价 | 先定策略与故障边界；不得重启不属于本应用的进程或自动采用旧内核；部署需另行授权 |
| P2-02 | 低 / 候选 | dialer-proxy 策略组、更多配置变体、认证公网 UDP | 独立设计+需求确认+安全审查；保持不降级、不直连回退 |
| P2-03 | 低 / 候选 | 独立概览/连接流量统计等增强 | 先确认指标来源/保留策略和界面范围，不以模拟数据交付 |

没有“因为会话恢复所以立即重启服务”的任务。新的人类请求优先于本列表；要改变范围，先记入[需求](<requirements.md>)和[决策](<decisions.md>)。

## 4. 每项开发的固定流程

1. 读[接续记录](<session-handoff.md>)，检查文件、进程和真实部署状态；检查工作区有无新增规则。
2. 写明需求 ID、作用范围、数据/协议兼容影响、验收和不做什么。
3. 涉及共享数据结构，先调整[Go 模型](<../internal/model/types.go>)和[前端类型](<../src/shared/types.ts>)，与实现/契约同步。
4. 添加最小复现/回归测试，实施改动；并行工作按目录划分，禁止交叉全局格式化。
5. 分层测试：纯逻辑 → API → 实际内核 → 浏览器。先收集所有相关后台任务结果，再宣布完成。
6. 构建到独立路径；有活动用户进程时不运行会覆盖原 exe 的全量构建/安装。
7. 回写状态/决策/变更/验收；部署另列为需要用户批准的操作，不把构建完成写成已上线。

## 5. 完成定义

- 没有未解释的测试失败，失败原因/限制明确；新增安全边界有回归测试。
- 没有示例密钥、用户订阅或原始节点文件进入源码/文档/日志。
- 期望/运行配置区分正确；中转及分组来源不因编辑、刷新、备份而丢失。
- 用实际构建验证；仅前端测试不证明代理可用，仅 cross-build 不证明部署成功。
- [会话快照](<session-handoff.md>)、本计划、[变更记录](<../CHANGELOG.md>)已更新；接续者无需读完整聊天即可行动。

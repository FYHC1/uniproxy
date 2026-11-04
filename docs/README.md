# UniProxy 文档索引

更新：2026-11-04。**入口先读接续快照，再按任务读取需求、计划和开发指南。** 这些文件保存在项目中，不依赖会话摘要；仍须按每次收尾规则维护。

## 最短接续路径

1. [session-handoff.md — 当前进度/运行与暂存构建/下一步](<session-handoff.md>)
2. [development-plan.md — 开发里程碑和待办](<development-plan.md>)
3. [requirements.md — 已确认需求及边界](<requirements.md>)
4. [decisions.md — 为什么这样设计、哪些不能随意更改](<decisions.md>)
5. [development-guide.md — 代码地图、构建测试、升级和回滚](<development-guide.md>)

根目录[AGENTS.md](<../AGENTS.md>)指向以上入口，供开发者/代理在新会话读取。[用户README](<../README.md>)保留启动与使用说明。

## 文档责任划分

| 文档 | 用途 | 性质/维护时机 |
|---|---|---|
| [需求基线](<requirements.md>) | 场景、R01–R36、接受的行为/限制、不做什么 | 用户确认/范围改变时更新 |
| [开发计划](<development-plan.md>) | M0–M31、P0/P1/P2待办、完成定义 | 每次任务阶段更新；候选不等于获准 |
| [开发指南](<development-guide.md>) | 源码模块、环境、数据规则、构建/测试、停机升级 | 工程操作或模块接口改变时更新 |
| [技术决策](<decisions.md>) | D01–D33、原因与后果、替代旧方案 | 新决策追加或标注被取代，保留历史 |
| [接续快照](<session-handoff.md>) | 一页恢复上下文，时间敏感运行状态和下一步 | 每轮收尾必更新，PID/哈希必须注明日期 |
| [架构](<architecture.md>) | 控制/转发面、核心对象、生命周期、安全边界 | 实现架构变动时更新 |
| [接口契约](<implementation-contract.md>) | Go/TS模型、API形状、内核适配、校验范围 | 字段/接口变动时同步源码测试 |
| [API示例](<api.md>) | 自动化调用与返回语义 | 用户可用接口变更时更新 |
| [测试指南](<testing.md>) | 分层验收方法、命令、未验证边界 | 测试入口/环境改变时更新 |
| [首版历史验收](<verification.md>) | 首版已运行的验证及当时5/9测试口径 | 历史记录，不改成后来重测的证据 |
| [中转修复验收](<relay-fix-verification.md>) | 328/178/325离线结果、7/21测试、待部署修复 | 本次修复交付证据；升级事实另记快照 |
| [代理池优化验收](<pool-workflow-verification.md>) | R16–R18、20/30测试、原子监听/池内移除/卡片及构建边界 | 本轮完成证据；部署另经确认 |
| [中转变更设计](<relay-change-contract.md>) | 图身份、截断、拖放、过滤的完成版设计 | 当前实现补充，不是活动子任务命令 |
| [数据目录与发布验收](<data-release-verification.md>) | 平台标准数据目录、旧目录沿用、版本注入、标签发布工作流 | 本轮完成证据；Release 需真实推送标签 |
| [入库与全面复查验收](<repository-review-verification.md>) | 首次入库、复查范围、7 项缺陷修复、32/32 浏览器与真实内核回归 | 本轮完成证据；明确未部署 |
| [订阅与分组验收](<subscription-groups-verification.md>) | Fake-IP原因/安全复核、紧凑池、组内管理，32/31验证与待部署界限 | M10完成证据；不宣称私有订阅全文已测 |
| [内置内核安装验收](<core-install-verification.md>) | R23内置下载/API/UI/并发、40安全测试/36工具/34浏览器、真实临时安装、四平台暂存 | M13开发交付时证据；随后随v0.2.1发布，未部署，平台限制明确 |
| [v0.2.1发布验收](<release-0.2.1-verification.md>) | 已推送源码/tag、CI/Release成功、四平台资产摘要与Windows运行验证 | M14完成证据，发布不等于部署 |
| [Windows启动与恢复验收](<windows-startup-verification.md>) | R24 malformed根因、授权离线恢复、错误可见性与Job保护、真实控制台/重启验收 | M15开发历史，后续修复已随v0.2.2发布，不等于部署 |
| [v0.2.2发布验收](<release-0.2.2-verification.md>) | 启动诊断/Windows进程保护正式发布、源码/CI/资产与程序核验 | M16已发布并验证，部署另行授权 |
| [订阅格式兼容验收](<subscription-format-verification.md>) | R25 TLS默认字段兼容、QX/Surge/Surfboard/Loon静态节点、授权样例与回归 | M17开发验收；源码/暂存不等于已发布或部署 |
| [v0.2.3发布验收](<release-0.2.3-verification.md>) | 订阅解析兼容正式发布、源码/CI/四平台资产与Windows程序核验 | M18已发布；资产/程序验收及未部署边界见记录 |
| [v0.2.4发布与部署验收](<release-0.2.4-verification.md>) | M19正式发布、四平台资产与Windows程序核验，以及本机部署替换、一致备份和运行复核 | M20已发布并部署；部署事实、备份与验证边界见记录 |
| [v0.2.5发布验收](<release-0.2.5-verification.md>) | M22/M23（Linux 稳定性、全局出站网卡）正式发布、四平台资产与Windows程序核验 | 2026-11-02 02:27:40已发布并设Latest；不含部署 |
| [v0.2.6发布验收](<release-0.2.6-verification.md>) | M24 现代 VLESS/VMess 兼容正式发布、四平台资产与 Windows 程序核验 | 2026-11-02 16:00:55已发布并设Latest；CI/资产已回填；不含部署 |
| [v0.2.7发布验收](<release-0.2.7-verification.md>) | M25 代理链正式发布、四平台资产与 Windows 程序核验 | 2026-11-02 21:52:52已发布并设Latest；CI/资产已回填；不含部署 |
| [v0.2.8发布验收](<release-0.2.8-verification.md>) | M26–M29 界面刷新与代理池体验正式发布、四平台资产与 Windows 程序核验 | 2026-11-03 10:28:28已发布并设Latest；CI/资产已回填；不含部署 |
| [v0.2.9发布验收](<release-0.2.9-verification.md>) | M30 协议适配三档正式发布、四平台资产与 Windows 程序核验 | 2026-11-03 16:22:39已发布并设Latest（不含部署） |
| [v0.3.0发布验收](<release-0.3.0-verification.md>) | M31 多引擎 sidecar、Xray 在线安装器与真实流程正式发布、四平台资产与 Windows 程序核验 | 2026-11-04 已发布；只发布，不部署 |
| [链式代理验收](<chain-verification.md>) | R30：有序链编译、整链检测、链/入口 API、编辑器交互、真实回环链路 | 2026-11-02 已随 v0.2.7 发布（不含部署） |
| [全局界面视觉刷新验收](<ui-refresh-verification.md>) | M27：theme.css 最后加载、亮色主题、布局表达层级、全部页面/弹窗/认证、42/42 UI 与 49/49 浏览器验证、本地预览 | 2026-11-03 已随 v0.2.8 发布（不含部署） |
| [代理池布局与节点批量栏验收](<pool-layout-verification.md>) | M28：监听区上移、分组固定四行滚动、节点区加宽加高、批量栏常驻、42/42 UI 与 52/52 浏览器验证 | 2026-11-03 已随 v0.2.8 发布（不含部署） |
| [代理池卡片紧凑化与双列验收](<pool-card-verification.md>) | M29：每页 10 节点、节点卡两层结构、容器查询双列/单列、触控按钮 44px、42/42 UI 与 52/52 浏览器验证 | 2026-11-03 已随 v0.2.8 发布（不含部署） |
| [现代VLESS/VMess协议验证](<modern-protocol-verification.md>) | M24：v2rayN/Mihomo 源码对照、XHTTP/HTTPUpgrade/VMess KCP/ECH/VLESS Encryption 映射、拒绝边界与本地测试 | 2026-11-02 已随 v0.2.6 发布（不含部署） |
| [协议适配三档验证](<protocol-adaptation-tiers.md>) | M30：严格 native、宽松预览 degraded、真实可用性三档；真实订阅 1064 条统计、修复项、测试与边界 | 2026-11-03 已随 v0.2.9 发布（不含部署） |
| [多引擎 sidecar 需求分析](<multi-engine-sidecar-requirements.md>) | M31：Mihomo 主核 + Xray/sing-box 双 sidecar 的需求、架构、能力矩阵、阶段计划、测试与风险 | 2026-11-04 第一至四批、真实流程、真实订阅与 Xray 在线安装器完成（模型/包/编译器/链校验/POC/端点状态 API/UI/端口预检/脱敏/Windows+WSL E2E/安装器与设置页）；真实 GitHub 资产下载与生产安装路径已验证，匿名 API 配额耗尽时 403；已提交 `00e3350`，未推送/未发布/未部署，Linux 实机安装、非 WSL 裸机仍待验证（真实远端节点已由 kr-vps E2E 覆盖） |
| [多引擎 sidecar 开发交接](<multi-engine-sidecar-handoff.md>) | M31：新对话入口、分支/目录、第一批任务、验收与边界 | 2026-11-04 第一批 1–5、第 6 项 app/API、UI、端口产品化、真实流程第 1–3 层、真实订阅与 Xray 在线安装器已完成并验证；真实 GitHub 资产下载已验证，Linux 实机安装、非 WSL 裸机待验证（真实远端节点已由 kr-vps E2E 覆盖） |
| [多引擎 sidecar 真实流程验证](<sidecar-real-process-verification.md>) | M31 真实流程第 1–4 层 + 安装器真实下载：Windows/WSL 三协议端到端、kr-vps 远端订阅、Phase 0、端口释放与 GitHub 资产安装 | 2026-11-04 开发分支证据；含真实远端与安装器真实下载，Linux 实机安装仍待验证 |
| [WSL实机运行与Linux修复验收](<wsl-runtime-verification.md>) | Linux amd64 WSL2 真实启动、固定内核、回环HTTP/SOCKS、持久化重启、认证与异常终止观察，以及探针重试/Pdeathsig 修复复验 | M21实机证据与M22修复；修复随v0.2.5发布；不替代裸机/ARM64/Docker/容量与真实远端节点 |
| [归档协作备注](<coordination.md>) | 早期并行划分和结束状态 | 归档，不用于判断剩余工作 |
| [CHANGELOG](<../CHANGELOG.md>) | 用户可见变化和交付节点 | 每个变更完成时追加；不等于已发布 |

### 模块细节

- [导入器兼容/测试](<../internal/importer/README.md>)
- [Mihomo内核与内置在线安装](<../internal/core/README.md>)
- [前端单元/fixture回归](<../src/web/tests/README.md>)
- [第三方许可证说明](<../THIRD_PARTY_NOTICES.md>)

## 权威性与冲突处理

- 最新用户确认和[需求基线](<requirements.md>)决定产品意图。
- 源码+实际测试决定实现事实；与文档冲突时报告并修复，不假定文档可覆盖代码。
- [快照](<session-handoff.md>)是带时间的运行观察，操作前重核对。
- 历史验收只证明当时执行过的范围，构建成功≠部署成功、解析成功≠远端可用、fixture UI≠内核互通。
- 初版Node/sing-box草案、旧“4/5浏览器测试”“UI待修”协作话语已失效；不要据此重开完成的任务。

## 文档维护清单

~~~text
[ ] 当前请求、完成项和下一步写入session-handoff
[ ] development-plan里的状态与部署事实一致
[ ] 新需求/改变行为同步requirements和decisions
[ ] 模型/API/限制同步契约、API例子、前端类型
[ ] 记录实际运行命令和结果，明确skip/未测/失败原因
[ ] CHANGELOG追加，历史验收保留，不伪称已发布
[ ] 确认文档链接指向真实文件；没有密码、令牌、私钥或用户原配置
[ ] 任务/后台作业已收集；用户实例保持原状或部署已获准且记录
~~~

文档固化轮只整理文档；之后代理池优化轮的源码、测试和构建状态以[接续快照](<session-handoff.md>)为准。每轮保留独立带日期验收，不覆盖旧结果。

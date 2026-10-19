# UniProxy 文档索引

更新：2026-10-20。**入口先读接续快照，再按任务读取需求、计划和开发指南。** 这些文件保存在项目中，不依赖会话摘要；仍须按每次收尾规则维护。

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
| [需求基线](<requirements.md>) | 场景、R01–R21、接受的行为/限制、不做什么 | 用户确认/范围改变时更新 |
| [开发计划](<development-plan.md>) | M0–M10、P0/P1/P2待办、完成定义 | 每次任务阶段更新；候选不等于获准 |
| [开发指南](<development-guide.md>) | 源码模块、环境、数据规则、构建/测试、停机升级 | 工程操作或模块接口改变时更新 |
| [技术决策](<decisions.md>) | D01–D19、原因与后果、替代旧方案 | 新决策追加或标注被取代，保留历史 |
| [接续快照](<session-handoff.md>) | 一页恢复上下文，时间敏感运行状态和下一步 | 每轮收尾必更新，PID/哈希必须注明日期 |
| [架构](<architecture.md>) | 控制/转发面、核心对象、生命周期、安全边界 | 实现架构变动时更新 |
| [接口契约](<implementation-contract.md>) | Go/TS模型、API形状、内核适配、校验范围 | 字段/接口变动时同步源码测试 |
| [API示例](<api.md>) | 自动化调用与返回语义 | 用户可用接口变更时更新 |
| [测试指南](<testing.md>) | 分层验收方法、命令、未验证边界 | 测试入口/环境改变时更新 |
| [首版历史验收](<verification.md>) | 首版已运行的验证及当时5/9测试口径 | 历史记录，不改成后来重测的证据 |
| [中转修复验收](<relay-fix-verification.md>) | 328/178/325离线结果、7/21测试、待部署修复 | 本次修复交付证据；升级事实另记快照 |
| [代理池优化验收](<pool-workflow-verification.md>) | R16–R18、20/30测试、原子监听/池内移除/卡片及构建边界 | 本轮完成证据；部署另经确认 |
| [中转变更设计](<relay-change-contract.md>) | 图身份、截断、拖放、过滤的完成版设计 | 当前实现补充，不是活动子任务命令 |
| [入库与全面复查验收](<repository-review-verification.md>) | 首次入库、复查范围、7 项缺陷修复、32/32 浏览器与真实内核回归 | 本轮完成证据；明确未部署 |
| [订阅与分组验收](<subscription-groups-verification.md>) | Fake-IP原因/安全复核、紧凑池、组内管理，32/31验证与待部署界限 | M10完成证据；不宣称私有订阅全文已测 |
| [归档协作备注](<coordination.md>) | 早期并行划分和结束状态 | 归档，不用于判断剩余工作 |
| [CHANGELOG](<../CHANGELOG.md>) | 用户可见变化和交付节点 | 每个变更完成时追加；不等于已发布 |

### 模块细节

- [导入器兼容/测试](<../internal/importer/README.md>)
- [Mihomo内核与安装器](<../internal/core/README.md>)
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

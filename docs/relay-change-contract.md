# Clash中转与UI修复设计（已实施）

状态：2026-10-19 源码、测试和暂存构建完成，用户进程未升级。此文记录完成版设计，不是当前子代理指令。

入口：[接续快照](<session-handoff.md>) · [需求R12–R14](<requirements.md>) · [修复验收](<relay-fix-verification.md>)。

## 1. 问题与不做什么

用户的本地Clash配置包含节点级dialer-proxy，旧解析器一概拒绝且记录128条错误后停止。修复要保持真实中转语义，不能删除字段再直接拨号，也不能把解析错误解释为网络屏蔽。

不复制用户原始文件/凭据进仓库，不自动接入其远端节点，不采用文件中的全局rules/controller/listener，不改现有用户数据，不擅自重启或覆盖在线exe。

## 2. 字段和责任

规范来源：[Go模型](<../internal/model/types.go>)、[TS投影](<../src/shared/types.ts>)。

| 字段 | 含义与流转 |
|---|---|
| ParsedNode.DialerProxy | 同文件原始引用名，预览可显示；存储前清空 |
| ParsedNode.DialerFingerprint | 解析完成的上游递归SHA256身份；不序列化，仅用于本次导入ID映射 |
| Node.DialerID | 实际持久节点ID，不受显示名变化影响；删除上游后仍保留阻断边 |
| Node.DialerIdentity | 最近验证的上游身份指纹，作为缺失上游的身份墓碑；备份恢复再导入保持原下游/监听ID |
| Node.DialerName / DialerError | 公共显示信息；不包含认证数据，阻断信息来自有效依赖链 |
| ParseResult/ImportResult.TotalErrors、ErrorsTruncated | 总错误数与有界详情分开；详情最多128条 |

Config始终是严格的原生节点参数映射，**不允许原始dialer-proxy**。编译器在独立图校验后才注入n-<stable ID>。不能将ParsedNode直接JSON持久化后指望DialerFingerprint还存在；解析→upsert必须同一内存数据流。

## 3. 解析和身份

[解析图实现](<../internal/importer/dialer.go>)采用两遍处理：收集所有具体节点、保留无效条目的名字索引，再解析引用并遍历图。输出/诊断保持原文件顺序。

- 只引用同一Clash文档内显式命名的具体代理；允许前向引用、Unicode名、不同标签的等价配置别名。
- 拒绝缺失/无效目标、重复显式名歧义、自引/环、超过16条依赖边、策略组以及规范化内置策略。
- builtin判断大小写/首尾空白规范化，包含DIRECT、REJECT、REJECT-DROP、GLOBAL、PASS、COMPATIBLE。
- 不从其它订阅/数据库猜测名称，不扁平化fallback/select/url-test组，不拓展URI delegation或sing-box detour。
- 直接节点Fingerprint与旧版bit-for-bit兼容；链Fingerprint包含递归上游身份，不包含原始引用名/显示名。
- 两个相同连接配置的中转别名可合并，引用它们的等价链也可合并；上游凭据或选项改变必须改变链身份。
- 达到128详情时继续扫描，总体5MiB/5000条/结构深度与图深度限制仍生效，安全错误不打印提交值。

## 4. 应用、订阅、删除、备份

[应用导入](<../internal/app/imports.go>)先保留所有指纹→稳定ID，再写入节点与DialerID，支持源顺序/别名去重。订阅以完整链指纹协调本源引用；不损坏其他来源和人工组。

[模型图](<../internal/model/relay.go>)与[编译器](<../internal/core/compiler.go>)共同保证：上游缺失/禁用时下游不被编译为直接节点、不参与池候选，独享入口拒绝新流量。已建立TCP尽量自然结束。

新导入缺失引用报错；已存节点的缺失上游可以保留，目的是可见阻断与后续恢复，不代表允许拨号。删除上游不删除DialerID/DialerIdentity。

[备份恢复/导出](<../internal/app/relay.go>)：

- 逐节点重做安全解析，再从稳定ID图重算Fingerprint。现存上游身份始终重算；缺失时保留有效DialerIdentity，不能只信备份提供的任意节点Fingerprint。
- 公共注解函数不能清空持久Identity；只在bootstrap/PATCH响应副本脱敏。
- 导出节点名避免大小写/空白规范化内置词，重复名生成唯一稳定标签。
- 缺失引用的占位名同样保留唯一性，防止另一真实节点恰好同名导致错误重连。
- [回归](<../internal/app/relay_test.go>)覆盖编辑→删除上游→备份→恢复→重导入，以及标签碰撞/保留词。

## 5. UI行为

- [文件导入](<../src/web/ImportDialog.tsx>)提供拖入和可访问浏览按钮；单文件≤5MiB，目录/多文件明确拒绝，拖放不触发浏览器跳转，支持相同文件重选。
- [读文件工具](<../src/web/importUtils.ts>)统一验证；读取和预览有代数保护，模式/内容/组/文件变化或卸载后旧结果不得回写；导入提交时禁止替换输入。
- [节点过滤](<../src/web/nodeFilters.ts>)包含any/all，独立排除any且优先，onlyUngrouped判定空groupIds。快捷未分组清空/禁用包含条件，保留排除条件。
- 筛选/全局重置重设页码，不悄悄清空已选ID；被筛掉的选择显示警告。手机长面板限制在视口，显式关闭和Escape返回焦点。
- 表格/详情/预览展示中转名/阻断原因，不展示配置密码。池预览排除public dialerError，与后端一致。
- 详情仍最多128，UI依据TotalErrors/ErrorsTruncated准确说明总数和截断；旧服务无总数时保守显示“至少”。

## 6. 验收和状态

实际私有文件离线计数328条/178中转/325唯一；合成图fixture中的318/168是另一组测试数据。7项真实隔离API浏览器、21项UI工具、fixture回归、Go/vet、实际本机两跳及原文件-t均通过；不是公网连通证据。详见[验收](<relay-fix-verification.md>)。

原并行范围importer / web / lead已结束，没有遗留写任务。新轮次不继承旧owner；部署暂存程序需用户确认，详情见[开发指南](<development-guide.md>)和[快照](<session-handoff.md>)。

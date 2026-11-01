# UniProxy 会话接续快照

**最后核对：2026-11-01 12:05（Asia/Shanghai）。M19/R26代理池编辑体验已完成并通过39项工具单测、严格TS、生产Web构建、40项隔离浏览器及完整fixture验证；功能提交8c047c0已推送main，尚未发布或部署，正式版仍为v0.2.3。用户服务最后观察为2026-10-22 18:08：主程序PID17124（16:17:19创建）+内核PID22212（16:17:20），9090归17124，19090空闲；本轮未触碰，操作前须重新核对。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.3](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.3>)，四平台程序与SHA256SUMS；[中文说明](<releases/v0.2.3.md>)已同步。
- 发布源码[5ed4cdfd5a40d32ed22f3183e16ce32eb91cb787](<https://github.com/FYHC1/uniproxy/commit/5ed4cdfd5a40d32ed22f3183e16ce32eb91cb787>)，annotated tag对象cb9f99ee126ec8cae3d6f962ec46f8c5f124a5fe，解引用为该提交；后续文档提交不移动标签。
- M19功能提交8c047c0（7个前端/测试文件）已推送main；CHANGELOG、requirements、decisions、development-plan与本快照随后作为文档提交更新，不改变功能。
- 当前栈Go + React/TS + SQLite加密payload + 外置Mihomo1.19.32；M19仅前端交互/样式与测试，无schema/API或内核升级。
- 用户已授权推送到GitHub；M19功能提交已推送。未授权发布、部署、停服、访问用户DB/key或真实节点连接。

## M19 — 编辑代理池体验（已推送，未发布）

- 用户要求：弹窗元素排版更紧凑；包含分组区域固定显示4行、超过时上下滚动；节点区改为延迟测试、排序和状态筛选（不再保留暂存/添加/移除）。
- [poolSelection.ts](<../src/web/poolSelection.ts>)新增`matchesPoolNodeStatus`与`sortPoolNodes`（名称 locale、延迟升序且未测最后、速度降序、最近更新倒序），删除暂存动作；[PoolDialog.tsx](<../src/web/PoolDialog.tsx>)恢复即时`changePoolSelection`，节点区增加状态筛选、排序和“测延迟”；[ui.tsx](<../src/web/ui.tsx>)的ChoiceList增加有界`filter`与`controls`，全选基于筛选结果、清理由完整options决定；[poolStyles.css](<../src/web/poolStyles.css>)压缩弹窗间距/行高，并把分组列表固定为167px内滚。
- 语义保持：组匹配动态、UDP兼容过滤、旧排除记录与删除引用处理不变；筛选/排序/搜索只影响显示，已选但隐藏的成员保存时保留。“测延迟”只提交已勾选、已启用且无中转错误的节点，要求内核运行、配置非dirty/applying且已应用；请求走真实`POST /api/tests`，不在前端伪造结果。
- 测试：`src/web/tests/utils.test.mjs` 39/39；[pool-selection.spec.ts](<../tests/browser/pool-selection.spec.ts>)9项覆盖筛选/排序仅显示、隐藏成员保留、真实检测请求体、禁用原因与四行滚动；pool-workflow、smoke、[verify.mjs](<../src/web/tests/verify.mjs>)、[pool-regressions.mjs](<../src/web/tests/pool-regressions.mjs>)同步移除暂存步骤。2026-11-01复跑生产构建与全量`pnpm test:e2e`，40 passed（1.1m），`pnpm test:ui`与fixture通过，严格TS与`pnpm run build:web`通过（index-CZ2krID9.js 523.52kB/gzip154.38kB，仅非失败分包提示）。

## 运行服务（2026-10-22 18:08 最后观察）

- 用户主程序PID17124：`C:\CustomFiles\CodeProjects\Github\uniproxy\bin\uniproxy-windows-amd64.exe`，16:17:19创建；内核PID22212：`C:\ProgramData\UniProxy\core\mihomo.exe`，16:17:20创建；9090归17124，19090无测试残留。此后未重新核对，操作前必须确认当前进程。
- 本轮未替换/停止；不要凭过期快照PID清理。后续获部署授权时，先核对进程/路径/版本/数据目录，正常退出后换程序；禁止在线复制覆盖主库或混用WAL/SHM。

## v0.2.3发布事实（已完成，勿重开）

- main CI39467680808、tag CI39468279998、Release39468280079全部成功；2026-10-21 01:42:43发布并设Latest，标签固定在5ed4cdf。
- 正式Windows程序0.2.3/5ed4cdfd5a40/2026-10-20T17:41:59Z，全部TestBuilt、真实WM_CLOSE重启及独立失败窗口验证通过；历史tag/Release/asset ID未变。
- 详细记录见[发布验收](<release-0.2.3-verification.md>)与[订阅兼容验收](<subscription-format-verification.md>)；不得把历史私有样例测试当本轮重跑。

## 正式下载副本

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.3/uniproxy-windows-amd64.exe>) | 13122048 | cbcbe3292866be5167da7b6ca16f275d1968636e04a676356561c237a69184e1 |
| [Windows arm64](<../bin/releases/v0.2.3/uniproxy-windows-arm64.exe>) | 12216320 | adb77c03a2d3abe97c7d92c7e1db403b364a4ce3bcd6277c1903dc4344f99fda |
| [Linux amd64](<../bin/releases/v0.2.3/uniproxy-linux-amd64>) | 12767416 | 16d9e6a9e963674bfb9ac7e82f5fa3ecd24bd82ea0fa30d5140364ad6b3932e5 |
| [Linux arm64](<../bin/releases/v0.2.3/uniproxy-linux-arm64>) | 12124344 | 0c527e5cd24130586299605941b45aa6390b088704a77123acad612c6c312758 |
| [SHA256SUMS](<../bin/releases/v0.2.3/SHA256SUMS.txt>) | 360 | 187ed2ebe065f1012e96913f85b6d2ce668e304beabaffe7ca19a060115dd8d3 |

## 结束状态与下一步

- M19功能提交8c047c0已推送main；本快照随文档提交更新。发布或部署仍需用户明确授权，不移动v0.2.3标签。
- 用户运行旧程序是否升级由其决定或另行授权；升级前退出旧实例，禁止运行中覆盖数据库，不自动重启服务。
- 常规测试清空UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD。Linux/ARM64实机、Docker、race、容量与真实节点连通仍未验证；解析、浏览器fixture与core-t不能证明远端连通。既有Docker忽略未排除.local，不构建/上传本机Docker上下文。

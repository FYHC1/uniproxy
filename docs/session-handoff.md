# UniProxy 会话接续快照

**最后核对：2026-10-21 01:47（Asia/Shanghai）。M18完成：订阅兼容修复已作为v0.2.3于01:42:43正式发布并设Latest。源码/标签已推送，main与标签Windows/Linux CI、四平台Release、所有正式资产摘要及Windows导入/关闭重启验收通过。本轮未部署，用户服务保持。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.3](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.3>)，四个平台程序与SHA256SUMS；[中文说明](<releases/v0.2.3.md>)已同步到Release。
- 发布源码[5ed4cdfd5a40d32ed22f3183e16ce32eb91cb787](<https://github.com/FYHC1/uniproxy/commit/5ed4cdfd5a40d32ed22f3183e16ce32eb91cb787>)，annotated tag对象cb9f99ee126ec8cae3d6f962ec46f8c5f124a5fe，解引用为该源码提交。后续文档收尾提交不移动标签。
- [发布验收](<release-0.2.3-verification.md>)记录本轮CI/资产/测试；[开发验收](<subscription-format-verification.md>)保留前轮私有样例/fuzz/本地候选历史，两者不能混称重跑。
- 当前栈Go + React/TS + SQLite加密payload + 外置Mihomo1.19.32；本版无schema/API或内核升级。
- 用户只授权推送和发布，不包含部署/停服、用户DB/key访问、私有上游节点连接或系统网络更改。

## 运行服务（01:45重新观察）

- [用户主程序](<../bin/uniproxy-windows-amd64.exe>)PID6244，01:04:17创建；其自有内核PID23716，01:04:19创建；9090归属6244，19090无测试残留。
- 原用户实例及创建时间未变，本轮未替换/重启；不要凭过期快照PID清理，不通过bootstrap读取用户配置。下载到发布目录不代表已升级运行程序。
- 后续如获部署授权，先核对当前进程/路径/版本和数据目录，正常退出后再换程序；禁止在线复制覆盖主库或混用WAL/SHM。历史恢复证据/匹配key不读不删。

## 修复范围与限制

- 可选TLS关闭只归一化严格false证书绕过和精确空SNI占位，保留实际TLS/验证/节点身份；原7VMess中4个占位误拒已修。活动TLS/安全字段仍严格校验。
- 静态QX/Surge/Surfboard/Loon节点适配、URI同值SNI别名与有界STATUS展示头；远程provider/include/策略组policy-path拒绝，不采用全局规则/DNS/MITM/脚本。结构化YAML键/块标量不误认节点。
- 静态文本未显式声明UDP则TCP-only，三种导入模式均提示；SNI/WS Host/插件Host歧义需明确源字段，不能宣称完整复制各客户端默认值。
- 来源全有或全无、错误force刷新保留、稳定身份和中转fail-closed及下载SSRF/Fake-IP/TLS防线保持。非法SNI含@仍拒绝，不擅自改值或跳证书。

## 本轮验证

- 本机Go1.27.2/Node24.21.0/pnpm12.9.1；生产Web、严格TS/noUnused、36工具、37隔离API浏览器（1.1分钟）、完整UI fixture通过。Vite520.87kB/gzip153.73kB只有非失败分包提示。
- 显式临时APP/受控本机CORE的Go全量通过（app12.754s/core10.270s/importer1.820s/integration13.936s），全量vet/gofmt/diff通过。包含9组合成格式core-t、3类自有回环ClientHello、来源/隐私回归；私有文件/线上下载开关保持关闭。
- [main CI39467680808](<https://github.com/FYHC1/uniproxy/actions/runs/39467680808>)成功（Ubuntu1:09/Windows1:50）后推tag；[tag CI39468279998](<https://github.com/FYHC1/uniproxy/actions/runs/39468279998>)成功（0:44/1:28）；[Release39468280079](<https://github.com/FYHC1/uniproxy/actions/runs/39468280079>)四平台+发布成功。CI为Go1.25/Node24/pnpm11.28.5，未配置CORE/APP/浏览器，不把其skip当通过。
- 正式Windows下载文件全部TestBuilt通过3.955秒：6种预览/保存及安全拒绝、CLI、嵌入UI/CSP、3轮强杀/子进程随退/重启、独立日志、healthcheck不堆日志。另真实WM_CLOSE两轮退出0并重启，独立失败窗口保留/关闭后退出1通过。
- 两项只读发布审查完成无已知阻塞；33文件精确提交白名单与私有标记筛查通过。四程序及校验清单大小/摘要与GitHub匹配，中文说明逐字匹配，历史tag/Release/asset ID保持。

## 正式下载副本

构建身份 **0.2.3 · commit5ed4cdfd5a40 · built2026-10-20T17:41:59Z**（Windows amd64）。只使用Actions正式产物，不把本机候选替换发布文件。

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.3/uniproxy-windows-amd64.exe>) | 13122048 | cbcbe3292866be5167da7b6ca16f275d1968636e04a676356561c237a69184e1 |
| [Windows arm64](<../bin/releases/v0.2.3/uniproxy-windows-arm64.exe>) | 12216320 | adb77c03a2d3abe97c7d92c7e1db403b364a4ce3bcd6277c1903dc4344f99fda |
| [Linux amd64](<../bin/releases/v0.2.3/uniproxy-linux-amd64>) | 12767416 | 16d9e6a9e963674bfb9ac7e82f5fa3ecd24bd82ea0fa30d5140364ad6b3932e5 |
| [Linux arm64](<../bin/releases/v0.2.3/uniproxy-linux-arm64>) | 12124344 | 0c527e5cd24130586299605941b45aa6390b088704a77123acad612c6c312758 |
| [SHA256SUMS](<../bin/releases/v0.2.3/SHA256SUMS.txt>) | 360 | 187ed2ebe065f1012e96913f85b6d2ce668e304beabaffe7ca19a060115dd8d3 |

## 结束状态与下一步

本轮构建/测试/流程观察jobs和两项只读协作均结束并收集；发布结果文档单独提交推送，标签固定在功能提交。下一动作是用户自行退出旧实例后换用正式版，或另行明确授权部署；不得自动重启用户服务。没有新增解析功能待做。

常规测试清空UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD。前轮私有样例的11个临时副本已清理，本轮未重下载、未读取原配置；纯解析/core-t不能证明远端连通。Linux/ARM64正式程序实机、Docker、race、容量/吞吐与私有上游连通仍未验证。既有Docker忽略未排除.local，不构建/上传本机Docker上下文；先前恢复授权不能复用于常规开发。

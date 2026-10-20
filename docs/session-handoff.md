# UniProxy 会话接续快照

**最后核对：2026-10-21 01:35（Asia/Shanghai）。M17/R25修复及本地验收完成，用户已授权M18/v0.2.3推送与发布，仍未授权部署。版本/说明就绪，Go全量、vet、严格TS、36工具和37浏览器本轮通过；准备提交推main、等CI，再打标签发布。**

## 当前任务

- 当前栈Go + React/TS + SQLite加密payload + 外置Mihomo1.19.32；本轮无schema/API或内核版本变化。
- 起点main/origin均28021bf96c39da2d8fbd826385b5b22409eb7430，当前正式最新[v0.2.2](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.2>)；v0.2.3标签尚未推送，不改历史标签/资产或仓库可见性（公开）。
- 新用户授权包含提交、推送、标签与Release；不包含停服/部署、访问用户DB/key/恢复材料、连接私有代理节点或修改系统网络。
- 01:29观察[用户主程序](<../bin/uniproxy-windows-amd64.exe>)PID6244，01:04:17创建；其内核PID23716，01:04:19创建；9090归属6244。不要按此前历史PID清理，不通过bootstrap读取用户配置。
- 权威进度：[v0.2.3发布验收](<release-0.2.3-verification.md>)、[计划](<development-plan.md>)、[中文说明](<releases/v0.2.3.md>)。

## 修复范围（M17已完成）

- 可选TLS关闭时仅归一化类型正确的false验证绕过/精确空SNI，不强制启TLS、不跳过证书、不丢弃活动字段；原图7VMess中4个模板误拒已修，原源7/7通过。
- 有界静态QX/Surge/Surfboard/Loon节点适配、URI同值SNI别名/STATUS头；远程provider/include/策略组policy-path拒绝，不采用全局脚本/规则。YAML显式键/块标量不能误认成节点。
- 未显式UDP的静态文本TCP-only，UI所有导入模式提示；TLS/WS SNI与Host、插件Host歧义需明确字段，不声称完整客户端默认等价。
- 来源全有或全无、错误force刷新保留、稳定身份/中转fail-closed及下载SSRF/Fake-IP/TLS边界保持。[完整开发验收](<subscription-format-verification.md>)保留历史样例/fuzz/候选结果。

## 本轮本地验证与产物

- 生产Web、严格TypeScript/noUnused、36/36工具通过；Vite520.87kB/gzip153.73kB仅非失败分包提醒。
- 临时[发布前候选](<../bin/updates/release-0.2.3/uniproxy-windows-amd64.exe>)身份0.2.3-pre.1/local-release-verification，构建2026-10-20T17:32:08Z；SHA256为229cb60b0a509245ec3d23aea74c8e049b7c84c87b9e2419a19adc0025c536b0。不是正式发布资产，未替换用户程序。
- 显式APP/受控本机CORE的Go全量通过（app12.754s/core10.270s/importer1.820s/integration13.936s），全量vet/gofmt/diff通过；37/37隔离API浏览器1.1分钟通过。Windows实际控制台WM_CLOSE两轮退出0/重启及独立错误窗口保留/关闭后退出1通过。
- 33发布路径私有标记筛查无命中；217tracked/eligible路径不含数据/密钥/EXE/本机诊断，30文档427本地链接有效；独立源码/隐私审查无已知发布阻塞。
- 仅合成/临时数据与回环服务。本轮没有重下载私有订阅，前轮9样例结果不冒充重跑；其11个临时副本已清理，不再引用旧清单。

## 下一步与边界

1. 两项只读review与候选Windows控制台验收均完成，无已知阻塞；33源码/测试/文档已按精确白名单暂存，下一步提交并推main。
2. 等main Windows/Linux CI成功，再新建annotated v0.2.3指向该提交。既有Release工作流不依赖CI，因此还须单独复核标签CI，不使用手动workflow_dispatch。
3. Release四平台产物由GitHub Actions构建，下载到独立忽略目录，核对四程序+SHA256SUMS、摘要及Windows正式文件准确版本/commit，再运行全部TestBuilt与真实WM_CLOSE测试。补中文说明/Latest并记录实际发布结果。
4. 完成后回写本快照/计划/发布验收并推文档收尾提交，不移动标签；结束相关后台作业，保留用户服务。

CI未配置真实CORE/APP/私有文件变量、不运行浏览器或主动fuzz，不能把skip当通过。Linux/ARM64实机、Docker、race与容量/私有上游连通不在本轮验收。既有Docker忽略未排除.local，不构建/上传本机Docker上下文。历史恢复证据/匹配密钥不读不删。

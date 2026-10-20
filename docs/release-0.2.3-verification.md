# UniProxy v0.2.3 发布验收

日期：2026-10-21（Asia/Shanghai）。用户明确要求推送GitHub并发布新Release，采用补丁版本0.2.3。授权包含本轮源码/文档提交、推送、新标签和Release，不包含部署、停服、用户数据库/主密钥访问或私有上游连接。

## 范围与基线

- M17/R25订阅兼容修复及回归，详情见[功能验收](<subscription-format-verification.md>)；[中文发布说明](<releases/v0.2.3.md>)列明静态格式/TCP-only限制、非法SNI拒绝和安全升级。无模型/API/schema变化，Mihomo保持1.19.32。
- 起点main HEAD为28021bf96c39da2d8fbd826385b5b22409eb7430，与origin/main一致；远端最新v0.2.2，v0.2.3尚不存在。仓库[FYHC1/uniproxy](<https://github.com/FYHC1/uniproxy>)为公开，本轮不更改可见性或历史标签/资产。
- 01:29重新观察用户主程序PID6244（[当前程序](<../bin/uniproxy-windows-amd64.exe>)，01:04:17启动）及其内核PID23716；9090归属6244，19090空闲。此前快照PID已过期，本轮不停止/替换当前实例，不访问bootstrap或数据。
- 只提交源码、测试和可公开文档；本机诊断/恢复、数据/密钥/订阅、生成物和二进制均排除。不构建或上传本机Docker上下文。

## 本轮验证与发布结果

- Go/package版本及当前文档同步为0.2.3；发布说明就绪。两项只读审查均完成：33文件源码/隐私审查无已知发布阻塞；流程审查明确Release工作流不等待CI，须先让main两平台CI通过再推标签并复核标签CI。Lead独占版本/文档写入。
- 本机Go1.27.2、Node24.21.0、pnpm12.9.1；CI使用既有Go1.25、Node24、pnpm11.28.5，无依赖/锁文件变更。生产Web构建通过（Vite7.3.7，520.87kB/gzip153.73kB，仅分包提示）；严格TypeScript/noUnused与36/36工具测试通过。临时0.2.3-pre.1候选构建01:32:08，SHA256为229cb60b0a509245ec3d23aea74c8e049b7c84c87b9e2419a19adc0025c536b0；不是正式发布资产。
- 显式APP/CORE的Go全量 `go test -mod=readonly -p 1 ./... -count=1` 通过（app12.754s、core10.270s、importer1.820s、integration13.936s），全量vet、gofmt与diff检查通过。37/37隔离API浏览器通过（1.1分钟）。不会重下载已清理的私有样例，前轮结果只作为历史证据。
- 发布前33个变更文件的私有标记筛查无命中；217个tracked/eligible路径没有DB/key/EXE/本机诊断，30文档427本地链接有效。
- 新候选实际Windows控制台WM_CLOSE两轮退出0、同目录重启通过；独立无参数错误窗口保留、诊断落盘，关闭后退出1。只作用于自建临时控制台与数据。33路径已按精确白名单暂存并通过cached diff检查。
- [源码提交5ed4cdf](<https://github.com/FYHC1/uniproxy/commit/5ed4cdfd5a40d32ed22f3183e16ce32eb91cb787>)已推送main（33文件3149新增/98删除）；Windows/Linux CI [39467680808](<https://github.com/FYHC1/uniproxy/actions/runs/39467680808>)成功（Ubuntu1分9秒、Windows1分50秒）。通过后创建annotated v0.2.3指向同一提交并推送，沿用四平台标签发布流程。
- 完整UI fixture验收已通过，零未捕获浏览器错误；合成fixture截图仅作交互验证，不是代理连通证据。
- annotated v0.2.3已推送，tag对象cb9f99ee126ec8cae3d6f962ec46f8c5f124a5fe，解引用提交5ed4cdfd5a40d32ed22f3183e16ce32eb91cb787。标签CI [39468279998](<https://github.com/FYHC1/uniproxy/actions/runs/39468279998>)成功（Ubuntu44秒、Windows1分28秒）；Release [39468280079](<https://github.com/FYHC1/uniproxy/actions/runs/39468280079>)四平台成功（Linux amd64 51秒/arm64 57秒、Windows amd64 56秒/arm64 55秒），release job9秒成功。
- [v0.2.3](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.3>)于2026-10-21 01:42:43正式发布，非draft/非prerelease，中文说明已应用并设Latest；API确认恰好4主程序+1校验和，无独立安装器或内核打包。独立下载和程序验收结果见下文。
- Actions仅Node20强制Node24和Ubuntu26镜像迁移的非失败注解，没有测试/构建失败。
- 正式资产已全部下载到独立忽略目录，精确4主程序+1校验和、文件大小、清单中的四SHA256和GitHub全部五digest逐项匹配；没有用本地候选覆盖远端产物。正式Windows文件准确报告0.2.3/5ed4cdfd5a40/2026-10-20T17:41:59Z。
- 中文发布说明与[源文件](<releases/v0.2.3.md>)逐字核对一致，Latest API指向v0.2.3。下载后的Windows正式文件全部TestBuilt通过（集成包3.955秒）：6格式/结构文档导入预览与保存、隐私/TLS/provider/SNI拒绝、CLI退出码/公开绑定保护、嵌入UI+CSP、3轮强杀父进程/内核随退/同目录重启、独立诊断和healthcheck不积累日志。
- 正式文件实际Windows控制台WM_CLOSE两轮退出0并重启通过；独立无参数失败窗口保留和报告落盘，关闭后退出1。没有使用用户数据。
- 01:45复核原用户主程序6244/其内核23716及创建时间未变，9090归6244，19090无残留；本轮未停止或部署。v0.2.0/v0.2.1/v0.2.2历史tag与Release/asset ID复核未变；全部构建/测试/流程观察jobs及两项只读协作已结束并收集。

## 正式资产

| 文件 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.3/uniproxy-windows-amd64.exe>) | 13122048 | cbcbe3292866be5167da7b6ca16f275d1968636e04a676356561c237a69184e1 |
| [Windows arm64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.3/uniproxy-windows-arm64.exe>) | 12216320 | adb77c03a2d3abe97c7d92c7e1db403b364a4ce3bcd6277c1903dc4344f99fda |
| [Linux amd64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.3/uniproxy-linux-amd64>) | 12767416 | 16d9e6a9e963674bfb9ac7e82f5fa3ecd24bd82ea0fa30d5140364ad6b3932e5 |
| [Linux arm64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.3/uniproxy-linux-arm64>) | 12124344 | 0c527e5cd24130586299605941b45aa6390b088704a77123acad612c6c312758 |
| [SHA256SUMS](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.3/SHA256SUMS.txt>) | 360 | 187ed2ebe065f1012e96913f85b6d2ce668e304beabaffe7ca19a060115dd8d3 |

[本地Windows发布文件](<../bin/releases/v0.2.3/uniproxy-windows-amd64.exe>)、[本地校验清单](<../bin/releases/v0.2.3/SHA256SUMS.txt>)仅为已校验下载副本，未替换[用户运行文件](<../bin/uniproxy-windows-amd64.exe>)。


发布结果文档收尾复核：30文档423本地链接有效，8个文档变更的私有标记筛查与diff检查通过；本轮最后一次提交仅记录上述已验证结果，使用 `[skip ci]`，不改变已验收源码或v0.2.3标签。

## 验证边界

仅本机受控数据与回环服务。常规验证清除UNIPROXY_TEST_SUBSCRIPTION_MANIFEST、IMPORT_FILE、SUBSCRIPTION_HOST与CORE_DOWNLOAD，显式未设置的测试跳过不能报告为通过。CI当前不启用真实内核/编译程序/私有文件变量，也不运行浏览器；这些测试由本地隔离验证补充。Linux/ARM64实机、Docker、race、公网私有节点及容量/吞吐未在本轮新增验证。

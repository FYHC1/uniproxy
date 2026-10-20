# UniProxy v0.2.3 发布验收

日期：2026-10-21（Asia/Shanghai）。用户明确要求推送GitHub并发布新Release，采用补丁版本0.2.3。授权包含本轮源码/文档提交、推送、新标签和Release，不包含部署、停服、用户数据库/主密钥访问或私有上游连接。

## 范围与基线

- M17/R25订阅兼容修复及回归，详情见[功能验收](<subscription-format-verification.md>)；[中文发布说明](<releases/v0.2.3.md>)列明静态格式/TCP-only限制、非法SNI拒绝和安全升级。无模型/API/schema变化，Mihomo保持1.19.32。
- 起点main HEAD为28021bf96c39da2d8fbd826385b5b22409eb7430，与origin/main一致；远端最新v0.2.2，v0.2.3尚不存在。仓库[FYHC1/uniproxy](<https://github.com/FYHC1/uniproxy>)为公开，本轮不更改可见性或历史标签/资产。
- 01:29重新观察用户主程序PID6244（[当前程序](<../bin/uniproxy-windows-amd64.exe>)，01:04:17启动）及其内核PID23716；9090归属6244，19090空闲。此前快照PID已过期，本轮不停止/替换当前实例，不访问bootstrap或数据。
- 只提交源码、测试和可公开文档；本机诊断/恢复、数据/密钥/订阅、生成物和二进制均排除。不构建或上传本机Docker上下文。

## 本轮进度

- Go/package版本及当前文档同步为0.2.3；发布说明就绪。两项只读审查均完成：33文件源码/隐私审查无已知发布阻塞；流程审查明确Release工作流不等待CI，须先让main两平台CI通过再推标签并复核标签CI。Lead独占版本/文档写入。
- 生产Web构建通过（Vite7.3.7，520.87kB/gzip153.73kB，仅分包提示）；严格TypeScript/noUnused与36/36工具测试通过。临时0.2.3-pre.1候选构建01:32:08，SHA256为229cb60b0a509245ec3d23aea74c8e049b7c84c87b9e2419a19adc0025c536b0；不是正式发布资产。
- 显式APP/CORE的Go全量 `go test -mod=readonly -p 1 ./... -count=1` 通过（app12.754s、core10.270s、importer1.820s、integration13.936s），全量vet、gofmt与diff检查通过。37/37隔离API浏览器通过（1.1分钟）。不会重下载已清理的私有样例，前轮结果只作为历史证据。
- 发布前33个变更文件的私有标记筛查无命中；217个tracked/eligible路径没有DB/key/EXE/本机诊断，30文档427本地链接有效。
- 新候选实际Windows控制台WM_CLOSE两轮退出0、同目录重启通过；独立无参数错误窗口保留、诊断落盘，关闭后退出1。只作用于自建临时控制台与数据。33路径已按精确白名单暂存并通过cached diff检查。
- 待执行：提交推送main并等待Windows/Linux CI；成功后创建annotated v0.2.3标签，沿用四平台标签发布流程。
- 待执行：正式资产精确名单/大小/SHA256/远端digest核验，Windows正式文件TestBuilt、最终发布记录及服务保持核对。

## 验证边界

仅本机受控数据与回环服务。常规验证清除UNIPROXY_TEST_SUBSCRIPTION_MANIFEST、IMPORT_FILE、SUBSCRIPTION_HOST与CORE_DOWNLOAD，显式未设置的测试跳过不能报告为通过。CI当前不启用真实内核/编译程序/私有文件变量，也不运行浏览器；这些测试由本地隔离验证补充。Linux/ARM64实机、Docker、race、公网私有节点及容量/吞吐未在本轮新增验证。

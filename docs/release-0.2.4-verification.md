# UniProxy v0.2.4 发布与部署验收

日期：2026-11-01（Asia/Shanghai）。用户明确要求“发布新 release，部署并替换运行中的服务”。本轮授权包含版本与文档变更、源码/标签推送、GitHub Release、停止旧实例并在保留原数据目录的前提下替换运行服务；未访问或解析数据库/主密钥内容，也未连接用户节点。

## 范围与基线

- M19/R26 代理池编辑体验：弹窗紧凑排版、包含分组固定四行内滚、节点状态筛选/排序/真实延迟测试。仅前端交互、样式与测试，无 API、数据库 schema、Mihomo 内核或安全策略变化；内核仍固定 1.19.32。
- 发布起点 main HEAD 为 `65bec6f0d2ad330c26a6d301e2da7db16bedd98e`，正式版为 v0.2.3。版本/文档提交 `72da273b5b8119de9a360b979ec33956ad0e4d23`（8 文件，54 新增/17 删除）。
- 部署前用户服务：主程序 PID10920，`C:\CustomFiles\CodeProjects\Github\uniproxy\bin\releases\v0.2.3\uniproxy-windows-amd64.exe`（12:07:17 启动）；内核 PID28120，`C:\ProgramData\UniProxy\core\mihomo.exe`；9090 归 10920，19090 空闲；健康接口报告 0.2.3，bootstrap 为 32 节点、3 分组、3 池、6 监听、内核运行中。

## 本轮验证与发布结果

- 版本/说明同步为 0.2.4；`pnpm run build:web` 通过（index-CZ2krID9.js 523.52kB / gzip154.38kB，index-CLn6wq_e.css 71.60kB，仅分包提示）；严格 TypeScript/noUnused、39/39 UI 工具、`pnpm test:e2e` 40/40（56.3s）通过。
- 显式本机内核的 Go 全量 `go test -mod=readonly -p 1 ./... -count=1` 与 `go vet ./...` 通过；发布提交的源码/隐私与发布流程只读审查无已知发布阻塞。
- main CI [41272387153](<https://github.com/FYHC1/uniproxy/actions/runs/41272387153>) 成功（Ubuntu 30s、Windows 1m22s）。通过后创建并推送 annotated tag v0.2.4，标签对象 `05037b0804d4efab7aac0ba87e5ec778ea54caff`，解引用 `72da273b5b8119de9a360b979ec33956ad0e4d23`。
- 标签 CI [41272518085](<https://github.com/FYHC1/uniproxy/actions/runs/41272518085>) 成功（Ubuntu 33s、Windows 1m39s）；Release 工作流 [41272518132](<https://github.com/FYHC1/uniproxy/actions/runs/41272518132>) 四平台构建成功。
- [v0.2.4](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.4>) 于 2026-11-01 12:18:53 正式发布（UTC 04:18:53Z），非 draft、非 prerelease，已设 Latest；中文说明与 [releases/v0.2.4.md](<releases/v0.2.4.md>) 逐字一致。
- 正式 Windows 资产 `-version` 实测：`UniProxy 0.2.4 · commit 72da273b5b81 · built 2026-11-01T04:17:59Z`。
- 正式 Windows 资产全部 TestBuilt 子测通过（集成包 4.093s）：CLI 版本/退出码 2/公开绑定保护、真实内核空配置与嵌入 UI/CSP、3 轮强杀父进程与内核随退/重启、启动失败隔离报告、healthcheck 不堆日志、6 格式订阅预览/保存/冲突拒绝。
- 正式 Windows 资产实际控制台关闭测试通过：两轮 WM_CLOSE 均退出 0 并在同目录重启；独立无参数失败窗口保留、报告落盘，关闭后退出 1。测试只使用自建临时控制台与临时数据。

## 正式资产

| 文件 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.4/uniproxy-windows-amd64.exe>) | 13126144 | 4d57d1fe98165f9c720ac2adcbbf00599ccab359017250a4bbf1588b57ae5a9c |
| [Windows arm64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.4/uniproxy-windows-arm64.exe>) | 12220416 | 16a91ae40710ad85d5429323686a3c9c883dacc379957010b8bce80bc021748b |
| [Linux amd64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.4/uniproxy-linux-amd64>) | 12771512 | c56a73ae7b6bb1985bf10750489b3e7642cadb1a52a544721e51b0ae306cecae |
| [Linux arm64](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.4/uniproxy-linux-arm64>) | 12124344 | d30f5023887ff713ba4de45eec03c4a72a1bff9dfe5f84573689dbb49a2568f7 |
| [SHA256SUMS](<https://github.com/FYHC1/uniproxy/releases/download/v0.2.4/SHA256SUMS.txt>) | 360 | 3f2e0c2aeaf2140aa9e558192943c21d66c6222fd9d5cfe56e0efd149dd40c01 |

本地已下载副本位于 [bin/releases/v0.2.4](<../bin/releases/v0.2.4>)，五个文件的清单 SHA256 与 GitHub digest 全部匹配；未用本地候选覆盖远端资产。

## 部署与替换结果

- 先调用 `POST /api/runtime/stop` 停止受管内核，返回 `running:false`、`lastError:"代理内核已手动停止"`、`dirty:true`，确认内核 PID28120 退出。
- 然后仅对确认只包含该实例及其内核的专用控制台发送受控中断；服务 PID10920 与内核均退出。未向其他用户控制台、终端或进程发信号。退出后 `C:\ProgramData\UniProxy` 中 `uniproxy.db-wal`/`-shm` 已消失，`uniproxy.db` 最后写入 12:23:02，说明 SQLite 已完成正常关闭/检查点。
- 停机后创建一致备份：`C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`，共 5 个文件、62,960,192 字节；源目录与备份逐文件 SHA256 全部一致。备份位于数据目录之外，未解析数据库或主密钥内容。
- 12:23:29 启动官方 `bin/releases/v0.2.4/uniproxy-windows-amd64.exe`（无参数，使用原默认数据目录）。新主程序 PID16568，新内核 PID28844（`C:\ProgramData\UniProxy\core\mihomo.exe`），9090 归 16568；19090 无监听。
- 部署后 `/api/health` 返回 `{"ok":true,"version":"0.2.4"}`；bootstrap 保持 32 节点、3 分组、3 池、6 监听，内核 1.19.32、运行中且 `dirty:false`。UI 根路径 HTTP 200 且包含 CSP；旧 PID10920/28120 均已不存在。
- 2026-11-01 16:32 再次复核：PID16568 与内核 PID28844 仍在运行，健康版本仍为 0.2.4，节点/分组/池/监听数量与内核状态未变。未发现指向旧 v0.2.3 程序的桌面/开始菜单快捷方式或 Run 注册表项。

## 验证边界

- 本轮仅在本机、原数据目录和回环管理接口上操作；未连接用户节点、未解析数据库内容、未修改系统代理或防火墙。
- Linux/ARM64 仅完成构建与摘要校验，未实机运行；Docker、race、容量/吞吐与真实远端连通未在本轮新增验证。
- 浏览器 fixture、订阅解析和内核 `-t` 不等于真实节点可用性；真实节点可用性以用户后续实际检测为准。
- 常规测试环境变量（订阅清单、私有导入文件、订阅主机、内核下载）均未作为发布或部署通过依据；未设置的测试跳过不报告为通过。

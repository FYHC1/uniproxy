# UniProxy 会话接续快照

**最后核对：2026-11-01 16:32（Asia/Shanghai）。v0.2.4 已正式发布并部署替换本机运行服务：主程序 PID16568（`bin/releases/v0.2.4/uniproxy-windows-amd64.exe`，12:23:29 启动）+ 内核 PID28844（`C:\ProgramData\UniProxy\core\mihomo.exe`），9090 归 16568，19090 无监听；健康接口版本 0.2.4，32 节点/3 分组/3 池/6 监听，内核 1.19.32 运行中且 dirty=false。操作前仍须重新核对当前进程。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.4](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.4>)，四平台程序与 SHA256SUMS；[中文说明](<releases/v0.2.4.md>)已同步。
- 发布源码 [72da273b5b8119de9a360b979ec33956ad0e4d23](<https://github.com/FYHC1/uniproxy/commit/72da273b5b8119de9a360b979ec33956ad0e4d23>)，annotated tag 对象 `05037b0804d4efab7aac0ba87e5ec778ea54caff`，解引用为该提交。
- main CI [41272387153](<https://github.com/FYHC1/uniproxy/actions/runs/41272387153>)、标签 CI [41272518085](<https://github.com/FYHC1/uniproxy/actions/runs/41272518085>)、Release [41272518132](<https://github.com/FYHC1/uniproxy/actions/runs/41272518132>) 全部成功；2026-11-01 12:18:53 发布并设 Latest。
- 已按用户授权部署替换：旧 v0.2.3 实例 PID10920/内核 28120 已优雅退出；原数据目录 `C:\ProgramData\UniProxy` 保留。停机后备份到 `C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`，5 文件 62,960,192 字节，源/备份逐文件 SHA256 一致。
- 当前栈仍为 Go + React/TS + SQLite 加密 payload + 外置 Mihomo1.19.32；M19/v0.2.4 仅前端交互、样式、测试、版本/文档和部署，无 schema/API/内核/安全策略变化。

## 当前运行服务

- 主程序 PID16568：`C:\CustomFiles\CodeProjects\Github\uniproxy\bin\releases\v0.2.4\uniproxy-windows-amd64.exe`，2026-11-01 12:23:29 启动，无参数、使用默认数据目录。
- 内核 PID28844：`C:\ProgramData\UniProxy\core\mihomo.exe`，1.19.32；父进程 16568。
- 127.0.0.1:9090 归 16568；19090 无监听；UI 根路径 HTTP 200 且含 CSP。
- 部署后与 16:32 复核：health `{"ok":true,"version":"0.2.4"}`；bootstrap 32 节点、3 分组、3 池、6 监听；内核 running、dirty=false。
- 未发现桌面/开始菜单快捷方式或 Run 注册表项指向旧 v0.2.3 程序。旧实例 PID10920/28120 已不存在。

## v0.2.4 发布与部署事实

- 版本/文档提交 `72da273`（8 文件，54 新增/17 删除）；发布前生产 Web、严格 TS、39/39 UI 工具、40/40 隔离浏览器、显式本机内核的 Go 全量/vet 通过。
- 正式 Windows 程序版本 `UniProxy 0.2.4 · commit 72da273b5b81 · built 2026-11-01T04:17:59Z`；全部 TestBuilt 子测通过（4.093 秒），真实控制台 WM_CLOSE 两轮退出 0/重启及独立失败窗口退出 1 通过。
- 部署顺序：`POST /api/runtime/stop` 停内核 → 确认仅含该实例的控制台并发送受控中断 → 服务/内核退出，WAL/SHM 清理完成 → 停机备份并校验 → 启动官方 v0.2.4 → health/bootstrap 核对。
- 详细验收见[发布与部署验收](<release-0.2.4-verification.md>)。

## 正式下载副本

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.4/uniproxy-windows-amd64.exe>) | 13126144 | 4d57d1fe98165f9c720ac2adcbbf00599ccab359017250a4bbf1588b57ae5a9c |
| [Windows arm64](<../bin/releases/v0.2.4/uniproxy-windows-arm64.exe>) | 12220416 | 16a91ae40710ad85d5429323686a3c9c883dacc379957010b8bce80bc021748b |
| [Linux amd64](<../bin/releases/v0.2.4/uniproxy-linux-amd64>) | 12771512 | c56a73ae7b6bb1985bf10750489b3e7642cadb1a52a544721e51b0ae306cecae |
| [Linux arm64](<../bin/releases/v0.2.4/uniproxy-linux-arm64>) | 12124344 | d30f5023887ff713ba4de45eec03c4a72a1bff9dfe5f84573689dbb49a2568f7 |
| [SHA256SUMS](<../bin/releases/v0.2.4/SHA256SUMS.txt>) | 360 | 3f2e0c2aeaf2140aa9e558192943c21d66c6222fd9d5cfe56e0efd149dd40c01 |

## 结束状态与下一步

- v0.2.4 已发布并部署，旧实例已替换；不移动 v0.2.4 标签。本轮发布/部署结果文档已作为独立文档提交（[skip ci]）并推送 main。
- 用户侧真实节点连通/日常使用需另行观察；部署成功不等于节点可用或吞吐达标。不要因会话恢复自动重启/停止当前服务。
- Linux/ARM64 仅构建与摘要校验，未实机运行；Docker、race、容量/吞吐与真实远端连通未在本轮新增验证。
- 常规测试清空 UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD。既有 Docker 忽略未排除 .local，不构建/上传本机 Docker 上下文。

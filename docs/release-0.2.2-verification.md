# UniProxy v0.2.2 发布验收

日期：2026-10-20（Asia/Shanghai）。用户明确要求把启动失败诊断日志与Windows自有子进程退出保护发布为新Release。采用补丁版本0.2.2，源码提交/推送/标签与发布获准；不包含部署、停服或用户数据访问。

## 范围

- M15修复代码与回归：独立Windows错误窗口/16KiB诊断日志、健康检查不积累日志、保留CLI退出码、损坏库不清空、初始SQLite锁等待、早期取消、自有子进程Job生命周期。
- 详情及前轮实际验证见[功能验收](<windows-startup-verification.md>)；本轮[中文发布说明](<releases/v0.2.2.md>)明确无自动数据库修复、升级需先退出旧实例和禁止在线覆盖数据库。
- 仓库[FYHC1/uniproxy](<https://github.com/FYHC1/uniproxy>)当前为公开，16:42查询确认；本轮未改变可见性。保持v0.2.0/v0.2.1历史标签/资产不变。
- 仅提交源码、测试、安全可公开文档；本地诊断/恢复材料、数据/密钥/备份、二进制与生成报告排除。

## 发布前基线

- 起点HEAD=58728fb，origin/main同步，v0.2.2本地/远端标签不存在，GitHub已发布最新版本为v0.2.1。
- 原M15两次静态审查已结束，阻塞问题修正；本轮额外只读发布预审完成，无新源码/Go1.25静态兼容或二进制打包阻塞。不构建包含本地私有材料的Docker工作区上下文。
- 16:40观察用户旧实例PID31404、子进程25652，9090由用户实例监听；不读取其data/bootstrap，不停止/替换。19090空闲可供临时浏览器验证。

## 本轮验证进度

版本Go/package已设0.2.2，安全说明与发布文档已同步。本地生产Web、严格类型/noUnused、36/36工具测试、临时0.2.2候选Go全量（显式受控Mihomo与APP，集成包10.139秒）/vet与真实WM_CLOSE两轮重启和错误窗口保留均通过。仅Vite520.50kB分包提示保留，非失败。

38个待发布源码/文档路径经精确白名单与敏感内容检查通过；未见数据库/密钥/EXE/本地诊断/个人路径或令牌。31份文档、391个本地链接有效。34/34隔离真实API浏览器通过（57.4秒）；main/标签CI与Release均完成，正式资产校验结果见下文。

## 远端发布进度

- [源代码提交22849c9](<https://github.com/FYHC1/uniproxy/commit/22849c94cb7412b2c16af2f18c704fb77970b39c>)已推送main，38文件2407新增/207删除；无数据/生成物入库。
- main CI [39395787987](<https://github.com/FYHC1/uniproxy/actions/runs/39395787987>)成功：Ubuntu47秒、Windows1分34秒。通过后新建annotated v0.2.2并推送，标签指向22849c94cb7412b2c16af2f18c704fb77970b39c。
- 标签CI [39396041236](<https://github.com/FYHC1/uniproxy/actions/runs/39396041236>)成功（Ubuntu40秒、Windows2分4秒）；Release [39396041266](<https://github.com/FYHC1/uniproxy/actions/runs/39396041266>)四平台成功：linux-amd64 52秒、linux-arm64 40秒、windows-amd64 48秒、windows-arm64 40秒，release7秒。
- [v0.2.2](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.2>)于2026-10-20 16:49:21发布，正式非draft/非prerelease，设为Latest，已应用中文说明。Actions仅有Node20→24及Ubuntu镜像迁移的非失败注解。

## 正式资产校验

精确4主程序+1校验和名单，下载至忽略目录后逐一匹配清单和GitHub digest，均通过；没有用本机pre-release候选替换Release资产。

| 文件 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.2/uniproxy-windows-amd64.exe>) | 13042176 | acf54a021395d24319148a5c505ff3fe6b3127fd435ef5987eaec60a64b4b4da |
| [Windows arm64](<../bin/releases/v0.2.2/uniproxy-windows-arm64.exe>) | 12144640 | e9ada8581f59d10e767d6553810f5733457dc6bae472c5af5766b36d02b93b61 |
| [Linux amd64](<../bin/releases/v0.2.2/uniproxy-linux-amd64>) | 12685496 | f1fe0a7cc29059282a5a18a9e8d6e82b6921c9ed07c6e36f6540f86f9a7159da |
| [Linux arm64](<../bin/releases/v0.2.2/uniproxy-linux-arm64>) | 12058808 | aafbff001dd83a276655f3efd01f8a749ca890e7b3aff5588dd1c19e95d3376b |
| [SHA256SUMS](<../bin/releases/v0.2.2/SHA256SUMS.txt>) | 360 | 5305dfee2ac07f92a797b966ebb53be8dba7fd9c9e04ba58c03433a5acec6ff0 |

Windows正式文件实际版本：UniProxy0.2.2 · commit22849c94cb74 · built2026-10-20T08:48:44Z。下载后运行所有TestBuilt用例通过（包4.524秒）：CLI版本/退出码2/公开绑定保护、真实内核空配置和嵌入UI/CSP、3轮强杀父进程并确认核心随退/同目录重启、独立错误报告、重复healthcheck不积累日志。随后真实WM_CLOSE两轮关闭并重启（退出码0）、无参数错误窗口保留（关闭后退出码1）均通过。全部临时数据/显式本机内核，未使用用户数据。

16:51重新核对：原用户PID31404和内核25652仍运行，9090仍归属31404，19090无残留；本轮未停止/部署/读取配置。

## 边界

本版Mihomo仍固定1.19.32，无schema迁移，不重装内核、不改变系统网络。已有损坏数据库依赖完整备份离线恢复，进程保护不是缺页修复。Linux/WindowsARM64/Docker实机、race、所有Terminal宿主和公网节点连通不属于本轮新增验收。

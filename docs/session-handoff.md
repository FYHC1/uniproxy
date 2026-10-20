# UniProxy 会话接续快照

**最后核对：2026-10-20 16:52（Asia/Shanghai）。M16完成：v0.2.2已正式发布并设Latest（22849c9），包含M15启动诊断日志/Windows自有子进程退出保护。main/标签两平台CI和四平台Release均成功，全部下载资产SHA256与正式Windows程序关闭/重启验收通过。仓库当前公开（非本轮修改）。本轮没有部署/停止用户程序或访问其配置。**

## 30秒摘要

- 当前栈Go + React/TS + SQLite加密payload + 外置Mihomo1.19.32；M0–M16完成，最新正式[UniProxy v0.2.2](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.2>)于16:49:21发布、非draft/非prerelease。
- 发布提交22849c94cb7412b2c16af2f18c704fb77970b39c，annotated v0.2.2指向该提交；后续main仅文档回写，不移动标签或覆盖资产。
- [正式发布验收](<release-0.2.2-verification.md>)和[中文说明](<releases/v0.2.2.md>)为本轮证据；[M15开发历史](<windows-startup-verification.md>)保留独立的开发验证与限制。
- 16:51用户旧[bin程序](<../bin/uniproxy.exe>)PID31404、内核25652仍在运行，9090归属31404，19090无残留。其data未重新读取，不能推断为恢复目录；本轮不停止/替换。
- 前轮用户数据恢复已按明确授权完成，原证据/匹配密钥需保留；本轮发布没有重新打开DB/密钥/备份。公开文档不含用户配置数量/备份定位。

先读[需求](<requirements.md>)、[计划](<development-plan.md>)、[决策D22](<decisions.md>)、[开发指南](<development-guide.md>)。

## 1. 发布内容与不变边界

[入口](<../cmd/uniproxy/main.go>)在开库前检查管理端口，开库失败时报告已解析data路径；局部FlagSet保持语法退出码2。Windows无参数独立交互控制台失败保留错误等Enter，普通shell/脚本/healthcheck不等待。仅服务自有安全日志16KiB，独立用户cache唯一文件保存，失败temp回退；不记录原始参数/配置/密钥/内核日志。失败healthcheck不生成持久报告。

[存储](<../internal/store/store.go>)先设busy_timeout再WAL；[错误分类](<../internal/store/errors.go>)明确损坏库需安全恢复，不重置/清空/换空库。[processguard](<../internal/processguard/processguard.go>)在Windows任何内核子进程前建立无名非继承句柄KILL_ON_JOB_CLOSE，主进程和后续子孙绑定，句柄保留至OS退出，不能Manager.Stop/defer关闭。非Windows不变；不扫描或杀旧进程。

前轮malformed(11)为在线覆盖SQLite主库与旧WAL冲突，已有合成复现和授权恢复。进程保护/重装Mihomo不能修复丢失数据库页。升级先退出旧实例只换主程序；物理恢复/迁移源目标全停，保留匹配密钥和同代WAL/SHM，禁止在线覆盖。

Mihomo仍固定1.19.32；在线安装与应用分离。空池/失效中转均REJECT，UDP仅回环、不支持UDP in-user；模型/API/schema未改，不改变系统代理/防火墙。

## 2. 发布与正式资产

| 项目 | 事实 |
|---|---|
| main CI | [39395787987](<https://github.com/FYHC1/uniproxy/actions/runs/39395787987>)成功，Ubuntu47s/Windows1m34s |
| 标签CI | [39396041236](<https://github.com/FYHC1/uniproxy/actions/runs/39396041236>)成功，Ubuntu40s/Windows2m4s |
| Release | [39396041266](<https://github.com/FYHC1/uniproxy/actions/runs/39396041266>)四平台和发布job成功 |
| 发布 | v0.2.2，2026-10-20 16:49:21，Latest；公开仓库可下载 |
| 正式Windows身份 | 0.2.2 · commit22849c94cb74 · built2026-10-20T08:48:44Z |
| 部署 | 未执行；用户旧实例保持运行 |

| 资产 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.2/uniproxy-windows-amd64.exe>) | 13042176 | acf54a021395d24319148a5c505ff3fe6b3127fd435ef5987eaec60a64b4b4da |
| [Windows arm64](<../bin/releases/v0.2.2/uniproxy-windows-arm64.exe>) | 12144640 | e9ada8581f59d10e767d6553810f5733457dc6bae472c5af5766b36d02b93b61 |
| [Linux amd64](<../bin/releases/v0.2.2/uniproxy-linux-amd64>) | 12685496 | f1fe0a7cc29059282a5a18a9e8d6e82b6921c9ed07c6e36f6540f86f9a7159da |
| [Linux arm64](<../bin/releases/v0.2.2/uniproxy-linux-arm64>) | 12058808 | aafbff001dd83a276655f3efd01f8a749ca890e7b3aff5588dd1c19e95d3376b |
| [SHA256SUMS](<../bin/releases/v0.2.2/SHA256SUMS.txt>) | 360 | 5305dfee2ac07f92a797b966ebb53be8dba7fd9c9e04ba58c03433a5acec6ff0 |

所有文件已与清单和GitHub digest双重核对。原v0.2.0/v0.2.1资产不变；[旧本机修复候选](<../bin/updates/startup-fix/uniproxy-windows-amd64.exe>)仍0.2.1-fix.1，不是正式0.2.2；[发布前候选](<../bin/updates/release-0.2.2-local/uniproxy.exe>)commit=pre-release，只用于发布前验证，不部署冒充正式文件。

## 3. 本轮实际验证

- 生产Web/严格TS/noUnused、36/36工具、Go全量/vet（显式受控本机Mihomo和临时0.2.2候选）通过；候选WM_CLOSE两轮/错误窗口通过。
- 34/34隔离API浏览器57.4秒通过，临时19090，不访问用户data、不下载核心。
- 38审计源码/测试/安全文档精确提交，未包含本机诊断、数据/密钥/备份、二进制/测试报告/个人路径或令牌；独立只读发布预审无阻塞。
- GitHub所有CI/Release通过后，下载正式Windows文件TestBuilt全套通过4.524秒：版本/退出码2/公开绑定保护、嵌入UI/CSP、3次强杀父进程确认内核随退和同目录重启、独立日志、重复healthcheck无日志积累；随后正式文件WM_CLOSE两轮退出0、错误窗口关闭退出1通过。
- Vite520.50kB分包、ActionsNode20→24及Ubuntu镜像迁移为非失败提示；本轮未扩展工具链升级。
- 其他架构资产构建/摘要通过但无新实机运行；Linux/Docker实跑、race、所有Terminal/ConPTY、私有公网节点和吞吐仍未验收。官网下载安装opt-in本轮未开。

## 4. 收尾与后续

1. v0.2.2发布已完成，中文说明和当前文档回写后单独提交main；不重建或移动已发布标签。
2. 用户可自行退出旧实例后使用正式0.2.2，沿用原数据目录，不在线复制数据库。代为停止/替换/启动仍需新授权，本轮不部署。
3. 所有实现/调查/发布预审已结束；后台测试/CIwatch/下载/正式程序验收均已收集。没有待运行子任务，不凭旧PID做清理。
4. 现有Docker忽略规则未排除.local；本轮未构建/发送本机Docker上下文。若以后做Docker，先避免把本地私有材料带入上下文；这不影响本次干净GitHub二进制发布。
5. 本地旧诊断/恢复材料继续保留，不读取/上传/删除；当前仓库PUBLIC是查询事实而非本轮可见性修改。新请求优先于本快照。

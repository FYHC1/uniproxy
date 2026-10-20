# UniProxy 会话接续快照

**最后核对：2026-10-20 11:09（Asia/Shanghai）。M13 内置 Mihomo 在线安装实现、验证与独立审查完成；暂存开发构建0.2.1-dev。用户PID29448运行已发布v0.2.0，9090正常。本轮没有部署、提交、推送、发布或改动用户数据。**

## 当前发布轮：v0.2.1（2026-10-20 15:18，执行中）

- 用户已明确授权提交、推送 GitHub 并发布 0.2.1，取代上轮“本轮不提交/发布”的范围限制；部署仍未获授权。
- 本地与origin/main同为b638308，远端不存在v0.2.1标签或Release；私有仓库设置保持不变。
- 版本已改0.2.1，[发布说明](<releases/v0.2.1.md>)已准备；本地Go全量/真实本机内核、vet、严格TS、36工具、生产UI及0.2.1候选CLI/嵌入服务通过。34项API浏览器回归全部通过（54.3秒）；独立发布前检查无阻塞问题。发布成功状态在真实完成后回写。
- 15:17重新核对：无uniproxy/mihomo/testserver进程，9090/19090无监听。这是用户侧状态变化，非本轮停止。没有读取数据库/主密钥。
- 后续顺序：完成本地验证 → 提交推送main → 等待Windows/Linux CI → 推送v0.2.1 → 检查Release四平台资产/校验和 → 回写。

以下11:09状态与哈希保留为M13开发交付历史，不作为当前进程事实。

## 30秒摘要

- 当前栈：Go + React/TypeScript + SQLite配置payload加密 + 外置Mihomo1.19.32。内核仍是外部进程，**只是安装逻辑已内置 UniProxy，不再需要独立安装器**。
- M0–M12历史完成，v0.2.0已发布；M13/R23新增首次缺失内核提示，运行与设置分类在线安装/重检入口、后台进度/重试、严格安全下载和无覆盖发布。
- 源码与Windows/Linux四平台开发构建已暂存；**用户当前服务仍是v0.2.0，本轮功能不会因浏览器刷新而出现在旧程序中**。部署和新发布另需用户授权。
- 验证：最终Go全量（显式本地Mihomo+暂存程序）/vet/gofmt通过；严格TS、36/36工具、fixture UI、34/34隔离真实API浏览器通过；临时目录真实官方下载安装/摘要/版本/显式空配置启停通过。详见[M13验收](<core-install-verification.md>)。
- 用户服务已于10:36:40从下载目录启动，与09:25“已停止”历史观察不同。PID29448、9090 health0.2.0；本轮未查询用户bootstrap或读取DB/主密钥，不猜测其当前数据目录。

先读：[需求](<requirements.md>) → [计划](<development-plan.md>) → [决策](<decisions.md>) → [开发指南](<development-guide.md>)。PID/路径/哈希都是带日期事实，后续操作需重新核对。

## 1. 当前工作区、服务和数据

| 项目 | 2026-10-20 11:08观察 |
|---|---|
| 工作区 | 本仓库根目录，由会话运行环境提供；文档不记录个人绝对路径 |
| 版本控制 | 已有私有Git仓库；HEAD=b638308，M13为未提交工作区变更。本轮不commit/push/tag/release |
| 用户程序 | PID29448，名称uniproxy-windows-amd64.exe，10:36:40启动，位于用户下载目录；不要盲杀该进程 |
| 服务版本 | 9090回环监听，公开health返回ok=true/version0.2.0；没有用正式API做写操作 |
| 用户内核 | 观察时没有名为mihomo.exe的进程；不因此宣称已检查所有文件或数据库 |
| 当前数据路径 | 未核实，不读取用户DB/主密钥，不依据进程exe目录或旧快照推断 |
| 历史数据 | 09:25时旧数据在[bin/data](<../bin/data/>)、主密钥同行、WAL已合并；用户此前已导出备份。**本轮未重新读取其内容或搬迁** |
| 测试 | Go全部临时数据；API浏览器19090仅测试期间运行，结束清理；官网安装测试临时目录，不复用用户实例 |

禁止为确认路径/界面而使用生产数据启动暂存程序：启动即尝试apply并启动订阅/检测调度。不要运行会覆盖[旧本机程序](<../bin/uniproxy.exe>)的普通pnpm build。构建使用build:web+明确bin/updates目标。

### 最终暂存构建身份

0.2.1-dev · commit b638308+work · built 2026-10-20T03:07:29Z。标识表示未发布工作区构建，不存在对应发布tag。

| 产物 | 字节数 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/updates/uniproxy.exe>) | 13882880 | 3FEFEC0DAA895EAF6779418887644E0BDC71EFB6F4DE3E17ED47604C290638FA |
| [Windows arm64](<../bin/updates/windows-arm64/uniproxy.exe>) | 12816896 | 42AB550604C37588BB9470B6411CD9582DFF8423CF892703A04B01BC3659738F |
| [Linux amd64](<../bin/updates/linux-amd64/uniproxy>) | 13549728 | 6B0CD85861CA0057B3C6598374A29AC1423CD84329FAC283FF2479388A187AEB |
| [Linux arm64](<../bin/updates/linux-arm64/uniproxy>) | 12714144 | 78667D0382883C9099EDC686C1D71771F473399F67DE53AC457BBA6D5910A9BA |

健康200只代表存活，不等于内核/监听已就绪；哈希不等于签名或部署授权。当前源码默认Version仍0.2.0且Commit=dev，未注入构建省略提交/时间；本轮刻意注入开发身份，不冒充v0.2.0发布资产。

## 2. M13当前安装实现语义

### 检测与安装位置

[统一发现](<../internal/core/installation.go>)：显式-core/UNIPROXY_CORE_PATH权威；未覆盖时依次data/core、工作目录bin、程序同目录、PATH。已有目录/断链/不兼容文件不当作缺失，也不静默跳到另一个文件。在线默认目标为解析后的data/core；显式缺失文件路径则写该路径，仅PATH命令名不作为可写目标。支持windows/linux × amd64/arm64。

[内部下载包](<../internal/coreinstall/install.go>)没有public URL/version/force参数。固定1.19.32与四平台摘要同源；官方元数据/URL/每跳GitHubHTTPS、公共DNS数字拨号/严格Fake-IP安全复核、TLS、内置+官方摘要先验、128MiB归档/256MiB二进制、归档路径/链接/CRC等保护。不开系统HTTP代理，不执行脚本、不改网络设置。文件关闭同步后os.Link同目录原子无覆盖发布；不支持硬链接失败，不降级Rename覆盖。详情[D21](<decisions.md>)。

### 管理API与并发

[安装API](<../internal/app/core_install.go>)：

~~~text
GET /api/core/installation -> CoreInstallation
POST /api/core/install {} -> 202 CoreInstallation
POST /api/core/inspect {} -> {coreInstallation,runtime}
~~~

Bootstrap新增coreInstallation，前端共享类型/契约同步。state=idle/installing/succeeded/failed；stage=metadata/downloading/verifying/extracting/installing/inspecting/complete；字节仅归档实际下载。installed=本地路径存在，available=固定版本探测通过，running=真实进程状态，三者不能混淆。

任务在内存，不进备份/schema、不增revision；已有/运行/重复拒绝409。请求断开不取消安装，Close取消并等待。安装预约排斥Apply/Validate，下载不持管理锁；最终探测时保存跳过内核校验仍经模型校验并保持pending。安装后的首次版本进程最多60秒，普通20秒；超时保留文件，可重检而非重下。手工修复后重检会退掉旧下载失败状态，不假报原任务成功。

安装开始后当前进程awaitingCoreApply暂停自动apply，包括随后的手动配置和订阅刷新；安装成功只是available，不启动代理。显式runtime Apply清除暂停；重启管理服务保留原有启动应用语义。手动Stop逻辑不被安装误清除。

### UI

[共享安装组件](<../src/web/CoreInstall.tsx>)+[状态提供器](<../src/web/state.tsx>)：鉴权后缺失自动弹一次（sessionStorage+内存按目标路径），允许稍后处理、关闭和页面切换。运行概况与检测/端口设置均保留入口；顶端全局安装/错误可再次打开详情。安装/重新检测共享请求锁，5秒真实bootstrap轮询，正常轮询不执行版本进程；请求/会话epoch阻止旧成功/401/过时bootstrap覆盖新状态。已可用内核优先于旧失败记录。仅下载有真实百分比，校验/解压等阶段不伪造完成度。

### 分发

删除旧CLI源码；构建脚本、package脚本、后续Release、Docker均不依赖独立安装器。Docker首次WebUI安装到/data/core持久卷；历史v0.2.0发布包和本机旧安装器文件不追溯删除。许可证义务保持。

## 3. 历史关键语义（保持不变）

- M9池选择/事务：组任一/全部限定候选，默认动态纳入、取消写节点排除、全选跨分页；旧excludeGroupIds仍保留。pool+listener一个mutate/revision，失效与空池REJECT；移除池成员不删总节点。[验收](<pool-workflow-verification.md>)。
- M10：全Fake-IP域名才公共DoH复核，混合/私网/literalFakeIP不放行；订阅诊断脱敏，失败/不完整不删除。紧凑池6条分页、实际统计；组名/数量打开20条节点管理，关系移除与全局启禁分离。[验收](<subscription-groups-verification.md>)。
- M11：显示名1–160字符，导入上游标签≤1024按字符边界截断；派生名预留后缀，活动UTF-8安全截断；多页监听共享运行状态判定，dirty/revision不一致不得称监听中。[复查验收](<repository-review-verification.md>)。
- M12：平台数据标准路径+旧目录沿用、不自动迁移，版本ldflags与标签驱动发布；v0.2.0历史发布四平台+安装器/校验和已核实。[历史验收](<data-release-verification.md>)。
- 早期Clash文件授权离线验证325唯一节点/178中转等是历史事实，本轮没有再次读原文件或联网实测其凭据。禁用/缺失中转不得删dialer字段回退直连。

## 4. 本轮验证、问题与限制

完整证据和具体失败调查在[M13验收](<core-install-verification.md>)。

- 40下载安全测试函数+97子用例通过，2个OS symlink子用例因Windows权限skip；归档symlink和原子硬链接竞态通过。最终模块coverage92.0%。
- 10个安装manager、7个app API测试通过；全量Go显式本地core+暂存APP（真实代理链、配置校验、CLI/进程/UI）通过，vet/gofmt通过。
- 严格TS、36工具0skip、全fixture UI与34真实API浏览器通过。截图仅隔离fixture证据，不是用户代理连通证明。
- 真实官方下载安装/API/版本/显式空配置启动停止通过；第一次下载成功但版本检查20秒超时，改60秒冷启动+重检恢复后通过，不禁用安全软件或强制覆盖。
- 没做：Linux/Docker/Windowsarm64实跑、race（无C工具链）、公网节点/私有订阅/容量压测。Vite520.50kB/gzip153.58kB非失败分包警告保留。

## 5. 下一步和收尾

1. 本轮开发实现/最终验证完成，独立只读review未发现未解决的阻塞/P1/P2缺陷；26份文档345个本地链接全有效、代码围栏闭合。无需重做本轮功能。
2. 要让用户9090看到新安装界面，必须获准停机并核实**当前**程序/数据路径/备份后部署暂存新版，刷新页面；不能直接刷新旧发布版当作部署。本轮未获此授权、未执行。
3. 如用户要求推送/发布，再审查diff、提交并推送/打新标签；不要改写历史v0.2.0资产或假称已发布。
4. coreinstall、UI和独立review子任务均已交付，任务板完成，子任务后台作业均收集。所有Lead构建/浏览器/Go验证job已结束并收集；用户服务始终保留。独立review列出的非阻塞测试扩展见验收限制，不把它们写成已测。

下次读[AGENTS入口](<../AGENTS.md>)与本快照即可继续；不要把历史验证当成本轮重测，不凭旧PID杀进程，不读取数据库/密钥作为开发夹具。

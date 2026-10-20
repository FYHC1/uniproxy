# UniProxy 会话接续快照

**最后核对：2026-10-20 15:29（Asia/Shanghai）。v0.2.1 已正式发布并校验（功能提交d8f394e），包含M13内置Mihomo在线安装。main/标签Windows+Linux CI与四平台Release全部成功；发布资产摘要和Windows真实程序测试通过。本轮没有部署、启动/停止用户实例或读取/修改用户数据。**

## 30秒摘要

- 当前栈仍为Go + React/TypeScript + SQLite加密配置payload + 独立Mihomo1.19.32进程；安装逻辑已经内置 UniProxy，无需独立安装器。
- M0–M14完成。[v0.2.1发布页](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.1>)15:25:50发布、Latest、非draft/non-prerelease，仓库保持私有。
- v0.2.1对应提交d8f394eb644159d3725b1c42a07d87bc1d7aacf1。后续main仅回写发布文档，不移动已发布tag、不重建覆盖资产。
- 15:28重新检查无uniproxy/mihomo/testserver进程，9090/19090无监听；与11:09的用户PID29448运行v0.2.0不同。**停止不是本轮操作**，不凭旧PID或路径部署。
- 资产已下载至[本地发布目录](<../bin/releases/v0.2.1/>)并逐一核验；[Windows发布程序](<../bin/releases/v0.2.1/uniproxy-windows-amd64.exe>)是应部署的0.2.1，旧[开发暂存](<../bin/updates/uniproxy.exe>)仍为0.2.1-dev，不要混用。

先读：[需求](<requirements.md>) → [计划](<development-plan.md>) → [决策](<decisions.md>) → [开发指南](<development-guide.md>)。发布完整证据：[v0.2.1验收](<release-0.2.1-verification.md>)；M13功能历史验收：[安装验收](<core-install-verification.md>)。

## 1. 当前仓库、服务与发布产物

| 项目 | 2026-10-20 15:28观察 |
|---|---|
| 工作区 | 当前仓库根目录；文档不记录个人绝对路径 |
| 版本控制 | 私有GitHub仓库FYHC1/uniproxy；功能提交d8f394e与v0.2.1已推送；发布回写文档作为后续main提交 |
| 用户应用/内核 | 无相关运行进程，9090/19090无监听；本轮未停止/启动或替换 |
| 当前生产数据目录 | 未核实，不读取DB/主密钥，不依据exe目录猜测 |
| 历史数据 | 旧[bin/data](<../bin/data/>)和用户私有导出备份仍应保留；本轮未读取或迁移 |
| 本轮测试 | 临时数据、显式本地Mihomo、受控空配置/本机链路；浏览器19090仅隔离测试期间运行并清理 |
| 部署授权 | 仅授权GitHub推送与发布，未授权部署、系统网络设置或正式数据写入 |

### 正式v0.2.1下载文件

Windows amd64实际版本输出：0.2.1 · commit d8f394eb6441 · built 2026-10-20T07:24:53Z。以下均与Release SHA256SUMS及GitHub digest一致。

| 产物 | 字节数 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.1/uniproxy-windows-amd64.exe>) | 13004800 | 27bd13b1735f6003b46b039d39ba51331b87c58ac2004aacd0f4106b4142951e |
| [Windows arm64](<../bin/releases/v0.2.1/uniproxy-windows-arm64.exe>) | 12109824 | 6d35f3a386feb60ed09b3a71f8353f3ef2c55c05246b7db497d3f58a07910ec0 |
| [Linux amd64](<../bin/releases/v0.2.1/uniproxy-linux-amd64>) | 12665016 | de578e4612fa6c8a3509f50a3eff0f3fd59aece17f5de90438da36e5643d382a |
| [Linux arm64](<../bin/releases/v0.2.1/uniproxy-linux-arm64>) | 12058808 | c5d07a82b200738f673e1bf64afd48d094c3f0bdcacad450473dc5cca76e956d |
| [校验和](<../bin/releases/v0.2.1/SHA256SUMS.txt>) | 360 | b9197d3e381262a153a62a08bacbce083e5948b21a337a7c8ed2f615f0444818 |

四平台发布包只提供主程序与校验和，Mihomo由WebUI另行安全下载；没有独立core-install资产。源码默认Version=0.2.1，未注入commit/date的本地构建不等于正式Release；必须核对实际文件身份。

### 旧构建（历史，未删除/覆盖）

- [旧开发暂存](<../bin/updates/uniproxy.exe>)：11:07的0.2.1-dev，SHA256 3FEFEC0DAA895EAF6779418887644E0BDC71EFB6F4DE3E17ED47604C290638FA。四平台旧开发产物见[历史安装验收](<core-install-verification.md>)。
- [发布前本机候选](<../bin/updates/release-0.2.1-local/uniproxy.exe>)：0.2.1、commit pre-release，仅用于本轮候选CLI/嵌入测试，不是GitHub正式资产。
- [更早本机程序](<../bin/uniproxy.exe>)未部署替换；不能从bin名称推断用户正使用哪个文件。

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


## 3. 验证结果与明确限制

### 当前发布轮实际执行

- 提交前Go全量（显式本机Mihomo受控链路）、vet、严格TS、36工具、生产Web构建、0.2.1本机候选CLI与临时内核/嵌入服务通过。
- 34/34真实API浏览器通过，54.3秒；测试服务/浏览器双重禁止联网安装。
- main CI [39385956807](<https://github.com/FYHC1/uniproxy/actions/runs/39385956807>)、标签CI [39386285704](<https://github.com/FYHC1/uniproxy/actions/runs/39386285704>)Windows/Linux均成功；Release [39386285720](<https://github.com/FYHC1/uniproxy/actions/runs/39386285720>)四平台build和发布job全部成功。
- 五个下载资产全部SHA256通过；Windows正式程序-version正确，TestBuiltCLIVersionAndPublicBindGuard/TestBuiltServiceWithEmbeddedUI通过（临时数据、真实本机Mihomo、UI+CSP/health）。不只验证候选文件。
- 独立只读发布预审无阻塞；精确暂存71路径，源码/文档检查未见GitHub/AWS密钥、私钥块或文档个人路径；未把数据、exe或测试报告提交源码。

### M13历史功能证据（11:09，不冒充本轮重跑）

下载安全40测试函数/97子用例，coverage92.0%，2个WindowsOS符号链接子用例因权限skip；manager10/API7、fixture UI/会话/进度重试、真实临时官网下载/摘要/版本/显式空配置启停通过。首次新文件20秒probe超时后增加仅安装后的60秒预算及重检恢复，不禁用安全软件。

### 未验证与非失败警告

- Linux/Windowsarm64/Docker实机部署、race（无C工具链）、公网代理握手/私人订阅/吞吐容量压测仍未做。本轮没有再次触发官网内核下载opt-in。
- Vite520.50kB/gzip153.58kB分包警告保留；GitHub旧actions的Node20→24及ubuntu-latest镜像迁移注解不影响此次成功，但后续可维护升级。
- health200只表示管理服务存活；配置已保存/内核已安装/代理正在运行三者不同。

## 4. 必须保留的旧安全语义

固定内核1.19.32、空池/失效/中转禁用REJECT，不靠丢dialer字段直连；UDP仅回环、不提供UDP in-user认证。配置经mutate校验/一次revision保存/应用失败dirty，手动Stop或安装后的awaitingCoreApply暂停自动应用。SQLite仅payload加密，备份必须匹配主密钥，在线WAL不能只复制主库。导入预览联网与离线解析区分清楚。

池/组/订阅的历史细节见[代理池验收](<pool-workflow-verification.md>)、[订阅/分组验收](<subscription-groups-verification.md>)、[复查验收](<repository-review-verification.md>)和[数据发布验收](<data-release-verification.md>)，不要重做已完成功能或重新打开原私有文件。

## 5. 下一步与收尾

1. v0.2.1发布任务完成；发布结果文档已回写，后续main文档提交不改变正式tag/二进制。
2. 要使用新功能，用户需自行升级或明确授权部署。先重新核对程序/当前data路径和备份，正常停机后替换主程序，沿用原数据，再刷新Web；本轮没有执行此步骤。
3. 上轮与本轮子代理均完成，没有共享写工作；本轮后台构建/CIwatch/下载/验证均已收集，收尾文档独立提交推送，不改变发布tag；最终确认main远端同步后交付。
4. 未经新请求不继续扩展、升级Mihomo、删除旧数据/安装器或启动原配置。所有PID与文件哈希都有时间边界，下次操作重新核对。

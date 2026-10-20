# M13：WebUI 内置 Mihomo 安装验收

日期：2026-10-20（Asia/Shanghai）。对应 R23。源码与暂存构建已完成；**没有部署、提交、推送或发布新版本**。历史 v0.2.0 发布资产保持不变。

## 1. 交付内容

- [内置下载器](<../internal/coreinstall/install.go>)取代独立 CLI；Windows/Linux × amd64/arm64 只允许固定 Mihomo v1.19.32。SHA256 与运行版本同一来源，源码不接受任意版本、URL 或 force。
- [内核管理器](<../internal/core/installation.go>)统一发现显式路径、平台数据/core、旧 bin、程序同目录和 PATH。已存在的无效/不兼容文件不会被当作缺失自动覆盖。
- [管理 API](<../internal/app/core_install.go>)提供状态 GET、安装 POST 和显式重新检测 POST；同源/Token/Host/Origin 防护不变，POST 仅允许空 JSON 对象。HTTP202 表示后台任务已受理，不是安装成功。
- [前端](<../src/web/CoreInstall.tsx>)在已验证管理会话内缺失时提示，当前标签页/路径只自动提示一次；运行概况与检测/端口设置分类常驻入口。支持关闭、切页继续、真实进度、失败/重试、手动安装后重检。
- 安装与代理运行严格分开：安装不改配置 revision、不启动监听。当前进程内配置编辑/订阅刷新仍只保存，直到显式 Apply；重启管理服务保留原有启动应用语义。
- [构建脚本](<../scripts/build-binaries.mjs>)、[发布流程](<../.github/workflows/release.yml>)和[容器](<../Dockerfile>)仅产出 UniProxy。Docker 从面板安装到 /data/core 持久卷，不再构建时下载内核。旧独立 CLI 源码删除，旧本机二进制不擅自清理。

## 2. 安全和并发验收

1. 官方固定 release 元数据、准确下载 URL、每跳 HTTPS GitHub 域名白名单；复用公网 DNS/TLS/严格 Fake-IP 复核，禁用系统/浏览器 HTTP 代理。不暴露签名 CDN URL、响应正文、证书详情或文件系统原始错误。
2. 元数据 ≤4MiB、归档 ≤128MiB、解压后二进制 ≤256MiB。GitHub SHA256 必须与内置摘要一致，实际下载摘要核对后才解析归档；不运行远程脚本。
3. ZIP/GZIP 非平面路径、符号链接、设备、重复/多可执行文件、CRC/截断、串联 gzip 与膨胀边界均受保护；读取和下载均可取消。
4. 同目录临时文件同步关闭后 os.Link 原子无覆盖发布。另一个进程在下载期间或提交时创建目标也不覆盖；不支持硬链接则安全失败，不回退 Rename 覆盖。
5. 安装预约与 Apply/Validate 串行；下载不持有生命周期/配置锁，最后版本检查期间管理保存不等待该锁。重复请求、运行中/应用中状态拒绝；浏览器断开不取消任务，服务关闭取消并等待。
6. 首次安装后版本检查最多60秒，普通重检20秒，仍受整个任务5分钟上限。已写入的验证文件若版本检查失败，保留并给出重检方法；手工修复后也可清理旧下载失败状态，不假称原任务下载成功。

## 3. 本轮实际验证

| 验证 | 实际结果 |
|---|---|
| 下载包离线安全单测 | 40个 Test 函数、97个子用例通过；2个 OS 符号链接子用例跳过（Windows 权限不支持） |
| 下载器重复/随机顺序 | 最终 go test -count=3 -shuffle=on -timeout=60s ./internal/coreinstall 通过；40测试版覆盖率92.0%，scoped vet/gofmt通过 |
| 安装生命周期与 API 回归 | 10个内核安装测试、7个 API 测试通过，含重复、状态、权限、失败重试、断开/关闭、暂停自动应用、手工重检与最终探测不阻塞 |
| Go 全量（最终暂存程序+真实本机内核） | go test -p 1 -count=1 ./... 通过；go vet ./... 和全部 Go 源 gofmt 检查通过 |
| 严格前端类型/工具 | tsc --noEmit --noUnusedLocals --noUnusedParameters 通过；36/36工具测试，0跳过 |
| 内存 fixture UI | 全套旧用例和新安装流程通过，零未捕获错误；375/390/768/1024尺寸、键盘焦点、会话/旧响应/请求锁回归；不联网不启动服务 |
| 真实 API 浏览器 | 最终嵌入 UI 下34/34通过，48.8秒；临时19090/临时数据库，安装POST有服务端+浏览器双重离线拦截 |
| 真实官网下载/API安装 | 显式 opt-in 用临时目录下载 Windows amd64 安装包22,525,067字节，官方/内置摘要通过，真实版本探测通过；确认仍未运行，再显式空配置Apply、启动、拒绝重装、Stop通过 |
| 构建 | Windows amd64/arm64、Linux amd64/arm64交叉构建通过；Windows amd64暂存程序 CLI、嵌入UI/health、真实本机内核进程测试通过 |

### 关键命令与环境

~~~powershell
# 下载测试之外，普通测试必须清空此 opt-in，避免隐式公网访问。
$env:UNIPROXY_TEST_CORE_DOWNLOAD = ""
$env:UNIPROXY_TEST_CORE_PATH = (Resolve-Path bin/mihomo.exe).Path
$env:UNIPROXY_TEST_APP_PATH = (Resolve-Path bin/updates/uniproxy.exe).Path
$env:UNIPROXY_TEST_IMPORT_FILE = ""
$env:UNIPROXY_TEST_SUBSCRIPTION_HOST = ""
go test -p 1 -count=1 ./...
go vet ./...
node node_modules/typescript/bin/tsc --noEmit --noUnusedLocals --noUnusedParameters
node --test src/web/tests/utils.test.mjs
node src/web/tests/verify.mjs
$env:PLAYWRIGHT_CHANNEL = "chrome"
pnpm test:e2e
~~~

真实安装单独设置 UNIPROXY_TEST_CORE_DOWNLOAD=1，执行 [TestOnlineCoreDownloadThroughManagementAPI](<../tests/integration/core_install_test.go>)。该测试只访问官方发布资产；临时核心只应用空配置，不访问真实代理节点或订阅。无私有文件/订阅变量，相关旧测试明确跳过。

### 已调查的失败（不隐去）

- 首次真实下载安装/摘要/解压均完成，但20秒版本检测超时，任务如实failed且保留文件；不是下载校验失败。为新文件首次启动增加有界60秒预算并支持原文件重新检测，不修改系统安全设置。随后同一真实流程通过（测试9.83秒/包13.541秒）。不能据此断言一定是防病毒软件造成。
- 前端新增旧bootstrap响应测试的第一次最终轮因 Playwright 重复处理 route 报错，定位为测试夹具嵌套路由问题；改为单路由并等待清理后全套通过，未放宽产品断言。
- 一次新测试源码的 JSON 引号转义错误被 gofmt 捕获并修复，之后 focused/full 测试均通过。其他命令编排错误（PowerShell 管道、工具转义、读写版本guard）均已核对效果并修正，不计作通过的产品测试。

### 独立只读审查与文档核验

独立审查覆盖下载/来源/归档/原子发布、App/Manager锁序、关闭等待、安装与应用分离、UI已装/失败状态与旧会话响应保护。当前源码没有确立未解决的阻塞/P1/P2缺陷；审查期间发现的旧失败记录遮盖已恢复可用内核问题已由UI和后端重检状态恢复共同修复并重测。审查本身不冒充新测试结果。

文档26份、345个本地链接全部有效，代码围栏闭合。

非阻塞覆盖扩展仍可补充：用真实被阻塞的版本命令把最终probe和App保存/关闭联合测试；直接断言60秒/20秒/外层5分钟预算并制造探测超时（当前有模拟状态恢复和锁级快速拒绝回归）；UI旧会话inspect200/401与最终reconciliation GET中logout的专门用例。现有共享锁/epoch代码已审查，不将尚未专测的边界写成实测通过。

## 4. 构建身份和部署边界

最终暂存构建标识：0.2.1-dev · commit b638308+work · built 2026-10-20T03:07:29Z。这是基于已有提交和未提交工作区的开发产物，不是已发布tag。

| 产物 | 字节数 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/updates/uniproxy.exe>) | 13882880 | 3FEFEC0DAA895EAF6779418887644E0BDC71EFB6F4DE3E17ED47604C290638FA |
| [Windows arm64](<../bin/updates/windows-arm64/uniproxy.exe>) | 12816896 | 42AB550604C37588BB9470B6411CD9582DFF8423CF892703A04B01BC3659738F |
| [Linux amd64](<../bin/updates/linux-amd64/uniproxy>) | 13549728 | 6B0CD85861CA0057B3C6598374A29AC1423CD84329FAC283FF2479388A187AEB |
| [Linux arm64](<../bin/updates/linux-arm64/uniproxy>) | 12714144 | 78667D0382883C9099EDC686C1D71771F473399F67DE53AC457BBA6D5910A9BA |

11:08观察：用户服务PID29448，10:36:40由用户从下载目录启动的发布版，9090 health={ok:true,version:0.2.0}；未见其 Mihomo 子进程。本轮没有停止、替换或给该实例在线安装，没有查询 bootstrap/读取当前数据库和主密钥。不能沿用09:25的“实例已停止”快照，也不能仅凭旧记录猜当前数据目录。

## 5. 未验证和限制

- 未在 Linux 主机、Windows arm64 或 Docker 实际运行；四平台编译与归档合成测试不是四平台部署验收。容器持久卷权限/硬链接能力仍需目标环境验证。
- 当前 Windows 无 OS symlink 权限，2个对应文件系统子用例skip；归档符号链接/硬链接发布竞态已测。无 C 工具链，race detector 未跑。
- 没有读取用户私有订阅正文/代理配置，不做公网节点握手、吞吐或容量压力测试。
- Vite 单chunk520.50kB/gzip153.58kB仍有500kB非失败警告，未调整阈值掩盖。
- 下载主机须能安全直连官方发布服务；本功能不增加任意镜像、代理转发、绕过TLS/摘要校验或自动升级。
- 安装数据保存在服务主机，不是浏览器机器；文件系统须支持无覆盖硬链接发布。已有损坏或不兼容文件需管理员先确认并处理，不自动删/覆。

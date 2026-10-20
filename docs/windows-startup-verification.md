# Windows 启动闪退、SQLite 恢复与退出保护验收

核对日期：2026-10-20（Asia/Shanghai）。本文记录 R24 / M15 的开发验收；当时修复候选未发布。后续作为0.2.2发布准备，见[发布记录](<release-0.2.2-verification.md>)；历史[0.2.1 Release](<release-0.2.1-verification.md>)不改写。

## 1. 根因与证据

用户先报告“关闭终端后再次双击EXE一闪而过，服务没启动”，随后在PowerShell提供实际错误：

~~~text
2026/10/20 15:56:48 database disk image is malformed (11)
~~~

- 核对下载文件为正式0.2.1（d8f394e），不是旧0.2.0；当时没有UniProxy/Mihomo残留进程，9090未监听。
- 用户明确允许只读检查现库与匹配密钥、在独立临时副本验证恢复；随后允许验证旧数据库与JSON备份，再明确允许离线恢复且**暂不启动**。
- Python SQLite3.53.1在复制的证据上以只读方式执行quick_check/integrity_check，均SQLITE_CORRUPT；故不是Go驱动单独误报。库头page_size4096、page_count179，但实际文件16384字节=4页；配置记录352709字节，首溢出页10已不存在；现场没有WAL/SHM可补全缺页。
- 用户确认**在UniProxy运行期间复制/覆盖过主数据库**。库头与旧完整库相同，保留的第1/2页完全相同，第3/4页为旧活动表写回后的不同内容。
- 在纯合成临时数据上重现：先创建小库并保留活动WAL，连接不关闭时用另一大库覆盖主文件，再Close；旧WAL回写后大库截断到4页，下次Open报malformed(11)。[永久回归](<../internal/store/corruption_test.go>)只用临时合成状态，不携带用户配置。

结论：本次直接原因是**在线替换SQLite主库与仍活动的旧WAL/连接状态不一致**，退出时暴露为文件截断/损坏；“闪退”由错误仅写stderr后退出而看不到。不是Mihomo安装失败，也不能靠删密钥、删WAL、修改数据库头或自动建空库修复。普通SQLite崩溃恢复本应保留并使用同代WAL，不应将关闭终端本身等同于必然损坏。

## 2. 授权恢复（已执行，不启动服务）

仅在再次确认所有用户实例/内核已退出且9090无监听、源/目标哈希未变、没有新增sidecar、密钥相同时恢复：

- 旧完整数据库quick_check/integrity_check均ok；匹配密钥可解密，现模型校验和离线内核编译通过，无网络/代理启动。
- 比较已有数据库和JSON备份后，选择较新的完整数据库，不回退到较早备份或清空；公开文档不记录用户配置数量和备份定位信息。
- 16:09:17用同目录暂存 + Windows原子File.Replace替换当前主库，同时保留损坏原件；现有主密钥未改，当前已装Mihomo未改。损坏证据和匹配密钥保留在实际数据目录的恢复子目录，用户已获路径；不把私有数据复制到仓库/发布包。
- 恢复后再次仅只读验证解密/模型/离线编译；另在授权的临时副本用产品Store.Open/Close连续3次验证配置身份不变。后续停机复核文件哈希未变。
- 16:22用户自行启动旧bin程序（PID31404）及其内核PID25652，9090健康检查报告0.1.0；不是本轮启动/替换，未停止或访问其bootstrap/配置，后续仅保留该用户实例。其实际data目录未重新读取。

用户配置未用于普通功能/连通性测试，也没有开启其监听、订阅刷新或健康检测。原始私有备份不上传、不展示内容。本验收不声称已验证用户节点公网连通。

## 3. 程序修复范围

### 启动诊断与保存保护

- [入口](<../cmd/uniproxy/main.go>)使用可测试的局部FlagSet；无效参数仅固定错误，不把原始参数/潜在凭据写进报告。版本/帮助和脚本不阻塞。
- 管理端口先绑定，失败不会创建/打开数据库；开库前打印实际数据路径和选择原因，失败带阶段及保留数据库/密钥提示。
- [诊断模块](<../cmd/uniproxy/diagnostics.go>)只收自有安全生命周期日志，16KiB内存上界；唯一O_EXCL文件保存到用户缓存UniProxy/logs，失败回退临时目录；不覆盖已有文件，不依赖坏库。无原始内核日志/节点/密钥/环境dump。
- [Windows控制台](<../cmd/uniproxy/console_windows.go>)在启动任何子进程前检查stdin/stderr控制台、GetConsoleProcessList仅自身、无命令参数；仅这种Explorer式错误窗口保留等待Enter。已有PowerShell/cmd、重定向、无控制台、CLI参数调用不等待；关闭信号能取消等待。
- [存储分类](<../internal/store/errors.go>)将SQLITE_CORRUPT/NOTADB（含extended code）转成明确安全提示，不重置现库/密钥，不回退空库。
- [存储初始化](<../internal/store/store.go>)将busy_timeout=5000放在journal_mode=WAL之前，初始短暂锁不立即报错。该独立缺陷曾以新测试复现失败SQLITE_BUSY，修复后通过；不是本次code11根因。

### 退出与自有进程

- 主进程在慢启动前注册终止信号；App.Close取消工作与HTTP排空同时进行，不等排空结束才取消。应用Close与启动并发由现有closed/background检查和入口sync.Once协调。
- [processguard](<../internal/processguard/processguard.go>)在Windows入口任何版本探测/校验/启动子进程前启用一次。无名KILL_ON_JOB_CLOSE Job把当前进程和以后子孙绑定；句柄创建时不可继承，始终保持至OS进程退出。不得从Manager.Stop/defer关闭此Job，否则会结束主程序自身。
- 宿主嵌套Job限制导致保护建立失败时明确报错，不以breakaway绕过或静默无保护。非Windows无变化，不承诺Linux父死清理。
- 只管理该主进程创建的子孙，不按名称扫描/强杀用户旧Mihomo。此功能处理已隔离复现的强杀残留风险，**并非修复在线数据库覆盖的替代办法**。

## 4. 已执行验证

- Go全量：显式本机已验证Mihomo与最终候选，私有导入/订阅域名/在线下载opt-in为空：

~~~powershell
$env:UNIPROXY_TEST_CORE_PATH = (Resolve-Path bin/mihomo.exe).Path
$env:UNIPROXY_TEST_APP_PATH = (Resolve-Path bin/updates/startup-fix/uniproxy-windows-amd64.exe).Path
go test -p 1 -count=1 ./...
go vet ./...
~~~

  最终16:29全量通过；集成包3.505秒，包含新增在线覆盖合成回归、CLI语法退出码2、重复失败健康检查不累积日志。Go/vet与严格控制台结果均来自08:28:42Z最终构建。之后只补强错误日志测试的临时数据/缺内核/工作目录防护，以及CLI临时安全参数放在位置参数之前，全部Built测试及vet再次通过1.334秒，产品未再改变。
- [存储测试](<../internal/store/corruption_test.go>)截断库错误/原件不改/密钥不改/extended code分类；[锁测试](<../internal/store/restart_test.go>)初始锁等待；原加密round-trip与缺钥不覆盖均通过。
- [CLI/启动](<../cmd/uniproxy/main_test.go>)及[诊断](<../cmd/uniproxy/diagnostics_test.go>)通过；同临时目录两次cancel/reopen、模拟被阻塞启动取消/预先取消/只Close一次，缺内核管理仍可用，数据版本保持。健康检查失败仅stderr+非零，不写持久startup报告；CLI语法错误保留退出码2。
- [Windows进程回归](<../tests/integration/startup_windows_test.go>)真实候选连续3轮强杀主进程（不先API Stop内核）、实际内核退出、同目录同端口重启，配置版本保持；孤立错误日志和脚本不阻塞通过。原CLI/嵌入UI+CSP进程测试通过。
- [真实控制台脚本](<../scripts/test-windows-console.py>)最终候选两轮独立控制台WM_CLOSE关闭并重启，关闭退出码均0；无参数错误启动窗口保留且报告保存，关闭后退出码1。目标PID/控制台身份严格核对，不向用户控制台发信号。
- processguard协作者Windows/amd64：普通单测、10遍重复9.046秒、checkptr=2、vet全通过，无skip。强杀、正常os.Exit、外层Job场景均清理子/孙；native inherit-all + canary event证明Job句柄不可继承；32并发Enable、所有设置/释放失败路径有测试。其他3目标仅测试交叉编译。
- 独立只读审查提出的测试边界已修正：CLI大小写环境/超时/临时路径，子进程PID代际归属。最终进程测试由native CreateProcess返回父句柄，暂停时加入仅测试用outer Job再恢复；outer Job一直持有至全部断言后，所有失败路径均兜底清理，未知PID只观察不终止。子进程句柄核验真实父PID/创建时间/完整内核路径。对正式0.2.1做负对照，精确断言“core survived parent”失败（5.302秒），随后outer Job清理无残留；修复版相同测试通过0.816秒，证明不是测试兜底掩盖产品回归。
- 16:27一轮全量真实内核URL-test偶发HTTP503。排查发现旧本机HEAD fixture没有耗时，可能低于固定内核毫秒分辨率而成为0延迟；只为该测试响应增加真实3ms等待，不改产品结果/重试绕过。修复后同一真实链路20遍通过13.275秒，最终全量通过。
- 严格TS/noUnused与36/36前端纯工具测试通过；前端源码未改，沿用既有嵌入资源，候选嵌入UI实测通过。本轮没有重跑浏览器全量/在线Mihomo下载。

## 5. 最终本地候选（非Release）

版本0.2.1-fix.1，commit标识local-fix，构建时间2026-10-20T08:28:42Z；不冒充已提交/已发布。

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/updates/startup-fix/uniproxy-windows-amd64.exe>) | 13920768 | 4953A4565193554FF9B47B70C3FF5786E341E6C8A4D807FAD2D2A9FCC6CC8C58 |
| [Windows arm64](<../bin/updates/startup-fix/uniproxy-windows-arm64.exe>) | 12851712 | F272C796C5C17C37756D062E5B39D6DF82993C8701D68872B9E43B672E1C01EE |
| [Linux amd64](<../bin/updates/startup-fix/uniproxy-linux-amd64>) | 13566112 | 4AED9A965686BF8C3D2EE998C6FAADE21D3808EDE014FED8CBAE2766A6AF8A68 |
| [Linux arm64](<../bin/updates/startup-fix/uniproxy-linux-arm64>) | 12779680 | 66A12FE8E509685A8C879E605494396477D60CEC8821E8D9EAA5DD4E5A9EBBEC |

Windows/amd64真实执行；其他平台产品构建和新增cmd/store/integration测试交叉编译全部通过，不等于实机测试。模拟阻塞启动回归只证明watcher取消/Close一次的接线，尚未注入真正阻塞的App/Core.Inspect，不把它写成该组合的实测。未覆盖所有Windows Terminal/ConPTY、Linux/Docker实跑、race（无C工具链）、私人节点公网连通或吞吐。没有推送/发布/更改正式v0.2.1资产，也没有替换下载目录的用户EXE。助手遵循用户授权只恢复数据、暂不启动；随后观察到用户自行启动旧0.1.0实例，保留不动。

独立只读审查最终复核：无剩余P0/P1阻塞或确认的P2生产回归；提出的测试隔离/进程身份、健康检查持久日志、CLI退出码与控制台退出断言均已修正。未注入实际App/Core探测阻塞、stderr/写入Sync/Close失败、受限宿主Job真实拒绝等组合仍为非阻塞覆盖扩展，不冒充已测。

## 6. 后续操作

用户需先自行退出已运行的旧0.1.0实例，再使用修复候选启动已恢复的标准数据目录；无需再复制数据库或重新安装Mihomo。若需代为部署/启动或发布新Release，必须另行确认。未来物理迁移源与目标均先完全停机，保留整套一致数据和匹配密钥，不能在线覆盖主库。

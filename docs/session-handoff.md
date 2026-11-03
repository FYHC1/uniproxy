# UniProxy 会话接续快照

**最后核对：2026-11-03 10:35（Asia/Shanghai）。v0.2.8 已于 2026-11-03 10:28:28 +08 正式发布并设 Latest，源码提交 `76f3c59d0adf42bd54a9abb6e167c9328f876bb9`，annotated tag 对象 `c9567f228657d63601dbf5d89dc8e4b7e08faebf`。本版汇总 M26–M29 界面与代理池体验，发布流程全部成功；**只发布、未部署**，未停止或替换任何运行实例，未触碰用户数据目录、数据库或主密钥。当前用户实例仍为 uniproxy PID18324（临时 go-build 路径，09:30:08 启动）与内核 mihomo PID24836（09:30:09 启动），监听 `127.0.0.1:9090` 与 20006；操作前仍须重新核对。**

## 30秒摘要

- 正式发布：[UniProxy v0.2.8](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.8>)，非 draft、非 prerelease、Latest；发布时间 2026-11-03 10:28:28 +08（UTC 2026-11-03T02:28:28Z）。
- 源码提交 `76f3c59d0adf42bd54a9abb6e167c9328f876bb9`（31 文件，1172 新增 / 120 删除）；annotated tag `v0.2.8`，tag 对象 `c9567f228657d63601dbf5d89dc8e4b7e08faebf`。
- main CI [41565978602](<https://github.com/FYHC1/uniproxy/actions/runs/41565978602>)（Ubuntu 42s、Windows 1m38s）、标签 CI [41566227166](<https://github.com/FYHC1/uniproxy/actions/runs/41566227166>)（Ubuntu 38s、Windows 1m39s）、Release [41566227152](<https://github.com/FYHC1/uniproxy/actions/runs/41566227152>) 四平台 build 与 release job 全部成功。
- 正式中文说明与本地 [docs/releases/v0.2.8.md](<releases/v0.2.8.md>) 逐字一致（1911 字符）；正式资产四平台程序与 SHA256SUMS 已下载核验，见下文。
- 正式 Windows 程序 `UniProxy 0.2.8 · commit 76f3c59d0adf · built 2026-11-03T02:27:52Z`；全部 TestBuilt 通过（4.253s），真实控制台 WM_CLOSE 两轮退出 0 并重启、独立失败窗口保留后退出 1 均通过。
- 本版内容：M26 新建代理链弹窗布局；M27 全局暖纸亮色视觉刷新；M28 新建/编辑代理池布局与节点批量栏；M29 代理池卡片紧凑化与出口节点双列。纯前端，无 API/模型/SQLite schema/Mihomo 内核变更。
- **v0.2.8 只创建 GitHub Release 与附件，不部署**；当前用户实例未被触碰。若后续获部署授权，先正常退出旧实例并确认内核退出，再替换程序；禁止运行中复制或覆盖 SQLite 数据库。
- 历史 v0.2.7 标签仍为 `14cc9f570b44422b9dfc1deec94b3556749a1a05`，Release 5 个资产未变；v0.2.6 及更早标签与资产未触碰。

## 当前运行服务

- 2026-11-03 10:33 实测：uniproxy PID18324（`C:\Users\h2147\AppData\Local\Temp\go-build2592475721\b001\exe\uniproxy.exe`，09:30:08 启动）与内核 mihomo PID24836（`C:\ProgramData\UniProxy\core\mihomo.exe`，09:30:09 启动）正在运行；`127.0.0.1:9090` 归 18324，`127.0.0.1:20006` 归 24836，19090 无监听。来源未被本轮确认，保留不动；本轮未停止、替换、读取或修改用户数据库。
- 部署前置条件（若后续获授权）：确认无实例与子内核运行，或先让其正常退出并确认内核退出 → 需要备份时停机后复制 → 替换主程序并沿用原数据目录/启动参数 → health 与 `/api/core/inspect` 复核；禁止运行中复制或覆盖 SQLite 数据库，禁止混用不同库的 WAL/SHM。

## v0.2.8 正式下载副本

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.8/uniproxy-windows-amd64.exe>) | 13,355,008 | `faff52411584a2aa44092775aa4282b22e8b28efcc889163822bbd52eb63a190` |
| [Windows arm64](<../bin/releases/v0.2.8/uniproxy-windows-arm64.exe>) | 12,439,040 | `6a729707e0034dc5cbf50f5f85a870cfb44ed0f0bcaec82acaa7fabf2fe09fe0` |
| [Linux amd64](<../bin/releases/v0.2.8/uniproxy-linux-amd64>) | 12,996,792 | `4445a859aabe425f47061ca1ebd873f7596e33a48d2a9ce2b91d5359621ccc9e` |
| [Linux arm64](<../bin/releases/v0.2.8/uniproxy-linux-arm64>) | 12,386,488 | `20b956356edacd0f0adb0146a841f95dc440a1ca3a64b399e8209d070b3e106b` |
| [SHA256SUMS](<../bin/releases/v0.2.8/SHA256SUMS.txt>) | 360 | `88f7e9276434c57aac65afa42c7fae8afbdcfcdf0d6e1f362bc116de34389c1d` |

副本位于 `bin/releases/v0.2.8/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致。

## v0.2.8 发布验收（M26–M29）

- M26 新建代理链弹窗：入口监听移到名称下方、节点库上方，认证置底；入口监听单行五列；节点库桌面 3×3；端口留空提交 0、手工限 1024–65535。
- M27 全局视觉：新增最后加载的 [theme.css](<../src/web/theme.css>)，暖纸亮色、深森林墨、青绿/酸橙强调、serif 标题；用布局表达层级，隐藏重复说明，压缩表格/卡片/弹窗；390px 底部导航七列单行无溢出。
- M28 代理池编辑：监听区上移，分组固定四行内滚，节点区加宽加高，批量栏常驻；即时勾选、筛选、排序与真实延迟检测保持。
- M29 代理池卡片：每页 10 节点，节点卡首行健康/名称/延迟/操作，次行协议徽章，地址/中转/检测/错误收进展开区；成员区 >430px 两列、≤430px 单列，列表 380px 内滚，触控按钮 44px。
- 本地验证：Go 全量（显式本机内核）、gofmt/vet/diff check、严格 TS、生产 Web 构建、`pnpm test:ui` 42/42、`pnpm test:e2e` 52/52、两个 importer fuzz、TestBuilt 与真实控制台均通过。完整命令与耗时见 [v0.2.8 发布验收](<release-0.2.8-verification.md>)。
- 验证边界：Linux/ARM64 资产仅构建与摘要核对，未在本轮实机运行；Docker、race、容量/吞吐和真实远端节点连通未测；fixture UI 不等于真实代理验证。

## 相关历史与边界

- M25 代理链已随 v0.2.7 发布：2–16 跳有序合成出站、`dialer-proxy` 串接、chain 监听、整链检测与原子创建；任一跳缺失/禁用/中转异常整链 REJECT，不缩短、不回退直连。详见[代理链验收](<chain-verification.md>)。
- 2026-11-02 9router JP-US 链排障为运行配置修复：第一跳由台湾家宽改为日本原生后链测试与 20006 HTTP/SOCKS 恢复；无源码改动。
- 更早历史：v0.2.4 于 2026-11-01 12:18:53 发布并部署替换本机实例（停机备份 `C:\ProgramData\UniProxy-backups\20261101-122319-pre-v0.2.4`）；v0.2.5/v0.2.6/v0.2.7 均只发布未部署；v0.2.0–v0.2.3 标签与资产不变。
- 当前栈仍为 Go + React/TypeScript + SQLite 加密配置 payload + 外置 Mihomo 1.19.32；无 schema 迁移、无内核升级。
- 常规测试清空 `UNIPROXY_TEST_SUBSCRIPTION_MANIFEST/IMPORT_FILE/SUBSCRIPTION_HOST/CORE_DOWNLOAD` 与 `UNIPROXY_TEST_APP_PATH/UNIPROXY_TEST_CORE_PATH`。既有 Docker 忽略未排除 `.local`，不构建/上传本机 Docker 上下文。

## 历史正式下载副本（v0.2.7）

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.7/uniproxy-windows-amd64.exe>) | 13,328,896 | `9eff0c7de7cb4b9c869fdbf697abeb7a0a7f236430d98c1244779cf430d41cbb` |
| [Windows arm64](<../bin/releases/v0.2.7/uniproxy-windows-arm64.exe>) | 12,412,928 | `792f4083d87a3fc8e64d36c2165beb9ce23230d0d3326509f612f9890a9fa1db` |
| [Linux amd64](<../bin/releases/v0.2.7/uniproxy-linux-amd64>) | 12,972,216 | `1d6b937714ec60aeef58d2a6b172c21d4b104a4ee10599963b2b2cfbf4e8e1f2` |
| [Linux arm64](<../bin/releases/v0.2.7/uniproxy-linux-arm64>) | 12,320,952 | `fd549e9daa6ee72f0f906924561da87a7cca3942aa75815147f2e8db4ddb2aa4` |
| [SHA256SUMS](<../bin/releases/v0.2.7/SHA256SUMS.txt>) | 360 | `cc23e3ba0b45fb8e2949be9a86b012e7c3fcdc27fd5f25113f2f6c6264ec5739` |

副本位于 `bin/releases/v0.2.7/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致；正式 Windows 程序版本为 `UniProxy 0.2.7 · commit 14cc9f570b44 · built 2026-11-02T13:51:54Z`。

## 历史正式下载副本（v0.2.6）

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.6/uniproxy-windows-amd64.exe>) | 13,244,416 | `3757fd9218d20eceaf1078930fe1dbc9931f9619d237603292f822188205190c` |
| [Windows arm64](<../bin/releases/v0.2.6/uniproxy-windows-arm64.exe>) | 12,326,400 | `f0a72a685ffab56739585f897aa68f0543f5e775ea6df59909cc06eb62ebc26f` |
| [Linux amd64](<../bin/releases/v0.2.6/uniproxy-linux-amd64>) | 12,890,296 | `0dd697146b6f8dc1c83e36c850c736417b29e523b22c25766804771bb0f4f2fb` |
| [Linux arm64](<../bin/releases/v0.2.6/uniproxy-linux-arm64>) | 12,189,880 | `f2fdca77f67c6b833b472b398e4e0c9cd574227c9e3b452cdb3abc972a805d56` |
| [SHA256SUMS](<../bin/releases/v0.2.6/SHA256SUMS.txt>) | 360 | `a2f2cb8328f41b3afb096d4ecb3256d0d05119e1a861b6b034fd6dcb7f93b43a` |

副本位于 `bin/releases/v0.2.6/`（gitignored），五个文件与 GitHub digest 及 `SHA256SUMS.txt` 逐项一致；正式 Windows 程序版本为 `UniProxy 0.2.6 · commit dba132cb379f · built 2026-11-02T08:00:13Z`。

## 结束状态与下一步

- v0.2.8 已发布并设 Latest，**未部署**；工作树在源码提交 `76f3c59` 后仅保留本轮发布验收文档回写，随本次收尾文档提交（`[skip ci]`）推送。
- 当前用户实例 PID18324/PID24836 保持运行，未触碰；不要因会话恢复自动启动、停止或替换服务。
- 本地候选 `bin/updates/release-0.2.8/uniproxy-windows-amd64.exe` 仅为发布前验收构建，不是正式资产；正式资产以下载副本与 GitHub Release 为准。
- 用户侧真实节点连通、日常使用与吞吐需另行观察；发布与程序验收不等于节点可用或吞吐达标。
- Linux amd64 实机运行验收停留在 v0.2.5 候选（M22 修复）；v0.2.6–v0.2.8 的 Linux 资产仅构建与摘要核对。Linux arm64、裸机/Docker、race、容量/吞吐与真实远端节点未在本轮验证。

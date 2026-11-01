# UniProxy WSL 实机运行与 Linux 修复验收（amd64）

日期：2026-11-01 17:43（Asia/Shanghai）。用户先要求“使用wsl来做实际运行测试”，随后要求“修复Linux测试中遇到的问题”。本文件记录两轮 WSL2/Fedora 44 实机工作：先用官方 Linux amd64 v0.2.4 完成运行验收并发现两项问题，再用本地源码构建的修复候选复验。两轮都只使用自建回环测试源，没有连接外部代理或用户节点，没有触碰 Windows 正在运行的服务、用户数据目录、数据库或主密钥。

## 环境与资产

- WSL：Fedora Linux 44 (WSL)，WSL2，内核 6.18.54.1-microsoft-standard-WSL2，x86_64，16 CPU、7.4 GiB 内存。
- 初次验收程序：官方 [Linux amd64 发布资产](<../bin/releases/v0.2.4/uniproxy-linux-amd64>)，12,771,512 字节，SHA256 `c56a73ae7b6bb1985bf10750489b3e7642cadb1a52a544721e51b0ae306cecae`；`-version` 为 `UniProxy 0.2.4 · commit 72da273b5b81 · built 2026-11-01T04:18:10Z`。
- 修复候选：本地构建 [.local/linux-fix/uniproxy-linux-amd64](<../.local/linux-fix/uniproxy-linux-amd64>)，13,656,224 字节，SHA256 `8ff1916249b5cd6486cb3921b11aa253f1490b4110c4630b20f19460b057f2dc`；`-version` 为 `UniProxy 0.2.4 · commit local-linux- · built 2026-11-01T09:41:18Z`（本地开发构建，未注入发布提交）。该候选不是 GitHub Release 资产；修复本身已随后合入 v0.2.5 发布。
- 内核：Mihomo 1.19.32 官方 linux-amd64-compatible 归档，22,877,876 字节，SHA256 `ba3ce607747a07f948fc35780e108a4a7c7f552a38b9bd4d115f313ebcb89c20`（与内置固定摘要一致）；解压后 62,054,526 字节，SHA256 `7a0d59da2e678d56c899a3db996a2ad8963286c4f3634b0451435db248f13fa1`，`-v` 输出 `Mihomo Meta v1.19.32 linux amd64 with go1.26.8 ... / Use tags: with_gvisor`。
- 初次验收临时根目录为 `/tmp/uniproxy-wsl-d9vHBBzR`；修复复验使用 `/tmp/uniproxy-linux-fix-o3nR0VOq` 与最终候选的 `/tmp/uniproxy-linux-fix-CICrkHzY`。均为临时空数据/自建回环数据，测试结束后程序、内核与本地测试服务器均已停止，临时目录已清理，不含用户配置。

## 初次实机验收（官方 v0.2.4）

- 启动命令为 `./uniproxy -host 127.0.0.1 -port 19090 -data ./data`；主进程 PID 1029，`/api/health` 返回 `{"ok":true,"version":"0.2.4"}`，`/api/bootstrap` 初始节点/分组/池/监听均为 0，runtime 为 `running:false、dirty:true`。
- UI 根路径 HTTP 200，响应含完整 CSP（`default-src 'self'` 等）与 `X-Content-Type-Options: nosniff`；页面 730 字节，引用 `/assets/index-CLn6wq_e.css` 和 `/assets/index-CZ2krID9.js`。
- 在线内核安装被安全复核拒绝：`POST /api/core/install` 返回 202 后状态为 `failed/stage=metadata/bytes=0`，错误原文为“检测到 DNS Fake-IP，但公共 DNS 安全复核失败；请检查服务器能否访问公共 DNS 与 GitHub 后重试”。本机 WSL 的 `/etc/resolv.conf` 指向 10.255.255.254，github.com 被解析到 198.18.0.81，`https://1.1.1.1/dns-query` 超时；这是 WSL 网络环境与产品 Fake-IP 安全设计的组合，不是本轮修复的缺陷。为继续实机测试，使用官方发布归档手工放入 `data/core/mihomo`，摘要与内置固定摘要一致；`/api/core/inspect` 随后报告 installed=true、version=1.19.32、available=true。
- 自建本地测试源 `127.0.0.1:28181` 返回固定文本 `UNIPROXY-WSL-E2E-OK`，另建 `127.0.0.1:28182` HTTP 转发代理，支持绝对 URI GET 与 CONNECT。通过 API 导入节点 `http://127.0.0.1:28182#wsl-local-http`，自动建组 `wsl-e2e`；创建 mixed 监听 `wsl-e2e-entrance`（127.0.0.1:28183，udp=false，node 入口）。`POST /api/listeners` 返回 appliedRevision=2、listener listening=true、dirty=false；HTTP 与 SOCKS5 均返回 `UNIPROXY-WSL-E2E-OK`，上游日志记录 `PROXY_GET http://127.0.0.1:28181/` 与 `PROXY_CONNECT 127.0.0.1:28181`。这是回环上的真实 Mihomo HTTP/SOCKS 转发链路，不代表外部节点可用性或带宽。
- `POST /api/runtime/stop` 返回 `running:false、lastError:"代理内核已手动停止"、dirty:true`；内核进程退出，28183 端口释放。对主程序发送 SIGTERM 后 200 ms 退出；`data` 中 `uniproxy.db-wal` 与 `-shm` 消失，仅保留 16,384 字节 `uniproxy.db` 与 `master.key`，说明正常关闭完成检查点。
- 用同一 `data` 目录重启同一官方程序：health 200 仅约 200 ms；节点 1、分组 1、监听 1 保留；runtime 回到 `running:true、dirty:false、version:1.19.32、appliedRevision:2`；28183 重新监听，HTTP 与 SOCKS5 再次返回 `UNIPROXY-WSL-E2E-OK`。`-healthcheck` CLI 退出 0，未产生重复日志。
- 同一管理端口第二实例退出 1，报错“admin listen 127.0.0.1:19090 ... address already in use；请检查是否已有 UniProxy 或其他程序占用该管理端口”；`-host 0.0.0.0` 且无 Token 的新实例退出 1，报错“non-loopback admin binding requires UNIPROXY_ADMIN_TOKEN (24+ characters)”。另起临时实例设置 24+ 字符管理员 Token 到 19092：`/api/health` 200；无凭据 `/api/bootstrap` 返回 401 与“请输入有效的管理员 API Token”；带 Bearer Token 返回 200；UI 根路径 200。实例随后正常停止。
- 初次验收发现的两个问题：
  1. 冷启动首次应用存在时序问题：前两次 `POST /api/runtime/apply` 返回“配置已保存但尚未生效：private authenticated core probe did not become ready; core stopped to prevent a partially applied configuration”，再次重试才变为 `running:true、dirty:false、version:1.19.32`。旧检查使用 750 ms 就绪窗口。
  2. 直接 `kill -9` 主程序后，主程序立即消失，但其 Mihomo 子进程仍在运行并继续监听 28183；这是 Linux 侧没有 Windows Job 等同生命期保护的表现，当时 [processguard](<../internal/processguard/processguard.go>) 明确只在 Windows 生效。随后向遗留内核发送 SIGINT，200 ms 内退出，端口释放；强制终止会留下 WAL/SHM，再次用同一数据目录启动后可从 WAL 恢复，节点/分组/监听与运行配置保持。

## 本地修复与复验

- **进程生命期**：新增 [processguard.PrepareCommand](<../internal/processguard/processguard.go>)，由各平台实现；[Linux 实现](<../internal/processguard/processguard_linux.go>)在 `cmd.SysProcAttr == nil` 时设置 `Pdeathsig = syscall.SIGTERM`。这是 Linux best-effort 保护，不等同于 Windows Job Object：信号按创建线程投递，线程退出模型变化时不能保证父进程消亡后一定送达；不按进程名/PID 扫描或清理旧实例。[manager.go](<../internal/core/manager.go>)在版本探测、配置校验和启动子进程三处调用该准备函数；Windows 继续使用原有 Job Object，行为不变。
- **首次应用就绪**：将单次 750 ms 私有认证 probe 改为 [controller.go](<../internal/core/controller.go>)中有界的 3 秒外层重试（单次尝试 1 秒、间隔 100 ms）；每次仍执行原有 HTTP/SOCKS/UDP 检查，超过总期限仍失败并停止内核，绝不把未就绪状态当成功。对 `/proxies` 原生出站暴露检查也使用同样的 3 秒有界重试，避免控制器已响应但提交配置尚未可见。新增 [controller_verify_test.go](<../internal/core/controller_verify_test.go>)：`verifyProxies` 空映射后重试成功、延迟 1,200 ms 绑定的 SOCKS listener 在旧实现必失败、新实现通过；父 deadline 到期仍失败。Linux 进程保护新增 [processguard_linux_test.go](<../internal/processguard/processguard_linux_test.go>)：验证 `SIGTERM` 设置及 helper 父进程被 `SIGKILL` 后子进程消失。
- **测试**：Windows `go test -mod=readonly ./internal/core -count=1` 通过（12.506s），全量中 processguard 1.835s；最终 Windows 全量 `go test -mod=readonly -p 1 ./... -count=1` 通过（app 11.310s、core 12.106s、coreinstall 1.261s、integration 6.994s）；WSL Fedora `go test -mod=readonly ./internal/core ./internal/processguard -count=1` 通过（core 6.123s、processguard 0.010s）；`go vet ./...`、`gofmt -l internal cmd tests`、`git diff --check` 通过。
- **最终修复候选 WSL 实跑**：使用 `.local/linux-fix/uniproxy-linux-amd64`（SHA256 `8ff1916249b5cd6486cb3921b11aa253f1490b4110c4630b20f19460b057f2dc`）与固定 Mihomo 1.19.32。health 0.2.4、core inspect installed 1.19.32；导入本机回环节点、创建 mixed 监听 28183；`POST /api/runtime/apply` 首次即返回 HTTP 200、`running:true`、`appliedRevision:2`，没有重复之前的 probe 失败；HTTP/SOCKS 均返回 `UNIPROXY-WSL-E2E-OK`。`kill -9` 主程序后 `CORE_AFTER_KILL9_ALIVE=no`，28183 端口释放；同一数据目录重启后 health 正常、节点/监听保留、runtime `running:true`，HTTP 流量恢复；最终 `runtime/stop` + SIGTERM 后 `FINAL_CORE=NONE`。

## 结论与边界

- 两个 WSL 实机问题已在本地源码和本地 Linux 候选上修复并复验：首次 apply 不再需要重试，`kill -9` 不再遗留 Mihomo。正式 v0.2.4 GitHub Release 资产未包含该修复；修复已提交 main（`ede12bb`）并随 v0.2.5 发布（不含部署）。
- 边界：仅 Fedora 44 WSL2 amd64；Linux arm64 仍是交叉构建未实机；未覆盖裸机/其他发行版、Docker、systemd/服务管理、Linux 原生异常退出在更多线程模型下的保证、UDP、并发/容量/吞吐、真实公网节点和 Windows 服务变更。测试内核与测试源均为本机回环合成数据，不代表真实订阅或远端节点可用性。
- 本机证据：`.local/wsl-runtime-verify/evidence/`（初次验收，未入库）与 `.local/linux-fix/`（修复候选/脚本，未入库）；临时根目录已在测试后清理。
- Windows 运行服务复核（2026-11-01 17:48）：PID16568 官方 `bin/releases/v0.2.4/uniproxy-windows-amd64.exe` + 内核 PID28844，9090 归 16568，19090 无监听，health 0.2.4；本环节未停止、替换或修改该服务。

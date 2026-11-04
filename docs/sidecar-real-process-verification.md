# UniProxy 多引擎 Sidecar 真实流程验证（第 1–4 层 + 安装器）

日期：2026-11-04 11:43–17:03（Asia/Shanghai）。本文记录真实流程测试第 1–4 层：Windows 11 amd64 本机真实进程闭环、VLESS Encryption/VMess XHTTP/Reality 三协议本地端到端、WSL2 Fedora Linux amd64 实跑、kr-vps 21443 的真实远端订阅端到端，以及第 5 节安装器真实 GitHub 资产下载。未启动、停止、替换用户实例，未读取或修改用户数据库/主密钥。

## 1. 环境与资产

- Windows：Windows 11 专业工作站版 10.0.26300.0，amd64；Go 1.27.2 windows/amd64。
- WSL：Fedora Linux 44 (WSL2)，内核 6.18.54.1-microsoft-standard-WSL2，x86_64；Go 1.26.10 linux/amd64。
- Xray：26.10.31 a4a3dff。
  - Windows：`C:\CustomFiles\CodeProjects\Github\uniproxy\.local\xray-test\xray.exe`，SHA256 `D5B558191CCCB1E92A5772B68B8CAB67717B26F634B0F15153215F65AE982A7E`。
  - WSL：`/opt/uniproxy-e2e/xray`，SHA256 `cc4b84d3069569f297a61f04b914f2bb37e9d202fd7b84cb50def1fc30fd40e1`。
- Mihomo：Meta v1.19.32 with_gvisor。
  - Windows：`C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe`，SHA256 `04F8D7FC2B314771E1ECBF15951D59F0E1CB7914503C88CFB6972FF6A824E314`。
  - WSL：`/opt/uniproxy-e2e/mihomo`，SHA256 `3122d100e8177501776109f1a6253a694611627cf4d7c7ec82705855cf8626a8`。
- 测试源码：`internal/e2e/real_process_test.go` 的 `TestRealAppProcessThreeProtocols`。测试数据全部为临时目录和 127.0.0.1 合成数据，不使用用户节点、订阅或数据库。

## 2. 测试结构

1. 用 Xray CLI 生成 VLESS Encryption（vlessenc 的 encryption/decryption）、Reality X25519 密钥和 mldsa65 seed/verify。
2. 在 127.0.0.1 启动临时 Xray 服务端，包含三个 inbound：`vless-enc-in`（VLESS Encryption）、`vmess-xhttp-in`（VMess XHTTP，path /xhttp）、`reality-in`（VLESS Reality + xtls-rprx-vision，目标为大证书 TLS 服务）。
3. 用 `store.Open(dataDir).Save` 预置真实 state：3 个 `EngineXray` 节点、3 个 mixed listener、`enableSidecar=true`、sidecar 端口范围 32400–32499。
4. 构建并启动真实 `cmd/uniproxy` 子进程，传 `-core <mihomo>` 与 `-xray <xray>`；轮询 `GET /api/health`、`GET /api/sidecars`、`GET /api/runtime`。
5. 通过每个 mixed listener 的 HTTP 代理访问本机 httptest origin，确认请求路径为 Mihomo → 本地 socks5 sidecar → Xray → 合成上游，返回 `real-e2e-ok`。
6. `POST /api/runtime/stop` 确认 sidecar 状态转为 stopped；结束进程后用同一 dataDir 重启，确认 sidecar/runtime 自动恢复、首个 listener 再次转发成功。
7. 再次启动后从 `GET /api/bootstrap` 读取 3 个 sidecar 端点端口，直接强杀主程序，逐一确认 listener 端口和 3 个 sidecar 端口释放（Windows Job Object / Linux Pdeathsig 行为）。

## 3. 执行命令与结果

Windows（worktree `C:\CustomFiles\CodeProjects\Github\uniproxy-sidecar`）：

```powershell
$env:UNIPROXY_E2E_REAL='1'
$env:UNIPROXY_E2E_XRAY='C:\CustomFiles\CodeProjects\Github\uniproxy\.local\xray-test\xray.exe'
$env:UNIPROXY_E2E_MIHOMO='C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe'
go test ./internal/e2e -run TestRealAppProcessThreeProtocols -count=1 -v -timeout 5m
```

结果：PASS，2.74s（11:38 首跑；11:42 复跑 2.82s）。日志：

```text
vless-enc through application process: real-e2e-ok
vmess-xhttp through application process: real-e2e-ok
reality through application process: real-e2e-ok
restart through application process: real-e2e-ok
abrupt application exit released 4 owned ports
```

WSL2 Fedora（源码复制到 `/opt/uniproxy-sidecar`，保留 /mnt/c 原始工作树不变）：

```bash
cd /opt/uniproxy-sidecar
export UNIPROXY_E2E_REAL=1
export UNIPROXY_E2E_XRAY=/opt/uniproxy-e2e/xray
export UNIPROXY_E2E_MIHOMO=/opt/uniproxy-e2e/mihomo
go test ./internal/e2e -run TestRealAppProcessThreeProtocols -count=1 -v -timeout 5m
```

结果：PASS，2.12s（11:39 首跑；11:42 复跑 2.26s）。同四项 `real-e2e-ok` 与 `abrupt application exit released 4 owned ports`；WSL 首轮冷构建耗时 15.49s，缓存后为 2.12s。

Phase 0 直接管理器 POC 复跑：

- Windows：`UNIPROXY_XRAY_BIN=...xray.exe go test ./internal/sidecar -run 'TestPhase0XrayPOC|TestBuildXrayConfigAcceptedByXray' -count=1 -v -timeout 5m` → PASS（Phase 0 0.50s；BuildXrayConfigAcceptedByXray 0.05s）。
- WSL：`UNIPROXY_XRAY_BIN=/opt/uniproxy-e2e/xray go test ./internal/sidecar -run TestPhase0XrayPOC -count=1 -v -timeout 5m` → PASS（0.38s），vless-enc 32300、vmess-xhttp 32301、reality 32302 均转发成功。

全量 Go：

- Windows：`go test ./... -count=1 -p 1` → 全部 PASS（internal/app 9.014s、core 6.633s、importer 0.751s、sidecar 0.820s 等）。
- WSL：`go test ./... -count=1 -p 1` → 全部 PASS（internal/app 2.390s、core 6.091s、sidecar 0.583s 等）。

## 4. 第 4 层：kr-vps 真实远端订阅 E2E

- 远端：`kr-vps`（SSH `210.109.205.7:20022`）为 Alpine Linux 3.24.2 x86_64 容器，公网地址 `210.109.205.7`。该环境只有 21443 真正映射到容器内的 Xray；21444/21445 虽能建立 TCP，但不会转发到 Xray，因此三协议采用每次只把一个协议部署到 21443 的顺序验收，不把“三协议并发在线”当作已验证事实。
- 远端 Xray：官方 `Xray-linux-64.zip` 16,692,690 bytes，SHA256 `e4bd9119f50ae7c41658b1abd0679ae06f98bd864a2907ff33f2f2cf7081b214`，与安装器内置 linux-amd64 pin 一致；版本 `Xray 26.10.31 (Xray, Penetrates Everything.) a4a3dff (go1.27.3 linux/amd64)`。
- 订阅与目标由既有 20080 darkhttpd 提供：订阅 `http://210.109.205.7:20080/links-xray.txt`；目标 `http://210.109.205.7:20080/ok-remote.txt` 返回 `remote-e2e-ok`。Xray 的 freedom 出站默认阻止回环目标（`blocked target: tcp:127.0.0.1:20080`），所以测试目标必须使用公网地址，不能用远端 `127.0.0.1`。
- 测试源码：`internal/e2e/real_remote_test.go` 的 `TestRealRemoteSubscription`。设置 `UNIPROXY_E2E_REMOTE_SUB` 启用；可选 `UNIPROXY_E2E_REMOTE_TARGET`、`UNIPROXY_E2E_REMOTE_NODES`（默认 3）、`UNIPROXY_E2E_REMOTE_NAMES`（逗号分隔，可选）。流程为 HTTP 订阅抓取 → `POST /api/import mode=subscriptions` → 校验 `EngineXray` 节点与 sidecar 状态 → 启动本地 Xray sidecar → 批量创建 mixed listener → 经 listener 访问远端目标 → 强制刷新订阅再验证 → 结束并重启应用后再次验证。
- 命令（每个协议前重新部署对应的单协议链接）：

```powershell
$env:UNIPROXY_E2E_REMOTE_SUB='http://210.109.205.7:20080/links-xray-vless.txt' # 或 -vmess/-reality
$env:UNIPROXY_E2E_REMOTE_TARGET='http://210.109.205.7:20080/ok-remote.txt'
$env:UNIPROXY_E2E_REMOTE_NODES='1'
$env:UNIPROXY_E2E_XRAY='C:\CustomFiles\CodeProjects\Github\uniproxy\.local\xray-test\xray.exe'
$env:UNIPROXY_E2E_MIHOMO='C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe'
go test ./internal/e2e -run '^TestRealRemoteSubscription$' -count=1 -v
```

- 结果（2026-11-04 16:40–16:41）：VLESS Encryption PASS 10.23s；VMess XHTTP PASS 6.12s；Reality PASS 5.66s。每个协议均依次通过“首次导入、强制刷新、应用重启”三次远端读取，三处日志均为 `remote target through listener 20000: remote-e2e-ok`。侧车状态为 `engine=xray, version=26.10.31, running=true`，本地端点为 `127.0.0.1:32400`。
- VLESS Encryption 的 `.native.0rtt.<ticket>` 约 600 秒过期；过期后服务端报 `ML-KEM-768 handshake failed > expired ticket`。每次测试前必须用 `xray vlessenc` 重新生成 server decryption 与 link encryption 并重启服务。测试结束后远端 21443 保留 Reality fixture，订阅链接长期有效。
- 调试用 `internal/sidecar/debug_remote_temp_test.go`、本地 port-mux 程序与临时客户端配置已在验证后删除；未提交、未发布、未部署。

## 5. 安装器真实 GitHub 下载验证（2026-11-04）

- 目标：验证 `internal/sidecarinstall.Install` 的生产路径能否从真实 GitHub 获取官方 Windows amd64 资产，完成下载、摘要校验、多文件 ZIP 解压、无覆盖发布和 `xray.exe version` 检测。
- 固定资产：`Xray-windows-64.zip`，16,623,726 bytes，SHA256 `2e04a9995a6f2ad1b28c0bb25c708ecb2695e43a1c9c7889c4b6ac2b48cf7a6a`，与 `sidecarinstall` 内置 windows-amd64 pin 一致。
- 直接下载：`gh release download v26.10.31 --repo XTLS/Xray-core -p Xray-windows-64.zip` 得到的 ZIP 摘要与 pin 完全一致；展开后含 xray.exe 34,555,392、geoip.dat 3,312,784、geosite.dat 7,554,929、wintun.dll 427,552、LICENSE 16,725、LICENSE-Wintun 5,431、README.md 12,666 bytes；`xray.exe version` 为 `Xray 26.10.31 (Xray, Penetrates Everything.) a4a3dff (go1.27.3 windows/amd64) A unified platform for anti-censorship.`。
- 生产安装路径：临时测试 `TestLiveGitHubInstall` 通过 `gh api` 读取真实 release metadata，仅把 `api.github.com` 的响应注入测试 transport；所有资产字节仍由真实 GitHub CDN 下载并交给 `install(...)`。结果 PASS 4.60s：阶段 `metadata → downloading → verifying → extracting → installing`，下载 16,623,726 bytes（3.784s），发布目录含 xray.exe/geoip.dat/geosite.dat/wintun.dll/LICENSE/LICENSE-Wintun，版本输出包含 26.10.31。
- 匿名 GitHub API 边界：本机共享出口的未认证配额已耗尽，直接调用生产 `Install` 时返回 `GitHub 返回 HTTP 403（访问受限或请求过于频繁）；请稍后重试，并检查服务器能否直连 GitHub`；响应头为 `X-RateLimit-Limit: 60`、`Remaining: 0`、`Used: 60`、`Reset: 1793786276`（2026-11-04 17:57:56 +08）。因此上述生产路径测试使用已认证 `gh api` 获取的同一真实 metadata 注入 API 响应；安装器本身未改为接受 token 或跳过 GitHub 元数据检查。
- 验证（2026-11-04 17:03）：删除临时测试后 `go vet ./...` 无输出、`go test ./... -count=1 -p 1` 全通过（app 11.143s、core 6.822s、sidecar 0.874s、sidecarinstall 0.204s、importer 0.830s、store 0.361s）。
- 清理：临时测试文件已删除；本轮验证不修改生产代码、不启动/停止用户实例、不读取用户数据库/主密钥，未提交、未发布、未部署。

## 6. 观察与边界

- Reality inbound 的大证书握手在测试日志中会产生预期内的 TLS handshake 错误（Reality 回落/探测路径），不影响三协议断言；这些日志不包含凭据。
- 强杀后 4 个自有端口释放，说明本轮 Windows 子进程 Job Object 与 Linux Pdeathsig 路径在测试场景下生效；不证明所有异常退出、线程迁移或受限宿主环境。
- 本地层仅验证 127.0.0.1 合成上游和本地 Xray 服务端；远端层只验证 kr-vps 单端口 21443、单节点、TCP HTTP 单目标，三协议为顺序切换而非同时在线。未验证多端口映射、UDP、公网带宽、并发/容量、长期运行、其他 VPS 发行版或裸机部署。
- kr-vps 为 Alpine 容器，不能代表物理机、systemd、防火墙或 NAT 完整环境；远端订阅与目标均由同一测试主机提供，不证明第三方订阅源或真实业务上游。
- 安装器真实下载只验证 Windows amd64 官方资产和本机出网路径；未验证 Linux 实机安装、非 WSL 裸机、GitHub 匿名 API 配额恢复后的完整 API 直连，以及 GitHub 服务端故障/限流时的重试行为。
- 本轮未提交、未推送、未发布、未部署；用户实例与数据目录未触碰。

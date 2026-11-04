# UniProxy：Mihomo 主核 + Xray / sing-box 双 sidecar 需求分析与实施交接

- 文档日期：2026-11-04
- 目标分支：`feat/multi-engine-sidecars`
- 开发工作目录：`C:\CustomFiles\CodeProjects\Github\uniproxy-sidecar`
- 基线：`main` = `eb7675c`（v0.2.9 发布后的文档提交）；发布 tag `v0.2.9` = `6ff15a8`
- 当前状态：第一至四批（模型字段、`internal/sidecar`、编译器本地 SOCKS、链位置校验、3 个离线 POC、app/API、UI、端口范围与预检）、真实进程 E2E、importer Xray-only 识别与真实订阅验收，以及固定版本 Xray 在线安装器/多文件 ZIP 安全解压/app/API/设置页安装状态已完成并验证。真实 GitHub 资产下载与生产安装路径已验证（Windows amd64 官方 ZIP，摘要匹配内置 pin）；匿名 GitHub API 配额耗尽时仍会返回 403；Linux 实机安装待验证。未提交、未发布、未部署。

## 0. 结论先行

**推荐方案：保留 Mihomo 为主核，Xray 作为 Xray-only 节点的受限本地 sidecar；sing-box sidecar 暂缓，只有明确需要 AnyTLS、Snell、ShadowTLS、Naive、Tor 等 R01 之外的协议时再引入。**

“全协议完美支持”需要改成可验证的能力矩阵：

- Mihomo 负责 R01 的 Hysteria2、TUIC、SSH、SOCKS5、HTTP 等，以及 R05 的四种原生池策略、监听、Clash API 和正常热更新。
- Xray 负责 Mihomo 1.19.32 未覆盖的现代 VLESS/VMess 字段：VLESS Encryption、VLESS/VMess XHTTP、Reality spiderX/mldsa65Verify、FinalMask、flow=xtls-rprx-vision-udp443 等。
- sing-box 能补 Hysteria2/TUIC/SSH，但这几项 Mihomo 已经覆盖；它的独立价值主要是 R01 之外的 AnyTLS/Snell/ShadowTLS/Naive/Tor 等协议，不作为当前双 sidecar 的默认必选项。

**主核结论：继续 Mihomo。** 换成 Xray 会丢 Hysteria2/TUIC/SSH，换成 sing-box 会丢 XHTTP、VLESS Encryption、VMess KCP 和现有四种池策略；当前产品范围不支持这两条主核迁移路线。

## 1. 背景与证据

### 1.1 当前项目基线

- 技术栈：Go 管理服务 + React/TypeScript + SQLite 加密配置快照 + 外置固定 Mihomo 1.19.32。
- 当前发布：v0.2.9，提交 `6ff15a8`，tag `v0.2.9`；`main` 在其后只有文档提交 `eb7675c`。
- 当前解析结果（真实样本 1064 条）：严格 `Parse` 697 条成功；`ParsePreview` 697 严格 + 365 降级 = 1062 条可展示；剩余 2 条是无效 UUID 和未知导出器参数组合。
- 解析成功不等于节点可用；真实连通、握手、UDP、吞吐仍需独立检测。

### 1.2 Xray 能力实测

Xray v26.10.31，本地验证：

- `xray vlessenc` 生成四段式 VLESS Encryption，ML-KEM-768 密钥块解码为 **1184 字节**，与当前样本中的 1184 字节 key 对应。
- `xray run -test -c` 接受同时包含 `VLESS Encryption + flow=xtls-rprx-vision-udp443` 和 `VMess network=xhttp` 的配置，输出 `Configuration OK`。
- Xray 同时提示 VMess 已弃用（no Forward Secrecy），建议迁移到 VLESS Encryption。

### 1.3 Xray 的能力边界

- 支持：VLESS Encryption、VLESS XHTTP、VMess XHTTP、Reality spiderX、mldsa65Verify、FinalMask、flow=xtls-rprx-vision-udp443。
- 不支持：Hysteria2、TUIC、SSH（v26.10.31 代码树未发现对应 outbound）。
- 池策略：balancer 只有 random、roundRobin、leastPing、leastLoad；没有 consistent-hashing、sticky-sessions。
- 控制面：只有 gRPC API（HandlerService、RoutingService、Stats、Observatory），没有 Clash HTTP API。
- 热更新：`xray run` 只监听 Interrupt/SIGTERM，没有 SIGHUP；动态变更通过 gRPC 或重启进程。

### 1.4 sing-box 的能力边界

- 支持：Hysteria2、TUIC、SSH、AnyTLS、Snell、ShadowTLS、Naive、Tor 等；有 Clash API。
- 缺少：XHTTP、VLESS Encryption、VMess KCP；只有 selector/urltest，没有四种负载均衡组。
- 因此：在 Mihomo 主核已经覆盖 Hysteria2/TUIC/SSH 的前提下，sing-box sidecar 对当前 R01 的边际价值很小。

## 2. 需求分析

### 2.1 功能性需求

| 编号 | 需求 | 验收标准 |
|---|---|---|
| F1 | Mihomo 仍是唯一主核，统一接收客户端 HTTP/SOCKS/mixed 流量 | 现有监听、认证、API、热更新行为保持；Mihomo 原生节点行为不变 |
| F2 | 节点可选择引擎：`mihomo`（默认）、`xray`，预留 `sing-box` | 导入、编辑、预览、列表、备份都能保留引擎字段；旧数据默认 mihomo |
| F3 | Xray-only 节点由 Xray sidecar 处理 | 至少覆盖 VLESS Encryption、VLESS/VMess XHTTP、Reality spiderX/mldsa65Verify、FinalMask、flow=xtls-rprx-vision-udp443 中的明确样本 |
| F4 | sidecar 只提供本地 SOCKS 出站，不监听公网入口 | 绑定 127.0.0.1；随机用户名密码；Mihomo 侧配置为 `socks5` 出站；无公网监听 |
| F5 | 一个引擎一个进程，节点通过多 inbound 映射 | 不为每个节点启动进程；sidecar 端口独立分配；进程崩溃时相关节点标记不可用 |
| F6 | Mihomo 池、监听、链继续可用 | sidecar 节点在池中可测延迟；sidecar 节点可作为单独出站或链首；不允许静默直连回退 |
| F7 | 链位置限制明确 | 本地 sidecar 节点只允许单独出站或链首；在链中间/链尾保存时直接报错，并给出原因 |
| F8 | 应用事务化 | 顺序：sidecar 配置校验 → 启动 sidecar → 本地 SOCKS 可用 → Mihomo 编译/校验/应用 → 健康检查；失败回滚并保持旧配置 |
| F9 | 安装与升级 | Xray/sing-box 固定版本、官方来源、SHA256、大小限制、安全解压；sidecar 缺失时主服务仍可启动，只影响相关节点 |
| F10 | 前端可见可控 | 节点列表显示引擎徽章；设置/状态页显示 sidecar 版本、运行状态、端口、错误；安装页可安装/重试 |
| F11 | 不泄露秘密 | 节点凭据、sidecar 认证信息不进入日志、前端列表或错误详情；预览默认清空 Config |
| F12 | 与现有 API 兼容 | 现有 Core 接口和 API 形状尽量不变；新增字段有默认值，旧 SQLite payload 无需迁移即可读 |

### 2.2 非功能性需求

- **规模**：第一阶段 limit 200 个 sidecar 节点；超过时直接报错并提示，并提示按需拆分/只对 Xray-only 节点使用。
- **端口**：sidecar 端口使用独立范围（建议 30000–30999），不与现有 20000–20999 监听端口冲突；保存/应用前做 bind 预检。
- **进程**：最多一个 Xray 进程和一个 sing-box 进程；启动、停止、崩溃重启纳入 processguard。
- **性能**：本地 SOCKS 多一跳，允许小幅延迟/CPU 增加；不承诺吞吐提升；需要和原生 Mihomo 节点做 A/B。
- **可用性**：sidecar 异常只影响其节点，不拖垮 Mihomo 原生节点、监听和其他池。
- **可测性**：所有 sidecar 配置可离线生成、`xray -test`/`sing-box check` 通过；不依赖真实公网节点即可做集成测试。
- **安全边界**：sidecar 仅回环；Xray/sing-box 管理 API 默认关闭或仅本地；长期明文凭据不落盘。
- **兼容性**：Windows/Linux amd64/arm64 的安装/启动矩阵分阶段覆盖；至少先 Windows amd64 + Linux amd64。

### 2.3 非目标（第一阶段）

- 不做三个对等主核。
- 不做远程 sidecar（可后续扩展）。
- 不做“整条链封装进 sidecar”。
- 不承诺“任意订阅、任意未来字段全部可用”；只承诺明确的 capability matrix。
- 不把 Xray/sing-box 的 API 当成 Mihomo Clash API 的完整替代。

## 3. 架构设计

### 3.1 数据流

```text
客户端
  │ HTTP / SOCKS / mixed（现有 Mihomo 监听）
  ▼
Mihomo 主核
  ├─ 原生 outbound ─────────────────────────────► 远端节点
  ├─ socks5 127.0.0.1:30001 ─► Xray sidecar ───► Xray-only 节点
  └─ socks5 127.0.0.1:30002 ─► sing-box sidecar ─► sing-box-only 节点
```

Mihomo 是唯一对外入口和唯一调度层；sidecar 只是“本地 SOCKS 出站提供者”。

### 3.2 节点模型

建议新增：

```go
type NodeEngine string

const (
    EngineMihomo  NodeEngine = "mihomo"
    EngineXray    NodeEngine = "xray"
    EngineSingBox NodeEngine = "sing-box"
)

type SidecarEndpoint struct {
    Host     string `json:"host" yaml:"host"`
    Port     int    `json:"port" yaml:"port"`
    Username string `json:"username,omitempty" yaml:"username,omitempty"`
    Password string `json:"password,omitempty" yaml:"password,omitempty"`
}
```

规则：

- `Node.Engine` 为空按 `mihomo` 处理，保证旧数据兼容。
- `Node.Server`/`Node.Port` 保留真实远端地址，用于 UI、导入和 sidecar 内部配置。
- `Node.SidecarEndpoint` 是 Mihomo 看到的本地入口；编译器只能把 sidecar 节点写成 `socks5` + 这个 endpoint。
- 不要把真实远端地址写进 Mihomo 的 sidecar outbound，否则会绕过 sidecar。
- sidecar 真实凭据只存在 sidecar 配置和加密 state 中，不进入日志/预览。

### 3.3 sidecar 进程

- 一个 Xray 进程，一个 sing-box 进程；每个进程一个配置文档，多 inbound/outbound。
- 每个 sidecar 节点对应一个本地 SOCKS inbound：
  - Xray：`protocol: socks`，`listen: 127.0.0.1`，`auth: password`，routing rule inbound→outbound。
  - sing-box：`type: socks`，`listen: 127.0.0.1`，`users`，route rule inbound→outbound。
- 启动顺序：生成配置 → `xray run -test -c` / `sing-box check -c` → 启动 sidecar → 等待本地端口 → 再应用 Mihomo。
- 停止顺序：先停 Mihomo 相关出站/应用旧配置 → 停 sidecar；避免残留连接。
- 进程崩溃：标记该 sidecar 全部节点不可用；Mihomo 相关池成员返回失败/REJECT，不直连回退。

### 3.4 编译器

- `internal/core/compiler.go` 对 `EngineXray/EngineSingBox` 生成：
  ```yaml
  - name: x-<NodeID>
    type: socks5
    server: 127.0.0.1
    port: <SidecarEndpoint.Port>
    username: <SidecarEndpoint.Username>
    password: <SidecarEndpoint.Password>
    udp: <Node.UDP>
  ```
- 池的成员保持 node tag，Mihomo 原生 group 继续工作。
- 健康检查仍走 Mihomo `/proxies/x-<NodeID>/delay`；sidecar 本地进程失败会表现为延迟失败。
- `nativeProxy` 白名单继续只允许 Mihomo 原生类型；sidecar 节点走独立的 `sidecarProxy` 生成路径。

### 3.5 链位置约束（关键）

Mihomo 的链是“每跳 outbound 的 server 是下一跳地址；`dialer-proxy` 只改拨号路径；协议由 outbound `type` 处理”。

- **本地 sidecar 节点作为单独出站/链首**：Mihomo 连本地 SOCKS，sidecar 处理真实协议，可行。
- **本地 sidecar 节点作为链中间/链尾**：前一个远端跳会访问它自己的 127.0.0.1，到不了 UniProxy 本机 sidecar；保存时必须报错。
- **远程 sidecar**：如果 sidecar 部署在可达主机，`SidecarEndpoint.Host` 是 sidecar 主机地址，不是真实节点地址；可以放任意位置，但属于后续阶段。
- **整链封装**：如果一条链包含多个 Xray-only 节点，未来可在 sidecar 内部完成整链，Mihomo 只看到一个本地 socks；第一阶段不实现。

因此第一阶段校验规则：

```text
if node.Engine != mihomo:
    node 只能单独使用，或位于 chain[0]
    若位于 chain[1:]：报错“sidecar 节点只允许位于链首”
```

这条规则必须前后端一致：导入、编辑、保存、编译、API 校验都要覆盖。

### 3.6 安装

- Xray 发布包为多文件：Windows ZIP 含 `xray.exe`、`geoip.dat`、`geosite.dat`、`wintun.dll`；Linux 同样需要 geo 文件。
- sing-box 发布包为归档，含单二进制（具体资产名按版本固定）。
- 新增 `internal/sidecarinstall` 或扩展 `internal/coreinstall`：固定版本、官方 release、SHA256、最大尺寸、安全解压、同目录无覆盖发布。
- 安装缺失不阻断主服务启动；只影响对应引擎节点。
- 版本固定，不自动升级；升级需重新跑兼容矩阵。

## 4. 能力矩阵

| 能力 | Mihomo 1.19.32 | Xray 26.10.31 | sing-box 1.14.4 | 使用引擎 |
|---|---|---|---|---|
| Hysteria2 / TUIC / SSH | 支持 | 未发现 | 支持 | Mihomo |
| VLESS Encryption | 解析器支持 1184 字节；实际连接需验证 | 原生支持 | 无 | Xray |
| VLESS XHTTP | 部分支持 | 原生支持 | 无 | Xray |
| VMess XHTTP | D27 排除 | 支持 | 无 | Xray |
| VMess KCP | 支持 | 支持 | 无 | Mihomo 或 Xray |
| Reality spiderX / mldsa65Verify | D27 排除 | 支持 | 无 | Xray |
| Reality FinalMask | D27 排除 | 支持 | 无 | Xray |
| flow=xtls-rprx-vision-udp443 | 排除 | 支持 | 无 | Xray |
| AnyTLS / Snell / ShadowTLS / Naive / Tor | 未覆盖 | 未发现 | 支持 | sing-box（仅必要范围） |
| 四种池策略 | 原生支持 | 无 | 无 | Mihomo |
| Clash HTTP API | 支持 | 无 | 部分 | Mihomo |
| 无感热更新 | 支持 | 无 SIGHUP | 无 | Mihomo |

## 5. 分阶段实施

### Phase 0：离线 POC（1–2 天）

- 用本地 Xray 生成 3 个 sidecar 节点配置（VLESS Encryption、VMess XHTTP、Reality 扩展）。
- 每个节点一个本地 SOCKS inbound，跑 `xray run -test -c`。
- 用 curl 通过本地 SOCKS 验证至少一个本地回环/模拟目标。
- 不接 UniProxy API，SQLite 保持不变，不启动用户服务。

### Phase 1：Xray sidecar 最小集成（1–2 周）

- 新增 `Node.Engine` 和 `SidecarEndpoint`。
- 新增 `internal/sidecar` 包：配置生成、`xray -test`、启动/停止、端口分配、进程守卫。
- 编译 sidecar 节点为 Mihomo `socks5` 出站。
- 支持 5–10 个节点、单独出站/链首；链中间/链尾直接校验报错。
- 功能开关默认关闭；不影响现有 Mihomo 节点。
- 验收：原生 R01 回归通过；sidecar 节点崩溃只影响自身；无直连回退。

### Phase 2：Xray sidecar 产品化（1–2 人月）

- 安装器、设置页、节点引擎徽章、sidecar 状态、错误提示。
- 端口范围设置/预检；200 节点上限；配置/日志脱敏。
- Windows amd64 + Linux amd64 实跑；arm64 构建/摘要。
- 完整 Go、TypeScript、UI、浏览器、真实 sidecar 集成测试。

### Phase 3：sing-box sidecar（仅按需求启动）

- 只有当实际节点需要 AnyTLS/Snell/ShadowTLS/Naive/Tor 等 R01 之外协议时启动。
- 复用同一 `SidecarManager` 接口，独立配置生成器和进程。
- 不与 Xray sidecar 共享端口；各自独立状态/版本/测试。

### Phase 4（可选）：远程 sidecar / 整链封装

- 远程 sidecar：`SidecarEndpoint.Host` 是可达 sidecar 主机；需要端口鉴权、访问控制和额外主机运维。
- 整链封装：sidecar 内部完成多跳，Mihomo 只看到一个出口；链 UI/健康语义需改。
- 这两项都不是第一阶段目标。

## 5.1 当前实现进度（2026-11-04）

- importer 已能识别 Xray-only 字段并生成 Xray outbound 模板：新增 `internal/importer/xray_uri.go`（fm/pqv/cs/spx、VLESS encryption、flow=xtls-rprx-vision-udp443、XHTTP、Reality 扩展、TLS cipher suites）与 `internal/importer/xray_vmess.go`（VMess JSON XHTTP）；`ParsedNode.Engine` 随节点导入复制到 `Node.Engine`，Mihomo 节点仍默认空引擎。
- 预览策略：合法 Xray-only 字段保留；坏 `fm`/`cs` 只在预览删除并降级；VLESS 明文公网 IP（无 TLS、encryption=none）回退 Mihomo 预览，避免生成会被 Xray 拒绝的配置。
- 真实订阅验收：`leida-vless.txt`（单行 base64，解码 1394 条 VLESS）经 `ParsePreview` 得到 1383 个可显示节点（strict 914：Mihomo 888 + Xray 26；degraded 469：Mihomo 418 + Xray 51；错误 11）。其中 77 个 Xray 节点逐节点 `xray -test` 全部通过，合并配置也返回 `Configuration OK`。
- 真实远端连通性（临时探针，不入库）：启动 77 个真实 Xray 节点，经原始 SOCKS5 CONNECT + HTTP GET 验证，当前 38/77 可达；失败多为 Cloudflare ws/tls `unexpected EOF`。该结果是单次快照，不代表节点长期可用性。
- 验证：`go test ./... -count=1 -p 1`、`go vet ./...` 通过；临时探针已删除，未提交、未发布、未部署。

## 6. 代码改动清单（预估）

| 模块 | 工作内容 | 预估 |
|---|---|---:|
| `internal/model/types.go` | `Engine`、`SidecarEndpoint`、状态字段；旧 state 默认值 | 100–200 行 |
| `internal/importer` | 识别 Xray-only 字段并标记引擎；保留真实节点配置 | 300–800 行 |
| `internal/core/compiler.go` | sidecar 节点生成 `socks5` outbound；链位置校验 | 300–600 行 |
| 新 `internal/sidecar` | manager、xray/sing-box config、port、process、health | 1,000–2,000 行 |
| `internal/coreinstall` / 新 sidecarinstall | Xray 多文件、sing-box 归档、摘要、安全解压 | 500–1,000 行 |
| `internal/app` | settings、状态 API、导入/校验、错误映射 | 300–600 行 |
| `src/shared/types.ts` | `Engine`、`SidecarStatus`、设置字段 | 50–100 行 |
| `src/web` | 引擎徽章、sidecar 状态、安装页、设置页 | 200–400 行 |
| 测试 | 单元/集成/浏览器/真实 sidecar | 2,000–4,000 行 |

总计粗估：**3–6 人月**（含 Xray + sing-box 两个 sidecar；只做 Xray sidecar 约 1–2 人月）。不包含真实公网节点采购、远程 sidecar 主机运维。

## 7. 测试矩阵

### 单元测试

- 旧 state 反序列化后 `Engine` 默认 `mihomo`。
- 节点引擎选择/导入/预览/编辑/备份往返。
- sidecar 配置生成：每个节点一个 inbound；认证随机；端口唯一。
- 链位置校验：链首允许，链中间/链尾报错。
- 端口范围/冲突/上限；sidecar 节点禁用/删除。
- 秘密脱敏：日志、错误、预览、API 不含密码。

### 集成测试

- 本地 Xray sidecar 启动 → 本地 SOCKS 代理 → 本地回环目标成功。
- sidecar 进程崩溃 → 相关节点不可用，Mihomo 原生节点不受影响。
- sidecar 端口占用 → 应用失败并回滚。
- 混合池：Mihomo 原生节点 + sidecar 节点，URL 测试/轮询正常。
- 链首 sidecar → 后续 Mihomo 节点，链路成功；链中间/链尾 sidecar 保存报错。
- 重载/重启 sidecar 不破坏 SQLite、不触碰用户数据。

### 浏览器/UI

- 导入预览显示引擎徽章；sidecar 状态页真实显示运行/未安装/错误。
- 节点编辑不允许把 sidecar 放到链中间/链尾。
- 安装流程、失败重试、版本显示。
- 390px 布局无溢出。

### 最终验证命令

```powershell
gofmt -l internal cmd tests
go vet ./...
go test ./... -count=1
pnpm typecheck
pnpm test:ui
pnpm build:web
pnpm test:e2e
```

真实 sidecar 测试使用临时数据目录和临时端口；不复用用户数据库、主密钥、订阅或代理凭据。

## 8. 风险与待决项

1. **D01 冲突**：D01 明确“当前只支持一套内核，不做双内核适配平台”。采用 sidecar 前需要新增决策记录（建议 D34）并说明“主核仍是 Mihomo，sidecar 仅本地插件”。
2. **链位置限制**：本地 sidecar 不得放链中间/链尾；如果产品要求任意位置，必须走远程 sidecar 或整链封装，成本显著增加。
3. **“全协议完美”不可验证**：以能力矩阵 + 真实节点样本为准，不承诺未来任意字段。
4. **Xray/sing-box 版本矩阵**：固定版本、固定摘要、每次升级重跑回归；不接受自动更新。
5. **端口/进程规模**：200 节点上限；端口 30000–30999；超过上限需限制或分片。
6. **秘密管理**：sidecar 配置含真实凭据；必须内存/临时文件最小化，日志/预览/API 脱敏。
7. **安装包体积/多文件**：Xray 不是单文件；Windows/Linux 资产名和归档格式不同，安装器要重写。
8. **性能**：本地 SOCKS 多一跳，不预设吞吐提升；需同节点 A/B。
9. **sing-box 必要性**：当前 R01 下多数冗余；如果仅为“全协议”而引入，先列出具体协议和真实节点，否则暂缓。
10. **许可证**：Xray/sing-box 均为 GPL-3.0 系；分发/修改义务需项目所有者确认，不应只靠独立进程规避。

## 9. 交接信息

### 9.1 分支与目录

```text
主工作区：C:\CustomFiles\CodeProjects\Github\uniproxy
开发工作区：C:\CustomFiles\CodeProjects\Github\uniproxy-sidecar
分支：feat/multi-engine-sidecars
基线：main = eb7675c
v0.2.9 tag：6ff15a8
```

进入开发目录：

```powershell
cd C:\CustomFiles\CodeProjects\Github\uniproxy-sidecar
git status --short --branch
```

### 9.2 第一轮实现顺序

1. 读本文件、`docs/decisions.md` D01/D27、`docs/requirements.md` R01/R05、`internal/core/README.md`。
2. 在 `internal/model/types.go` 增加 `Node.Engine`、`SidecarEndpoint`，补旧 state 兼容测试。
3. 新建 `internal/sidecar`：先只做 Xray 配置生成 + `xray -test` + 单进程启动/停止；不接 UI。
4. 在 `internal/core/compiler.go` 增加 sidecar 节点 → Mihomo `socks5` 出站；先用手工状态测试。
5. 加入链位置校验：sidecar 只允许单独出站或链首。
6. 用本地回环目标做一次真实 sidecar 代理测试；再补 app API/UI。
7. 功能开关默认关闭；未通过 Phase 1 gate 前不合并 main、不发布。

### 9.3 第一轮完成标准

- 现有 Go/TS/UI/E2E 全量通过。
- 旧数据（Engine 为空）行为不变。
- 至少 3 个 Xray-only 配置（VLESS Encryption、VMess XHTTP、Reality 扩展）通过 `xray -test` 并经本地 SOCKS 实际转发。
- 本地 sidecar 作为链首可用；链中间/链尾保存时报错。
- sidecar 进程崩溃只影响相关节点；Mihomo 原生节点、监听、池、链不受影响。
- 无用户数据库/密钥/订阅/真实凭据进入仓库或日志。

### 9.4 不要做的事

- 不要直接在 `main` 上改共享模型或 `Node.Config` 语义。
- 不要启动/停止/替换用户正在运行的 UniProxy/Mihomo 实例。
- 不要把真实订阅 URL、节点凭据、UUID、密码写入仓库、测试 fixture 或日志。
- 不要把本地 sidecar 节点写成真实远端地址；Mihomo 侧必须使用 `127.0.0.1:<port>`。
- 不要在没有远程 sidecar 设计时允许 sidecar 节点出现在链中间/链尾。

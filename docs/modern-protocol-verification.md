# 现代 VLESS/VMess 协议兼容验证

更新：2026-11-02 15:20（Asia/Shanghai）。

状态：**本地实现与验证完成；计划随 v0.2.6 发布（不含部署）。** 本轮未启动、停止或替换任何运行中的 UniProxy 实例，未读取用户数据库、主密钥或原始订阅凭据。

## 1. 问题与根因

社区反馈“UniProxy 无法识别最新版 VLESS/VMess”。检查确认旧版 URI 解析器的 transport 白名单只有 `tcp/ws/grpc/http/h2`，VLESS `encryption` 只接受 `""/none`；因此 v2rayN/Xray 新导出的 `type=xhttp`、`type=httpupgrade`、VMess `net=kcp`、VLESS Encryption、ECH、`vcn`/`pcs` 等会被“unsupported query option”或 transport/TLS 校验拒绝。问题不是订阅格式层，而是 URI/native 规范化层缺少现代字段。

## 2. 对照来源

- v2rayN master：`ServiceLib/Handler/Fmt/BaseFmt.cs`、`VmessFmt.cs`、`V2rayOutboundService.cs`，以及 `TransportExtraItem.cs`。确认 URI 参数与 JSON 字段：`type=raw/xhttp/kcp/grpc/ws/httpupgrade`、`extra`（Xray SplitHTTPConfig JSON）、`spx/pqv/fm/ech/vcn/pcs`、VMess `net=raw/kcp/httpupgrade/xhttp` 与 `insecure`。
- 固定内核 Mihomo 1.19.32（tag `88dcbf7f1614a67c3b36b848ee3592dfa92ada36`）出站源码：`adapter/outbound/vless.go`、`vmess.go`、`ech.go`、`reality.go`、`transport/xhttp`、`transport/vless/encryption`。
- Xray 文档：VLESS Encryption 四段式格式与 `flow=xtls-rprx-vision-udp443` 语义；Xray `SplitHTTPConfig` 的 `extra` 字段及外层 `path/host/mode` 覆盖规则。

关键事实：`mihomo -t` **不校验 transport 名称**，未知 `network` 会走默认 TCP；不能仅凭 `-t` 成功判断映射正确。本实现按固定内核源码分支映射，并用单元测试断言生成的 native map。

## 3. 已实现映射

| 输入 | 规范化输出 | 说明 |
|---|---|---|
| VLESS `type=xhttp` | `network: xhttp` + `xhttp-opts` | 支持 `path/host/mode/headers`、`x-padding-*`、session/seq/uplink 选项、`reuse-settings`、`download-settings`；Xray `extra` camelCase 转 kebab-case。外层 `path/host/mode` 显式优先，缺省时才采用 extra 内层值。 |
| VLESS/VMess `type=httpupgrade` | `network: ws` + `ws-opts.v2ray-http-upgrade: true` | Mihomo 的 HTTPUpgrade 是 WS transport 标志，不是独立 network 名称；path 默认 `/`，Host 写入 `ws-opts.headers.Host`。 |
| VMess `net=kcp/mkcp` | `network: kcp` + `kcp-opts` | 支持 `header/seed/mtu/tti/uplink-capacity/downlink-capacity/congestion/write-buffer/read-buffer`；KCP 的 URI `path` 按 v2rayN 语义作为 seed，`host` 非空拒绝。 |
| VLESS `encryption=mlkem768x25519plus...` | 原值保留在 `encryption` | 校验四段前缀、native/xorpub/random、1rtt/0rtt、至少一个 32/1088 字节 RawURL key 与 padding 规则；空/ `none` 省略。 |
| URI `ech` / native `ech-opts` | `ech-opts{enable,config,query-server-name}` | `config` 必须是标准 Base64；Reality 与 ECH 同时出现拒绝；TLS 关闭时拒绝。 |
| URI `vcn` / `pcs` | `name-cert-verify` / `fingerprint` | VMess JSON 同名字段也支持；证书 pin 必须符合现有格式校验。 |
| VMess JSON `net=raw` / `insecure` | `network: tcp` / `skip-cert-verify` | `insecure` 支持 v2rayN 的 `"1"/"0"` 与布尔；与 `allowInsecure` 重复时拒绝。 |
| URI 别名 | 规范化 | `raw/tcp`、`mkcp/kcp`、`websocket/ws`、`httpupgrade`，`type`/`network` 值大小写归一。 |

## 4. 明确拒绝的边界

以下字段在固定 Mihomo 1.19.32 中没有等价语义，若接受会造成静默降级或错误认证，因此导入明确报错：

- **VMess XHTTP**：`VmessOption` 没有 `XHTTPOpts`，VMess `StreamConnContext` 没有 xhttp 分支；不能写 `network: xhttp` 让它走 TCP。
- **Reality `spx`**：Mihomo `RealityOptions` 无 spider-X；仅空值或 `/` 可接受。
- **Reality `pqv` / Xray `fm`**：无 ML-DSA-65 验证或 finalmask 字段。
- **`flow=xtls-rprx-vision-udp443`**：Mihomo 把 flow 截为前 16 个字符，只剩 Vision，丢失 UDP443 语义。
- **XHTTP `noSSEHeader` / `scMaxBufferedPosts` / `scStreamUpServerSecs` / `serverMaxHeaderBytes` / `h2Params`**：固定内核 XHTTP 适配器无对应字段。
- **VMess XHTTP / 其他未列出的 transport**：不会静默改成 TCP；错误信息说明不支持。

同时保留既有安全限制：TLS 关闭时不接受活动证书/SNI/ALPN/Reality/ECH 参数；Reality 与 ECH 不混合；HTTPUpgrade 仍要求明确 Host/SNI 身份；XHTTP download security 只允许 `tls/reality/none`，不继承 uplink 的证书绕过或 Reality 配置。

## 5. 验证记录（本机实测）

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：通过。
- `go test -mod=readonly -p 1 ./... -count=1`，显式 `UNIPROXY_TEST_CORE_PATH=C:\CustomFiles\CodeProjects\Github\uniproxy\bin\mihomo.exe`：cmd 0.120s、app 9.121s、core 8.805s、coreinstall 0.482s、importer 0.751s、integration 4.069s，其余包均通过。
- `go test ./internal/importer -count=1`：通过 0.751s；新增 `modern_test.go` 覆盖 XHTTP extra/outer-wins/inner-fallback/defaults/stream-one 拒绝、HTTPUpgrade、VLESS Encryption、ECH/vcn/pcs、VMess KCP、VMess XHTTP 拒绝、native XHTTP/KCP/ECH、KCP 无 KCP transport 拒绝。
- `FuzzParseNeverPanics`，15 秒/16 workers：PASS，246481 次执行，0 崩溃。
- 合成配置 `.local/probe-modern/modern.yaml` 用 `bin/mihomo.exe -t -f`：输出 `configuration file ... test is successful`，exit 0。该结果只证明配置可被核心解析，不证明真实远端连通或 transport 实际工作；transport 正确性由 native map 单测和源码映射保证。
- 未执行：真实远端节点连通/吞吐、发布二进制构建、前端浏览器测试（本轮无前端改动）、Linux/macOS 实机、race/CGO、Docker 与容量测试。

## 6. 修改文件

- 新增 `internal/importer/modern.go`、`internal/importer/modern_test.go`。
- 修改 `internal/importer/uri.go`、`transport.go`、`native.go`、`vmess.go`、`security_test.go`、`uri_test.go`、`internal/importer/README.md`。
- 文档：`docs/requirements.md` R29、`docs/development-plan.md` M24、`CHANGELOG.md` v0.2.6 段、`docs/implementation-contract.md` 导入接口、`docs/README.md` 索引、本文件。

## 7. 剩余状态

- 计划随 v0.2.6 发布（不含部署）；发布前基线 HEAD 为 `fac4a90`（v0.2.5 发布后文档提交）。
- 未修改 API、数据库 schema、Mihomo 版本或运行配置；未触碰用户服务。
- 发布与部署仍按项目既有门禁分离；本版发布不自动替换运行实例。

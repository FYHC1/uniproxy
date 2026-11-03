# 协议适配三档实现与验证

更新：2026-11-03 14:12（Asia/Shanghai）。

状态：**本地实现与验证完成；未提交、未发布、未部署。** 本轮未触碰运行中的 UniProxy 实例、用户数据库、主密钥或原始订阅文件；真实样本只在本机临时目录用于统计，验证后删除。

## 1. 三档定义

| 档位 | 入口 | 行为 | 能否用于应用 |
|---|---|---|---|
| 严格 native | `Parse` | 只接受固定 Mihomo 1.19.32 有可证明等价语义的字段；任何未知/无等价字段报错 | 可以，且必须通过 `TotalErrors == 0` |
| 宽松预览 | `ParsePreview` | 保留严格节点；对可识别但会丢失语义的 URI 字段做预览级归一，写入 `Degraded` 并附原因 | 不能；`Degraded` 不进入 `addParsed`、不写入状态、不生成配置 |
| 真实可用性 | 独立检测 | 解析成功不等于远端可用；需要真实代理/URL 测试、握手与稳定性验证 | 仅作为诊断结果，不冒充解析结果 |

`ParsePreview` 只放宽 URI 列表；Clash YAML/JSON、sing-box JSON 和静态客户端文本仍走严格解析，避免结构化配置被静默改写。

## 2. 实现结构

- `internal/model/types.go`：`ParsedNode` 增加 `Degraded` / `DegradedReasons`；`ParseResult` 增加 `Degraded`；`ImportResult` 增加 `Degraded` 计数。
- `internal/importer/parser.go`：`Parse` 保持严格；新增 `ParsePreview`；`parseContent` 增加 preview 参数；`parseURIList` 将严格节点放入 `Nodes`，预览修复成功项放入 `Degraded`。
- `internal/importer/preview.go`：新增 URI 级预览归一，包括 security=false、packetEncoding=none、type=none、空/无效 fp、重复 skip-cert 声明、双重编码 ALPN、WS path 缺前导 `/`、security=none 下的 TLS 占位、Reality 自定义 ALPN、Xray-only Reality/finalmask 字段、无效 XHTTP extra、gRPC authority/mode、空查询参数和未知导出器参数。最终仍调用严格 `parseURI` 复核。
- `internal/app/imports.go`：订阅、text、file 都使用 `ParsePreview`；预览响应带 `degraded` 数组；预览不持久化 `Config`；订阅提交仍保持整源 all-or-nothing；含降级节点的订阅预览允许展示严格节点和降级节点，但有错误的订阅不能确认导入。
- `src/shared/types.ts` / `src/web/ImportDialog.tsx`：预览区分“可导入”和“仅预览”；增加降级原因表（前 50）和结果中的“仅预览”计数；订阅预览有错误时禁用确认。

## 3. 严格解析修复

这些修复不改变“只接受固定内核可证明字段”的原则：

- `security=false` 归一为 `none`。
- `packetEncoding=none` 忽略。
- `type=none` 归一为 `tcp`。
- `extra=null` / 空 `extra` 按无 extra 处理。
- 空 `fp` 跳过；`fp=ios` 大小写归一为 `iOS`。
- WS `headerType=none` 接受；TCP 的 `path`/`host` 占位在 preview 层忽略（严格层仍遵循原规则）。
- VLESS Encryption 的 ML-KEM-768 1184 字节 key 接受；32/1088 字节原有路径不变。

## 4. 真实订阅统计

使用用户提供的 VLESS 订阅进行离线解析，原始内容 366,604 字符（base64，1064 条 `vless://`）。样本只用于解析，节点实际可用性未验证。

| 指标 | 结果 |
|---|---:|
| 总条目 | 1,064 |
| 严格解析成功 | 697（65.5%） |
| 严格 TotalErrors | 367（错误详情按 128 条截断） |
| 预览可展示 | 697 严格 + 365 降级 = 1,062（99.8%） |
| 预览未恢复 | 2（1 条 UUID 凭据非法；1 条未知导出器参数组合） |

主要降级原因（同一节点可有多条）：security=none 下的 TLS 占位 122、TCP headerType/host/path 占位 86/82/45、skip-cert 重复声明 62、fp=unsafe 53、security=false 34、Xray finalmask 20+12、Reality spider-X 12、WS path 12、packetEncoding=none 10、Reality 关闭证书校验 9、空查询参数 8、其他少量。

严格错误从原始 413 降至 367；不能把 697 当成“全部解析”，也不能把 1,062 的预览结果当成可导入节点集合。预览中保留的 2 条错误被明确显示，不强行吞掉。

## 5. 验证

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：通过。
- `go test ./... -count=1`：通过；其中 importer 4.046s、app 13.922s。
- `go test ./internal/importer ./internal/app -count=1`：通过；新增 `preview_test.go` 覆盖严格/降级分离、security=none+WS path、VLESS Encryption 1184、Xray-only Reality 字段、Reality 自定义 ALPN、干净节点 fp=ios；`preview_import_test.go` 覆盖 text 预览不持久化和订阅降级预览。
- `pnpm typecheck`：通过。
- `pnpm test:ui`：42/42 通过；`pnpm build:web`：Vite 7.3.7，4655 modules，CSS 110.64kB/gzip 22.97、JS 557.83kB/gzip 162.32，仅有 chunk 体积提示。
- `pnpm exec playwright test tests/browser/subscription-formats.spec.ts`：3/3 通过（含“1 个可导入”预览断言）；随后 `pnpm test:e2e` 全量 52/52 通过（1.2m）。
- 真实订阅统计使用本机临时测试读取 `.local/vless-live.txt`；该文件、`live_*_tmp_test.go` 与 TestSimplePreviewTmp 均已删除，不进入仓库、文档或测试夹具。

## 6. 边界

- 预览不是“导入成功”；`Degraded` 节点永不进入 `addParsed`、SQLite、编译器和运行配置。
- 订阅提交仍要求 `TotalErrors == 0`；预览展示部分节点不等于允许提交部分订阅。
- 真实节点连通性、吞吐、握手成功率未验证；“解析成功”只证明字段被正确识别。
- 固定内核 1.19.32 的明确无等价字段仍不伪造支持；需要这些字段时只能使用兼容客户端或另行评估内核版本。
- 未提交、未发布、未部署；用户运行实例保持原状。

## 7. 关键文件

- `internal/importer/parser.go`
- `internal/importer/preview.go`
- `internal/importer/uri.go`
- `internal/importer/native.go`
- `internal/importer/modern.go`
- `internal/model/types.go`
- `internal/app/imports.go`
- `src/shared/types.ts`
- `src/web/ImportDialog.tsx`
- `internal/importer/preview_test.go`
- `internal/app/preview_import_test.go`

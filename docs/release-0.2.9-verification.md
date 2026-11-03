# UniProxy v0.2.9 发布验收

更新日期：2026-11-03
发布阶段状态：**已发布（非 draft、非 prerelease、Latest）；CI、标签、四平台资产与正式 Windows 程序验收均完成；只发布，未部署。**

## 1. 范围

本版为 v0.2.8 之后的协议解析兼容版本，汇总 M30：

- 严格 `Parse` 保持固定 Mihomo 1.19.32 可证明等价字段映射。
- 新增 `ParsePreview`：仅 URI 列表的宽松预览，降级节点写入 `Degraded` 并附原因，不进入可应用节点。
- 导入预览区分“可导入/仅预览”，订阅提交仍 all-or-nothing。
- 版本与文档：`internal/version/version.go`=0.2.9、`internal/version/version_test.go`=0.2.9、`package.json`=0.2.9；README/CHANGELOG/requirements/decisions/development-plan/session-handoff 同步；新增 `docs/releases/v0.2.9.md` 与本记录。
- 不包含：部署、停止/替换运行实例、数据库 schema 迁移、Mihomo 内核升级、真实远端节点连通性或吞吐测试。

## 2. 发布前本地验证

### 2.1 静态检查

- `gofmt -l internal cmd tests`：无输出。
- `go vet ./...`：通过。
- `git diff --check`：通过（提交前复核）。

### 2.2 Go 全量测试

命令：`UNIPROXY_TEST_CORE_PATH=bin/mihomo.exe go test -mod=readonly -p 1 ./... -count=1`。

结果：全部通过（core 12.533s、app 11.499s、integration 6.813s）。并行运行 `go test ./...` 曾因本机资源竞争超时，串行复跑通过；不把超时当作测试失败，也不把并行结果写成通过。

### 2.3 前端与浏览器

- `pnpm typecheck`：通过。
- `pnpm test:ui`：42/42 通过。
- `pnpm build:web`：通过（Vite 7.3.7，4655 modules）。
- `pnpm test:e2e`：52/52 通过（1.2m）。
- `tests/browser/subscription-formats.spec.ts`：3/3 通过。

### 2.4 真实订阅解析

- 样本：https://leidahub.top/by-protocol/vless.txt，1064 条（仅本地临时解析，已删除样本与本机测试）。
- 严格 `Parse`：697 条成功，`TotalErrors=367`。
- `ParsePreview`：697 条严格 + 365 条降级 = 1062 条可展示，2 条未恢复（1 条非法 UUID、1 条未知导出器参数组合）。
- 该结果只证明解析覆盖，不证明节点实际可用。

## 3. CI 与正式资产

### 3.1 源码与 CI

- 源码提交：`6ff15a8c2a0fb44e3dbc14c4e024b1f7c6b4d54a`（29 files，841 insertions / 85 deletions）。
- main CI：`41602628999` 成功（Ubuntu 42s、Windows 1m35s）。
- annotated tag `v0.2.9`：对象 `4d05181f557445f5f6325bd57e3821d138c981a5`，解引用 `6ff15a8c2a0fb44e3dbc14c4e024b1f7c6b4d54a`。
- 标签 CI / Release workflow：`41602671353` / `41602671127`，均成功。

### 3.2 正式发布

- 发布时间：`2026-11-03T08:22:39Z`（2026-11-03 16:22:39 +08），非 draft、非 prerelease、Latest。
- Release：[v0.2.9](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.9>)。
- 正式资产与 SHA256（下载副本 `bin/releases/v0.2.9`，与 `SHA256SUMS.txt` 逐项一致）：

| 资产 | 字节 | SHA256 |
|---|---:|---|
| uniproxy-windows-amd64.exe | 13,387,776 | `e778e15a4c833fa11da51b95e91700b033b967deac2876e57b09af7c40832982` |
| uniproxy-windows-arm64.exe | 12,464,128 | `b906721d3a217a883524534d36b5b0ed1af536624cd561faa1d8c327eba09e78` |
| uniproxy-linux-amd64 | 13,029,560 | `801c275b1cb30f0f7199963a6ea2e1364ad04a4d59b42c2dede39e6c01f0162b` |
| uniproxy-linux-arm64 | 12,386,488 | `f57b9ab949e2ad51b1a21aba0bdaab27b502694fcc38f9c40f85ff6a4019eec7` |
| SHA256SUMS.txt | 360 | `e6400560719980178e37c93cf851f8f7c62275d6e5bfbb3ea8543313fe3c7505` |

### 3.3 正式 Windows 程序验收

- `-version`：`UniProxy 0.2.9 · commit 6ff15a8c2a0f · built 2026-11-03T08:21:55Z`。
- `TestBuilt`：全部 PASS（3.275s）。
- 真实 Windows 控制台：两轮 WM_CLOSE exit 0；standalone error exit 1 且 report saved。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。

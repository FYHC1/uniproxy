# UniProxy v0.2.9 发布验收

更新日期：2026-11-03
发布阶段状态：**本地验证完成；CI、标签、四平台资产与正式 Windows 程序验收待回填。**

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

命令：

```powershell
go test ./... -count=1
```

结果：全部通过（importer 4.046s、app 13.922s）。

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

- 源码提交：待回填。
- main CI：待回填。
- annotated tag `v0.2.9`：待回填。
- 标签 CI / Release workflow：待回填。

### 3.2 正式发布

- 发布时间：待回填。
- Release：待回填。
- 正式资产与 SHA256：待回填。

### 3.3 正式 Windows 程序验收

- `-version`：待回填。
- `TestBuilt`：待回填。
- 真实 Windows 控制台：待回填。

## 4. 部署边界

本版**不部署**。发布只创建 GitHub Release 与附件，不停止、不替换、不启动任何运行实例。用户如要升级，需先正常退出旧实例并确认内核退出，再替换主程序、沿用原数据目录与匹配主密钥；禁止在运行中复制或覆盖 SQLite 数据库。

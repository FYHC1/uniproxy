# 数据目录与发布流程验收（v0.2.0）

**日期：2026-10-20（Asia/Shanghai）。范围：数据目录改为平台标准位置、版本注入、GitHub 标签发布流程。未部署到用户当前运行实例，未改动其数据。**

## 1. 用户数据备份

用户已在 `%USERPROFILE%\Downloads\uniproxy-backup-2026-10-20.json` 导出完整备份（349,830 字节，schemaVersion 1，344 节点 / 2 分组 / 1 池 / 4 监听 / revision 24）。本轮只读核对了文件存在性与结构，未打印任何凭据字段。

备份为**明文敏感文件**，包含节点密码、订阅地址与代理账户，不应提交仓库或上传第三方。

## 2. 数据目录规范化

### 2.1 问题

原默认值为 `./data`，相对**进程工作目录**。同一二进制从别的目录启动会新建空库，表现如同“数据丢失”；服务/容器/计划任务场景尤其容易踩到。

### 2.2 新规则

| 平台 | 默认目录 | 不可写时回退 |
|---|---|---|
| Windows | `%ProgramData%\UniProxy` | `%LOCALAPPDATA%\UniProxy` |
| Linux | `/var/lib/uniproxy` | `$XDG_DATA_HOME/uniproxy` 或 `~/.local/share/uniproxy` |
| macOS | `~/Library/Application Support/UniProxy` | 同左 |

解析优先级：

1. `-data` 或 `UNIPROXY_DATA_DIR`（显式指定，目录不存在会创建，创建失败直接报错）；
2. 已存在的平台标准目录；
3. 已存在的旧版目录（工作目录下 `data`，其次可执行文件同目录 `data`）——**沿用并提示迁移**，不在新位置建空库；
4. 新建平台标准目录；
5. 标准目录不可写时回退用户目录，并在日志说明。

启动日志新增一行 `数据目录: <path> (<source>)`，source 取 `configured`/`standard`/`legacy`/`fallback`，迁移建议另起一行。

零字节的 `uniproxy.db` 不视为已有数据，避免把一次失败的创建当作真实库并阻止默认路径生效。

### 2.3 实测

用开发构建在本机（Windows，非管理员）验证：

| 场景 | 结果 |
|---|---|
| `-data <临时目录>` | 日志 `(configured)`，health 200，数据写入该目录 |
| 无 `-data`，工作目录无旧数据 | 日志 `C:\ProgramData\UniProxy (standard)`，目录被创建（验证后已删除该探测目录） |
| 无 `-data`，工作目录存在旧 `data` | 日志 `(<工作目录>\data) (legacy)`，并提示迁移到 ProgramData；ProgramData 未被创建 |
| 非管理员写 ProgramData | 本机可写，故回退分支由单元测试覆盖 |

## 3. 版本注入

[internal/version](<../internal/version/version.go>) 是唯一来源：

- `Version`（默认 `0.2.0`）、`Commit`（默认 `dev`）、`Date`（默认 `unknown`）；
- 发布构建通过 `-ldflags -X` 注入三者；
- `-version`、启动日志、`/api/health` 报告同一字符串；
- 未注入的本地构建只显示版本号，不伪造提交或构建时间。

本地用真实提交哈希与时间验证过注入效果：

~~~text
UniProxy 0.2.0 · commit b0a04aa7cbc2 · built 2026-10-20T01:01:36Z
~~~

`/api/health` 同时返回 `{"ok":true,"version":"0.2.0"}`，可用它确认运行实例对应哪个发布。

## 4. 发布工作流

[.github/workflows/release.yml](<../.github/workflows/release.yml>)：

- 触发：推送 `v*.*.*` 标签，或在 Actions 手动触发并填写版本号；
- 矩阵：linux/windows × amd64/arm64，共 4 个二进制，另含 `core-install`；
- 构建前先 `pnpm install --frozen-lockfile` + `pnpm run build:web`，前端嵌入 Go；
- 生成 `SHA256SUMS.txt`；
- 用 `gh-release` 创建 Release 并附加全部产物，自动生成 release notes。

工作流已纳入 [manifest 解析测试](<../tests/integration/manifests_test.go>)，避免 YAML 语法错误只在推送标签时才暴露。

## 5. 验证结果

| 检查 | 结果 |
|---|---|
| `go test -count=1 ./...` | 通过 |
| `go vet ./...` / `gofmt -l` | 通过 / 无差异 |
| `internal/appdata` 单元测试 | 通过（跨平台路径、旧目录沿用、回退、零字节库、显式目录失败） |
| `internal/version` 单元测试 | 通过（开发版不伪造元数据、发布版含提交与时间） |
| `/api/health` 版本测试 | 通过（默认 dev，可注入） |
| 严格 TS + 前端单测 | 通过 / 32 通过 |
| 真实 API 浏览器 | 32 通过 |
| fixture UI 回归 | 通过，零未捕获错误 |
| 真实本机 Mihomo 链路 | 通过（本轮未改动核心路径，回归执行） |

## 5.1 构建产物（暂存，未部署）

| 产物 | 字节 | 时间 | SHA256 |
|---|---|---|---|
| `bin/updates/uniproxy.exe` | 13579776 | 2026-10-20 09:12:31 | `0E014D1B9DCF712B4E6A4FC0EB162CAD5514AD5A55318E26E1894CFC399FB9E6` |
| `bin/updates/linux-amd64/uniproxy` | 13250720 | 09:12:33 | `7B9D90337803EC2775E1656A6553A4802E7846F10CB6038C8CCDE3992BCE2740` |
| `bin/updates/linux-arm64/uniproxy` | 12452000 | 09:12:35 | `E5C1B982CB9116B2B53167DA664677702EAD0E97B8E99C1A09F54BC0CDCEC377` |

均带 `-ldflags` 注入，`-version` 输出 `UniProxy 0.2.0 · commit c45e8f3fe13d · built 2026-10-20T01:12:29Z`。体积明显小于此前构建，因为新增 `-s -w` 去符号。

当前运行中的 `bin/uniproxy.exe` 仍是 `510CAE7F…`（19,785,216 字节），**本轮未替换**。

## 5.2 已发布 v0.2.0

标签 `v0.2.0`（提交 `a9d3fc8`）已推送，Release 工作流两个 job 全部成功，产物已发布：

| 资产 | 字节 |
|---|---|
| `uniproxy-windows-amd64.exe` | 12,748,800 |
| `uniproxy-windows-arm64.exe` | 11,871,744 |
| `uniproxy-linux-amd64` | 12,419,256 |
| `uniproxy-linux-arm64` | 11,796,664 |
| `core-install-*`（4 个平台） | 5.9–6.5 MB |
| `SHA256SUMS.txt` | 736 |

发布地址：`https://github.com/FYHC1/uniproxy/releases/tag/v0.2.0`

验证（下载真实发布资产后执行，非本地构建）：

- `SHA256SUMS.txt` 全部 8 项与本机重新计算的 SHA256 一致，0 处不符；
- `uniproxy-windows-amd64.exe -version` 输出 `UniProxy 0.2.0 · commit a9d3fc886674 · built 2026-10-20T01:19:05Z`，与标签提交一致；
- 用临时数据目录启动该发布二进制：`/api/health` 返回 `{"ok":true,"version":"0.2.0"}`，内嵌前端 HTTP 200，启动日志同时打印版本与数据目录。

## 6. 限制与后续

- 未在 Linux/Docker 实跑；`/var/lib/uniproxy` 与 XDG 回退由单元测试覆盖，非真实主机验证。
- 未创建正式 Release：需要真实推送标签，本轮只提交源码。发布前应确认许可证与内核 GPL 分发义务。
- 用户当前运行的仍是旧构建（哈希 `510CAE7F…`，数据目录 `bin\data`）。升级后默认目录会变为 `C:\ProgramData\UniProxy`，但旧目录会被自动沿用；如需真正迁移，按 README「数据目录」一节的步骤复制数据库+主密钥后删除旧目录。
- 用户备份 JSON 含明文凭据，建议迁移完成后从下载目录移入受控位置或加密保存。

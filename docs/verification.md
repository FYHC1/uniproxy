# UniProxy v0.1.0 首版历史验收记录

> 这是首次实现时的历史记录，5个浏览器/9个工具测试与覆盖率只代表当时范围。后续修复的最新证据见[中转修复验收](<relay-fix-verification.md>)（7/21），当前运行/部署状态见[接续快照](<session-handoff.md>)。2026-10-19文档整理没有重新运行本页测试。

日期：2026-10-19。主机：Windows amd64。Go1.27.2、Node24.21.0、pnpm11.28.5；真实代理内核为官方 SHA256 验证的 Mihomo v1.19.32。

## 已验证

| 检查 | 结果 |
|---|---|
| go test -count=1 -cover ./...（设置真实内核与构建产物路径） | 通过 |
| go vet ./... | 通过 |
| TypeScript + Vite生产构建 | 通过 |
| Windows管理二进制 + 嵌入Web + 空配置启动真实内核 | 通过 |
| 代表协议配置：VLESS、VMess、Trojan、SS、HY1/2、TUIC、SSH、SOCKS5、HTTP，以及Reality/WS/gRPC | 真实内核配置校验通过 |
| HTTP / SOCKS / mixed认证入口，经HTTP/SOCKS上游的真实本地转发 | 通过 |
| SOCKS UDP实际回显、空池新UDP流拒绝 | 通过 |
| 四种原生代理池策略、in-user TCP策略路由 | 通过 |
| 正常重载/端口交换后已有TCP流继续传输 | 通过 |
| 延迟API与限量下载测速，不直连/不假数据 | 通过 |
| 端口占用、校验/应用失败、回滚和恢复失败状态 | 测试通过 |
| 节点多来源、多分组、失败订阅保留、异常删减保护 | 测试通过 |
| 配置变化中途检测结果丢弃、旧订阅刷新不覆盖恢复数据 | 回归测试通过 |
| 超过12MiB的合法配置备份导出/恢复 | 回归测试通过 |
| 订阅SSRF、防重定向Referer泄露、Host/Origin/Token、凭据脱敏 | 测试通过 |
| Playwright真实隔离API：空态、导入、分组、池、认证入口及窄屏 | 5项通过 |
| 额外UI工具9项及fixture：401、会话Token、预览、批量、密码保留、响应式 | 前端独立测试通过（不视为真实内核验证） |
| Linux amd64/arm64 UniProxy与安装器交叉编译 | 通过；非Linux运行测试 |
| compose/CI/pnpm YAML语法 | 通过 |

本机完整测试覆盖率：管理服务60.1%、内核83.2%、导入解析91.8%、安全下载77.2%、存储69.9%、安装器69.6%。端到端跨进程代码不计入普通包覆盖率。

导入解析fuzz：最终30秒163781次；SSH密钥解析fuzz：30秒814857次，均通过。竞态检测器无法运行：CGO=0且未安装C编译器；不把普通并发测试当作race结果。

## 未验证/不得据此推断

- 未提供真实远端VLESS/VMess/Trojan/QUIC/SSH账号，未验证所有上游协议的公网握手和运营商兼容性。
- 未做公网吞吐或大规模长时间压测。500节点管理验证微基准不等于数据面性能承诺。
- 当前没有Docker CLI，未实际构建/启动镜像；提供bridge TCP和Linux host UDP两种部署文件。
- Linux构建成功，不等于已在Linux主机实跑。
- 原生UDP仅回环且不认证数据包；公网UDP认证网关不在第一版范围。
- 正常修改尽量保留TCP，不保证UDP映射/粘性缓存或进程重启无损。

## 重现

~~~powershell
pnpm install --frozen-lockfile
pnpm build
$env:UNIPROXY_TEST_CORE_PATH = (Resolve-Path bin/mihomo.exe).Path
$env:UNIPROXY_TEST_APP_PATH = (Resolve-Path bin/uniproxy.exe).Path
go test -count=1 -cover ./...
go vet ./...
pnpm test:e2e
~~~

若内核安装到默认目录，使用 data/core/mihomo.exe 路径。不要将测试用节点导入真实数据目录；浏览器和Go测试使用隔离临时存储。没有把示例节点/假延迟写入产品初始状态。

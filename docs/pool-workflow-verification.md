# 代理池选择、监听与卡片优化验收

记录：2026-10-19 22:23（Asia/Shanghai）。需求 R16–R18，用户提供原界面与监听卡片参考。**本轮源码、测试、Windows/Linux暂存构建完成；未部署本次代理池新版，不改变用户数据或运行实例。**

## 1. 用户可见变化

- 包含分组按任一/全部限定节点候选，未选分组时显示全部节点；UDP兼容过滤与分组条件共同生效。
- 分组动态成员默认勾选，取消勾选转换为本池节点排除，重新勾选清除此排除。未来订阅新增的组内节点仍动态纳入，不把分组悄悄变成静态快照。
- 包含分组和包含节点都有全选/取消全选以及部分选择状态。全选覆盖当前搜索的全部结果，不止首100个渲染项；搜索之外选择保留。
- 移除代理池的排除分组选择器；新池默认无排除组。旧池已有规则保留、显示说明，可明确清除后保存，取消不生效。节点管理页的排除组筛选不受影响。
- 显式改变分组、any/all或UDP范围时清理范围外手动节点并提示；仅编辑名称/策略不清理旧选择。对话框期间真正被其他操作删除的节点引用，保存时清理并解释，范围外仍存在的节点不误删。
- 新建代理池必须同时配置共享监听，可自动选空闲端口或指定端口；默认127.0.0.1、mixed、仅TCP。保留认证、回环UDP、in-user限制。
- 代理池改卡片：展示全部关联监听地址、协议、认证及实际/待应用状态；显示节点协议、最近检测延迟、禁用/中转阻断；可复制地址、同页编辑/添加监听、管理节点、单个/整池测延迟和从池移除。
- 从池移除需确认，只改当前池，不删总节点、不影响其他池/独享入口。动态组节点加入excludeNodeIds防止立即加回；最后成员移除仍保留入口并拒绝新连接，不直连。

## 2. 实现入口与兼容

- [组合API](<../internal/app/pools.go>)：POST /api/pools/with-listener，返回 Mutation<{pool,listener}>；DELETE /api/pools/{id}/members/{nodeId}，返回 Mutation<Pool>。
- [端口分配](<../internal/app/ports.go>)：抽取旧批量分配器，跳过管理/所有已配置端口，检查OS TCP和显式UDP占用，支持取消。绑定检查和外部内核最终绑定之间仍有竞态，不能承诺端口永久预留。
- [创建/编辑对话框](<../src/web/PoolDialog.tsx>)、[勾选映射](<../src/web/poolSelection.ts>)、[通用全选](<../src/web/selection.ts>)和[共享控件](<../src/web/ui.tsx>)。
- [池卡片](<../src/web/PoolCard.tsx>)、[真实状态投影](<../src/web/poolView.ts>)、[池页面](<../src/web/PoolsPage.tsx>)、[响应式样式](<../src/web/poolStyles.css>)。
- [共享TS契约](<../src/shared/types.ts>)与[API说明](<api.md>)同步。旧池POST/PUT仍兼容，持久模型/schema不变。

“一次创建”是期望配置的事务：一次验证、一个revision、一次保存和应用。校验失败不保存半个池/入口；外部内核应用失败可能已保存两者，响应dirty且解释错误。前端不发送补偿删除，不把保存成功当代理已经运行。旧删除整个池行为仍停用并保留关联入口。

卡片延迟来自全局检测URL的最近真实任务结果，不是内核实时最优节点或此池专属URL即时值。dirty/applying时不使用旧listener ID的listening标记宣称新地址已生效。任务记录随服务重启丢失时显示失效说明，解除忙碌，不能假报完成。

## 3. 本轮实际运行

宿主：Windows/amd64；Go1.27.2、Node24.21.0、pnpm11.28.5、Vite7.3.7；Mihomo固定1.19.32。以下命令均从项目根运行，工具的绝对路径仅用于调用现有运行时，没有安装或修改系统工具链。

| 验证 | 本轮结果 |
|---|---|
| 严格TypeScript：tsc --noEmit --noUnusedLocals --noUnusedParameters | 通过 |
| node --test src/web/tests/utils.test.mjs | **30/30通过**，包含9项本轮纯逻辑回归 |
| node src/web/tests/verify.mjs | 通过；内存生产构建、拦截测试域名、零未捕获浏览器错误；不启动Go或内核 |
| pnpm run build:web | 通过，生产前端已嵌入暂存Go程序 |
| 后端定向测试 + go test ./internal/app -count=1 + go vet ./internal/app | 通过；16个新增测试函数及安全/事务子场景，临时SQLite/fake core |
| go test -p 1 -count=1 ./... + go vet ./... | **全量通过**，显式指定真实core和Windows暂存APP；私有导入变量为空 |
| TestRealPoolAtomicListenerRemoveReinclude | 独立通过后纳入全量通过：实际认证代理、真实延迟、空池拒绝、同端口重新纳入 |
| pnpm test:e2e | **20/20通过（41.9秒）**；Go/API/临时SQLite/Edge，19090，故意无内核 |
| Windows暂存Go构建 | 通过；进程/CLI隔离验证包含于全量Go测试 |
| Linux amd64 / arm64暂存交叉构建 | 通过；不代表Linux/Docker实跑 |

可复查命令：

~~~powershell
node node_modules/typescript/bin/tsc --noEmit --noUnusedLocals --noUnusedParameters
node --test src/web/tests/utils.test.mjs
node src/web/tests/verify.mjs
pnpm run build:web
go build -mod=readonly -trimpath -o bin/updates/uniproxy.exe ./cmd/uniproxy
$env:UNIPROXY_TEST_CORE_PATH=(Resolve-Path bin/mihomo.exe).Path
$env:UNIPROXY_TEST_APP_PATH=(Resolve-Path bin/updates/uniproxy.exe).Path
$env:UNIPROXY_TEST_IMPORT_FILE=''
go test -p 1 -count=1 ./...
go vet ./...
pnpm test:e2e
~~~

[浏览器选择回归](<../tests/browser/pool-selection.spec.ts>)覆盖125项组内选择、110项搜索全选、任一/全部、UDP、未分组、动态新增、旧排除和打开对话框期间删除。[工作流回归](<../tests/browser/pool-workflow.spec.ts>)覆盖原子失败/重试、手动端口、池内移除/重新纳入、同页旧池加监听、1440/390布局。[临时fixture](<../tests/browser/fixtures.ts>)使用唯一名称/文档IP、关闭真实内核、只清理本测试拥有对象。

[真实内核工作流](<../tests/integration/pool_workflow_test.go>)仅本机受控目标和上游。没有使用用户节点/订阅或读取用户DB/主密钥/原始配置。旧7项E2E/21项工具数据仍是M7历史，不覆盖为本轮结果。

## 4. 审查与失败调查

独立只读审查发现并修复3个中等交互问题，均加回归：

1. 搜索后移除节点使数量从9降至8，不能隐藏搜索框而保留无法清除的条件；搜索非空时仍显示清除入口。
2. 服务重启清空任务记录，卡片不能永久回退本地running任务；新的成功bootstrap缺少任务时显示记录失效并解除忙碌。
3. 打开编辑期间全局删除曾排除节点，草稿不能携带失效ID阻止保存；只清理真正不存在的引用，保留隐藏有效节点和旧排除组。

另调查两次测试问题：

- 后端假内核保存int而快照JSON还原float64，DeepEqual错误地报告旧运行状态变化；比较两侧统一快照格式后定向/全量测试通过，非产品回滚缺陷。
- fixture在375px切换的中间帧读取了旧桌面布局位置。增加最长2.5秒media-query布局稳定断言，持续横向溢出仍失败；随后fixture及真实API390px布局均通过，未用overflow:hidden掩盖问题。

## 5. 本轮未验证 / 部署事实

- 本轮无用户真实上游公网探测、原始私有Clash复测、race、性能压测、Linux主机/Docker实跑。
- 22:21只读核对发现当前用户进程已是先前中转修复构建，启动时间21:15，运行文件SHA256为4A8FC2F8…，不再是20:31文档轮的旧首版。**本次会话未执行该升级，也没有停止/替换用户进程。**
- 22:26最终复核用户PID/哈希不变，19090测试端口已无监听；所有本轮后台任务与协作者结果已收集。文档19份/本地链接263个均有效，代码围栏配对通过。
- 本次代理池新版暂存在[Windows程序](<../bin/updates/uniproxy.exe>)、[Linux amd64](<../bin/updates/linux-amd64/uniproxy>)、[Linux arm64](<../bin/updates/linux-arm64/uniproxy>)；准确哈希与运行PID见[接续快照](<session-handoff.md>)。
- 前端嵌入Go。源码/暂存构建完成不等于线上生效；需用户批准停机后按[升级指南](<development-guide.md>)保留数据/密钥、原工作目录启动、刷新页面。不要同时启动两份程序使用同一份数据。

## 6. 测试界面截图

以下为隔离fixture，仅用于检查布局和交互，不是用户数据或真实网络测量截图：

- [桌面池卡片](<../src/web/tests/artifacts/pools-desktop.png>)
- [移动端池卡片](<../src/web/tests/artifacts/pools-mobile.png>)

# 订阅导入与管理体验修复验收

记录：2026-10-19 23:20（Asia/Shanghai）。对应R19–R21。**源码、隔离验证与暂存构建已完成；本次新版未部署，不操作用户正式数据或服务。**

## 1. 订阅导入故障：事实与修复

截图显示的是下载失败，解析器未收到内容。主机名级排查确认：

1. 系统DNS为截图中的公共订阅域名返回198.18.0.230，属于Fake-IP保留范围，原下载器正确地拒绝直接拨该地址，但错误原因被笼统提示隐藏。
2. 固定数字地址bootstrap的Cloudflare和Google HTTPS DNS均返回公网地址；以原域名SNI连接该公网IP的TLS1.3证书校验通过。
3. 使用本轮产品默认[安全客户端](<../internal/safehttp/client.go>)，在同一系统DNS环境中，对公共域名根路径HEAD测试通过，HTTP200、TLS验证正常（2.30秒）。没有跟随重定向，也没有读取正文。

**未请求截图含令牌的订阅路径，未保存用户原始订阅内容，未把URL交给搜索/第三方网页转换器。** 所以本轮证明了导致“下载失败”的Fake-IP传输问题已修复，而不是宣称用户整份私有订阅或真实上游节点已经逐一验收。

### 安全修复边界

[公共DNS复核](<../internal/safehttp/doh.go>)仅在默认模式、域名解析结果全部为198.18/15 Fake-IP时触发。字面Fake-IP、普通内网域名或混合非公网答案仍拒绝。

- 固定Cloudflare1.1.1.1与Google8.8.8.8数字bootstrap，保留服务名TLS/SNI校验。只发送原主机名的A/AAAA、类型及关闭ECS参数，不发送订阅路径/令牌/端口/请求头。
- A/AAAA全部结果与CNAME关系校验；公私混合、站点本地IPv6、保留结果仍拒绝，不挑一个公网IP放过其余非法结果；数字IP固定拨号，原域名HTTPS验证。
- 保留直连模式、不启用环境代理、不修改OS DNS/TUN/防火墙、不自动开启allowPrivate、不降低TLS。
- 有界时间：请求25秒，DNS5秒，DoH总6秒/每provider3秒，TCP总10秒在候选IP间分摊，TLS10秒。DoH64KiB/64RR/CNAME16，订阅解压后5MiB。
- 重定向每跳验证、拒绝HTTPS降级、总链至多4请求、删除Referer；取消可传递到原本会被net/http脱离的拨号。
- [分类错误](<../internal/safehttp/errors.go>)只保留允许公开的种类/HTTP数字/上下文哨兵，不带原始URL、DNS错误文本、证书信息或响应内容。已确认HTML页面与配置解析问题分开。
- [导入协调](<../internal/app/imports.go>)显示来源编号、解析错误总量和第一个无秘密原因。失败/解析不完整的来源不导入或删旧节点，force也不绕过；空200仍由原清理确认语义控制。
- [导入对话框](<../src/web/ImportDialog.tsx>)改为“获取或解析失败”，说明服务器直连和Fake-IP复核，不再把所有下载失败都称为格式解析错误。

## 2. 更紧凑的代理池

[池页面](<../src/web/PoolsPage.tsx>)用4项真实统计替代高横幅：池数、共享入口、跨池去重可转发候选、启用空池。增加名称/分组/地址搜索、策略/状态筛选和重置。

[池卡片](<../src/web/PoolCard.tsx>)及[样式](<../src/web/poolStyles.css>)：

- 单池宽屏占满工作区并左右排列配置/节点，不再固定占三分之一留白；多池自适应栅格。
- 成员6条分页，列表限高；节点为紧凑行，协议/最近延迟与操作直达，地址/中转/错误/检测时间按需展开。
- 默认展示前2个入口，明确可展开全部；支持复制/编辑/追加入口。
- 最近成功/失败/未知和成功延迟中位数均基于现有测量；未知不记0，不能冒充实时在线。
- 保存响应可能先更新runtime、bootstrap随后失败：版本不一致显示“状态待同步”，绝不把旧地址标成已监听。

150节点浏览器用例验证仅6行、宽度占工作区90%以上、卡片高小于750px；不是仅通过隐藏溢出减少高度。详细内容/分页均可访问。375/390/768/1024视口fixture以及真实API390/1440布局通过。

[紧凑单池预览](<../src/web/tests/artifacts/pools-compact-single.png>)使用隔离fixture，非用户节点/真实公网测量截图。

## 3. 点击分组管理节点

[分组页面](<../src/web/GroupsPage.tsx>)的组名和节点数量打开[分组节点管理](<../src/web/GroupNodesDialog.tsx>)；铅笔仍编辑名称/颜色。

- 20条分页、组内搜索/状态过滤、选本页/全部筛选结果、隐藏选择提示；添加已有节点支持超过100条候选全选。
- 添加和移出只发送当前groupId，不用旧完整groupIds覆盖；移出本组不删除总节点或其他分组/入口。
- 全局禁用独立确认并说明影响所有分组/池/独享和中转；启用也明确全局范围，不伪装组内停用。
- 延迟检测复用现有真实API，严格运行/版本gate、禁用/阻断节点跳过说明；未知/旧结果诚实显示。
- 单Modal中切换成员、添加、移出确认和禁用确认，无嵌套表单。最新bootstrap变化清理失效成员/添加/确认ID，删除后返回不恢复旧选择；组消失仅可关闭。
- 订阅刷新或修改可重分配来源组，给予通用警告而不猜精确归属；人工加组不会提升Node.Manual永久保留标志。
- 表格分开“成员来源引用”与“旧排除规则引用”；删除组/移出旧排除组可能扩大池候选，确认文案说明。

## 4. 本轮验证

| 范围 | 结果与界限 |
|---|---|
| 严格TypeScript | tsc --noEmit --noUnusedLocals --noUnusedParameters通过 |
| UI工具测试 | **31/31通过** |
| 内存构建fixture浏览器 | 通过，零未捕获浏览器错误；仅fixture、无产品假数据 |
| 实际Go/API/临时SQLite浏览器 | **32/32通过，最终56.4秒**；故意无内核，部分健康/来源投影明确为UI-only，不冒充真实代理证据 |
| safehttp专测 | **23测试函数**（19新增+4旧）及子场景通过，全部注入/本机，默认无远端访问；vet通过 |
| app新增回归 | **14测试函数**（订阅8+分组6）及子场景通过，6.129秒，vet通过 |
| Go全量 | 固定真实core+Windows暂存APP，go test -p1 -count1 ./...通过；go vet ./...通过 |
| 真实代理回归 | 原有HTTP/SOCKS/mixed/UDP/策略/中转/热更新/池创建与移除链路在全量测试执行，只本机受控目标 |
| 主机级诊断 | 裸域名系统DNS/DoH/TLS与新安全客户端根HEAD通过；不是私有路径订阅测试 |
| 构建 | Web + Windows暂存 + Linux amd64/arm64交叉构建通过；没有部署/容器实跑 |

主要新增测试：[下载策略](<../internal/safehttp/client_test.go>)、[DoH](<../internal/safehttp/doh_test.go>)、[下载安全](<../internal/safehttp/fetch_security_test.go>)、[站点本地拒绝](<../internal/safehttp/site_local_test.go>)、[订阅诊断](<../internal/app/subscription_diagnostics_test.go>)、[分组归属](<../internal/app/group_membership_test.go>)、[分组浏览器](<../tests/browser/group-management.spec.ts>)、[紧凑池](<../tests/browser/pool-compact.spec.ts>)、[订阅浏览器](<../tests/browser/subscription-diagnostics.spec.ts>)。

~~~powershell
node node_modules/typescript/bin/tsc --noEmit --noUnusedLocals --noUnusedParameters
node --test src/web/tests/utils.test.mjs
node src/web/tests/verify.mjs
pnpm run build:web
pnpm test:e2e
go build -mod=readonly -trimpath -o bin/updates/uniproxy.exe ./cmd/uniproxy
$env:UNIPROXY_TEST_CORE_PATH=(Resolve-Path bin/mihomo.exe).Path
$env:UNIPROXY_TEST_APP_PATH=(Resolve-Path bin/updates/uniproxy.exe).Path
$env:UNIPROXY_TEST_IMPORT_FILE=''
$env:UNIPROXY_TEST_SUBSCRIPTION_HOST=''
go test -p 1 -count=1 ./...
go vet ./...
~~~

[裸主机诊断测试](<../tests/integration/subscription_host_test.go>)仅在设置对应bare-host环境变量时运行，默认skip。此轮单独按用户问题运行一次，随后全量将该变量清空；私有文件变量始终为空。

## 5. 审查、失败调查与限制

两次独立只读审查发现并修复2个中等问题：

1. 旧地址+新runtime混合时可能误显示“监听中”，加revision一致性与fixture/工具回归。
2. PublicIP旧规则遗漏已废弃IPv6站点本地fec0::/10，补拒绝规则和字面/系统DNS/DoH单一及混合零拨号回归。

最终无剩余具体高/中审查项（不等于全面安全担保）。一次strict tsc撞到并行分组文件尚未创建导致TS2307；协调完整source freeze后检查通过，无遗留编译错误。Vite提示单chunk509.25kB、gzip150.31kB大于500kB警戒线，这是非失败构建提示；未调高阈值掩盖，代码分包可后续独立优化。

没有执行：私有完整订阅下载/解析、用户真实节点探测、系统代理/DNS更改、race、Linux/Docker实跑、容量/公网吞吐。截图中的网络问题已定位和传输层验证，但私有内容可能另有格式/服务端授权问题，应由新版明确诊断后再处理。

## 6. 构建与运行差异

23:18只读核对用户运行PID29280、Mihomo25908，应用启动22:30:36，仍为M9构建SHA2562F3FB93D…；本轮未停止或替换。新Windows暂存[程序](<../bin/updates/uniproxy.exe>)19779584字节，SHA25622596106E243811EC3FCB81A5C39A982643D731D9782CBAC8EDC49F1F1DC81AB。

最终文档校验20份、307个本地链接全部有效，围栏配对；本轮协作者与后台任务全部收集，23:21用户进程/哈希不变且测试19090已退出。

Linux暂存路径与完整哈希、数据/CWD规则见[接续快照](<session-handoff.md>)。升级仍需用户允许停机，保留正确数据/主密钥、原位置启动并刷新；不要把源码/测试完成称为线上已生效。

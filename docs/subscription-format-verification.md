# M17 — 订阅格式兼容修复验收

更新：2026-10-20 23:32（Asia/Shanghai）。对应R25；本文记录开发阶段的实现与本地验证，当时未发布或部署；替代独立review最终点验通过，无剩余已知阻塞。2026-10-21已另获发布授权，后续状态见[v0.2.3发布验收](<release-0.2.3-verification.md>)。结果以本文测试表和[接续快照](<session-handoff.md>)为准。

## 1. 原因与范围

截图报错发生在订阅下载后、节点解析阶段，不是HTTPS证书握手错误。对用户明确提供的源进行一次有界安全下载，原始响应仅在仓库外临时目录处理，验收结束已清理本轮临时副本。返回7个VMess，其中4个同时带 `tls: false` 和 `skip-cert-verify: false`。旧版按TLS字段是否存在拒绝，恰好产生4项错误，订阅来源完整性保护因此不导入全部来源。合成回归先复现同一错误；修复后的共享解析器已完整解析7个，未改变TLS开关或主动关闭证书校验。

同一请求中提供的14个客户端名称实际对应8个不同URL，只下载每个唯一地址；不把客户端名称当作格式识别依据。原始URL、账号、节点地址、密码与完整配置均不写入本文/仓库/日志；没有请求节点握手或测速，也没有访问运行中的数据库/主密钥。

## 2. 实现

- [TLS读取](<../internal/importer/native.go>)：可选TLS关闭时，仅消费类型正确的false验证绕过标志和精确空SNI，输出与旧已支持的不带占位字段配置一致；不根据443端口、SNI或指纹猜测开启TLS。非空SNI、uTLS指纹、ALPN、证书pin/name、Reality或true绕过标志仍报错。特别是不丢弃非空servername（Mihomo中它可能也是gRPC authority）。
- [sing-box](<../internal/importer/singbox.go>)：同样只接受关闭TLS中的显式无操作字段；嵌套uTLS关闭+空指纹、Reality仅关闭标志可以忽略，活动/未知/文件字段仍拒绝。[插件](<../internal/importer/transport.go>)只允许关闭TLS+false验证标志，不改变插件transport或密码。
- [客户端文本解析](<../internal/importer/client_formats.go>)：独立实现Quantumult X `protocol=endpoint,...` 与Surge/Surfboard/Loon `name=protocol,...` 静态节点适配，进入现有native白名单校验。支持双引号中的逗号、等号和转义，IPv6端点及原始行号；不对INI密码做URI解码。协议参数按方言映射，无法精确表达的设置明确报错。
- 完整客户端文件仅取 `[server_local]` / `[Proxy]`，不采用全局监听、规则、DNS、MITM或脚本，不执行/请求其中任何内容。含实际节点的远程provider/include/条件指令拒绝，不能把漏节点的文件当作完整来源；精确无参数 `DIRECT = direct` / `REJECT = reject` 仅作为完整Proxy段中的非节点策略声明忽略，不成为可选出口或直连回退。
- [URI处理](<../internal/importer/uri.go>)只在解码后值逐字相同的SNI别名间接受重复语义，冲突值/相同key重复仍拒绝；其他选项别名策略不变。[分发](<../internal/importer/parser.go>)允许URI列表开头一个有界 `STATUS=` 展示元数据，重复/中途/非法行仍报错；没有节点不能成为成功空刷新。
- API/schema无变动。仍按订阅来源全有或全无，普通和强制刷新都不能绕过不完整解析；保留节点归属、稳定身份和中转依赖。下载层公网地址、Fake-IP复核、原域名TLS及重定向边界不变。

## 3. 参考源码与差异

以下均为只读GitHub源码核对，不是复制/翻译GPL实现：

| 参考 | 实际处理路径 | 借鉴与不采用的行为 |
|---|---|---|
| [Clash Verge Rev 9dd5aa6](<https://github.com/clash-verge-rev/clash-verge-rev/blob/9dd5aa676b7fc8c3a82c968103de89b07bf0a9be/src-tauri/src/config/prfitem.rs#L254-L381>) | 下载、BOM/元数据、YAML Mapping含proxies/provider；[运行前Mihomo -t](<https://github.com/clash-verge-rev/clash-verge-rev/blob/9dd5aa676b7fc8c3a82c968103de89b07bf0a9be/src-tauri/src/core/validate.rs#L349-L383>) | 不是通用节点URI转换器；不照搬外部provider/系统代理/不安全下载选项 |
| [FlClash 8d4f974](<https://github.com/chen08209/FlClash/blob/8d4f9744948751f1273fe871e1d05276fac09b0e/lib/core/controller.dart#L99-L128>) | 配置解码失败后[调用core转换](<https://github.com/chen08209/FlClash/blob/8d4f9744948751f1273fe871e1d05276fac09b0e/core/proxy.go#L213-L219>)，不是Dart自行解析每协议 | 内容识别优先，不按URL后缀；不照搬跳过坏节点后视为完整成功 |
| [FlClash固定core 8380d0e](<https://github.com/chen08209/Clash.Meta/blob/8380d0e7462f061a964323e9bf3129a236fcc5d5/common/convert/converter.go#L284-L328>) | VMess转换明确产生tls:false及skip-cert-verify:false | 证明模板占位是正常输入；不照搬HTTP/SOCKS转换中默认跳证书验证 |
| [Mihomo v1.19.32 / 88dcbf7](<https://github.com/MetaCubeX/mihomo/blob/88dcbf7f1614a67c3b36b848ee3592dfa92ada36/adapter/outbound/vmess.go#L609-L645>) | tls开关决定握手，servername还可作gRPC Host | 不能一概删非空SNI来兼容；本轮只消除无效果默认字段 |

静态文本语法还核对[Quantumult X样例](<https://github.com/crossutility/Quantumult-X/blob/master/sample.conf>)、[Surge格式](<https://manual.nssurge.com/profile/format.html>)、[Loon节点说明](<https://nsloon.app/docs/Node/>)。上游完整软件的导入成功不代表内核运行有效或代理连通。Verge core构建使用移动stable/alpha下载，不能把应用提交当作固定内核身份；UniProxy仍固定1.19.32。

## 4. 实际验证记录

最终源码已冻结，四平台候选构建完成，Windows最终文件的编译程序验收通过；下列结果均为隔离本地开发验收，不等于已更新用户正在运行的版本。

- 旧实现基线：importer/safehttp/app测试通过，新增TLS正例及同值SNI测试按预期失败，复现占位/别名问题。
- TLS修复后：聚焦TLS/URI/危险设置回归通过；临时SQLite+fakeCore测试7节点预览/导入/去默认字段刷新/普通及强制错误刷新保留通过。
- 严格TypeScript/noUnused与36项UI工具通过。
- 静态方言API（QX/Surge/Loon/STATUS-URI/Base64）预览/提交5种通过；9组合成格式全部过固定Mihomo1.19.32配置校验（1.96秒），不启动代理。
- 用户授权9份只读样例离线验证通过：7份有效样例都经过native身份round-trip和真实core-t；2份非法SNI样例按预期拒绝，未作为成功节点导入。最终样例重放4.22秒通过；3种自有回环TLS观察1.05秒通过。没有运行任何私有上游。
- 37/37完整隔离浏览器约1.1分钟通过，末次两项Host边界硬化后再跑5项导入/下载诊断17.2秒通过；严格TypeScript/noUnused和36/36工具再次通过。
- 全量Go `go test -mod=readonly -p 1 -count=1 ./...` 显式临时候选和本机core通过，`go vet ./...`通过；Windows控制台2轮WM_CLOSE退出0/重启及独立失败窗口退出1/日志通过。最后URI诊断固定优先级变更后又跑4000次并行负例通过，候选刷新后再次验收。
- 静态适配24函数通过（Lead补显式YAML键分发回归）；最终30秒2worker适配fuzz188523次通过（33.120秒），完整解析器20秒fuzz125370次通过（22.655秒）。fuzz只用合成种子，无私有样例。

| 样例（URL不落文档） | 实际响应格式 | 有效节点 / 错误 | 结论 |
|---|---|---|---|
| Shadowrocket/Linux基础地址 | Base64 URI + STATUS | 12 / 0 | 全量解析、离线core-t通过 |
| Quantumult X | server_local全配置 | 12 / 0 | 静态段解析、离线core-t通过 |
| Stash/Clash Windows/macOS共用地址 | Clash YAML | 12 / 0 | 全量解析、离线core-t通过 |
| Loon | Proxy全配置 | 12 / 0 | 静态段解析、离线core-t通过 |
| Clash Android | Clash YAML | 12 / 0 | 全量解析、离线core-t通过 |
| Surfboard | Proxy全配置 + MANAGED-CONFIG | 12 / 0 | 静态段解析、离线core-t通过 |
| SagerNet/AnXray/v2rayNG/v2ray共用地址 | Base64 Trojan URI | 0 / 5 | 响应内SNI含@，按预期拒绝 |
| 标为Surge4的地址 | 实际Base64 Trojan URI + STATUS | 0 / 5 | 同样非法SNI，不能按后缀猜格式/修主机名 |
| 截图报告的订阅 | Clash YAML / VMess | 7 / 0 | 4个明文占位误拒已修，离线core-t通过 |

初次唯一base下载遇公共DNS超时，按相同安全客户端只重试缺失地址一次后成功；未打开内网或跳过证书验证。两个URI样本的SNI实际含 `@`，不是合法主机名，修复应继续拒绝；不得为了让全部样例显示成功而删除/替换SNI或自动置insecure。

## 5. 复查发现与保守兼容边界

- 独立review发现Surge策略组 `policy-path` 实际可加载额外节点，不能当作普通全局选组信息忽略；加入有界拒绝和普通/force刷新保留回归，禁止静默漏源节点。依据[Surge Policy Including](<https://manual.nssurge.com/policy-groups/policy-including.html>)。
- TLS+WS的HTTP Host与TLS servername可能不同；[Surge](<https://manual.nssurge.com/policies/tls.html>)缺省SNI为代理端点，而Mihomo在VMess/VLESS无servername时可能回退WS Host。相同语法的客户端默认值不能总是确定：非QX VMess/VLESS TLS+WS的Host不同于端点且SNI缺省/空时明确要求显式SNI，不能猜测或悄悄改变证书身份；QX有文档明确的obfs-host映射保留。测试覆盖IP/空/显式SNI边界。
- UDP默认存在无法从文本消歧的真实冲突：[Surge](<https://manual.nssurge.com/policies/udp.html>)VMess/Trojan自动UDP，而相同语法的[Surfboard Trojan](<https://getsurfboard.com/docs/profile-format/proxy/external-proxy/trojan>) / [VMess](<https://getsurfboard.com/docs/profile-format/proxy/external-proxy/vmess>)明确udp-relay默认false，AEAD默认也不同。本轮不新增来源方言选择字段、不按URL/UA猜软件：静态客户端文本未显式声明UDP时保守TCP-only；需要UDP必须源中显式udp-relay=true（QX/Loon也不推断未声明能力）。UI和导入说明明确提示此限制，不能宣称完整复刻Surge默认。需要精确AEAD的keyed VMess要求显式支持模式，不能用文档警告替代握手安全校验。

- 另核对并收紧：Trojan WS显式SNI不同于端点却未声明HTTP Host、SS obfs/plugin未声明Host，不允许默用内核默认Host；verify-name与skip验证冲突明确拒绝；URI混合重复/非法编码错误不再由map迭代次序随机决定；STATUS头下缩进的URI文本仍按YAML块标量处理、显式 `?` YAML键交由文档解析器，不能偷取正文/标签当节点；策略组恰好名为policy-path不误判为后续同名provider选项。原始值均不回显。
- 独立只读review的聚焦importer回归运行3次通过8.985秒；Lead接手后补显式YAML `?` 键识别，适配文件最终SHA256为0bb3ac9a8efec15fcf52d58fe1e398f20f10ddf0326af43212d8b92d00549936，test为58e9a33f491d7e64746b2080d75aa593064f38c77fef65a57e4054f183ec4104。UDP歧义为公开受限行为，不宣称所有客户端默认值完全等价。首review在关闭前harness失败；替代review核对全部修复与上述源码hash，独立聚焦回归通过2.755秒，最终无剩余已知阻塞，不把失败的review流程冒充正常完成。

## 6. 最终暂存资产

身份 `0.2.2-fix.1` / `local-subscription-fix`，构建时间2026-10-20T15:30:46Z（本地23:30:46）。最终23:25候选重新跑完整Go全量（含全部TestBuilt）和全量vet通过：importer3.612秒、app12.267秒、core17.304秒、integration14.776秒；随后WM_CLOSE两轮退出0/重启、独立失败窗口退出1/日志再次通过。TestBuilt包含6种格式/结构文档预览、无凭据响应、保存、实际安全冲突、CLI退出2、嵌入UI、3轮强杀重启/子进程退出、启动日志/healthcheck。最后策略组同名保护与显式YAML键已包含在最终源码；之后仅将UDP保守提示移至文本/文件/订阅全部模式可见，5聚焦浏览器12.3秒与严格TS通过，重新构建23:30最终候选。该最终文件全部TestBuilt通过28.278秒（含首次CLI启动等待）、WM_CLOSE两轮/错误窗口再次通过，不交付过期嵌入Web。

| 产物 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/updates/subscription-fix/uniproxy-windows-amd64.exe>) | 13999616 | 54cb442153c2c8554bcc88b3ede6bbfdaa4090961771c22e062dbc772b22f511 |
| [Windows arm64](<../bin/updates/subscription-fix/uniproxy-windows-arm64.exe>) | 12921856 | 66a66fd37f3ecf6f0f9c71f474a2d33c06ee5f6bcaf03fd60ccd23fce17d0f3a |
| [Linux amd64](<../bin/updates/subscription-fix/uniproxy-linux-amd64>) | 13648032 | 0e48ce9cb7f6de3985a720470a6e8a87e958c1652a06f5a026fa97a457a82be2 |
| [Linux arm64](<../bin/updates/subscription-fix/uniproxy-linux-arm64>) | 12845216 | d40d85c77ad2c2af0a3b7496f4ecd246c68a0dd962fcd4ace4ed63295d8c4469 |

[校验清单](<../bin/updates/subscription-fix/SHA256SUMS.txt>)与[安全换用说明](<../bin/updates/subscription-fix/使用说明.md>)同目录。原始订阅下载/清单测试结束后已仅清理本轮创建的11个临时文件；不保留复制凭据，不动用户原始文件/数据库/此前恢复证据。

## 7. 交付边界

2026-10-20开发轮只修源码与暂存候选，当时不提交/推送/发布，也未替换或停止用户服务。v0.2.2标签/正式下载不包含这些新改动；2026-10-21后续发布另见[v0.2.3记录](<release-0.2.3-verification.md>)。解析和Mihomo `-t` 是离线结构/配置验收，不是远端连通性测试；Linux/ARM64实跑、Docker、race与容量/吞吐未在本轮验收。

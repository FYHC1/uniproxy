# UniProxy API 使用示例

所有管理请求都发送到 UniProxy 的管理端口（默认9090），不是代理入口端口。启用管理Token时附带 Authorization: Bearer。写操作使用 Content-Type: application/json。

## 自动化的基本顺序

1. GET /api/bootstrap 读取当前节点、分组、池、入口及运行状态。
2. POST /api/import 创建节点/订阅；可以先 preview:true 校验预览。
3. 创建池或独享入口。不要根据显示名称猜测ID，使用API返回ID。
4. 检查修改响应中的 runtime，只有 running=true 且 dirty=false 才代表期望配置已应用。
5. POST /api/tests 获取异步任务ID，GET /api/jobs/{id}查询结果。

## 节点导入

~~~json
{
  "mode": "text",
  "content": "socks5://user:password@127.0.0.1:1080?udp=true#my-node",
  "groupIds": [],
  "groupNames": ["浏览器", "自动化"],
  "preview": false
}
~~~

订阅使用 mode:subscriptions，content 中每行一个HTTP(S) URL。文件上传或拖入由Web读取为文本后使用 mode:file，同样最大5MiB。Clash同文件dialer-proxy依赖自动保留；预览可显示dialerProxy。公开节点可含dialerId、dialerName、dialerError；后者表示中转缺失/禁用，不能直接转发。预览诊断totalErrors为完整数量，errorsTruncated表示详细错误被截断（最多128条），后续有效节点仍会处理。批量订阅不支持从URL查询参数自动推断组名，groupIds/groupNames明确控制分组。

## 订阅下载与排错

“预览”由管理服务联网下载，不是浏览器下载，也不等于已经保存。默认不自动使用环境/系统HTTP代理；操作系统TUN路由仍可能影响实际网络。

- 系统DNS若对域名只返回198.18/15的Fake-IP，客户端自动向固定公共加密DNS复核。Cloudflare（1.1.1.1）优先、Google（8.8.8.8）次之，仅查询主机名的A/AAAA；订阅路径和查询令牌不会发给DNS服务。
- 公共DNS结果仍需通过公网地址验证，再固定数字IP连接，原始域名用于HTTPS证书验证。不会直接放行Fake-IP、关闭证书校验或启用全局内网访问。普通私网/混合答案、保留地址字面URL默认仍拒绝。
- 出错按DNS、Fake-IP复核、内网拦截、连接、TLS、重定向、HTTP状态、响应大小/读取或HTML页面分别提示。诊断不包含原始链接、响应内容或证书详情。
- 下载成功但配置不完整时说明总错误数量及首个安全解析原因，并保留该来源的所有旧节点；强制刷新也不绕过格式错误。真正空200的清理仍需显式force。
- 若需要登录/验证的HTML页面，而不是实际订阅内容，应获取正确直链或使用受信任客户端下载后的本地文件；不要为此禁用TLS或把含令牌的链接交给第三方在线转换器。

## 四种代理池策略

POST /api/pools，PUT /api/pools/{id}完整替换可编辑配置。

~~~json
{
  "name": "浏览器池",
  "strategy": "url-test",
  "hashKey": "",
  "nodeIds": ["replace-with-node-id"],
  "groupIds": [],
  "groupMode": "any",
  "excludeNodeIds": [],
  "excludeGroupIds": [],
  "enabled": true,
  "udp": false,
  "testUrl": "https://www.gstatic.com/generate_204",
  "intervalSeconds": 300,
  "timeoutMs": 8000,
  "toleranceMs": 50
}
~~~

strategy可取url-test、round-robin、consistent-hashing、sticky-sessions。后两者允许hashKey:in-user，但UDP入口不能绑定in-user池，TCP入口必须配置已注册代理账户。池的udp:true会过滤不具备UDP能力的节点，而不是使TCP节点获得UDP能力。

## 一步创建代理池和共享监听

Web 新建代理池使用 POST /api/pools/with-listener，不再分两次保存。旧 POST /api/pools 仍可只创建池，兼容自动化脚本。

~~~json
{
  "pool": {
    "name": "自动化出口",
    "strategy": "round-robin",
    "nodeIds": ["replace-with-node-id"],
    "groupIds": [],
    "excludeNodeIds": [],
    "excludeGroupIds": []
  },
  "listener": {
    "protocol": "mixed",
    "host": "127.0.0.1",
    "port": 0,
    "udp": false,
    "users": []
  }
}
~~~

- 两个对象均必填；返回 data:{pool,listener} 和 runtime。服务端创建 ID 并固定 kind=pool、targetId=新池 ID，拒绝用户注入身份字段。
- port:0 或省略时，在设置范围内选空闲端口；显式端口允许范围外但必须在1024–65535且不冲突。所有已有监听（包括禁用）和管理端口被排除。需要 UDP 时同时探测 UDP 占用。
- 默认回环 mixed TCP；不会随池 udp:true 自动开启入口 UDP。非回环必须认证，UDP仍只回环，in-user入口必须认证且不能 UDP。
- 池或入口验证失败，不保存任何一个；内核应用失败则两者可能已经保存，runtime.dirty=true。不要自动重试创建或补偿删除池，先刷新核实。端口探测不是永久预留，最终以内核运行状态为准。

## 从池移除节点（不删除总节点）

DELETE /api/pools/{poolId}/members/{nodeId} 返回 Mutation<Pool>。只从当前池手动引用删除节点 ID，并加入 excludeNodeIds，避免包含分组立即把节点加回。重复请求效果不变，池/节点不存在返回404；不会更改节点分组、订阅、中转依赖或其他入口。

恢复时在编辑池中重新勾选节点，或用 PUT /api/pools/{id} 移除该节点排除项。旧 excludeGroupIds 仍具优先级。删除最后成员不会删除监听，空池拒绝新连接。

Web 的包含分组限定候选范围并按 any/all 动态纳入；勾选框反映纳入/排除，取消组内勾选写入节点排除。全选作用于当前搜索的全部结果而非首100项。排除分组选择器已移除，新池默认空；旧排除规则编辑时保留并提示，只有明确清除后才移除。节点管理页的排除组筛选不受此改动影响。

## 监听入口

POST /api/listeners，PUT /api/listeners/{id}。

~~~json
{
  "name": "浏览器入口",
  "kind": "pool",
  "targetId": "replace-with-pool-id",
  "protocol": "mixed",
  "host": "127.0.0.1",
  "port": 20000,
  "udp": false,
  "enabled": true,
  "users": [{"username": "browser", "password": "set-a-strong-password"}]
}
~~~

独享入口改kind为node，targetId为节点ID。监听列表返回hasPassword而不返回password。编辑同一username时省略或留空password表示保留旧密码；新增/改名账户必须提供密码。users:[]移除认证，只允许回环IP。UDP只允许回环IP且协议mixed/socks。

批量端口POST /api/listeners/batch：

~~~json
{"nodeIds":["node-id-a","node-id-b"],"protocol":"mixed","udp":false,"host":"127.0.0.1","users":[]}
~~~

服务在设置中的端口范围分配，整个批次通过校验后一次应用。 监听名称由服务生成为“<节点名> · 独享”，节点名接近160字符上限时会按字符边界截断并保留后缀，不会让整批失败。显示名称统一限制1–160个字符；导入的超长上游标签会截断保留节点，预览与保存结果一致。

## 节点批量操作

POST /api/nodes/batch：

~~~json
{"ids":["node-id-a","node-id-b"],"action":"disable"}
~~~

action：enable、disable、delete、add_groups、remove_groups；后两项附groupIds数组。

## 检测任务

~~~json
{"nodeIds":["node-id-a","node-id-b"],"kind":"latency"}
~~~

POST /api/tests返回202和Job。kind:speed执行真实限量下载。每次只运行一个用户检测任务，任务内并行；速度任务并行最多2。GET /api/jobs/{id}返回queued/running/completed/cancelled，逐节点结果。POST /api/jobs/{id}/cancel取消任务，不伪造尚未测试项。

配置尚未应用或测试中途配置版本改变时，拒绝/丢弃过期检测，不将旧内核结果写到新节点。

## 刷新与删除保护

POST /api/subscriptions/{id}/refresh，body:{}。若返回409、code:subscription_shrink，表示空订阅或超过半数节点将被删除，旧数据保留。确认确实需要清理后再次发送{force:true}。force不能绕过格式错误；删除订阅/节点是另一个显式操作。

## 错误与期望状态

- 400：无效参数/内核校验失败，变更不提交。
- 401：管理令牌缺失或错误。
- 403：Host/Origin/跨站访问不允许。
- 404：记录不存在。
- 409：检测冲突、配置未应用、异常订阅删减或源已变化。
- 415：非JSON写请求。
- 200+runtime.dirty:true：数据已保存但尚未成功应用，不能视为代理已切换；修复后POST /api/runtime/apply。

操作错误中的error可展示给管理员；code用于程序分支。不把error当作内核原始日志或上游密钥来源。

## 配置备份

GET /api/export/backup返回{schemaVersion:1,state:...}，**包含明文密钥/密码/订阅地址**。POST /api/import/backup接受同一对象（或仅{state}），恢复会替换全部配置并自动应用，属于高影响操作。配置快照最大32MiB，恢复请求最大40MiB。

GET /api/export/nodes为Clash/Mihomo proxies YAML，同样包含凭据。无任何自动上传第三方功能。

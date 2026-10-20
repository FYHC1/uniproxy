# UniProxy v0.2.1 发布验收

日期：2026-10-20（Asia/Shanghai）。用户明确授权提交、推送并发布0.2.1；本轮不部署、不修改用户数据或系统网络。

## 发布目标与代码

- 仓库：[FYHC1/uniproxy](<https://github.com/FYHC1/uniproxy>)，保持私有。
- 功能提交：[d8f394e](<https://github.com/FYHC1/uniproxy/commit/d8f394eb644159d3725b1c42a07d87bc1d7aacf1>)，包含M13内置Mihomo安装、移除独立安装器、0.2.1版本及[发布说明](<releases/v0.2.1.md>)。
- v0.2.1标签在两平台CI成功后推送，指向d8f394eb644159d3725b1c42a07d87bc1d7aacf1；已发布，非draft、非prerelease，并设为Latest。
- 实际资产：Windows/Linux × amd64/arm64四个UniProxy程序，加SHA256SUMS.txt，名单精确核对。没有上传本机旧产物、数据目录、备份或密钥。

## 提交前实际验证

- GitHub账户/远端/权限检查通过；本地与远端原始HEAD=b638308一致，v0.2.1不存在，没有重打已有标签或修改历史v0.2.0。
- 71个审阅路径提交；168份可发布文本检查未见GitHub令牌、AWS access-key、私钥块、文档个人路径；暂存索引禁止DB/key/exe/data/bin/测试报告等生成或敏感文件。
- Go全量go test -p1 -count1 ./...（显式已安装Mihomo本机受控链路）和go vet ./...通过；官网下载安装opt-in、私有导入/订阅变量为空，未重新访问私人节点或订阅。
- 严格TS、36/36前端工具测试、生产Web构建通过。Vite单chunk520.50kB/gzip153.58kB的非失败分包警告保留。
- 本地0.2.1候选CLI版本/非回环无Token拒绝/意外参数拒绝、临时数据真实内核进程和嵌入UI通过；不是最终GitHub资产的验证，资产下载后另验。
- 34/34隔离真实API浏览器回归通过（54.3秒）；测试19090临时状态，内核在线安装在测试服务端与浏览器双重禁用。
- 独立只读发布预审未发现阻塞问题。完整M13功能与下载安全验收见[上轮记录](<core-install-verification.md>)；上轮fixture/真实下载结果不是本轮重复执行。
- 最终文档28份、365本地链接全部有效，代码围栏闭合。

## CI与发布进度

- main CI：[39385956807](<https://github.com/FYHC1/uniproxy/actions/runs/39385956807>)成功，ubuntu-latest 51秒、windows-latest 2分18秒；构建/测试/vet均完成。
- 标签Release：[39386285720](<https://github.com/FYHC1/uniproxy/actions/runs/39386285720>)成功，四个平台build与release job全部通过；标签CI：[39386285704](<https://github.com/FYHC1/uniproxy/actions/runs/39386285704>)两平台成功。
- [v0.2.1 Release](<https://github.com/FYHC1/uniproxy/releases/tag/v0.2.1>)于2026-10-20 15:25:50发布，非draft、非prerelease；精确资产名单为四个UniProxy程序+SHA256SUMS.txt。四个文件和校验和清单已全部下载，与清单以及GitHub资产digest双重比对一致；中文发布说明已更新。
- Actions出现Node20 action迁移到Node24及ubuntu-latest镜像迁移的非失败注解，不影响本次成功结论；后续可维护action版本，本轮不扩大升级范围。

## 下载资产校验

发布文件保存在忽略目录[本地发布下载](<../bin/releases/v0.2.1/>)，不会提交入库，也没有替换用户程序。

| 文件 | 字节 | SHA256 |
|---|---:|---|
| [Windows amd64](<../bin/releases/v0.2.1/uniproxy-windows-amd64.exe>) | 13004800 | 27bd13b1735f6003b46b039d39ba51331b87c58ac2004aacd0f4106b4142951e |
| [Windows arm64](<../bin/releases/v0.2.1/uniproxy-windows-arm64.exe>) | 12109824 | 6d35f3a386feb60ed09b3a71f8353f3ef2c55c05246b7db497d3f58a07910ec0 |
| [Linux amd64](<../bin/releases/v0.2.1/uniproxy-linux-amd64>) | 12665016 | de578e4612fa6c8a3509f50a3eff0f3fd59aece17f5de90438da36e5643d382a |
| [Linux arm64](<../bin/releases/v0.2.1/uniproxy-linux-arm64>) | 12058808 | c5d07a82b200738f673e1bf64afd48d094c3f0bdcacad450473dc5cca76e956d |
| [SHA256清单](<../bin/releases/v0.2.1/SHA256SUMS.txt>) | 360 | b9197d3e381262a153a62a08bacbce083e5948b21a337a7c8ed2f615f0444818 |

发布的Windows amd64程序运行前摘要通过；实际-version为“UniProxy 0.2.1 · commit d8f394eb6441 · built 2026-10-20T07:24:53Z”。随后TestBuiltCLIVersionAndPublicBindGuard及TestBuiltServiceWithEmbeddedUI全部通过（包3.317秒）：临时目录、显式受控Mihomo、health就绪、嵌入UI与CSP、空配置启动/停止。没有使用用户数据。其他架构仅下载校验和构建元信息，不在Windows假称实跑。

## 用户实例与边界

15:17重新核对没有uniproxy/mihomo/testserver进程，9090/19090无监听；与11:09用户PID29448运行v0.2.0的历史观察不同，停止不是本轮操作。未读取用户数据库/主密钥、未启动真实数据实例、未迁移数据、未升级用户程序。

Linux/WindowsARM64/Docker实机部署、race detector与性能压测仍无新证据；交叉构建和CI单测不等于这几项已完成。

# UniProxy 会话接续快照

**最后核对：2026-10-20 16:44（Asia/Shanghai）。M15启动诊断/Windows自有子进程退出保护修复已完成。用户新授权发布v0.2.2，M16正在验证和发布；尚未创建新标签。当前仓库为公开（非本轮修改），不上传任何本机恢复材料或用户数据。本轮不部署/停止旧程序。**

## 30秒摘要

- 栈：Go + React/TS + SQLite加密payload + 外置Mihomo1.19.32；M13在线安装已在v0.2.1发布，M15修复将作为v0.2.2发布。
- [发布验收](<release-0.2.2-verification.md>)记录本轮进度；[中文说明](<releases/v0.2.2.md>)已准备；[M15历史验收](<windows-startup-verification.md>)记录开发验证，不冒充本轮正式资产测试。
- HEAD基线58728fb=origin/main，本地M15源码尚未提交，v0.2.2远端/本地均不存在；版本Go/package已更新0.2.2，下一步提交前回归/源文件审计。
- 16:40用户旧[bin程序](<../bin/uniproxy.exe>)PID31404、内核25652仍运行，9090监听；不停止/替换。其data路径未重新读取，禁止从exe/CWD猜测并启动相同数据实例。
- 用户数据上一轮按明确授权完成离线恢复，原损坏证据和匹配密钥仍须保留；本轮发布不再读取或修改这些文件。公开文档不保存私有备份定位或配置数量。

先读[需求](<requirements.md>)、[计划](<development-plan.md>)、[决策D22](<decisions.md>)、[开发指南](<development-guide.md>)。

## 1. 发布内容与已知边界

[入口](<../cmd/uniproxy/main.go>)在开库前检查管理端口、报告实际data路径；独立FlagSet保持语法退出码2。Windows无参数独立交互控制台失败会保留错误等Enter，普通shell/脚本/healthcheck不等待。仅服务自有安全日志16KiB，无原始配置/密钥/内核输出，独立用户cache唯一文件保存，失败temp回退；healthcheck错误不生成持久报告。

[Store](<../internal/store/store.go>)先设busy_timeout再WAL；[错误分类](<../internal/store/errors.go>)清楚报告损坏库，不重置/清空/换空库。[processguard](<../internal/processguard/processguard.go>)在Windows任何核心子进程前建无名非继承句柄KILL_ON_JOB_CLOSE，主进程和后续子孙绑定，句柄保留至OS退出，绝不可Manager.Stop/defer关闭。非Windows不变；不扫描或杀旧进程。

前轮malformed(11)直接原因是在线覆盖SQLite主库和旧WAL冲突，已有合成复现与授权恢复；不能用进程保护或重装Mihomo修复丢失页。升级只换EXE且先退出旧实例；迁移/物理恢复源目标全停，保留匹配密钥和同代WAL/SHM。

## 2. 本轮发布流程

1. v0.2.2版本/中文说明、公开安全审计、回归与本地候选验证。
2. 精确暂存源码/测试/文档；提交推送main，等待Windows/Linux CI成功。
3. 新建并推送v0.2.2，等待标签CI和四平台Release；不移动旧标签/覆盖旧资产。
4. 下载四平台主程序及SHA256SUMS，逐项比对GitHub digest，再运行Windows正式文件CLI/临时数据重启/控制台回归。
5. 回写发布URL/提交/CI/资产证据，文档收尾后工作树/远端一致，保持用户旧实例。

## 3. 验证与构建

本轮生产Web、严格TS/noUnused、36工具、Go全量/vet（显式本机Mihomo与0.2.2候选）、候选WM_CLOSE两轮/失败窗口、34项隔离API浏览器全部通过。作业输出已收集，下一步提交/CI和正式资产；详细M15历史见[验收](<windows-startup-verification.md>)，不把候选测通当作正式Release已验。

[前轮Windows修复候选](<../bin/updates/startup-fix/uniproxy-windows-amd64.exe>)：0.2.1-fix.1/local-fix，SHA2564953A4565193554FF9B47B70C3FF5786E341E6C8A4D807FAD2D2A9FCC6CC8C58，未发布。最终v0.2.2必须使用GitHub工作流资产，不用旧本机候选冒充。

Linux/WindowsARM64/Docker实机、race、所有Terminal/ConPTY宿主、私有节点/吞吐仍未验收。普通Mihomo缺失/apply失败不会使管理服务退出，不混淆installed/available/running。

## 4. 收尾与下一个动作

- 当前任务M16按用户要求发布，不扩展其他功能，不读取用户DB/密钥/订阅，不做新的恢复/部署。
- 只读发布预审子任务运行；所有实现/前轮审查已结束，无并行源码写范围。Lead独占版本/文档与发布。
- 所有后台作业需收集；不要忙轮询或重复测试。发布完成后更新本快照和计划，不保留虚假的“待发布”。
- GitHub当前PUBLIC，旧文档“保持私有”仅为当时历史；本轮没有改可见性。所有恢复副本/本地日志继续排除在仓库与资产之外。

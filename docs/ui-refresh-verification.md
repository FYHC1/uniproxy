# 全局界面视觉刷新验收（M27）

**状态：2026-11-03 本地实现并通过验证；未提交、未发布、未部署，用户运行实例未触碰。**

## 目标与范围

用户要求（m09350）：整体优化 UniProxy Web UI，做到“避免不必要的二级标签和说明，让布局来传达说明”，布局更紧凑、更易使用，界面简洁明了、一目了然；以亮色主题为基调，并参考外部项目的完成度做美化。范围是全部管理控制台页面及弹窗，不改变业务接口、数据模型、SQLite schema、Mihomo 版本或代理行为。

## 实现

- 新增 [theme.css](<../src/web/theme.css>)，在 [main.tsx](<../src/web/main.tsx>) 第 6–9 行按 `styles.css` → `poolStyles.css` → `chainStyles.css` → `theme.css` 的顺序加载，作为最后一层视觉主题；原有页面结构、组件和交互保持不变。
- 采用暖纸亮色画布、深森林墨文字、青绿/酸橙强调色、serif 大标题与深色首张统计卡；统一 shell、侧栏、顶栏、页面标题、统计条、面板、表格、按钮、表单、弹窗、池/链卡片、设置页、空态、认证页与通知的圆角、边框、阴影和间距。
- 用布局表达层级：隐藏重复的 eyebrow、页面 description、SectionHeading description、subtle-meta、footnote、group hint、context banner、侧栏说明、面包屑和页脚辅助文案；保留必要安全提示、真实状态、空态说明和就地错误。
- 提高信息密度与可读性：表格表头 10px、辅助文字 10.5px；活动列表限制 396px 内滚；表格、统计、弹窗和卡片间距压缩；移动端底部导航改为 62px 高的 7 列单行布局，390px 页面无水平溢出。
- 认证页与通知统一为同一亮色语言；`src/web/App.tsx` 的登录错误只在用户提交或点击“尝试本机免令牌访问”后显示，避免首次打开即出现红色错误。

## 页面覆盖

节点管理、分组管理、代理池、代理链、监听端口、订阅管理、运行与设置（运行概况/检测与端口/后台任务/备份与安全）、导入弹窗、池编辑弹窗、链编辑弹窗、认证页、Toast/全局告警、空态与加载态。

## 验证结果

- `pnpm run build:web`：Vite 7.3.7，4655 modules；最终资源 `internal/webui/assets/dist/assets/index-4zjJTZUI.css` 106.42kB / gzip 22.24kB、`index-HlpWasqw.js` 556.48kB / gzip 162.18kB；仅有 Vite 500kB chunk 体积提示，不是构建失败。
- 严格 TypeScript（`tsc --noEmit --noUnusedLocals --noUnusedParameters`）通过。
- `pnpm test:ui`：42/42 通过；fixture 全部 PASS，包含 401 认证、空态、真实状态、响应式 375/768/1024 等检查。
- `pnpm test:e2e`：49/49 通过（1.2m）；覆盖全部页面、链/池/分组/订阅/设置/导入/监听及 390px 响应式流程。
- 视觉检查截图（本地 `.local/ui-theme/`，不进入发布资产）：`nodes.png` 1440×1167、`groups.png`、`listeners.png`、`subscriptions.png`、`pools.png` 1440×1614、`chains.png`、`settings-runtime.png`、`settings-detection.png` 1440×1800、`settings-backup.png`、`nodes-mobile.png` 390×844、`nodes-selected.png`、`import-dialog.png`、`pool-dialog.png`、`chain-dialog.png`、`chain-dialog-mobile.png` 390×844、`auth.png` 1440×1000。
- 本机 Windows 预览：`bin/updates/ui-refresh/uniproxy-windows-amd64.exe`，14,227,456 bytes，SHA256 `54BFEBCD087DA9D05EF395785F4CC77BD54CF45AB0B2A4338D7BC36CFCFC38B8`，身份 `UniProxy 0.2.8-ui-preview.1 · commit local-ui-ref · built 2026-11-02T18:36:12Z`；隔离 19091 启动后 `/api/health` 返回 `{"ok":true,"version":"0.2.8-ui-preview.1"}`；启动与验收命令见本地 `bin/updates/ui-refresh/使用说明.md`（不进入仓库）。

## 边界

- 仅前端样式与少量登录态显示逻辑；不改变 API、模型、数据库 schema、Mihomo 版本、代理/证书/SSRF 安全策略或用户数据。
- 视觉截图与浏览器测试使用隔离 fixture（内核不可用）和 headless Microsoft Edge；未在 Safari、Firefox、真实移动设备或所有系统缩放下逐一验证。
- 预览程序是本地构建，不是正式 Release 资产；本轮未提交、未发布、未部署，正式发布和运行服务替换需另行授权。

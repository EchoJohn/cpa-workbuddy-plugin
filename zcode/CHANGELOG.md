# Changelog

## [0.1.7] - 2026-10-11

### Fixed
- 修复 OAuth 授权重定向地址：将授权 URL 中的 `redirect_uri` 由桌面客户端地址（`zcode.z.ai/app/oauth/login?redirect=zcode://oauth/callback`）调整为 CPA 宿主标准回调地址（`http://localhost:1455/auth/callback` 或 `req.BaseURL`），彻底消除登录完成后调起本地 ZCode 客户端的问题，实现直接回调至 CPA 接收凭据。
- 令牌交换兼容容错：`exchangeZaiOAuthCode` 优先使用 CPA 回调地址，同时保留备用桌面客户端地址重试兜底，并支持自适应从完整回调 URL 中解析回调基地址。

## [0.1.6] - 2026-10-11

### Fixed
- 修复 OAuth 授权重定向地址：将授权 URL 中的 `redirect_uri` 由桌面客户端地址（`zcode.z.ai/app/oauth/login?redirect=zcode://oauth/callback`）调整为 CPA 宿主标准回调地址（`http://localhost:1455/auth/callback` 或 `req.BaseURL`），彻底消除登录完成后调起本地 ZCode 客户端的问题，实现直接回调至 CPA 接收凭据。

### Performance
- 测试探活（`/test-active`）改为流式首包即停：`active_ping.go` 的 `handleTestActiveWithAuth` 改为流式请求（`stream=true, max_tokens=1`），读取首个 512 字节数据块并校验无错误后即刻返回并异步关闭底层流，消除 `GLM-5.3` 等思考模型的完整生成等待。

## [0.1.5] - 2026-10-11

### Changed
- 重构 ZCode 账号授权为官方真实 Z.ai OAuth 授权链路：
  - 授权直达端点：对接官方 `chat.z.ai/auth`，下发真实 `client_id=client_P8X5CMWmlaRO9gyO-KSqtg` 与带版本及协议的 `redirect_uri=https://zcode.z.ai/app/oauth/login?redirect=zcode%3A%2F%2Foauth%2Fcallback&app_version=3.15.1`，生成 32 字符 hex `state`；
  - 令牌交换接口：通过 `https://zcode.z.ai/api/v1/oauth/token`（`provider=zai`）将授权码交换为正式 `zcodeJwtToken`、`access_token` 及用户档案数据；
  - 凭据存储升级：`StoredAuth` 扩展 `token`、`access_token`、`user_id`、`username`、`email`、`avatar_url`、`expires_at`，保持与已有 APIKey 的完全向下兼容；
  - 桥接页面升级：`/oauth-bridge` 改为 Z.ai 官方登录授权引导与回调地址（含 code 与 state）解析提交；
  - 宿主回调自动捕获：`handlePollLogin` 支持自动读取宿主 `.oauth-zcode-provider-<state>.oauth` 载荷文件并兑换落盘；
  - 管理面板升级：`panel.html` 增加「OAuth 授权登录」按钮，支持账号用户名/邮箱/用户ID展示与凭据导入。

## [0.1.4] - 2026-10-11

### Fixed
- 修复桥接授权页丢失 `state` 参数：宿主将 URL path 与 query 分开传递（Path 不含 query、参数在 Query 字段），此前误用从 path 解析出的 query 覆盖宿主传入的 `req.Query`，导致管理后台「OAuth 登录」下发的授权链接打开后显示「缺少 state 参数」、无法完成绑定。
- 新增宿主调用契约回归测试，复现「Path 不含 query + 参数在 Query」的真实调用形状，防止再次回归。

## [0.1.3] - 2026-10-11

### Added
- 实现管理后台「OAuth 登录」桥接授权机制：支持 `StartLogin`、桥接授权输入页（`/oauth-bridge`）、`PollLogin` 轮询保存与 `RefreshAuth`，用户可在 CPA 管理后台直接点击 ZCode 发起授权绑定并自动保存账号。
- 恢复 `Capabilities.AuthProvider = true`，管理后台「OAuth 登录」卡片恢复展示且点击可正常交互完成授权。

## [0.1.2] - 2026-10-11

### Fixed
- 修正 capabilities 注册配置：将 AuthProvider 设为 false，消除在 CPA 宿主管理后台「OAuth 登录」中误暴露出 zcode-provider 卡片并在点击时报 `failed to generate authorization url` 的缺陷。
- 为 `MethodAuthLoginStart` 补充明确的拒绝提示处理（API Key 凭据请在专属面板导入管理），避免未注册方法异常。

## [0.1.1] - 2026-10-11

### Changed
- 管理面板按 WorkBuddy 面板设计系统整体重写，视觉与交互风格统一：
  - 主题令牌（`--bg/--card/--acc/--ok/--warn/--err` 等）与 WorkBuddy 面板同源，支持 `data-theme=white|dark` 与嵌入时镜像 CPA 父页主题（早期主题同步脚本 + MutationObserver）。
  - 布局改为「标题 + 副标题 + 工具栏 + 筛选标签 + 用量汇总卡 + 三列账号卡网格」。
  - 新增账号概览汇总卡（总账号数 / 可用 / 禁用 / 测试失败 / 冷却中 / 模型条目）与筛选标签实时计数。
  - 账号卡改为徽章式状态（可用 / 禁用 / 测试 / 冷却）、脱敏 Key、创建与更新时间、模型标签、卡片内操作按钮。
  - 新增右上角滑入式 toast 通知栈（成功 / 警告 / 失败三态）与统一 busy spinner 按钮态。
  - 模态框（导入 / 删除 / 测试）改为标准 modal 结构（header / body / footer + 遮罩点击关闭），删除走二次确认。
  - 新增工具栏搜索（备注 / Key / ID 模糊匹配）与排序三态（创建倒序 → 名称升序 → 名称降序）。
  - 测试模态框改为「先列出账号模型 → 点选模型即测」，结果内联展示并同步刷新账号状态。
- 管理路由统一由 `normalizeMgmtSubPath` 解析（面板 `/panel`、`/status`、`/v0/resource/...` 与 `/v0/management/...` 前缀全部兼容），移除冗余的 `isPanelPath`。
- 新增 `panel_test.go` 面板渲染与路由契约回归测试。

## [0.1.0] - 2026-10-11

### Added
- 初始版本：移植并重构 `cpa-plugin-zcode` 为 `zcode-provider` 独立插件。
- ZCode 桌面客户端指纹（`internal/mimic`）Header 注入。
- 多账号管理体系（`authfile.go`、`management.go`）：导入、导出、删除、启停与全部启停。
- 专属管理面板（`panel.html`）：支持深浅色主题、指标概览、选模型连通性测试弹窗。
- 多账号轮询调度器（`scheduler.go`）与 40x 换号 / 429 阶梯退避故障转移（`failover.go`）。
- Anthropic 协议非流式与流式推理执行器（`executor.go`、`stream.go`）。
- Token 用量统计上报（`usage_feed.go`）。

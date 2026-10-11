# 项目当前状态

## 目标与范围

- 目标：维护 CLIProxyAPI (CPA) 的 Go 原生插件套件 `cpa-workbuddy-plugin`（涵盖 8 大 AI 服务商 Provider + 1 统一用量统计插件），提供多账号管理、动态模型发现、流式转发、专属现代管理控制台、扫码/Token 登录、保号巡检与统一 token 用量统计分析服务。
- 范围：仓库内 9 大插件体系的代码实现、单元测试、资产构建、发布流水线、registry 元数据同步等：
  1. `workbuddy-provider`：腾讯 CodeBuddy（国内版与 Global 版）；
  2. `workbuddy-ai-provider`：WorkBuddy AI（独立国际版）；
  3. `traework-provider`：Trae SOLO（字节跳动海外/国内 AI IDE 客户端）；
  4. `qoderwork-provider`：QoderWork（阿里通义灵码衍生国内版）；
  5. `qoder-ai-provider`：Qoder AI（国际版 qoder.com，含 UMID 协议支持）；
  6. `cursor-provider`：Cursor（cursor.sh，Connect-RPC 协议与会话连续性）；
  7. `zcode-provider`：ZCode（BigModel Coding Plan，Anthropic 兼容与客户端真实指纹 mimic）；
  8. `gemini-provider`：Google Gemini CLI；
  9. `workbuddy-token-usage`：统一 Token 用量统计与实时看板服务。
- 非范围：CLIProxyAPI 核心服务本体；跨项目文件修改。

## 项目概况

- 状态：活跃维护中。9 大插件全景全量编译与单测验证全绿（cgo-shim 零错误）。最新发布/就绪版本：
  - `cursor-provider`: **0.1.4**（修复 Query 参数反序列化丢失导致真实发现模型被屏蔽、建立点号模型别名归一化根治上游 not_found、测试弹窗支持实时搜索过滤与秒开交互）
  - `qoder-ai-provider`: **0.1.6**（纯 Go 实现阿里云安全盾 UMID 协议与设备指纹，彻底修复签到活动隐藏与风控拦截，生产真实验证通过）
  - `zcode-provider`: **0.1.1**（全新完成移植接入：BigModel Coding Plan Anthropic 协议、客户端桌面指纹 mimic、多账号与探活面板）
  - `traework-provider`: **0.2.5**（修复签到 x-device-id 尾零风控拦截 9074、调度器绝对时间对齐、默认会话路由与配置动态联动打通）
  - `qoderwork-provider`: **0.9.27**（CN 签到端点迁移 campaigns，彻底移除保号池，对齐测试标签与低积分排序调度）
  - `workbuddy-provider`: **0.15.4**（5 模型轮测探活、移除保号池、硬排除测试标签 + 低积分优先）
  - `workbuddy-ai-provider`: **0.1.9**（补齐完整路由层单测资产、5 模型轮测探活、国际版签到）
  - `gemini-provider`: **0.1.1**
  - `workbuddy-token-usage`: **0.2.4**（移除短轮询，改手动刷新并全面接入 Cursor/Gemini/ZCode 供应商归一化映射）
- 活动工作区：F:\cpa-plugin
- 当前时间：2026-10-11 (GMT+8)

## 活动会话进展摘要

- 当前会话（2026-10-11）：**Cursor Provider 0.1.4 修复模型获取与测试探活缺陷（Query 参数反序列化丢失、真实 256 模型展现、点号模型别名归一化与测试弹窗交互增强）**：
  - 用户反馈与目标：用户指出 Cursor 插件的模型获取好像有问题，测试不通过，附带管理面板与测试弹窗截图。
  - 核心根因精准收敛（双缺陷闭环）：
    1. **为什么测试弹窗只有 7 个固定模型而卡片显示 256 个模型**：宿主 `CLIProxyAPI` 派发 RPC `management.handle` 时，URL 路径 `Path` 为纯路由 `/v0/management/plugins/cursor-provider/models`（不含 `?`），Query 参数封装在 `url.Values`（`map[string][]string`）。`cursor-provider` 内部 `managementRequest` 原代码将 `Query` 字段打了 `json:"-"` 忽略反序列化，且代码仅通过 `parseAuthIndexFromPath` 去从 `Path` 中解析 `?`，导致 `auth_index` 永远解析为空字符串 `""`；因此管理接口 `/models` 100% 触发 fallback 返回硬编码的 `defaultCursorModels`（7~8 个旧模型），使账号真实 DiscoverModels 获取到的 256 个模型被彻底屏蔽。
    2. **为什么测试点选 `claude-3.5-sonnet` 报错 `Cursor stream failed: not_found`**：Cursor 上游 Connect-RPC `AgentService/Run` 遵循 Protobuf 模型契约，权威模型 ID 为连字符格式（如 `claude-3-5-sonnet`、`claude-3-5-haiku`、`claude-3-7-sonnet`、`gpt-4.5-preview`、`default`）；当 fallback 列表或外部请求传入带点号的 `claude-3.5-sonnet` 时，Cursor 服务端无该模型注册项，直接抛出 Connect-RPC end stream 错误 `not_found`。
  - 实施修复与系统实现（v0.1.4）：
    1. **对齐宿主 ManagementRequest 契约**：在 `management.go` 中将 `managementRequest` 字段 `Query` 正确声明为 `map[string][]string json:"Query"`；重构 `parseAuthIndexFromRequest` 优先提取 Query 参数并向下兼容 Path query string。
    2. **优雅账号回退与真实模型展现**：新增 `getFirstAvailableCredential`，当请求未指定 `auth_index` 时自动获取首个可用账号的凭据执行 `DiscoverModels`，仅在系统无任何可用账号时才回退默认列表，确保账号拥有的 256 个真实模型顺利透传到前端。
    3. **模型别名权威归一化机制（根除 `not_found`）**：在 `internal/openai/request.go` 中实现 `CanonicalModel`，将 `claude-3.5-sonnet` 等点号模型、版本后缀及常见别名统一映射为上游权威连字符 ID（`claude-3-5-sonnet` 等），并在 `handleTestActiveWithAuth` 与 `ParseChatRequest` 中统一接入；修正 `defaultCursorModels` 剔除错误点号项，补充 `default` 与 `gpt-4.5-preview`。
    4. **前端测试弹窗海量模型搜索与秒开交互**：在 `assets/management.html` 的测试弹窗中增加实时搜索过滤输入框（`filterTestModels`），支持在 256 个海量模型中即时模糊过滤；打开弹窗时优先利用当前卡片已有的模型缓存即时秒开，后台并发异步校验刷新最新列表。
  - 验证与门禁全绿：
    - `python scripts/cgo-shim-build.py cursor` build/vet/test 全绿；
    - 必失败哨兵反证测试成功拦截；
    - Node 前端自动化测试 `node test/cursor/panel_auto_entry_repro.mjs` 场景全断言 100% 通过；
    - 归档 6-review 风格回归报告 `doc/6-review/2026-10-11_180000_Cursor模型获取与测试探活修复_6-review.md`（STYLE: PASS）；
    - 知识库沉淀 `Cursor-Connect-RPC模型命名契约与宿主管理Query反序列化修复.md` 并入网挂载。

- 当前会话（2026-10-11）：**QoderWork 与 Qoder AI 两插件模型推理请求终止契约与长超时加固（对齐 WorkBuddy 成熟机制）**：
  - 用户反馈与目标：测试出 Qoder 的两个插件模型请求有些问题，不稳定且有时候超时时间很长，要求参考 WorkBuddy 插件排查推理请求不完善之处并彻底修复。
  - 核心根因定位（多维收敛）：
    1. **同步 collect 路径遇到 [DONE] 误用 continue 导致死等长超时**：`collectUpstreamStreamQoder` 历史代码对 `bodyStr == "[DONE]"` 误用了 `continue` 而非 `break`；上游长连接 Keep-Alive 未断开时扫描器永久挂起，直到底层长超时。
    2. **多种 [DONE] 协议形态无法被识别**：上游发送的顶层裸 `data: [DONE]`、带空格或换行的变体在反序列化 JSON 失败后被静默跳过，未能触发 `break`，流式响应迟迟不 close 导致客户端长时间转圈等待。
    3. **换号重试 Context 丢失与挂死**：`rebuildRequestWithQoderAuth` 丢失了请求级 Context（硬编码 `context.Background()`），客户端断开时无法级联取消上游请求，后台 goroutine 持续占用资源；且换号前缺失会话粘性驱逐（`evictSessionBindingsForAuth`）。
    4. **非流式执行无超时控制**：`doExecuteOnceQoder` 未施加 Context 超时控制，上游无响应时无限挂起。
    5. **UTF-8 字符截断与 prompt 为空隐患**：`buildQoderBody` 中 `prompt[:30]` 按字节截断易切断多字节中文制造非法 UTF-8，且无 user 消息时直接 500 报错。
  - 实施修复与系统实现（两插件完全对齐）：
    1. **抽取出统一的 `parseQoderSSEChunk`**：同时识别顶层 `[DONE]`、嵌套信封 `[DONE]` 及扁平 OpenAI chunk，遇到终止即刻 `break` 退出循环并关闭流。
    2. **全链路 Context 超时与会话驱逐对齐**：`rebuildRequestWithQoderAuth` 支持继承 Context；非流式与同步路径施加 10 分钟预算超时；换号前统一驱逐会话绑定。
    3. **UTF-8 安全截断与 prompt 降级兜底**：使用 `[]rune(prompt)[:30]` 字符切片；`extractLatestUserPrompt` 增加非 user 消息优雅降级。
    4. **消除 streamCloseOnce 内存泄露**：在 `once.Do` 执行后主动清理 map 条目。
  - 验证与门禁全绿：
    - `python scripts/cgo-shim-build.py qoderwork` build/vet/test 全绿（7.112s）；
    - `python scripts/cgo-shim-build.py qoder-ai` build/vet/test 全绿（7.188s）；
    - `python scripts/cgo-shim-build.py workbuddy` 基线验证全绿（11.273s）；
    - 新增单测通过并经必失败哨兵反证确证真实进入编译；
    - 6-review 风格回归归档 `doc/6-review/2026-10-11_173500_Qoder插件推理请求终止契约与超时加固_6-review.md`（STYLE: PASS）；
    - 知识库沉淀 `Qoder插件推理请求终止契约与KeepAlive超时规避.md`。

- 当前会话（2026-10-11）：**吸收「生产容器重启红线」到项目规则与 skill（非必要不重启 cpa 容器）**：
  - 用户口径：非必要不要重启生产的 cpa 容器；插件的更新安装一般不需要重启（`plugin-store install` 热重载即生效）。
  - 依据与风险：install 默认返回 `restart_required:false`，`docker logs cli-proxy-api | grep "hot reloaded"` 可见 `active_version` 生效；重启 `cli-proxy-api` 会中断在线流量并触发 60–80 秒冷启动（9 插件 + 模型刷新），属高成本、高影响操作。
  - 落点（四处同步）：
    1. `AGENTS.md` / `CLAUDE.md`「项目特有规则 → 仓库与发布」新增「生产容器重启红线（非必要不重启，强制）」；
    2. 项目 skill `project-cpa-workbuddy-plugin-release-rules`：Step 13 校验项改用 `docker logs | grep "hot reloaded"` 判定（明确不需重启）、Step 13 新增重启红线条、权责边界末条由「提醒用户重启宿主」改为「不主动重启生产 cpa 容器」；
    3. 项目 skill `project-cpa-workbuddy-plugin-host-config-safety-rules`：配置写入红线新增第 7 条（配置损坏恢复为重启的必要例外，恢复流程 Step 4 同步注明）；
    4. 两个 skill 的 `references/source-notes.md` 追加变更记录；`PROJECT_MEMORY.md`（核心记忆 + 变更记录）与 `PROJECT_HISTORY.md`（事件 + 计数锚点）同步。
  - 例外边界：仅在热重载经证据确认未生效（如踩坑 48 偶发形态）或生产配置损坏恢复等必要场景，且用户明确确认时才重启；否则保持容器运行，把重启时机交用户决定。

- 当前会话（2026-10-11）：**Qoder AI 国际版签到活动 UMID 设备指纹与多账号风控拦截修复，发布部署 `qoder-ai-provider 0.1.6` 并完成生产真实 +100 积分与批量签到验收**：
  - 用户反馈与目标：用户指出 Qoder AI 签到仍有问题，客户端显示可以签到（每日 100 Credits 20:35:13，每日 10:00 UTC+8 刷新），但生产上的账号一个都无法真正签到成功，积分没有增加，要求彻底修复。
  - 核心排查与根因收敛：
    1. **为什么客户端有活动而接口无活动**：反解官方客户端 `app.asar` 发现，客户端在调用 campaigns 接口前通过阿里云安全盾（Alibaba SecurityGuard / UMID）获取 `machineToken`（`P1gA...`），并计算出 18 字符的 `machineType` 与 `machineCode`；缺少这些设备头时，上游风控静默隐藏每日活动返回空列表。
    2. **为什么多账号会签到失败**：上游实行同机每日限领 1 次的风控拦截策略，若多账号使用相同的机器标识，后续账号将返回 `BLOCKED: SAME_PERSON_ALREADY_CLAIMED` 阻断。
    3. **为什么面板刷新延迟**：`checkin.go` 提取 `rewardCredits` 时使用 float64 类型断言，而底报文为 int64，导致断言失败且未即时回写缓存快照。
  - 实施修复与系统实现（v0.1.6）：
    1. **纯 Go 独立实现安全盾协议（零 CGO）**：在 `qoder-ai/umid.go`、`umid_codec.go`、`umid_crypto.go` 中完整实现 Section 0x1b 数据段封装、zlib 压缩、10 组密钥 AES-128-CBC 加密以及标准 XXH64 派生 `machineType`/`machineCode`，支持跨全部 7 平台构建。
    2. **多账号确定性虚拟硬件指纹隔离**：按账号 UID 确定性生成独立的硬件特征并获取独立的设备凭证，遇到风控拦截时自动轮换重试（最多 3 次）。
    3. **请求头注入与缓存即时刷新**：在 `campaignHeaders` 注入完整设备标识，签到成功后即时刷新缓存额度与签到快照。
  - 验证与门禁全绿：
    - 本地 `python scripts/cgo-shim-build.py qoder-ai` build/vet/test 全绿；
    - 自动化单测覆盖算法金标准对照、解密还原与请求头断言，必失败反证全部通过；
    - 6-review 风格回归归档 `doc/6-review/2026-10-11_150600_QoderAI签到UMID设备标识修复_6-review.md`（STYLE: PASS）。
  - 发布闭环与生产验收：
    - 源码 commit `973279c` 推送到 main；CI run `38084535313` 65 个 jobs 全部成功；
    - 8 个 release 资产校验入库 commit `0952920`；registry.json 0.1.6 同步 commit `292ceb9`；
    - 生产服务器执行 `plugin-store install qoder-ai-provider?version=0.1.6` 成功热重载生效；
    - 生产账号 `u4aac824d` 调用签到返回成功，赠送/签到积分从 0 真实增加到 100；批量签到 `summary={already:6, no_activity:2, fail:0}` 全量通过！
    - 发布后清理：删除 release-assets 下旧版 0.1.5 资产。

- 当前会话（2026-10-11）：**消除 Cursor 双菜单重复注册问题，并全量对齐 WorkBuddy / ZCode 现代面板设计风格**：
  - 用户反馈与目标：用户指出侧边栏菜单出现 2 个 Cursor，且面板与 workbuddy 完全不是一个风格，要求再次调整。
  - 核心排查与根因收敛：
    1. **为什么菜单会有 2 个**：`cursor/internal/plugin/management.go` 中的 `managementRegistration()` 此前在 `Resources` 中同时注册了 `{Path: "/panel", Menu: "Cursor"}` 与 `{Path: "/status", Menu: "Cursor"}`。宿主 CLIProxyAPI 在生成侧边栏菜单时为每一个带 `Menu` 属性的 Resource 创建一个菜单条目，导致侧边栏重复渲染两个 "Cursor" 菜单项。
    2. **面板风格不一致**：Cursor 面板原本使用的是移植初期遗留的旧版 `management.html`，缺乏 CPA Management 统一主题变量、布局结构、操作栏、状态过滤胶囊、汇总概览与现代卡片网格系统。
  - 实施修复与全量对齐：
    1. **消除双菜单**：修改 `cursor/internal/plugin/management.go`，将 `managementRegistration().Resources` 收敛为仅声明 `/panel` 单一菜单项，同时保持后端路由对 `/status` 兼容别名及 `/enable-all`、`/disable-all` 的完整支持；同步更新 `handler_registration_test.go` 单测断言。
    2. **面板全量对齐 WorkBuddy / ZCode 规范**：
       - **主题与视觉体系**：接入 CPA 官方 `:root` / `:root[data-theme="white"]` / `:root[data-theme="dark"]` CSS 变量系统与主题监听同步脚本 `__cursorThemeSync`（支持嵌入父窗口 `MutationObserver` 监听与独立深浅色模式）；
       - **布局与工具栏**：采用 `.wrap` 规范，增加 `导入 Token`、`导出凭证`、`全部启用`、`全部禁用`、实时搜索框与排序切换按钮；
       - **状态过滤与汇总概览**：新增「全部 / 可用 / 禁用 / 异常」状态筛选胶囊与 `.summary` 统计概览卡片；
       - **卡片网格与交互**：现代化响应式 `.grid` 与 `.card`，展示账号名称、AuthIndex、状态 Badge、累计请求（成功/失败）、支持的核心模型标签、测试探活按钮、启用/禁用切换与右上角红叉删除确认；
       - **弹窗与组件**：导入 Token 弹窗（支持多行输入与备份恢复）、选模型测试探活弹窗、删除二次确认弹窗与右上角滑入式 Toast 通知系统。
  - 验证与门禁全绿：
    - `python scripts/cgo-shim-build.py cursor`：build、vet、test 全部通过（all green）；
    - 必失败哨兵反证测试真实进编译执行；
    - `node test/cursor/panel_auto_entry_repro.mjs`：免密直入与各种场景全部 100% PASS；
    - `node --check`：内嵌脚本语法校验全部通过。

- 当前会话（2026-10-11）：**Cursor Provider 0.1.2 对齐 WorkBuddy 核心能力（统一用量上报、JSON 备份恢复、面板搜索与概览统计）**：
  - 用户反馈与目标：发现 Cursor 插件与 WorkBuddy 差距较大，要求同步 WorkBuddy 差距的功能，对 Cursor 插件进行系统改造。
  - 核心实施与同步对齐：
    1. **统一用量通道（Token Tracker 接入）**：新增 `cursor/internal/plugin/usage_feed.go`，在流式与非流式请求完成时异步追加 NDJSON 记录至 `<root>/data/token-usage-feed.ndjson`；在 `token-usage-tracker/usage_stats/auth_identity.go` 中注册 Cursor 供应商映射，打通统一大屏看板。
    2. **备份 JSON 凭据恢复导入**：增强 `cursor/internal/plugin/import.go` 的 `importBackupJSON`，智能识别完整备份导出的 JSON（以及单/数组凭据），一键批量恢复账号，与导出功能形成闭环。
    3. **面板体验全面升级**：在 `management.html` 顶部增加账号状态概览统计条（总数、活跃、禁用、可用模型），操作栏增加实时模糊搜索过滤输入框，导入弹窗新增从备份恢复独立区域。
  - 验证与门禁全绿：`python scripts/cgo-shim-build.py cursor` 与 `token-usage-tracker` 全绿；必失败哨兵反证测试真实执行；`node test/cursor/panel_auto_entry_repro.mjs` 11 项断言全通过；`node --check` 语法检查全部通过；6-review 风格回归 `STYLE: PASS`。

- 当前会话（2026-10-11）：**兄弟插件同步 workbuddy 路由能力（批次 A 至 D 全量本地实施收口，严格无提交无发布）**：
  - 用户目标：按顺序推进兄弟插件同步 workbuddy 路由能力，直到全部完成，先在本地全部完成，不提交、不发布。
  - 核心实施与全批次收口：
    1. **批次 A（traework 默认会话路由与配置动态联动修复）**：`scheduler_mode` 默认值与描述从 `off` 改为 `session`，并在 `config.go:175` 补齐 `setSchedulerMode(v)` 彻底打通配置与调度器状态脱钩，新增 `TestSchedulerMode_DefaultIsSession` 单测锁定。
    2. **批次 B（workbuddy-ai 补齐路由回归单测资产）**：为 `workbuddy-ai` 移植并补充 `scheduler_test.go`、`session_auth_test.go`、`routing_exclusion_test.go`、`failover_retry_test.go`、`accountFailover_test.go`，消除未测试盲区。
    3. **批次 C（qoderwork 深度路由改造）**：原位精简 `watchdogLoop`（移除保号池翻转，保留 10m 计数落盘与清扫驱动）；移植 `test_failed_tag.go` 并在 `active_ping.go` 补齐打标清标；`scheduler.go` 引入硬排除、低积分优先排序与全灭 defer；彻底移除 `preserve.go` 及相关引用；面板三文件接入 `test_failed` 并移除保号 UI；补齐单测全绿。
    4. **批次 D（qoder-ai 镜像结构改造与低积分选卡）**：镜像批次 C 成熟改法，并在 `active_auth.go` 补齐原计划独缺的 `accountLowerCredits` 与 `ensureDefaultActiveAuth` 低积分选卡逻辑，彻底卸载 `preserve.go` 并补齐单测。
  - 验证证据与门禁全绿：五插件（traework、workbuddy-ai、qoderwork、qoder-ai 及基线 workbuddy）本地 `cgo-shim-build.py` build/vet/test 全部通过，必失败哨兵反证全部通过，`node` 脚本对 panel.html 语法校验全绿，归档 6-review 风格回归报告 `doc/6-review/2026-10-11_041000_兄弟插件同步workbuddy路由能力全量收口_6-review.md`（STYLE: PASS）。
  - 本地保护：严格执行用户当轮显式约束，零提交、零推送、零发布，全部修改完整安全保留在工作区。

- 当前会话（2026-10-11）：**自动探活多模型轮测实施收口（v1.1 completed）与下一阶段兄弟插件路由能力同步推进**：
  - 实施计划收口：正式更新《自动测试多模型依次探活改造实施总览》（`doc/3-实施/2026-10-09_233000_自动测试多模型依次探活改造_实施总览.md`）为完成态（`status: completed`，`version: v1.1`），TASK-001 至 TASK-005 全部勾选完成，对齐 `doc/6-review/2026-10-09_234800_自动测试多模型依次探活改造_6-review.md`（STYLE: PASS）。
  - 核心能力巩固：workbuddy / workbuddy-ai / traework 具备最多 5 模型随机轮测、单次 20s 超时、整轮 30s 预算熔断、any-success 快速退出且清标，彻底阻断单模型偶发故障导致账号被调度器误杀硬排除。
  - 下一阶段计划推进：跟进《兄弟插件同步 workbuddy 路由能力实施总览》（`doc/3-实施/2026-10-10_235000_兄弟插件同步workbuddy路由能力实施总览.md`），分批次实施：
    - **批次 A（低风险）**：`traework` 对齐 `scheduler_mode: session` 默认值与描述（4 处改动）；
    - **批次 B（纯测试）**：`workbuddy-ai` 移植与补齐完整路由回归单测资产；
    - **批次 C（结构改造）**：`qoderwork` 迁移 watchdog 计数落盘驱动源、接入 `test_failed` 机制、彻底移除 `preserve` 保号池并补齐全灭 defer 与低积分优先排序；
    - **批次 D（结构改造）**：`qoder-ai` 镜像批次 C 结构改造并补 `ensureDefaultActiveAuth` 低积分比较。

- 当前会话（2026-10-11）：**Cursor Provider 0.1.1 补充官方高清图标、路由规范对齐、选模型测试探活与可用性完善**：
  - 用户反馈与目标：继续完善 Cursor 插件，补充图标等等，解决 Cursor 插件目前基本无法使用的问题。
  - 核心问题排查与解决：
    1. **图标缺失与破图**：补充官方高清图标至 `assets/icons/Cursor.png`（640×640 32-bit RGBA），并在 `cursor/internal/plugin/handler.go` 配置 `pluginLogoURL`、在 `registry.json` 中配置 `logo` 字段，并在面板 header 左侧增加品牌 Logo。
    2. **面板路由 404 致无法打开**：CPA 宿主后台管理点击面板默认请求 `/v0/resource/plugins/cursor-provider/panel`，而原插件仅注册 `/status`；在资源路由中补充标准 `/panel` 并兼容 `/status` 别名。
    3. **管理接口 `/accounts` 缺失**：增加 `GET /plugins/cursor-provider/accounts` 别名路由指向账号状态，对齐全仓库统一接口规范。
    4. **缺少模型测试探活**：新增 `GET /models?auth_index=` 与 `POST /test-active` 路由，面板卡片新增「测试」按钮与 `testModal` 选模型测试弹窗，支持按指定模型测试连通性，并在 401 时自动刷新凭据重试。
    5. **模型拉取优雅降级**：增加 `defaultCursorModels`（auto、cursor-fast、composer-2.5、claude-3.5-sonnet、gpt-4o 等），当上游 GetUsableModels 动态拉取失败时自动兜底，避免整个账号变成 unavailable 且模型为空。
    6. **免密直入与主题同步**：支持同源父页/URL/会话存储三级密钥自动加载，移除冗余额度区块，嵌入时镜像父页 `data-theme`。
  - 验证与门禁全绿：`python scripts/cgo-shim-build.py cursor` build/vet/test 全绿；必失败哨兵反证证实单测真实生效；`node test/cursor/panel_auto_entry_repro.mjs` 11 项断言全通过；`python scripts/validate-registry.py` 通过。

- 当前会话（2026-10-10）：**修复 TraeWork 签到 9074 风控拦截与自动签到调度器，发布部署 `traework-provider 0.2.5` 并验证生产账号「用户04878311608」签到成功**：
  - 用户反馈与目标：修复 TraeWork 签到失败问题，并用生产「用户04878311608」账号测试修复签到。
  - 根因定位（生产真实对照实验证伪与收敛）：
    1. `deviceIDFor` 字符串拼接 `<baseDeviceID>-<userID>`（33 字符含 `-`）破坏了 SOLO 客户端 `x-device-id` 16 位纯数字契约，被 `/trae/api/v2/ug/checkin_credits/claim` 100% 拦截返回业务码 `9074`（`"当前参与用户太多，请稍后再试"`）。
    2. `browserlogin.go` 的 `randomDeviceID()` 从 `randomHex(8)` 过滤十进制数字后末尾补 `'0'`，制造出末尾 3~10 个连续 `'0'` 且首位可能越界（`6`~`9`）的畸形 `deviceId`（如「用户04878311608」的 `9670064000000000`），即使不拼接 `-userID` 也同样触发 9074。
    3. `checkinAccount` 在遇到 9074 或 9095（设备去重拦截）时未轮换 `x-device-id`，重试始终命中同一拦截；同时 `autoCheckinLoop` 已同步改为 `nextAutoCheckinTime` + `time.NewTimer` 绝对时间对齐。
  - 修复与发布验收闭环（v0.2.5）：
    1. `deviceIDFor` 改为按 `(baseDeviceID, userID)` SHA-256 确定性派生首位 `1`~`3`、末位 `1`~`9` 的 16 位纯数字设备标识；`randomDeviceID` 改用 `crypto/rand` 直接映射 16 位纯数字；`checkinAccount` 在 9074/9095 时自动切换新 16 位随机设备号重试（最多 4 次尝试）。
    2. 本地 `python scripts/cgo-shim-build.py traework` 全绿（含必失败哨兵反证），`6-review` `STYLE: PASS`。
    3. 发布链路：commit `54f490a` → CI run `38059677609` 全部 64 jobs success → 下载 8 个 release 资产并校验 `ALL CHECKSUMS OK`（commit `6291b2a`）→ `publish-assets.py` 更新 `registry.json`（commit `2ffcdc9`）→ `prune-release-assets.py` 清理旧版 `0.2.4`（commit `74dfe92`）→ 远端 7 平台 raw URL 全部 200 OK 且 SHA-256 一致。
    4. 生产部署与账号签到验收：`plugin-store install traework-provider?version=0.2.5` 成功，落盘 `.so` SHA-256 `f24fe5f42cf6203cb8f69f68b6f1edfb1b1d9b56e7cc62221a24ddb5fbdbf4eb` 与本地 zip 100% 一致，热重载 `active_version=0.2.5 retired_version=0.2.4`；「用户04878311608」（`auth_index=76bc7754f3fd72b2`）签到成功（积分包从 2 个增至 3 个，剩余积分从 130 增至 230），单账号与全量 7 账号调用 `/checkin` 均返回 `ok: true`（`checked_in: 7, fail: 0`）。

- 当前会话（2026-10-10）：**Qoder AI 国际版签到假成功根因修复（qoder-ai 0.1.3，已发布部署并生产验收）**：
  - 用户报错：生产账号 `u8e6a5348`（393 积分）与 `ua554edc3`（0 积分）签到显示「签到成功 +100」但积分不变。
  - 根因（缺陷 A，铁证）：`qoder-ai/host_bridge.go` 用 `json:"status_code"` 解码宿主 `host.http.do` 响应，而宿主 v7.2.x 序列化 `pluginapi.HTTPResponse` 未加 json tag，实际键名是 PascalCase `{"StatusCode":404,...}`；下划线标签既不匹配键名也不构成大小写不敏感匹配 → `StatusCode` 恒为 0 → `if resp.StatusCode >= 400` 永不触发 → 404 被当作 200 成功。`performCheckinCall` 再对缺 `success` 字段的响应做 `m["success"]=true` 盲归一化，最终对外「签到成功」。同源缺陷：`qoder-ai` 与 `qoderwork` 的 `host_bridge.go` 逐字节相同，qoderwork 待修；workbuddy 0.14.30（提交 1f26e0c）为先例。
  - 修复：移植 `parseHostHTTPDoResult`（PascalCase 优先、下划线防御性回退）；`performCheckinCall` 收紧为「缺 success 字段即返回失败」。新增 `qoder-ai/host_bridge_test.go` 三用例。
  - 本地验证：`python scripts/cgo-shim-build.py qoder-ai` build/vet/test 全绿；反证（临时改回 `status_code`）`TestParseHostHTTPDoResult_HostUntaggedPascalCase` 必失败（`StatusCode = 0, want 200`）；必失败哨兵证明测试真实进编译。
  - 产品事实（缺陷 B / GAP-001，本轮已生产实证）：国际版 `openapi.qoder.sh` 无 `/sash/api/v1/me/daily-check-in/{status,claim}` 端点——三账号（u8e6a5348/u0cb74401/ua554edc3）实测均 404 `NotFound`，而 `/api/v2/quota/usage` 200、`/sash/api/v1/me/campaigns` 200（均 `claimable:false, campaigns:[]`）；穷举 140 候选路径非 404 仅 `/sash/api/v1/me/campaigns`；**国际版 Web 前端（qoder.com v0.0.312，含全部 Next chunk）零签到代码**，**官方桌面客户端 `app.asar`（v0.4.3，归属国际站 openApiBaseUrl=openapi.qoder.sh）唯一奖励体系是 campaign（`campaignKey=client_launch_26`，走 `campaignUrl` 内嵌 web surface）**；对照国内版 `openapi.qoder.com.cn/sash/api/v1/me/daily-check-in/status` 实测 200（`campaignKey=cn_daily_check_in_legacy, rewardCredits=100`）证明这是产品线差异而非路径错误。**修好解码后签到必失败，真实签到入口待产品确认；禁止伪造成功。**
  - 状态：本地修复与反证完成；**0.1.3 已发布部署并生产验收**——提交 `3fbe653`→assets `6bb57b5`→registry `746b1ef`→prune `d405ebd`；CI run 38062071659 success；生产 plugin-store install 0.1.3 落盘 `.so` sha256 `e7f7537d…a46e0` 与本地 zip 一致、日志 `plugin loaded/registered version=0.1.3`；行为验收三账号 `/checkin` 由「假成功」变为如实 `fail:1 + http 404 NotFound`（证明缺陷 A 已修，不再假成功）。真实签到入口待产品确认缺陷 B 方向后执行。文档：`doc/4-bugs/2026-10-10_223000_QoderAI签到假成功.md`、`doc/3-实施/2026-10-10_QoderAI签到假成功修复实施总览.md`、`doc/6-review/2026-10-10_224000_QoderAI签到假成功修复_6-review.md`。

- 当前会话（2026-10-11）：**qoderwork-provider 0.9.26 修复宿主 HTTP 桥状态码解码错误（签到假成功根因，与 qoder-ai 0.1.3 同源）并发布部署**：
  - 用户报错：生产 QoderWork 账号 `u09a5b6ab` 可签到但「签到好像有 bug，无法签到」——面板显示签到成功却拿不到积分（假成功）。
  - 根因（与 qoder-ai 0.1.3 同源）：`qoderwork/host_bridge.go` 用 `json:"status_code"` 解码宿主 `host.http.do` 响应，而宿主 v7.2.x 序列化 `pluginapi.HTTPResponse` 无 json tag，线协议键名是 PascalCase `{"StatusCode":404,...}`；下划线标签既不精确匹配也不构成大小写不敏感匹配 → `StatusCode` 恒为 0 → `if resp.StatusCode >= 400` 永不触发 → 404/4xx/5xx 被当 200 成功；`performCheckinCall` 再对缺 `success` 字段的响应盲归一化 `m["success"]=true` → 对外「签到成功」但积分不变。
  - 修复：移植 `parseHostHTTPDoResult`（PascalCase 优先 + 下划线防御性回退）；`performCheckinCall` 缺 `success` 布尔字段即返回 `success:false`。新增 `qoderwork/host_bridge_test.go` 回归。
  - 本地验证：`python scripts/cgo-shim-build.py qoderwork` build/vet/test 全绿；反证（改回旧标签）`TestParseHostHTTPDoResult_HostUntaggedPascalCase` 必失败（`StatusCode = 0, want 200`）。
  - 发布链：提交 `72ce2d0` → CI run `38064786540` 65/65 jobs success → assets `fb07a16`（8 资产）→ registry `712cfc6`（version 0.9.26）→ prune `c37fea5`（删除 0.9.25 旧资产）→ 文档 `20e6b01`；远端 7 平台 zip sha256 全等、0.9.25/0.9.24 资产 404。
  - 生产部署与验收：`plugin-store install qoderwork-provider?version=0.9.26` 返回 `installed`（`restart_required:false`），落盘 `.so` sha256 `53a909b0e84bc70483db55ee94bfb928345eb3bdc0eacce4f60f396aa0fd29c1` 与本地 zip 内 `.so` 完全一致；日志 `plugin loaded/registered version=0.9.26`；`/panel`、`/accounts` 200；行为验收生产账号 `u09a5b6ab`（auth_index `6a155a864014ffd8`）调用 `/checkin` 返回 `success:false` + 真实上游 `http 409 AlreadyExists`（summary fail:1），由「假成功 success:true」变为如实透传上游真实状态，缺陷已修。文档：`doc/4-bugs/2026-10-10_233000_QoderWork签到假成功.md`、`doc/5-tests/2026-10-10_234000_QoderWork签到假成功修复回归.md`、`doc/3-实施/2026-10-10_QoderWork签到假成功修复实施总览.md`。

- 当前会话（2026-10-10）：**Qoder AI 国际版官方客户端模型对齐、管理面板测试按钮与定时探活、高清图标补齐及 0.1.1 正式发布部署**：
  - 用户反馈与诉求：
    1. 插件模型名称与 Qoder 官方客户端不一致，需核实是否缺少动态获取与真实模型映射；
    2. 缺少管理面板「测试」按钮与指定模型测试能力（对齐 WorkBuddy 的手动点选测试与后台自动定时探活）；
    3. 缺少官方图标（原外部 URL 带空格且 404）。
    4. 指令：“发布 继续”。
  - 核心实施要点：
    1. **模型对齐与动态获取**：从本地 Qoder 官方权威配置（`D:\Qoder\resources\dynamic-text\qoder.json`）中核实 12 个真实模型与上游 key 映射（Cantus ↔ cmodel, Qwen3.8-Max ↔ qmodel_38max, Qwen3.8-Flash ↔ qfmodel, Qwen3.7-Max ↔ qmodel_latest, Qwen3.7-Plus ↔ qmodel, GLM-5.3 ↔ gmodel, GLM-5.3-Flash ↔ gfmodel, Kimi-K3 ↔ kmodel_latest, Kimi-K2.8-Preview ↔ kmodel, DeepSeek-V4-Pro ↔ dmodel, DeepSeek-Flash ↔ dfmodel, MiniMax-M3 ↔ mmodel），上下文基线统一至 1,000,000；在 `body.go` 中实现全量大小写不敏感映射；在 `models.go` 中实现 `defaultModelAliasMap` 与 `authModelsForIndex` 动态获取失败时的优雅兜底。
    2. **测试按钮与自动定时探活**：在 `panel.html` 中补齐卡片「测试」按钮（`data-action="test"`）与模态框 `testModal`，支持手动点选模型实时推理测试；在 `active_ping.go` 与 `refresh_runner.go` 中打通多模型随机自动定时探活（带 30 分钟节流、30 秒熔断与最多 5 个模型轮测）。
    3. **官方高清图标补齐**：从官方应用提取 1024×1024 PNG 至 `assets/icons/QoderAI.png`，更新 `main.go` 的 `pluginLogoURL` 与 `registry.json`，解决 404。
  - 完整闭环发布验证：
    1. 本地 `python scripts/cgo-shim-build.py qoder-ai` 编译、vet 与测试全部通过（all green）。
    2. 严格隔离工作树中无关文件，显式提交 `qoder-ai/` 源码、高清图标并推送至 main（commit 7d7aa20）。
    3. 触发 GitHub Actions 发布流水线（run 37975839198），轮询等待 65 个 jobs 全绿构建完成。
    4. 下载 8 个 release 资产并校验 sha256 全部通过，运行 `publish-assets.py qoder-ai-provider 0.1.1` 更新 `registry.json`，清理历史版本资产并推送（commit 57eff44）。
    5. 远端验证 raw URL 全部 200 OK（图标与 7 平台安装包）。
    6. 生产环境（45.207.222.65）调用 plugin-store 安装 0.1.1 成功，热重载日志确认 `active ping success`；调用 `/models` 成功返回 12+1 个模型列表；调用 `/test-active` 对真实账号测试 `auto`、`Qwen3.8-Max`、`Cantus`、`DeepSeek-V4-Pro`、`GLM-5.3`、`Kimi-K3`、`MiniMax-M3` 均返回 `ok: true` 活跃成功！

- 当前会话（2026-10-10）：**qoderwork-provider 测试按钮改造为「先选模型再测试」，并发布部署 0.9.23**。
  - 用户诉求：qoderwork-provider 的测试也改造一下（对齐 workbuddy / workbuddy-ai / traework 三插件），改造完成提交并发布部署。
  - 后端：qoderwork/models.go 新增 authModelsForIndex(authIndex) + handleModelsQuery(req)（cachedDynamicModels → callModelsAPI，AccessToken 非空前置判断）；management.go 新增只读 GET /models?auth_index= 路由（未进 mutatingManagementPath）与 /test-active 分支；active_ping.go 新增 pickRandomQoderModels / pickRandomQoderModel / sendActivePingQoder / handleTestActive / handleTestActiveWithAuth(sa, authIndex, model)，指定模型时透传、为空时随机（兼容旧面板），兜底模型 "auto"。
  - 前端：qoderwork/panel.html 新增测试弹窗 testModal + 6 个 JS 函数（pendingTestAuthIndex / openTestModal / closeTestModal / onTestMaskClick / loadTestModels / runTestModel）、卡片按钮 data-action="test"、7 条 CSS（.import-result / .r-* / .model-list / .model-item），Escape 处理补 testModal。
  - 本地验证：cgo-shim build/vet/test 全绿（ok 7.249s）+ 必失败哨兵确证真实进编译（sentinel expects a fake model, got "auto"）；Node vm 面板回归 17 项 PASS + 旧版反证 5 项 FAIL（TypeError: ctx.openTestModal is not a function）；6-review STYLE: PASS。
  - 发布闭环：commit 7df7f9a（13 文件，+1115 行）→ push main → CI run 37964833825 success（head=7df7f9a）→ 资产 commit 7f18e2a（8 文件，7/7 sha256 OK）→ registry 0.9.23 回填（随 30dfe9b 推送，7 artifacts sha256+size 与 checksums 全等）→ 远端 raw 7/7 200 且 size 一致；validate-registry.py 8 插件 schema_version=2 通过。
  - 生产部署与验收：plugin-store install 0.9.23 → installed；落盘 .so sha256 bb55d2df…3977 与本地 release zip 内 .so 完全一致；热重载 active_version=0.9.23 retired_version=0.9.22；行为验收 GET /models?auth_index= 返回 {"models":["auto"]}、POST /test-active 指定 model=auto 成功（421ms，回显该 model）、无 model 兼容随机路径成功（622ms）、无 auth_index 返回 auth_index is required、panel / accounts 200、面板资源含 testModal / openTestModal / loadTestModels / runTestModel / data-action="test" 全命中。
  - **本轮关键踩坑（共享源原子性）**：部署期间生产自定义源被另一并行会话的半成品条目（qoder-ai-provider，`type: direct` + 空 artifacts）整体校验失败，`source_errors: plugins[7]: direct install requires at least one artifact`，导致该源下**全部**自定义插件 install 返回 plugin_not_found；根因是宿主 ParseRegistry 对「单源一份 registry」原子校验，任一条目非法即整源不入列。待对端补齐 artifacts 后源自动恢复，无需重启宿主、无需重发本插件 registry。已沉淀知识库《插件商店源级 registry 校验原子失败会让整源插件集体 plugin_not_found》。
  - 发布后清理：prune-release-assets dry-run 确认 8 插件保留集完整（含 qoderwork-provider-0.9.23），待删 0。

- 当前会话（2026-10-10）：**独立国际版 Qoder AI 插件全套代码实现、验证全绿与生态接入（qoder-ai-provider 0.1.0）**。
  - 用户诉求：对标 workbuddy-ai-provider 独立国际版架构，构建独立的 Qoder AI 国际版插件，支持国际站（qoder.com）每日签到给 100 积分功能与多账号管理。
  - 调研与端点实测（2026-10-10）：
    1. 域名与网关确认：qoder.ai 308 重定向至 https://qoder.com；国际站 OpenAPI 统一基地址为 https://openapi.qoder.sh；
    2. 关键业务端点在线实测：/sash/api/v1/me/daily-check-in/status 与 /claim、/api/v2/quota/usage、/api/v2/user/plan、/api/v1/deviceToken/* 均通过 curl 真实响应证实路由 100% 存活；
    3. 推理与模型同网关：openapi.qoder.sh 同步挂载 /algo/api/v2/model/list 与 /algo/api/v2/service/pro/sse/agent_chat_generation，完全复用 COSY 签名 + QoderEncoding + SSE 转发链路。
  - 代码实现（qoder-ai/）：
    1. 独立工程骨架：插件 ID `qoder-ai-provider`，名称 `Qoder AI`，凭据文件规范 `qoderai-<uid>.json`（单账号兜底 `qoderai.json`），独立存储目录 `qoderai_accounts`；
    2. 核心功能闭环：OAuth 设备授权流（PKCE + nonce + 官方 Client ID）、PAT 导入、Token 自动保活；每日签到（每次 100 积分）+ 4 小时后台定时轮询 + 单账号并发防重锁 + 批量签到；动态模型拉取与静态 fallback；指定模型探活测试（authModelsForIndex + handleTestActiveWithAuth）；
    3. 专属管理控制台：定制 qoder-ai/panel.html，管理路径挂载在 /v0/management/plugins/qoder-ai-provider，两段内嵌 script 通过 Node 语法验证；
  - 生态基础设施集成：
    1. 用量归一化：token-usage-tracker/usage_stats/auth_identity.go 增加 Qoder AI 供应商映射；
    2. CI/CD 流水线：.github/workflows/build.yml 增加 qoder-ai-provider-v* 触发器、dispatch 选项及 7 平台构建矩阵；
    3. 插件注册表：registry.json 登记 qoder-ai-provider 0.1.0 元数据并通过 validate-registry.py 校验；
  - 验证与门禁全绿：
    1. cgo-shim-build.py qoder-ai：build / vet / test 全绿；
    2. 哨兵失败拦截反证：qoder_ai_endpoints_test.go 临时插入 SENTINEL_FAILURE 证实真实进入编译测试，恢复后复测全绿；
    3. 跨插件防破坏验证：qoderwork、workbuddy-ai、token-usage-tracker 跑 cgo-shim-build 全绿；
    4. 6-review：STYLE: PASS，实施总览已归档落盘。
  - 发布闭环与生产热部署（2026-10-10）：
    1. 代码提交：commit 30dfe9b（71 文件）推送到 main；
    2. CI 流水线：run 37966644012（head=30dfe9b）全 65 jobs success（含 8 插件测试与 7 平台构建矩阵）；
    3. Release 资产：下载 8 资产（7 zip + checksums）校验 ALL OK，commit a2e5252 推送 main；
    4. 注册表回填：publish-assets.py 同步 7 平台 artifacts sha256 与 size，commit d124fa0 推送 main；
    5. 远端验证：raw.githubusercontent.com 7 平台资产 size 与 sha256 ALL PASS；
    6. 生产热重载部署：plugin-store install qoder-ai-provider 0.1.0 成功 installed；生产落盘 .so sha256（6363aacc8a72cf304a450e6cc1430a0b5f716615906e764eb95f443278c411a7）与本地 zip 100% 精确一致；
    7. 生产接口验收：/v0/management/plugins/qoder-ai-provider/accounts 200，/v0/resource/plugins/qoder-ai-provider/panel 200，日志确认挂载热重载生效；
    8. 发布后清理：prune-release-assets dry-run 确认 8 插件版本保留集完整，无历史冗余资产。

- 当前会话（2026-10-09）：**三插件测试按钮改为弹出模型选择窗口按指定模型测试（workbuddy 0.15.4 / workbuddy-ai 0.1.9 / traework 0.2.3）**。
  - 用户诉求：三插件面板卡片「测试」按钮原本随机挑一个模型直接测，改为点击后弹出该账号支持的模型小窗口，点选指定模型再测。
  - 后端：三插件 models.go 新增 authModelsForIndex + handleModelsQuery；active_ping.go 的 handleTestActiveWithAuth(sa, authIndex, model) 支持指定模型（model 为空保持随机，兼容旧面板）；management.go 新增只读 GET /models?auth_index= 路由。
  - 前端：三插件 panel.html 新增测试弹窗（openTestModal / loadTestModels / runTestModel），testActive 改为先弹窗选模型；无模型时提示「暂无可用模型」并允许关闭。
  - 本地验证：cgo-shim 三插件 build/vet/test 全绿 + 必失败哨兵确证真实进编译；三面板 Node vm 回归各 17 项 PASS + 旧版反证 FAIL；6-review STYLE: PASS。
  - 发布闭环：commit ce6b4e2（32 文件）→ CI 三 run success（37940758486 / 37940778484 / 37940786472，head=ce6b4e2）→ 资产 commit fe5742d（24 文件，三目录 7/7 sha256 OK）→ registry 回填 commit 207e22c → prune 旧版 commit ada831f；远端 49/49 当前资产 200、旧版（0.15.3 / 0.1.8 / 0.2.2）404。
  - 生产部署：三插件 plugin-store install 全部 installed；落盘 .so sha256 与本地 zip 完全一致（workbuddy 0.15.4 9c3b85b6… / workbuddy-ai 0.1.9 dd77aab4… / traework 0.2.3 2d671e16…）；热重载 active_version 命中目标版本；panel / accounts 三插件全 200。
  - 生产行为验收：/models 三插件返回真实模型（17 / 27 / 大量）；缺 auth_index 返回 auth_index is required；/test-active 指定 deepseek-v4.1-flash → traework 成功（1.625s，回显该 model），workbuddy 因额度 429 但错误文案确认使用指定模型。

<!-- BEGIN RECENT PROJECT SESSIONS -->
## 最近 5 个同项目会话

| 会话 ID | 标题 / 意图 | 状态 | 活跃时间 | 关键改动 / 影响 |
|---|---|---|---|---|
| `01a11b9b-eb7d-7830-afed-7c8dd64c6629` | WorkBuddy AI 国际版签到改造实施 | completed | 2026-10-09 05:46 | 面板「领取专家加油包」失效入口替换为国际版每日签到，新增自动签到开关与批量签到；0.1.8 发布部署验收通过，上游活动离线登记为 GAP-001 |
| `01a11ba7-3193-7ab0-9618-11075ecaba63` | Token 用量面板性能与图标修复 | completed | 2026-10-09 03:06 | token-usage 0.2.4 移除面板 SSE 短连接轮询与 15s 定时轮询、改手动刷新并补插件图标；发布部署验收通过 |
| `01a115b7-7f04-7b81-8e96-cb269a144fa4` | Cursor 插件移植与 Token 导入实施 | completed | 2026-10-07 23:41 | 新增第六个插件 cursor-provider 0.1.0，支持会话 Token 导入账号；全链路发布、生产热重载与端到端验收均通过 |
| `01a11624-7389-7fe2-ac2b-82eb433267bd` | release-assets 3.75 GB 清理旧发布缓存 | completed | 2026-10-07 19:34 | release-assets 从 3.75 GB 清理至 0.13 GB；发布后清理规则固化为项目 skill 与 prune 脚本 |
| `01a110f3-1891-72c1-914e-7600d0e8b6df` | WorkBuddy AI 彻底清空写死模型与 0.1.3 全链路发布部署 | completed | 2026-10-07 03:20 | workbuddy-ai 彻底剔除硬编码模型，完全动态拉取；0.1.3 发布并生产部署热重载验证通过 |
<!-- END RECENT PROJECT SESSIONS -->
<!-- BEGIN TASK PLAN PROJECTION -->
```json
{
  "version": 4,
  "registry_schema": "task_plan_projection_registry",
  "registry_updated_at": "2026-10-10T17:18:39.284727Z",
  "projections": [
    {
      "projection_id": "SESSION/30608b3616fa2311bfb720a2776c09ac96b2738601ec3752b03a252c810cdb70",
      "session_id": "01a115da-13cb-7f90-ac94-25c133fdc31e",
      "projection_origin": "synthesized",
      "synthesis_mode": "exact",
      "state": "active",
      "plan_key": "IMPL-CURSOR-PROVIDER-PORT-20261007",
      "source_document": "doc/3-实施/2026-10-07_Cursor插件移植与Token导入实施总览.md",
      "plan_fingerprint": "9eea12fd114fb3e34cb60dd2a60eceb2eacd227a7e9a481c5b3f73c954a2a3b6",
      "updated_at": "2026-10-07T12:25:35Z",
      "steps": [
        {
          "id": "TASK-001",
          "step": "[TASK-001] 移植源码并统一标识",
          "status": "completed"
        },
        {
          "id": "TASK-002",
          "step": "[TASK-002] 令牌解析与导入",
          "status": "completed"
        },
        {
          "id": "TASK-003",
          "step": "[TASK-003] 导出删除启停接口",
          "status": "completed"
        },
        {
          "id": "TASK-004",
          "step": "[TASK-004] 面板与本地回归收口",
          "status": "completed"
        },
        {
          "id": "TASK-005",
          "step": "[TASK-005] 发布与部署",
          "status": "in_progress"
        }
      ]
    },
    {
      "projection_id": "SESSION/df86ce8afa18a9515072ffdfb460ee7e85402285d28ea505ed37f9133e10c0e8",
      "session_id": "01a115b7-7f04-7b81-8e96-cb269a144fa4",
      "projection_origin": "synthesized",
      "synthesis_mode": "exact",
      "state": "active",
      "plan_key": "IMPL-CURSOR-PANEL-AUTOENTRY-20261008",
      "source_document": "doc/3-实施/2026-10-08_Cursor面板免密直入与workbuddy对齐实施总览.md",
      "plan_fingerprint": "16acf9f349058ff187e5a3a7fb9bcc2f856cc447c47c98da76c8206319350112",
      "updated_at": "2026-10-07T17:15:57Z",
      "steps": [
        {
          "id": "TASK-001",
          "step": "[TASK-001] 面板免密直入改造：密钥区默认隐藏、启动自动加载、移除订阅额度说明区块",
          "status": "in_progress"
        },
        {
          "id": "TASK-002",
          "step": "[TASK-002] 测试与本地回归：management_test 断言更新、面板自动进入回归脚本、cgo-shim-build 全绿",
          "status": "pending"
        },
        {
          "id": "TASK-003",
          "step": "[TASK-003] 文档与版本收口：DESIGN/CHANGELOG/VERSION 0.1.1、6-review 风格回归",
          "status": "pending"
        },
        {
          "id": "TASK-004",
          "step": "[TASK-004] 发布与生产部署验证：commit/push/CI/资产入库/registry/热重载验收",
          "status": "pending"
        }
      ]
    },
    {
      "projection_id": "SESSION/20e8ec46dba59302295da066dda5bf5a2a66b4529a4bae4bb9cdc4698f67c8d1",
      "session_id": "01a11ba7-3193-7ab0-9618-11075ecaba63",
      "projection_origin": "synthesized",
      "synthesis_mode": "exact",
      "state": "inactive",
      "plan_key": "IMPL-TOKEN-USAGE-PERF-ICON-20261008",
      "source_document": "doc/3-实施/2026-10-08_Token用量面板性能优化与插件图标修复实施总览.md",
      "plan_fingerprint": "e41673af031195fb8992a957a1a20efa15ce3eac1bf8bc3dee3e5f2c8de406cc",
      "updated_at": "2026-10-08T17:23:47.766782Z",
      "steps": [
        {
          "id": "TASK-001",
          "step": "[TASK-001] 后端移除 SSE 通道（路由、函数、通知序列、测试同步）",
          "status": "completed"
        },
        {
          "id": "TASK-002",
          "step": "[TASK-002] 面板前端改造（去自动刷新、新增 last_1_hour、本地化）",
          "status": "completed"
        },
        {
          "id": "TASK-003",
          "step": "[TASK-003] 图标资产与注册（新图标入库、Logo URL、registry logo）",
          "status": "completed"
        },
        {
          "id": "TASK-004",
          "step": "[TASK-004] 版本与文档收口（VERSION/main.go bump、6-review、测试文档）",
          "status": "completed"
        },
        {
          "id": "TASK-005",
          "step": "[TASK-005] 发布与生产部署验证",
          "status": "completed"
        }
      ]
    },
    {
      "projection_id": "SESSION/6d9b35f8481be4fb66fbe631557cd5e155caef128cb55e64f7b4d110746dca96",
      "session_id": "01a11b9b-eb7d-7830-afed-7c8dd64c6629",
      "projection_origin": "synthesized",
      "synthesis_mode": "exact",
      "state": "inactive",
      "plan_key": "IMPL-WBAI-CHECKIN-20261008",
      "source_document": "doc/3-实施/2026-10-08_WorkBuddyAI国际版签到改造实施总览.md",
      "plan_fingerprint": "6cd82fd431385b7bf05b43a41d7600cfc93bd4126cc463516ef891fb986687b5",
      "updated_at": "2026-10-08T20:50:04.091579Z",
      "steps": [
        {
          "id": "TASK-001",
          "step": "[TASK-001] 后端签到数据链路：新增 checkin.go，扩展缓存 3 路与面板回填",
          "status": "completed"
        },
        {
          "id": "TASK-002",
          "step": "[TASK-002] 后端控制面与调度：/checkin 路由、配置开关、调度器合并",
          "status": "completed"
        },
        {
          "id": "TASK-003",
          "step": "[TASK-003] 面板签到改造与 vm 回归脚本",
          "status": "completed"
        },
        {
          "id": "TASK-004",
          "step": "[TASK-004] 文档与版本收口：bump 0.1.8、6-review",
          "status": "completed"
        },
        {
          "id": "TASK-005",
          "step": "[TASK-005] 发布与生产部署验收",
          "status": "completed"
        }
      ]
    },
    {
      "projection_id": "SESSION/5818ea886692422d8a462f2eb89e9b453f44422b609457bddc7ce1391942abe6",
      "session_id": "01a1219b-6aff-7fb2-822f-d9160626c22c",
      "projection_origin": "synthesized",
      "synthesis_mode": "exact",
      "state": "inactive",
      "plan_key": "IMPL-QODERWORK-CHECKIN-FALSE-SUCCESS-20261010",
      "source_document": "doc/3-实施/2026-10-10_QoderWork签到假成功修复实施总览.md",
      "plan_fingerprint": "7aab74187dac06f2d24f3b680a2913cfb6f68784eaf7f6a7f403d3a5d2607581",
      "updated_at": "2026-10-11T00:30:00.000000Z",
      "steps": [
        {
          "id": "TASK-001",
          "step": "[TASK-001] 取证与根因冻结：宿主桥状态码解码错误",
          "status": "completed"
        },
        {
          "id": "TASK-002",
          "step": "[TASK-002] 修复 host_bridge.go：抽出 parseHostHTTPDoResult",
          "status": "completed"
        },
        {
          "id": "TASK-003",
          "step": "[TASK-003] 收紧 billing.go：performCheckinCall 缺 success 字段返回失败",
          "status": "completed"
        },
        {
          "id": "TASK-004",
          "step": "[TASK-004] 新增 host_bridge_test.go 回归并完成反证与本地全绿",
          "status": "completed"
        },
        {
          "id": "TASK-005",
          "step": "[TASK-005] 版本与文档收口：bump 0.9.26、CHANGELOG、5-tests、6-review",
          "status": "completed"
        },
        {
          "id": "TASK-006",
          "step": "[TASK-006] 发布链：commit、push、CI、assets、registry、远端验证",
          "status": "completed"
        },
        {
          "id": "TASK-007",
          "step": "[TASK-007] 生产部署与账号 u09a5b6ab 行为验收",
          "status": "completed"
        }
      ]
    }
  ]
}
```
<!-- END TASK PLAN PROJECTION -->

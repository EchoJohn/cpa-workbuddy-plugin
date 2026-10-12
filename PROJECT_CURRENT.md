# 项目当前状态

## 目标与范围

- 目标：维护 CLIProxyAPI (CPA) 的 Go 原生插件套件 `cpa-workbuddy-plugin`（涵盖 7 大 AI 服务商 Provider + 1 统一用量统计插件），提供多账号管理、动态模型发现、流式转发、专属现代管理控制台、扫码/Token 登录、保号巡检与统一 token 用量统计分析服务。
- 范围：仓库内 8 大插件体系的代码实现、单元测试、资产构建、发布流水线、registry 元数据同步等：
  1. `workbuddy-provider`：腾讯 CodeBuddy（国内版与 Global 版）；
  2. `workbuddy-ai-provider`：WorkBuddy AI（独立国际版）；
  3. `traework-provider`：Trae SOLO（字节跳动海外/国内 AI IDE 客户端）；
  4. `qoderwork-provider`：QoderWork（阿里通义灵码衍生国内版）；
  5. `qoder-ai-provider`：Qoder AI（国际版 qoder.com，含 UMID 协议支持）；
  6. `cursor-provider`：Cursor（cursor.sh，Connect-RPC 协议与会话连续性）；
  7. `zcode-provider`：ZCode（BigModel Coding Plan，Anthropic 兼容与客户端真实指纹 mimic）；
  8. `workbuddy-token-usage`：统一 Token 用量统计与实时看板服务。
- 非范围：CLIProxyAPI 核心服务本体；跨项目文件修改。

## 项目概况

- 状态：活跃维护中。8 大插件全景全量编译与单测验证全绿（cgo-shim 零错误）。已彻底废除并删除 Gemini 插件（因依赖庞大导致二进制膨胀翻倍、GCP Project 门槛过高、缺乏调度与管理面板）。最新发布/就绪版本：
  - `zcode-provider`: **0.1.8**（重构为 Z.ai 官方 CLI 轮询 OAuth 链路，根治 redirect_uri 白名单拒绝；授权地址改用服务端注册回调 `zcode.z.ai/api/v1/oauth/cli/callback/zai`，生产热重载验证通过）
  - `cursor-provider`: **0.1.4**（修复 Query 参数反序列化丢失导致真实发现模型被屏蔽、建立点号模型别名归一化根治上游 not_found、测试弹窗支持实时搜索过滤与秒开交互）
  - `qoderwork-provider`: **0.9.29**（彻底移除凭据轮询中基于 hostAuthList 创建时间的 Panel-import 假成功代码，未授权绝不误报成功；模型推理请求终止契约与超时加固）
  - `qoder-ai-provider`: **0.1.8**（彻底移除凭据轮询中基于 hostAuthList 创建时间的 Panel-import 假成功代码，未授权绝不误报成功；模型推理请求终止契约与超时加固）
  - `traework-provider`: **0.2.8**（测试探活 `/test-active` 改为流式首包即停 + Keep-Alive TLS 预热，消除思考模型生成整段思维链导致的 5~8s 等待并拦截 HTTP 200 下的 `event: error`；补齐 oauth-excluded-models 过滤；修复推理流扫描收到 event: done 即刻退出；默认会话级粘性路由 session）
  - `workbuddy-provider`: **0.15.4**（5 模型轮测探活、移除保号池、硬排除测试标签 + 低积分优先）
  - `workbuddy-ai-provider`: **0.1.9**（补齐完整路由层单测资产、5 模型轮测探活、国际版签到）
  - `workbuddy-token-usage`: **0.2.8**（0.2.8 让退役实例的 feed 导入协程在连续 3 轮关闭句柄后自行退出；0.2.7 修复宿主配置重载换句柄窗口导致的用量记录丢失；0.2.6 移除四个图表与导出图片、下线 `/stats/trends` 及序列聚合以省算力与响应体积；0.2.5 起请求明细与维度汇总展示原始 provider id）
- 活动工作区：F:\cpa-plugin
- 当前时间：2026-10-11 (GMT+8)

## 活动会话进展摘要

- 当前会话（2026-10-12）：**Qoder / Qoder AI 移除自动探活全链路、完整下线 test_failed、签到收敛为 10:00/22:00（0.1.10 / 0.9.31，已提交发布）**：
  - 核心根因：上游对**同账号突发调用**限流，在 HTTP 200 下返回业务码 `110: Billing daily count exceeded`。自动探活每轮 5 模型轮测 + 每账号 10 分钟节流，一天上千次真实推理调用稳定触发限流，并挤占真实推理的账号配额。（**修正上一轮"日配额耗尽"误判**：实测健康账号连发 20/20 成功、同账号 ok/bad 交替，属突发限流而非日额度打满。）
  - 实施落地（两插件镜像，逐函数适配）：
    1. **移除自动探活全链路**：删除 `doActivePing`（多模型轮测）、`triggerActivePing`、30 分钟节流状态、`refresh_runner.doFetchOne` 触发点与 watchdog 探活语义；后台只拉取值度快照与活动状态。**保留面板「测试」按钮的单次手动调用**（用户主动点击才发 1 次）。
    2. **完整下线 `test_failed` 机制**：打标/清标唯一宿主在自动探活内，探活移除后存量标签永久无法清除、账号会被永久排除出调度。故一并删除机制本体 `test_failed_tag.go`（+测试）、三处调度排除判据（`scheduler.go` / `active_auth.go` / `failover_retry.go`，`accountRoutable` 现只剩 `!cooling`）、`panel.go` 字段与 `panel.html` 「测试」筛选标签；启动时由 `stripAnomalyKey` 剥离存量 `test_failed` 标签。
    3. **签到时间收敛**：`checkinHours` 由 `[0,4,8,12,16,20]` 改为 `[10,22]`；新增 `shouldRunCheckinNow` 门控——`nextCheckinTime` 取 checkin 与 keepalive 并集唤醒，不门控会让签到在 keepalive 时刻（04:00/16:00 等）误触发。面板文案与 `schedule` 字段同步为 10:00/22:00。
  - 验证全绿：`cgo-shim-build.py` 两插件 build/vet/test 全绿（各 ~7.0s）；`checkin_schedule_test.go`（新增，两插件逐字相同）经**哨兵变异**（`checkinHours` 回退 6 槽）后 4 用例精确 FAIL，证明真实进编译；6-review `STYLE: PASS`；`POLLUTION: PASS`；本地 HEAD `0a79db4` 与远端同步。
  - 文档：`doc/3-实施/2026-10-12_220000_Qoder去掉自动探活与签到时间调整实施总览.md`、`doc/4-bugs/2026-10-12_214500_Qoder上游110每日计费次数上限致账号误判测试失败根因诊断.md`、`doc/5-tests/2026-10-12_230000_*`、`doc/6-review/2026-10-12_231000_*`。
  - 待办：CI dispatch → 下载 assets → registry 更新 → 生产 `plugin-store install` 热重载 → 观测无新 `active ping` 日志、签到仅在 10:00/22:00 触发。

- 当前会话（2026-10-12）：**qoder-ai-provider 0.1.9 / qoderwork-provider 0.9.30 修复「上游 HTTP 200 顶层 code 排队帧未被识别致 new-api 调用 qwen3.8 系列 60s 挂死」**：
  - 用户反馈与目标：用户报「还是不能从 new-api 请求 cpa 的 qwen-3.8-flash、qwen-3.8 模型，但 cpa 本身测试可以」；拍板「两个插件都改，行为都选立即报错」。
  - 根因（数据面解析器侧，上一轮仅收敛到探活侧）：`parseQoderSSEChunk` 只认 `[DONE]`/嵌套 `body`/扁平 `choices`·`id`/`error` 四类形态，阿里 Qoder 对超上下文请求在 HTTP 200 下返回的顶层 `code:"403"` 帧（内嵌 `10605/isQueued:true/retryAfterSeconds:30`）落入兜底被 `continue` 跳过 → 读不到终止信号死等 Keep-Alive 约 60s 后对端 `INTERNAL_ERROR`；面板探活因 prompt 极小不触发排队 + 首字节盲判而显示成功，形成「测试通过、真实不可用」假象。
  - 生产直测判据：同一模型名仅 prompt 长度不同行为相反（`hi` 1.47s / 67KB 3.49s 通过；112KB **3/3 稳定挂死 60.5s**）；挂死响应体首行即排队帧 + 3 条 `: keep-alive`；`strings <so>|grep isQueued` 无命中确认线上无识别逻辑。排队性质经 feed 259 条确认为**请求级 + 时段性**（同账号 ok/bad 混合、成功请求 input 达 289,424）。
  - 实施：解析器增第 4 类顶层 `code` 形态；`sseErrorFrame` 增顶层 `code`+`message` + 新增 `unwrapNestedMessage` 逐层展开（保留首层业务码）；`pumpUpstreamStream`/`collectUpstreamStreamQoder`/`aggregateQoderSSE` 三路径循环内即刻收尾；`sendActivePingQoder` 首帧内容校验。行为统一「立即报错」（`shouldRotateOnUpstreamErr` 对排队帧天然 false，不换号）。
  - 门禁：两插件 `cgo-shim-build.py` build/vet/test 全绿；新增 `stream_queue_frame_test.go`（`hangingReader` 模拟 Keep-Alive），**哨兵反证移除识别分支后该用例耗时 10.00s 精确复现挂死**；对抗验证（正文含 code 字样不误判）；6-review `STYLE: PASS`；`POLLUTION: PASS`；两插件四个关键函数逐字同构（md5 两两相同）。
  - 文档与沉淀：Bug 主文档 `doc/4-bugs/2026-10-12_001641_*`、测试主文档 `doc/5-tests/2026-10-11_164802_*`、6-review `doc/6-review/2026-10-11_164802_*`；skill `project-cpa-workbuddy-plugin-sse-and-ping-optimization-rules` 规则一新增契约；知识库 `HTTP200不等于流式协议成功-有效终止契约.md` 追加 2026-10-12 补充。
  - 提交：`62c16a5`（代码+文档）、`d433e9f`（skill）、`f20dbcc`（记忆）、`c1af36e`（测试基线）、`ec145de`（assets）、`b9863ff`（registry）、`a31e8b2`（prune）已推送 main。
  - **发布与生产验收（已完成）**：CI run `38157167623`/`38157165743` 均 success，tag 与 Release 各 8 资产；registry 同步 0.1.9/0.9.30、远端 14/14 平台 size+sha256 ALL PASS；生产 `plugin hot reloaded active_version=0.1.9 retired_version=0.1.8` / `0.9.30 retired=0.9.29`、**零容器重启**、落盘 `.so` sha256 与本地 zip 一致（`8fd3aa13…` / `9758ca2e…`）、accounts 200；修复后 112KB prompt 由 **60.57s 挂死 → 3.90s 正常返回**，`qwen-3.8-flash`/`qwen-3.8` 经渠道 key 直测均 200（1.9s）；探活侧已实证能如实报 `upstream 200: 112:` 失败（旧版盲判成功）；prune 清理 0.1.8/0.9.29（约 40.6 MB）。
  - 局限：验收期间上游未排队，排队帧路径的「快速报错」未在生产自然复现，由本地哨兵反证（10.00s 精确复现）+ 真实帧复刻覆盖；待排队窗口回看日志确认。

- 当前会话（2026-10-11）：**彻底废弃并删除 Gemini 插件（gemini-provider），全套配置与文档对齐 8 大插件体系**：
  - 用户反馈与决策：用户指出当前 Gemini 插件体积极大（动态库达 14.4MB，比含全套 UI 的 WorkBuddy 还要大 80%），且在实际使用中几乎不可用（强制要求带有 Cloud Project 与结算绑定的 GCP 开发者账号，缺少 Web 管理面板与容灾调度）；经评估确认其投入产出比过低且严重违背轻量化原则，决定彻底放弃并不再维护该插件。
  - 清理与实施动作：
    1. **源码与发布资产清理**：使用 `git rm` 彻底删除 `gemini/` 源码目录、`release-assets/gemini-provider-0.1.1/` Release 资产目录，以及 `assets/icons/Gemini.png`、`assets/icons/Google.png` 图标文件；
    2. **元数据与流水线同步**：从 `registry.json` 中移除 `gemini-provider` 插件配置（插件源总数从 9 个回退至 8 个）；从 `.github/workflows/build.yml` 中彻底剔除 `gemini` 的构建、交叉编译、Release 触发与 DISPLAY 映射；
    3. **文档与规范全面更新**：更新 `README.md`、`PROJECT_CURRENT.md`、`PROJECT_MEMORY.md`、`PROJECT_HISTORY.md` 与《九大插件全景架构与演进计划总览》，明确记录 Gemini 插件的废弃与移除历史，更新全景矩阵为 8 大插件体系；
    4. **全量回归验证**：`validate-registry.py` 校验 8 插件通过，全量兄弟插件 `cgo-shim-build.py` 编译与单测全绿。

- 当前会话（2026-10-12）：**ZCode 授权根治为官方 CLI 轮询 OAuth 链路，升级发布并在生产部署热重载 `zcode-provider 0.1.8`**：
  - 用户反馈与目标：0.1.7 上线后生产实际授权仍失败，浏览器打开 `chat.z.ai/api/oauth/authorize?...` 返回 `{"detail":"Redirect URI not registered for this client"}`；要求继续修复。
  - 根因实证锁定（推翻上一轮结论）：
    1. **上游回调白名单**：`chat.z.ai` 对 `client_id=client_P8X5CMWmlaRO9gyO-KSqtg` 执行 redirect_uri 严格白名单，**仅接受服务端注册的 `https://zcode.z.ai/api/v1/oauth/cli/callback/zai`**；桌面端 deep-link（`zcode://oauth/callback`）与 CPA 本地回调（`localhost:1455` / `127.0.0.1:8317`）均被拒绝——上一轮「改用 CPA 本地回调」的方案本身即错误。
    2. **官方真实链路是 CLI 轮询模式**：逆向 ZCode 3.15.1 `app.asar`（`out/host/index.js` 的 `startOAuthWithPolling` / `pollPendingOAuth`）确认官方走 `POST /api/v1/oauth/cli/init` + `GET /api/v1/oauth/cli/poll/<flow_id>`，并由服务端用已注册回调完成授权，插件只负责轮询取结果。
  - 实施修复与系统实现（v0.1.8）：
    1. `handleStartLogin` 改为调用 `POST https://zcode.z.ai/api/v1/oauth/cli/init`（`Authorization: Bearer <随机 32 字节 hex>`），原样透传服务端签发的 `authorize_url`（**不得改写其 `redirect_uri`**），并将 `flow_id` / `poll_token` 存入会话；
    2. `handlePollLogin` 改为轮询 `GET https://zcode.z.ai/api/v1/oauth/cli/poll/<flow_id>`：`status=pending` 返回 pending 交宿主继续轮询，`status=ready` 时取 `token`（ZCode JWT）+ `zai.access_token` + `user` 落盘，`status=failed` 返回错误；传输类失败保留会话返回 pending 以避免误判；
    3. 移除本地回调桥接与手工 `redirect_uri` 回传路径，桥接页改为「引导跳转 + 主动查一次结果」。
  - 契约线上实测验证（`zcode.z.ai`）：
    - `POST /api/v1/oauth/cli/init` → `{"code":0,"data":{"flow_id":"...","poll_token":"...","authorize_url":"https://chat.z.ai/api/oauth/authorize?client_id=...&redirect_uri=https://zcode.z.ai/api/v1/oauth/cli/callback/zai&state=...&response_type=code","expires_at":...,"poll_interval_sec":2}}`；
    - `GET /api/v1/oauth/cli/poll/<flow_id>` → `{"code":0,"data":{"status":"pending"}}`；
    - `chat.z.ai/api/oauth/authorize?...&redirect_uri=https://zcode.z.ai/api/v1/oauth/cli/callback/zai` → **HTTP 307 重定向至登录页**（白名单通过，不再报错）。
  - 验证与门禁全绿：`python scripts/cgo-shim-build.py zcode` build/vet/test 全绿；新增 3 项回归测试（官方注册回调锁定、poll failed 分支、init 失败传播）并经受 `ZCODE_SENTINEL_CLI_FAIL=1` 必失败哨兵反证。
  - 全链路发布与生产验收：
    - 提交 `fb24235` 推送 main；CI Run `38155115546` 65/65 jobs 全绿；8 资产 `ALL CHECKSUMS OK`（`4e8a925`）；registry 同步 `9362c3f`；远端 raw 7/7 平台 200 且 sha256 一致；prune 0.1.7（`9171562`）；
    - 生产 `plugin-store install zcode-provider?version=0.1.8` 热重载生效（`active_version=0.1.8 retired_version=0.1.7`，`restart_required:false`，零容器重启）；落盘 `.so` sha256 `aa82c93a282adb435f329cad399f0064b3a186f4a93dde54ad767ec58f2bb197` 与本地 zip 逐字节一致；
    - 生产 `GET /v0/management/zcode-provider-auth-url` 实测返回 **`https://chat.z.ai/api/oauth/authorize?client_id=...&redirect_uri=https://zcode.z.ai/api/v1/oauth/cli/callback/zai&response_type=code&state=...`**，已指向官方注册回调，白名单校验通过。
  - 知识沉淀：知识库笔记《Z.ai-ZCode-OAuth授权契约-CLI轮询模式》+ 项目记忆条目 `feedback-cpa-oauth-redirect-uri`（已纠正为「上游有白名单时必须复用官方 CLI 轮询链路」）。

- 当前会话（2026-10-11）：**ZCode 插件重构为官方 Z.ai OAuth 授权链路，升级发布并在生产部署热重载 `zcode-provider 0.1.5`**：
  - 用户反馈与目标：用户指出 zcode 插件的授权此前误抄自开源项目要求粘贴 BigModel API Key，实际上应该走账号授权模式，通过官方 `https://chat.z.ai/auth?response_type=code&client_id=client_P8X5CMWmlaRO9gyO-KSqtg&redirect_uri=https%3A%2F%2Fzcode.z.ai%2Fapp%2Foauth%2Flogin%3Fredirect%3Dzcode%253A%252F%252Foauth%252Fcallback%26app_version%3D3.15.1&state=...` URL 登录授权并重新实现。
  - 逆向分析与契约收敛：
    1. 反编译分析本地 `D:\zcode\resources\app.asar`（ZCode 3.15.1 官方客户端），锁定官方 OAuth 协议全链路：授权页面 `chat.z.ai/auth`、客户端 ID `client_P8X5CMWmlaRO9gyO-KSqtg`、桌面端重定向 URI `https://zcode.z.ai/app/oauth/login?redirect=zcode%3A%2F%2Foauth%2Fcallback&app_version=3.15.1`；
    2. 授权码交换接口：`POST https://zcode.z.ai/api/v1/oauth/token`（`provider: "zai"`），上游响应正式鉴权令牌 `data.token`（`zcodeJwtToken`）、`data.zai.access_token` 以及完整用户档案 `data.user`（`user_id`、`name`、`email`、`avatar` 等）；业务令牌可选解析接口为 `POST https://api.z.ai/api/auth/z/login`。
  - 实施修复与系统实现（v0.1.5）：
    1. **对齐官方 Z.ai 授权 URL 规范**（`zcode/oauth.go`）：实现 `buildZaiAuthorizeURL` 生成与用户要求及官方客户端逐字符一致的 32 字符 hex state 授权 URL；`handleStartLogin` 直接下发该 URL；
    2. **对接令牌交换与双通道回调捕获**：实现 `exchangeZaiOAuthCode` 向上游兑换正式令牌；升级桥接页 `/oauth-bridge` 支持一键打开官方授权与回调地址/code 自动解析；`handlePollLogin` 增加对宿主原生回调载荷 `.oauth-zcode-provider-<state>.oauth` 的自动读取与落盘支持，彻底消除假成功代码；
    3. **凭据结构与面板升级**：`StoredAuth` 扩展 `token`、`access_token`、`user_id`、`username`、`email`、`avatar_url`、`expires_at`，保持与调用端 `sa.APIKey` 的向下兼容；`panel.html` 增加「OAuth 授权登录」直达入口与卡片用户身份展示。
  - 验证与门禁全绿：
    - 本地 `python scripts/cgo-shim-build.py zcode` 编译、vet 与单测全部通过（包含 `TestZaiOAuth_ExactUserURLMatch` 与 `TestZaiOAuth_HostCallbackFile`）；
    - 必失败哨兵反证测试确证测试进编译真实执行；
    - `node --check` 前端脚本语法校验全绿。
  - 全链路发布与生产热重载验收：
    - 提交 `d92c37b` 推送到 main；GitHub Actions CI Run `38148171959` 73/73 jobs 全绿完成构建与 Release 资产打包；
    - 下载 8 个 Release 资产（7 平台 zip + checksums）校验 `ALL CHECKSUMS OK` 并提交推送 `24570ac`；
    - `registry.json` 同步回填 0.1.5 并验证 schema 通过，提交推送 `ef6523d`；
    - 远端 raw URL 7/7 平台 200 OK 且 sha256 逐字节一致；`prune-release-assets.py` 清理 0.1.4 旧资产释放空间并提交推送 `d6f23c2`；
    - 生产服务器执行 `plugin-store install zcode-provider?version=0.1.5` 热重载生效（`status: installed, restart_required: false`，零容器重启）；
    - 生产落盘 `.so` sha256 `88607b6b0cbc9cca4773b39058205ac388fee65cd34f42c1ce83ecfa0f756cf5` 与本地 zip 逐字节 100% 一致；
    - 生产管理接口验证：`GET /v0/management/plugins/zcode-provider/accounts` 200 OK、`GET /v0/resource/plugins/zcode-provider/panel` 200 OK、`GET /v0/management/zcode-provider-auth-url` 稳定下发真实 `chat.z.ai/auth` 授权地址。

- 当前会话（2026-10-11）：**Qoder 403 首包假成功与全链路流式诊断，经验沉淀至项目 skill，完成生产 Docker Nginx proxy_buffering off 重载验证**：
  - 用户反馈与目标：用户反馈用 New-API 接入 qwen-3.8-flash 进行测试极慢卡死无法使用，但 CPA 面板测试极快、Token 用量插件显示响应不慢；要求排查真实根因，将本次推理请求测试经验吸收到项目 skill，并检查生产 Docker Nginx 容器配置能否 reload 并完成重载。
  - 核心根因锁定（实测铁证）：
    1. **阿里 Qoder 上游 403 排队拦截与 60s 长连接挂死**：通过 SSH 登录生产服务器（45.207.222.65:18998）直测 CPA 8317，发现阿里通义灵码 Qoder 服务端对当前账号的 `qfmodel`（Qwen3.8-Flash）和 `qmodel_38max` 限制调用，在 HTTP 200 下返回 `data: {"code":"403","message":"...10605...isQueued:true...serviceAvailable:false..."}` 业务报错；返回后连接不关闭，持续发送 `: keep-alive` 心跳僵死整整 60 秒，最终上游 HTTP/2 报错 `INTERNAL_ERROR` 崩溃中断。
    2. **CPA 面板探活 219ms 假成功盲区**：`active_ping.go` 发起流式测试后，只要 `reader.Read(buf)` 读到前 512 字节未校验内容即判定流已建立成功并 return；收到 403 错误包仅耗时 219ms，面板误报“活跃成功，耗时 219ms”。
    3. **Token 用量插件 TTFT 334ms 误判陷阱**：用量日志 `token-usage-feed.ndjson` 记录的首包时间 `ttft_ns: 334153032`（334ms）恰为 403 错误帧到达时间，容易造成“响应不慢”假象；但后置字段实为 `failed: true`、`latency_ms: 60100`（卡死 60 秒）且 `total_tokens: 0`。
    4. **New-API 渠道测试机制盲区**：New-API 后台渠道测试按钮底层强制发送 `stream: false`，必须等待全量 Blocking Aggregation 聚合，不能用于流式验证。
    5. **链路通畅对照验证**：用生产可用模型 `auto`（1.4s）与 `deepseek-v4.1-flash`（1.1s）经 New-API 直测，流式打字机逐字实时吐出，确证 New-API 直连 CPA 8317 毫无流式阻塞。
  - 运维与经验沉淀闭环：
    1. **Docker Nginx 语法校验与平滑热重载**：在生产服务器执行 `docker exec nginx nginx -t` 语法检测 PASS（确认已包含 `proxy_buffering off; proxy_cache off;`），执行 `docker exec nginx nginx -s reload` 平滑重载成功；外网域名 `https://api.luode.vip/v1` 实测 1.35s 端到端打字机流式验证通过。
    2. **项目 Skill 经验固化**：更新项目 skill `project-cpa-workbuddy-plugin-sse-and-ping-optimization-rules`（新增规则二首包业务报错帧白名单校验拦截、规则三全链路流式不缓冲配置与推理延迟判读规范）及 `source-notes.md`、`PROJECT_MEMORY.md`、`PROJECT_HISTORY.md`。

- 当前会话（2026-10-11）：**deepseek-v4.1-flash 推理断开根因诊断 + traework 补齐 oauth-excluded-models 治理入口（修复随 traework-provider 0.2.7 上线，现承载于 0.2.8）**：
  - 用户诉求：`new-api → CPA(https://api.luode.vip/v1) → 服务商` 链路上 `deepseek-v4.1-flash` 推理会断开，要求先定位原因并修复，再继续 Token 用量插件（图表移除）任务。
  - 根因（三层叠加，证据见 `doc/4-bugs/2026-10-11_212700_deepseek多服务商同名认领导致推理零首字节断开根因诊断.md`）：
    1. 同名裸模型多服务商认领：`deepseek-v4.1-flash` 由 workbuddy-provider 与 traework-provider 同时动态认领（生产日志 14:06–19:20 内该模型 163 次绑定到 `traework-*.json`），且 traework 是唯一未实现 `filterExcludedModels` 的 OAuth 插件（workbuddy / workbuddy-ai / qoder-ai / qoderwork 均已实现），导致宿主 `oauth-excluded-models` 对它只生效一半——宿主只在 `model.for_auth` 按账号注册路径过滤（`applyExcludedModels`），`model.static` 返回列表被原样登记进全局模型目录（`internal/pluginhost/adapters.go` 静态注册不过滤）。
    2. 会话亲和钉死故障服务商：Trae 号池配额枯竭（16 次 `upstream 200: Your requests have exceeded the quota`）后仍被同一会话反复选中，宿主失败后重绑仍落到另一个 Trae 账号。
    3. 零字节长挂：`traework/host_bridge.go:283 hostBridgeOpenTimeout=30s` 与降级直连 `ResponseHeaderTimeout=30s`（`:430`）**串行**相加，单号最坏 60s 完全不产出字节，而 `handleExecStream`（`traework/executor.go:316`）已先把响应头返回宿主 → 客户端只看到"连上但零输出"。new-api 侧 4 次断开全部 `end_reason=client_gone`、`frt=-1000`、`completion_tokens=0`，耗时 69/80/103/104s；对照成功请求首字节仅 3.7–5.5s。
  - 本轮修复：`traework/models.go` 新增 `filterExcludedModels` 并在 `handleModelStatic` + `handleModelForAuth` 全部返回出口应用（未配置时原样返回，零行为变化；返回新切片避免污染动态缓存）。新增 `traework/models_excluded_test.go` 5 例，含"排除项永不命中"语义变异反证。`gofmt`/`cgo-shim-build.py traework` 全绿，6-review `STYLE: PASS`。
  - 文档：Bug 主文档 + 测试主文档 + 6-review 已落盘；知识库笔记 `20-Knowledge/工程实践/CPA同名模型多服务商认领治理与零首字节断开判读法.md`（含判读法与 `PATCH /v8/management/oauth-excluded-models` 安全写入路径）。
  - 现状：症状已随 Trae 流量退去自然平息（近 2h 该模型 599 次全部 workbuddy-provider、`client_gone` 0 次），但同名认领风险未消除。**0.2.7 发布与生产上线已完成**（后续 0.2.8 由并行会话发布并携带本修复，已在生产 `.so` 内确认 `filterExcludedModels` 存在）：CI run `38143777373` success、registry 同步、生产 `plugin-store install` 热重载 `registered version=0.2.7`、落盘 `.so` sha256 `23ea904c5bec…` 与产物一致、零容器重启、accounts 200（7 账号）、无 panic/fuse。
  - 待办（需用户拍板，已扩为批量）：生产治理动作 `PATCH /v0/management/oauth-excluded-models`（实测前缀只能是 `/v0`，`/v8` 下该路由 404；PATCH 是整表覆盖该 provider，须先 GET 合并）。按 24:03 实测的实时认领矩阵，traework 与 workbuddy / workbuddy-ai 各共享 **6** 个裸名（`deepseek-v4.1-flash`、`glm-5.2`、`glm-5.3`、`glm-5.3-flash`、`kimi-k2.6`、`minimax-m3`/`kimi-k3`），故建议一次决策整批而非只摘一个；`auto` 被四家同时认领属治理盲区，排除前须单独评估。当前触发面活跃：Trae 账号实时列表仍含该名字、7 账号中仅 1 个 active（剩 83 积分）、2 个 exhausted、近 60 分钟仍有 9 次配额错误，而该模型近 24h 无新请求 ⇒ 属已上膛未击发。

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
  "registry_updated_at": "2026-10-12T04:30:00Z",
  "projections": [
    {
      "projection_id": "SESSION/all-plugins-panorama-plan-20261012",
      "session_id": "current-session",
      "projection_origin": "synthesized",
      "synthesis_mode": "exact",
      "state": "active",
      "plan_key": "IMPL-ALL-PLUGINS-PANORAMA-PLAN-20261011",
      "source_document": "doc/3-实施/2026-10-11_043000_九大插件全景架构与演进计划总览.md",
      "plan_fingerprint": "b78912ef34c90ab12de45bc67fa8901234567890abcdef1234567890abcdef12",
      "updated_at": "2026-10-12T04:30:00Z",
      "steps": [
        {
          "id": "TASK-001",
          "step": "[TASK-001] 兄弟插件路由内核同步（traework/workbuddy-ai/qoderwork/qoder-ai）与单测覆盖",
          "status": "completed"
        },
        {
          "id": "TASK-002",
          "step": "[TASK-002] ZCode 插件移植与多账号、指纹 mimic、现代控制台及用量通道对齐",
          "status": "completed"
        },
        {
          "id": "TASK-003",
          "step": "[TASK-003] Cursor 插件体验升级（消除双菜单、现代控制台、备份恢复与用量流接入）",
          "status": "completed"
        },
        {
          "id": "TASK-004",
          "step": "[TASK-004] Qoder 与 Qoder AI 禁用后台自动探活机制（每日请求上限保护）",
          "status": "completed"
        },
        {
          "id": "TASK-005",
          "step": "[TASK-005] 九大插件本地 cgo-shim 全量编译单测与全景计划收口",
          "status": "completed"
        }
      ]
    }
  ]
}
```
<!-- END TASK PLAN PROJECTION -->

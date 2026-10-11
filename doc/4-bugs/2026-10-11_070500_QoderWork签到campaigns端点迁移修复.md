---
schema_version: 1
doc_id: BUG-QODERWORK-CHECKIN-CAMPAIGN-20261011
doc_type: bug
source_ids: [USER-REPORT-QODERWORK-CHECKIN-20261011]
status: fixed-pending-production
version: v1.0
current_slice: 本地修复与回归完成，发布链与生产验收进行中
updated_at: 2026-10-11 07:05:00
reader_level: business_general
writing_style: plain_chinese
appendix_policy: preserve_existing_or_one_terminal_appendix
---

# QoderWork CN 每日签到返回刷新成功但积分不变（生产账号 u09a5b6ab）

结论：是缺陷，且与上一轮「签到假成功」（宿主桥状态码解码，0.9.26 已修）**不同源**。上一轮只把「假成功」修正为「诚实的失败」，但签到的**真实入口仍然错误**：插件仍在调用已废弃端点 `POST /sash/api/v1/me/daily-check-in/claim`，该端点在生产上恒返回 HTTP 409 `AlreadyExists`，永远领不到积分；面板/客户端却显示「可签到 +100」。根因是 CN 每日签到已迁移到 **campaigns（活动）API**，插件未同步迁移（与 qoder-ai 国际版 0.1.4 完全同源：国际域旧端点 404，CN 域旧端点 409）。

影响：受影响的是 qoderwork-provider 的每日签到（面板「签到」/「全部签到」/ 定时自动签到）。用户点位前 188 分不再增长，签到后仅等到「后台刷新完成」提示，积分不变。

范围：`qoderwork/main.go`（端点常量）、`qoderwork/billing.go`（campaigns 头/解析/领取）、`qoderwork/checkin.go`（流程与统计）、`qoderwork/management.go`（`campaign_id`）、`qoderwork/panel.html`（文案）、新增 `qoderwork/checkin_campaigns_test.go`。非范围：不改推理链路、模型映射、账号调度与生命周期、宿主 ABI。

完成标准：

1. CN 签到改走 campaigns API（`GET /sash/api/v1/me/campaigns` + `POST .../{campaignId}/claim`），并带 `Cosy-ClientType:10` + `User-Agent:Qoder` 头。
2. `performCheckinCall` 按 `status=CLAIMED` 判成功，`replayed=true` 归一为「今日已签到」；无活动时如实返回而非发起必然失败的 claim。
3. 本地 `python scripts/cgo-shim-build.py qoderwork` build / vet / test 全绿，新增回归测试在修复前必失败（双反证）。
4. 发布并热重载 0.9.27，生产账号 `u09a5b6ab` 真实签到成功 +100 积分（剩余 188 → 288）。

验证状态：本地修复、回归与双反证已完成；发布链与生产行为验收进行中（2026-10-11）。

## 问题现象

用户原话：「QoderWork 账号, 也就是qoder的国内版, 签到有问题, 我们生产只有一个账号, 积分188, 我们客户端显示是可以签到的, 可以多加100积分, 但是签到无法有效成功, 他会返回后台刷新成功, 但是积分没有变化。需要修复。」

即：账号具备签到资格（客户端显示 +100），点击签到后却拿不到积分，只看到「后台刷新成功/完成」提示。

## 复现

生产环境对单账号调用签到链路并对照上游真实响应：

1. `POST .../qoderwork-provider/checkin` body `{"auth_index":"<u09a5b6ab 对应 auth_index>"}` → 0.9.26 起返回 `success:false` + `http 409 AlreadyExists`。
2. 直接对上游 `GET https://openapi.qoder.com.cn/sash/api/v1/me/daily-check-in/status`（同一 Bearer token）→ 200 但 `{"status":"DISABLED","campaignKey":"cn_daily_check_in_legacy","rewardCredits":100}`。
3. 直接 `POST .../daily-check-in/claim` → HTTP 409 `{"errorCode":"AlreadyExists"}`。
4. 对照 `GET /sash/api/v1/me/campaigns`（带 `Cosy-ClientType:10`+`User-Agent:Qoder`）→ 200 `claimable:true`，`campaigns[]` 含 `actionType=CLAIM_BENEFIT`、`claimStatus=CLAIMABLE`、`benefit.amount=100`。
5. 对比签到前后 `credits.total_remain`：不变（188）。

## 根因

**根因（缺陷 C，生产实测 + 客户端逆向双证）**：CN 签到端点已废弃，真实签到走 campaigns API。

- 生产实测（2026-10-11，账号 `u09a5b6ab`，token 前缀 `dt-`）：
  - `GET daily-check-in/status` → 200 `{"status":"DISABLED","campaignKey":"cn_daily_check_in_legacy"}`。
  - `POST daily-check-in/claim` → 409 `AlreadyExists`（恒失败，永不发奖）。
  - `GET /sash/api/v1/me/campaigns`（**不带** Cosy 头）→ 200 `{"showCampaign":false,"claimable":false,"campaigns":[]}`（活动被**掩盖**）。
  - `GET /sash/api/v1/me/campaigns`（**带** `Cosy-ClientType:10`+`User-Agent:Qoder`）→ 200 真实活动：`campaignId=01a12102-...`、`campaignKey=act-20261009-747`、`actionType=CLAIM_BENEFIT`、`claimStatus=CLAIMABLE`、`benefit={"kind":"CREDITS","amount":100}`。
  - `POST /sash/api/v1/me/campaigns/{campaignId}/claim`（合法 UUID 假 id）→ 404 `CAMPAIGN_NOT_FOUND`（证明 claim 路由存在）。
- 官方客户端反解与运行日志（CN 版 v0.4.4，`com.qodercn.app.stable`）：`app.asar` 中 `daily-check-in` **0 命中**、`/claim` **0 命中**；campaign 服务常量 `zBr="/sash/api/v1/me/campaigns"`、`campaignKey="client_launch_26"`；请求头契约 `Fh={clientType:10}` → `{Accept:"application/json", Authorization:"Bearer ...", "Cosy-ClientType":"10", "User-Agent":"Qoder"}`。运行日志实测 `GET https://openapi.qoder.com.cn/sash/api/v1/me/campaigns` 返回真实活动（`campaignKey=act-20261009-747`，`claimStatus=CLAIMABLE`，`benefit.amount=100`），全程无 `daily-check-in` 调用。
- 结论：插件仍用 CN 已废弃的 daily-check-in 端点，导致「返回刷新成功但积分不变」。

**同类已修先例**：qoder-ai 国际版 0.1.4（提交 `340923c`）修复过完全相同的形态——国际域旧端点 404，改走 campaigns API + Cosy 头。qoderwork 未同步迁移。

## 修复边界

- 只改 CN 签到端点与解析（缺陷 C），不改推理、模型、调度、宿主 ABI。

## 修复与验证结论

主方案：把 qoder-ai 0.1.4 已验证的 campaigns 方案移植到 qoderwork。

1. `main.go`：新增 `endpointCampaigns` / `endpointCampaignClaim`，移除 `endpointCheckinStatus` / `endpointCheckinClaim`。
2. `billing.go`：新增 `campaignHeaders`（`Cosy-ClientType:10` + `User-Agent:Qoder`）与 `campaignStatusResponse` / `campaign` / `benefit` 结构；`fetchCheckinStatus` 改解析 campaigns 列表并用 `pickCheckinCampaign` 挑窗口内 `CLAIM_BENEFIT` 的 CREDITS 活动；`performCheckinCall(sa, campaignID)` 改 POST `.../campaigns/{campaignId}/claim`，按 `status=CLAIMED` 判成功、`replayed=true` 归一为「今日已签到」。
3. `checkin.go`：自动/手动签到均传入 `ci.CampaignID`；`ci.Active=false` 时如实返回「当前无可领取的签到活动」（reason=global），不再走必然失败的 claim；summary 新增 `no_activity`（不再计入 fail）。
4. `management.go`：`checkinSummary` 增加 `campaign_id`。
5. `panel.html`：成功提示改用 `reward_credits`；「无可领取活动」文案修正；批量签到单列「无活动」。

本地验证：`python scripts/cgo-shim-build.py qoderwork` build / vet / test 全绿；新增 `qoderwork/checkin_campaigns_test.go` 覆盖端点契约、Cosy 头契约、CLAIM_BENEFIT 命中/已领取/窗口外/无活动/非 CREDITS 赠品。双反证：① 把 `ClaimStatus=="CLAIMED"` 改成错误值 → `TestPickCheckinCampaign_ClaimedInWindow` 必失败（`checked=false`）；② 删掉 `Cosy-ClientType` 头 → `TestCampaignHeaders_CosyContract` 必失败（`Cosy-ClientType=""; want "10"`）。panel.html 两段 script `node --check` 通过，并额外用 vm + DOM stub 真实执行顶层脚本（无 ReferenceError）。

发布与生产部署（2026-10-11，进行中）：源码 commit `5b4acba` → CI run `38119587051` → assets → registry → 生产 plugin-store install 0.9.27 → 生产账号 `u09a5b6ab` 真实签到 +100 验收。

## 追踪附录

| 追踪 ID | 对应内容 | 证据 |
| --- | --- | --- |
| REQ-QODERWORK-CHECKIN-FIX-20261011 | 生产账号签到真实成功、积分增加 | 用户原话 |
| AC-QODERWORK-CHECKIN-003 | CN 签到改走 campaigns API 且带 Cosy 头 | `checkin_campaigns_test.go` 回归 + 双反证 |
| AC-QODERWORK-CHECKIN-004 | 生产账号真实复验签到成功 +100 | 生产 `/checkin` 返回 success:true + reward_credits=100 |
| EVIDENCE-QODERWORK-CHECKIN-002 | 旧端点已废弃（status DISABLED / claim 409） | 生产 token 实测 |
| EVIDENCE-QODERWORK-CHECKIN-003 | CN 客户端 v0.4.4 用 campaigns 且需 Cosy 头 | app.asar 反解 + 运行日志 |

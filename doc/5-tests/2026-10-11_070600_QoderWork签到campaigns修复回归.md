---
schema_version: 1
doc_id: TEST-QODERWORK-CHECKIN-CAMPAIGN-20261011
doc_type: test
source_ids: [BUG-QODERWORK-CHECKIN-CAMPAIGN-20261011]
status: passed
version: v1.0
updated_at: 2026-10-11 07:06:00
reader_level: business_general
writing_style: plain_chinese
---

# QoderWork CN 签到改走 campaigns API 回归验证

## 验证范围

- CN 签到端点由 `daily-check-in/{status,claim}` 改为 campaigns（活动）API。
- 新增 `campaignHeaders`（`Cosy-ClientType:10` + `User-Agent:Qoder`）与 `pickCheckinCampaign` 活动挑选。
- `performCheckinCall(sa, campaignID)` 改 POST `.../campaigns/{campaignId}/claim`。
- `checkinSummary` 增加 `campaign_id`；`handleManualCheckin` summary 新增 `no_activity`。
- 不改推理链路、模型映射、账号调度与生命周期、宿主 ABI。

## 验证入口

- 真实测试入口：`python scripts/cgo-shim-build.py qoderwork`（build + vet + test）。
- 测试文件：`qoderwork/checkin_campaigns_test.go`（与源码同目录，满足 Go 测试可编译路径要求）。

## 用例与结果

| 用例 | 断言 | 结果 |
| --- | --- | --- |
| TestQoderWork_CampaignEndpointsContract | 端点常量指向 campaigns（含 claim 前缀） | PASS |
| TestCampaignHeaders_CosyContract | campaigns 请求带 `Cosy-ClientType:10` + `User-Agent:Qoder` + Bearer | PASS |
| TestPickCheckinCampaign_ClaimBenefit | 挑出窗口内 CLAIM_BENEFIT 的 CREDITS 活动，识别 CLAIMABLE | PASS |
| TestPickCheckinCampaign_ClaimedInWindow | 窗口内已领取活动识别为 todayChecked | PASS |
| TestPickCheckinCampaign_OutOfWindow | 窗口外活动不算 active | PASS |
| TestPickCheckinCampaign_NoCampaign | 无活动时全零 | PASS |
| TestPickCheckinCampaign_SkipsNonCreditsBenefit | 非 CREDITS 赠品不被误当签到活动 | PASS |

## 必失败反证

反证 1：把 `pickCheckinCampaign` 的 `ClaimStatus == "CLAIMED"` 临时改成错误哨兵值后执行测试：

```
--- FAIL: TestPickCheckinCampaign_ClaimedInWindow (0.00s)
    checkin_campaigns_test.go:92: want active=true checked=true; got active=true checked=false
FAIL
```

反证 2：删除 `campaignHeaders` 中的 `Cosy-ClientType` 设置行后执行测试：

```
--- FAIL: TestCampaignHeaders_CosyContract (0.00s)
    checkin_campaigns_test.go:34: Cosy-ClientType=""; want "10"
FAIL
```

证明：
1. 新增测试真实进入编译并覆盖根因路径（活动识别与请求头契约）；
2. 修复前（缺 Cosy 头 / 未迁移 campaigns）确实产生错误行为。

恢复正确实现后重跑，build / vet / test 全绿（`ok github.com/luode0320/cpa-workbuddy-plugin/qoderwork`）。

## 面板脚本校验

- `qoderwork/panel.html` 两段 `<script>` 经 `node --check` 通过。
- 额外用 Node `vm` + DOM/BOM stub 真实执行两段脚本顶层，无 `ReferenceError`（覆盖改名变量 `noActivity` 定义/使用成对；`skippedCN` 已无残留）。

## 提交态独立编译校验

- 工作树含并行会话对 qoderwork 其它文件（watchdog/scheduler/session_auth/panel 的 preserve→test 等）的未提交改动。
- 按仓库「非本轮改动默认保护」，用 `git archive HEAD qoderwork` 导出干净基线，叠加本轮 8 个文件后跑 `cgo-shim-build` 全绿，证明**提交态（HEAD + 本轮改动）可独立编译测试**。

## 结论

本地验证通过：`cgo-shim-build.py qoderwork` build / vet / test 全绿，双反证成立，提交态独立编译全绿。生产复验（账号 `u09a5b6ab` 签到 +100）随 0.9.27 发布部署执行。

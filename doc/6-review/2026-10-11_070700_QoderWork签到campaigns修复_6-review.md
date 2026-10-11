# 6-review 风格回归：QoderWork CN 签到改走 campaigns API（0.9.27）

- 日期：2026-10-11 07:07
- 范围：qoderwork/main.go、qoderwork/billing.go、qoderwork/checkin.go、qoderwork/management.go、qoderwork/panel.html、qoderwork/checkin_campaigns_test.go、qoderwork/CHANGELOG.md、qoderwork/VERSION
- 结论：STYLE: PASS

## 检查步骤（static_owner_router.py 推导）

- STYLE-01 静态格式与编码一致性：变更文件均为 UTF-8；Go 文件经 `cgo-shim-build.py` 的 build/vet 校验通过，格式与仓库既有 Go 写法一致；panel.html 两段 script `node --check` 通过，另经 vm+DOM stub 顶层执行无 ReferenceError。
- STYLE-02 命名与符号引用一致：新增 `campaignHeaders` / `campaignStatusResponse` / `campaign` / `benefit` / `pickCheckinCampaign` 命名与既有 `*Response` / `fetch*` 风格一致；端点常量沿用 `endpoint*` 前缀（`endpointCampaigns` / `endpointCampaignClaim`），与 qoder-ai 同源命名对齐。
- STYLE-03 注释分层与定义位置：新增/修改函数注释含 `[参数]`/`[返回]`/`最近修改时间` 元信息，其余位置为单行目的说明；`campaignHeaders`、`pickCheckinCampaign`、`performCheckinCall`、`fetchCheckinStatus` 均补齐中文元信息。
- STYLE-04 函数签名与参数结构：`pickCheckinCampaign` 返回多值而非输出参数；`performCheckinCall(sa, campaignID)` 参数清晰，与调用方一致。
- STYLE-08 语言与框架特异写法：Go 惯用法，`json.Unmarshal` + map 断言显式判空；时间窗用 `time.Time.Unix()` 比较；HTTP 头集中由 `campaignHeaders` 构造。
- STYLE-09 测试资产与编码前契约：回归测试落在 `qoderwork/checkin_campaigns_test.go`，命名 `TestPickCheckinCampaign_*` / `TestCampaignHeaders_*` / `TestQoderWork_CampaignEndpointsContract` 符合仓库惯例。

## 证据

- `python scripts/cgo-shim-build.py qoderwork`：build / vet / test 全绿。
- 双反证：① 破坏 `ClaimStatus=="CLAIMED"`，`TestPickCheckinCampaign_ClaimedInWindow` 必失败（`checked=false`）；② 删除 `Cosy-ClientType` 头，`TestCampaignHeaders_CosyContract` 必失败（`Cosy-ClientType=""; want "10"`）。
- panel.html 两段 script `node --check` 通过；vm + DOM stub 顶层执行无 ReferenceError。
- 提交态独立编译：`git archive HEAD qoderwork` + 叠加本轮 8 文件后 cgo-shim 全绿。
- 变更 diff：端点常量替换、billing 新增 campaigns 解析与头、checkin 传参与统计、management 加字段、panel 文案，无无关格式化或整文件重排（panel.html 已剥离并行会话的 `preserve→test` 改动，仅保留本轮 3 处签到改动）。

## 负向边界

本记录只判断写法、位置、格式与既有习惯，不判断业务正确性、需求覆盖或发布放行。

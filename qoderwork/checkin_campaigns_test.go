package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestQoderWork_CampaignEndpointsContract 校验 CN 签到端点已从废弃的
// daily-check-in 迁到 campaigns（活动）API。生产实测（2026-10-11）：旧
// claim 端点恒返回 409 AlreadyExists，永远领不到积分。
func TestQoderWork_CampaignEndpointsContract(t *testing.T) {
	if endpointCampaigns != "https://openapi.qoder.com.cn/sash/api/v1/me/campaigns" {
		t.Fatalf("endpointCampaigns=%q; want .../sash/api/v1/me/campaigns", endpointCampaigns)
	}
	if endpointCampaignClaim != "https://openapi.qoder.com.cn/sash/api/v1/me/campaigns/" {
		t.Fatalf("endpointCampaignClaim=%q; want .../campaigns/ prefix", endpointCampaignClaim)
	}
}

// TestCampaignHeaders_CosyContract 校验 campaigns 请求头契约：CN 官方客户端
// v0.4.4 要求 Cosy-ClientType:10 + User-Agent:Qoder；缺这两个头时活动列表被
// 掩盖为 {"showCampaign":false,"campaigns":[]}（生产实测）。
func TestCampaignHeaders_CosyContract(t *testing.T) {
	sa := &storedAuth{}
	sa.Auth.AccessToken = "dt-test-token"
	req, err := http.NewRequest(http.MethodGet, endpointCampaigns, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	campaignHeaders(req, sa)
	if got := req.Header.Get("Cosy-ClientType"); got != "10" {
		t.Fatalf("Cosy-ClientType=%q; want \"10\"", got)
	}
	if got := req.Header.Get("User-Agent"); got != "Qoder" {
		t.Fatalf("User-Agent=%q; want \"Qoder\"", got)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer dt-test-token" {
		t.Fatalf("Authorization=%q; want Bearer token", got)
	}
}

// TestPickCheckinCampaign_ClaimBenefit 用 CN 生产真实响应形状验证：挑出窗口内
// CLAIM_BENEFIT 的 CREDITS 活动，并识别其可领取状态。
func TestPickCheckinCampaign_ClaimBenefit(t *testing.T) {
	raw := `{
      "uid":"01a04386-6ee7-7b2e-adde-d0cc806974a3",
      "showCampaign":true,"claimable":true,
      "campaignUrl":"https://openapi.qoder.com.cn/growth-page/activity-iframe",
      "campaigns":[
        {"campaignId":"01a12102-1a77-7699-860d-136e42ef52d1","campaignKey":"act-20261009-747",
         "actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMABLE",
         "startAt":1791684000,"endAt":1791770340,
         "benefit":{"kind":"CREDITS","amount":100}},
        {"campaignId":"01a05bbf-5668-7031-83d6-91545f97ec05","campaignKey":"act-20260901-922",
         "actionType":"VIEW_DETAILS","claimStatus":"CLAIMED",
         "startAt":1788243600,"endAt":1793462340,"benefit":{}}
      ]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, checked, cid, credit := pickCheckinCampaign(&q, time.Unix(1791700000, 0))
	if !active {
		t.Fatal("active=false; want true (CLAIM_BENEFIT CREDITS campaign in window)")
	}
	if checked {
		t.Fatal("todayChecked=true; want false (claimStatus=CLAIMABLE)")
	}
	if cid != "01a12102-1a77-7699-860d-136e42ef52d1" {
		t.Fatalf("campaignID=%q; want the CLAIM_BENEFIT campaign id", cid)
	}
	if credit != 100 {
		t.Fatalf("dailyCredit=%d; want 100", credit)
	}
}

// TestPickCheckinCampaign_ClaimedInWindow 验证窗口内已领取的活动被识别为
// todayChecked（用于「今日已签到」分支）。
func TestPickCheckinCampaign_ClaimedInWindow(t *testing.T) {
	raw := `{"campaigns":[{"campaignId":"c1","campaignKey":"act-20261009-319",
      "actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMED",
      "startAt":1791597600,"endAt":1791683940,
      "benefit":{"kind":"CREDITS","amount":100}}]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, checked, cid, credit := pickCheckinCampaign(&q, time.Unix(1791600000, 0))
	if !active || !checked {
		t.Fatalf("want active=true checked=true; got active=%v checked=%v", active, checked)
	}
	if cid != "c1" || credit != 100 {
		t.Fatalf("want cid=c1 credit=100; got cid=%q credit=%d", cid, credit)
	}
}

// TestPickCheckinCampaign_OutOfWindow 验证窗口外的活动不算 active（避免对已
// 过期活动发起必然失败的 claim）。
func TestPickCheckinCampaign_OutOfWindow(t *testing.T) {
	raw := `{"campaigns":[{"campaignId":"x","actionType":"CLAIM_BENEFIT",
      "claimStatus":"CLAIMABLE","startAt":100,"endAt":200,
      "benefit":{"kind":"CREDITS","amount":100}}]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, checked, _, _ := pickCheckinCampaign(&q, time.Unix(300, 0))
	if active {
		t.Fatal("active=true; want false (now past endAt)")
	}
	if checked {
		t.Fatal("todayChecked=true; want false")
	}
}

// TestPickCheckinCampaign_NoCampaign 验证无 CLAIM_BENEFIT 活动时全零（对应
// 活动未开放或账号不适用，面板显示「暂无可领取活动」）。
func TestPickCheckinCampaign_NoCampaign(t *testing.T) {
	raw := `{"showCampaign":false,"claimable":false,"campaigns":[]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, checked, cid, credit := pickCheckinCampaign(&q, time.Unix(1791700000, 0))
	if active || checked || cid != "" || credit != 0 {
		t.Fatalf("want all zero for empty campaigns; got active=%v checked=%v cid=%q credit=%d", active, checked, cid, credit)
	}
}

// TestPickCheckinCampaign_SkipsNonCreditsBenefit 验证非 CREDITS 赠品（如
// 会员权益）不被误当作签到活动。
func TestPickCheckinCampaign_SkipsNonCreditsBenefit(t *testing.T) {
	raw := `{"campaigns":[{"campaignId":"x","actionType":"CLAIM_BENEFIT",
      "claimStatus":"CLAIMABLE","startAt":100,"endAt":9999999999,
      "benefit":{"kind":"SUBSCRIPTION","amount":1}}]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, _, cid, credit := pickCheckinCampaign(&q, time.Unix(200, 0))
	if active || cid != "" || credit != 0 {
		t.Fatalf("want all zero for non-CREDITS benefit; got active=%v cid=%q credit=%d", active, cid, credit)
	}
}

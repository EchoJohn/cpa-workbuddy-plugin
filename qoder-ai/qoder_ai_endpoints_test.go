package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestQoderAI_GlobalEndpointsContract verifies that Qoder AI uses dedicated
// global domain constants and correct endpoints.
func TestQoderAI_GlobalEndpointsContract(t *testing.T) {
	if providerName != "qoder-ai-provider" {
		t.Fatalf("providerName=%q; want 'qoder-ai-provider'", providerName)
	}
	if authFileName != "qoderai.json" {
		t.Fatalf("authFileName=%q; want 'qoderai.json'", authFileName)
	}
	if authFilePrefix != "qoderai-" {
		t.Fatalf("authFilePrefix=%q; want 'qoderai-'", authFilePrefix)
	}
	if upstreamBase != "https://openapi.qoder.sh" {
		t.Fatalf("upstreamBase=%q; want 'https://openapi.qoder.sh'", upstreamBase)
	}
	if gatewayBase != "https://openapi.qoder.sh" {
		t.Fatalf("gatewayBase=%q; want 'https://openapi.qoder.sh'", gatewayBase)
	}
	// 国际版签到走 campaigns（活动）API，不再使用 CN 专有的 daily-check-in 端点
	// （后者在 openapi.qoder.sh 恒返回 404 NotFound）。
	if endpointCampaigns != "https://openapi.qoder.sh/sash/api/v1/me/campaigns" {
		t.Fatalf("endpointCampaigns=%q; want .../sash/api/v1/me/campaigns", endpointCampaigns)
	}
	if !strings.HasPrefix(endpointCampaignClaim, "https://openapi.qoder.sh/sash/api/v1/me/campaigns/") {
		t.Fatalf("endpointCampaignClaim=%q; want prefix .../campaigns/", endpointCampaignClaim)
	}
	if qoderWebsite != "https://qoder.com" {
		t.Fatalf("qoderWebsite=%q; want 'https://qoder.com'", qoderWebsite)
	}
}

// TestPickCheckinCampaign_ClaimBenefit 验证从 campaigns 列表里正确挑出
// CLAIM_BENEFIT 的 CREDITS 活动，并识别已领取状态。
func TestPickCheckinCampaign_ClaimBenefit(t *testing.T) {
	// 生产真实响应形状（2026-10-11 实测）。
	raw := `{
      "uid":"01a125f7-f6f1-73ff-9928-1ae6a554edc3",
      "showCampaign":true,"claimable":false,
      "campaignUrl":"https://openapi.qoder.sh/growth-page/activity-iframe",
      "campaigns":[
        {"campaignId":"01a120ff-e293-7c9c-864c-c21392e9a771","campaignKey":"act-20261009-118",
         "actionType":"CLAIM_BENEFIT","claimStatus":"CLAIMED",
         "startAt":1000,"endAt":9999999999,
         "benefit":{"kind":"CREDITS","amount":100}},
        {"campaignId":"01a05bce-e800-7494-a002-806e4438f483","campaignKey":"act-20260901-493",
         "actionType":"VIEW_DETAILS","claimStatus":"CLAIMED",
         "startAt":1000,"endAt":9999999999,"benefit":{}}
      ]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, checked, cid, credit := pickCheckinCampaign(&q, time.Unix(2000, 0))
	if !active {
		t.Fatal("active=false; want true (CLAIM_BENEFIT CREDITS campaign in window)")
	}
	if !checked {
		t.Fatal("todayChecked=false; want true (claimStatus=CLAIMED)")
	}
	if cid != "01a120ff-e293-7c9c-864c-c21392e9a771" {
		t.Fatalf("campaignID=%q; want the CLAIM_BENEFIT campaign id", cid)
	}
	if credit != 100 {
		t.Fatalf("dailyCredit=%d; want 100", credit)
	}
}

// TestPickCheckinCampaign_OutOfWindow 验证窗口外的活动不算 active。
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

// TestPickCheckinCampaign_NoCampaign 验证无 CLAIM_BENEFIT 活动时 active=false
// （对应国际版活动未开放或账号不适用）。
func TestPickCheckinCampaign_NoCampaign(t *testing.T) {
	raw := `{"showCampaign":false,"claimable":false,"campaigns":[]}`
	var q campaignStatusResponse
	if err := json.Unmarshal([]byte(raw), &q); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	active, checked, cid, credit := pickCheckinCampaign(&q, time.Unix(2000, 0))
	if active || checked || cid != "" || credit != 0 {
		t.Fatalf("want all zero for empty campaigns; got active=%v checked=%v cid=%q credit=%d", active, checked, cid, credit)
	}
}

// TestDeriveUMIDCodes_GoldenParity 验证 deriveUMIDCodes 与官方 runtime-info 二进制实测样本逐字符一致，防 xxh64 或位插值偏移，无副作用。
func TestDeriveUMIDCodes_GoldenParity(t *testing.T) {
	// 1. 校验 Windows 实测样本（vmCode=14, pct=77）
	winToken := "P1gArIgCgHLNfpZ2s8k6Ns0cOmaAAacODyke6bidFVndFvh_-SI_Etjx2D2Hea6fsE2E4hSkFDet8bxb_HNMaZab"
	winCodes := deriveUMIDCodes(umidDeriveInput{MachineToken: winToken, VMCode: 14, Pct: 77})
	if winCodes.MachineType != "51f6feff14f099a261" {
		t.Fatalf("mTypeWin=%q; want 51f6feff14f099a261", winCodes.MachineType)
	}
	if winCodes.MachineCode != "1c06e2fa7691148402" {
		t.Fatalf("mCodeWin=%q; want 1c06e2fa7691148402", winCodes.MachineCode)
	}
	// 2. 校验 Linux 实测样本（vmCode=91, pct=0）
	linToken := "P1gApdVL9Wb9zzWpUqwS0F-9U6ZyrPo4q7CJDsUxC0YurygCnQY5Fif0m5qGrb_S6WvCtTPTCFTZcFQR8sd8N_47"
	linCodes := deriveUMIDCodes(umidDeriveInput{MachineToken: linToken, VMCode: 91, Pct: 0})
	if linCodes.MachineType != "9698528191ba3fcff5" {
		t.Fatalf("mTypeLin=%q; want 9698528191ba3fcff5", linCodes.MachineType)
	}
	if linCodes.MachineCode != "a0d07094007ade534b" {
		t.Fatalf("mCodeLin=%q; want a0d07094007ade534b", linCodes.MachineCode)
	}
}

// TestDecryptUMIDResponseDT_GoldenSample 验证 UMID 响应 dt 解密能够还原真实 P1gA 令牌，防密钥表或 CBC 解密漂移，无副作用。
func TestDecryptUMIDResponseDT_GoldenSample(t *testing.T) {
	dtSample := "CuzZqBuG25Skd+agWO6iNFya1P5vBqkvigEjqXiJZTh3zFcOuLWWyI2ku0TZYoiy5MPkEa0FGT4BvvNoh9hA5K2TkhjQvSn+N1dh5Ph95tQXXip3Pen5+HV/7cMc+eW75hq+xi3lDEwrg6rD8jhhG/s="
	wantToken := "P1gAyGUvqyAFif6i1bwrTu-DuqZuAYTM3s-17fy1MFe8k_zOs6c6ST6wnUZbvdQFK-mXcBCDvCtED485o8TJFC_t"
	got, err := decryptUMIDResponseDT(dtSample)
	if err != nil {
		t.Fatalf("decryptUMIDResponseDT err: %v", err)
	}
	if got != wantToken {
		t.Fatalf("got=%q; want %q", got, wantToken)
	}
}

// TestCampaignHeaders_InjectsMachineIdentity 验证 campaignHeaders 正确注入完整 Cosy-* 设备标识头，防缺头导致上游隐藏每日签到活动，无外部副作用。
func TestCampaignHeaders_InjectsMachineIdentity(t *testing.T) {
	uid := "unit-test-uid-001"
	token := "P1gAyGUvqyAFif6i1bwrTu-DuqZuAYTM3s-17fy1MFe8k_zOs6c6ST6wnUZbvdQFK-mXcBCDvCtED485o8TJFC_t"
	codes := deriveUMIDCodes(umidDeriveInput{MachineToken: token, VMCode: 91, Pct: 0})
	umidCache.Store(uid, &umidCacheEntry{
		identity: umidIdentity{
			MachineID:       "11111111-2222-3333-4444-555555555555",
			MachineHostname: "qoder-test-host",
			MachineOS:       "x86_64_linux",
			MachineToken:    token,
			MachineType:     codes.MachineType,
			MachineCode:     codes.MachineCode,
		},
		salt:      0,
		fetchedAt: time.Now(),
	})
	defer umidCache.Delete(uid)

	sa := &storedAuth{
		Auth:    storedTokens{AccessToken: "jt-test-token"},
		Account: storedAccount{UID: uid},
	}
	req, err := http.NewRequest(http.MethodGet, endpointCampaigns, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	campaignHeaders(req, sa)
	if got := req.Header.Get("Cosy-ClientType"); got != "10" {
		t.Fatalf("Cosy-ClientType=%q; want 10", got)
	}
	if got := req.Header.Get("Cosy-Version"); got != qoderClientVer {
		t.Fatalf("Cosy-Version=%q; want %q", got, qoderClientVer)
	}
	if got := req.Header.Get("Cosy-MachineToken"); got != token {
		t.Fatalf("Cosy-MachineToken=%q; want %q", got, token)
	}
	if got := req.Header.Get("Cosy-MachineType"); got != codes.MachineType {
		t.Fatalf("Cosy-MachineType=%q; want %q", got, codes.MachineType)
	}
	if got := req.Header.Get("Cosy-MachineCode"); got != codes.MachineCode {
		t.Fatalf("Cosy-MachineCode=%q; want %q", got, codes.MachineCode)
	}
	if got := req.Header.Get("Cosy-MachineId"); got != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("Cosy-MachineId=%q; want 11111111-2222-3333-4444-555555555555", got)
	}
}


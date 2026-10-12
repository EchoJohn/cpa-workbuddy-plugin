package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// TestFetchAccountQuota_ParsesPlan 验证 billing/current 响应被正确解析为额度快照。
func TestFetchAccountQuota_ParsesPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
			t.Errorf("expected Bearer auth, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"","data":{"server_time":1,"plans":[{"user_plan_id":"upl_1","plan_id":"zcode-v3-start-plan-0817","name":"ZCode Start Plan","description":"免费","status":"active","starts_at":1,"ends_at":2,"entitlements":[{"entitlement_id":"ent_1","show_name":"GLM-5.3","meter":"model_usage","unit_type":"token","capabilities":["model:glm-5.3"],"grant_units":3000000,"period":"daily","priority":110},{"entitlement_id":"ent_2","show_name":"GLM-5.3-Flash","meter":"model_usage","unit_type":"token","capabilities":["model:glm-5.3-flash"],"grant_units":5000000,"period":"daily","priority":80}]}]}}`))
	}))
	defer srv.Close()

	// 临时替换端点常量不可行（const），改用注入变量。
	oldURL := quotaURLForTest
	quotaURLForTest = srv.URL
	defer func() { quotaURLForTest = oldURL }()

	snap, err := fetchAccountQuota(&StoredAuth{AuthID: "zcode-test", Token: "jwt-token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.PlanName != "ZCode Start Plan" {
		t.Fatalf("expected plan name, got %q", snap.PlanName)
	}
	if len(snap.Entitlements) != 2 {
		t.Fatalf("expected 2 entitlements, got %d", len(snap.Entitlements))
	}
	if snap.TotalGrant != 8000000 {
		t.Fatalf("expected total grant 8000000, got %d", snap.TotalGrant)
	}
	if snap.Entitlements[0].Capability != "model:glm-5.3" {
		t.Fatalf("expected capability model:glm-5.3, got %q", snap.Entitlements[0].Capability)
	}
}

// TestFetchAccountQuota_RequiresJWT 验证缺少 JWT 时返回错误而非空快照。
func TestFetchAccountQuota_RequiresJWT(t *testing.T) {
	if _, err := fetchAccountQuota(&StoredAuth{AuthID: "x"}); err == nil {
		t.Fatalf("expected error for account without jwt")
	}
}

// TestQuotaForAccount_Caches 验证额度结果被缓存，重复查询不再打上游。
func TestQuotaForAccount_Caches(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"code":0,"data":{"plans":[{"name":"P","status":"active","entitlements":[{"show_name":"M","grant_units":100}]}]}}`))
	}))
	defer srv.Close()
	oldURL := quotaURLForTest
	quotaURLForTest = srv.URL
	defer func() { quotaURLForTest = oldURL }()

	sa := &StoredAuth{AuthID: "zcode-cache", Token: "jwt"}
	if _, err := quotaForAccount(sa, false); err != nil {
		t.Fatalf("first fetch failed: %v", err)
	}
	if _, err := quotaForAccount(sa, false); err != nil {
		t.Fatalf("second fetch failed: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 upstream hit due to cache, got %d", hits)
	}
	// force=true 必须绕过缓存。
	if _, err := quotaForAccount(sa, true); err != nil {
		t.Fatalf("forced fetch failed: %v", err)
	}
	if hits != 2 {
		t.Fatalf("expected 2 upstream hits after force, got %d", hits)
	}
	invalidateQuota("zcode-cache")
	if cachedQuota("zcode-cache") != nil {
		t.Fatalf("expected cache cleared after invalidate")
	}
}

// TestCachedQuota_Expires 验证过期缓存不被返回。
func TestCachedQuota_Expires(t *testing.T) {
	zcodeQuotaMu.Lock()
	zcodeQuotaCache["zcode-exp"] = zcodeQuotaCacheEntry{
		quota:     &zcodeQuotaSnapshot{PlanName: "old"},
		fetchedAt: time.Now().Add(-2 * zcodeQuotaTTL),
	}
	zcodeQuotaMu.Unlock()
	if cachedQuota("zcode-exp") != nil {
		t.Fatalf("expected expired cache to be ignored")
	}
	zcodeQuotaMu.Lock()
	delete(zcodeQuotaCache, "zcode-exp")
	zcodeQuotaMu.Unlock()
}

// TestRefreshRunner_SnapshotProgress 验证刷新器进度快照与幂等入队。
func TestRefreshRunner_SnapshotProgress(t *testing.T) {
	r := newZcodeRefreshRunner()
	// 不启动真实刷新，直接构造状态验证快照口径。
	r.mu.Lock()
	r.order = []string{"a", "b", "c"}
	r.states = map[string]refreshJobState{
		"a": {Status: "done"},
		"b": {Status: "failed", Error: "boom"},
		"c": {Status: "pending"},
	}
	r.inFlight = true
	r.started = time.Now()
	r.mu.Unlock()

	snap := r.Snapshot()
	if snap.Total != 3 || snap.Done != 1 || snap.Failed != 1 {
		t.Fatalf("unexpected snapshot counts: %+v", snap)
	}
	if !snap.Running || snap.StartedAt == "" {
		t.Fatalf("expected running snapshot with started_at, got %+v", snap)
	}
	// 幂等：已有刷新在跑时 EnqueueAll 返回 -1。
	if got := r.EnqueueAll([]refreshTarget{{AuthID: "d"}}); got != -1 {
		t.Fatalf("expected -1 when in-flight, got %d", got)
	}
	// 快照必须可 JSON 序列化（前端契约）。
	if _, err := json.Marshal(snap); err != nil {
		t.Fatalf("snapshot not marshalable: %v", err)
	}
}

// TestHandleQuotaQuery_AllAccounts 验证全量额度查询逐账号返回，单账号失败不影响整体。
func TestHandleQuotaQuery_AllAccounts(t *testing.T) {
	dir := t.TempDir()
	setAccountsDir(dir)
	defer setAccountsDir("")

	writeAuthFileDirect(dir+"/zcode-aaaaaaaaaaaaaaaa.json", &StoredAuth{
		Type: zcodeProviderID, Provider: zcodeProviderID, AuthID: "zcode-aaaaaaaaaaaaaaaa",
		APIKey: "jwt-a", Token: "jwt-a", Label: "A",
	})

	raw, err := handleQuotaQuery("", false)
	if err != nil {
		t.Fatalf("handleQuotaQuery failed: %v", err)
	}
	// handleQuotaQuery 返回 okEnvelope(ManagementResponse)，需先解外层 envelope，
	// 再取 ManagementResponse 的 body 字段。
	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("envelope parse failed: %v", err)
	}
	if !env.OK {
		t.Fatalf("expected ok envelope, got %s", string(raw))
	}
	var resp pluginapi.ManagementResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("management response parse failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct {
		Accounts []map[string]any `json:"accounts"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("body parse failed: %v", err)
	}
	if len(body.Accounts) != 1 {
		t.Fatalf("expected 1 account row, got %d", len(body.Accounts))
	}
	// 上游不可达时必须返回 error 字段而非 panic。
	if _, hasErr := body.Accounts[0]["error"]; !hasErr {
		t.Fatalf("expected error field for unreachable upstream, got %+v", body.Accounts[0])
	}
}

// TestClaimAccountStartPlan_MockSuccess 验证套餐激活开通接口与请求头构造。
func TestClaimAccountStartPlan_MockSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/claim" {
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("X-Aliyun-Captcha-Verify-Param") != "mock_verify_param_123" {
				t.Fatalf("missing or wrong verify param header")
			}
			if r.Header.Get("X-Aliyun-Captcha-Verify-Region") != "sgp" {
				t.Fatalf("missing or wrong verify region header")
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"plan":{"plan_id":"zcode-v3-start-plan","status":"active"}}}`))
			return
		}
		if r.URL.Path == "/current" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"code":0,"data":{"plans":[{"plan_id":"zcode-v3-start-plan","name":"ZCode Start Plan","status":"active","entitlements":[{"entitlement_id":"e1","show_name":"GLM-5.3","grant_units":3000000}]}]}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	claimURLForTest = ts.URL + "/claim"
	quotaURLForTest = ts.URL + "/current"
	defer func() {
		claimURLForTest = ""
		quotaURLForTest = ""
	}()

	sa := &StoredAuth{
		AuthID: "zcode-test-claim",
		Token:  "jwt-test-claim",
		APIKey: "jwt-test-claim",
	}

	snap, err := claimAccountStartPlan(sa, "mock_verify_param_123", "sgp")
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if snap == nil || snap.PlanID != "zcode-v3-start-plan" || snap.TotalGrant != 3000000 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

// TestHandleClaimAccount_CaptchaRequired 验证当上游要求验证码时返回结构化配置。
func TestHandleClaimAccount_CaptchaRequired(t *testing.T) {
	dir := t.TempDir()
	setAccountsDir(dir)
	defer setAccountsDir("")

	writeAuthFileDirect(dir+"/zcode-bbbbbbbbbbbbbbbb.json", &StoredAuth{
		Type: zcodeProviderID, Provider: zcodeProviderID, AuthID: "zcode-bbbbbbbbbbbbbbbb",
		APIKey: "jwt-b", Token: "jwt-b", Label: "B",
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":3007,"msg":"captcha verify failed"}`))
	}))
	defer ts.Close()

	claimURLForTest = ts.URL
	defer func() { claimURLForTest = "" }()

	body := []byte(`{"auth_id":"zcode-bbbbbbbbbbbbbbbb"}`)
	raw, err := handleClaimAccount(body)
	if err != nil {
		t.Fatalf("handleClaimAccount error: %v", err)
	}

	var env struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	_ = json.Unmarshal(raw, &env)
	var resp pluginapi.ManagementResponse
	_ = json.Unmarshal(env.Result, &resp)

	var res struct {
		OK              bool   `json:"ok"`
		CaptchaRequired bool   `json:"captcha_required"`
		SceneID         string `json:"scene_id"`
	}
	_ = json.Unmarshal(resp.Body, &res)
	if res.OK || !res.CaptchaRequired || res.SceneID != defaultAliyunSceneID {
		t.Fatalf("expected captcha_required response, got %+v", res)
	}
}

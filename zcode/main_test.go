package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestMain_RegistrationAndCapabilities(t *testing.T) {
	if os.Getenv("ZCODE_SENTINEL_FAILURE") == "1" {
		t.Fatalf("SENTINEL_FAILURE: verifying that tests actually compile and run")
	}

	raw, err := handleMethod(pluginabi.MethodPluginRegister, []byte(`{}`))
	if err != nil {
		t.Fatalf("handleMethod register failed: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope failed: %v", err)
	}
	if !env.OK {
		t.Fatalf("registration envelope not OK")
	}

	var reg registration
	if err := json.Unmarshal(env.Result, &reg); err != nil {
		t.Fatalf("unmarshal registration failed: %v", err)
	}

	if reg.Metadata.Name != providerName {
		t.Fatalf("expected name %s, got %s", providerName, reg.Metadata.Name)
	}
	if reg.Metadata.Version != "0.1.6" {
		t.Fatalf("expected version 0.1.6, got %s", reg.Metadata.Version)
	}
	if !reg.Capabilities.AuthProvider {
		t.Fatalf("expected AuthProvider capability to be true for OAuth login")
	}
	if !reg.Capabilities.ModelProvider || !reg.Capabilities.Executor || !reg.Capabilities.Scheduler || !reg.Capabilities.ManagementAPI {
		t.Fatalf("missing required capabilities: %+v", reg.Capabilities)
	}
}

func TestMain_OAuthLoginFlow(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zcode_oauth_test_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	setAccountsDir(tempDir)

	// 1. StartLogin
	rawStart, err := handleMethod(pluginabi.MethodAuthLoginStart, nil)
	if err != nil {
		t.Fatalf("handleMethod AuthLoginStart failed: %v", err)
	}
	var envStart envelope
	if err := json.Unmarshal(rawStart, &envStart); err != nil || !envStart.OK {
		t.Fatalf("unmarshal AuthLoginStart failed: %v", err)
	}
	var startResp pluginapi.AuthLoginStartResponse
	if err := json.Unmarshal(envStart.Result, &startResp); err != nil {
		t.Fatalf("unmarshal startResp failed: %v", err)
	}
	if startResp.Provider != providerName || startResp.State == "" {
		t.Fatalf("unexpected startResp: %+v", startResp)
	}
	if !strings.HasPrefix(startResp.URL, "https://chat.z.ai/auth?") {
		t.Fatalf("expected Z.ai auth URL, got %s", startResp.URL)
	}
	if !strings.Contains(startResp.URL, "client_id=client_P8X5CMWmlaRO9gyO-KSqtg") {
		t.Fatalf("expected client_id in auth URL, got %s", startResp.URL)
	}
	if !strings.Contains(startResp.URL, "response_type=code") {
		t.Fatalf("expected response_type=code in auth URL, got %s", startResp.URL)
	}
	if !strings.Contains(startResp.URL, "zcode.z.ai%2Fapp%2Foauth%2Flogin") {
		t.Fatalf("expected redirect_uri in auth URL, got %s", startResp.URL)
	}
	if !strings.Contains(startResp.URL, "state="+startResp.State) {
		t.Fatalf("expected state in auth URL, got %s", startResp.URL)
	}

	// 2. Poll (pending)
	pollBody, _ := json.Marshal(pluginapi.AuthLoginPollRequest{State: startResp.State})
	rawPoll, err := handleMethod(pluginabi.MethodAuthLoginPoll, pollBody)
	if err != nil {
		t.Fatalf("handleMethod AuthLoginPoll failed: %v", err)
	}
	var envPoll envelope
	_ = json.Unmarshal(rawPoll, &envPoll)
	var pollResp pluginapi.AuthLoginPollResponse
	_ = json.Unmarshal(envPoll.Result, &pollResp)
	if pollResp.Status != pluginapi.AuthLoginStatusPending {
		t.Fatalf("expected pending status, got %s", pollResp.Status)
	}

	// 3. 启动 Mock 上游 Token 交换服务器
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req zaiTokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Provider != "zai" || req.Code != "test-auth-code-12345" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		resp := zaiTokenResponse{
			Code: 0,
			Msg:  "",
		}
		resp.Data.Token = "mock_jwt_token_99999"
		resp.Data.ExpiresIn = 2592000
		resp.Data.Zai.AccessToken = "mock_access_token_88888"
		resp.Data.Zai.TokenType = "Bearer"
		resp.Data.User = &ZaiUserInfo{
			UserID: "u_test_123",
			Name:   "测试用户",
			Email:  "test@z.ai",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()
	mockOAuthTokenURL = ts.URL
	defer func() { mockOAuthTokenURL = "" }()

	// 4. 模拟在桥接页提交完整回调 URL
	callbackURL := "https://zcode.z.ai/app/oauth/login?redirect=zcode%3A%2F%2Foauth%2Fcallback&app_version=3.15.1&code=test-auth-code-12345&state=" + startResp.State
	q := url.Values{}
	q.Set("state", startResp.State)
	q.Set("callback_url", callbackURL)
	bridgeReq := pluginapi.ManagementRequest{
		Method: "GET",
		Path:   "/v0/resource/plugins/" + providerName + "/oauth-bridge",
		Query:  q,
	}
	bridgeResp := handleOAuthBridge(bridgeReq)
	if bridgeResp.StatusCode != 200 || !strings.Contains(string(bridgeResp.Body), "授权绑定成功") {
		t.Fatalf("bridgeResp failed: status=%d body=%s", bridgeResp.StatusCode, string(bridgeResp.Body))
	}

	// 5. Poll (success)
	rawPoll2, _ := handleMethod(pluginabi.MethodAuthLoginPoll, pollBody)
	var envPoll2 envelope
	_ = json.Unmarshal(rawPoll2, &envPoll2)
	var pollResp2 pluginapi.AuthLoginPollResponse
	_ = json.Unmarshal(envPoll2.Result, &pollResp2)
	if pollResp2.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("expected success status, got %s msg=%s", pollResp2.Status, pollResp2.Message)
	}
	if pollResp2.Auth.Provider != providerName || !strings.Contains(pollResp2.Auth.Label, "测试用户") {
		t.Fatalf("unexpected Auth record: %+v", pollResp2.Auth)
	}

	// 6. Refresh
	refreshBody, _ := json.Marshal(map[string]any{"auth": pollResp2.Auth})
	rawRefresh, err := handleMethod(pluginabi.MethodAuthRefresh, refreshBody)
	if err != nil {
		t.Fatalf("handleMethod AuthRefresh failed: %v", err)
	}
	var envRefresh envelope
	_ = json.Unmarshal(rawRefresh, &envRefresh)
	if !envRefresh.OK {
		t.Fatalf("expected AuthRefresh OK")
	}
}

// TestMain_OAuthBridgeReceivesHostQuery 复现宿主真实调用契约：宿主把 URL path
// 与 query 分开传递（Path 不含 query，参数在 Query），桥接页必须能读到 state。
// 回归背景：曾用从 path 解析出的 query 覆盖 req.Query，导致 state 丢失、
// 桥接页返回「缺少 state 参数」，管理后台授权链接不可用。
func TestMain_OAuthBridgeReceivesHostQuery(t *testing.T) {
	rawStart, err := handleMethod(pluginabi.MethodAuthLoginStart, nil)
	if err != nil {
		t.Fatalf("start login failed: %v", err)
	}
	var envStart envelope
	_ = json.Unmarshal(rawStart, &envStart)
	var startResp pluginapi.AuthLoginStartResponse
	_ = json.Unmarshal(envStart.Result, &startResp)

	// 宿主契约：Path 不含 query，query 单独在 Query 字段。
	bridgeBody, _ := json.Marshal(pluginapi.ManagementRequest{
		Method: "GET",
		Path:   "/v0/resource/plugins/" + providerName + "/oauth-bridge",
		Query:  url.Values{"state": []string{startResp.State}},
	})
	rawBridge, err := handleMethod(pluginabi.MethodManagementHandle, bridgeBody)
	if err != nil {
		t.Fatalf("management handle failed: %v", err)
	}
	var envBridge envelope
	if err := json.Unmarshal(rawBridge, &envBridge); err != nil || !envBridge.OK {
		t.Fatalf("bridge envelope not OK: %v", err)
	}
	var resp pluginapi.ManagementResponse
	if err := json.Unmarshal(envBridge.Result, &resp); err != nil {
		t.Fatalf("unmarshal management response failed: %v", err)
	}
	body := string(resp.Body)
	if strings.Contains(body, "缺少 state 参数") {
		t.Fatalf("bridge page lost the host-provided state query")
	}
	if !strings.Contains(body, "Z.ai 官方登录") {
		t.Fatalf("bridge page did not render the authorization form, body=%s", body)
	}
}

func TestZaiOAuth_ExactUserURLMatch(t *testing.T) {
	state := "a87957462749638ca95fb1afe0073c43"
	got := buildZaiAuthorizeURL(state)
	expected := "https://chat.z.ai/auth?client_id=client_P8X5CMWmlaRO9gyO-KSqtg&redirect_uri=https%3A%2F%2Fzcode.z.ai%2Fapp%2Foauth%2Flogin%3Fredirect%3Dzcode%253A%252F%252Foauth%252Fcallback%26app_version%3D3.15.1&response_type=code&state=a87957462749638ca95fb1afe0073c43"

	uGot, err1 := url.Parse(got)
	uExp, err2 := url.Parse(expected)
	if err1 != nil || err2 != nil {
		t.Fatalf("parse url failed: %v %v", err1, err2)
	}
	if uGot.Scheme != uExp.Scheme || uGot.Host != uExp.Host || uGot.Path != uExp.Path {
		t.Fatalf("URL mismatch: got %s vs exp %s", got, expected)
	}
	qGot := uGot.Query()
	qExp := uExp.Query()
	for k := range qExp {
		if qGot.Get(k) != qExp.Get(k) {
			t.Fatalf("param %s mismatch: got %q vs exp %q", k, qGot.Get(k), qExp.Get(k))
		}
	}
}

func TestZaiOAuth_HostCallbackFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zcode_host_cb_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	setAccountsDir(tempDir)

	state := "test_host_cb_state_12345"
	session := &oauthLoginSession{CreatedAt: time.Now()}
	oauthSessions.Store(state, session)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req zaiTokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		resp := zaiTokenResponse{Code: 0}
		resp.Data.Token = "mock_token_from_host_cb"
		resp.Data.ExpiresIn = 86400
		resp.Data.User = &ZaiUserInfo{UserID: "u_cb", Name: "回调用户"}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()
	mockOAuthTokenURL = ts.URL
	defer func() { mockOAuthTokenURL = "" }()

	cbFile := filepath.Join(tempDir, ".oauth-"+providerName+"-"+state+".oauth")
	payload := map[string]string{
		"code":  "cb_auth_code_999",
		"state": state,
	}
	cbData, _ := json.Marshal(payload)
	if err := os.WriteFile(cbFile, cbData, 0600); err != nil {
		t.Fatalf("write cb file failed: %v", err)
	}

	pollBody, _ := json.Marshal(pluginapi.AuthLoginPollRequest{State: state})
	rawPoll, err := handleMethod(pluginabi.MethodAuthLoginPoll, pollBody)
	if err != nil {
		t.Fatalf("poll failed: %v", err)
	}
	var envPoll envelope
	_ = json.Unmarshal(rawPoll, &envPoll)
	var pollResp pluginapi.AuthLoginPollResponse
	_ = json.Unmarshal(envPoll.Result, &pollResp)
	if pollResp.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("expected success, got %s: %s", pollResp.Status, pollResp.Message)
	}
	if !strings.Contains(pollResp.Auth.Label, "回调用户") {
		t.Fatalf("unexpected label: %s", pollResp.Auth.Label)
	}
}

func TestMain_IdentifierAndModels(t *testing.T) {
	// 认证标识
	rawAuth, err := handleMethod(pluginabi.MethodAuthIdentifier, nil)
	if err != nil {
		t.Fatalf("auth identifier failed: %v", err)
	}
	var envAuth envelope
	_ = json.Unmarshal(rawAuth, &envAuth)
	var idResp identifierResponse
	_ = json.Unmarshal(envAuth.Result, &idResp)
	if idResp.Identifier != providerName {
		t.Fatalf("expected identifier %s, got %s", providerName, idResp.Identifier)
	}

	// 执行器标识
	rawExec, err := handleMethod(pluginabi.MethodExecutorIdentifier, nil)
	if err != nil {
		t.Fatalf("executor identifier failed: %v", err)
	}
	var envExec envelope
	_ = json.Unmarshal(rawExec, &envExec)
	_ = json.Unmarshal(envExec.Result, &idResp)
	if idResp.Identifier != providerName {
		t.Fatalf("expected identifier %s, got %s", providerName, idResp.Identifier)
	}

	// 静态模型列表
	rawModels, err := handleMethod(pluginabi.MethodModelStatic, nil)
	if err != nil {
		t.Fatalf("model static failed: %v", err)
	}
	var envModels envelope
	_ = json.Unmarshal(rawModels, &envModels)
	var modelResp pluginapi.ModelResponse
	_ = json.Unmarshal(envModels.Result, &modelResp)
	if len(modelResp.Models) == 0 {
		t.Fatalf("expected non-empty static models")
	}
}

func TestMain_Reconfigure(t *testing.T) {
	configYAML := `
api_key: "cfg-test-key.secret"
base_url: "https://test.upstream.ai"
timeout_seconds: 60
`
	req := struct {
		ConfigYAML []byte `json:"config_yaml"`
	}{
		ConfigYAML: []byte(configYAML),
	}
	reqBytes, _ := json.Marshal(req)

	raw, err := handleMethod(pluginabi.MethodPluginReconfigure, reqBytes)
	if err != nil {
		t.Fatalf("reconfigure failed: %v", err)
	}
	var env envelope
	_ = json.Unmarshal(raw, &env)
	if !env.OK {
		t.Fatalf("reconfigure envelope not OK")
	}

	cfg := currentConfig()
	if cfg.APIKey != "cfg-test-key.secret" {
		t.Fatalf("expected APIKey cfg-test-key.secret, got %s", cfg.APIKey)
	}
	if cfg.BaseURL != "https://test.upstream.ai" {
		t.Fatalf("expected BaseURL https://test.upstream.ai, got %s", cfg.BaseURL)
	}
	if cfg.TimeoutSeconds != 60 {
		t.Fatalf("expected TimeoutSeconds 60, got %d", cfg.TimeoutSeconds)
	}
}

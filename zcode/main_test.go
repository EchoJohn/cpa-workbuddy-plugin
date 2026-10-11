package main

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"

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
	if reg.Metadata.Version != "0.1.3" {
		t.Fatalf("expected version 0.1.3, got %s", reg.Metadata.Version)
	}
	if !reg.Capabilities.AuthProvider {
		t.Fatalf("expected AuthProvider capability to be true for OAuth login")
	}
	if !reg.Capabilities.ModelProvider || !reg.Capabilities.Executor || !reg.Capabilities.Scheduler || !reg.Capabilities.ManagementAPI {
		t.Fatalf("missing required capabilities: %+v", reg.Capabilities)
	}
}

func TestMain_OAuthLoginFlow(t *testing.T) {
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
	if startResp.Provider != providerName || startResp.State == "" || !strings.Contains(startResp.URL, "/oauth-bridge?state=") {
		t.Fatalf("unexpected startResp: %+v", startResp)
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

	// 3. 模拟在桥接页提交 Key
	q := url.Values{}
	q.Set("state", startResp.State)
	q.Set("api_key", "test-bigmodel-key-12345.secret67890")
	q.Set("label", "单元测试账号")
	bridgeReq := pluginapi.ManagementRequest{
		Method: "GET",
		Path:   "/v0/resource/plugins/" + providerName + "/oauth-bridge",
		Query:  q,
	}
	bridgeResp := handleOAuthBridge(bridgeReq)
	if bridgeResp.StatusCode != 200 || !strings.Contains(string(bridgeResp.Body), "授权绑定成功") {
		t.Fatalf("bridgeResp failed: status=%d body=%s", bridgeResp.StatusCode, string(bridgeResp.Body))
	}

	// 4. Poll (success)
	rawPoll2, _ := handleMethod(pluginabi.MethodAuthLoginPoll, pollBody)
	var envPoll2 envelope
	_ = json.Unmarshal(rawPoll2, &envPoll2)
	var pollResp2 pluginapi.AuthLoginPollResponse
	_ = json.Unmarshal(envPoll2.Result, &pollResp2)
	if pollResp2.Status != pluginapi.AuthLoginStatusSuccess {
		t.Fatalf("expected success status, got %s msg=%s", pollResp2.Status, pollResp2.Message)
	}
	if pollResp2.Auth.Provider != providerName || pollResp2.Auth.Label != "单元测试账号" {
		t.Fatalf("unexpected Auth record: %+v", pollResp2.Auth)
	}

	// 5. Refresh
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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/luode0320/cpa-workbuddy-plugin/zcode/internal/mimic"
	"github.com/tidwall/gjson"
)

type testActiveRequest struct {
	AuthIndex string `json:"auth_index"`
	Model     string `json:"model"`
}

type testActiveResponse struct {
	OK        bool   `json:"ok"`
	Status    string `json:"status"`
	Model     string `json:"model,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

// handleModelsQuery 处理 GET /models?auth_index= 路由请求。
func handleModelsQuery(authID string) ([]byte, error) {
	models := authModelsForIndex(authID)
	return json.Marshal(map[string]any{
		"models": models,
	})
}

// handleTestActive 处理 POST /test-active 请求。
func handleTestActive(body []byte) ([]byte, error) {
	authIndex := gjson.GetBytes(body, "auth_index").String()
	if authIndex == "" {
		authIndex = gjson.GetBytes(body, "auth_id").String()
	}
	model := gjson.GetBytes(body, "model").String()

	authIndex = strings.TrimSpace(authIndex)
	if authIndex == "" {
		return json.Marshal(testActiveResponse{
			OK:     false,
			Status: "failed",
			Error:  "auth_index is required",
		})
	}

	accounts, err := listAllAuthFiles()
	if err != nil || len(accounts) == 0 {
		return json.Marshal(testActiveResponse{
			OK:     false,
			Status: "failed",
			Error:  "no accounts found",
		})
	}

	var target *StoredAuth
	for _, acc := range accounts {
		if acc.AuthID == authIndex || authFileNameFor(acc.APIKey) == authIndex+".json" {
			target = acc
			break
		}
	}

	if target == nil {
		return json.Marshal(testActiveResponse{
			OK:     false,
			Status: "failed",
			Error:  fmt.Sprintf("account %s not found", authIndex),
		})
	}

	res := handleTestActiveWithAuth(target, model)
	return json.Marshal(res)
}

// handleTestActiveWithAuth 向上游发起单次 Anthropic 流式首包即停真实探活。
// [参数] sa: ZCode 账号凭据；model: 待测试模型名称。
// [返回] testActiveResponse: 探活状态、耗时与结果消息。
// 最近修改时间：2026-10-12 00:20:00 改用业务令牌（zai access_token → business token），根治 OAuth 账号 401
func handleTestActiveWithAuth(sa *StoredAuth, model string) testActiveResponse {
	if sa == nil || (sa.APIKey == "" && sa.Token == "" && sa.AccessToken == "") {
		return testActiveResponse{
			OK:     false,
			Status: "failed",
			Error:  "invalid account or empty api key",
		}
	}

	cfg := currentConfig()

	// 按账号类型选择上游端点与凭证（start-plan 走 zcode-plan + JWT，付费走 api.z.ai + 业务令牌）。
	targets := resolveUpstreamTargets(sa, cfg)
	if len(targets) == 0 {
		return testActiveResponse{
			OK:     false,
			Status: "failed",
			Error:  "unable to resolve upstream credential",
		}
	}
	target := targets[0]

	if strings.TrimSpace(model) == "" {
		models := authModelsForIndex(sa.AuthID)
		if len(models) > 0 {
			model = models[0]
		} else {
			model = "GLM-5.3"
		}
	}
	upstreamModel := mappedModel(model, cfg)

	// 1. 构造流式最小探活负载（stream=true, max_tokens=1）
	pingPayload := map[string]any{
		"model":      upstreamModel,
		"max_tokens": 1,
		"stream":     true,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
	}
	payloadBytes, _ := json.Marshal(pingPayload)

	reqURL := target.URL + "/v1/messages"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payloadBytes))
	if err != nil {
		cancel()
		return testActiveResponse{
			OK:     false,
			Status: "failed",
			Model:  model,
			Error:  err.Error(),
		}
	}

	fp := mimic.New(cfg.Mimic, keyID(target.Credential))
	fp.ApplyRequest(httpReq.Header, target.Credential, mimic.UUID4())
	applyTargetAuth(httpReq.Header, target)

	// 2. 发起请求并在收到首个数据块后立即异步关闭流，避免等待整段思考链完成
	start := time.Now()
	resp, err := modelHTTPClient.Do(httpReq)
	if err != nil {
		cancel()
		latency := time.Since(start).Milliseconds()
		sa.TestFailed = true
		saveAuthTestStatus(sa)
		noteAccountFailure(sa.AuthID, 0, err.Error())
		publishUsage(sa.AuthID, sa.Label, model, upstreamModel, start, usageTokensDetail{InputTokens: 1, OutputTokens: 0, TotalTokens: 1}, latency, 0, true, err.Error(), 0)
		return testActiveResponse{
			OK:        false,
			Status:    "failed",
			Model:     model,
			LatencyMS: latency,
			Error:     err.Error(),
		}
	}
	defer func() {
		go func() {
			_ = resp.Body.Close()
			cancel()
		}()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		latency := time.Since(start).Milliseconds()
		sa.TestFailed = true
		saveAuthTestStatus(sa)
		noteAccountFailure(sa.AuthID, resp.StatusCode, string(bodyBytes))
		publishUsage(sa.AuthID, sa.Label, model, upstreamModel, start, usageTokensDetail{InputTokens: 1, OutputTokens: 0, TotalTokens: 1}, latency, resp.StatusCode, true, string(bodyBytes), 0)
		return testActiveResponse{
			OK:        false,
			Status:    "failed",
			Model:     model,
			LatencyMS: latency,
			Error:     fmt.Sprintf("upstream status %d: %s", resp.StatusCode, string(bodyBytes)),
		}
	}

	// 3. 读取首个分片（512 字节）即视为流已建立，拦截流内显式错误帧
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	latency := time.Since(start).Milliseconds()
	firstChunk := string(buf[:n])
	ttftNS := uint64(latency * 1000000)
	if strings.Contains(firstChunk, `"type":"error"`) || strings.Contains(firstChunk, `"type": "error"`) {
		sa.TestFailed = true
		saveAuthTestStatus(sa)
		noteAccountFailure(sa.AuthID, http.StatusForbidden, firstChunk)
		publishUsage(sa.AuthID, sa.Label, model, upstreamModel, start, usageTokensDetail{InputTokens: 1, OutputTokens: 0, TotalTokens: 1}, latency, http.StatusForbidden, true, firstChunk, ttftNS)
		return testActiveResponse{
			OK:        false,
			Status:    "failed",
			Model:     model,
			LatencyMS: latency,
			Error:     fmt.Sprintf("upstream stream error: %s", firstChunk),
		}
	}

	// 4. 探活成功：清标 test_failed，重置 failover
	sa.TestFailed = false
	saveAuthTestStatus(sa)
	resetAccountFailover(sa.AuthID)
	publishUsage(sa.AuthID, sa.Label, model, upstreamModel, start, usageTokensDetail{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}, latency, resp.StatusCode, false, "", ttftNS)

	return testActiveResponse{
		OK:        true,
		Status:    "active",
		Model:     model,
		LatencyMS: latency,
		Message:   "活跃测试成功",
	}
}

// saveAuthTestStatus 保存账号探活测试状态落盘。
func saveAuthTestStatus(sa *StoredAuth) {
	if sa == nil || sa.APIKey == "" {
		return
	}
	var fileName string
	if sa.AuthID != "" {
		authID := strings.TrimSpace(sa.AuthID)
		if !strings.HasPrefix(authID, zcodeAuthFilePrefix) {
			authID = zcodeAuthFilePrefix + authID
		}
		fileName = authID + ".json"
	} else {
		fileName = authFileNameFor(sa.APIKey)
	}
	fullPath := filepath.Join(getAccountsDir(), fileName)
	sa.UpdatedAt = time.Now().Format(time.RFC3339)
	_ = writeAuthFileDirect(fullPath, sa)
}

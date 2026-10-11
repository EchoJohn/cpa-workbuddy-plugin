package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Z.ai 业务令牌（business access_token）换取与缓存。
//
// 背景（2026-10-12 由 zcode.z.ai 线上接口实测 + ZCode 3.15.1 桌面端 app.asar 逆向确认）：
// OAuth 轮询拿到的 `zai.access_token`（user-service 签发的短时效 token）**不能**直接
// 作为模型 API Key 使用——直接打 `api.z.ai/api/anthropic` 会返回 401
// `token expired or incorrect`（这正是面板「测试模型」报 401 的根因）。
//
// 正确链路（与官方 `zai-api` 模板 `access.type=zhipu-coding-plan-api-key` 一致）：
//
//	POST https://api.z.ai/api/auth/z/login
//	Content-Type: application/json
//	{"token": "<zai access_token>"}
//	→ {"code":200,"data":{"access_token":"<business token>", ...}}
//
// 随后以该 business token 作为模型请求的 `x-api-key`（或 `Authorization: Bearer`）
// 访问 `https://api.z.ai/api/anthropic/v1/messages`。
const (
	zaiBusinessLoginEndpoint = "https://api.z.ai/api/auth/z/login"
	// businessTokenTTL 是业务令牌的保守缓存时长。上游未返回可靠 expires_in 时使用；
	// 401 会被上层换号/重试逻辑兜住，缓存过长只会多一次失败请求，不会造成永久错误。
	businessTokenTTL = 30 * time.Minute
)

// mockZaiBusinessLoginURL 供单元测试注入 mock 端点。
var mockZaiBusinessLoginURL string

type businessTokenCacheEntry struct {
	token     string
	expiresAt time.Time
}

var (
	businessTokenMu    sync.RWMutex
	businessTokenCache = make(map[string]businessTokenCacheEntry)
)

// zaiBusinessLoginRequest 是 /api/auth/z/login 的请求体。
type zaiBusinessLoginRequest struct {
	Token string `json:"token"`
}

// zaiBusinessLoginResponse 是 /api/auth/z/login 的响应体。
type zaiBusinessLoginResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

// isBusinessLoginSuccess 判定业务登录响应是否成功。
// 上游实测返回 `code:200`（HTTP 语义），部分路径返回 `code:0`，两者均视为成功。
// [参数] code: 上游响应 code。
// [返回] true 表示成功。
func isBusinessLoginSuccess(code int) bool {
	return code == 0 || code == 200
}

// resolveBusinessToken 用 zai access_token 换取业务令牌，带进程内缓存。
// [参数] zaiAccessToken: OAuth 轮询得到的 zai access_token。
// [返回] 业务 access_token；失败时返回错误，由调用方决定回退策略。
func resolveBusinessToken(zaiAccessToken string) (string, error) {
	token := strings.TrimSpace(zaiAccessToken)
	if token == "" {
		return "", fmt.Errorf("empty zai access token")
	}

	businessTokenMu.RLock()
	entry, ok := businessTokenCache[token]
	businessTokenMu.RUnlock()
	if ok && entry.token != "" && time.Now().Before(entry.expiresAt) {
		return entry.token, nil
	}

	endpoint := zaiBusinessLoginEndpoint
	if mockZaiBusinessLoginURL != "" {
		endpoint = mockZaiBusinessLoginURL
	}

	body, errMarshal := json.Marshal(zaiBusinessLoginRequest{Token: token})
	if errMarshal != nil {
		return "", fmt.Errorf("marshal business login request: %w", errMarshal)
	}

	req, errNew := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if errNew != nil {
		return "", fmt.Errorf("create business login request: %w", errNew)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, errDo := modelHTTPClient.Do(req)
	if errDo != nil {
		return "", fmt.Errorf("request business login: %w", errDo)
	}
	defer resp.Body.Close()

	raw, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return "", fmt.Errorf("read business login response: %w", errRead)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("business login upstream error %d: %s", resp.StatusCode, string(raw))
	}

	var parsed zaiBusinessLoginResponse
	if errUnmarshal := json.Unmarshal(raw, &parsed); errUnmarshal != nil {
		return "", fmt.Errorf("parse business login response: %w", errUnmarshal)
	}
	if !isBusinessLoginSuccess(parsed.Code) {
		msg := strings.TrimSpace(parsed.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code=%d", parsed.Code)
		}
		return "", fmt.Errorf("business login failed: %s", msg)
	}
	businessToken := strings.TrimSpace(parsed.Data.AccessToken)
	if businessToken == "" {
		return "", fmt.Errorf("business login response missing data.access_token")
	}

	businessTokenMu.Lock()
	businessTokenCache[token] = businessTokenCacheEntry{
		token:     businessToken,
		expiresAt: time.Now().Add(businessTokenTTL),
	}
	businessTokenMu.Unlock()

	return businessToken, nil
}

// invalidateBusinessToken 使某个 zai access_token 对应的业务令牌缓存失效。
// [参数] zaiAccessToken: OAuth 轮询得到的 zai access_token。
// [返回] 无。
func invalidateBusinessToken(zaiAccessToken string) {
	token := strings.TrimSpace(zaiAccessToken)
	if token == "" {
		return
	}
	businessTokenMu.Lock()
	delete(businessTokenCache, token)
	businessTokenMu.Unlock()
}

// modelCredentialFor 返回该账号用于模型请求的鉴权令牌。
//
// 优先使用换取到的业务令牌（OAuth 账号的正确凭证）；换取失败或账号仅有静态
// API Key 时回退到账号自身保存的 Key，保证旧有 API Key 账号不受影响。
// [参数] sa: 账号凭据。
// [返回] 用于模型请求的令牌。
func modelCredentialFor(sa *StoredAuth) string {
	if sa == nil {
		return ""
	}
	if source := strings.TrimSpace(sa.AccessToken); source != "" {
		if businessToken, errResolve := resolveBusinessToken(source); errResolve == nil {
			return businessToken
		}
	}
	if key := strings.TrimSpace(sa.APIKey); key != "" {
		return key
	}
	return strings.TrimSpace(sa.Token)
}

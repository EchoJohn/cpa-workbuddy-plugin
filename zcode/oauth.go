package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// Z.ai 官方 OAuth 配置常量（与 ZCode 3.15.1 官方桌面端完全一致）
const (
	zaiOAuthAuthorizeURL = "https://chat.z.ai/auth"
	zaiOAuthClientID     = "client_P8X5CMWmlaRO9gyO-KSqtg"
	zaiOAuthRedirectURI  = "https://zcode.z.ai/app/oauth/login?redirect=zcode%3A%2F%2Foauth%2Fcallback&app_version=3.15.1"
	zaiOAuthTokenURL     = "https://zcode.z.ai/api/v1/oauth/token"
	zaiBusinessLoginURL  = "https://api.z.ai/api/auth/z/login"
	oauthTTL             = 15 * time.Minute
)

var (
	oauthHTTPClient   = &http.Client{Timeout: 30 * time.Second}
	mockOAuthTokenURL string // 供单元测试注入 mock 端点
)

type oauthLoginSession struct {
	CreatedAt time.Time
	Auth      *pluginapi.AuthData
	Error     string
	Completed bool
}

var oauthSessions sync.Map

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// buildZaiAuthorizeURL 生成真实的 Z.ai 登录授权链接。
func buildZaiAuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("response_type", "code")
	q.Set("client_id", zaiOAuthClientID)
	q.Set("redirect_uri", zaiOAuthRedirectURI)
	q.Set("state", state)
	return zaiOAuthAuthorizeURL + "?" + q.Encode()
}

// zaiTokenRequest 向上游 /api/v1/oauth/token 发送的请求体。
type zaiTokenRequest struct {
	Provider    string `json:"provider"`
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
	State       string `json:"state"`
}

// zaiTokenResponse 上游响应体。
type zaiTokenResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Token     string `json:"token"`
		ExpiresIn int64  `json:"expires_in"`
		Zai       struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
		} `json:"zai"`
		User *ZaiUserInfo `json:"user,omitempty"`
	} `json:"data"`
}

// exchangeZaiOAuthCode 向 ZCode 上游交换授权码为正式 Token 并落盘保存账号。
func exchangeZaiOAuthCode(code, state, label string) (*StoredAuth, error) {
	code = strings.TrimSpace(code)
	state = strings.TrimSpace(state)
	if code == "" {
		return nil, fmt.Errorf("authorization code is required")
	}

	tokenURL := zaiOAuthTokenURL
	if mockOAuthTokenURL != "" {
		tokenURL = mockOAuthTokenURL
	}

	reqPayload := zaiTokenRequest{
		Provider:    "zai",
		Code:        code,
		RedirectURI: zaiOAuthRedirectURI,
		State:       state,
	}
	bodyBytes, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal token request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, tokenURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := oauthHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request oauth token: %w", err)
	}
	defer httpResp.Body.Close()

	respRaw, err := io.ReadAll(io.LimitReader(httpResp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read oauth token response: %w", err)
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth token upstream error %d: %s", httpResp.StatusCode, string(respRaw))
	}

	var tokenResp zaiTokenResponse
	if err := json.Unmarshal(respRaw, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse oauth token response: %w", err)
	}

	if tokenResp.Code != 0 {
		msg := strings.TrimSpace(tokenResp.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code=%d", tokenResp.Code)
		}
		return nil, fmt.Errorf("Z.ai token 交换失败: %s", msg)
	}

	jwtToken := strings.TrimSpace(tokenResp.Data.Token)
	accessToken := strings.TrimSpace(tokenResp.Data.Zai.AccessToken)
	if jwtToken == "" && accessToken == "" {
		return nil, fmt.Errorf("token 交换响应缺少 data.token 或 data.zai.access_token")
	}

	var expiresAt time.Time
	if tokenResp.Data.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.Data.ExpiresIn) * time.Second)
	} else {
		expiresAt = time.Now().Add(30 * 24 * time.Hour)
	}

	// 落盘写入账号
	sa, errSave := saveZaiAccount(jwtToken, accessToken, "", tokenResp.Data.User, expiresAt, label)
	if errSave != nil {
		return nil, fmt.Errorf("save account: %w", errSave)
	}
	return sa, nil
}

// extractCodeAndState 从任意输入（完整回调 URL 或单个 code）中提取 code 和 state。
func extractCodeAndState(input string) (string, string) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", ""
	}
	if strings.Contains(raw, "?") || strings.Contains(raw, "&") || strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "zcode://") {
		uStr := raw
		if strings.HasPrefix(uStr, "zcode://") {
			uStr = "http://" + strings.TrimPrefix(uStr, "zcode://")
		}
		if u, err := url.Parse(uStr); err == nil {
			q := u.Query()
			c := q.Get("code")
			if c == "" {
				c = q.Get("authCode")
			}
			s := q.Get("state")
			if c != "" {
				return strings.TrimSpace(c), strings.TrimSpace(s)
			}
		}
	}
	return raw, ""
}

// checkHostCallbackPayload 检查宿主是否在凭据目录写入了 .oauth-zcode-provider-<state>.oauth 回调文件。
func checkHostCallbackPayload(state string) (string, bool) {
	state = strings.TrimSpace(state)
	if state == "" {
		return "", false
	}
	targetFile := ".oauth-" + providerName + "-" + state + ".oauth"
	candidates := []string{
		filepath.Join(getAccountsDir(), targetFile),
		filepath.Join(filepath.Dir(getAccountsDir()), targetFile),
		filepath.Join(os.TempDir(), targetFile),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			_ = os.Remove(p)
			var payload struct {
				Code  string `json:"code"`
				State string `json:"state"`
				Error string `json:"error"`
			}
			if errUnmarshal := json.Unmarshal(data, &payload); errUnmarshal == nil && strings.TrimSpace(payload.Code) != "" {
				return strings.TrimSpace(payload.Code), true
			}
		}
	}
	return "", false
}

// handleStartLogin 处理宿主发起 OAuth 登录请求，生成带唯一 32 字符 hex state 的 Z.ai 官方登录授权链接。
func handleStartLogin(raw []byte) ([]byte, error) {
	var req pluginapi.AuthLoginStartRequest
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}

	state := randomHex(16) // 32 字符十六进制，与官方客户端和标准协议对齐
	session := &oauthLoginSession{
		CreatedAt: time.Now(),
	}
	oauthSessions.Store(state, session)

	authURL := buildZaiAuthorizeURL(state)
	bridgeURL := "/v0/resource/plugins/" + providerName + "/oauth-bridge?state=" + url.QueryEscape(state)
	now := time.Now()
	return okEnvelope(pluginapi.AuthLoginStartResponse{
		Provider:  providerName,
		URL:       authURL, // 打开链接直达 Z.ai 官方登录授权
		State:     state,
		ExpiresAt: now.Add(oauthTTL).UTC(),
		Metadata: map[string]any{
			"logo":       pluginLogoURL,
			"auth_url":   authURL,
			"bridge_url": bridgeURL,
			"prompt":     "请在打开的 Z.ai 页面完成登录授权；完成后复制浏览器地址栏的完整跳转网址（含 code 与 state）粘贴到下方提交。",
		},
	})
}

// handlePollLogin 处理宿主轮询状态，检查该 state 是否已完成授权。
func handlePollLogin(raw []byte) ([]byte, error) {
	var req pluginapi.AuthLoginPollRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("poll: %w", err)
	}
	state := strings.TrimSpace(req.State)
	if state == "" {
		return nil, fmt.Errorf("poll: empty state")
	}

	rawSession, ok := oauthSessions.Load(state)
	if !ok {
		return okEnvelope(pluginapi.AuthLoginPollResponse{
			Status:  pluginapi.AuthLoginStatusError,
			Message: "授权会话不存在或已过期",
		})
	}
	session, ok := rawSession.(*oauthLoginSession)
	if !ok || session == nil {
		return okEnvelope(pluginapi.AuthLoginPollResponse{
			Status:  pluginapi.AuthLoginStatusError,
			Message: "授权会话数据异常",
		})
	}

	if time.Since(session.CreatedAt) > oauthTTL {
		oauthSessions.Delete(state)
		return okEnvelope(pluginapi.AuthLoginPollResponse{
			Status:  pluginapi.AuthLoginStatusError,
			Message: "授权会话已超时（15分钟），请重新发起",
		})
	}

	// 1. 若内存 session 已经完成
	if session.Completed {
		oauthSessions.Delete(state)
		if session.Error != "" {
			return okEnvelope(pluginapi.AuthLoginPollResponse{
				Status:  pluginapi.AuthLoginStatusError,
				Message: session.Error,
			})
		}
		if session.Auth == nil {
			return okEnvelope(pluginapi.AuthLoginPollResponse{
				Status:  pluginapi.AuthLoginStatusError,
				Message: "缺少凭据信息",
			})
		}
		return okEnvelope(pluginapi.AuthLoginPollResponse{
			Status:  pluginapi.AuthLoginStatusSuccess,
			Message: "ZCode (Z.ai) 账号授权成功",
			Auth:    *session.Auth,
		})
	}

	// 2. 检查宿主是否写入了回调载荷文件
	if code, found := checkHostCallbackPayload(state); found {
		stored, errExchange := exchangeZaiOAuthCode(code, state, "")
		if errExchange != nil {
			session.Error = errExchange.Error()
			session.Completed = true
			oauthSessions.Delete(state)
			return okEnvelope(pluginapi.AuthLoginPollResponse{
				Status:  pluginapi.AuthLoginStatusError,
				Message: errExchange.Error(),
			})
		}
		rawStorage, _ := json.Marshal(stored)
		authData := pluginapi.AuthData{
			Provider:    providerName,
			ID:          stored.AuthID,
			FileName:    stored.AuthID + ".json",
			Label:       stored.Label,
			Disabled:    stored.Disabled,
			StorageJSON: rawStorage,
			Metadata: map[string]any{
				"type":     providerName,
				"provider": providerName,
				"logo":     pluginLogoURL,
				"disabled": stored.Disabled,
			},
		}
		session.Auth = &authData
		session.Completed = true
		oauthSessions.Delete(state)
		return okEnvelope(pluginapi.AuthLoginPollResponse{
			Status:  pluginapi.AuthLoginStatusSuccess,
			Message: "ZCode (Z.ai) 账号授权成功",
			Auth:    authData,
		})
	}

	return okEnvelope(pluginapi.AuthLoginPollResponse{
		Status:  pluginapi.AuthLoginStatusPending,
		Message: "等待在 Z.ai 页面完成授权登录并提交回调地址...",
	})
}

// handleRefreshAuth 处理凭据刷新请求。
func handleRefreshAuth(raw []byte) ([]byte, error) {
	var req struct {
		Auth pluginapi.AuthData `json:"auth"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	return okEnvelope(pluginapi.AuthRefreshResponse{
		Auth:             req.Auth,
		NextRefreshAfter: time.Now().Add(15 * 24 * time.Hour),
	})
}

// handleOAuthBridge 处理 /v0/resource/plugins/zcode-provider/oauth-bridge 页面与提交。
func handleOAuthBridge(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	q := req.Query
	state := strings.TrimSpace(q.Get("state"))
	rawCallback := strings.TrimSpace(q.Get("callback_url"))
	rawCode := strings.TrimSpace(q.Get("code"))
	label := strings.TrimSpace(q.Get("label"))

	// 兼容通过 callback_url 传入
	input := rawCallback
	if input == "" {
		input = rawCode
	}

	if state == "" {
		return oauthBridgeHTML("缺少 state 参数", "请从 CLIProxyAPI 管理后台的「OAuth 登录」中点击 ZCode 重新发起。", "", false)
	}

	rawSession, ok := oauthSessions.Load(state)
	if !ok {
		return oauthBridgeHTML("会话已失效", "授权会话不存在或已完成，请回到管理后台重新点击发起登录。", "", false)
	}
	session, ok := rawSession.(*oauthLoginSession)
	if !ok || session == nil {
		return oauthBridgeHTML("会话异常", "会话数据损坏，请重新发起登录。", "", false)
	}

	if time.Since(session.CreatedAt) > oauthTTL {
		oauthSessions.Delete(state)
		return oauthBridgeHTML("会话已超时", "授权会话已超过 15 分钟有效期，请回到管理后台重新发起。", "", false)
	}

	// 用户提交了回调 URL 或 code
	if input != "" {
		code, parsedState := extractCodeAndState(input)
		if parsedState != "" && parsedState != state {
			// 若回调 URL 中携带了新的 state，优先校验
			state = parsedState
		}

		if code == "" {
			return oauthBridgeHTML("授权码无效", "未能从输入中提取到有效的授权码 (code)，请复制完整的跳转网址后重试。", state, false)
		}

		// 向上游兑换 Token 并落盘
		stored, errExchange := exchangeZaiOAuthCode(code, state, label)
		if errExchange != nil {
			session.Error = "授权交换失败: " + errExchange.Error()
			session.Completed = true
			return oauthBridgeHTML("授权失败", "上游令牌交换失败: "+errExchange.Error(), state, false)
		}

		rawStorage, _ := json.Marshal(stored)
		authData := pluginapi.AuthData{
			Provider:    providerName,
			ID:          stored.AuthID,
			FileName:    stored.AuthID + ".json",
			Label:       stored.Label,
			Disabled:    stored.Disabled,
			StorageJSON: rawStorage,
			Metadata: map[string]any{
				"type":     providerName,
				"provider": providerName,
				"logo":     pluginLogoURL,
				"disabled": stored.Disabled,
			},
		}

		session.Auth = &authData
		session.Completed = true

		accountLabel := stored.Label
		if accountLabel == "" {
			accountLabel = stored.AuthID
		}

		return oauthBridgeHTML("授权绑定成功！", "已成功通过 Z.ai 授权绑定账号「"+accountLabel+"」。您可以关闭此窗口，回到 CLIProxyAPI 管理后台查看已完成的授权卡片。", state, true)
	}

	// 首次打开：渲染授权引导表单
	authURL := buildZaiAuthorizeURL(state)
	return oauthBridgeFormHTML(state, authURL)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&#39;")
	return r.Replace(s)
}

func oauthBridgeHTML(title, message, state string, success bool) pluginapi.ManagementResponse {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>ZCode 授权绑定</title>")
	b.WriteString("<style>")
	b.WriteString("body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;background:#0f172a;color:#f8fafc;margin:0;padding:40px 16px;display:flex;justify-content:center;align-items:center;min-height:80vh}")
	b.WriteString(".card{background:#1e293b;border:1px solid #334155;border-radius:12px;box-shadow:0 10px 25px -5px rgba(0,0,0,.5);max-width:540px;width:100%;padding:32px;box-sizing:border-box}")
	b.WriteString("h1{font-size:20px;margin:0 0 16px;display:flex;align-items:center;gap:10px}")
	b.WriteString("p{line-height:1.7;margin:10px 0;font-size:14px;color:#94a3b8}")
	b.WriteString(".btn{display:inline-block;margin-top:16px;background:#3b82f6;color:#fff;border:none;border-radius:6px;padding:10px 20px;font-size:14px;font-weight:500;text-decoration:none;cursor:pointer}")
	b.WriteString(".btn:hover{background:#2563eb}")
	if success {
		b.WriteString(".box{background:rgba(34,197,94,.1);border:1px solid rgba(34,197,94,.3);color:#4ade80;border-radius:8px;padding:16px;margin-top:16px}")
	} else {
		b.WriteString(".box{background:rgba(239,68,68,.1);border:1px solid rgba(239,68,68,.3);color:#f87171;border-radius:8px;padding:16px;margin-top:16px}")
	}
	b.WriteString("</style></head><body><div class=\"card\">")
	b.WriteString("<h1>" + htmlEscape(title) + "</h1>")
	b.WriteString("<div class=\"box\"><p style=\"color:inherit;margin:0;\">" + htmlEscape(message) + "</p></div>")
	if success {
		b.WriteString("<p style=\"margin-top:20px;\">窗口将在 3 秒后尝试自动关闭…</p>")
		b.WriteString("<script>setTimeout(function(){try{window.close();}catch(e){}},3000);</script>")
	} else if state != "" {
		b.WriteString("<a class=\"btn\" href=\"/v0/resource/plugins/zcode-provider/oauth-bridge?state=" + url.QueryEscape(state) + "\">重试提交</a>")
	}
	b.WriteString("</div></body></html>")

	h := http.Header{}
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: []byte(b.String())}
}

func oauthBridgeFormHTML(state, authURL string) pluginapi.ManagementResponse {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>ZCode 账号授权（Z.ai 官方登录）</title>")
	b.WriteString("<style>")
	b.WriteString("body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;background:#0f172a;color:#f8fafc;margin:0;padding:40px 16px;display:flex;justify-content:center;align-items:center;min-height:80vh}")
	b.WriteString(".card{background:#1e293b;border:1px solid #334155;border-radius:12px;box-shadow:0 10px 25px -5px rgba(0,0,0,.5);max-width:560px;width:100%;padding:32px;box-sizing:border-box}")
	b.WriteString(".logo-row{display:flex;align-items:center;gap:12px;margin-bottom:16px}")
	b.WriteString(".logo-img{width:36px;height:36px;border-radius:8px}")
	b.WriteString("h1{font-size:20px;margin:0;font-weight:600}")
	b.WriteString(".desc{color:#94a3b8;font-size:13px;line-height:1.6;margin-bottom:20px}")
	b.WriteString(".step-box{background:#0f172a;border:1px solid #334155;border-radius:8px;padding:16px;margin-bottom:20px;font-size:13px;color:#cbd5e1;line-height:1.6}")
	b.WriteString(".step-btn{display:inline-block;background:#3b82f6;color:#fff;border-radius:6px;padding:9px 18px;font-size:13px;font-weight:500;text-decoration:none;margin-top:10px;transition:background .2s}")
	b.WriteString(".step-btn:hover{background:#2563eb}")
	b.WriteString(".field{margin-bottom:16px}")
	b.WriteString("label{display:block;font-size:13px;color:#cbd5e1;margin-bottom:6px;font-weight:500}")
	b.WriteString("input,textarea{width:100%;box-sizing:border-box;background:#0f172a;border:1px solid #475569;border-radius:6px;padding:10px 12px;color:#f8fafc;font-size:13px;outline:none;transition:border-color .2s;font-family:ui-monospace,monospace}")
	b.WriteString("input:focus,textarea:focus{border-color:#3b82f6}")
	b.WriteString(".btn{width:100%;background:#10b981;color:#fff;border:none;border-radius:6px;padding:12px;font-size:14px;font-weight:600;cursor:pointer;margin-top:8px;transition:background .2s}")
	b.WriteString(".btn:hover{background:#059669}")
	b.WriteString("</style></head><body>")
	b.WriteString("<div class=\"card\">")
	b.WriteString("<div class=\"logo-row\">")
	b.WriteString("<img class=\"logo-img\" src=\"" + htmlEscape(pluginLogoURL) + "\" alt=\"ZCode\">")
	b.WriteString("<h1>ZCode 账号授权（Z.ai 官方登录）</h1>")
	b.WriteString("</div>")
	b.WriteString("<div class=\"desc\">通过 Z.ai 官方 OAuth 授权连接您的 ZCode 账号，授权后系统自动获取令牌并接入调度。</div>")
	b.WriteString("<div class=\"step-box\">")
	b.WriteString("<strong>第 1 步：前往官方授权</strong><br>")
	b.WriteString("点击下方按钮打开 Z.ai 授权页，使用您的手机号/微信完成登录。<br>")
	b.WriteString("<a class=\"step-btn\" href=\"" + htmlEscape(authURL) + "\" target=\"_blank\" rel=\"noopener noreferrer\">打开 Z.ai 授权页面 ↗</a>")
	b.WriteString("</div>")
	b.WriteString("<form method=\"GET\" action=\"/v0/resource/plugins/zcode-provider/oauth-bridge\">")
	b.WriteString("<input type=\"hidden\" name=\"state\" value=\"" + htmlEscape(state) + "\">")
	b.WriteString("<div class=\"field\">")
	b.WriteString("<label for=\"callback_url\"><strong>第 2 步：粘贴授权完成后的网址或授权码 (code)</strong> <span style=\"color:#ef4444\">*</span></label>")
	b.WriteString("<textarea id=\"callback_url\" name=\"callback_url\" rows=\"4\" placeholder=\"例如: https://zcode.z.ai/app/oauth/login?redirect=...&code=xxxxxx&state=... 或直接输入 code 授权码\" required autofocus></textarea>")
	b.WriteString("</div>")
	b.WriteString("<div class=\"field\">")
	b.WriteString("<label for=\"label\">账号备注 (可选)</label>")
	b.WriteString("<input id=\"label\" name=\"label\" type=\"text\" placeholder=\"例如: 我的工作账号\">")
	b.WriteString("</div>")
	b.WriteString("<button type=\"submit\" class=\"btn\">完成授权绑定</button>")
	b.WriteString("</form>")
	b.WriteString("</div></body></html>")

	h := http.Header{}
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: []byte(b.String())}
}

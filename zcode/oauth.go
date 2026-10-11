package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

type oauthLoginSession struct {
	CreatedAt time.Time
	Auth      *pluginapi.AuthData
	Error     string
	Completed bool
}

var (
	oauthSessions sync.Map
	oauthTTL      = 15 * time.Minute
)

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// handleStartLogin 处理宿主发起 OAuth 登录请求，生成带唯一 state 的桥接页面链接。
func handleStartLogin(raw []byte) ([]byte, error) {
	var req pluginapi.AuthLoginStartRequest
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}

	state := fmt.Sprintf("zc-%d-%s", time.Now().UnixNano(), randomHex(6))
	session := &oauthLoginSession{
		CreatedAt: time.Now(),
	}
	oauthSessions.Store(state, session)

	bridgeURL := "/v0/resource/plugins/" + providerName + "/oauth-bridge?state=" + url.QueryEscape(state)
	now := time.Now()
	return okEnvelope(pluginapi.AuthLoginStartResponse{
		Provider:  providerName,
		URL:       bridgeURL,
		State:     state,
		ExpiresAt: now.Add(oauthTTL).UTC(),
		Metadata: map[string]any{
			"logo":       pluginLogoURL,
			"bridge_url": bridgeURL,
			"prompt":     "请在打开的页面中输入或粘贴您的 BigModel Coding Plan API Key 完成授权绑定。",
		},
	})
}

// handlePollLogin 处理宿主轮询状态，检查该 state 是否已在桥接页完成 Key 提交。
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
			Message: "ZCode 账号授权成功",
			Auth:    *session.Auth,
		})
	}

	return okEnvelope(pluginapi.AuthLoginPollResponse{
		Status:  pluginapi.AuthLoginStatusPending,
		Message: "等待在授权页面提交 API Key...",
	})
}

// handleRefreshAuth 处理凭据刷新请求。API Key 为长期有效凭据，直接返回并在 30 天后再次探活。
func handleRefreshAuth(raw []byte) ([]byte, error) {
	var req struct {
		Auth pluginapi.AuthData `json:"auth"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &req)
	}
	return okEnvelope(pluginapi.AuthRefreshResponse{
		Auth:             req.Auth,
		NextRefreshAfter: time.Now().Add(30 * 24 * time.Hour),
	})
}

// handleOAuthBridge 处理 /v0/resource/plugins/zcode-provider/oauth-bridge 页面与提交。
func handleOAuthBridge(req pluginapi.ManagementRequest) pluginapi.ManagementResponse {
	q := req.Query
	state := strings.TrimSpace(q.Get("state"))
	rawKey := strings.TrimSpace(q.Get("api_key"))
	label := strings.TrimSpace(q.Get("label"))

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

	// 用户提交了 API Key
	if rawKey != "" {
		// 支持 KEY----备注 语法
		actualKey := rawKey
		if idx := strings.Index(rawKey, "----"); idx != -1 {
			actualKey = strings.TrimSpace(rawKey[:idx])
			if label == "" {
				label = strings.TrimSpace(rawKey[idx+4:])
			}
		}

		if len(actualKey) < 8 {
			return oauthBridgeHTML("API Key 无效", "输入的 API Key 长度不足，请核对后重试。", state, false)
		}

		if label == "" {
			label = "ZCode " + maskAPIKey(actualKey)
		}

		// 落盘写入账号存储
		stored, errSave := saveAPIKeyAccount(actualKey, label)
		if errSave != nil {
			session.Error = "账号保存失败: " + errSave.Error()
			session.Completed = true
			return oauthBridgeHTML("保存失败", "写入凭据文件失败: "+errSave.Error(), state, false)
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

		return oauthBridgeHTML("授权绑定成功！", "已成功保存账号「"+label+"」。您可以关闭此窗口，回到 CLIProxyAPI 管理后台查看已完成的授权卡片。", state, true)
	}

	// 首次打开：渲染授权引导表单
	return oauthBridgeFormHTML(state)
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
	b.WriteString(".card{background:#1e293b;border:1px solid #334155;border-radius:12px;box-shadow:0 10px 25px -5px rgba(0,0,0,.5);max-width:520px;width:100%;padding:32px;box-sizing:border-box}")
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
		b.WriteString("<a class=\"btn\" href=\"/v0/resource/plugins/zcode-provider/oauth-bridge?state=" + url.QueryEscape(state) + "\">重试输入</a>")
	}
	b.WriteString("</div></body></html>")

	h := http.Header{}
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: []byte(b.String())}
}

func oauthBridgeFormHTML(state string) pluginapi.ManagementResponse {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>ZCode 授权绑定</title>")
	b.WriteString("<style>")
	b.WriteString("body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;background:#0f172a;color:#f8fafc;margin:0;padding:40px 16px;display:flex;justify-content:center;align-items:center;min-height:80vh}")
	b.WriteString(".card{background:#1e293b;border:1px solid #334155;border-radius:12px;box-shadow:0 10px 25px -5px rgba(0,0,0,.5);max-width:540px;width:100%;padding:32px;box-sizing:border-box}")
	b.WriteString(".logo-row{display:flex;align-items:center;gap:12px;margin-bottom:16px}")
	b.WriteString(".logo-img{width:36px;height:36px;border-radius:8px}")
	b.WriteString("h1{font-size:20px;margin:0;font-weight:600}")
	b.WriteString(".desc{color:#94a3b8;font-size:13px;line-height:1.6;margin-bottom:20px}")
	b.WriteString(".guide{background:#0f172a;border:1px solid #334155;border-radius:8px;padding:14px;margin-bottom:20px;font-size:13px;color:#cbd5e1}")
	b.WriteString(".guide-link{color:#60a5fa;text-decoration:none;font-weight:500}")
	b.WriteString(".guide-link:hover{text-decoration:underline}")
	b.WriteString(".field{margin-bottom:16px}")
	b.WriteString("label{display:block;font-size:13px;color:#cbd5e1;margin-bottom:6px;font-weight:500}")
	b.WriteString("input{width:100%;box-sizing:border-box;background:#0f172a;border:1px solid #475569;border-radius:6px;padding:10px 12px;color:#f8fafc;font-size:14px;outline:none;transition:border-color .2s}")
	b.WriteString("input:focus{border-color:#3b82f6}")
	b.WriteString(".btn{width:100%;background:#2563eb;color:#fff;border:none;border-radius:6px;padding:11px;font-size:14px;font-weight:500;cursor:pointer;margin-top:8px;transition:background .2s}")
	b.WriteString(".btn:hover{background:#1d4ed8}")
	b.WriteString("</style></head><body>")
	b.WriteString("<div class=\"card\">")
	b.WriteString("<div class=\"logo-row\">")
	b.WriteString("<img class=\"logo-img\" src=\"" + htmlEscape(pluginLogoURL) + "\" alt=\"ZCode\">")
	b.WriteString("<h1>ZCode 账号授权</h1>")
	b.WriteString("</div>")
	b.WriteString("<div class=\"desc\">将智谱 BigModel Coding Plan 授权接入 CLIProxyAPI，绑定后即可在后台调度使用。</div>")
	b.WriteString("<div class=\"guide\">")
	b.WriteString("1. 前往 <a class=\"guide-link\" href=\"https://bigmodel.cn/usercenter/apikeys\" target=\"_blank\" rel=\"noopener noreferrer\">智谱开放平台 API Keys 页面 ↗</a> 获取 Key。<br>")
	b.WriteString("2. 在下方填入获得的 API Key，点击「确认授权绑定」即可完成。")
	b.WriteString("</div>")
	b.WriteString("<form method=\"GET\" action=\"/v0/resource/plugins/zcode-provider/oauth-bridge\">")
	b.WriteString("<input type=\"hidden\" name=\"state\" value=\"" + htmlEscape(state) + "\">")
	b.WriteString("<div class=\"field\">")
	b.WriteString("<label for=\"api_key\">BigModel API Key <span style=\"color:#ef4444\">*</span></label>")
	b.WriteString("<input id=\"api_key\" name=\"api_key\" type=\"text\" placeholder=\"例如: 607c3xxxxxxxx.xxxxxxxx\" required autofocus autocomplete=\"off\">")
	b.WriteString("</div>")
	b.WriteString("<div class=\"field\">")
	b.WriteString("<label for=\"label\">账号备注 (可选)</label>")
	b.WriteString("<input id=\"label\" name=\"label\" type=\"text\" placeholder=\"例如: 智谱工作账号\">")
	b.WriteString("</div>")
	b.WriteString("<button type=\"submit\" class=\"btn\">确认授权绑定</button>")
	b.WriteString("</form>")
	b.WriteString("</div></body></html>")

	h := http.Header{}
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	return pluginapi.ManagementResponse{StatusCode: http.StatusOK, Headers: h, Body: []byte(b.String())}
}

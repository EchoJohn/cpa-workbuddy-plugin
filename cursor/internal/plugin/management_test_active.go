package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/cursorapi"
	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/cursorauth"
	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/cursorproto"
	"github.com/luode0320/cpa-workbuddy-plugin/cursor/internal/openai"
)

// parseAuthIndexFromRequest 从管理请求中解析 auth_index，优先从 Query 参数提取，向下兼容 URL Path query string。
// [参数] request: 管理 API 请求对象。
// [返回] 解析出的 auth_index 字符串，未指定时返回空字符串。
// 最近修改时间 2026-10-11（修复 Query map[string][]string 与 Path 解析兼容）
func parseAuthIndexFromRequest(request managementRequest) string {
	if vals, ok := request.Query["auth_index"]; ok && len(vals) > 0 {
		if v := strings.TrimSpace(vals[0]); v != "" {
			return v
		}
	}
	if idx := strings.Index(request.Path, "?"); idx >= 0 {
		query, err := url.ParseQuery(request.Path[idx+1:])
		if err == nil {
			return strings.TrimSpace(query.Get("auth_index"))
		}
	}
	return ""
}

// getFirstAvailableCredential 扫描当前所有已加载的 Cursor 认证文件，获取第一个可用凭据。
// [参数] ctx: 上下文。
// [返回] 账号索引、账号凭据及错误信息。
// 最近修改时间 2026-10-11（未指定 auth_index 时自动优雅回退到首个可用账号发现模型）
func (handler *Handler) getFirstAvailableCredential(ctx context.Context) (string, cursorauth.Credentials, error) {
	files, err := handler.cursorAuthFiles(ctx)
	if err != nil || len(files) == 0 {
		return "", cursorauth.Credentials{}, fmt.Errorf("no cursor auth files: %w", err)
	}
	for _, file := range files {
		if cred, err := handler.getCursorCredential(ctx, file.AuthIndex); err == nil && cred.AccessToken != "" {
			return file.AuthIndex, cred, nil
		}
	}
	return "", cursorauth.Credentials{}, errors.New("no valid cursor credentials found")
}

// handleModelsQuery 处理 GET /plugins/cursor-provider/models 请求，返回指定账号或全局可用模型。
// [参数] ctx: 上下文；request: 管理请求对象，包含 Query 或 Path 参数。
// [返回] 统一响应对象，包含 models 列表 JSON；若上游获取失败则优雅降级为默认常用模型。
// 最近修改时间 2026-10-11（修复 Query 解析使账号真实发现模型列表正常返回）
func (handler *Handler) handleModelsQuery(ctx context.Context, request managementRequest) (managementResponse, error) {
	authIndex := parseAuthIndexFromRequest(request)
	var credential cursorauth.Credentials
	var err error
	if authIndex != "" {
		credential, err = handler.getCursorCredential(ctx, authIndex)
	} else {
		authIndex, credential, err = handler.getFirstAvailableCredential(ctx)
	}
	if err != nil {
		return managementJSON(http.StatusOK, map[string]any{"models": defaultCursorModels})
	}
	var models []string
	if handler.cursor != nil {
		models, _ = handler.cursor.DiscoverModels(ctx, credential.AccessToken)
	}
	if len(models) == 0 {
		models = append([]string(nil), defaultCursorModels...)
	}
	filtered := filterDisabledModels(models, credential.DisabledModels)
	if len(filtered) == 0 {
		filtered = append([]string(nil), defaultCursorModels...)
	}
	return managementJSON(http.StatusOK, map[string]any{"models": filtered})
}

// handleTestActive 处理 POST /plugins/cursor-provider/test-active 请求，对指定账号发起手动活跃推理测试。
// [参数] ctx: 上下文；body: JSON 请求体，必须包含 auth_index，可选 model。
// [返回] 测试结果响应，成功时包含 ok: true、model、elapsed 及中文描述，失败时包含错误信息。
// 最近修改时间 2026-10-11（新增面板卡片测试按钮手动探活接口支持）
func (handler *Handler) handleTestActive(ctx context.Context, body []byte) (managementResponse, error) {
	var req struct {
		AuthIndex string `json:"auth_index"`
		Model     string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return managementError(http.StatusBadRequest, "invalid JSON body"), nil
	}
	authIndex := strings.TrimSpace(req.AuthIndex)
	if authIndex == "" {
		return managementError(http.StatusBadRequest, "auth_index is required"), nil
	}
	_, credential, err := handler.getCursorAuth(ctx, authIndex)
	if err != nil {
		return managementError(http.StatusNotFound, fmt.Sprintf("获取凭据失败: %v", err)), nil
	}
	return handler.handleTestActiveWithAuth(ctx, credential, authIndex, req.Model)
}

// errCursorTestActiveFirstChunkDone 标记探活流已收到首个有效输出帧，用于立即终止后续流读取。
var errCursorTestActiveFirstChunkDone = errors.New("cursor: test active first chunk received")

// handleTestActiveWithAuth 使用指定凭据对 Cursor 上游执行真实轻量推理探活。
// [参数] ctx: 上下文；credential: Cursor 认证凭据；authIndex: 账号索引；model: 指定模型名称（可为空）。
// [返回] 管理接口响应体，包含测试往返结果与耗时。
// 最近修改时间 2026-10-11 22:30:00（实现流式首包即停，收到首个正文/思考/完成帧立即返回，消除思考模型整段思维链等待）
func (handler *Handler) handleTestActiveWithAuth(ctx context.Context, credential cursorauth.Credentials, authIndex, model string) (managementResponse, error) {
	if handler.cursor == nil {
		return managementJSON(http.StatusOK, map[string]any{
			"ok":    false,
			"error": "Cursor 客户端未就绪",
		})
	}
	chosenModel := strings.TrimSpace(model)
	if chosenModel == "" {
		chosenModel = pickDefaultCursorModel(credential)
	} else {
		chosenModel = openai.CanonicalModel(chosenModel)
	}

	start := time.Now()
	testCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	input := cursorapi.RunInput{
		AccessToken: credential.AccessToken,
		Model:       chosenModel,
		Prompt:      "User: ping",
		Mode:        cursorproto.FullReplay,
	}

	// 1. 构造首包即停回调：收到首个非空正文、思考片段或完成帧立即返回哨兵终止流读取
	var responseText strings.Builder
	firstChunkSeen := false
	emitFirstChunk := func(event cursorproto.ServerEvent) error {
		switch event.Kind {
		case cursorproto.EventText, cursorproto.EventThinking:
			if strings.TrimSpace(event.Text) != "" {
				responseText.WriteString(event.Text)
				firstChunkSeen = true
				return errCursorTestActiveFirstChunkDone
			}
		case cursorproto.EventToolCall, cursorproto.EventDone:
			firstChunkSeen = true
			return errCursorTestActiveFirstChunkDone
		}
		return nil
	}

	_, err := handler.cursor.Run(testCtx, input, emitFirstChunk)
	if firstChunkSeen || errors.Is(err, errCursorTestActiveFirstChunkDone) {
		err = nil
	}

	elapsed := time.Since(start).Round(time.Millisecond)

	// 2. 若遇到未授权错误且存在 RefreshToken，尝试自动刷新一次并重试
	if err != nil && (strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "unauthorized")) && credential.RefreshToken != "" && handler.auth != nil {
		if refreshed, refreshErr := handler.auth.Refresh(ctx, credential); refreshErr == nil {
			credential = refreshed
			_ = handler.saveRefreshedCredential(ctx, authIndex, refreshed)
			input.AccessToken = refreshed.AccessToken
			firstChunkSeen = false
			startRetry := time.Now()
			_, err = handler.cursor.Run(testCtx, input, emitFirstChunk)
			if firstChunkSeen || errors.Is(err, errCursorTestActiveFirstChunkDone) {
				err = nil
			}
			elapsed = time.Since(startRetry).Round(time.Millisecond)
		}
	}

	// 3. 检查最终执行结果
	if err != nil {
		return managementJSON(http.StatusOK, map[string]any{
			"ok":    false,
			"error": fmt.Sprintf("活跃测试失败 (模型: %s): %v", chosenModel, err),
			"model": chosenModel,
		})
	}

	return managementJSON(http.StatusOK, map[string]any{
		"ok":      true,
		"model":   chosenModel,
		"elapsed": elapsed.String(),
		"message": fmt.Sprintf("活跃成功 (模型: %s, 耗时: %v)", chosenModel, elapsed),
	})
}

// saveRefreshedCredential 尝试把刷新后的凭据持久化写回宿主。
// [参数] ctx: 上下文；authIndex: 账号索引；refreshed: 刷新后的凭据。
// [返回] 错误信息（若有）。
// 最近修改时间 2026-10-11（测试探活自动刷新凭据持久化）
func (handler *Handler) saveRefreshedCredential(ctx context.Context, authIndex string, refreshed cursorauth.Credentials) error {
	if handler.host == nil {
		return nil
	}
	file, _, err := handler.getCursorAuth(ctx, authIndex)
	if err != nil {
		return err
	}
	name := file.Name
	if name == "" {
		name = authFileNameFor(refreshed)
	}
	raw, err := buildCursorAuthFileJSON(refreshed, false, "refreshed via active ping test")
	if err != nil {
		return err
	}
	_, err = handler.host.Call(ctx, "host.auth.save", map[string]any{
		"name": name, "json": json.RawMessage(raw),
	})
	return err
}

// pickDefaultCursorModel 从账号凭据中选取一个可用的默认模型（优先 default/auto/常用高可用模型）。
// [参数] credential: 账号凭据，包含已禁用的模型列表。
// [返回] 选取的默认模型名称。
// 最近修改时间 2026-10-11（对齐官方 default 基准与常用推荐优先级）
func pickDefaultCursorModel(credential cursorauth.Credentials) string {
	disabled := normalizedModelSet(credential.DisabledModels)
	for _, preferred := range []string{"default", "auto", "cursor-fast", "claude-3-5-sonnet", "gpt-4o"} {
		if _, isBlocked := disabled[preferred]; !isBlocked {
			return preferred
		}
	}
	for _, m := range defaultCursorModels {
		if _, isBlocked := disabled[m]; !isBlocked {
			return m
		}
	}
	return "default"
}

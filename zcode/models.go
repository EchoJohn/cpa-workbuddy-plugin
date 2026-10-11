package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/luode0320/cpa-workbuddy-plugin/zcode/internal/mimic"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

var defaultModels = []string{
	"GLM-5.3",
	"GLM-5.3-Flash",
	"GLM-4-Plus",
	"GLM-4-Air",
	"GLM-4-Flash",
}

var (
	modelHTTPClient = &http.Client{Timeout: 15 * time.Second}
	modelCache      struct {
		sync.RWMutex
		byKey     map[string][]string
		expiresAt map[string]time.Time
	}
)

func init() {
	modelCache.byKey = make(map[string][]string)
	modelCache.expiresAt = make(map[string]time.Time)
}

type upstreamModelListResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// fetchUpstreamModels 使用指定的 API Key 请求上游 /v1/models。
func fetchUpstreamModels(ctx context.Context, apiKey string) ([]string, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return append([]string(nil), defaultModels...), nil
	}

	cfg := currentConfig()
	modelCache.RLock()
	if exp, ok := modelCache.expiresAt[apiKey]; ok && time.Now().Before(exp) {
		if list, found := modelCache.byKey[apiKey]; found && len(list) > 0 {
			defer modelCache.RUnlock()
			return append([]string(nil), list...), nil
		}
	}
	modelCache.RUnlock()

	targets := resolveUpstreamTargets(&StoredAuth{APIKey: apiKey}, currentConfig())
	if len(targets) == 0 {
		return append([]string(nil), defaultModels...), nil
	}
	target := targets[0]

	reqURL := target.URL + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return append([]string(nil), defaultModels...), err
	}

	fp := mimic.New(cfg.Mimic, keyID(target.Credential))
	fp.ApplyRequest(req.Header, target.Credential, mimic.UUID4())
	applyTargetAuth(req.Header, target)

	resp, err := modelHTTPClient.Do(req)
	if err != nil {
		return append([]string(nil), defaultModels...), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return append([]string(nil), defaultModels...), nil
	}

	var parsed upstreamModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return append([]string(nil), defaultModels...), err
	}

	var models []string
	seen := make(map[string]struct{})
	for _, item := range parsed.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			models = append(models, id)
		}
	}

	if len(models) == 0 {
		models = append([]string(nil), defaultModels...)
	}

	ttl := time.Duration(cfg.ModelCacheSecs) * time.Second
	if ttl <= 0 {
		ttl = 300 * time.Second
	}

	modelCache.Lock()
	modelCache.byKey[apiKey] = models
	modelCache.expiresAt[apiKey] = time.Now().Add(ttl)
	modelCache.Unlock()

	return models, nil
}

// authModelsForIndex 获取指定账号（或默认）的可用模型列表。
func authModelsForIndex(authID string) []string {
	authID = strings.TrimSpace(authID)
	accounts, err := listAllAuthFiles()
	if err == nil {
		for _, acc := range accounts {
			if acc.AuthID == authID || authFileNameFor(acc.APIKey) == authID+".json" {
				if len(acc.Models) > 0 {
					return acc.Models
				}
				models, errFetch := fetchUpstreamModels(context.Background(), acc.APIKey)
				if errFetch == nil && len(models) > 0 {
					return models
				}
			}
		}
	}

	cfg := currentConfig()
	if cfg.APIKey != "" {
		models, errFetch := fetchUpstreamModels(context.Background(), cfg.APIKey)
		if errFetch == nil && len(models) > 0 {
			return models
		}
	}
	return append([]string(nil), defaultModels...)
}

// allConfiguredModels 返回所有已配置/已发现的模型 ModelInfo。
func allConfiguredModels() []pluginapi.ModelInfo {
	cfg := currentConfig()
	modelsSet := make(map[string]struct{})
	for _, m := range defaultModels {
		modelsSet[m] = struct{}{}
	}
	for _, m := range cfg.Models {
		if strings.TrimSpace(m) != "" {
			modelsSet[strings.TrimSpace(m)] = struct{}{}
		}
	}

	// 汇总所有账号的模型
	if accounts, err := listAllAuthFiles(); err == nil {
		for _, acc := range accounts {
			for _, m := range acc.Models {
				if strings.TrimSpace(m) != "" {
					modelsSet[strings.TrimSpace(m)] = struct{}{}
				}
			}
		}
	}

	var result []pluginapi.ModelInfo
	for id := range modelsSet {
		result = append(result, pluginapi.ModelInfo{
			ID:                         id,
			Object:                     "model",
			OwnedBy:                    zcodeProviderID,
			DisplayName:                id,
			Name:                       mappedModel(id, cfg),
			SupportedGenerationMethods: []string{"chat"},
			ContextLength:              200000,
			MaxCompletionTokens:        128000,
			UserDefined:                true,
		})
	}
	return result
}

// filterExcludedModels 剔除宿主 oauth-excluded-models 中为 zcode-provider 列出的模型。
// 宿主只在 model.for_auth 的按账号注册路径上替插件过滤（sdk/cliproxy/service_executors.go
// 的 applyExcludedModels），model.static 返回的列表会被原样登记进全局模型目录并参与
// 路由选模（internal/pluginhost/adapters.go 的 RegisterModels 无排除调用），因此插件必须
// 在出口自行过滤；同名裸模型被多个 OAuth 插件同时认领时，这是唯一能把模型从某个服务商
// 摘掉的入口。
// [参数] models: 插件已解析完成的模型列表；host: 宿主随请求传入的配置摘要。
// [返回] 过滤后的模型列表；宿主未配置排除项时原样返回入参。
// 最近修改时间：2026-10-12 02:05:00；改动原因：补齐 zcode 与 workbuddy/qoder-ai/qoderwork/
// traework 一致的 oauth-excluded-models 过滤，使 GLM-5.3 这类多服务商同名模型可被定向摘除。
func filterExcludedModels(models []pluginapi.ModelInfo, host pluginapi.HostConfigSummary) []pluginapi.ModelInfo {
	if len(host.ExcludedModels) == 0 {
		return models
	}
	excluded := host.ExcludedModels[providerName]
	if len(excluded) == 0 {
		for channel, list := range host.ExcludedModels {
			if strings.EqualFold(strings.TrimSpace(channel), providerName) {
				excluded = list
				break
			}
		}
	}
	if len(excluded) == 0 {
		return models
	}
	excludeSet := make(map[string]struct{}, len(excluded))
	for _, m := range excluded {
		excludeSet[strings.ToLower(strings.TrimSpace(m))] = struct{}{}
	}
	// 返回新切片而非 models[:0] 就地压缩：入参可能来自账号模型缓存或
	// allConfiguredModels 的底层数组，就地写回会让缓存永久变成"已过滤列表"。
	out := make([]pluginapi.ModelInfo, 0, len(models))
	for _, m := range models {
		if _, skip := excludeSet[strings.ToLower(m.ID)]; skip {
			continue
		}
		out = append(out, m)
	}
	return out
}

func mappedModel(model string, cfg Config) string {
	if upstream, exists := cfg.ModelMap[model]; exists && strings.TrimSpace(upstream) != "" {
		return strings.TrimSpace(upstream)
	}
	return model
}

func keyID(apiKey string) string {
	if idx := strings.Index(apiKey, "."); idx > 0 {
		return apiKey[:idx]
	}
	return apiKey
}

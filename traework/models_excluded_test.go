// models_excluded_test.go 覆盖宿主 oauth-excluded-models 在 traework 模型出口的过滤语义。
// 多服务商同名裸模型（如 deepseek-v4.1-flash）只能靠该配置把模型从某个 provider 摘除。
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// traeResponseModelIDs 返回响应体里的模型 ID 列表，便于断言过滤结果。
//
// [参数] t: 当前测试；raw: 插件模型出口返回的 envelope JSON。
// [返回] 响应中的模型 ID 切片。
// 最近修改时间：2026-10-11 21:28:00；改动原因：静态与账号级两个出口共用同一断言口径。
func traeResponseModelIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	resp := decodeTraeModelResponse(t, raw)
	ids := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		ids = append(ids, m.ID)
	}
	return ids
}

// staticModelRequestWithExclusions 构造带宿主排除项的 model.static 请求。
//
// [参数] t: 当前测试；excluded: 宿主 oauth-excluded-models 原始形态，nil 表示宿主未配置。
// [返回] 已序列化的静态模型请求。
// 最近修改时间：2026-10-11 21:28:00；改动原因：排除项由宿主配置传入，用例需可控 provider 键形态。
func staticModelRequestWithExclusions(t *testing.T, excluded map[string][]string) []byte {
	t.Helper()
	raw, err := json.Marshal(pluginapi.StaticModelRequest{Host: pluginapi.HostConfigSummary{ExcludedModels: excluded}})
	if err != nil {
		t.Fatalf("marshal static model request: %v", err)
	}
	return raw
}

// authModelRequestWithExclusions 构造带宿主排除项的 model.for_auth 请求。
//
// [参数] t: 当前测试；excluded: 为本插件 provider 配置的排除模型名。
// [返回] 已序列化的账号级模型请求。
// 最近修改时间：2026-10-11 21:21:00；改动原因：账号级出口必须与静态出口遵守同一排除语义。
func authModelRequestWithExclusions(t *testing.T, excluded ...string) []byte {
	t.Helper()
	storageJSON, err := json.Marshal(traeAuth{Token: "test-token", UserID: "test-user"})
	if err != nil {
		t.Fatalf("marshal storage: %v", err)
	}
	raw, err := json.Marshal(pluginapi.AuthModelRequest{
		StorageJSON: storageJSON,
		Host:        pluginapi.HostConfigSummary{ExcludedModels: map[string][]string{providerName: excluded}},
	})
	if err != nil {
		t.Fatalf("marshal auth model request: %v", err)
	}
	return raw
}

// TestHandleModelStaticAppliesHostExclusions 验证 model.static 按宿主排除项摘掉同名模型。
//
// [参数] t: 当前测试。
// [返回] 无。
// 最近修改时间：2026-10-11 21:21:00；改动原因：补齐 traework 与 workbuddy/qoder-ai 一致的排除过滤。
func TestHandleModelStaticAppliesHostExclusions(t *testing.T) {
	resetTraeConfiguredModels(t)
	resetTraeDynamicCache(t)
	storeTraeDynamicModels([]pluginapi.ModelInfo{
		{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", OwnedBy: providerName},
		{ID: "qwen3.8-max", Name: "Qwen3.8 Max", OwnedBy: providerName},
	})

	// 1. 命中排除项 → 同名模型不再出现在插件声明的模型列表里。
	raw, err := handleModelStatic(staticModelRequestWithExclusions(t, map[string][]string{providerName: {"deepseek-v4.1-flash"}}))
	if err != nil {
		t.Fatalf("handleModelStatic: %v", err)
	}
	ids := traeResponseModelIDs(t, raw)
	if len(ids) != 1 || ids[0] != "qwen3.8-max" {
		t.Fatalf("excluded model still advertised: %+v", ids)
	}

	// 2. 宿主未配置排除项 → 列表原样返回，保证配置缺省零行为变化。
	raw, err = handleModelStatic(staticModelRequestWithExclusions(t, nil))
	if err != nil {
		t.Fatalf("handleModelStatic without exclusions: %v", err)
	}
	if ids := traeResponseModelIDs(t, raw); len(ids) != 2 {
		t.Fatalf("unconfigured exclusions must not filter: %+v", ids)
	}

	// 3. provider 键大小写漂移仍生效，避免配置写法差异造成静默不生效。
	raw, err = handleModelStatic(staticModelRequestWithExclusions(t, map[string][]string{strings.ToUpper(providerName): {"QWEN3.8-MAX"}}))
	if err != nil {
		t.Fatalf("handleModelStatic with upper-case channel: %v", err)
	}
	if ids := traeResponseModelIDs(t, raw); len(ids) != 1 || ids[0] != "deepseek-v4.1-flash" {
		t.Fatalf("case-insensitive channel match failed: %+v", ids)
	}
}

// TestHandleModelForAuthAppliesHostExclusions 验证 model.for_auth 同样遵守宿主排除项。
//
// [参数] t: 当前测试。
// [返回] 无。
// 最近修改时间：2026-10-11 21:21:00；改动原因：账号级列表是宿主挑选候选凭据的另一事实源。
func TestHandleModelForAuthAppliesHostExclusions(t *testing.T) {
	resetTraeConfiguredModels(t)
	resetTraeDynamicCache(t)
	// 1. 动态发现不可用时走配置兜底，排除过滤仍要在兜底链末端生效。
	configured := []pluginapi.ModelInfo{
		{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", OwnedBy: providerName},
		{ID: "glm-5.2", Name: "GLM 5.2", OwnedBy: providerName},
	}
	previous := configuredModels
	configuredModels = configured
	t.Cleanup(func() { configuredModels = previous })

	raw, err := handleModelForAuth(authModelRequestWithExclusions(t, "deepseek-v4.1-flash"))
	if err != nil {
		t.Fatalf("handleModelForAuth: %v", err)
	}
	if ids := traeResponseModelIDs(t, raw); len(ids) != 1 || ids[0] != "glm-5.2" {
		t.Fatalf("account-level exclusion failed: %+v", ids)
	}
}

// TestFilterExcludedModelsKeepsCacheIntact 验证过滤不复用入参底层数组写回缓存。
// 就地压缩会让动态缓存永久变成"已过滤列表"，后续未配置排除项的请求拿不到全量模型。
//
// [参数] t: 当前测试。
// [返回] 无。
// 最近修改时间：2026-10-11 21:21:00；改动原因：锁定与兄弟插件同源的 P0 别名污染回归。
func TestFilterExcludedModelsKeepsCacheIntact(t *testing.T) {
	resetTraeDynamicCache(t)
	cached := []pluginapi.ModelInfo{
		{ID: "deepseek-v4.1-flash", Name: "DeepSeek V4.1 Flash", OwnedBy: providerName},
		{ID: "qwen3.8-max", Name: "Qwen3.8 Max", OwnedBy: providerName},
		{ID: "glm-5.2", Name: "GLM 5.2", OwnedBy: providerName},
	}
	storeTraeDynamicModels(cached)

	filtered := filterExcludedModels(cachedTraeDynamicModels(), pluginapi.HostConfigSummary{
		ExcludedModels: map[string][]string{providerName: {"deepseek-v4.1-flash", "GLM-5.2"}},
	})
	if len(filtered) != 1 || filtered[0].ID != "qwen3.8-max" {
		t.Fatalf("filter result = %+v", filtered)
	}

	// 2. 缓存必须仍是全量三项，证明过滤返回的是新切片。
	after := cachedTraeDynamicModels()
	if len(after) != 3 {
		t.Fatalf("dynamic cache corrupted by in-place filter: %+v", after)
	}
}

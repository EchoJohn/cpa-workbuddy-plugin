// models_excluded_test.go 覆盖宿主 oauth-excluded-models 在 zcode 模型出口的过滤语义。
// 多服务商同名裸模型（如 GLM-5.3）只能靠该配置把模型从某个 provider 摘除。
package main

import (
	"encoding/json"
	"strings"
	"testing"

	pluginabi "github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// zcodeResponseModelIDs 返回模型出口响应体里的模型 ID 列表，便于断言过滤结果。
//
// [参数] t: 当前测试；raw: 插件模型出口返回的 envelope JSON。
// [返回] 响应中的模型 ID 切片。
// 最近修改时间：2026-10-12 01:40:00；改动原因：静态与账号级两个出口共用同一断言口径。
func zcodeResponseModelIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var resp pluginapi.ModelResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("unmarshal model response: %v", err)
	}
	ids := make([]string, 0, len(resp.Models))
	for _, m := range resp.Models {
		ids = append(ids, m.ID)
	}
	return ids
}

// zcodeStaticRequestWithExclusions 构造带宿主排除项的 model.static 请求。
//
// [参数] t: 当前测试；excluded: 宿主 oauth-excluded-models 原始形态，nil 表示宿主未配置。
// [返回] 已序列化的静态模型请求。
// 最近修改时间：2026-10-12 01:40:00；改动原因：排除项由宿主配置传入，用例需可控 provider 键形态。
func zcodeStaticRequestWithExclusions(t *testing.T, excluded map[string][]string) []byte {
	t.Helper()
	raw, err := json.Marshal(pluginapi.StaticModelRequest{Host: pluginapi.HostConfigSummary{ExcludedModels: excluded}})
	if err != nil {
		t.Fatalf("marshal static model request: %v", err)
	}
	return raw
}

// zcodeAuthRequestWithExclusions 构造带宿主排除项的 model.for_auth 请求。
//
// [参数] t: 当前测试；excluded: 为本插件 provider 配置的排除模型名。
// [返回] 已序列化的账号级模型请求。
// 最近修改时间：2026-10-12 01:40:00；改动原因：账号级出口必须与静态出口遵守同一排除语义。
func zcodeAuthRequestWithExclusions(t *testing.T, excluded ...string) []byte {
	t.Helper()
	raw, err := json.Marshal(pluginapi.AuthModelRequest{
		AuthID: "nonexistent-auth",
		Host:   pluginapi.HostConfigSummary{ExcludedModels: map[string][]string{providerName: excluded}},
	})
	if err != nil {
		t.Fatalf("marshal auth model request: %v", err)
	}
	return raw
}

// TestZcodeHandleModelStaticAppliesHostExclusions 验证 model.static 按宿主排除项摘掉同名模型。
//
// [参数] t: 当前测试。
// [返回] 无。
// 最近修改时间：2026-10-12 01:40:00；改动原因：补齐 zcode 与兄弟插件一致的排除过滤。
func TestZcodeHandleModelStaticAppliesHostExclusions(t *testing.T) {
	// 1. 命中排除项 → 该模型不再出现在插件声明的模型列表里（GLM-5.3 与 workbuddy 系列同名）。
	raw, err := handleMethod(pluginabi.MethodModelStatic, zcodeStaticRequestWithExclusions(t, map[string][]string{providerName: {"GLM-5.3"}}))
	if err != nil {
		t.Fatalf("handleMethod model.static: %v", err)
	}
	for _, id := range zcodeResponseModelIDs(t, raw) {
		if strings.EqualFold(strings.TrimSpace(id), "GLM-5.3") {
			t.Fatalf("excluded model still advertised: %s", id)
		}
	}

	// 2. 宿主未配置排除项 → 列表原样返回，保证配置缺省零行为变化。
	raw, err = handleMethod(pluginabi.MethodModelStatic, zcodeStaticRequestWithExclusions(t, nil))
	if err != nil {
		t.Fatalf("handleMethod model.static without exclusions: %v", err)
	}
	if ids := zcodeResponseModelIDs(t, raw); len(ids) == 0 {
		t.Fatalf("unconfigured exclusions must not filter: %+v", ids)
	}

	// 3. provider 键大小写漂移仍生效，避免配置写法差异造成静默不生效。
	raw, err = handleMethod(pluginabi.MethodModelStatic, zcodeStaticRequestWithExclusions(t, map[string][]string{strings.ToUpper(providerName): {"glm-5.3"}}))
	if err != nil {
		t.Fatalf("handleMethod model.static with upper-case channel: %v", err)
	}
	for _, id := range zcodeResponseModelIDs(t, raw) {
		if strings.EqualFold(strings.TrimSpace(id), "GLM-5.3") {
			t.Fatalf("case-insensitive channel match failed: %s", id)
		}
	}
}

// TestZcodeHandleModelForAuthAppliesHostExclusions 验证 model.for_auth 同样遵守宿主排除项。
//
// [参数] t: 当前测试。
// [返回] 无。
// 最近修改时间：2026-10-12 01:40:00；改动原因：账号级列表是宿主挑选候选凭据的另一事实源。
func TestZcodeHandleModelForAuthAppliesHostExclusions(t *testing.T) {
	// 无匹配账号时回落到默认清单，排除过滤要在兜底链末端生效。
	raw, err := handleMethod(pluginabi.MethodModelForAuth, zcodeAuthRequestWithExclusions(t, "GLM-5.3"))
	if err != nil {
		t.Fatalf("handleMethod model.for_auth: %v", err)
	}
	for _, id := range zcodeResponseModelIDs(t, raw) {
		if strings.EqualFold(strings.TrimSpace(id), "GLM-5.3") {
			t.Fatalf("account-level exclusion failed: %s", id)
		}
	}
}

// TestZcodeFilterExcludedModelsKeepsInputIntact 验证过滤返回新切片，不就地压缩入参。
// 就地压缩会让调用方持有的底层数组变成"已过滤列表"，后续未配置排除项的请求拿不到全量模型。
//
// [参数] t: 当前测试。
// [返回] 无。
// 最近修改时间：2026-10-12 01:40:00；改动原因：锁定与兄弟插件同源的别名污染回归。
func TestZcodeFilterExcludedModelsKeepsInputIntact(t *testing.T) {
	source := []pluginapi.ModelInfo{
		{ID: "GLM-5.3", OwnedBy: providerName},
		{ID: "GLM-5.3-Flash", OwnedBy: providerName},
		{ID: "GLM-4-Plus", OwnedBy: providerName},
	}

	filtered := filterExcludedModels(source, pluginapi.HostConfigSummary{
		ExcludedModels: map[string][]string{providerName: {"GLM-5.3", "glm-5.3-flash"}},
	})
	if len(filtered) != 1 || filtered[0].ID != "GLM-4-Plus" {
		t.Fatalf("filter result = %+v", filtered)
	}

	// 入参必须仍是全量三项，证明过滤返回的是新切片。
	if len(source) != 3 {
		t.Fatalf("input slice mutated by in-place filter: %+v", source)
	}
}

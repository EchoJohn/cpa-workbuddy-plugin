// models_alias_test.go 覆盖 ZCode 模型别名反向解析、GLM 官方大小写归一化与统一用量流记录格式。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// TestNormalizeOfficialGLMModelID 验证模型标识大小写不敏感归一化为上游 Anthropic 端点识别的权威格式。
func TestNormalizeOfficialGLMModelID(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"glm-5.3", "GLM-5.3"},
		{"GLM-5.3", "GLM-5.3"},
		{"Glm-5.3", "GLM-5.3"},
		{"glm-5.3-flash", "GLM-5.3-Flash"},
		{"GLM-5.3-Flash", "GLM-5.3-Flash"},
		{"glm-4-plus", "GLM-4-Plus"},
		{"glm-4-air", "GLM-4-Air"},
		{"glm-4-flash", "GLM-4-Flash"},
		{"glm-5.2", "GLM-5.2"},
		{"glm-5.1", "GLM-5.1"},
		{"glm-5-turbo", "GLM-5-Turbo"},
		{"glm-custom-test", "GLM-custom-test"},
		{"claude-3-5-sonnet", "claude-3-5-sonnet"}, // 非 glm 模型保持原样
	}

	for _, c := range cases {
		got := normalizeOfficialGLMModelID(c.input)
		if got != c.expected {
			t.Errorf("normalizeOfficialGLMModelID(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

// TestResolveUpstreamModelWithHostAlias 验证宿主 oauth-model-alias 反向还原真实模型 ID。
func TestResolveUpstreamModelWithHostAlias(t *testing.T) {
	host := pluginapi.HostConfigSummary{
		OAuthModelAlias: map[string][]pluginapi.ModelAlias{
			providerName: {
				{Name: "GLM-5.3", Alias: "glm-5.3"},
				{Name: "GLM-5.3-Flash", Alias: "glm-5.3-flash"},
				{Name: "GLM-5.3", Alias: "my-custom-glm"},
			},
		},
	}
	cacheModelAliases(host)

	// 1. 通过别名映射回真实模型
	if got := resolveUpstreamModel("my-custom-glm", nil); got != "GLM-5.3" {
		t.Errorf("resolveUpstreamModel('my-custom-glm') = %q, want 'GLM-5.3'", got)
	}
	// 2. 小写 alias 还原
	if got := resolveUpstreamModel("glm-5.3", nil); got != "GLM-5.3" {
		t.Errorf("resolveUpstreamModel('glm-5.3') = %q, want 'GLM-5.3'", got)
	}
	// 3. 未配置别名但输入小写时，经归一化回退至权威大写
	if got := resolveUpstreamModel("glm-4-plus", nil); got != "GLM-4-Plus" {
		t.Errorf("resolveUpstreamModel('glm-4-plus') = %q, want 'GLM-4-Plus'", got)
	}
}

// TestResolveUpstreamModelWithAuthAttributes 验证账号属性级别名优先级高于宿主别名。
func TestResolveUpstreamModelWithAuthAttributes(t *testing.T) {
	host := pluginapi.HostConfigSummary{
		OAuthModelAlias: map[string][]pluginapi.ModelAlias{
			providerName: {
				{Name: "GLM-5.3", Alias: "fast-model"},
			},
		},
	}
	cacheModelAliases(host)

	// 账号属性覆盖：将 fast-model 映射为 GLM-5.3-Flash
	attrs := map[string]string{
		"oauth-model-alias": `[{"name":"GLM-5.3-Flash","alias":"fast-model"}]`,
	}
	if got := resolveUpstreamModel("fast-model", attrs); got != "GLM-5.3-Flash" {
		t.Errorf("resolveUpstreamModel with attrs = %q, want 'GLM-5.3-Flash'", got)
	}
}

// TestUsageFeedRecord_ValidSchemaAndValues 验证 publishUsage 输出的 NDJSON 包含所有必需字段。
func TestUsageFeedRecord_ValidSchemaAndValues(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "zcode-feed-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	feedPath := filepath.Join(tmpDir, "test-feed.ndjson")
	setUsageFeedPath(feedPath)
	defer setUsageFeedPath("")

	start := time.Now().Add(-500 * time.Millisecond)
	detail := usageTokensDetail{
		InputTokens:         12,
		OutputTokens:        34,
		ReasoningTokens:     20,
		CachedTokens:        5,
		CacheReadTokens:     3,
		CacheCreationTokens: 2,
		TotalTokens:         46,
	}

	publishUsage("zcode-test-123", "ZCode 旅行者2435", "glm-5.3", "GLM-5.3", start, detail, 500, 200, false, "", 120000000)

	// 等待异步写入完成
	time.Sleep(50 * time.Millisecond)

	data, err := os.ReadFile(feedPath)
	if err != nil {
		t.Fatalf("read feed file: %v", err)
	}

	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatalf("feed line is empty")
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(line), &parsed); err != nil {
		t.Fatalf("parse feed json: %v", err)
	}

	// 校验顶层契约字段
	if parsed["source"] != "ZCode 旅行者2435" {
		t.Errorf("source = %v, want 'ZCode 旅行者2435'", parsed["source"])
	}
	if parsed["auth_index"] != "zcode-test-123" {
		t.Errorf("auth_index = %v, want 'zcode-test-123'", parsed["auth_index"])
	}
	if parsed["provider"] != "zcode-provider" {
		t.Errorf("provider = %v, want 'zcode-provider'", parsed["provider"])
	}
	if parsed["model"] != "GLM-5.3" {
		t.Errorf("model = %v, want 'GLM-5.3'", parsed["model"])
	}
	if parsed["alias"] != "glm-5.3" {
		t.Errorf("alias = %v, want 'glm-5.3'", parsed["alias"])
	}

	// 校验 tokens 嵌套结构
	tokens, ok := parsed["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("missing tokens object")
	}
	if int64(tokens["input_tokens"].(float64)) != 12 {
		t.Errorf("tokens.input_tokens = %v, want 12", tokens["input_tokens"])
	}
	if int64(tokens["output_tokens"].(float64)) != 34 {
		t.Errorf("tokens.output_tokens = %v, want 34", tokens["output_tokens"])
	}
	if int64(tokens["total_tokens"].(float64)) != 46 {
		t.Errorf("tokens.total_tokens = %v, want 46", tokens["total_tokens"])
	}
}

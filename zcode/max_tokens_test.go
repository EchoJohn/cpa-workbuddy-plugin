package main

import (
	"encoding/json"
	"testing"
)

// TestComputeAdaptiveMaxTokens_ZCode 验证 zcode 自适应 max_tokens 计算。
func TestComputeAdaptiveMaxTokens_ZCode(t *testing.T) {
	// 1. 未传 max_tokens (0)：GLM-5.3 默认给常规 16384
	if got := computeAdaptiveMaxTokens("glm-5.3", 0, false); got != 16384 {
		t.Fatalf("glm-5.3 unconfigured non-reasoning = %d, want 16384", got)
	}

	// 2. 未传 max_tokens (0) 且为推理模式：GLM-5.3 补足至 ReasoningMax 128000
	if got := computeAdaptiveMaxTokens("glm-5.3", 0, true); got != 128000 {
		t.Fatalf("glm-5.3 unconfigured reasoning = %d, want 128000", got)
	}

	// 3. 客户端保守传入 8192 且为推理模式：放宽至 128000
	if got := computeAdaptiveMaxTokens("glm-5.3", 8192, true); got != 128000 {
		t.Fatalf("glm-5.3 8192 reasoning = %d, want 128000", got)
	}

	// 4. 超出上限 (200000)：截断回 128000
	if got := computeAdaptiveMaxTokens("glm-5.3", 200000, true); got != 128000 {
		t.Fatalf("glm-5.3 200000 = %d, want 128000", got)
	}
}

// TestMutateModel_AdaptiveMaxTokens 验证 mutateModel 会就地重写自适应 max_tokens。
func TestMutateModel_AdaptiveMaxTokens(t *testing.T) {
	req := map[string]any{
		"model":      "glm-5.3",
		"max_tokens": 4096,
		"messages": []map[string]any{
			{"role": "user", "content": "hi"},
		},
		"reasoning_effort": "high",
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	mutated := mutateModel(raw, "GLM-5.3")
	var parsed map[string]any
	if err := json.Unmarshal(mutated, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if parsed["model"] != "GLM-5.3" {
		t.Fatalf("model = %v, want GLM-5.3", parsed["model"])
	}

	mt, ok := parsed["max_tokens"].(float64)
	if !ok || int64(mt) < 32768 {
		t.Fatalf("max_tokens = %v, want >= 32768", parsed["max_tokens"])
	}
}

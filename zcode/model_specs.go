// model_specs.go 维护 ZCode 插件权威模型规格表与自适应 max_tokens 计算。
package main

import (
	"fmt"
	"strings"
)

// ModelOfficialSpec 定义模型官方规格参数。
type ModelOfficialSpec struct {
	ContextWindow   int64 // 官方最大上下文长度
	MaxOutputTokens int64 // 官方常规最大输出 token
	ReasoningMax    int64 // 官方思维链/推理模式下的最大输出 token
}

// 常见主流模型的权威官方规格表（特别强化智谱 GLM 家族）。
var officialModelSpecs = map[string]ModelOfficialSpec{
	// GLM 智谱家族
	"glm-5.3":       {ContextWindow: 1048576, MaxOutputTokens: 16384, ReasoningMax: 128000},
	"glm-5.3-flash": {ContextWindow: 1048576, MaxOutputTokens: 16384, ReasoningMax: 128000},
	"glm-5.2":       {ContextWindow: 200000, MaxOutputTokens: 16384, ReasoningMax: 64000},
	"glm-4-plus":    {ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 16384},
	"glm-4-air":     {ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 16384},
	"glm-4-flash":   {ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 16384},
	"glm-4.5":       {ContextWindow: 131072, MaxOutputTokens: 16384, ReasoningMax: 64000},

	// 通用兜底
	"deepseek-reasoner": {ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 64000},
	"deepseek-chat":     {ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 8192},
}

// resolveOfficialModelSpec 根据模型名称匹配官方规格，未精确匹配时按前缀/族系泛化。
func resolveOfficialModelSpec(model string) ModelOfficialSpec {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return ModelOfficialSpec{ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 8192}
	}
	if spec, ok := officialModelSpecs[m]; ok {
		return spec
	}
	for k, spec := range officialModelSpecs {
		if strings.HasPrefix(m, k) || strings.Contains(m, k) {
			return spec
		}
	}
	switch {
	case strings.Contains(m, "glm-5"):
		return ModelOfficialSpec{ContextWindow: 1048576, MaxOutputTokens: 16384, ReasoningMax: 128000}
	case strings.Contains(m, "glm-4"):
		return ModelOfficialSpec{ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 16384}
	case strings.Contains(m, "reasoner") || strings.Contains(m, "thinking"):
		return ModelOfficialSpec{ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 64000}
	default:
		return ModelOfficialSpec{ContextWindow: 131072, MaxOutputTokens: 8192, ReasoningMax: 8192}
	}
}

// computeAdaptiveMaxTokens 核心自适应计算算法：
// 1. 未传或 <= 0：自适应补齐为官方常规最大输出 token (如果是推理模型则为 ReasoningMax)。
// 2. 推理模型且客户端保守传入 <= 8192：自适应放宽至官方 ReasoningMax。
// 3. 超出官方上限：截断至官方支持的上限。
// 4. 处于合法区间内：保留客户端原值。
func computeAdaptiveMaxTokens(model string, requestedTokens int64, isReasoning bool) int64 {
	spec := resolveOfficialModelSpec(model)
	targetCap := spec.MaxOutputTokens
	if isReasoning && spec.ReasoningMax > targetCap {
		targetCap = spec.ReasoningMax
	}
	if targetCap <= 0 {
		targetCap = 8192
	}

	if requestedTokens <= 0 {
		return targetCap
	}

	if isReasoning && requestedTokens <= 8192 && targetCap > requestedTokens {
		return targetCap
	}

	if requestedTokens > targetCap {
		return targetCap
	}

	return requestedTokens
}

// relaxMaxTokensInPlace 对传入的 JSON payload map 执行原地自适应 max_tokens 优化。
func relaxMaxTokensInPlace(obj map[string]any) bool {
	model := jsonStr(obj, "model")
	effort := strings.ToLower(strings.TrimSpace(jsonStr(obj, "reasoning_effort")))
	isReasoning := effort != "" && effort != "none"
	if !isReasoning {
		mLower := strings.ToLower(model)
		if strings.Contains(mLower, "reasoner") || strings.Contains(mLower, "thinking") {
			isReasoning = true
		}
	}

	val := jsonI64(obj, "max_tokens", "max_completion_tokens")
	finalTokens := computeAdaptiveMaxTokens(model, val, isReasoning)

	if finalTokens != val || obj["max_tokens"] == nil {
		obj["max_tokens"] = finalTokens
		if _, hasMCT := obj["max_completion_tokens"]; hasMCT {
			delete(obj, "max_completion_tokens")
		}
		return true
	}
	return false
}

func jsonI64(m map[string]any, keys ...string) int64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return int64(t)
			case int64:
				return t
			case string:
				var n int64
				fmt.Sscanf(t, "%d", &n)
				return n
			}
		}
	}
	return 0
}

func jsonStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok {
			return s
		}
	}
	return ""
}

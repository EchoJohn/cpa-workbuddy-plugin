package main

import (
	"encoding/json"
	"testing"
)

// TestRelaxMaxTokensInPlace 验证输出 token 放宽至 128k 的核心规则：
// 1. 客户端写死保守值（<=8192）提升为 128000
// 2. 未设置 max_tokens 时补足 128000
// 3. 显式大于 128000 时原样保留
// 4. max_completion_tokens 小于 128000 时提升并归一化
// 最近修改时间：2026-10-13 01:25:00；改动原因：长思维链吃满 8192 上限截断缺陷治理。
func TestRelaxMaxTokensInPlace(t *testing.T) {
	cases := []struct {
		name        string
		input       map[string]any
		wantTokens  int64
		wantHasMCT  bool
		wantChanged bool
	}{
		{
			name:        "Qoder 客户端写死 8192 提升至 128000",
			input:       map[string]any{"max_tokens": float64(8192)},
			wantTokens:  128000,
			wantHasMCT:  false,
			wantChanged: true,
		},
		{
			name:        "未传 max_tokens 补足至 128000",
			input:       map[string]any{"messages": []any{}},
			wantTokens:  128000,
			wantHasMCT:  false,
			wantChanged: true,
		},
		{
			name:        "较小自定义值 4096 提升至 128000",
			input:       map[string]any{"max_tokens": float64(4096)},
			wantTokens:  128000,
			wantHasMCT:  false,
			wantChanged: true,
		},
		{
			name:        "显式超大值 200000 保持不变",
			input:       map[string]any{"max_tokens": float64(200000)},
			wantTokens:  200000,
			wantHasMCT:  false,
			wantChanged: false,
		},
		{
			name:        "max_completion_tokens 8192 归一化为 max_tokens 128000",
			input:       map[string]any{"max_completion_tokens": float64(8192)},
			wantTokens:  128000,
			wantHasMCT:  false,
			wantChanged: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := relaxMaxTokensInPlace(tc.input)
			if changed != tc.wantChanged {
				t.Fatalf("changed = %v, want %v", changed, tc.wantChanged)
			}
			got := jsonI64(tc.input, "max_tokens")
			if got != tc.wantTokens {
				t.Fatalf("max_tokens = %d, want %d", got, tc.wantTokens)
			}
			_, hasMCT := tc.input["max_completion_tokens"]
			if hasMCT != tc.wantHasMCT {
				t.Fatalf("has max_completion_tokens = %v, want %v", hasMCT, tc.wantHasMCT)
			}
		})
	}
}

// TestPrepareUpstreamBody_RelaxesMaxTokens 验证 prepareUpstreamBody 端到端放宽行为。
// [参数] t: 测试上下文
// [返回] 无
// 最近修改时间：2026-10-13 01:25:00；改动原因：验证流式请求体最终包含放宽后的 128k 上限。
func TestPrepareUpstreamBody_RelaxesMaxTokens(t *testing.T) {
	in := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"}],"max_tokens":8192}`)
	out := prepareUpstreamBody(in, nil, nil, "deepseek-v4.1-flash")

	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if got := jsonI64(obj, "max_tokens"); got != 128000 {
		t.Fatalf("max_tokens = %d, want 128000; raw: %s", got, string(out))
	}
}

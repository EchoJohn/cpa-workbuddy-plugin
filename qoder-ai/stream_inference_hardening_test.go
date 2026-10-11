// stream_inference_hardening_test.go 锁定推理请求加固的核心行为：
// 1. parseQoderSSEChunk 对多种 [DONE] 终止形态与扁平 OpenAI chunk 的识别契约；
// 2. buildQoderBody 截断 prompt 时不破坏多字节 UTF-8 字符；
// 3. extractLatestUserPrompt 面对非 user 消息时的健壮兜底。
package main

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestParseQoderSSEChunk_DoneVariants 验证多种协议形态的 [DONE] 终止符均被即时识别并触发 isDone=true。
func TestParseQoderSSEChunk_DoneVariants(t *testing.T) {
	cases := []struct {
		name     string
		line     string
		wantDone bool
	}{
		{"bare done line", "data: [DONE]", true},
		{"compact done line", "data:[DONE]", true},
		{"done with extra spaces", "data:   [DONE]  ", true},
		{"nested envelope done", `data:{"body":"[DONE]"}`, true},
		{"nested envelope data done", `data:{"body":"data: [DONE]"}`, true},
		{"nested envelope with newline", "data:{\"body\":\"[DONE]\\n\"}", true},
		{"heartbeat comment", ": keep-alive", false},
		{"empty line", "", false},
		{"whitespace line", "   ", false},
	}
	for _, tc := range cases {
		_, isDone := parseQoderSSEChunk(tc.line)
		if isDone != tc.wantDone {
			t.Fatalf("%s: line %q, got isDone=%v, want %v", tc.name, tc.line, isDone, tc.wantDone)
		}
	}
}

// TestParseQoderSSEChunk_FlatOpenAIChunk 验证扁平 OpenAI SSE 格式（非嵌套 body 结构）兼容识别。
func TestParseQoderSSEChunk_FlatOpenAIChunk(t *testing.T) {
	flat := `data: {"id":"chatcmpl-1","choices":[{"index":0,"delta":{"content":"hello"}}]}`
	chunk, isDone := parseQoderSSEChunk(flat)
	if isDone {
		t.Fatal("flat chunk must not be marked done")
	}
	if !strings.Contains(chunk, "hello") || !strings.Contains(chunk, "chatcmpl-1") {
		t.Fatalf("flat chunk parsing failed: %s", chunk)
	}
}

// TestBuildQoderBody_UTF8CutSafe 验证长 prompt 截断 business.name 时不破坏多字节 UTF-8 字符。
func TestBuildQoderBody_UTF8CutSafe(t *testing.T) {
	// 5 字节英文 + 9 个 3 字节汉字（27 字节）= 32 字节。
	// 若按字节切 [:30]，第 30 字节落在第 9 个汉字的第一字节，制造出非法 UTF-8。
	prompt := "Hello你好世界测试一段比较长的中文消息"
	req := &openAIRequest{
		Model: "qoder/auto",
		Messages: []openAIMessage{
			{Role: "user", Content: prompt},
		},
	}
	body, err := buildQoderBody(req, "auto", "")
	if err != nil {
		t.Fatalf("buildQoderBody failed: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("unmarshal produced body failed: %v", err)
	}
	biz, ok := obj["business"].(map[string]any)
	if !ok {
		t.Fatal("missing business object")
	}
	name, _ := biz["name"].(string)
	if !utf8.ValidString(name) {
		t.Fatalf("business.name contains invalid UTF-8 sequence: %q", name)
	}
	if len([]rune(name)) > 30 {
		t.Fatalf("business.name rune length exceeded 30: %d", len([]rune(name)))
	}
}

// TestExtractLatestUserPrompt_Fallback 验证无显式 user 消息时的健壮兜底。
func TestExtractLatestUserPrompt_Fallback(t *testing.T) {
	// 场景 1：包含 user 消息
	msgs1 := []openAIMessage{
		{Role: "system", Content: "you are a helper"},
		{Role: "user", Content: "test prompt"},
	}
	if got := extractLatestUserPrompt(msgs1); got != "test prompt" {
		t.Fatalf("user message not returned: got %q", got)
	}

	// 场景 2：仅有 system 消息（无 user 消息），应优雅降级返回 system 内容而非引发错误
	msgs2 := []openAIMessage{
		{Role: "system", Content: "autonomous system instruction"},
	}
	if got := extractLatestUserPrompt(msgs2); got != "autonomous system instruction" {
		t.Fatalf("fallback to system message failed: got %q", got)
	}

	// 场景 3：全部为空，应兜底返回 continue
	msgs3 := []openAIMessage{}
	if got := extractLatestUserPrompt(msgs3); got != "continue" {
		t.Fatalf("empty fallback failed: got %q", got)
	}
}

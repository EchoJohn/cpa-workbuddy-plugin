package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Model_identity_normalizes_prefix_whitespace_before_session_keying(t *testing.T) {
	for _, model := range []string{"auto", "cursor/auto", "cursor/ auto", " cursor/ auto "} {
		raw, err := json.Marshal(map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hello"}}})
		require.NoError(t, err)

		chat, err := ParseChatRequest(raw)

		require.NoError(t, err)
		require.Equal(t, "auto", chat.Model)
	}
}

func Test_CanonicalModel_normalizes_dotted_and_alias_variants(t *testing.T) {
	cases := map[string]string{
		"claude-3.5-sonnet":         "claude-3-5-sonnet",
		"cursor/claude-3.5-sonnet":  "claude-3-5-sonnet",
		"claude-3.5-sonnet-latest":  "claude-3-5-sonnet",
		"sonnet-3.5":                "claude-3-5-sonnet",
		"claude-3.5-haiku":          "claude-3-5-haiku",
		"cursor/claude-3.5-haiku":   "claude-3-5-haiku",
		"claude-3.7-sonnet":         "claude-3-7-sonnet",
		"claude-3.7-sonnet-thought": "claude-3-7-sonnet-thought",
		"claude-3-opus":             "claude-3-opus",
		"gpt-4.5":                   "gpt-4.5-preview",
		"cursor-small":              "cursor-fast",
		"default":                   "default",
		"auto":                      "auto",
		"cursor-fast":               "cursor-fast",
	}
	for input, expected := range cases {
		require.Equal(t, expected, CanonicalModel(input))

		// 验证经由 ParseChatRequest 同样正确归一化
		raw, err := json.Marshal(map[string]any{"model": input, "messages": []map[string]string{{"role": "user", "content": "ping"}}})
		require.NoError(t, err)
		chat, err := ParseChatRequest(raw)
		require.NoError(t, err)
		require.Equal(t, expected, chat.Model)
	}
}

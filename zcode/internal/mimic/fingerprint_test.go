package mimic

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func TestFingerprint_ApplyRequest(t *testing.T) {
	cfg := DefaultConfig()
	fp := New(cfg, "testkey123")

	h := make(http.Header)
	fp.ApplyRequest(h, "testkey123.secret", "req-1")

	if h.Get("x-api-key") != "testkey123.secret" {
		t.Fatalf("expected x-api-key, got %s", h.Get("x-api-key"))
	}
	if h.Get("http-referer") != "https://zcode.z.ai" {
		t.Fatalf("expected referer, got %s", h.Get("http-referer"))
	}
	if h.Get("User-Agent") != "ZCode/unknown" {
		t.Fatalf("expected User-Agent, got %s", h.Get("User-Agent"))
	}
	if h.Get("X-ZCode-App-Version") != "unknown" {
		t.Fatalf("expected X-ZCode-App-Version unknown, got %s", h.Get("X-ZCode-App-Version"))
	}
	if h.Get("x-zcode-agent") != "glm" {
		t.Fatalf("expected agent glm, got %s", h.Get("x-zcode-agent"))
	}
	if h.Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("expected anthropic-version, got %s", h.Get("anthropic-version"))
	}
	if h.Get("x-request-id") != "req-1" {
		t.Fatalf("expected request id req-1, got %s", h.Get("x-request-id"))
	}
	if h.Get("x-query-id") == "" {
		t.Fatalf("expected query id to be non-empty")
	}
	if h.Get("x-session-id") == "" {
		t.Fatalf("expected session id to be non-empty")
	}
}

func TestFingerprint_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = false
	fp := New(cfg, "testkey")

	h := make(http.Header)
	fp.ApplyRequest(h, "my-api-key", "req-2")

	if h.Get("x-api-key") != "my-api-key" {
		t.Fatalf("expected x-api-key, got %s", h.Get("x-api-key"))
	}
	if h.Get("User-Agent") != "" {
		t.Fatalf("expected empty User-Agent when disabled, got %s", h.Get("User-Agent"))
	}
}

func TestFingerprint_AppVersionMustNotBeConcreteRelease(t *testing.T) {
	// 回归守卫：X-ZCode-App-Version 一旦被改成具体版本号（如 3.14.4），
	// 上游 zcode-plan 端点会以 405 + code 3012 风控拦截，导致模型测试全部失败。
	// 详见 DefaultConfig 中 AppVersion 的线上实测说明。
	cfg := DefaultConfig()
	fp := New(cfg, "k")
	h := make(http.Header)
	fp.ApplyRequest(h, "key", "req")

	if got := h.Get("X-ZCode-App-Version"); got != "unknown" {
		t.Fatalf("X-ZCode-App-Version 必须是上游放行值 unknown，当前为 %q；"+
			"改成具体版本号会触发 3012 风控", got)
	}
	if matched, _ := regexp.MatchString(`^\d+\.\d+`, h.Get("X-ZCode-App-Version")); matched {
		t.Fatalf("X-ZCode-App-Version 不得是语义化版本号，当前为 %q", h.Get("X-ZCode-App-Version"))
	}
}

func TestUUID_Formats(t *testing.T) {
	u4 := UUID4()
	parts := strings.Split(u4, "-")
	if len(parts) != 5 {
		t.Fatalf("expected 5 uuid parts, got %d for %s", len(parts), u4)
	}

	u7 := UUID7()
	parts7 := strings.Split(u7, "-")
	if len(parts7) != 5 {
		t.Fatalf("expected 5 uuid parts, got %d for %s", len(parts7), u7)
	}
}

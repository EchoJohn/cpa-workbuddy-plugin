// stream_queue_frame_test.go 锁定上游 HTTP 200 顶层 code 排队帧的识别契约。
//
// 生产证据（2026-10-12）：阿里 Qoder 对超上下文请求投入排队，在 HTTP 200 下
// 返回 `data: {"code":"403","message":"{\"code\":\"10605\",\"message\":
// \"{\\\"isQueued\\\":true,...}\"}"}`，随后保持 Keep-Alive 心跳约 60 秒才由对端
// 复位。旧解析器不识别该形态，将其当作无关行丢弃 → 插件死等 60 秒、客户端零字节。
//
// 本文件覆盖：解析器识别顶层 code 帧、错误帧提取（含内嵌 message 逐层展开）、
// 聚合路径 fail-fast 且不再死等 Keep-Alive，以及「排队帧不触发换号」的判定边界。
package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

// queueFramePayload 是生产实测的排队帧 JSON（与 CPA 日志逐字一致）。
const queueFramePayload = `{"code":"403","message":"{\"code\":\"10605\",\"message\":\"{\\\"isQueued\\\":true,\\\"modelKey\\\":\\\"qfmodel\\\",\\\"queueCount\\\":0,\\\"queueType\\\":\\\"p3\\\",\\\"retryAfterSeconds\\\":30,\\\"serviceAvailable\\\":false,\\\"waitTime\\\":30}\"}"}`

// hangingReader 先吐出 prefix 字节，随后在 EOF 前阻塞 blockFor，模拟上游
// Keep-Alive 长连接不主动关闭。用于证明解析路径能在读到错误帧后立即退出，
// 而不是继续读到连接空闲超时。
type hangingReader struct {
	prefix   []byte
	off      int
	blockFor time.Duration
	blocked  bool
}

func (r *hangingReader) Read(p []byte) (int, error) {
	if r.off < len(r.prefix) {
		n := copy(p, r.prefix[r.off:])
		r.off += n
		return n, nil
	}
	if !r.blocked {
		r.blocked = true
		time.Sleep(r.blockFor)
	}
	return 0, io.EOF
}

// TestParseQoderSSEChunk_TopLevelCodeFrame 验证顶层 code 帧不再被丢弃，
// 且正常 chunk / 心跳 / [DONE] 的既有语义不受影响。
func TestParseQoderSSEChunk_TopLevelCodeFrame(t *testing.T) {
	// 1. 排队帧必须被识别并原样返回（交 sseErrorFrame 提取语义）
	chunk, isDone := parseQoderSSEChunk("data: " + queueFramePayload)
	if isDone {
		t.Fatal("queue frame must not be treated as stream done")
	}
	if !strings.Contains(chunk, "10605") {
		t.Fatalf("queue frame payload not returned: %q", chunk)
	}

	// 2. 无 message 的纯 code 帧也要被识别
	if got, _ := parseQoderSSEChunk(`data: {"code":"500"}`); !strings.Contains(got, `"code":"500"`) {
		t.Fatalf("code-only frame not recognized: %q", got)
	}

	// 3. 正常 chunk 仍走 choices 分支（不被误判为错误帧）
	normal := `data: {"id":"1","choices":[{"index":0,"delta":{"content":"hi"}}]}`
	if got, _ := parseQoderSSEChunk(normal); !strings.Contains(got, "hi") {
		t.Fatalf("normal chunk broken: %q", got)
	}

	// 4. 心跳与空行仍返回空
	if got, done := parseQoderSSEChunk(": keep-alive"); got != "" || done {
		t.Fatalf("heartbeat must be ignored, got %q done=%v", got, done)
	}

	// 5. 空 code 不触发识别（防误伤）
	if got, _ := parseQoderSSEChunk(`data: {"code":""}`); got != "" {
		t.Fatalf("empty code must not be treated as error frame: %q", got)
	}
}

// TestSSEErrorFrame_TopLevelCodeFrame 验证顶层 code 帧的语义提取，
// 含内嵌 JSON message 的逐层展开，且不影响既有 error 帧契约。
func TestSSEErrorFrame_TopLevelCodeFrame(t *testing.T) {
	got := sseErrorFrame(queueFramePayload)
	if !strings.HasPrefix(got, "403: ") {
		t.Fatalf("expected 403 prefix, got %q", got)
	}
	if !strings.Contains(got, "10605") || !strings.Contains(got, "isQueued") {
		t.Fatalf("nested message not unwrapped: %q", got)
	}

	// 纯 code 无 message：只回 code
	if got := sseErrorFrame(`{"code":"500"}`); got != "500" {
		t.Fatalf("code-only frame = %q, want 500", got)
	}

	// 既有 error 帧契约不变
	if got := sseErrorFrame(`{"error":{"code":"14018","message":"quota"}}`); got != "14018: quota" {
		t.Fatalf("legacy error frame broken: %q", got)
	}
	// 正常 chunk 不误报
	if got := sseErrorFrame(`{"id":"1","choices":[{"index":0,"delta":{"content":"hi"}}]}`); got != "" {
		t.Fatalf("normal chunk false positive: %q", got)
	}
	// 顶层 code 但 code 为空串：不识别
	if got := sseErrorFrame(`{"code":"","message":"x"}`); got != "" {
		t.Fatalf("empty code false positive: %q", got)
	}
}

// TestUnwrapNestedMessage 验证内嵌 JSON message 的逐层展开与边界（保留首层业务码）。
func TestUnwrapNestedMessage(t *testing.T) {
	// 双层嵌套：保留内层业务码 10605，最终拼成 "code: 描述"
	two := `{"code":"10605","message":"{\"isQueued\":true}"}`
	if got := unwrapNestedMessage(two); got != `10605: {"isQueued":true}` {
		t.Fatalf("two-level unwrap = %q", got)
	}
	// 非 JSON 原样返回
	if got := unwrapNestedMessage("plain text"); got != "plain text" {
		t.Fatalf("plain text mangled: %q", got)
	}
	// 只有 code 没有 message 时停在当前层（不 panic、不死循环）
	if got := unwrapNestedMessage(`{"code":"10605"}`); got != `10605: {"code":"10605"}` {
		t.Fatalf("code-only unwrap = %q", got)
	}
}

// TestAggregateQoderSSE_QueueFrameFailsFast 是核心回归：HTTP 200 下先到排队帧、
// 随后连接 Keep-Alive 挂起 10 秒时，聚合必须在读到错误帧后立即返回错误
// （远快于挂起时长），而不是死等到连接空闲超时。
func TestAggregateQoderSSE_QueueFrameFailsFast(t *testing.T) {
	body := "data: " + queueFramePayload + "\n\n"
	r := &hangingReader{prefix: []byte(body), blockFor: 10 * time.Second}

	start := time.Now()
	_, err := aggregateQoderSSE(r, "qwen3.8-flash", 200)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("queue frame must fail the completion, got nil")
	}
	if !strings.Contains(err.Error(), "upstream 200:") || !strings.Contains(err.Error(), "10605") {
		t.Fatalf("error = %v, want upstream 200 prefixed queue message", err)
	}
	// 修复前会读到 EOF（10s 挂起）；修复后应在毫秒级返回。
	if elapsed > 2*time.Second {
		t.Fatalf("fail-fast violated: took %v, expected < 2s (hangingReader blocks 10s)", elapsed)
	}
}

// TestAggregateQoderSSE_HealthyStreamUnaffected 验证正常流不受排队帧识别影响。
func TestAggregateQoderSSE_HealthyStreamUnaffected(t *testing.T) {
	sse := `data: {"id":"1","choices":[{"index":0,"delta":{"content":"hello"}}]}` + "\n\n" +
		"data: [DONE]\n\n"
	out, err := aggregateQoderSSE(strings.NewReader(sse), "m", 200)
	if err != nil {
		t.Fatalf("healthy stream must not error: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("content mangled: %s", out)
	}
}

// TestQueueFrameDoesNotRotate 锁定「排队帧按立即报错处理、不触发换号」的判定：
// 排队帧文本不含 credit / rate-limit marker，shouldRotateOnUpstreamErr 必须为
// false，使请求立即以错误收尾而非在号池内继续消耗。
func TestQueueFrameDoesNotRotate(t *testing.T) {
	msg := sseErrorFrame(queueFramePayload)
	if msg == "" {
		t.Fatal("queue frame must produce an error message")
	}
	// 同步路径构造的 canonical 错误文本
	canonical := "upstream 200: " + msg
	if shouldRotateOnUpstreamErr(200, canonical) {
		t.Fatalf("queue frame must NOT rotate accounts, body=%q", canonical)
	}
}

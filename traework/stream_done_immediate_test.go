// stream_done_immediate_test.go 验证收到 event: done 时立即退出流扫描，
// 不阻塞等待后续的 Keep-Alive 闲置连接，彻底解决推理完成后长延迟的问题。
package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

// hangingReader 在提供指定初始数据后，后续读取会陷入阻塞，模拟 Keep-Alive 长连接不主动断开的场景。
type hangingReader struct {
	r      io.Reader
	hanged bool
}

func (h *hangingReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	if err == io.EOF {
		h.hanged = true
		// 阻塞 10 秒模拟 Keep-Alive 闲置等待
		time.Sleep(10 * time.Second)
		return 0, io.EOF
	}
	return n, err
}

func TestCollectTraeStream_ExitsImmediatelyOnDone(t *testing.T) {
	sseContent := "event: output\ndata: {\"response\":\"你好，世界\"}\n\nevent: done\ndata: {}\n\n"
	hr := &hangingReader{r: strings.NewReader(sseContent)}

	doneCh := make(chan error, 1)
	start := time.Now()

	go func() {
		chunks, hasDone, _, _, err := collectTraeStream(hr, "qwen3.8-max", 200)
		if err != nil {
			doneCh <- err
			return
		}
		if !hasDone && len(chunks) == 0 {
			t.Errorf("expected chunks or done")
		}
		doneCh <- nil
	}()

	select {
	case err := <-doneCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		elapsed := time.Since(start)
		if elapsed >= 2*time.Second {
			t.Fatalf("collectTraeStream took too long (%v), did not exit immediately on done", elapsed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("collectTraeStream hanged waiting for EOF instead of exiting immediately on done!")
	}
}

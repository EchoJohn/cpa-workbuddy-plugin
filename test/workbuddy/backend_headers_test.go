package main

import (
	"net/http"
	"testing"
)

// TestBackendHeaders_ClientIdentity 验证聊天补全请求携带使用端标识头
// （X-IDE-Name=WorkBuddy、X-Agent-Purpose=conversation）；判据防标识头被误删导致
// 上游用量记录 client 字段为空、请求无法归属 WorkBuddy 活动积分；无副作用。
// 最近修改时间：2026-10-11 21:05:00 新增，覆盖使用端标识头契约
func TestBackendHeaders_ClientIdentity(t *testing.T) {
	sa := &storedAuth{
		Auth:    storedTokens{AccessToken: "tok-cn", Domain: "www.codebuddy.cn"},
		Account: storedAccount{UID: "uid-cn-1"},
	}
	req, err := http.NewRequest(http.MethodPost, endpointChatFor(sa), nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	backendHeaders(req, sa)

	want := map[string]string{
		"X-IDE-Type":      "WorkBuddy",
		"X-IDE-Name":      "WorkBuddy",
		"X-Agent-Purpose": "conversation",
	}
	for name, value := range want {
		if got := req.Header.Get(name); got != value {
			t.Fatalf("%s = %q; want %q", name, got, value)
		}
	}
}

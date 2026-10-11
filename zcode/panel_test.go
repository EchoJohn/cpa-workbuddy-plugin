package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestPanelPage_RendersHTML(t *testing.T) {
	raw := handlePanelPage()

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope failed: %v", err)
	}
	if !env.OK {
		t.Fatalf("panel envelope not OK: %+v", env.Error)
	}

	var resp pluginapi.ManagementResponse
	if err := json.Unmarshal(env.Result, &resp); err != nil {
		t.Fatalf("unmarshal ManagementResponse failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Headers.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected text/html, got %s", ct)
	}

	body := string(resp.Body)
	for _, want := range []string{"ZCode 账号面板", "导入 Key", "全部禁用", "testModal", "deleteModal", "importModal", "zcode-provider"} {
		if !strings.Contains(body, want) {
			t.Fatalf("panel HTML missing %q", want)
		}
	}
}

func TestPanelRoute_ResolvesPanelAndRoot(t *testing.T) {
	cases := []string{
		"/v0/resource/plugins/zcode-provider/panel",
		"/v0/resource/plugins/zcode-provider",
		"/v0/management/plugins/zcode-provider/panel",
		"/plugins/zcode-provider/panel",
	}
	for _, path := range cases {
		sub, _ := normalizeMgmtSubPath(path)
		if sub != "/panel" && sub != "/" {
			t.Fatalf("path %s resolved to %q, want /panel or /", path, sub)
		}
	}
}

func TestPanelRoute_ServedViaManagement(t *testing.T) {
	req := pluginapi.ManagementRequest{
		Method: http.MethodGet,
		Path:   "/v0/resource/plugins/zcode-provider/panel",
	}
	raw, _ := json.Marshal(req)
	resp, err := handleManagement(raw)
	if err != nil {
		t.Fatalf("handleManagement failed: %v", err)
	}

	var env envelope
	if err := json.Unmarshal(resp, &env); err != nil {
		t.Fatalf("unmarshal envelope failed: %v", err)
	}
	if !env.OK {
		t.Fatalf("envelope not OK")
	}
	var mgmt pluginapi.ManagementResponse
	if err := json.Unmarshal(env.Result, &mgmt); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if !strings.Contains(string(mgmt.Body), "ZCode 账号面板") {
		t.Fatalf("panel route did not return the panel HTML")
	}
}

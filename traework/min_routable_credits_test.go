package main

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// TestMinRoutableCredits_TraeWork 验证 TraeWork 安全缓冲额度红线默认 20、可配置、
// 调度排除 remain < 20、平摊轮询。
func TestMinRoutableCredits_TraeWork(t *testing.T) {
	resetActiveAuth(t)
	resetTestFailed(t)
	resetFailover(t)
	restoreMin := setMinRoutableCredits(defaultMinRoutableCredits)
	defer restoreMin()

	// 1. 默认阈值为 20
	if got := loadedMinRoutableCredits(); got != 20 {
		t.Fatalf("default minRoutableCredits = %d, want 20", got)
	}

	// 2. 通过 configure() 动态修改阈值为 50
	cfgBytes, _ := json.Marshal(map[string]any{
		"config_yaml": []byte("min_routable_credits: 50\nscheduler_mode: credits\n"),
	})
	configure(cfgBytes)
	if got := loadedMinRoutableCredits(); got != 50 {
		t.Fatalf("configured minRoutableCredits = %d, want 50", got)
	}
	setMinRoutableCredits(20)

	// 3. 调度器硬排除 remain < 20 账号，并在健康账号间平摊轮询
	accountCache.Store("tr-low", &accountCacheEntry{
		credits: &traeCredits{TotalRemain: 15},
	})
	accountCache.Store("tr-ok1", &traeCredits{TotalRemain: 100})
	accountCache.Store("tr-ok2", &traeCredits{TotalRemain: 200})
	defer func() {
		accountCache.Delete("tr-low")
		accountCache.Delete("tr-ok1")
		accountCache.Delete("tr-ok2")
	}()

	setActiveAuthID("tr-low")
	raw1, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "tr-low", Provider: providerName},
			{ID: "tr-ok1", Provider: providerName},
			{ID: "tr-ok2", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("pick1 err: %v", err)
	}
	resp1 := parsePickResponse(t, raw1)
	if !resp1.Handled || resp1.AuthID == "tr-low" {
		t.Fatalf("expected tr-low (<20) to be excluded, got %+v", resp1)
	}
	if resp1.AuthID != "tr-ok1" && resp1.AuthID != "tr-ok2" {
		t.Fatalf("expected a healthy candidate (tr-ok1 or tr-ok2), got %+v", resp1)
	}

	// 4. 所有候选均 < 20 时，必须全灭 defer (Handled: false)
	rawAllLow, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "tr-low", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("pickAllLow err: %v", err)
	}
	respAllLow := parsePickResponse(t, rawAllLow)
	if respAllLow.Handled {
		t.Fatalf("expected all low-credit candidates to defer (Handled: false), got %+v", respAllLow)
	}

	// 5. accountRoutable 对 remain < 20 判定为 false
	if accountRoutable("tr-low") {
		t.Fatal("expected accountRoutable(tr-low) == false for remain=15 < 20")
	}
	if !accountRoutable("tr-ok1") {
		t.Fatal("expected accountRoutable(tr-ok1) == true for remain=100 >= 20")
	}
}

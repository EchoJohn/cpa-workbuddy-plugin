package main

import (
	"encoding/json"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

// TestMinRoutableCredits_ConfigAndExclusion 验证安全缓冲额度红线默认 20、可配置、
// 调度排除 remain < 20、平摊轮询以及 14018 换号判据。
func TestMinRoutableCredits_ConfigAndExclusion(t *testing.T) {
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
	accountCache.Store("acc-low", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 15, TotalUsed: 85, TotalSize: 100},
	})
	accountCache.Store("acc-ok1", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 100, TotalUsed: 0, TotalSize: 100},
	})
	accountCache.Store("acc-ok2", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 200, TotalUsed: 0, TotalSize: 200},
	})
	defer func() {
		accountCache.Delete("acc-low")
		accountCache.Delete("acc-ok1")
		accountCache.Delete("acc-ok2")
	}()

	setActiveAuthID("acc-low")
	raw1, err := handleSchedulerPick(mustMarshalBytes(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "acc-low", Provider: providerName},
			{ID: "acc-ok1", Provider: providerName},
			{ID: "acc-ok2", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("pick1 err: %v", err)
	}
	resp1 := parsePickResponse(t, raw1)
	if !resp1.Handled || resp1.AuthID != "acc-ok2" {
		t.Fatalf("expected acc-low (<20) excluded and highest healthy acc-ok2 picked first, got %+v", resp1)
	}

	// 再次切换（清空当前选中模拟轮询）：应轮询到下一个健康候选 acc-ok1
	activeAuthMu.Lock()
	activeAuthID = ""
	activeAuthMu.Unlock()
	raw2, err := handleSchedulerPick(mustMarshalBytes(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "acc-low", Provider: providerName},
			{ID: "acc-ok1", Provider: providerName},
			{ID: "acc-ok2", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("pick2 err: %v", err)
	}
	resp2 := parsePickResponse(t, raw2)
	if !resp2.Handled || resp2.AuthID != "acc-ok1" {
		t.Fatalf("expected round-robin to pick acc-ok1 on second turn, got %+v", resp2)
	}

	// 4. 所有候选均 < 20 时，必须全灭 defer (Handled: false)
	rawAllLow, err := handleSchedulerPick(mustMarshalBytes(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "acc-low", Provider: providerName},
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
	if accountRoutable("acc-low") {
		t.Fatal("expected accountRoutable(acc-low) == false for remain=15 < 20")
	}
	if !accountRoutable("acc-ok1") {
		t.Fatal("expected accountRoutable(acc-ok1) == true for remain=100 >= 20")
	}

	// 6. 14018 与 exceeded the quota 必须触发换号
	if !shouldRotateOnUpstreamErr(200, `upstream 200: 14018: Your requests have exceeded the quota.`) {
		t.Fatal("expected 14018 / exceeded the quota to trigger account rotation")
	}
}

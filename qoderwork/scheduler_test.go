package main

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func resetActiveAuth(t *testing.T) {
	t.Helper()
	setActiveAuthID("")
	restoreMode := setSchedulerMode(schedulerModeCredits)
	t.Cleanup(func() {
		setActiveAuthID("")
		restoreMode()
	})
}

func TestSchedulerPick_NonQoderwork_Defers(t *testing.T) {
	resetActiveAuth(t)
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: "other",
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "x", Provider: "other"},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if resp.Handled {
		t.Fatal("non-qoderwork candidates should defer")
	}
}

func TestSchedulerPick_OffMode_Defers(t *testing.T) {
	setActiveAuthID("")
	restoreMode := setSchedulerMode(schedulerModeOff)
	t.Cleanup(func() {
		setActiveAuthID("")
		restoreMode()
	})
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-only", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if resp.Handled {
		t.Fatal("scheduler_mode=off should defer to built-in scheduler")
	}
}

func TestSchedulerPick_SingleCandidate_PicksIt(t *testing.T) {
	resetActiveAuth(t)
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-only", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if !resp.Handled || resp.AuthID != "qo-only" {
		t.Fatalf("want qo-only handled, got %+v", resp)
	}
	if getActiveAuthID() != "qo-only" {
		t.Fatalf("active auth should stick to qo-only, got %q", getActiveAuthID())
	}
}

func TestSchedulerPick_PrefersLowCreditOverHighCredit(t *testing.T) {
	resetActiveAuth(t)
	accountCache.Store("qo-a", &accountCacheEntry{credits: &creditsSummary{TotalRemain: 10, TotalSize: 10}})
	accountCache.Store("qo-b", &accountCacheEntry{credits: &creditsSummary{TotalRemain: 500, TotalSize: 500}})
	defer func() {
		accountCache.Delete("qo-a")
		accountCache.Delete("qo-b")
	}()
	setActiveAuthID("qo-a")
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-a", Provider: providerName},
			{ID: "qo-b", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	// qo-a (remain=10 < minRoutableCredits=20) 被低额安全红线硬排除，自动切换到健康的 qo-b (500)
	if !resp.Handled || resp.AuthID != "qo-b" {
		t.Fatalf("want low-credit account qo-a (<20) excluded and qo-b routed, got %+v", resp)
	}
}

func TestSchedulerPick_KeepsHealthyPanelSelection(t *testing.T) {
	resetActiveAuth(t)
	accountCache.Store("qo-a", &accountCacheEntry{credits: &creditsSummary{TotalRemain: 300, TotalSize: 300}})
	accountCache.Store("qo-b", &accountCacheEntry{credits: &creditsSummary{TotalRemain: 500, TotalSize: 500}})
	defer func() {
		accountCache.Delete("qo-a")
		accountCache.Delete("qo-b")
	}()
	setActiveAuthID("qo-a")
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-a", Provider: providerName},
			{ID: "qo-b", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if !resp.Handled || resp.AuthID != "qo-a" {
		t.Fatalf("want healthy panel selection qo-a kept, got %+v", resp)
	}
}

func TestSchedulerPick_StaysOnExhaustedSelection(t *testing.T) {
	resetActiveAuth(t)
	accountCache.Store("qo-exhausted", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 0, TotalUsed: 500, TotalSize: 500},
	})
	accountCache.Store("qo-ok", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 300, TotalUsed: 0, TotalSize: 300},
	})
	defer func() {
		accountCache.Delete("qo-exhausted")
		accountCache.Delete("qo-ok")
	}()
	setActiveAuthID("qo-exhausted")
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-exhausted", Provider: providerName},
			{ID: "qo-ok", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if !resp.Handled || resp.AuthID != "qo-ok" {
		t.Fatalf("want switch to qo-ok, got %+v", resp)
	}
	if getActiveAuthID() != "qo-ok" {
		t.Fatalf("active should update to qo-ok, got %q", getActiveAuthID())
	}
}

func TestSchedulerPick_AllExhausted_Defers(t *testing.T) {
	resetActiveAuth(t)
	accountCache.Store("qo-a", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 0, TotalUsed: 100, TotalSize: 100},
	})
	accountCache.Store("qo-b", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 0, TotalUsed: 200, TotalSize: 200},
	})
	defer func() {
		accountCache.Delete("qo-a")
		accountCache.Delete("qo-b")
	}()
	setActiveAuthID("qo-a")
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-a", Provider: providerName},
			{ID: "qo-b", Provider: providerName},
			{ID: "tr-a", Provider: "traework-provider"},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if resp.Handled {
		t.Fatalf("all exhausted should defer to host (cross-provider failover), got %+v", resp)
	}
}

func TestSchedulerPick_SwitchesOnlyWhenSelectionGone(t *testing.T) {
	resetActiveAuth(t)
	accountCache.Store("qo-ok", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 300, TotalUsed: 0, TotalSize: 300},
	})
	defer accountCache.Delete("qo-ok")
	setActiveAuthID("qo-gone")
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-ok", Provider: providerName},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if !resp.Handled || resp.AuthID != "qo-ok" {
		t.Fatalf("want switch to qo-ok, got %+v", resp)
	}
	if getActiveAuthID() != "qo-ok" {
		t.Fatalf("active should update to qo-ok, got %q", getActiveAuthID())
	}
}

func TestSchedulerPick_SkipsDisabledCandidates(t *testing.T) {
	resetActiveAuth(t)
	accountCache.Store("qo-live", &accountCacheEntry{
		credits: &creditsSummary{TotalRemain: 50, TotalSize: 50},
	})
	defer accountCache.Delete("qo-live")
	raw, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-off", Provider: providerName, Status: "disabled"},
			{ID: "qo-live", Provider: providerName, Status: "active"},
		},
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	resp := parsePickResponse(t, raw)
	if !resp.Handled || resp.AuthID != "qo-live" {
		t.Fatalf("want qo-live, got %+v", resp)
	}
	raw2, err := handleSchedulerPick(mustMarshal(t, pluginapi.SchedulerPickRequest{
		Provider: providerName,
		Candidates: []pluginapi.SchedulerAuthCandidate{
			{ID: "qo-off", Provider: providerName, Status: "disabled", Metadata: map[string]any{"disabled": true}},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	resp2 := parsePickResponse(t, raw2)
	if resp2.Handled {
		t.Fatalf("all disabled should defer, got %+v", resp2)
	}
}

func TestCandidateDisabled(t *testing.T) {
	if !candidateDisabled(pluginapi.SchedulerAuthCandidate{Status: "disabled"}) {
		t.Fatal("status disabled")
	}
	if !candidateDisabled(pluginapi.SchedulerAuthCandidate{Metadata: map[string]any{"disabled": true}}) {
		t.Fatal("meta disabled")
	}
	if candidateDisabled(pluginapi.SchedulerAuthCandidate{Status: "active"}) {
		t.Fatal("active should not be disabled")
	}
}

func TestEnsureDefaultActiveAuth(t *testing.T) {
	resetActiveAuth(t)
	id := ensureDefaultActiveAuth([]wbAccount{
		{AuthIndex: "a1", AuthID: "a1", Disabled: true},
		{AuthIndex: "a2", AuthID: "a2", Exhausted: false},
		{AuthIndex: "a3", AuthID: "a3"},
	})
	if id != "a2" {
		t.Fatalf("want first enabled a2, got %q", id)
	}
}

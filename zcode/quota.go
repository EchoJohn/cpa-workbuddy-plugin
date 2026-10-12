package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// zcode-plan 额度（billing/current）查询与缓存。
//
// 数据源（2026-10-12 线上实测）：GET https://zcode.z.ai/api/v1/zcode-plan/billing/current
// 以账号 JWT 作 `Authorization: Bearer`，返回当前生效计划及其 entitlements：
// 每个 entitlement 描述一个模型在某个周期内的授予额度（grant_units，如每日 300 万 token）。
// 该端点是 zcode 面板额度可视化的唯一可靠来源——billing/balance（余额/已用）在免费
// start-plan 账号上固定返回 `400 code 3001 parameter error`，故不采用。
const zcodeQuotaURL = "https://zcode.z.ai/api/v1/zcode-plan/billing/current"

// quotaURLForTest 供单元测试注入 mock 端点；为空时使用 zcodeQuotaURL。
var quotaURLForTest string

// zcodeQuotaTTL 是额度快照的进程内缓存时长，避免面板频繁打开时反复打上游。
const zcodeQuotaTTL = 5 * time.Minute

// zcodeQuotaEntitlement 是单个模型的额度授予项。
type zcodeQuotaEntitlement struct {
	EntitlementID string `json:"entitlement_id"`
	ShowName      string `json:"show_name"`   // 展示名，如 GLM-5.3
	Meter         string `json:"meter"`       // 计量方式，如 model_usage
	UnitType      string `json:"unit_type"`   // 单位，如 token
	Capability    string `json:"capability"`  // 归一化能力，如 model:glm-5.3
	GrantUnits    int64  `json:"grant_units"` // 该周期授予额度
	Period        string `json:"period"`      // 周期，如 daily
	Priority      int64  `json:"priority"`
}

// zcodeQuotaSnapshot 是单个账号的额度快照。
type zcodeQuotaSnapshot struct {
	PlanName     string                  `json:"plan_name"`
	PlanID       string                  `json:"plan_id"`
	PlanStatus   string                  `json:"plan_status"`
	Description  string                  `json:"description,omitempty"`
	StartsAt     int64                   `json:"starts_at,omitempty"`
	EndsAt       int64                   `json:"ends_at,omitempty"`
	Entitlements []zcodeQuotaEntitlement `json:"entitlements"`
	TotalGrant   int64                   `json:"total_grant"` // 全部 entitlement 授予额度合计
	FetchedAt    string                  `json:"fetched_at"`
}

// zcodeQuotaCacheEntry 是缓存条目（含过期时间）。
type zcodeQuotaCacheEntry struct {
	quota     *zcodeQuotaSnapshot
	fetchedAt time.Time
}

var (
	zcodeQuotaMu    sync.RWMutex
	zcodeQuotaCache = make(map[string]zcodeQuotaCacheEntry)
)

// zcodeQuotaResponse 是 billing/current 的响应体。
type zcodeQuotaResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ServerTime int64 `json:"server_time"`
		Plans      []struct {
			UserPlanID   string `json:"user_plan_id"`
			PlanID       string `json:"plan_id"`
			Name         string `json:"name"`
			Description  string `json:"description"`
			Priority     int64  `json:"priority"`
			Status       string `json:"status"`
			StartsAt     int64  `json:"starts_at"`
			EndsAt       int64  `json:"ends_at"`
			Entitlements []struct {
				EntitlementID string   `json:"entitlement_id"`
				ShowName      string   `json:"show_name"`
				Meter         string   `json:"meter"`
				UnitType      string   `json:"unit_type"`
				Capabilities  []string `json:"capabilities"`
				GrantUnits    int64    `json:"grant_units"`
				Period        string   `json:"period"`
				Priority      int64    `json:"priority"`
			} `json:"entitlements"`
		} `json:"plans"`
	} `json:"data"`
}

// fetchAccountQuota 查询单个账号的额度快照（不走缓存）。
//
// [参数] sa: 账号凭据（需含 JWT）。
// [返回] 额度快照；账号无 JWT 或上游异常时返回错误。
func fetchAccountQuota(sa *StoredAuth) (*zcodeQuotaSnapshot, error) {
	if sa == nil {
		return nil, fmt.Errorf("nil account")
	}
	jwt := strings.TrimSpace(sa.Token)
	if jwt == "" {
		jwt = strings.TrimSpace(sa.APIKey)
	}
	if jwt == "" {
		return nil, fmt.Errorf("账号缺少 JWT，无法查询额度")
	}

	req, errNew := http.NewRequest(http.MethodGet, quotaEndpoint(), nil)
	if errNew != nil {
		return nil, fmt.Errorf("create quota request: %w", errNew)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ZCode/"+mimicAppVersion())
	req.Header.Set("X-ZCode-App-Version", mimicAppVersion())
	req.Header.Set("HTTP-Referer", "https://zcode.z.ai")
	req.Header.Set("X-Title", "Z Code@electron")

	resp, errDo := modelHTTPClient.Do(req)
	if errDo != nil {
		return nil, fmt.Errorf("request quota: %w", errDo)
	}
	defer resp.Body.Close()

	raw, errRead := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if errRead != nil {
		return nil, fmt.Errorf("read quota response: %w", errRead)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("quota upstream error %d: %s", resp.StatusCode, string(raw))
	}

	var parsed zcodeQuotaResponse
	if errUnmarshal := json.Unmarshal(raw, &parsed); errUnmarshal != nil {
		return nil, fmt.Errorf("parse quota response: %w", errUnmarshal)
	}
	if parsed.Code != 0 {
		msg := strings.TrimSpace(parsed.Msg)
		if msg == "" {
			msg = fmt.Sprintf("code=%d", parsed.Code)
		}
		return nil, fmt.Errorf("quota query failed: %s", msg)
	}
	if len(parsed.Data.Plans) == 0 {
		return nil, fmt.Errorf("账号无生效计划")
	}

	plan := parsed.Data.Plans[0]
	snap := &zcodeQuotaSnapshot{
		PlanName:    strings.TrimSpace(plan.Name),
		PlanID:      strings.TrimSpace(plan.PlanID),
		PlanStatus:  strings.TrimSpace(plan.Status),
		Description: strings.TrimSpace(plan.Description),
		StartsAt:    plan.StartsAt,
		EndsAt:      plan.EndsAt,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	for _, ent := range plan.Entitlements {
		capability := ""
		if len(ent.Capabilities) > 0 {
			capability = strings.TrimSpace(ent.Capabilities[0])
		}
		snap.Entitlements = append(snap.Entitlements, zcodeQuotaEntitlement{
			EntitlementID: strings.TrimSpace(ent.EntitlementID),
			ShowName:      strings.TrimSpace(ent.ShowName),
			Meter:         strings.TrimSpace(ent.Meter),
			UnitType:      strings.TrimSpace(ent.UnitType),
			Capability:    capability,
			GrantUnits:    ent.GrantUnits,
			Period:        strings.TrimSpace(ent.Period),
			Priority:      ent.Priority,
		})
		snap.TotalGrant += ent.GrantUnits
	}
	return snap, nil
}

// quotaForAccount 返回账号额度，优先命中进程内缓存。
//
// [参数] sa: 账号凭据；force: true 时跳过缓存强制刷新。
// [返回] 额度快照与错误。
func quotaForAccount(sa *StoredAuth, force bool) (*zcodeQuotaSnapshot, error) {
	if sa == nil || strings.TrimSpace(sa.AuthID) == "" {
		return fetchAccountQuota(sa)
	}
	key := sa.AuthID

	if !force {
		zcodeQuotaMu.RLock()
		entry, ok := zcodeQuotaCache[key]
		zcodeQuotaMu.RUnlock()
		if ok && entry.quota != nil && time.Since(entry.fetchedAt) < zcodeQuotaTTL {
			return entry.quota, nil
		}
	}

	snap, errFetch := fetchAccountQuota(sa)
	if errFetch != nil {
		return nil, errFetch
	}
	zcodeQuotaMu.Lock()
	zcodeQuotaCache[key] = zcodeQuotaCacheEntry{quota: snap, fetchedAt: time.Now()}
	zcodeQuotaMu.Unlock()
	return snap, nil
}

// cachedQuota 只读缓存中的额度（不触发上游请求），供 /accounts 列表附带展示。
//
// [参数] authID: 账号标识。
// [返回] 已缓存且未过期的额度；无缓存或已过期时返回 nil。
func cachedQuota(authID string) *zcodeQuotaSnapshot {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return nil
	}
	zcodeQuotaMu.RLock()
	defer zcodeQuotaMu.RUnlock()
	entry, ok := zcodeQuotaCache[authID]
	if !ok || entry.quota == nil || time.Since(entry.fetchedAt) >= zcodeQuotaTTL {
		return nil
	}
	return entry.quota
}

// invalidateQuota 清除某账号的额度缓存（删除账号或探活后调用）。
//
// [参数] authID: 账号标识。
// [返回] 无。
func invalidateQuota(authID string) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	zcodeQuotaMu.Lock()
	delete(zcodeQuotaCache, authID)
	zcodeQuotaMu.Unlock()
}

// quotaEndpoint 返回当前生效的额度查询端点（测试可注入覆盖）。
//
// [参数] 无。
// [返回] 端点 URL。
func quotaEndpoint() string {
	if strings.TrimSpace(quotaURLForTest) != "" {
		return quotaURLForTest
	}
	return zcodeQuotaURL
}

// mimicAppVersion 返回当前指纹使用的 app 版本（额度请求复用同一取值）。
//
// [参数] 无。
// [返回] app 版本字符串。
func mimicAppVersion() string {
	v := strings.TrimSpace(currentConfig().Mimic.AppVersion)
	if v == "" {
		return "unknown"
	}
	return v
}

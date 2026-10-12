package main

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type failoverEntry struct {
	failCount int
	cooldown  time.Time
	lastError string
}

var (
	failoverMu      sync.RWMutex
	failoverRecords = make(map[string]*failoverEntry)
)

// failoverCooldown 是单次账号故障后的固定冷却窗口（对齐 workbuddy 成熟策略）。
// 避免因瞬时并发满（429 code 3009）动辄封禁 1~10 分钟导致整个插件无账号可用。
const failoverCooldown = 15 * time.Second

// isAccountCoolingDown 检查账号是否处于冷却期。
func isAccountCoolingDown(authID string) bool {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return false
	}
	failoverMu.RLock()
	defer failoverMu.RUnlock()

	entry, ok := failoverRecords[authID]
	if !ok {
		return false
	}
	return time.Now().Before(entry.cooldown)
}

// noteAccountFailure 记录账号故障并设置固定 15 秒冷却。
func noteAccountFailure(authID string, statusCode int, errMsg string) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	failoverMu.Lock()
	defer failoverMu.Unlock()

	entry, ok := failoverRecords[authID]
	if !ok {
		entry = &failoverEntry{}
		failoverRecords[authID] = entry
	}

	entry.failCount++
	entry.lastError = errMsg
	entry.cooldown = time.Now().Add(failoverCooldown)
}

// resetAccountFailover 成功调用后重置故障计数与冷却状态。
func resetAccountFailover(authID string) {
	authID = strings.TrimSpace(authID)
	if authID == "" {
		return
	}
	failoverMu.Lock()
	defer failoverMu.Unlock()

	delete(failoverRecords, authID)
}

// shouldRotateOnUpstreamErr 判断上游错误是否应触发同请求换号重试。
func shouldRotateOnUpstreamErr(statusCode int, errMsg string) bool {
	if statusCode == http.StatusUnauthorized ||
		statusCode == http.StatusForbidden ||
		statusCode == http.StatusNotFound ||
		statusCode == http.StatusMethodNotAllowed ||
		statusCode == http.StatusTooManyRequests {
		return true
	}
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "quota") ||
		strings.Contains(lower, "insufficient") ||
		strings.Contains(lower, "concurrency") ||
		strings.Contains(lower, "3009") {
		return true
	}
	return false
}

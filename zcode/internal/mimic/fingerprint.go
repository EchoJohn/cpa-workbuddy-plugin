// Package mimic generates ZCode client request fingerprints.
package mimic

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	AppVersion     string `yaml:"app_version" json:"app_version"`
	SessionID      string `yaml:"session_id" json:"session_id"`
	TraceID        string `yaml:"trace_id" json:"trace_id"`
	UserID         string `yaml:"user_id" json:"user_id"`
	DeviceMid      string `yaml:"device_mid" json:"device_mid"`
	Platform       string `yaml:"platform" json:"platform"`
	OSCategory     string `yaml:"os_category" json:"os_category"`
	OSVersion      string `yaml:"os_version" json:"os_version"`
	Language       string `yaml:"language" json:"language"`
	Timezone       string `yaml:"timezone" json:"timezone"`
	ReleaseChannel string `yaml:"release_channel" json:"release_channel"`
	Title          string `yaml:"title" json:"title"`
}

func DefaultConfig() Config {
	return Config{
		Enabled: true,
		// AppVersion 必须保持上游放行的兜底值，不能填具体版本号。
		//
		// 2026-10-12 线上实测（zcode.z.ai/api/v1/zcode-plan/anthropic/v1/messages，
		// 同一账号 JWT、同一 IP 交错复测 8 次）：X-ZCode-App-Version 取具体版本号
		// （3.14.4 / 3.15.1 / 3.13.0 …）一律被上游 WAF 拦截为 405 + code 3012
		// `request has been blocked due to unusual activity`；取非版本号字面量
		// （unknown / cli / electron / 3.14.4.1）稳定放行 200。
		// 桌面端 buildZCodeSourceHeadersFromContext 亦以 `??"unknown"` 兜底，
		// 说明 unknown 是上游认可的合法取值，而具体版本号已被风控名单命中。
		AppVersion:     "unknown",
		Platform:       "win32-x64",
		OSCategory:     "windows",
		OSVersion:      "10.0.19045",
		Language:       "zh-CN",
		Timezone:       "Asia/Shanghai",
		ReleaseChannel: "production",
		Title:          "Z Code@electron",
	}
}

type Fingerprint struct {
	cfg       Config
	sessionID string
	traceID   string
	userID    string
	deviceMid string
}

func New(cfg Config, keyID string) *Fingerprint {
	return &Fingerprint{
		cfg:       cfg,
		sessionID: orUUID(cfg.SessionID),
		traceID:   orUUID(cfg.TraceID),
		userID:    stableUserID(cfg.UserID, keyID),
		deviceMid: stableUserID(cfg.DeviceMid, keyID),
	}
}

func (f *Fingerprint) UserID() string {
	return f.userID
}

// ApplyRequest 注入 ZCode 桌面客户端全量指纹头。
//
// 头名与取值对齐桌面端 `buildZCodeSourceHeadersFromContext`（app.asar 逆向，2026-10-12）：
// HTTP-Referer 为 zcode 站点 origin，X-Os-Category 由平台名归一化（win32→windows、
// darwin→macos），X-Device-Mid 为设备中台标识。其中 X-ZCode-App-Version 取值直接
// 决定上游风控是否放行（见 DefaultConfig 中 AppVersion 的实测说明），必须保持
// `unknown` 之类的非版本号字面量。
// [参数] h: 待写入的请求头；apiKey: 鉴权凭证（Bearer 由调用方另行覆盖）；
//
//	requestID: 本次请求的 x-request-id。
//
// [返回] 无。
func (f *Fingerprint) ApplyRequest(h http.Header, apiKey, requestID string) {
	if !f.cfg.Enabled {
		h.Set("x-api-key", apiKey)
		return
	}
	h.Set("Content-Type", "application/json")
	h.Set("anthropic-version", "2023-06-01")
	h.Set("x-api-key", apiKey)
	h.Set("HTTP-Referer", "https://zcode.z.ai")
	// User-Agent 与 X-ZCode-App-Version 解耦：前者只是客户端标识，后者才是上游风控
	// 判定字段，必须保持放行值（见 DefaultConfig 中 AppVersion 的实测说明）。
	h.Set("User-Agent", "ZCode/"+f.cfg.AppVersion)
	h.Set("X-ZCode-App-Version", f.cfg.AppVersion)
	h.Set("X-Title", "Z Code@"+sourceTitle(f.cfg.Title))
	h.Set("X-Release-Channel", f.cfg.ReleaseChannel)
	h.Set("X-Client-Language", f.cfg.Language)
	h.Set("X-Client-Timezone", f.cfg.Timezone)
	h.Set("x-zcode-agent", "glm")
	h.Set("X-Platform", f.cfg.Platform)
	h.Set("X-Os-Category", osCategory(f.cfg.OSCategory))
	h.Set("X-Os-Version", f.cfg.OSVersion)
	h.Set("X-Device-Mid", f.deviceMid)
	h.Set("x-request-id", requestID)
	h.Set("x-zcode-session-type", "main")
	h.Set("x-zcode-trace-id", f.traceID)
	h.Set("x-query-id", UUID7())
	h.Set("x-session-id", f.sessionID)
}

// sourceTitle 从配置的标题（如 "Z Code@electron"）中取出 `@` 后的来源标识。
// [参数] title: 配置标题。
// [返回] 来源标识；缺省为 electron。
func sourceTitle(title string) string {
	title = strings.TrimSpace(title)
	if idx := strings.LastIndex(title, "@"); idx >= 0 && idx+1 < len(title) {
		return strings.TrimSpace(title[idx+1:])
	}
	if title == "" {
		return "electron"
	}
	return title
}

// osCategory 将平台名归一化为上游期望的类别（win32→windows、darwin→macos）。
// [参数] category: 配置中的平台类别。
// [返回] 归一化后的类别。
func osCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "win32", "windows":
		return "windows"
	case "darwin", "macos":
		return "macos"
	case "linux":
		return "linux"
	}
	return strings.TrimSpace(category)
}

func orUUID(s string) string {
	if s != "" {
		return s
	}
	return UUID4()
}

func stableUserID(configured, keyID string) string {
	if configured != "" {
		return configured
	}
	h := sha1.New()
	h.Write([]byte("zcode2api:user"))
	h.Write([]byte(keyID))
	var b [16]byte
	copy(b[:], h.Sum(nil))
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

func UUID4() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

func UUID7() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	ms := uint64(time.Now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80
	return formatUUID(b)
}

func formatUUID(b [16]byte) string {
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	)
}

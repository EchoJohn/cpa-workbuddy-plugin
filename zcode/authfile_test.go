package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthFile_NamingAndSafety(t *testing.T) {
	key := "607c3abcdef.1234567890"
	fileName := authFileNameFor(key)
	if fileName == "" {
		t.Fatalf("expected non-empty fileName")
	}
	if !isZCodeAuthFileName(fileName) {
		t.Fatalf("expected valid auth file name, got %s", fileName)
	}

	authID := authIDFor(key)
	if authID == "" {
		t.Fatalf("expected non-empty authID")
	}

	if !isSafeZCodeAuthPath(fileName) {
		t.Fatalf("expected safe path for %s", fileName)
	}
	if isSafeZCodeAuthPath("../etc/passwd") {
		t.Fatalf("expected unsafe path for ../etc/passwd")
	}
	if isSafeZCodeAuthPath("other.json") {
		t.Fatalf("expected unsafe path for other.json")
	}
}

func TestAuthFile_CRUD(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zcode_test_accounts_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	setAccountsDir(tempDir)

	apiKey := "test_api_key_12345.secret"
	authID := authIDFor(apiKey)
	fileName := authFileNameFor(apiKey)
	filePath := filepath.Join(tempDir, fileName)

	sa := &StoredAuth{
		Type:       "zcode",
		Provider:   zcodeProviderID,
		AuthID:     authID,
		APIKey:     apiKey,
		Label:      "Test Account",
		Disabled:   false,
		Models:     []string{"GLM-5.3"},
		TestFailed: false,
	}

	if err := writeAuthFileDirect(filePath, sa); err != nil {
		t.Fatalf("writeAuthFileDirect failed: %v", err)
	}

	readSA, err := readAuthFile(filePath)
	if err != nil {
		t.Fatalf("readAuthFile failed: %v", err)
	}
	if readSA.APIKey != apiKey || readSA.Label != "Test Account" {
		t.Fatalf("readSA content mismatch: %+v", readSA)
	}

	list, err := listAllAuthFiles()
	if err != nil {
		t.Fatalf("listAllAuthFiles failed: %v", err)
	}
	if len(list) != 1 || list[0].AuthID != authID {
		t.Fatalf("expected 1 account, got %d", len(list))
	}

	if err := deleteAuthFileDirect(authID); err != nil {
		t.Fatalf("deleteAuthFileDirect failed: %v", err)
	}

	listAfter, _ := listAllAuthFiles()
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 accounts after delete, got %d", len(listAfter))
	}
}

func TestMaskAPIKey(t *testing.T) {
	if maskAPIKey("short") != "******" {
		t.Fatalf("expected ****** for short key")
	}
	masked := maskAPIKey("1234567890abcdef")
	if masked != "1234....cdef" {
		t.Fatalf("unexpected mask: %s", masked)
	}
}

func TestAuthFile_ZaiAccount(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "zcode_test_zai_*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	setAccountsDir(tempDir)

	user := &ZaiUserInfo{
		UserID: "10086",
		Name:   "张三",
		Email:  "zhangsan@z.ai",
		Avatar: "https://avatar.z.ai/1.png",
	}
	sa, err := saveZaiAccount("mock_token_12345678", "mock_access_token", "mock_biz_token", user, time.Now().Add(30*24*time.Hour), "测试Zai账号")
	if err != nil {
		t.Fatalf("saveZaiAccount failed: %v", err)
	}
	if sa.UserID != "10086" || sa.Username != "张三" || sa.Email != "zhangsan@z.ai" {
		t.Fatalf("user fields mismatch: %+v", sa)
	}
	if sa.APIKey != "mock_token_12345678" || sa.Token != "mock_token_12345678" {
		t.Fatalf("token mismatch: %+v", sa)
	}

	list, err := listAllAuthFiles()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 account, got %d (err=%v)", len(list), err)
	}
	if list[0].Username != "张三" {
		t.Fatalf("expected username 张三, got %s", list[0].Username)
	}
}

// secrets_test.go —— 密码哈希迁移与敏感配置拆分（secrets.yaml）的行为测试。
package webui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func authedJSON(t *testing.T, ts *httptest.Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// TestLoginMigratesPlaintextPassword 旧版主配置明文密码：登录成功后自动迁移为
// secrets.yaml 哈希，主配置明文移除，新旧两种方式登录均按哈希校验。
func TestLoginMigratesPlaintextPassword(t *testing.T) {
	ts := newTestServer(t)

	if code, _ := login(t, ts, "admin", "secret123"); code != http.StatusOK {
		t.Fatalf("明文密码登录失败: %d", code)
	}
	mainData, err := os.ReadFile(filepath.Join(RT().RootDir, "config", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainData), "secret123") {
		t.Error("登录后主配置仍含明文密码")
	}
	secretsPath := filepath.Join(RT().RootDir, "config", "secrets.yaml")
	secretsData, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("secrets.yaml 未生成: %v", err)
	}
	if !strings.Contains(string(secretsData), "password_hash") {
		t.Error("secrets.yaml 缺少 password_hash")
	}
	if st, err := os.Stat(secretsPath); err == nil && st.Mode().Perm() != 0o600 {
		t.Errorf("secrets.yaml 权限 = %o, want 600", st.Mode().Perm())
	}

	cfg := RT().GetConfig()
	if cfg.WebUI.Password != "" || cfg.WebUI.PasswordHash == "" {
		t.Errorf("迁移后 Password=%q PasswordHash 非空期望, got %q", cfg.WebUI.Password, cfg.WebUI.PasswordHash)
	}
	// 哈希路径登录：正确密码通过，错误密码拒绝
	if code, _ := login(t, ts, "admin", "secret123"); code != http.StatusOK {
		t.Errorf("迁移后原密码登录失败: %d", code)
	}
	if code, _ := login(t, ts, "admin", "错误密码"); code != http.StatusUnauthorized {
		t.Errorf("错误密码应 401, got %d", code)
	}
}

// TestSaveConfigSplitsSecrets 配置保存：敏感键入 secrets.yaml（0600），
// 主配置不含敏感值；新密码哈希生效；读接口敏感值全部掩码。
func TestSaveConfigSplitsSecrets(t *testing.T) {
	ts := newTestServer(t)
	_, out := login(t, ts, "admin", "secret123")
	token, _ := out["token"].(string)

	payload := map[string]any{
		"webui": map[string]any{"username": "admin", "password": "新密码456", "token_expire_hours": 24},
		"u115":  map[string]any{"cookie": "UID=1; CID=2; SEID=3", "qps": 3},
		"tmdb":  map[string]any{"api_key": "test-key"},
	}
	if code, resp := authedJSON(t, ts, http.MethodPut, "/api/config/main", token, map[string]any{"data": payload}); code != http.StatusOK {
		t.Fatalf("保存配置失败: %d %v", code, resp)
	}

	root := RT().RootDir
	mainData, _ := os.ReadFile(filepath.Join(root, "config", "config.yaml"))
	if strings.Contains(string(mainData), "UID=1") || strings.Contains(string(mainData), "新密码456") {
		t.Error("主配置不应含 cookie 或明文密码")
	}
	secretsPath := filepath.Join(root, "config", "secrets.yaml")
	secretsData, _ := os.ReadFile(secretsPath)
	if !strings.Contains(string(secretsData), "UID=1; CID=2; SEID=3") {
		t.Error("secrets.yaml 缺少 cookie")
	}
	if !strings.Contains(string(secretsData), "password_hash") || strings.Contains(string(secretsData), "新密码456") {
		t.Error("secrets.yaml 应含 password_hash 且不含明文")
	}
	if st, err := os.Stat(secretsPath); err == nil && st.Mode().Perm() != 0o600 {
		t.Errorf("secrets.yaml 权限 = %o, want 600", st.Mode().Perm())
	}

	// 新密码登录成功、旧密码失效
	if code, _ := login(t, ts, "admin", "新密码456"); code != http.StatusOK {
		t.Errorf("新密码登录失败: %d", code)
	}
	if code, _ := login(t, ts, "admin", "secret123"); code != http.StatusUnauthorized {
		t.Errorf("旧密码应 401, got %d", code)
	}

	// 读接口：敏感值不回传（cookie/api_key 显示掩码占位符）；密码与哈希不回传
	code, cfgOut := authedJSON(t, ts, http.MethodGet, "/api/config/main", token, nil)
	if code != http.StatusOK {
		t.Fatalf("读取配置失败: %d", code)
	}
	if got := cfgOut["u115"].(map[string]any)["cookie"]; got != secretMaskPlaceholder {
		t.Errorf("cookie 回显 = %v, want 掩码占位符", got)
	}
	if got := cfgOut["tmdb"].(map[string]any)["api_key"]; got != secretMaskPlaceholder {
		t.Errorf("api_key 回显 = %v, want 掩码占位符", got)
	}
	webuiSec := cfgOut["webui"].(map[string]any)
	if pw, _ := webuiSec["password"].(string); pw != "" {
		t.Errorf("密码不应回传, got %q", pw)
	}
	if _, ok := webuiSec["password_hash"]; ok {
		t.Error("password_hash 不应回传")
	}
}

// TestSaveConfigSecretMaskRoundTrip 掩码占位符原样回传保存 = 敏感值保持不变；
// 清空保存 = 移除该敏感项。
func TestSaveConfigSecretMaskRoundTrip(t *testing.T) {
	ts := newTestServer(t)
	_, out := login(t, ts, "admin", "secret123")
	token, _ := out["token"].(string)

	payload := map[string]any{
		"webui": map[string]any{"username": "admin", "token_expire_hours": 24},
		"u115":  map[string]any{"cookie": "UID=1; CID=2; SEID=3", "qps": 3},
	}
	if code, resp := authedJSON(t, ts, http.MethodPut, "/api/config/main", token, map[string]any{"data": payload}); code != http.StatusOK {
		t.Fatalf("保存配置失败: %d %v", code, resp)
	}

	// 读接口返回占位符；原样回传保存 → 现有值不变
	_, cfgOut := authedJSON(t, ts, http.MethodGet, "/api/config/main", token, nil)
	masked := cfgOut["u115"].(map[string]any)["cookie"]
	if masked != secretMaskPlaceholder {
		t.Fatalf("cookie 读回 = %v, want %s", masked, secretMaskPlaceholder)
	}
	payload["u115"].(map[string]any)["cookie"] = masked
	if code, resp := authedJSON(t, ts, http.MethodPut, "/api/config/main", token, map[string]any{"data": payload}); code != http.StatusOK {
		t.Fatalf("占位符保存失败: %d %v", code, resp)
	}
	secretsData, _ := os.ReadFile(filepath.Join(RT().RootDir, "config", "secrets.yaml"))
	if !strings.Contains(string(secretsData), "UID=1; CID=2; SEID=3") {
		t.Error("占位符保存后 cookie 被改动")
	}

	// 清空保存 → 移除
	payload["u115"].(map[string]any)["cookie"] = ""
	if code, resp := authedJSON(t, ts, http.MethodPut, "/api/config/main", token, map[string]any{"data": payload}); code != http.StatusOK {
		t.Fatalf("清空保存失败: %d %v", code, resp)
	}
	if got := RT().GetConfig().U115.Cookie; got != "" {
		t.Errorf("清空保存后 cookie 应为空, got %q", got)
	}
}

// TestSaveConfigKeepsPasswordWhenEmpty 密码留空保存 = 保持现有密码不变。
func TestSaveConfigKeepsPasswordWhenEmpty(t *testing.T) {
	ts := newTestServer(t)
	_, out := login(t, ts, "admin", "secret123")
	token, _ := out["token"].(string)
	hashBefore := RT().GetConfig().WebUI.PasswordHash
	if hashBefore == "" {
		t.Fatal("前置条件：登录迁移后应已有 password_hash")
	}

	// payload 完全不带 password 键
	payload := map[string]any{
		"webui": map[string]any{"username": "admin", "token_expire_hours": 24},
		"tmdb":  map[string]any{"api_key": "test-key"},
	}
	if code, resp := authedJSON(t, ts, http.MethodPut, "/api/config/main", token, map[string]any{"data": payload}); code != http.StatusOK {
		t.Fatalf("保存配置失败: %d %v", code, resp)
	}
	cfg := RT().GetConfig()
	if cfg.WebUI.PasswordHash != hashBefore {
		t.Errorf("密码留空保存后哈希被改动")
	}
	if code, _ := login(t, ts, "admin", "secret123"); code != http.StatusOK {
		t.Errorf("留空保存后原密码登录失败: %d", code)
	}
}

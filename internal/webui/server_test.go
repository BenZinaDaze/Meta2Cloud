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

	"gopkg.in/yaml.v3"
)

// newTestServer 构造测试服务：独立 rootDir + 密码配置。
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgYAML := map[string]any{
		"webui": map[string]any{"username": "admin", "password": "secret123"},
		"tmdb":  map[string]any{"api_key": "test-key"},
	}
	data, _ := yaml.Marshal(cfgYAML)
	if err := os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	server := NewServer()
	t.Cleanup(server.Close)
	return httptest.NewServer(server.Handler())
}

func login(t *testing.T, ts *httptest.Server, user, pass string) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	resp, err := http.Post(ts.URL+"/api/auth:login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestLoginFlow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	// 错误密码
	if code, _ := login(t, ts, "admin", "wrong"); code != 401 {
		t.Errorf("错误密码 code = %d, want 401", code)
	}
	// 正确登录
	code, out := login(t, ts, "admin", "secret123")
	if code != 200 {
		t.Fatalf("login code = %d: %v", code, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("未返回 token")
	}

	// 带 token 访问受保护接口
	req, _ := http.NewRequest("GET", ts.URL+"/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var me map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&me)
	if resp.StatusCode != 200 || me["username"] != "admin" {
		t.Errorf("me = %v %v", resp.StatusCode, me)
	}

	// 无 token 访问受保护接口 → 401
	resp2, err := http.Get(ts.URL + "/api/library")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 401 {
		t.Errorf("no token code = %d, want 401", resp2.StatusCode)
	}

	// 伪造 token → 401
	req3, _ := http.NewRequest("GET", ts.URL+"/api/stats", nil)
	req3.Header.Set("Authorization", "Bearer fake.token.here")
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatal(err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != 401 {
		t.Errorf("fake token code = %d, want 401", resp3.StatusCode)
	}
}

func TestLibraryEmptyHint(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	req, _ := http.NewRequest("GET", ts.URL+"/api/library", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 200 {
		t.Fatalf("library code = %d", resp.StatusCode)
	}
	if body["scanned_at"] != nil {
		t.Errorf("未扫描时 scanned_at 应为 null")
	}
	if hint, _ := body["hint"].(string); !strings.Contains(hint, "扫描") {
		t.Errorf("hint = %q", hint)
	}
}

func TestLoginWithoutPassword(t *testing.T) {
	// 未配置密码时应拒绝登录
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte("webui:\n  username: admin\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	server := NewServer()
	defer server.Close()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	code, out := login(t, ts, "admin", "")
	if code != 403 {
		t.Errorf("未设密码 code = %d, want 403: %v", code, out)
	}
}

func TestTokenRoundTrip(t *testing.T) {
	newTestServer(t) // 初始化 runtime
	token, err := CreateToken("tester", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := VerifyToken(token); got != "tester" {
		t.Errorf("verify = %q", got)
	}
	if got := VerifyToken(token + "x"); got != "" {
		t.Errorf("篡改 token 不应通过: %q", got)
	}
}

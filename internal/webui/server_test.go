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
	"time"

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

// ── 媒体库刷新 / 扫描 / 日志 / 入库历史 端到端 ─────────────

func newScanServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	cfgYAML := "webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n  movie_root_id: \"movies\"\n  tv_root_id: \"tv\"\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	// 构造假网盘：movies/电影 (2010)，tv/剧 (2008)/Season 01/S01E01
	fp := newFakeWebProvider()
	fp.addFolder("movies", "0")
	fp.addFolder("tv", "0")
	movie := fp.addFolder("电影 (2010)", "movies")
	_ = movie
	show := fp.addFolder("剧 (2008)", "tv")
	season := fp.addFolder("Season 01", show.ID)
	fp.addFile("剧 - S01E01.mkv", season.ID)

	RT().SetProviderForTest(fp.provider())

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	_, out := login(t, ts, "admin", "secret123")
	return ts, out["token"].(string)
}

func authedGet(t *testing.T, ts *httptest.Server, token, path string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestRefreshLibraryAndScan(t *testing.T) {
	ts, token := newScanServer(t)

	// 增量刷新
	req, _ := http.NewRequest("POST", ts.URL+"/api/library:refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var diff map[string]any
	json.NewDecoder(resp.Body).Decode(&diff)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("refresh code = %d: %v", resp.StatusCode, diff)
	}
	if diff["new_items"] != float64(2) {
		t.Errorf("diff = %v, want 2 new items", diff)
	}

	// 媒体库列表（应包含扫描结果）
	code, lib := authedGet(t, ts, token, "/api/library")
	if code != 200 {
		t.Fatalf("library code = %d", code)
	}
	if lib["total_movies"] != float64(1) || lib["total_tv"] != float64(1) {
		t.Errorf("library = movies:%v tv:%v", lib["total_movies"], lib["total_tv"])
	}

	// 统计
	_, stats := authedGet(t, ts, token, "/api/stats")
	if stats["total_movies"] != float64(1) {
		t.Errorf("stats = %v", stats)
	}

	// 电影 / 剧集扫描
	_, movies := authedGet(t, ts, token, "/api/movies")
	if _, ok := movies["detail"]; ok {
		t.Errorf("movies err: %v", movies)
	}
	_, shows := authedGet(t, ts, token, "/api/tv-shows")
	if _, ok := shows["detail"]; ok {
		t.Errorf("shows err: %v", shows)
	}

	// 日志（refresh 完成事件已写入；响应是 JSON 数组）
	reqLogs, _ := http.NewRequest("GET", ts.URL+"/api/logs?limit=50", nil)
	reqLogs.Header.Set("Authorization", "Bearer "+token)
	respLogs, err := http.DefaultClient.Do(reqLogs)
	if err != nil {
		t.Fatal(err)
	}
	var logEntries []map[string]any
	json.NewDecoder(respLogs.Body).Decode(&logEntries)
	respLogs.Body.Close()
	if len(logEntries) == 0 {
		t.Fatal("日志为空，refresh 事件未写入")
	}
}

func TestIngestEndpointsEmpty(t *testing.T) {
	ts, token := newScanServer(t)
	code, history := authedGet(t, ts, token, "/api/ingest/history")
	if code != 200 {
		t.Fatalf("history code = %d", code)
	}
	if history["pagination"] == nil {
		t.Errorf("history = %v", history)
	}
	_, stats := authedGet(t, ts, token, "/api/ingest/stats?days=7")
	if stats["days"] != float64(7) {
		t.Errorf("stats = %v", stats)
	}
}

func TestPipelineStatusAndWebhook(t *testing.T) {
	ts, token := newScanServer(t)
	code, status := authedGet(t, ts, token, "/api/pipeline/status")
	if code != 200 || status["running"] != false {
		t.Fatalf("status = %v %v", code, status)
	}
	// webhook 无密钥配置 → 直接触发
	resp, err := http.Post(ts.URL+"/trigger", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != 200 || out["status"] != "triggered" {
		t.Errorf("webhook = %d %v", resp.StatusCode, out)
	}
	// 等待后台流程完成（无 TMDB 配置，流程会快速结束）
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		_, st := authedGet(t, ts, token, "/api/pipeline/status")
		if st["running"] == false {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}

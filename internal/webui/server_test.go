package webui

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meta2cloud/internal/u115"

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

// ── 配置读写端到端 ─────────────────────────────────────────

func TestConfigEndpoints(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	cfgYAML := "webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: test\ntelegram:\n  bot_token: tg-token\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	os.WriteFile(filepath.Join(rootDir, "config", "parser-rules.yaml"),
		[]byte("parser:\n  custom_words:\n    - 测试词\n  custom_release_groups:\n    - FSD\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	// 读取合并配置（含 parser 规则合并）
	code, cfgData := authedGet(t, ts, token, "/api/config")
	if code != 200 {
		t.Fatalf("config code = %d", code)
	}
	parser, ok := cfgData["parser"].(map[string]any)
	if !ok {
		t.Fatalf("parser 段缺失: %v", cfgData)
	}
	if words, _ := parser["custom_words"].([]any); len(words) != 1 || words[0] != "测试词" {
		t.Errorf("custom_words = %v", parser["custom_words"])
	}

	// 解析测试（跳过 TMDB）
	parseReq, _ := http.NewRequest("POST", ts.URL+"/api/parser/test?skip_tmdb=true",
		strings.NewReader(`{"filename": "Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG.mkv"}`))
	parseReq.Header.Set("Authorization", "Bearer "+token)
	parseReq.Header.Set("Content-Type", "application/json")
	parseResp, err := http.DefaultClient.Do(parseReq)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	json.NewDecoder(parseResp.Body).Decode(&parsed)
	parseResp.Body.Close()
	if parseResp.StatusCode != 200 || parsed["name"] != "Breaking Bad" {
		t.Errorf("parse = %d %v", parseResp.StatusCode, parsed["name"])
	}
	if parsed["release_group"] != "TTG" {
		t.Errorf("release_group = %v", parsed["release_group"])
	}

	// 保存主配置（校验+持久化+运行时失效）
	saveBody := map[string]any{"data": map[string]any{
		"webui":    map[string]any{"username": "admin", "password": "secret123", "token_expire_hours": 48},
		"telegram": map[string]any{"bot_token": "tg-token", "debounce_seconds": "10"},
	}}
	saveJSON, _ := json.Marshal(saveBody)
	saveReq, _ := http.NewRequest("PUT", ts.URL+"/api/config/main", bytes.NewReader(saveJSON))
	saveReq.Header.Set("Authorization", "Bearer "+token)
	saveReq.Header.Set("Content-Type", "application/json")
	saveResp, err := http.DefaultClient.Do(saveReq)
	if err != nil {
		t.Fatal(err)
	}
	var saveOut map[string]any
	json.NewDecoder(saveResp.Body).Decode(&saveOut)
	saveResp.Body.Close()
	if saveResp.StatusCode != 200 {
		t.Fatalf("save = %d %v", saveResp.StatusCode, saveOut)
	}
	// 重读：debounce 字符串应被规范化为整数 10
	_, reread := authedGet(t, ts, token, "/api/config/main")
	tg := reread["telegram"].(map[string]any)
	if tg["debounce_seconds"] != float64(10) {
		t.Errorf("debounce_seconds = %v (%T), want 10", tg["debounce_seconds"], tg["debounce_seconds"])
	}
	webuiSec := reread["webui"].(map[string]any)
	if webuiSec["token_expire_hours"] != float64(48) {
		t.Errorf("token_expire_hours = %v", webuiSec["token_expire_hours"])
	}
	// 新配置生效（运行时缓存失效）
	if RT().GetConfig().WebUI.TokenExpireHours != 48 {
		t.Errorf("运行时未感知新配置: %d", RT().GetConfig().WebUI.TokenExpireHours)
	}

	// 越界校验
	badBody := map[string]any{"data": map[string]any{
		"webui": map[string]any{"username": "a", "password": "b", "token_expire_hours": 999999}}}
	badJSON, _ := json.Marshal(badBody)
	badReq, _ := http.NewRequest("PUT", ts.URL+"/api/config/main", bytes.NewReader(badJSON))
	badReq.Header.Set("Authorization", "Bearer "+token)
	badResp, err := http.DefaultClient.Do(badReq)
	if err != nil {
		t.Fatal(err)
	}
	badResp.Body.Close()
	if badResp.StatusCode != 400 {
		t.Errorf("越界值 code = %d, want 400", badResp.StatusCode)
	}
}

// ── 115 OAuth 与云下载端到端 ───────────────────────────────

func newU115TestServer(t *testing.T, confirmed bool) (*httptest.Server, string) {
	t.Helper()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/open/authDeviceCode"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0,
				"data": map[string]any{"qrcode": "https://115.com/qr?token=x", "uid": "u1", "time": "123", "sign": "s1"}})
		case strings.HasSuffix(r.URL.Path, "/get/status/"):
			status := 2
			if !confirmed {
				status = 0
			}
			json.NewEncoder(w).Encode(map[string]any{"state": true, "data": map[string]any{"status": status, "msg": ""}})
		case strings.HasSuffix(r.URL.Path, "/open/deviceCodeToToken"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0,
				"data": map[string]any{"access_token": "at", "refresh_token": "rt", "expires_in": 3600}})
		case strings.HasSuffix(r.URL.Path, "/open/offline/get_task_list"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": map[string]any{
				"page": 1, "page_count": 1, "count": 1,
				"tasks": []any{map[string]any{
					"info_hash": "h1", "name": "任务1", "status": 2, "percentDone": 100.0,
					"size": 1024.0, "add_time": 1700000000.0, "last_update": 1700000100.0,
				}}}})
		case strings.HasSuffix(r.URL.Path, "/open/offline/get_quota_info"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": map[string]any{
				"count": 10, "used": 3, "surplus": 7, "package": []any{}}})
		case strings.HasSuffix(r.URL.Path, "/open/offline/add_task_urls"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0,
				"data": []any{map[string]any{"state": true, "code": 0, "message": "ok", "url": "magnet:?xt=1"}}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": map[string]any{}})
		}
	}))
	t.Cleanup(apiServer.Close)

	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	cfgYAML := "webui:\n  username: admin\n  password: secret123\nu115:\n  client_id: test-cid\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	client := u115.NewClient("test-cid",
		u115.WithToken(u115.Token{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, RefreshTime: time.Now().Unix()}),
		u115.WithQPS(1000, 1000))
	client.SetTestBaseURL(apiServer.URL)
	RT().SetU115ClientForTest(client)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	return ts, out["token"].(string)
}

func TestU115OauthFlow(t *testing.T) {
	ts, token := newU115TestServer(t, true)

	// 初始状态：无 token 文件未授权（注入客户端不算持久化 token）
	req, _ := http.NewRequest("GET", ts.URL+"/api/u115/oauth/status", nil)
	req.Header.Set("Authorization", "Bearer "+authToken(ts, t))
	resp, _ := http.DefaultClient.Do(req)
	var status map[string]any
	json.NewDecoder(resp.Body).Decode(&status)
	resp.Body.Close()
	if status["client_id"] != "test-cid" {
		t.Errorf("status = %v", status)
	}

	// 创建扫码会话
	createReq, _ := http.NewRequest("POST", ts.URL+"/api/u115/oauth/create", strings.NewReader("{}"))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatal(err)
	}
	var created map[string]any
	json.NewDecoder(createResp.Body).Decode(&created)
	createResp.Body.Close()
	if created["ok"] != true || created["uid"] != "u1" {
		t.Fatalf("create = %v", created)
	}

	// 二维码 PNG
	qrReq, _ := http.NewRequest("GET", ts.URL+"/api/u115/oauth/qrcode", nil)
	qrReq.Header.Set("Authorization", "Bearer "+token)
	qrResp, err := http.DefaultClient.Do(qrReq)
	if err != nil {
		t.Fatal(err)
	}
	qrData := make([]byte, 4)
	io.ReadFull(qrResp.Body, qrData)
	qrResp.Body.Close()
	if qrResp.Header.Get("Content-Type") != "image/png" || string(qrData[:4]) != "\x89PNG" {
		t.Errorf("qrcode = %s %v", qrResp.Header.Get("Content-Type"), qrData[:4])
	}

	// 轮询（已确认 → 换取 token → 会话文件删除）
	pollReq, _ := http.NewRequest("GET", ts.URL+"/api/u115/oauth/poll", nil)
	pollReq.Header.Set("Authorization", "Bearer "+token)
	pollResp, err := http.DefaultClient.Do(pollReq)
	if err != nil {
		t.Fatal(err)
	}
	var pollOut map[string]any
	json.NewDecoder(pollResp.Body).Decode(&pollOut)
	pollResp.Body.Close()
	if pollOut["ok"] != true || pollOut["confirmed"] != true {
		t.Errorf("poll = %v", pollOut)
	}
}

func authToken(ts *httptest.Server, t *testing.T) string {
	t.Helper()
	_, out := login(t, ts, "admin", "secret123")
	return out["token"].(string)
}

func TestU115OfflineEndpoints(t *testing.T) {
	ts, token := newU115TestServer(t, true)

	// 任务列表
	code, overview := authedGet(t, ts, token, "/api/u115/offline/overview")
	if code != 200 || overview["ok"] != true {
		t.Fatalf("overview = %d %v", code, overview)
	}
	tasks := overview["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %v", tasks)
	}
	task := tasks[0].(map[string]any)
	if task["is_finished"] != true || task["name"] != "任务1" {
		t.Errorf("task = %v", task)
	}

	// 配额
	_, quota := authedGet(t, ts, token, "/api/u115/offline/quota")
	quotaData := quota["quota"].(map[string]any)
	if quotaData["surplus"] != float64(7) {
		t.Errorf("quota = %v", quotaData)
	}

	// 添加链接
	addReq, _ := http.NewRequest("POST", ts.URL+"/api/u115/offline/add-urls",
		strings.NewReader(`{"urls": "magnet:?xt=1\nmagnet:?xt=2"}`))
	addReq.Header.Set("Authorization", "Bearer "+token)
	addReq.Header.Set("Content-Type", "application/json")
	addResp, err := http.DefaultClient.Do(addReq)
	if err != nil {
		t.Fatal(err)
	}
	var addOut map[string]any
	json.NewDecoder(addResp.Body).Decode(&addOut)
	addResp.Body.Close()
	if addOut["ok"] != true || addOut["count"] != float64(1) {
		t.Errorf("add = %v", addOut)
	}

	// 空链接拒绝
	emptyReq, _ := http.NewRequest("POST", ts.URL+"/api/u115/offline/add-urls",
		strings.NewReader(`{"urls": "  \n "}`))
	emptyReq.Header.Set("Authorization", "Bearer "+token)
	emptyResp, _ := http.DefaultClient.Do(emptyReq)
	emptyResp.Body.Close()
	if emptyResp.StatusCode != 400 {
		t.Errorf("empty urls code = %d", emptyResp.StatusCode)
	}
}

package webui

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/jsonx"
	"meta2cloud/internal/tmdb"
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
	cfgYAML := "webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	// 构造假网盘（分类布局）：媒体库根/电影/电影 (2010)；媒体库根/剧集/剧 (2008)/Season 01/S01E01
	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0") // 配置 root_folder_id 存目录名，扫描时按名解析出真实 ID
	movieCat := fp.addFolder("电影", root.ID)
	tvCat := fp.addFolder("剧集", root.ID)
	movie := fp.addFolder("电影 (2010)", movieCat.ID)
	_ = movie
	show := fp.addFolder("剧 (2008)", tvCat.ID)
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

	// 日志（refresh 完成事件已写入；结构为 {items, summary, maxId}）
	reqLogs, _ := http.NewRequest("GET", ts.URL+"/api/logs?limit=50", nil)
	reqLogs.Header.Set("Authorization", "Bearer "+token)
	respLogs, err := http.DefaultClient.Do(reqLogs)
	if err != nil {
		t.Fatal(err)
	}
	var logsPayload struct {
		Items   []map[string]any `json:"items"`
		Summary map[string]any   `json:"summary"`
		MaxID   int64            `json:"maxId"`
	}
	json.NewDecoder(respLogs.Body).Decode(&logsPayload)
	respLogs.Body.Close()
	if len(logsPayload.Items) == 0 {
		t.Fatal("日志为空，refresh 事件未写入")
	}
	if logsPayload.Summary["retentionDays"] == nil {
		t.Errorf("summary = %v", logsPayload.Summary)
	}
	// 空库时 items 必须是 [] 而非 null(前端 res.data.items 会抛错)
	if logsPayload.Items == nil {
		t.Fatal("items 为 null，应为空数组")
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
		u115.WithQPS(1000))
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

// ── .torrent 链接推送（BT 任务） ───────────────────────────

// fakeTorrentProvider 测试用 Provider：记录 UploadBytes 内容并返回固定 pick_code。
type fakeTorrentProvider struct {
	uploadedName  string
	uploadedBytes []byte
}

func (f *fakeTorrentProvider) ProviderName() string { return "fake" }
func (f *fakeTorrentProvider) ListFiles(string, int) ([]cloudfile.CloudFile, error) {
	return nil, nil
}
func (f *fakeTorrentProvider) ListAllFiles(string) ([]cloudfile.CloudFile, error) {
	return nil, nil
}
func (f *fakeTorrentProvider) GetFile(fileID string) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{ID: fileID}, nil
}
func (f *fakeTorrentProvider) ReadText(cloudfile.CloudFile) (string, error) { return "", nil }
func (f *fakeTorrentProvider) FindFile(name string, _ string) (*cloudfile.CloudFile, error) {
	if f.uploadedName == name {
		return &cloudfile.CloudFile{ID: "f1", Name: name, FileType: cloudfile.TypeFile,
			Extra: map[string]any{"pick_code": "pick123"}}, nil
	}
	return nil, nil
}
func (f *fakeTorrentProvider) RenameFile(fileID, newName string) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{ID: fileID, Name: newName}, nil
}
func (f *fakeTorrentProvider) MoveFile(fileID, _, _ string) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{ID: fileID}, nil
}
func (f *fakeTorrentProvider) UploadText(content string, name, _, _ string, _ bool) (cloudfile.CloudFile, error) {
	return f.UploadBytes([]byte(content), name, "", "", false)
}
func (f *fakeTorrentProvider) UploadBytes(content []byte, name, _, _ string, _ bool) (cloudfile.CloudFile, error) {
	f.uploadedName = name
	f.uploadedBytes = content
	return cloudfile.CloudFile{ID: "f1", Name: name, FileType: cloudfile.TypeFile,
		Extra: map[string]any{"pick_code": "pick123"}}, nil
}
func (f *fakeTorrentProvider) TrashFile(string) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{}, nil
}
func (f *fakeTorrentProvider) DeleteFile(string) error { return nil }
func (f *fakeTorrentProvider) CreateFolder(name, _ string) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{ID: "c1", Name: name, FileType: cloudfile.TypeFolder}, nil
}

func TestIsTorrentLink(t *testing.T) {
	cases := map[string]bool{
		"https://anibt.net/api/torrent/rel_xxx.torrent":                    true,
		"https://mikan/Download/x/" + strings.Repeat("a", 40) + ".TORRENT": true,
		"magnet:?xt=urn:btih:abc":                                          false,
		"https://example.com/file":                                         false,
		"https://example.com/a.mkv":                                        false,
		"":                                                                 false,
	}
	for raw, want := range cases {
		if got := isTorrentLink(raw); got != want {
			t.Errorf("isTorrentLink(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestPushTorrentLinkAddsBTTask(t *testing.T) {
	torrentBytes := []byte("d4:infod6:lengthi123e6:name5:helloee")
	torrentSHA1 := fmt.Sprintf("%x", sha1.Sum(torrentBytes))

	// mock 种子下载站
	torrentSite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(torrentBytes)
	}))
	t.Cleanup(torrentSite.Close)

	// mock 115 API：记录 add_task_bt 参数
	var btForm url.Values
	u115Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch {
		case strings.HasSuffix(r.URL.Path, "/open/folder/get_info"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0,
				"data": map[string]any{"file_id": "100", "file_name": "种子文件"}})
		case strings.HasSuffix(r.URL.Path, "/open/offline/torrent"):
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0,
				"data": []any{map[string]any{
					"info_hash": "abc123", "torrent_name": "hello", "file_size": 123, "file_count": 1,
					"torrent_filelist": []any{map[string]any{"path": "hello.mp4", "size": 123, "wanted": 1}},
				}}})
		case strings.HasSuffix(r.URL.Path, "/open/offline/add_task_bt"):
			btForm = r.PostForm
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": map[string]any{}})
		default:
			json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": map[string]any{}})
		}
	}))
	t.Cleanup(u115Server.Close)

	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	cfgYAML := "webui:\n  username: admin\n  password: secret123\n" +
		"u115:\n  client_id: cid\n  download_folder_id: \"555\"\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	// 持久化 token 文件 → authorized = true
	tokenData := fmt.Sprintf(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"refresh_time":%d}`, time.Now().Unix())
	os.WriteFile(filepath.Join(rootDir, "config", "115-token.json"), []byte(tokenData), 0o600)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	client := u115.NewClient("cid",
		u115.WithToken(u115.Token{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, RefreshTime: time.Now().Unix()}),
		u115.WithQPS(1000))
	client.SetTestBaseURL(u115Server.URL)
	RT().SetU115ClientForTest(client)
	provider := &fakeTorrentProvider{}
	RT().SetProviderForTest(provider)

	// 无 magnet、仅 .torrent 链接的条目 → 应创建 BT 任务而非链接任务
	err := pushItem("u115", map[string]any{
		"torrent_url": torrentSite.URL + "/api/torrent/rel_xxx.torrent",
	})
	if err != nil {
		t.Fatalf("pushItem = %v", err)
	}
	if btForm == nil {
		t.Fatal("add_task_bt 未被调用")
	}
	if got := btForm.Get("info_hash"); got != "abc123" {
		t.Errorf("info_hash = %q", got)
	}
	if got := btForm.Get("torrent_sha1"); got != torrentSHA1 {
		t.Errorf("torrent_sha1 = %q, want %q", got, torrentSHA1)
	}
	if got := btForm.Get("pick_code"); got != "pick123" {
		t.Errorf("pick_code = %q", got)
	}
	if got := btForm.Get("wp_path_id"); got != "555" {
		t.Errorf("wp_path_id = %q, want 555（下载目录）", got)
	}
	if got := btForm.Get("wanted"); got != "0" {
		t.Errorf("wanted = %q, want 0（种子内标记的文件）", got)
	}
	if string(provider.uploadedBytes) != string(torrentBytes) {
		t.Errorf("上传内容不匹配：%q", provider.uploadedBytes)
	}
	if provider.uploadedName != "rel_xxx.torrent" {
		t.Errorf("上传文件名 = %q", provider.uploadedName)
	}
}

// ── RSS 订阅端到端 ─────────────────────────────────────────

func newSubscriptionTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	// mock mikan RSS 站点
	mikanServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/RSS/Bangumi") {
			w.Header().Set("Content-Type", "application/rss+xml")
			w.Write([]byte(`<?xml version="1.0"?>
				<rss><channel>
				<item><title>[GroupX] 测试剧 - 01 [1080p]</title>
					<pubDate>Tue, 13 Oct 2026 08:00:00 GMT</pubDate>
					<enclosure url="https://mikan/Download/x/091b1b30d24c41759da1b0e24ddfffc89925c4.torrent" length="104857600"/></item>
				<item><title>[GroupX] 测试剧 - 02 [1080p]</title>
					<pubDate>Tue, 13 Oct 2026 09:00:00 GMT</pubDate>
					<enclosure url="https://mikan/Download/y/091b1b30d24c4d41759da1b0e24ddfffc89925c5.torrent" length="104857600"/></item>
				</channel></rss>`))
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(mikanServer.Close)

	// mock 115 API（add_task_urls 接收推送）
	u115Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0,
			"data": []any{map[string]any{"state": true, "code": 0, "message": "ok"}}})
	}))
	t.Cleanup(u115Server.Close)

	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config", "data"), 0o755)
	cfgYAML := "webui:\n  username: admin\n  password: secret123\nu115:\n  client_id: cid\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	// 持久化 token 文件 → authorized = true
	tokenData := fmt.Sprintf(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"refresh_time":%d}`, time.Now().Unix())
	os.WriteFile(filepath.Join(rootDir, "config", "115-token.json"), []byte(tokenData), 0o600)

	ResetJWTSecretCache()
	InitRuntime(rootDir)

	// 注入测试 115 客户端
	u115Client := u115.NewClient("cid",
		u115.WithToken(u115.Token{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, RefreshTime: time.Now().Unix()}),
		u115.WithQPS(1000))
	u115Client.SetTestBaseURL(u115Server.URL)
	RT().SetU115ClientForTest(u115Client)

	// 注入测试 mikan spider
	mikanTestSpider(t).SetTestBaseURL(mikanServer.URL)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	return ts, out["token"].(string)
}

func authedRequest(t *testing.T, ts *httptest.Server, token, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, ts.URL+path, reader)
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func TestSubscriptionCRUDAndCheck(t *testing.T) {
	ts, token := newSubscriptionTestServer(t)
	rssURL := mikanTestRSSURL(ts)

	// 解析 RSS 建议
	code, parsed := authedRequest(t, ts, token, "POST", "/api/subscriptions:parse-rss",
		map[string]any{"rss_url": rssURL})
	if code != 200 || parsed["success"] != true {
		t.Fatalf("parse-rss = %d %v", code, parsed)
	}
	if parsed["media_title"] != "测试剧" || parsed["subgroup_name"] != "GroupX" {
		t.Errorf("parsed = %v", parsed)
	}

	// 测试匹配（第 2 集起）
	testBody := map[string]any{
		"site": "mikan", "rss_url": rssURL, "season_number": 1, "start_episode": 2,
		"keyword_all": []string{"groupx"},
	}
	code, testOut := authedRequest(t, ts, token, "POST", "/api/subscriptions:test", testBody)
	if code != 200 {
		t.Fatalf("test = %d %v", code, testOut)
	}
	summary := testOut["summary"].(map[string]any)
	if summary["total_items"] != float64(2) || summary["matched_items"] != float64(1) {
		t.Errorf("summary = %v (第2集起应只匹配 1 条)", summary)
	}

	// 创建订阅（推送目标 u115）
	createBody := map[string]any{
		"name": "测试剧 [GroupX]", "media_title": "测试剧", "media_type": "tv",
		"site": "mikan", "rss_url": rssURL, "subgroup_name": "GroupX",
		"season_number": 1, "start_episode": 2, "keyword_all": []string{"GroupX"},
		"push_target": "u115", "enabled": true,
	}
	code, created := authedRequest(t, ts, token, "POST", "/api/subscriptions", createBody)
	if code != 200 || created["ok"] != true {
		t.Fatalf("create = %d %v", code, created)
	}
	subID := int64(1)

	// 重复创建 → 409
	code, _ = authedRequest(t, ts, token, "POST", "/api/subscriptions", createBody)
	if code != 409 {
		t.Errorf("duplicate code = %d, want 409", code)
	}

	// 手动检查 → 命中 1 条并推送（mock 115）
	code, checked := authedRequest(t, ts, token, "POST",
		fmt.Sprintf("/api/subscriptions/%d:check", subID), nil)
	if code != 200 || checked["ok"] != true {
		t.Fatalf("check = %d %v", code, checked)
	}
	result := checked["result"].(map[string]any)
	if result["pushed"] != float64(1) {
		t.Errorf("result = %v (应推送 1 条)", result)
	}

	// 再次检查 → 去重跳过
	_, checked2 := authedRequest(t, ts, token, "POST",
		fmt.Sprintf("/api/subscriptions/%d:check", subID), nil)
	result2 := checked2["result"].(map[string]any)
	if result2["pushed"] != float64(0) || result2["skipped"] != float64(1) {
		t.Errorf("result2 = %v (应去重跳过)", result2)
	}

	// 列表
	code, listOut := authedRequest(t, ts, token, "GET", "/api/subscriptions", nil)
	if code != 200 {
		t.Fatalf("list = %d", code)
	}
	subs := listOut["subscriptions"].([]any)
	if len(subs) != 1 {
		t.Fatalf("subscriptions = %d", len(subs))
	}
	sub := subs[0].(map[string]any)
	if sub["hit_count"] != float64(1) {
		t.Errorf("hit_count = %v", sub["hit_count"])
	}

	// 更新 → 修改起始集数避免重复
	updateBody := createBody
	updateBody["start_episode"] = 5
	code, updated := authedRequest(t, ts, token, "PUT",
		fmt.Sprintf("/api/subscriptions/%d", subID), updateBody)
	if code != 200 || updated["ok"] != true {
		t.Fatalf("update = %d %v", code, updated)
	}

	// 删除
	code, _ = authedRequest(t, ts, token, "DELETE", fmt.Sprintf("/api/subscriptions/%d", subID), nil)
	if code != 200 {
		t.Errorf("delete code = %d", code)
	}
	// 再删 → 404
	code, _ = authedRequest(t, ts, token, "DELETE", fmt.Sprintf("/api/subscriptions/%d", subID), nil)
	if code != 404 {
		t.Errorf("redelete code = %d, want 404", code)
	}
}

func mikanTestRSSURL(_ *httptest.Server) string {
	return "https://mikanani.me/RSS/Bangumi?bangumiId=3065"
}

// 同一部剧多个译名目录 → 刷新后集数聚合(令和的斑小姐场景)
func TestScanAggregatesDuplicateShowFolders(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0") // 配置 root_folder_id 存目录名，扫描时按名解析出真实 ID
	tvCat := fp.addFolder("剧集", root.ID)
	// 目录 A: 官方译名, 12 集
	showA := fp.addFolder("令和的斑小姐 (2026)", tvCat.ID)
	seasonA := fp.addFolder("Season 01", showA.ID)
	for ep := 1; ep <= 12; ep++ {
		fp.addFile(fmt.Sprintf("令和的斑小姐 - S01E%02d.mkv", ep), seasonA.ID)
	}
	// 目录 B: 民间译名, 仅 1 集(E06)
	showB := fp.addFolder("令和妖神斑小姐 (2026)", tvCat.ID)
	seasonB := fp.addFolder("Season 01", showB.ID)
	fp.addFile("令和妖神斑小姐 - S01E06.mkv", seasonB.ID)
	RT().SetProviderForTest(fp.provider())

	// 预置库条目: 两个目录都已关联同一 tmdb_id(手动修正后的状态)
	libStore := RT().GetLibraryStore()
	libStore.SaveIncremental([]map[string]any{
		{"drive_folder_id": showA.ID, "media_type": "tv", "tmdb_id": 312507, "title": "令和的斑小姐", "year": "2026"},
		{"drive_folder_id": showB.ID, "media_type": "tv", "tmdb_id": 312507, "title": "令和妖神斑小姐", "year": "2026"},
	}, nil)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	// 全量刷新
	code, _ := authedRequest(t, ts, token, "POST", "/api/library:refresh?full=true", nil)
	if code != 200 {
		t.Fatalf("refresh code = %d", code)
	}

	// 同 tmdb_id 的两个目录合并为一张卡片，集数取聚合值 12
	// (A 有 E01-E12，B 只有 E06，并集 = 12)
	_, lib := authedGet(t, ts, token, "/api/library")
	shows, _ := lib["tv_shows"].([]any)
	if len(shows) != 1 {
		t.Fatalf("tv_shows = %d, want 1(同 tmdb_id 合并为一张卡片)", len(shows))
	}
	show := shows[0].(map[string]any)
	if show["in_library_episodes"] != float64(12) {
		t.Errorf("%q in_library_episodes = %v, want 12(跨目录聚合)",
			show["title"], show["in_library_episodes"])
	}
	if show["tmdb_id"] != float64(312507) {
		t.Errorf("tmdb_id = %v", show["tmdb_id"])
	}
}

// 电影详情直链: /api/movies/{tmdbID} 命中库条目,未知 ID 返回 404
func TestGetMovieDetailByTmdbID(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	movieFolder := fp.addFolder("盗梦空间 (2010)", root.ID)
	fp.addFile("盗梦空间 (2010).mkv", movieFolder.ID)
	RT().SetProviderForTest(fp.provider())

	libStore := RT().GetLibraryStore()
	libStore.SaveIncremental([]map[string]any{
		{"drive_folder_id": movieFolder.ID, "media_type": "movie", "tmdb_id": 27205, "title": "盗梦空间", "year": "2010"},
	}, nil)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	code, movie := authedGet(t, ts, token, "/api/movies/27205")
	if code != 200 {
		t.Fatalf("movie detail code = %d", code)
	}
	if movie["tmdb_id"] != float64(27205) || movie["title"] != "盗梦空间" {
		t.Errorf("movie = %v", movie)
	}

	code, _ = authedGet(t, ts, token, "/api/movies/999999")
	if code != 404 {
		t.Fatalf("missing movie code = %d, want 404", code)
	}
}

// 增量刷新: 只变 B 目录时,同 tmdb_id 的 A 也应联动重扫聚合集数
func TestIncrementalSiblingRescan(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0") // 配置 root_folder_id 存目录名，扫描时按名解析出真实 ID
	tvCat := fp.addFolder("剧集", root.ID)
	showA := fp.addFolder("令和的斑小姐 (2026)", tvCat.ID)
	seasonA := fp.addFolder("Season 01", showA.ID)
	for ep := 1; ep <= 12; ep++ {
		if ep == 6 {
			continue // A 缺 E06
		}
		fp.addFile(fmt.Sprintf("令和的斑小姐 - S01E%02d.mkv", ep), seasonA.ID)
	}
	showB := fp.addFolder("令和妖神斑小姐 (2026)", tvCat.ID)
	seasonB := fp.addFolder("Season 01", showB.ID)
	fp.addFile("令和妖神斑小姐 - S01E06.mkv", seasonB.ID)
	// 剧集增量时间按 Season 叶子目录计算，初始 mtime 一致
	for _, id := range []string{showA.ID, seasonA.ID, showB.ID, seasonB.ID} {
		fp.setMtime(id, 1000)
	}
	RT().SetProviderForTest(fp.provider())

	// 预置库条目: 两个目录都已关联同一 tmdb_id(手动修正后的状态)
	libStore := RT().GetLibraryStore()
	libStore.SaveIncremental([]map[string]any{
		{"drive_folder_id": showA.ID, "media_type": "tv", "tmdb_id": 312507, "title": "令和的斑小姐", "year": "2026"},
		{"drive_folder_id": showB.ID, "media_type": "tv", "tmdb_id": 312507, "title": "令和妖神斑小姐", "year": "2026"},
	}, nil)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	// 首次全量: 两目录聚合并集 11+1=12(A 缺 E06，B 只有 E06)
	if code, ref := authedRequest(t, ts, token, "POST", "/api/library:refresh?full=true", nil); code != 200 {
		t.Fatalf("full refresh failed: %d %v", code, ref)
	}
	_, lib := authedGet(t, ts, token, "/api/library")
	shows, _ := lib["tv_shows"].([]any)
	if len(shows) != 1 || shows[0].(map[string]any)["in_library_episodes"] != float64(12) {
		t.Fatalf("首次全量: %+v", shows)
	}

	// 模拟旧版本遗留的未聚合行: A 行回退为 11(mtime 不变)，B 行保持 12
	libStore.SaveIncremental([]map[string]any{
		{"drive_folder_id": showA.ID, "media_type": "tv", "tmdb_id": 312507,
			"title": "令和的斑小姐", "year": "2026", "in_library_episodes": 11},
	}, map[string]int64{showA.ID: 1000})

	// 增量: 仅 B 的 Season 叶子 mtime 变化，A 未变
	fp.addFile("令和妖神斑小姐 - S01E06v2.mkv", seasonB.ID)
	fp.setMtime(seasonB.ID, 2000)
	if code, _ := authedRequest(t, ts, token, "POST", "/api/library:refresh", nil); code != 200 {
		t.Fatal("incremental refresh failed")
	}

	// 联动重扫后 A 行应被重写为聚合值 12，卡片保持一张
	itemA := RT().GetLibraryStore().GetLibraryItemByFolderID(showA.ID)
	if itemA == nil || jsonx.Int(itemA, "in_library_episodes", 0) != 12 {
		t.Fatalf("增量联动: A 行未联动重扫, in_library_episodes = %+v", itemA)
	}
	_, lib = authedGet(t, ts, token, "/api/library")
	shows, _ = lib["tv_shows"].([]any)
	if len(shows) != 1 || shows[0].(map[string]any)["in_library_episodes"] != float64(12) {
		t.Fatalf("增量联动卡片: %+v", shows)
	}
}

// 分类启用时,扫描应把一级/二级分类目录当透传层,正确识别其中的媒体
func TestScanCategoryPassthrough(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	ji := fp.addFolder("剧集", root.ID)
	anime := fp.addFolder("动漫", ji.ID)
	show := fp.addFolder("学生会也有洞！ (2026)", anime.ID)
	season := fp.addFolder("Season 01", show.ID)
	fp.addFile("学生会也有洞！ - S01E01.mkv", season.ID)
	fp.addFolder("电影", root.ID)
	RT().SetProviderForTest(fp.provider())

	RT().SetCategories(&config.CategoriesConfig{
		Dirs: config.CategoryDirs{TV: "剧集", Movie: "电影", Other: "其他"},
		TV: []config.CategoryRule{
			{Name: "动漫", GenreIDs: []int{16}, Languages: []string{"ja"}},
			{Name: "普通电视剧"},
		},
	})

	shows := ScanTVShows(fp.provider(), RT().GetConfig(), false)
	if len(shows) != 1 {
		t.Fatalf("shows = %d, want 1（分类目录应透传）", len(shows))
	}
	if !strings.Contains(shows[0].Title, "学生会也有洞") {
		t.Errorf("title = %q", shows[0].Title)
	}
}

// 修正识别后媒体目录应按新 TMDB 流派重新归类（移动到新一级/二级分类目录下）
func TestReidentifyReclassifiesFolder(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: k\nu115:\n  root_folder_id: \"root\"\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	// TMDB mock：新识别的媒体带动画流派（16）与日语
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/season/") {
			json.NewEncoder(w).Encode(map[string]any{"name": "Season 1", "episodes": []any{}})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/tv/") {
			json.NewEncoder(w).Encode(map[string]any{
				"id": 325052, "name": "从后面来的神威先生", "original_name": "kami-senpai",
				"original_language": "ja", "first_air_date": "2026-04-01",
				"overview": "test", "poster_path": "/p.jpg", "backdrop_path": "/b.jpg",
				"number_of_episodes": 12, "status": "Returning Series",
				"genres":          []any{map[string]any{"id": 16, "name": "动画"}},
				"credits":         map[string]any{"crew": []any{}, "cast": []any{}},
				"external_ids":    map[string]any{},
				"content_ratings": map[string]any{"results": []any{}},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer tmdbServer.Close()
	tmdbClient := tmdb.NewClient("k", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(tmdbServer.URL)
	RT().SetTmdbClientForTest(tmdbClient)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	tvDir := fp.addFolder("剧集", root.ID)
	wrongCat := fp.addFolder("普通电视剧", tvDir.ID)
	rightCat := fp.addFolder("动漫", tvDir.ID)
	show := fp.addFolder("误识别的剧 (2026)", wrongCat.ID)
	season := fp.addFolder("Season 01", show.ID)
	video := fp.addFile("误识别的剧 - S01E01.mkv", season.ID)
	subtitle := fp.addFile("误识别的剧 - S01E01.chs.srt", season.ID)
	RT().SetProviderForTest(fp.provider())

	RT().SetCategories(&config.CategoriesConfig{
		Dirs: config.CategoryDirs{TV: "剧集", Movie: "电影", Other: "其他"},
		TV: []config.CategoryRule{
			{Name: "动漫", GenreIDs: []int{16}, Languages: []string{"ja"}},
			{Name: "普通电视剧"},
		},
	})

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	code, resp := authedRequest(t, ts, token, "POST", "/api/library:reidentify", map[string]any{
		"drive_folder_id": show.ID, "media_type": "tv", "tmdb_id": 325052,
		"title": "从后面来的神威先生", "rename_folder": true,
	})
	if code != 200 {
		t.Fatalf("reidentify code = %d: %v", code, resp)
	}
	if resp["ok"] != true || resp["reclassified"] != true {
		t.Fatalf("reidentify 结果异常: %v", resp)
	}
	if resp["category"] != "剧集/动漫" {
		t.Fatalf("分类错误: %v", resp["category"])
	}
	// 目录与目录内媒体文件都应按新识别改名
	if resp["renamed"] != true || resp["folder_name"] != "从后面来的神威先生 (2026)" {
		t.Fatalf("目录改名异常: renamed=%v folder_name=%v", resp["renamed"], resp["folder_name"])
	}
	if resp["files_renamed"] != float64(2) {
		t.Fatalf("files_renamed = %v, want 2（视频+字幕）", resp["files_renamed"])
	}
	if got, _ := fp.GetFile(video.ID); got.Name != "从后面来的神威先生 - S01E01.mkv" {
		t.Fatalf("视频文件未按新名改名: %q", got.Name)
	}
	if got, _ := fp.GetFile(subtitle.ID); got.Name != "从后面来的神威先生 - S01E01.chs.srt" {
		t.Fatalf("字幕文件未随视频改名: %q", got.Name)
	}
	moved, err := fp.GetFile(show.ID)
	if err != nil || moved.ParentID != rightCat.ID {
		t.Fatalf("目录应移动到 动漫 分类下: parent=%v err=%v", moved.ParentID, err)
	}
	reloaded, _ := fp.GetFile(rightCat.ID)
	children, _ := fp.ListFiles(reloaded.ID, 100)
	found := false
	for _, c := range children {
		if c.ID == show.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("动漫 分类下应包含移动后的目录")
	}
}

// 全库重新归类：干跑统计 + 后台执行 + 错放目录移动到正确分类
func TestCategoriesReclassifyFlow(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	strmLocal := filepath.Join(t.TempDir(), "media")
	cfgYAML := "webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: k\nu115:\n  root_folder_id: \"root\"\nstrm:\n  enabled: true\n  url_base: \"http://127.0.0.1:38765\"\n  mappings:\n    - pan_folder_id: \"root\"\n      local_path: \"" + strmLocal + "\"\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	tvDir := fp.addFolder("剧集", root.ID)
	anime := fp.addFolder("动漫", tvDir.ID)
	normal := fp.addFolder("普通电视剧", tvDir.ID)
	wrongShow := fp.addFolder("错放的动漫 (2026)", normal.ID) // 属于动漫却放在普通电视剧
	rightShow := fp.addFolder("普通剧 (2025)", normal.ID)
	RT().SetProviderForTest(fp.provider())

	RT().SetCategories(&config.CategoriesConfig{
		Dirs: config.CategoryDirs{TV: "剧集", Movie: "电影", Other: "其他"},
		TV: []config.CategoryRule{
			{Name: "动漫", GenreIDs: []int{16}, Languages: []string{"ja"}},
			{Name: "普通电视剧"},
		},
	})

	ls := RT().GetLibraryStore()
	_ = RT().GetStrmStore() // 预开 STRM 存储：归位完成触发的异步同步复用句柄，避免清理阶段懒建目录

	// TMDB mock：执行阶段会绕缓存拉最新详情重判分类，返回与上方缓存一致的判定
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/tv/325052"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": 325052, "name": "错放的动漫", "original_language": "ja",
				"genres":          []any{map[string]any{"id": 16, "name": "动画"}},
				"credits":         map[string]any{"crew": []any{}, "cast": []any{}},
				"external_ids":    map[string]any{},
				"content_ratings": map[string]any{"results": []any{}},
			})
		case strings.HasPrefix(r.URL.Path, "/tv/999"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": 999, "name": "普通剧", "original_language": "ja",
				"genres":          []any{map[string]any{"id": 18, "name": "剧情"}},
				"credits":         map[string]any{"crew": []any{}, "cast": []any{}},
				"external_ids":    map[string]any{},
				"content_ratings": map[string]any{"results": []any{}},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer tmdbServer.Close()
	tmdbClient := tmdb.NewClient("k", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(tmdbServer.URL)
	RT().SetTmdbClientForTest(tmdbClient)

	ls.SaveIncremental([]map[string]any{
		{"drive_folder_id": wrongShow.ID, "media_type": "tv", "tmdb_id": 325052, "title": "错放的动漫", "year": "2026"},
		{"drive_folder_id": rightShow.ID, "media_type": "tv", "tmdb_id": 999, "title": "普通剧", "year": "2025"},
	}, nil)
	expires := float64(time.Now().Unix()) + 3600
	_ = ls.UpsertTmdbDetail("tv", 325052, "zh-CN", map[string]any{
		"name": "错放的动漫", "original_language": "ja",
		"genres": []any{map[string]any{"id": 16, "name": "动画"}},
	}, expires)
	_ = ls.UpsertTmdbDetail("tv", 999, "zh-CN", map[string]any{
		"name": "普通剧", "original_language": "ja",
		"genres": []any{map[string]any{"id": 18, "name": "剧情"}},
	}, expires)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	// 干跑：目标分布统计
	code, dry := authedRequest(t, ts, token, "POST", "/api/categories:reclassify-dry", nil)
	if code != 200 {
		t.Fatalf("dry code = %d: %v", code, dry)
	}
	if dry["total"] != float64(2) {
		t.Fatalf("dry total = %v", dry["total"])
	}
	targets, _ := dry["targets"].(map[string]any)
	if targets["剧集/动漫"] != float64(1) || targets["剧集/普通电视剧"] != float64(1) {
		t.Fatalf("dry targets 错误: %v", targets)
	}

	// 执行
	code, exec := authedRequest(t, ts, token, "POST", "/api/categories:reclassify", nil)
	if code != 200 || exec["started"] != true {
		t.Fatalf("执行启动失败: %d %v", code, exec)
	}

	// 轮询至完成（后台执行，最多 5 秒）
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, st := authedGet(t, ts, token, "/api/categories:reclassify-status")
		if st["running"] != true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("归位任务超时未完成")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// 错放目录应已移动到 动漫；正确目录保持不动
	moved, err := fp.GetFile(wrongShow.ID)
	if err != nil || moved.ParentID != anime.ID {
		t.Fatalf("错放目录应移入 动漫: parent=%v err=%v", moved.ParentID, err)
	}
	stay, err := fp.GetFile(rightShow.ID)
	if err != nil || stay.ParentID != normal.ID {
		t.Fatalf("正确目录不应被移动: parent=%v err=%v", stay.ParentID, err)
	}

	// 等待归位触发的 STRM 同步结束，避免清理阶段与异步建库竞态
	deadline2 := time.Now().Add(5 * time.Second)
	for {
		if run := RT().GetStrmStore().GetLastFinishedSyncRun(); run != nil && run.Trigger == "reclassify" {
			break
		}
		if time.Now().After(deadline2) {
			t.Fatal("归位触发的 STRM 同步未完成")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestMediaClassificationEndpoint(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	cfgYAML := "webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: k\nu115:\n  root_folder_id: \"root\"\n"
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfgYAML), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	// TMDB mock：执行/核对均绕缓存拉最新详情，返回动画流派
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/tv/325052") {
			json.NewEncoder(w).Encode(map[string]any{
				"id": 325052, "name": "错放的动漫", "original_language": "ja",
				"genres":          []any{map[string]any{"id": 16, "name": "动画"}},
				"credits":         map[string]any{"crew": []any{}, "cast": []any{}},
				"external_ids":    map[string]any{},
				"content_ratings": map[string]any{"results": []any{}},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer tmdbServer.Close()
	tmdbClient := tmdb.NewClient("k", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(tmdbServer.URL)
	RT().SetTmdbClientForTest(tmdbClient)

	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	tvDir := fp.addFolder("剧集", root.ID)
	normal := fp.addFolder("普通电视剧", tvDir.ID)
	wrongShow := fp.addFolder("错放的动漫 (2026)", normal.ID)
	RT().SetProviderForTest(fp.provider())
	RT().SetCategories(&config.CategoriesConfig{
		Dirs: config.CategoryDirs{TV: "剧集", Movie: "电影", Other: "其他"},
		TV: []config.CategoryRule{
			{Name: "动漫", GenreIDs: []int{16}, Languages: []string{"ja"}},
			{Name: "普通电视剧"},
		},
	})
	ls := RT().GetLibraryStore()
	ls.SaveIncremental([]map[string]any{
		{"drive_folder_id": wrongShow.ID, "drive_parent_id": normal.ID, "media_type": "tv", "tmdb_id": 325052, "title": "错放的动漫"},
	}, nil)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	code, data := authedGet(t, ts, token, "/api/library:classification?folder_id="+wrongShow.ID)
	if code != 200 {
		t.Fatalf("classification code = %d: %v", code, data)
	}
	if data["available"] != true {
		t.Fatalf("available 应为 true: %v", data)
	}
	// 当前实际位置在 普通电视剧 下；应属分类按 TMDB 最新流派（16/ja）为 动漫
	if data["current"] != "剧集/普通电视剧" {
		t.Fatalf("current = %v, want 剧集/普通电视剧", data["current"])
	}
	if data["expected"] != "剧集/动漫" {
		t.Fatalf("expected = %v, want 剧集/动漫", data["expected"])
	}
}

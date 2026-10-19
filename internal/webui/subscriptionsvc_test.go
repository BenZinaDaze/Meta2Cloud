// 订阅创建/更新保留 tmdb_id 与 poster_url 的回归测试。
//
// 背景：subscriptionBody.toPayload 产出 *int64/*string，而 store 的
// nullableInt64FromAny/anyStr 不认指针，导致 HTTP 创建的订阅
// tmdb_id/poster_url 静默丢成 NULL——资源检索直创的订阅卡片
// 因此既无封面也无 TMDB 信息。
package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"meta2cloud/internal/tmdb"
)

// ── 测试辅助 ────────────────────────────────────────────────

// newSubscriptionTmdbMock 模拟 TMDB 详情接口（供缓存预取）。
func newSubscriptionTmdbMock(t *testing.T, tmdbID float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": tmdbID, "name": "伊甸园的骄傲", "original_name": "Edens no Kokoroe",
			"poster_path": "/mockposter.jpg",
		})
	}))
}

// setupSubscriptionTestConfig 写入最小可用的 config.yaml。
func setupSubscriptionTestConfig(t *testing.T, rootDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(rootDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: k\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newSubscriptionTmdbClient 构建指向 mock 的 TMDB 客户端（走实体缓存）。
func newSubscriptionTmdbClient(t *testing.T, tmdbServer *httptest.Server) *tmdb.Client {
	t.Helper()
	client := tmdb.NewClient("k", "zh-CN", "", 5, RT().GetTmdbCache())
	client.SetTestBaseURL(tmdbServer.URL)
	return client
}

// newSubscriptionTestHTTPServer 启动被测服务。
func newSubscriptionTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := NewServer()
	t.Cleanup(server.Close)
	return httptest.NewServer(server.Handler())
}

// subscriptionTestToken 登录并返回 JWT。
func subscriptionTestToken(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	_, out := login(t, ts, "admin", "secret123")
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("登录未返回 token")
	}
	return token
}

// 资源检索直创场景：带 tmdb_id/poster_url 的载荷经 HTTP 创建后不丢字段，
// 且列表/详情能返回 TMDB 信息与封面。
func TestCreateSubscriptionKeepsTmdbIDAndPoster(t *testing.T) {
	tmdbServer := newSubscriptionTmdbMock(t, 111110)

	rootDir := t.TempDir()
	setupSubscriptionTestConfig(t, rootDir)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	RT().SetTmdbClientForTest(newSubscriptionTmdbClient(t, tmdbServer))

	ts := newSubscriptionTestHTTPServer(t)
	defer ts.Close()
	token := subscriptionTestToken(t, ts)

	body := map[string]any{
		"name":          "伊甸园的骄傲 [TestGroup]",
		"media_title":   "伊甸园的骄傲",
		"media_type":    "tv",
		"tmdb_id":       111110,
		"poster_url":    "https://tmdb.nmd.im/t/p/w500/fakeposterTEST.jpg",
		"site":          "mikan",
		"rss_url":       "https://mikanani.me/RSS/Bangumi?bangumiId=3333&subgroupid=583",
		"subgroup_name": "TestGroup",
		"season_number": 1,
		"start_episode": 1,
		"keyword_all":   []string{},
		"push_target":   "u115",
		"enabled":       true,
	}
	code, created := authedRequest(t, ts, token, "POST", "/api/subscriptions", body)
	if code != 200 {
		t.Fatalf("create = %d %v", code, created)
	}
	sub := created["subscription"].(map[string]any)
	if sub["tmdb_id"] == nil {
		t.Error("创建响应 tmdb_id = null, 应保留载荷中的 111110")
	}
	if id, ok := sub["tmdb_id"].(float64); !ok || int64(id) != 111110 {
		t.Errorf("创建响应 tmdb_id = %v, want 111110", sub["tmdb_id"])
	}
	if sub["poster_url"] == "" || sub["poster_url"] == nil {
		t.Error("创建响应 poster_url 为空, 应保留载荷海报")
	}
	if v, ok := sub["keyword_all"].([]any); !ok || len(v) != 0 {
		t.Errorf("创建响应 keyword_all = %v, want []", sub["keyword_all"])
	}

	// 创建响应应已带 TMDB 缓存信息（预取详情入 tmdb_media）
	if sub["tmdb"] == nil {
		t.Error("创建响应 tmdb = null, 创建时应补拉详情入缓存")
	}

	// 详情接口同样完整
	code, detail := authedRequest(t, ts, token, "GET", "/api/subscriptions/1", nil)
	if code != 200 {
		t.Fatalf("get = %d", code)
	}
	got := detail["subscription"].(map[string]any)
	if got["tmdb_id"] == nil || got["poster_url"] == "" {
		t.Errorf("详情 tmdb_id=%v poster=%v, 均不应为空", got["tmdb_id"], got["poster_url"])
	}
	if got["tmdb"] == nil {
		t.Error("详情 tmdb = null, 缓存补全后应可关联")
	}

	// 更新（暂停）不得把 tmdb_id/poster_url 抹掉
	body["enabled"] = false
	code, updated := authedRequest(t, ts, token, "PUT", "/api/subscriptions/1", body)
	if code != 200 {
		t.Fatalf("update = %d %v", code, updated)
	}
	usub := updated["subscription"].(map[string]any)
	if usub["tmdb_id"] == nil {
		t.Error("更新后 tmdb_id = null, 更新不应丢字段")
	}
	if usub["poster_url"] == "" || usub["poster_url"] == nil {
		t.Error("更新后 poster_url 为空")
	}

	// 列表（卡片数据源）同样完整
	code, listed := authedRequest(t, ts, token, "GET", "/api/subscriptions", nil)
	if code != 200 {
		t.Fatalf("list = %d", code)
	}
	subs := listed["subscriptions"].([]any)
	if len(subs) != 1 {
		t.Fatalf("list len = %d, want 1", len(subs))
	}
	card := subs[0].(map[string]any)
	if card["tmdb_id"] == nil || card["poster_url"] == "" || card["tmdb"] == nil {
		t.Errorf("列表卡片 tmdb_id=%v poster=%v tmdb=%v, 均不应为空",
			card["tmdb_id"], card["poster_url"], card["tmdb"])
	}
}

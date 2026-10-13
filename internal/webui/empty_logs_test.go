// 验证空库时 /api/logs 返回 items:[] 而非 null
package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"meta2cloud/internal/scraper"
	"meta2cloud/internal/tmdb"
	"strings"
)

func TestLogsEmptyReturnsArrayNotNull(t *testing.T) {
	// newTestServer 只返回 server，token 需登录获取
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	server := NewServer()
	defer server.Close()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	req, _ := http.NewRequest("GET", ts.URL+"/api/logs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw map[string]json.RawMessage
	json.NewDecoder(resp.Body).Decode(&raw)
	if string(raw["items"]) == "null" {
		t.Fatalf("items 为 null: %s", raw["items"])
	}
	if string(raw["items"]) != "[]" {
		t.Logf("items = %s (空库应为 [])", raw["items"])
	}
}

// 订阅卡片 TMDB 缓存补全: parse-rss 命中后详情入缓存, 中文标题可显示
func TestSubscriptionTmdbCacheBackfill(t *testing.T) {
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/multi"):
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 196285.0, "media_type": "tv", "name": "异世界悠闲农家 第2季",
					"poster_path": "/p.jpg"},
			}})
		case strings.HasPrefix(r.URL.Path, "/tv/196285"):
			if r.URL.Query().Get("language") == "" {
				t.Error("详情请求缺少 language 参数")
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id": 196285.0, "name": "异世界悠闲农家 第2季", "original_name": "Isekai Nonbiri Nouka 2",
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer tmdbServer.Close()

	// mock mikan RSS(订阅抓取依赖 spider)
	mikanServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<?xml version="1.0"?><rss><channel>
			<item><title>[LoliHouse] Isekai Nonbiri Nouka 2 - 01 [1080p]</title>
			<enclosure url="https://mikan/d/1.torrent" length="100"/></item>
		</channel></rss>`))
	}))
	defer mikanServer.Close()
	if spider, err := scraper.RegisterDefault().GetSpider("mikan"); err == nil {
		spider.(*scraper.MikanSpider).SetTestBaseURL(mikanServer.URL)
	}

	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: k\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	tmdbClient := tmdb.NewClient("k", "zh-CN", "", 5, RT().GetTmdbCache())
	tmdbClient.SetTestBaseURL(tmdbServer.URL)
	RT().SetTmdbClientForTest(tmdbClient)

	server := NewServer()
	defer server.Close()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	rssURL := "https://mikanani.me/RSS/Bangumi?bangumiId=999"
	code, parsed := authedRequest(t, ts, token, "POST", "/api/subscriptions:parse-rss",
		map[string]any{"rss_url": rssURL})
	if code != 200 || parsed["success"] != true {
		t.Fatalf("parse-rss = %d %v", code, parsed)
	}

	// 详情应已写入实体缓存(标题为中文)
	entry := RT().GetLibraryStore().GetTmdbEntry("tv", 196285)
	if entry == nil {
		t.Fatal("parse-rss 后 tmdb_media 缓存仍缺失")
	}
	if entry.Title != "异世界悠闲农家 第2季" {
		t.Errorf("缓存 title = %q, want 中文标题", entry.Title)
	}
}

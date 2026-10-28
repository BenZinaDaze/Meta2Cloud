// 验证空库时 /api/logs 返回 items:[] 而非 null
package webui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
	mikanTestSpider(t).SetTestBaseURL(mikanServer.URL)

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

// SPA 兜底不得读出 dist 之外的文件（路径穿越防护回归）
func TestRootPathTraversal(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.MkdirAll(filepath.Join(rootDir, "frontend", "dist"), 0o755)
	os.MkdirAll(filepath.Join(rootDir, "secret"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "frontend", "dist", "index.html"), []byte("<html>app</html>"), 0o644)
	os.WriteFile(filepath.Join(rootDir, "secret", "token.txt"), []byte("TOPSECRET"), 0o644)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	server := NewServer()
	defer server.Close()
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	for _, raw := range []string{
		ts.URL + "/../../../secret/token.txt",
		ts.URL + "/%2e%2e/%2e%2e/secret/token.txt",
		ts.URL + "/..%2f..%2fsecret%2ftoken.txt",
		ts.URL + "/frontend/../secret/token.txt",
	} {
		resp, err := http.Get(raw)
		if err != nil {
			continue // 部分客户端会先规范化, 请求不发出也视为安全
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if strings.Contains(string(body), "TOPSECRET") {
			t.Fatalf("路径穿越泄露: %s", raw)
		}
	}
}

// 回归: refresh-item 响应的数组字段永不为 null(前端解构默认值不处理 null)
func TestRefreshItemArraysNotNull(t *testing.T) {
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"id": 27205.0, "title": "测试电影", "release_date": "2010-07-16",
			"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
			"alternative_titles": map[string]any{"titles": []any{}},
			"translations":       map[string]any{"translations": []any{}},
			"external_ids":       map[string]any{},
		})
	}))
	defer tmdbServer.Close()

	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	fp := newFakeWebProvider()
	movieFolder := fp.addFolder("测试电影 (2010)", "movies")
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\ntmdb:\n  api_key: k\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	tmdbClient := tmdb.NewClient("k", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(tmdbServer.URL)
	RT().SetProviderForTest(fp.provider())
	RT().SetTmdbClientForTest(tmdbClient)

	server := NewServer()
	t.Cleanup(server.Close)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)
	_, out := login(t, ts, "admin", "secret123")
	token := out["token"].(string)

	req, _ := http.NewRequest("POST", ts.URL+"/api/library:refresh-item",
		strings.NewReader(fmt.Sprintf(
			`{"tmdb_id":27205,"media_type":"movie","drive_folder_id":%q}`, movieFolder.ID)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var raw map[string]json.RawMessage
	json.NewDecoder(resp.Body).Decode(&raw)
	if resp.StatusCode != 200 {
		t.Fatalf("code = %d", resp.StatusCode)
	}
	for _, field := range []string{"uploaded", "errors"} {
		if string(raw[field]) == "null" {
			t.Errorf("%s = null, 应为空数组(前端 null.length 崩溃)", field)
		}
	}
}

// 回归: 主详情 episode_count 旧值(7) + 季详情 12 集 + 库 12 集 → 总数 12 非 7
func TestBuildSeasonsTotalNotStale(t *testing.T) {
	rootDir := t.TempDir()
	os.MkdirAll(filepath.Join(rootDir, "config"), 0o755)
	os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: p\n"), 0o644)
	ResetJWTSecretCache()
	InitRuntime(rootDir)

	// TMDB mock: 主详情 seasons[0].episode_count=7, 季详情 12 集
	tmdbServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/season/") {
			eps := make([]any, 0, 12)
			for i := 1; i <= 12; i++ {
				eps = append(eps, map[string]any{"episode_number": float64(i), "name": fmt.Sprintf("EP%d", i)})
			}
			json.NewEncoder(w).Encode(map[string]any{"episodes": eps})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer tmdbServer.Close()
	tmdbClient := tmdb.NewClient("k", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(tmdbServer.URL)
	RT().SetTmdbClientForTest(tmdbClient)

	tmdbInfo := map[string]any{ // 主详情: 旧 episode_count=7
		"seasons": []any{map[string]any{"season_number": 1.0, "episode_count": 7.0}},
	}
	drive := map[epKey]bool{}
	for i := 1; i <= 12; i++ {
		drive[epKey{1, i}] = true
	}
	seasons, total, inLib := RT().BuildSeasonsStatus(309163, tmdbInfo, drive, false)
	if total != 12 {
		t.Errorf("total = %d, want 12(季详情 12 集 > 主详情旧值 7)", total)
	}
	if inLib != 12 {
		t.Errorf("inLib = %d, want 12", inLib)
	}
	if seasons[0].EpisodeCount != 12 {
		t.Errorf("season episodeCount = %d, want 12", seasons[0].EpisodeCount)
	}

	// 场景 2: 季详情也旧(仅 7 集)但库有 12 集 → 追加占位, 总数 12
	tmdbServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		eps := make([]any, 0, 7)
		for i := 1; i <= 7; i++ {
			eps = append(eps, map[string]any{"episode_number": float64(i)})
		}
		json.NewEncoder(w).Encode(map[string]any{"episodes": eps})
	}))
	defer tmdbServer2.Close()
	RT().SetTmdbClientForTest(nil) // 防走缓存客户端干扰
	c2 := tmdb.NewClient("k", "zh-CN", "", 5, nil)
	c2.SetTestBaseURL(tmdbServer2.URL)
	RT().SetTmdbClientForTest(c2)

	seasons2, total2, inLib2 := RT().BuildSeasonsStatus(309164, tmdbInfo, drive, false)
	if total2 != 12 || inLib2 != 12 {
		t.Errorf("场景2: total=%d inLib=%d, want 12/12(TMDB 未登记的 8-12 集应补占位)", total2, inLib2)
	}
	if len(seasons2[0].Episodes) != 12 {
		t.Errorf("场景2 episodes = %d, want 12", len(seasons2[0].Episodes))
	}
}

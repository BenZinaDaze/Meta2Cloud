package tmdb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"panoply/internal/mediaparser"
)

func TestBuildCachePath(t *testing.T) {
	// 与 Python 版 urlencode(sorted(items)) 规则一致，排除 api_key
	got := buildCachePath("/movie/123", map[string]string{"append_to_response": "credits,translations", "api_key": "secret"})
	want := "/movie/123?append_to_response=credits%2Ctranslations"
	if got != want {
		t.Errorf("cachePath = %q, want %q", got, want)
	}
	if got := buildCachePath("/tv/1", nil); got != "/tv/1" {
		t.Errorf("no params: %q", got)
	}
}

func TestNameMatch(t *testing.T) {
	if !nameMatch("Breaking Bad", "Breaking  Bad!") {
		t.Error("忽略大小写与特殊字符后应相等")
	}
	if nameMatch("Breaking Bad", "Better Call Saul") {
		t.Error("不同名称不应匹配")
	}
	// 中文匹配
	if !nameMatch("庆余年", "庆 余 年") {
		t.Error("中文应忽略空白匹配")
	}
}

func TestImageURL(t *testing.T) {
	if got := ImageURL("/abc.jpg", "w500", ""); got != "https://image.tmdb.org/t/p/w500/abc.jpg" {
		t.Errorf("imageURL = %q", got)
	}
	if got := ImageURL("", "w500", ""); got != "" {
		t.Errorf("empty path should give empty url, got %q", got)
	}
}

func TestGetWith429RetryAndCache(t *testing.T) {
	var calls, cacheHits int
	cache := &fakeCache{data: map[string]map[string]any{}, hits: &cacheHits}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") == "" {
			t.Error("缺少 api_key")
		}
		if r.URL.Query().Get("language") == "" {
			t.Error("缺少 language")
		}
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"id": 42.0, "title": "Test Movie"})
	}))
	defer server.Close()

	client := NewClient("key", "zh-CN", "", 5, cache)
	client.baseURLOverride = server.URL

	info := client.get("/movie/42", nil, true)
	if info == nil || jsonx_Int(info, "id") != 42 {
		t.Fatalf("429 后重试应成功: %v", info)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
	// 第二次命中缓存
	info2 := client.get("/movie/42", nil, true)
	if info2 == nil || cacheHits != 1 {
		t.Errorf("应命中缓存, hits=%d", cacheHits)
	}
}

func TestMatchMovieFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/movie") {
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 1.0, "title": "Other Film", "release_date": "2010-01-01"},
				map[string]any{"id": 42.0, "title": "Inception", "release_date": "2010-07-16"},
			}})
			return
		}
		// 详情（含 append 字段）
		json.NewEncoder(w).Encode(map[string]any{
			"id": 42.0, "title": "Inception",
			"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
			"alternative_titles": map[string]any{"titles": []any{}},
			"translations":       map[string]any{"translations": []any{}},
			"external_ids":       map[string]any{},
		})
	}))
	defer server.Close()

	client := NewClient("key", "zh-CN", "", 5, nil)
	client.baseURLOverride = server.URL
	info := client.matchMovie("Inception", "2010")
	if info == nil {
		t.Fatal("应匹配到 Inception")
	}
	if v, _ := info["tmdb_id"].(float64); v != 42 {
		t.Errorf("tmdb_id = %v", info["tmdb_id"])
	}
	if info["media_type"] != mediaparser.MediaMovie.String() {
		t.Errorf("media_type = %v", info["media_type"])
	}
}

func TestRecognizeByName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/tv") {
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 7.0, "name": "Breaking Bad", "first_air_date": "2008-01-20"},
			}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"id": 7.0, "name": "Breaking Bad",
			"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
			"alternative_titles": map[string]any{"results": []any{}},
			"translations":       map[string]any{"translations": []any{}},
			"external_ids":       map[string]any{},
		})
	}))
	defer server.Close()

	client := NewClient("key", "zh-CN", "", 5, nil)
	client.baseURLOverride = server.URL

	meta := mediaparser.MetaInfo("Breaking.Bad.S01E03.1080p.BluRay.HEVC", "", false, nil, nil)

	info := client.Recognize(meta)
	if info == nil {
		t.Fatal("Recognize 应找到 Breaking Bad")
	}
}

func jsonx_Int(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

type fakeCache struct {
	data map[string]map[string]any
	hits *int
}

func (f *fakeCache) Get(path, language string) (map[string]any, bool) {
	if d, ok := f.data[path+"|"+language]; ok {
		*f.hits++
		return d, true
	}
	return nil, false
}
func (f *fakeCache) IsFailed(path, language string) bool { return false }
func (f *fakeCache) Set(path string, data map[string]any, language string) {
	f.data[path+"|"+language] = data
}
func (f *fakeCache) SetFailed(path, language string) {}

// 回归: path 带 query 时(?append_to_response=...)不得被 api_key/language 覆盖
func TestGetMergesExistingQuery(t *testing.T) {
	var gotAppend, gotAPIKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAppend = r.URL.Query().Get("append_to_response")
		gotAPIKey = r.URL.Query().Get("api_key")
		if r.URL.Query().Get("language") == "" {
			t.Error("language 参数丢失")
		}
		json.NewEncoder(w).Encode(map[string]any{"id": 1.0, "credits": map[string]any{}})
	}))
	defer server.Close()

	client := NewClient("key", "zh-CN", "", 5, nil)
	client.SetTestBaseURL(server.URL)
	data := client.Get("/movie/27205?append_to_response=credits,external_ids")
	if data == nil || data["credits"] == nil {
		t.Fatalf("data = %v", data)
	}
	if gotAppend != "credits,external_ids" {
		t.Errorf("append_to_response = %q (被覆盖丢失)", gotAppend)
	}
	if gotAPIKey != "key" {
		t.Errorf("api_key = %q", gotAPIKey)
	}
}

// 回归: 民间译名(非官方名/不在别名表)搜索——TMDB 模糊搜索返回唯一
// 年份吻合的候选时应采纳(「令和妖神斑小姐」→「令和的斑小姐」场景)
func TestMatchTVUniqueCandidateFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/tv"):
			// 模糊搜索返回官方译名的正确条目
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 312507.0, "name": "令和的斑小姐",
					"original_name": "令和のダラさん", "first_air_date": "2026-07-02"},
			}})
		case strings.Contains(r.URL.Path, "/tv/312507"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": 312507.0, "name": "令和的斑小姐", "original_name": "令和のダラさん",
				"first_air_date":     "2026-07-02",
				"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
				"alternative_titles": map[string]any{"results": []any{}},
				"translations":       map[string]any{"translations": []any{}},
				"external_ids":       map[string]any{},
			})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := NewClient("key", "zh-CN", "", 5, nil)
	client.SetTestBaseURL(server.URL)

	// 精确匹配与别名表都不会命中「令和妖神斑小姐」,但唯一候选应被采纳
	info := client.SearchByName("令和妖神斑小姐", "2026", mediaparser.MediaTV)
	if info == nil {
		t.Fatal("唯一年份吻合候选未被采纳(民间译名识别失败)")
	}
	if id := int(jsonx_TestInt(info, "tmdb_id")); id != 312507 {
		if id2 := int(jsonx_TestInt(info, "id")); id2 != 312507 {
			t.Errorf("id = %v/%v, want 312507", info["tmdb_id"], info["id"])
		}
	}
}

// 对照: 多候选且无一精确匹配时不得随意采纳(防误识别)
func TestMatchTVMultipleCandidatesNoFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/tv") {
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 1.0, "name": "完全不相关剧A", "first_air_date": "2026-01-01"},
				map[string]any{"id": 2.0, "name": "完全不相关剧B", "first_air_date": "2026-02-01"},
			}})
			return
		}
		if strings.Contains(r.URL.Path, "/tv/") {
			json.NewEncoder(w).Encode(map[string]any{
				"id": 1.0, "name": "完全不相关剧A",
				"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
				"alternative_titles": map[string]any{"results": []any{}},
				"translations":       map[string]any{"translations": []any{}},
				"external_ids":       map[string]any{},
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()
	client := NewClient("key", "zh-CN", "", 5, nil)
	client.SetTestBaseURL(server.URL)

	if info := client.SearchByName("某部不存在的剧", "2026", mediaparser.MediaTV); info != nil {
		t.Errorf("多候选无精确匹配时不应回退采纳: %v", info["id"])
	}
}

func jsonx_TestInt(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0
}

package trakt

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"meta2cloud/internal/watched"
)

// newTestClient 构造指向 httptest 服务器的客户端（API 与 OAuth 主机同指测试服务器）。
func newTestClient(t *testing.T, handler http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := NewClient("test-id", WithHTTPClient(srv.Client()))
	c.testingAPIBase = srv.URL
	c.testingAuthBase = srv.URL
	return c
}

func testToken() *Token {
	return &Token{AccessToken: "tok", RefreshTime: time.Now().Unix(), ExpiresIn: 1 << 30}
}

func movieKey(tmdb int64) watched.MediaKey { return watched.MediaKey{Type: "movie", TMDBID: tmdb} }

func episodeKey(tmdb int64, season, episode int) watched.MediaKey {
	return watched.MediaKey{Type: "tv", TMDBID: tmdb, Season: season, Episode: episode}
}

func TestSyncWatchedMoviesPagination(t *testing.T) {
	var gotPages []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync/watched/movies" {
			t.Fatalf("意外路径：%s", r.URL.Path)
		}
		page := r.URL.Query().Get("page")
		limit := r.URL.Query().Get("limit")
		if limit != "250" {
			t.Fatalf("期望 limit=250，实际 %s", limit)
		}
		gotPages = append(gotPages, page)
		w.Header().Set("X-Pagination-Page-Count", "2")
		switch page {
		case "1", "":
			_ = json.NewEncoder(w).Encode([]WatchedMovie{{
				Plays: 1, LastWatchedAt: "2023-11-14T20:00:00.000Z",
				Movie: Media{Title: "A", IDs: IDs{TMDB: 100}},
			}})
		case "2":
			_ = json.NewEncoder(w).Encode([]WatchedMovie{{
				Plays: 2, LastWatchedAt: "2023-11-15T20:00:00.000Z",
				Movie: Media{Title: "B", IDs: IDs{TMDB: 200}},
			}})
		default:
			t.Fatalf("意外页码：%s", page)
		}
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	movies, err := c.SyncWatchedMovies()
	if err != nil {
		t.Fatalf("SyncWatchedMovies 出错：%v", err)
	}
	if len(movies) != 2 || movies[0].Movie.IDs.TMDB != 100 || movies[1].Movie.IDs.TMDB != 200 {
		t.Fatalf("期望两页共 2 条，实际 %+v", movies)
	}
	if len(gotPages) != 2 {
		t.Fatalf("期望请求 2 页，实际 %v", gotPages)
	}
}

func TestSyncWatchedShowsPagination(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode([]WatchedShow{{
			Show: Media{IDs: IDs{TMDB: 300}},
			Seasons: []WatchedSeason{{
				Number: 1,
				Episodes: []WatchedEpisode{
					{Number: 1, Plays: 1, LastWatchedAt: "2023-11-14T20:00:00.000Z"},
					{Number: 2, Plays: 1, LastWatchedAt: "2023-11-15T20:00:00.000Z"},
				},
			}},
		}})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	shows, err := c.SyncWatchedShows()
	if err != nil {
		t.Fatalf("SyncWatchedShows 出错：%v", err)
	}
	if len(shows) != 1 || len(shows[0].Seasons[0].Episodes) != 2 {
		t.Fatalf("期望 1 部剧 2 集，实际 %+v", shows)
	}
}

func TestAddHistoryChunkingAndBody(t *testing.T) {
	var bodies []map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync/history" {
			t.Fatalf("意外路径：%s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added":     map[string]int{"movies": 0, "episodes": 1},
			"not_found": map[string]any{},
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	// 250 集 → 3 个分块（100+100+50）
	ops := make([]watched.WatchOp, 0, 250)
	for i := 1; i <= 250; i++ {
		ops = append(ops, watched.WatchOp{Key: episodeKey(300, 1, i), At: 1700000000})
	}
	results := source.MarkWatched(context.Background(), ops)
	if len(results) != 250 {
		t.Fatalf("期望 250 条结果，实际 %d", len(results))
	}
	for _, res := range results {
		if !res.OK {
			t.Fatalf("期望全部成功，实际 %+v", res)
		}
	}
	if len(bodies) != 3 {
		t.Fatalf("期望 3 次 POST，实际 %d", len(bodies))
	}
	// 校验嵌套 body 形状：shows[{ids.tmdb, seasons[{number, episodes[{number, watched_at}]}]}]
	first := bodies[0]
	shows, ok := first["shows"].([]any)
	if !ok || len(shows) == 0 {
		t.Fatalf("期望 shows 嵌套结构：%v", first)
	}
	show := shows[0].(map[string]any)
	ids := show["ids"].(map[string]any)
	if ids["tmdb"].(float64) != 300 {
		t.Fatalf("期望 tmdb=300：%v", ids)
	}
	seasons := show["seasons"].([]any)
	season := seasons[0].(map[string]any)
	if season["number"].(float64) != 1 {
		t.Fatalf("期望 season 1：%v", season)
	}
	episodes := season["episodes"].([]any)
	if len(episodes) != 100 {
		t.Fatalf("首块期望 100 集，实际 %d", len(episodes))
	}
	ep := episodes[0].(map[string]any)
	if _, ok := ep["watched_at"]; !ok {
		t.Fatalf("集条目应携带 watched_at：%v", ep)
	}
}

func TestAddHistoryNotFoundFailsChunk(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added":     map[string]int{"movies": 0},
			"not_found": map[string]any{"movies": []any{map[string]any{"ids": map[string]any{"tmdb": 999}}}},
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	results := source.MarkWatched(context.Background(), []watched.WatchOp{{Key: movieKey(999)}})
	// not_found 非空 → 整块失败（状态记录不更新，下轮重试）
	for _, res := range results {
		if res.OK {
			t.Fatalf("not_found 非空时期望整块失败：%+v", res)
		}
	}
}

func TestDeviceCodeFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/device/code", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"device_code": "dc123", "user_code": "ABCD",
			"verification_url": "https://trakt.tv/activate", "expires_in": 600, "interval": 1,
		})
	})
	mux.HandleFunc("POST /oauth/device/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// Trakt 为 PKCE 模式：请求体只应有 code + client_id，不得出现 client_secret
		if _, has := body["client_secret"]; has {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client"})
			return
		}
		if body["code"] == "pending-code" {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "authorization_pending"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 7776000, "created_at": 1700000000,
		})
	})
	c := newTestClient(t, mux)

	code, err := c.CreateDeviceCode()
	if err != nil {
		t.Fatalf("CreateDeviceCode 出错：%v", err)
	}
	if code.DeviceCode != "dc123" || code.UserCode != "ABCD" {
		t.Fatalf("设备码不符：%+v", code)
	}

	pending, err := c.PollDeviceToken("pending-code")
	if err != nil {
		t.Fatalf("PollDeviceToken 出错：%v", err)
	}
	if pending.Confirmed() || pending.State != PollPending {
		t.Fatalf("期望 pending：%+v", pending)
	}

	confirmed, err := c.PollDeviceToken(code.DeviceCode)
	if err != nil {
		t.Fatalf("PollDeviceToken 出错：%v", err)
	}
	if !confirmed.Confirmed() || confirmed.Token.AccessToken != "at" {
		t.Fatalf("期望确认：%+v", confirmed)
	}
	if !c.HasToken() {
		t.Fatal("确认后客户端应持有 token")
	}
}

// TestDeviceCodeFlowInvalidClient 被拒绝时给出可操作的错误提示。
func TestDeviceCodeFlowInvalidClient(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/device/token", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client"})
	})
	c := newTestClient(t, mux)
	_, err := c.PollDeviceToken("dc")
	if err == nil || !strings.Contains(err.Error(), "invalid_client") {
		t.Fatalf("期望 invalid_client 错误提示，实际：%v", err)
	}
}

func TestTokenPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trakt-token.json")
	token := Token{AccessToken: "a", RefreshToken: "r", ExpiresIn: 100, RefreshTime: 1}
	if err := SaveToken(token, path); err != nil {
		t.Fatalf("SaveToken 出错：%v", err)
	}
	loaded, ok, err := LoadToken(path)
	if err != nil || !ok || loaded.AccessToken != "a" {
		t.Fatalf("LoadToken 结果异常：ok=%v loaded=%+v err=%v", ok, loaded, err)
	}
	// 不存在文件 → (zero, false, nil)
	missing, ok, err := LoadToken(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || ok || missing.AccessToken != "" {
		t.Fatalf("缺失文件应返回 (zero,false,nil)：ok=%v err=%v", ok, err)
	}
}

func TestUnwatchedUsesRemoveEndpoint(t *testing.T) {
	var paths []string
	var bodies []map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		bodies = append(bodies, body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deleted":   map[string]int{"movies": 1},
			"not_found": map[string]any{},
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	results := source.MarkUnwatched(context.Background(), []watched.MediaKey{movieKey(100)})
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("期望成功：%+v", results)
	}
	if len(paths) != 1 || paths[0] != "/sync/history/remove" {
		t.Fatalf("期望调用 /sync/history/remove，实际 %v", paths)
	}
	movies := bodies[0]["movies"].([]any)
	if movies[0].(map[string]any)["ids"].(map[string]any)["tmdb"].(float64) != 100 {
		t.Fatalf("期望 movies[0].ids.tmdb=100：%v", bodies[0])
	}
	if _, hasWatchedAt := movies[0].(map[string]any)["watched_at"]; hasWatchedAt {
		t.Fatal("取消观看不应携带 watched_at")
	}
}

func TestRateLimitCooldown(t *testing.T) {
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	if _, err := c.SyncWatchedMovies(); err == nil {
		t.Fatal("期望 429 返回错误")
	}
	before := calls
	if _, err := c.SyncWatchedMovies(); err == nil || calls != before {
		t.Fatalf("冷却期内不应再发请求：calls=%d err=%v", calls, err)
	}
}

func TestListAllShowLevelEntriesViaProgress(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sync/watched/movies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode([]WatchedMovie{})
	})
	mux.HandleFunc("GET /sync/watched/shows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Pagination-Page-Count", "1")
		// 剧级条目：plays 有值但无 seasons 明细（真实 API 对 Plex 来源记录的形态）
		_ = json.NewEncoder(w).Encode([]WatchedShow{{
			Plays:         11,
			LastWatchedAt: "2026-05-18T14:53:00.000Z",
			Show:          Media{Title: "Laid-Back Camp", IDs: IDs{TMDB: 76075, Trakt: 121326}},
		}})
	})
	mux.HandleFunc("GET /shows/121326/progress/watched", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"aired": 37, "completed": 3,
			"reset_at": nil,
			"seasons": []map[string]any{
				{"number": 1, "episodes": []map[string]any{
					{"number": 1, "completed": true, "last_watched_at": "2024-01-01T00:00:00.000Z"},
					{"number": 2, "completed": true, "last_watched_at": "2024-01-02T00:00:00.000Z"},
					{"number": 3, "completed": false},
				}},
				{"number": 3, "episodes": []map[string]any{
					{"number": 1, "completed": true, "last_watched_at": "2026-05-18T14:53:00.000Z"},
				}},
			},
		})
	})
	c := newTestClient(t, mux)
	c.token = testToken()

	state, err := NewTraktSource(c).ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll 出错：%v", err)
	}
	for _, want := range []watched.MediaKey{
		episodeKey(76075, 1, 1), episodeKey(76075, 1, 2), episodeKey(76075, 3, 1),
	} {
		if got, ok := state[want]; !ok || !got.Watched {
			t.Fatalf("期望已看 %s：%+v", want, state)
		}
	}
	if got, ok := state[episodeKey(76075, 1, 3)]; ok && got.Watched {
		t.Fatalf("completed=false 不应算已看：%+v", state)
	}
}

// TestListAllRespectsResetAt reset 后早于 reset_at 的观看应被忽略。
func TestListAllRespectsResetAt(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sync/watched/movies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode([]WatchedMovie{})
	})
	mux.HandleFunc("GET /sync/watched/shows", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode([]WatchedShow{{
			Show: Media{IDs: IDs{TMDB: 500, Trakt: 500}},
		}})
	})
	mux.HandleFunc("GET /shows/500/progress/watched", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"reset_at": "2026-06-01T00:00:00.000Z",
			"seasons": []map[string]any{
				{"number": 1, "episodes": []map[string]any{
					// reset 前的旧观看 → 忽略
					{"number": 1, "completed": true, "last_watched_at": "2024-01-01T00:00:00.000Z"},
					// reset 后的新观看 → 保留
					{"number": 2, "completed": true, "last_watched_at": "2026-06-02T00:00:00.000Z"},
				}},
			},
		})
	})
	c := newTestClient(t, mux)
	c.token = testToken()

	state, err := NewTraktSource(c).ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll 出错：%v", err)
	}
	if _, ok := state[episodeKey(500, 1, 1)]; ok {
		t.Fatal("reset 前的观看不应算已看")
	}
	if got, ok := state[episodeKey(500, 1, 2)]; !ok || !got.Watched {
		t.Fatal("reset 后的观看应算已看")
	}
}

// TestAddHistoryZeroAddedFails added=0 视为写入未接受（部分失败），整块重试。
func TestAddHistoryZeroAddedFails(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added":     map[string]int{"movies": 0, "episodes": 0},
			"not_found": map[string]any{},
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	results := source.MarkWatched(context.Background(), []watched.WatchOp{{Key: movieKey(100)}})
	for _, res := range results {
		if res.OK {
			t.Fatalf("added=0 不应视为成功：%+v", res)
		}
	}
	// 取消已看不要求 deleted>0
	removeHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deleted":   map[string]int{"movies": 0},
			"not_found": map[string]any{},
		})
	})
	c2 := newTestClient(t, removeHandler)
	c2.token = testToken()
	results = NewTraktSource(c2).MarkUnwatched(context.Background(), []watched.MediaKey{movieKey(100)})
	if len(results) != 1 || !results[0].OK {
		t.Fatalf("deleted=0 的取消应幂等成功：%+v", results)
	}
}

// TestOAuthEndpointsUseAuthHost 官方要求 OAuth 端点（设备码、token 交换/刷新）走
// auth.trakt.tv，API 端点走 api.trakt.tv；两者不得混用。
func TestOAuthEndpointsUseAuthHost(t *testing.T) {
	apiHits, authHits := 0, 0
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits++
		w.Header().Set("X-Pagination-Page-Count", "1")
		_ = json.NewEncoder(w).Encode([]WatchedMovie{})
	}))
	t.Cleanup(apiSrv.Close)
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHits++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "at", "refresh_token": "rt", "expires_in": 7776000,
		})
	}))
	t.Cleanup(authSrv.Close)

	c := NewClient("test-id", WithHTTPClient(apiSrv.Client()))
	c.testingAPIBase = apiSrv.URL
	c.testingAuthBase = authSrv.URL

	if _, err := c.PollDeviceToken("dc"); err != nil {
		t.Fatalf("PollDeviceToken 出错：%v", err)
	}
	if err := c.RefreshToken(); err != nil {
		t.Fatalf("RefreshToken 出错：%v", err)
	}
	c.token = testToken()
	if _, err := c.SyncWatchedMovies(); err != nil {
		t.Fatalf("SyncWatchedMovies 出错：%v", err)
	}
	if authHits != 2 || apiHits != 1 {
		t.Fatalf("OAuth 应打 auth 主机（2 次）、API 应打 api 主机（1 次），实际 auth=%d api=%d", authHits, apiHits)
	}
	if DefaultVerificationURL != "https://auth.trakt.tv/activate" {
		t.Fatalf("激活页默认值应为 auth.trakt.tv/activate，实际 %s", DefaultVerificationURL)
	}
}

// TestNotFoundShowLevelAttribution 整剧未匹配：只连坐该剧的键，同批他剧按成功。
func TestNotFoundShowLevelAttribution(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added":     map[string]int{"episodes": 2},
			"not_found": map[string]any{"shows": []any{map[string]any{"ids": map[string]any{"tmdb": 329623}}}},
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	ops := []watched.WatchOp{}
	for ep := 1; ep <= 12; ep++ {
		ops = append(ops, watched.WatchOp{Key: watched.MediaKey{Type: "tv", TMDBID: 329623, Season: 1, Episode: ep}})
	}
	ops = append(ops,
		watched.WatchOp{Key: watched.MediaKey{Type: "tv", TMDBID: 999, Season: 1, Episode: 1}},
		watched.WatchOp{Key: watched.MediaKey{Type: "tv", TMDBID: 999, Season: 1, Episode: 2}})
	results := source.MarkWatched(context.Background(), ops)
	failed, ok := 0, 0
	for _, res := range results {
		if res.Key.TMDBID == 329623 {
			if res.OK {
				t.Fatalf("未收录剧的键应失败：%+v", res)
			}
			if want := "Trakt 未收录该剧（tmdb 329623 无映射）"; res.Err != want {
				t.Fatalf("错误说明不符：%q", res.Err)
			}
			failed++
		} else if res.Key.TMDBID == 999 {
			if !res.OK {
				t.Fatalf("同批已匹配的他剧键应成功：%+v", res)
			}
			ok++
		}
	}
	if failed != 12 || ok != 2 {
		t.Fatalf("归因数量不符：failed=%d ok=%d", failed, ok)
	}
}

// TestNotFoundEpisodeLevelAttribution 单集级未匹配：仅该集失败，同剧其余成功。
func TestNotFoundEpisodeLevelAttribution(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added": map[string]int{"episodes": 2},
			"not_found": map[string]any{"episodes": []any{
				map[string]any{"ids": map[string]any{"tmdb": 500}, "season": 1, "number": 5},
			}},
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	ops := []watched.WatchOp{
		{Key: watched.MediaKey{Type: "tv", TMDBID: 500, Season: 1, Episode: 4}},
		{Key: watched.MediaKey{Type: "tv", TMDBID: 500, Season: 1, Episode: 5}},
		{Key: watched.MediaKey{Type: "tv", TMDBID: 500, Season: 1, Episode: 6}},
	}
	results := source.MarkWatched(context.Background(), ops)
	for _, res := range results {
		if res.Key.Episode == 5 {
			if res.OK || res.Err != "Trakt 未收录该集 S01E05（tmdb 500）" {
				t.Fatalf("未收录集应失败且说明带集号：%+v", res)
			}
		} else if !res.OK {
			t.Fatalf("同剧其余集应成功：%+v", res)
		}
	}
}

// TestNotFoundUnknownShapeFallsBackWholeChunk 未知 not_found 形态：回退整批失败。
func TestNotFoundUnknownShapeFallsBackWholeChunk(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added":     map[string]int{"episodes": 0},
			"not_found": "weird-shape", // 非对象非数组的异常形态
		})
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c)
	results := source.MarkWatched(context.Background(), []watched.WatchOp{
		{Key: watched.MediaKey{Type: "tv", TMDBID: 500, Season: 1, Episode: 1}},
		{Key: watched.MediaKey{Type: "tv", TMDBID: 501, Season: 1, Episode: 1}},
	})
	for _, res := range results {
		if res.OK {
			t.Fatalf("未知形态应整批保守失败：%+v", res)
		}
	}
}

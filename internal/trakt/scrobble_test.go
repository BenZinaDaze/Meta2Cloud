// scrobble_test.go —— scrobble 三段式的载荷、响应分派与多 ID 断言。
// 响应语义对照官方 spec（developer.trakt.tv/openapi.json，2026-10 核对）：
// 2xx 按 action 区分计次；409 去重（watched_at/expires_at）；422 忽略；
// 401 归类授权失效；429 进入冷却。
package trakt

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"meta2cloud/internal/watched"
)

// capturedRequest 记录一次请求的方法/路径/载荷。
type capturedRequest struct {
	Method string
	Path   string
	Body   map[string]any
}

type bodyRecorder struct {
	mu   sync.Mutex
	got  []capturedRequest
	next func(w http.ResponseWriter, r *http.Request)
}

func (b *bodyRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	b.mu.Lock()
	b.got = append(b.got, capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body})
	next := b.next
	b.mu.Unlock()
	next(w, r)
}

func (b *bodyRecorder) requests() []capturedRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]capturedRequest(nil), b.got...)
}

func TestScrobbleStopScrobbled(t *testing.T) {
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 12345, "action": "scrobble", "progress": 92.5,
		})
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	res, err := c.Scrobble(ScrobbleStop, nil, traktStopEpisode(), 92.5)
	if err != nil {
		t.Fatalf("stop 应成功：%v", err)
	}
	if res.Action != "scrobble" || res.ID != 12345 || res.Progress != 92.5 {
		t.Fatalf("结果不符：%+v", res)
	}

	reqs := rec.requests()
	if len(reqs) != 1 || reqs[0].Path != "/scrobble/stop" {
		t.Fatalf("应恰好一次 /scrobble/stop：%+v", reqs)
	}
	if got := reqs[0].Body["progress"].(float64); got != 92.5 {
		t.Fatalf("progress 不符：%v", got)
	}
	show := reqs[0].Body["show"].(map[string]any)["ids"].(map[string]any)
	if show["tmdb"].(float64) != 335790 || show["tvdb"].(float64) != 474294 || show["imdb"] != "tt43650323" {
		t.Fatalf("show ids 应携带全部已知 ID：%v", show)
	}
	ep := reqs[0].Body["episode"].(map[string]any)
	if ep["season"].(float64) != 1 || ep["number"].(float64) != 2 {
		t.Fatalf("episode 季集号不符：%v", ep)
	}
}

func traktStopEpisode() *HistoryEpisode {
	return &HistoryEpisode{
		ShowTMDBID: 335790, ShowTVDBID: 474294, ShowIMDBID: "tt43650323",
		Season: 1, Episode: 2,
	}
}

func TestScrobbleStopDuplicate409(t *testing.T) {
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"watched_at": "2026-10-24T10:00:00.000Z",
			"expires_at": "2026-10-24T11:00:00.000Z",
		})
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	_, err := c.Scrobble(ScrobbleStop, nil, traktStopEpisode(), 90)
	var dup *DuplicateScrobbleError
	if !errors.As(err, &dup) {
		t.Fatalf("409 应返回 DuplicateScrobbleError，实际 %v", err)
	}
	if dup.WatchedAt.IsZero() || dup.ExpiresAt.IsZero() {
		t.Fatalf("409 应解析出 watched_at/expires_at：%+v", dup)
	}
	if got := dup.WatchedAt.UTC(); got.Hour() != 10 || got.Day() != 24 {
		t.Fatalf("watched_at 解析不符：%v", got)
	}
	if got := dup.ExpiresAt.UTC(); got.Hour() != 11 {
		t.Fatalf("expires_at 解析不符：%v", got)
	}
}

func TestScrobbleStopIgnored422(t *testing.T) {
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"progress must be between 1 and 100"}`))
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	_, err := c.Scrobble(ScrobbleStop, nil, traktStopEpisode(), 0.2)
	var ignored *ScrobbleIgnoredError
	if !errors.As(err, &ignored) {
		t.Fatalf("422 应返回 ScrobbleIgnoredError，实际 %v", err)
	}
	if ignored.Detail == "" {
		t.Fatal("422 错误应携带服务端说明")
	}
}

func TestScrobbleStartUnauthorized(t *testing.T) {
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	_, err := c.Scrobble(ScrobbleStart, &HistoryMovie{TMDBID: 100}, nil, 5)
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 应归类为授权失效，实际 %v", err)
	}
}

func TestScrobblePauseOK(t *testing.T) {
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"action": "pause", "progress": 40})
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	res, err := c.Scrobble(ScrobblePause, &HistoryMovie{TMDBID: 100, IMDBID: "tt100"}, nil, 40)
	if err != nil {
		t.Fatalf("pause 应成功：%v", err)
	}
	if res.Action != "pause" {
		t.Fatalf("action 应为 pause：%+v", res)
	}
	movie := rec.requests()[0].Body["movie"].(map[string]any)["ids"].(map[string]any)
	if movie["tmdb"].(float64) != 100 || movie["imdb"] != "tt100" {
		t.Fatalf("movie ids 不符：%v", movie)
	}
	if _, hasTVDB := movie["tvdb"]; hasTVDB {
		t.Fatalf("零值 ID 不应出现在载荷：%v", movie)
	}
}

func TestScrobbleProgressClamped(t *testing.T) {
	var got float64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		got = body["progress"].(float64)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"action":"pause","progress":100}`))
	}))
	defer srv.Close()
	c := NewClient("test-id", WithHTTPClient(srv.Client()), WithAPIBase(srv.URL))
	c.token = testToken()

	if _, err := c.Scrobble(ScrobbleStop, &HistoryMovie{TMDBID: 1}, nil, 250); err != nil {
		t.Fatalf("stop 失败：%v", err)
	}
	if got != 100 {
		t.Fatalf("超上限进度应钳制到 100，实际 %v", got)
	}
	if _, err := c.Scrobble(ScrobbleStop, &HistoryMovie{TMDBID: 1}, nil, 0); err != nil {
		var ignored *ScrobbleIgnoredError
		if !errors.As(err, &ignored) {
			t.Fatalf("0 进度应走 422 忽略分支：%v", err)
		}
	}
}

func TestAddHistoryPayloadCarriesMultiIDs(t *testing.T) {
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added": map[string]int{"movies": 1, "episodes": 2},
		})
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	source := NewTraktSource(c, watched.NewMemIDMapper(), nil)
	movie := watched.MediaKey{Type: "movie", TMDBID: 603}
	// 先归并多 ID，再推送（与真实路径一致：身份解析时写入映射）
	if _, ok := source.Mapper.ResolveKey("movie", watched.MediaIDs{TMDB: 603, IMDB: "tt0120915"}, 0, 0); !ok {
		t.Fatal("电影 ID 归并应成功")
	}
	showKey, ok := source.Mapper.ResolveKey("tv", watched.MediaIDs{TMDB: 335790, TVDB: 474294, IMDB: "tt43650323"}, 0, 0)
	if !ok || showKey.TMDBID != 335790 {
		t.Fatalf("剧集归并应得 tmdb 锚：%+v ok=%v", showKey, ok)
	}
	episode := showKey
	episode.Season, episode.Episode = 1, 2

	results := source.MarkWatched(context.Background(), []watched.WatchOp{
		{Key: movie, At: 1700000000},
		{Key: episode, At: 1700000000},
	})
	for _, res := range results {
		if !res.OK {
			t.Fatalf("写入应成功：%+v", res)
		}
	}
	reqs := rec.requests()
	if len(reqs) != 1 || reqs[0].Path != "/sync/history" {
		t.Fatalf("应合并为一次 /sync/history：%+v", reqs)
	}
	movies := reqs[0].Body["movies"].([]any)
	movieIDs := movies[0].(map[string]any)["ids"].(map[string]any)
	if movieIDs["tmdb"].(float64) != 603 || movieIDs["imdb"] != "tt0120915" {
		t.Fatalf("电影载荷应带 imdb：%v", movieIDs)
	}
	shows := reqs[0].Body["shows"].([]any)
	showIDs := shows[0].(map[string]any)["ids"].(map[string]any)
	if showIDs["tmdb"].(float64) != 335790 || showIDs["tvdb"].(float64) != 474294 || showIDs["imdb"] != "tt43650323" {
		t.Fatalf("剧载荷应带全部 ID：%v", showIDs)
	}
}

func TestNotFoundAttributionWithTVDBEcho(t *testing.T) {
	// tvdb 锚定的键：载荷只带 tvdb，not_found 回带 tvdb → 仍可归因
	rec := &bodyRecorder{next: func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"added": map[string]int{"episodes": 2},
			"not_found": map[string]any{"episodes": []any{
				map[string]any{"ids": map[string]any{"tvdb": 474294}, "season": 1, "number": 5},
			}},
		})
	}}
	c := newTestClient(t, rec)
	c.token = testToken()

	source := NewTraktSource(c, watched.NewMemIDMapper(), nil)
	showKey, ok := source.Mapper.ResolveKey("tv", watched.MediaIDs{TVDB: 474294}, 0, 0)
	if !ok || showKey.TVDBID != 474294 {
		t.Fatalf("tvdb 锚应成键：%+v ok=%v", showKey, ok)
	}
	ops := []watched.WatchOp{}
	for ep := 4; ep <= 6; ep++ {
		k := showKey
		k.Season, k.Episode = 1, ep
		ops = append(ops, watched.WatchOp{Key: k})
	}
	results := source.MarkWatched(context.Background(), ops)
	failed := 0
	for _, res := range results {
		if res.Key.Episode == 5 {
			if res.OK {
				t.Fatal("未收录集应失败")
			}
			failed++
		} else if !res.OK {
			t.Fatalf("同剧其余集应成功：%+v", res)
		}
	}
	if failed != 1 {
		t.Fatalf("应恰好归因 1 集：%d", failed)
	}
}

func TestListAllTVDBAnchoredShow(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sync/watched/movies":
			w.Header().Set("X-Pagination-Page-Count", "1")
			_, _ = w.Write([]byte(`[]`))
		case "/sync/watched/shows":
			w.Header().Set("X-Pagination-Page-Count", "1")
			_ = json.NewEncoder(w).Encode([]WatchedShow{{
				Plays: 1, LastWatchedAt: "2026-10-20T10:00:00.000Z",
				Show: Media{Title: "只挂 tvdb 的剧", IDs: IDs{Trakt: 328808, TVDB: 474294, Slug: "x"}},
			}})
		case "/shows/328808/progress/watched":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"aired": 3, "completed": 2,
				"seasons": []any{map[string]any{
					"number": 1,
					"episodes": []any{
						map[string]any{"number": 1, "completed": true, "last_watched_at": "2026-10-20T10:00:00.000Z"},
						map[string]any{"number": 2, "completed": true, "last_watched_at": "2026-10-21T10:00:00.000Z"},
						map[string]any{"number": 3, "completed": false},
					},
				}},
			})
		default:
			t.Fatalf("意外路径：%s", r.URL.Path)
		}
	})
	c := newTestClient(t, handler)
	c.token = testToken()

	source := NewTraktSource(c, watched.NewMemIDMapper(), nil)
	state, err := source.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(state) != 2 {
		t.Fatalf("应有 2 个已看集键：%+v", state)
	}
	for key, item := range state {
		if key.Type != "tv" || key.TVDBID != 474294 || key.TMDBID != 0 {
			t.Fatalf("应为 tvdb 锚定键：%+v", key)
		}
		if !item.Watched {
			t.Fatalf("应为已看：%+v", item)
		}
	}
}

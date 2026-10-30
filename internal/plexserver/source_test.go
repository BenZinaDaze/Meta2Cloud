package plexserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"panoply/internal/watched"
)

// fakeTitleIndex 标题反查替身。
type fakeTitleIndex map[string]int64 // "type|title|year" → tmdb

func (f fakeTitleIndex) Lookup(mediaType, title string, year int64) (watched.MediaKey, bool) {
	if id, ok := f[fmt.Sprintf("%s|%s|%d", mediaType, title, year)]; ok {
		return watched.MediaKey{Type: mediaType, TMDBID: id}, true
	}
	return watched.MediaKey{}, false
}

func TestParseTMDBGuid(t *testing.T) {
	cases := []struct {
		guid string
		want int64
	}{
		{"plex://movie/5d77684b2df3e5002f4d6f72", 0},
		{"tmdb://278", 278},
		{"imdb://tt0111161", 0},
		{"com.plexapp.agents.tmdb://278?lang=zh", 278},
		{"tmdb://", 0},
		{"tmdb://abc", 0},
	}
	m := &Metadata{Guid: "tmdb://278"}
	for _, tc := range cases {
		m.GuidList = []GuidEntry{{ID: tc.guid}}
		m.Guid = ""
		if got := m.TMDBGuid(); got != tc.want {
			t.Fatalf("GuidList=%s 期望 %d 实际 %d", tc.guid, tc.want, got)
		}
	}
	// 主 guid 兜底
	m.GuidList = nil
	m.Guid = "tmdb://123"
	if got := m.TMDBGuid(); got != 123 {
		t.Fatalf("主 guid 兜底期望 123 实际 %d", got)
	}
}

func TestParseTitleYear(t *testing.T) {
	cases := []struct {
		segment string
		title   string
		year    int64
		ok      bool
	}{
		{"阿凡达 (2009)", "阿凡达", 2009, true},
		{"Arrival (2016) ", "Arrival", 2016, true},
		{"无年份目录", "", 0, false},
		{"某剧 (abcd)", "", 0, false},
	}
	for _, tc := range cases {
		title, year, ok := parseTitleYear(tc.segment)
		if ok != tc.ok || (ok && (title != tc.title || year != tc.year)) {
			t.Fatalf("parseTitleYear(%q) = (%q,%d,%v)，期望 (%q,%d,%v)",
				tc.segment, title, year, ok, tc.title, tc.year, tc.ok)
		}
	}
}

func TestResolveByPathFallback(t *testing.T) {
	idx := fakeTitleIndex{
		"tv|沙丘|2021":     418249,
		"movie|阿凡达|2009": 19995,
	}
	s := NewSource(nil, idx, watched.NewMemIDMapper())

	// 剧集：挂载前缀 + 分类目录 + Season 段
	item := &Metadata{
		Type: "episode", Index: 2, ParentIndex: 1,
		Media: []MediaInfo{{Part: []PartInfo{{File: "/mnt/115/媒体库/剧集/沙丘 (2021)/Season 01/沙丘 - S01E02.mkv"}}}},
	}
	key, ok := s.resolveByPath("tv", item.FilePath(), item)
	if !ok || key.TMDBID != 418249 || key.Season != 1 || key.Episode != 2 {
		t.Fatalf("剧集路径解析不符：key=%+v ok=%v", key, ok)
	}

	// 电影：反斜杠路径（Windows 形态）
	movie := &Metadata{Type: "movie", Media: []MediaInfo{{Part: []PartInfo{{File: `D:\media\电影\阿凡达 (2009)\阿凡达.mkv`}}}}}
	key, ok = s.resolveByPath("movie", movie.FilePath(), movie)
	if !ok || key.TMDBID != 19995 {
		t.Fatalf("电影路径解析不符：key=%+v ok=%v", key, ok)
	}

	// 反查未命中 → false
	idx2 := fakeTitleIndex{}
	s2 := NewSource(nil, idx2, watched.NewMemIDMapper())
	key, ok = s2.resolveByPath("tv", item.FilePath(), item)
	if ok {
		t.Fatalf("未命中应返回 false：key=%+v", key)
	}
}

// TestResolveSessionRefEpisodeGuidIsEpisodeId 集详情的 tmdb:// 是「集」级 id，
// 不得当剧集身份；身份应取自上级剧集条目的权威 Guid。
// 真实案例：剧集本身 tmdb://335790，其 S01E02 集级却是 tmdb://7905973。
func TestResolveSessionRefEpisodeGuidIsEpisodeId(t *testing.T) {
	sections := []Section{{Key: "3", Type: "show", Title: "动漫"}}
	itemsByRef := map[string]Metadata{
		"856": {
			RatingKey: "856", Type: "episode", Key: "/library/metadata/856",
			GrandparentTitle: "黑暗召唤师在地下恋爱中", GrandparentRatingKey: "853",
			ParentIndex: 1, Index: 2,
			GuidList: []GuidEntry{{ID: "tmdb://7905973"}, {ID: "tvdb://12018489"}},
		},
		"853": {
			RatingKey: "853", Type: "show", Title: "黑暗召唤师在地下恋爱中",
			GuidList: []GuidEntry{{ID: "tmdb://335790"}},
		},
	}
	_, source := newPlexTestServer(t, sections, nil, itemsByRef)

	key, ok := source.ResolveMetadataRef("856")
	if !ok {
		t.Fatal("应通过上级剧集解析成功")
	}
	want := watched.MediaKey{Type: "tv", TMDBID: 335790, Season: 1, Episode: 2}
	if key != want {
		t.Fatalf("身份应取上级剧集 guid %+v，实际 %+v", want, key)
	}
}

// newPlexTestServer 构造带常用接口的 Plex httptest 服务器。
func newPlexTestServer(t *testing.T, sections []Section, itemsBySection map[string][]Metadata, itemsByRef map[string]Metadata) (*Client, *Source) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /library/sections", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{"Directory": sections},
		})
	})
	mux.HandleFunc("GET /library/sections/", func(w http.ResponseWriter, r *http.Request) {
		// /library/sections/{key}/all
		key := strings.Split(strings.TrimPrefix(r.URL.Path, "/library/sections/"), "/")[0]
		items := itemsBySection[key]
		start := 0
		if v := r.URL.Query().Get("X-Plex-Container-Start"); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &start)
		}
		size := len(items)
		if v := r.URL.Query().Get("X-Plex-Container-Size"); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &size)
		}
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		if start > len(items) {
			start = len(items)
		}
		w.Header().Set("X-Plex-Container-Total-Size", fmt.Sprintf("%d", len(items)))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{
				"Metadata": items[start:end], "totalSize": len(items),
			},
		})
	})
	mux.HandleFunc("GET /library/metadata/", func(w http.ResponseWriter, r *http.Request) {
		ref := strings.Split(strings.TrimPrefix(r.URL.Path, "/library/metadata/"), "/")[0]
		item, ok := itemsByRef[ref]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{"Metadata": []Metadata{item}},
		})
	})
	mux.HandleFunc("GET /:/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := NewClient(srv.URL, "test-token", "test-identifier", WithHTTPClient(srv.Client()))
	client.PlexTVBase = srv.URL
	return client, NewSource(client, nil, watched.NewMemIDMapper())
}

func TestListAllGuidAndAggregation(t *testing.T) {
	sections := []Section{{Key: "1", Type: "movie", Title: "电影"}}
	items := []Metadata{
		{RatingKey: "1", Type: "movie", Title: "A", ViewCount: 1, LastViewedAt: 1700000000,
			GuidList: []GuidEntry{{ID: "tmdb://100"}}, Media: []MediaInfo{{Part: []PartInfo{{File: "/m/A (2000)/A.mkv"}}}}},
		// 同一部电影的多版本：Watched=OR、LastAt=max、Plays=max
		{RatingKey: "2", Type: "movie", Title: "A", ViewCount: 3, LastViewedAt: 1700050000,
			GuidList: []GuidEntry{{ID: "tmdb://100"}}},
		{RatingKey: "3", Type: "movie", Title: "B", GuidList: []GuidEntry{{ID: "tmdb://200"}}},
	}
	client, source := newPlexTestServer(t, sections, map[string][]Metadata{"1": items}, nil)

	state, err := source.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll 出错：%v", err)
	}
	key100 := watched.MediaKey{Type: "movie", TMDBID: 100}
	key200 := watched.MediaKey{Type: "movie", TMDBID: 200}
	if got := state[key100]; !got.Watched || got.LastAt != 1700050000 || got.Plays != 3 {
		t.Fatalf("聚合语义不符：%+v", got)
	}
	if got := state[key200]; got.Watched {
		t.Fatalf("未看条目不应为 watched：%+v", got)
	}
	refs := source.RefsOf(key100)
	if len(refs) != 2 {
		t.Fatalf("多版本应保留全部 ratingKey：%v", refs)
	}
	_ = client
}

func TestListAllPathFallbackAndUnresolved(t *testing.T) {
	sections := []Section{{Key: "1", Type: "movie", Title: "电影"}}
	items := []Metadata{
		// 无 Guid，靠路径 + TitleIndex
		{RatingKey: "10", Type: "movie", Title: "阿凡达", Year: 2009,
			Media: []MediaInfo{{Part: []PartInfo{{File: "/mnt/115/媒体库/电影/阿凡达 (2009)/阿凡达 (2009).mkv"}}}}},
		// 无 Guid 无路径 → unresolved
		{RatingKey: "11", Type: "movie", Title: "神秘影片"},
	}
	client, _ := newPlexTestServer(t, sections, map[string][]Metadata{"1": items}, nil)
	source := NewSource(client, fakeTitleIndex{"movie|阿凡达|2009": 19995}, watched.NewMemIDMapper())

	state, err := source.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll 出错：%v", err)
	}
	key := watched.MediaKey{Type: "movie", TMDBID: 19995}
	if got := state[key]; !got.Present {
		t.Fatalf("路径兜底解析失败：%+v", state)
	}
	unresolved := source.DrainUnresolved()
	if len(unresolved) != 1 || unresolved[0].Ref != "11" {
		t.Fatalf("期望 1 条 unresolved：%+v", unresolved)
	}
}

func TestListAllEpisodeGrandparentFallback(t *testing.T) {
	sections := []Section{{Key: "2", Type: "show", Title: "剧集"}}
	episodes := []Metadata{
		{RatingKey: "21", Key: "/library/metadata/20/children/22/21", Type: "episode",
			Title: "第二集", Index: 2, ParentIndex: 1, GrandparentTitle: "沙丘",
			Media: []MediaInfo{{Part: []PartInfo{{File: "/mnt/115/剧集/沙丘 (2021)/Season 01/S01E02.mkv"}}}}},
	}
	shows := map[string]Metadata{
		"20": {RatingKey: "20", Type: "show", Title: "沙丘",
			GuidList: []GuidEntry{{ID: "tmdb://418249"}}},
	}
	client, _ := newPlexTestServer(t, sections, map[string][]Metadata{"2": episodes}, shows)
	source := NewSource(client, fakeTitleIndex{}, watched.NewMemIDMapper())

	state, err := source.ListAll(context.Background())
	if err != nil {
		t.Fatalf("ListAll 出错：%v", err)
	}
	key := watched.MediaKey{Type: "tv", TMDBID: 418249, Season: 1, Episode: 2}
	if got, ok := state[key]; !ok || !got.Present {
		t.Fatalf("上级剧集 Guid 兜底解析失败：%+v", state)
	}
}

func TestLibraryItemsPaging(t *testing.T) {
	items := make([]Metadata, 0, 450)
	for i := 0; i < 450; i++ {
		items = append(items, Metadata{RatingKey: fmt.Sprintf("%d", i), Type: "movie", Title: fmt.Sprintf("M%d", i)})
	}
	client, _ := newPlexTestServer(t,
		[]Section{{Key: "1", Type: "movie", Title: "电影"}},
		map[string][]Metadata{"1": items}, nil)

	got, err := client.LibraryItems("1", 1)
	if err != nil {
		t.Fatalf("LibraryItems 出错：%v", err)
	}
	if len(got) != 450 {
		t.Fatalf("期望翻页取回 450 条，实际 %d", len(got))
	}
}

func TestScrobbleURLShape(t *testing.T) {
	var gotPath, gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /:/scrobble", func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte("{}"))
	})
	mux.HandleFunc("GET /:/unscrobble", func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte("{}"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, "tok", "cid", WithHTTPClient(srv.Client()))

	if err := client.Scrobble("12345"); err != nil {
		t.Fatalf("Scrobble 出错：%v", err)
	}
	if gotPath != "/:/scrobble" ||
		gotQuery != "identifier=com.plexapp.plugins.library&key=12345" {
		t.Fatalf("scrobble URL 形状不符：%s?%s", gotPath, gotQuery)
	}
	if err := client.Unscrobble("12345"); err != nil {
		t.Fatalf("Unscrobble 出错：%v", err)
	}
	if gotPath != "/:/unscrobble" {
		t.Fatalf("unscrobble 路径不符：%s", gotPath)
	}
}

func TestPINFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v2/pins", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 777, "code": "ABCD", "authToken": ""})
	})
	mux.HandleFunc("GET /api/v2/pins/777", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 777, "code": "ABCD", "authToken": "tok-x"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := NewClient("http://127.0.0.1:32400", "", "my-identifier", WithHTTPClient(srv.Client()))
	client.PlexTVBase = srv.URL

	pin, authURL, err := client.CreatePIN()
	if err != nil {
		t.Fatalf("CreatePIN 出错：%v", err)
	}
	if pin.ID != 777 || pin.Code != "ABCD" {
		t.Fatalf("PIN 不符：%+v", pin)
	}
	if !strings.Contains(authURL, "clientID=my-identifier") || !strings.Contains(authURL, "code=ABCD") {
		t.Fatalf("授权链接不符：%s", authURL)
	}

	polled, err := client.PollPIN(pin.ID)
	if err != nil {
		t.Fatalf("PollPIN 出错：%v", err)
	}
	if polled.AuthToken != "tok-x" {
		t.Fatalf("期望拿到 authToken：%+v", polled)
	}
}

func TestIdentityAndSessions(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /identity", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{"machineIdentifier": "abc", "version": "1.41.0"},
		})
	})
	mux.HandleFunc("GET /status/sessions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{"Metadata": []Metadata{{
				SessionKey: "s1", RatingKey: "9", Type: "movie", Title: "播放中",
				ViewOffset: 480000, Duration: 600000,
			}}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, "tok", "cid", WithHTTPClient(srv.Client()))

	identity, err := client.Identity()
	if err != nil {
		t.Fatalf("Identity 出错：%v", err)
	}
	if identity.MediaContainer.Version != "1.41.0" {
		t.Fatalf("identity 不符：%+v", identity)
	}
	sessions, err := client.Sessions()
	if err != nil {
		t.Fatalf("Sessions 出错：%v", err)
	}
	if len(sessions) != 1 || sessions[0].RatingKey != "9" {
		t.Fatalf("会话不符：%+v", sessions)
	}
	progress := float64(sessions[0].ViewOffset) / float64(sessions[0].Duration)
	if progress < 0.8 {
		t.Fatalf("期望进度 ≥0.8，实际 %.2f", progress)
	}
}

// TestStreamNotifications SSE 事件流解析（含字符串数字兼容）。
func TestStreamNotifications(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /:/eventsource/notifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.URL.Query().Get("filters"); got != "playing" {
			t.Errorf("filters 参数应为 playing，实际 %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		fmt.Fprint(w, "event: notification\n")
		fmt.Fprint(w, `data: {"type":"playing","PlaySessionStateNotification":[{"state":"playing","sessionKey":"s1","ratingKey":"9","viewOffset":"480000","duration":600000}]}`+"\n\n")
		f.Flush()
		<-r.Context().Done() // 保持连接直至客户端断开
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, "tok", "cid", WithHTTPClient(srv.Client()))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := make(chan PlaybackNotification, 4)
	opened := make(chan struct{}, 1)
	go func() {
		_ = client.StreamNotifications(ctx, func() { opened <- struct{}{} }, func(n PlaybackNotification) { got <- n })
	}()

	select {
	case <-opened:
	case <-ctx.Done():
		t.Fatal("超时未回调 onOpen")
	}
	select {
	case n := <-got:
		if n.State != "playing" || n.SessionKey != "s1" || n.RatingKey != "9" ||
			n.ViewOffsetMS != 480000 || n.DurationMS != 600000 {
			t.Fatalf("事件解析不符：%+v", n)
		}
	case <-ctx.Done():
		t.Fatal("超时未收到事件")
	}
}

// TestStreamNotificationsObjectForm 真实 PMS 形态走完整订阅管道：
// 单对象、data 无 type 字段，事件类型来自 event: 行。
func TestStreamNotificationsObjectForm(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /:/eventsource/notifications", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		f := w.(http.Flusher)
		fmt.Fprint(w, "event: playing\n")
		fmt.Fprint(w, `data: {"PlaySessionStateNotification":{"sessionKey":"23","ratingKey":"856","viewOffset":103166,"state":"playing"}}`+"\n\n")
		f.Flush()
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, "tok", "cid", WithHTTPClient(srv.Client()))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	got := make(chan PlaybackNotification, 4)
	go func() {
		_ = client.StreamNotifications(ctx, nil, func(n PlaybackNotification) { got <- n })
	}()
	select {
	case n := <-got:
		if n.State != "playing" || n.SessionKey != "23" || n.RatingKey != "856" || n.ViewOffsetMS != 103166 {
			t.Fatalf("事件解析不符：%+v", n)
		}
	case <-ctx.Done():
		t.Fatal("超时未收到事件")
	}
}

// TestStreamNotificationsNotSupported 旧版 PMS 无 SSE 端点时应返回可识别的哨兵错误。
func TestStreamNotificationsNotSupported(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /:/eventsource/notifications", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, "tok", "cid", WithHTTPClient(srv.Client()))

	err := client.StreamNotifications(context.Background(), nil, func(PlaybackNotification) {})
	if !errors.Is(err, ErrNotificationsUnsupported) {
		t.Fatalf("应返回 ErrNotificationsUnsupported，实际 %v", err)
	}
}

func TestParsePlaybackNotificationIgnoresNonPlaying(t *testing.T) {
	if got := parsePlaybackNotification(`{"type":"activity","ActivityNotification":[{"event":"updated"}]}`, "activity"); len(got) != 0 {
		t.Fatalf("非 playing 类型应忽略：%+v", got)
	}
	if got := parsePlaybackNotification(`not-json`, "playing"); len(got) != 0 {
		t.Fatalf("坏载荷应返回空：%+v", got)
	}
}

// TestParsePlaybackNotificationRealWorldShapes 真实 PMS 两种报文形态：
// 数组+内嵌 type，以及单对象、data 无 type 字段（类型只在 SSE event: 行）。
func TestParsePlaybackNotificationRealWorldShapes(t *testing.T) {
	arrayForm := `{"type":"playing","PlaySessionStateNotification":[{"state":"playing","sessionKey":"s1","ratingKey":"9","viewOffset":"480000","duration":600000}]}`
	got := parsePlaybackNotification(arrayForm, "notification")
	if len(got) != 1 || got[0].RatingKey != "9" || got[0].ViewOffsetMS != 480000 {
		t.Fatalf("数组形态应解析：%+v", got)
	}

	objectForm := `{"PlaySessionStateNotification":{"sessionKey":"23","clientIdentifier":"c1","guid":"","ratingKey":"856","url":"","key":"/library/metadata/856","viewOffset":103166,"state":"playing"}}`
	got = parsePlaybackNotification(objectForm, "playing")
	if len(got) != 1 || got[0].RatingKey != "856" || got[0].SessionKey != "23" || got[0].ViewOffsetMS != 103166 {
		t.Fatalf("单对象形态应解析：%+v", got)
	}

	// event: 行不是 playing 且 data 无 type：不视为播放事件
	got = parsePlaybackNotification(objectForm, "ping")
	if len(got) != 0 {
		t.Fatalf("ping 事件应忽略：%+v", got)
	}
}

// TestMetadataAndLibraryItemsRequestIncludeGuids 详情与枚举请求必须带
// includeGuids=1：部分条目状态下 PMS 不带此参数不回传 Guid（2026-10-26
// 实测事故根因），身份解析会因外部 ID 全空而静默失败。
func TestMetadataAndLibraryItemsRequestIncludeGuids(t *testing.T) {
	var metaQuery, listQuery url.Values
	mux := http.NewServeMux()
	mux.HandleFunc("GET /library/metadata/862", func(w http.ResponseWriter, r *http.Request) {
		metaQuery = r.URL.Query()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{"Metadata": []Metadata{{RatingKey: "862", Type: "show"}}},
		})
	})
	mux.HandleFunc("GET /library/sections/3/all", func(w http.ResponseWriter, r *http.Request) {
		listQuery = r.URL.Query()
		w.Header().Set("X-Plex-Container-Total-Size", "0")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"MediaContainer": map[string]any{"Metadata": []Metadata{}, "totalSize": 0},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := NewClient(srv.URL, "tok", "cid", WithHTTPClient(srv.Client()))

	if _, err := client.Metadata("862"); err != nil {
		t.Fatalf("Metadata 出错：%v", err)
	}
	if metaQuery.Get("includeGuids") != "1" {
		t.Fatalf("Metadata 应带 includeGuids=1，实际 %v", metaQuery)
	}
	if _, err := client.LibraryItems("3", 2); err != nil {
		t.Fatalf("LibraryItems 出错：%v", err)
	}
	if listQuery.Get("includeGuids") != "1" {
		t.Fatalf("LibraryItems 应带 includeGuids=1，实际 %v", listQuery)
	}
}

// TestResolveEpisodeTVDBAnchored 剧集条目只有 tvdb guid（无 tmdb）：
// 上级剧集应以 tvdb 锚成键（1b：缺 tmdb 的剧不再不可见）。
func TestResolveEpisodeTVDBAnchored(t *testing.T) {
	sections := []Section{{Key: "3", Type: "show", Title: "动漫"}}
	itemsByRef := map[string]Metadata{
		"856": {
			RatingKey: "856", Type: "episode", Key: "/library/metadata/856",
			GrandparentTitle: "某番剧", GrandparentRatingKey: "853",
			ParentIndex: 1, Index: 2,
		},
		"853": {
			RatingKey: "853", Type: "show", Title: "某番剧",
			GuidList: []GuidEntry{{ID: "tvdb://474294"}, {ID: "imdb://tt43"}},
		},
	}
	_, source := newPlexTestServer(t, sections, nil, itemsByRef)

	key, ok := source.ResolveMetadataRef("856")
	if !ok {
		t.Fatal("应通过上级剧集解析成功")
	}
	want := watched.MediaKey{Type: "tv", TVDBID: 474294, Season: 1, Episode: 2}
	if key != want {
		t.Fatalf("应为 tvdb 锚键 %+v，实际 %+v", want, key)
	}
}

// TestResolveMovieMergesIMDB 电影条目带 tmdb+imdb：tmdb 锚成键，
// imdb 并入映射（推送载荷可携带）。
func TestResolveMovieMergesIMDB(t *testing.T) {
	sections := []Section{{Key: "1", Type: "movie", Title: "电影"}}
	items := []Metadata{{
		RatingKey: "1", Type: "movie", Title: "A",
		GuidList: []GuidEntry{{ID: "tmdb://603"}, {ID: "imdb://tt0120915"}},
	}}
	client, source := newPlexTestServer(t, sections, map[string][]Metadata{"1": items}, nil)

	state, err := source.ListAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	key := watched.MediaKey{Type: "movie", TMDBID: 603}
	if _, ok := state[key]; !ok {
		t.Fatalf("应为 tmdb 键：%+v", state)
	}
	ids := source.Mapper.IDsOf(key)
	if ids.IMDB != "tt0120915" {
		t.Fatalf("imdb 应并入映射：%+v", ids)
	}
	_ = client
}

// TestParseTVDBAndIMDBGuids tvdb/imdb guid 解析（含旧代理拼接形态）。
func TestParseTVDBAndIMDBGuids(t *testing.T) {
	m := &Metadata{GuidList: []GuidEntry{
		{ID: "imdb://tt43650323"},
		{ID: "tvdb://474294"},
		{ID: "com.plexapp.agents.tvdb://123?lang=zh"},
	}}
	if got := m.TVDBGuid(); got != 474294 {
		t.Fatalf("TVDBGuid 应为 474294，实际 %d", got)
	}
	if got := m.IMDBGuid(); got != "tt43650323" {
		t.Fatalf("IMDBGuid 应为 tt43650323，实际 %q", got)
	}
	m2 := &Metadata{Guid: "com.plexapp.agents.tvdb://999"}
	if got := m2.TVDBGuid(); got != 999 {
		t.Fatalf("主 guid 兜底应为 999，实际 %d", got)
	}
}

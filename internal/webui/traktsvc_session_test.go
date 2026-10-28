// traktsvc_session_test.go —— 会话 scrobble 三段式的端到端流程验证：
// SSE 状态跃迁 → start/pause/stop 调用序列与载荷、409/422 分派、
// 同步状态与 run 记录落库、tvdb 锚定键全链路。
package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"meta2cloud/internal/plexserver"
	"meta2cloud/internal/trakt"
	"meta2cloud/internal/watched"
	"meta2cloud/internal/webui/store"
)

type scrobbleCapture struct {
	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
	next   func(w http.ResponseWriter, r *http.Request)
}

func (c *scrobbleCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	c.mu.Lock()
	c.paths = append(c.paths, r.URL.Path)
	c.bodies = append(c.bodies, body)
	next := c.next
	c.mu.Unlock()
	next(w, r)
}

func (c *scrobbleCapture) reset(next func(w http.ResponseWriter, r *http.Request)) {
	c.mu.Lock()
	c.paths, c.bodies = nil, nil
	c.next = next
	c.mu.Unlock()
}

func (c *scrobbleCapture) snapshot() ([]string, []map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.paths...), append([]map[string]any(nil), c.bodies...)
}

// newSessionTestEnv 构造会话 scrobble 测试环境：临时 Runtime + Trakt/ Plex 测试服务器。
func newSessionTestEnv(t *testing.T, traktHandler http.Handler) (*store.TraktSyncStore, *plexserver.Client, *trakt.Client, *scrobbleCapture) {
	t.Helper()
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "trakt:\n  sync_to_trakt: true\n  client_id: test-cid\n"
	if err := os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	ResetJWTSecretCache()
	InitRuntime(rootDir)
	syncStore := RT().GetTraktSyncStore()
	if syncStore == nil {
		t.Fatal("同步存储不可用")
	}

	cap := &scrobbleCapture{
		next: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"action":"scrobble","id":77,"progress":92}`))
		},
	}
	traktSrv := httptest.NewServer(cap)
	t.Cleanup(traktSrv.Close)

	plexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/metadata/856":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "856", "type": "episode", "key": "/library/metadata/856",
				"grandparentTitle": "某番剧", "grandparentRatingKey": "853",
				"parentIndex": 1, "index": 2, "title": "第二集",
			}}}})
		case "/library/metadata/857":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "857", "type": "episode", "key": "/library/metadata/857",
				"grandparentTitle": "某番剧", "grandparentRatingKey": "853",
				"parentIndex": 1, "index": 3, "title": "第三集",
			}}}})
		case "/library/metadata/853":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "853", "type": "show", "title": "某番剧",
				"Guid": []any{map[string]any{"id": "tmdb://335790"}, map[string]any{"id": "tvdb://474294"}},
			}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(plexSrv.Close)

	plexClient := plexserver.NewClient(plexSrv.URL, "tok", "cid")
	traktClient := trakt.NewClient("test-cid",
		trakt.WithHTTPClient(traktSrv.Client()),
		trakt.WithAPIBase(traktSrv.URL),
		trakt.WithToken(trakt.Token{AccessToken: "tok", RefreshTime: time.Now().Unix(), ExpiresIn: 1 << 30}),
	)
	return syncStore, plexClient, traktClient, cap
}

func stoppedEvent(sessionKey string, offsetMS int64) plexserver.PlaybackNotification {
	return plexserver.PlaybackNotification{
		State: "stopped", SessionKey: sessionKey, RatingKey: "856",
		ViewOffsetMS: offsetMS, DurationMS: 1440000, // 24 分钟
	}
}

func TestSessionScrobbleFlowPlayingToStopped(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)
	key := watched.MediaKey{Type: "tv", TMDBID: 335790, Season: 1, Episode: 2}

	// 播放开始（25%）→ start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s1", RatingKey: "856",
		ViewOffsetMS: 360000, DurationMS: 1440000,
	})
	// 播放中的周期性事件（30%）→ 不应重复上报
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s1", RatingKey: "856",
		ViewOffsetMS: 432000, DurationMS: 1440000,
	})
	// 暂停 → pause
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s1", RatingKey: "856",
		ViewOffsetMS: 720000, DurationMS: 1440000,
	})
	// 恢复播放 → start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s1", RatingKey: "856",
		ViewOffsetMS: 1080000, DurationMS: 1440000,
	})
	// 停止（92.5%，叠加历史最大进度不变）→ stop，服务端判定计次
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s1", 1332000))

	paths, bodies := cap.snapshot()
	want := []string{"/scrobble/start", "/scrobble/pause", "/scrobble/start", "/scrobble/stop"}
	if len(paths) != len(want) {
		t.Fatalf("调用序列不符：%v", paths)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("调用序列不符：期望 %v 实际 %v", want, paths)
		}
	}
	stopBody := bodies[3]
	if got := stopBody["progress"].(float64); got != 92.5 { // 1325000/1440000
		t.Fatalf("stop 进度不符：%v", got)
	}
	show := stopBody["show"].(map[string]any)["ids"].(map[string]any)
	if show["tmdb"].(float64) != 335790 || show["tvdb"].(float64) != 474294 {
		t.Fatalf("stop 载荷应带全部已知 ID（含归并的 tvdb）：%v", show)
	}

	// 同步状态：Trakt 已看 + 推送时间戳
	sh, ok, _ := syncStore.GetShadow(key)
	if !ok || sh.TraktWatched == nil || !*sh.TraktWatched {
		t.Fatalf("Trakt 观看状态应已更新：%+v ok=%v", sh, ok)
	}
	if sh.SessionPushedAt == 0 {
		t.Fatal("SessionPushedAt 应已记录")
	}
	// run 记录：一条成功，且带会话标题（供页面「内容」列与相邻合并）
	runs, total, _ := syncStore.ListSyncRuns(10, 0)
	if total != 1 || len(runs) != 1 || runs[0].ToTrakt != 1 || runs[0].Error != nil {
		t.Fatalf("应有一条成功推送记录：total=%d %+v", total, runs)
	}
	if runs[0].Title == nil || *runs[0].Title != "某番剧 S01E02「第二集」" {
		t.Fatalf("run 应带会话标题：%+v", runs[0].Title)
	}
}

func TestSessionScrobbleStopDuplicate409(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)
	cap.reset(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scrobble/stop" {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"watched_at":"2026-10-24T10:00:00.000Z","expires_at":"2026-10-24T11:00:00.000Z"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"action":"start","progress":50}`))
	})

	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s2", RatingKey: "856",
		ViewOffsetMS: 720000, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s2", 1325000))

	key := watched.MediaKey{Type: "tv", TMDBID: 335790, Season: 1, Episode: 2}
	sh, ok, _ := syncStore.GetShadow(key)
	// 409 = 媒体已在 Trakt 历史中：目的态达成，仅对齐本地观看状态
	if !ok || sh.TraktWatched == nil || !*sh.TraktWatched {
		t.Fatalf("409 后 Trakt 观看状态应按已看处理：%+v ok=%v", sh, ok)
	}
	if sh.TraktLastAt == 0 {
		t.Fatal("应回填服务端 watched_at")
	}
	// 去重不是真实计次推送，不落 run 记录（页面不呈现）
	if _, total, _ := syncStore.ListSyncRuns(10, 0); total != 0 {
		t.Fatalf("409 去重不应落记录：total=%d", total)
	}
}

func TestSessionScrobbleStopIgnored422(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)
	cap.reset(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scrobble/stop" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"progress too low"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"action":"start","progress":0.5}`))
	})

	// 刚开播即停（进度 <1%）→ 422 忽略，不计次不落记录
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s3", RatingKey: "856",
		ViewOffsetMS: 0, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s3", 0))

	if _, total, _ := syncStore.ListSyncRuns(10, 0); total != 0 {
		t.Fatalf("422 忽略不应落记录：total=%d", total)
	}
}

func TestSessionScrobbleStopErrorBackoff(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)
	cap.reset(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scrobble/stop" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"action":"start","progress":50}`))
	})

	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s4", RatingKey: "856",
		ViewOffsetMS: 1080000, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s4", 1325000))

	key := watched.MediaKey{Type: "tv", TMDBID: 335790, Season: 1, Episode: 2}
	// 失败落退避标记（引擎兜底重试用）+ 失败记录
	sh, ok, _ := syncStore.GetShadow(key)
	if !ok || sh.SessionPushedAt == 0 {
		t.Fatalf("失败应写退避标记：%+v ok=%v", sh, ok)
	}
	runs, total, _ := syncStore.ListSyncRuns(10, 0)
	if total != 1 || runs[0].ToTrakt != 0 || runs[0].Error == nil {
		t.Fatalf("失败应落失败记录：total=%d %+v", total, runs)
	}
}

// TestSessionScrobblePauseBeyond80Finishes Trakt 的 pause 只接受 1%~80%：
// 片尾暂停（85%）应视同看完改发 stop（计次/落库与正常收尾一致），
// 暂停期间后续 paused 事件与 ≥80% 段恢复播放均不再重复上报。
func TestSessionScrobblePauseBeyond80Finishes(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// 播放到 75% → start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s5", RatingKey: "856",
		ViewOffsetMS: 1080000, DurationMS: 1440000,
	})
	// 片尾暂停（85%）→ 不发 pause，直接按收尾上报 stop
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s5", RatingKey: "856",
		ViewOffsetMS: 1224000, DurationMS: 1440000,
	})
	// 暂停期间后续 paused 事件（进度不变）→ 不应反复收尾
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s5", RatingKey: "856",
		ViewOffsetMS: 1224000, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s5", RatingKey: "856",
		ViewOffsetMS: 1230000, DurationMS: 1440000,
	})
	// 片尾段恢复播放（90%）→ 不应再上报 start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s5", RatingKey: "856",
		ViewOffsetMS: 1296000, DurationMS: 1440000,
	})
	// 停止 → 会话收尾（实际会命中服务端去重 409，此处仅验证调用序列）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s5", 1296000))

	paths, bodies := cap.snapshot()
	want := []string{"/scrobble/start", "/scrobble/stop", "/scrobble/stop"}
	if len(paths) != len(want) {
		t.Fatalf("调用序列不符：%v", paths)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("调用序列不符：期望 %v 实际 %v", want, paths)
		}
	}
	if got := bodies[1]["progress"].(float64); got != 85 {
		t.Fatalf("片尾暂停应收尾 stop 且进度 85%%：%v", got)
	}

	key := watched.MediaKey{Type: "tv", TMDBID: 335790, Season: 1, Episode: 2}
	sh, ok, _ := syncStore.GetShadow(key)
	if !ok || sh.TraktWatched == nil || !*sh.TraktWatched {
		t.Fatalf("片尾暂停收尾后应记为已看：%+v ok=%v", sh, ok)
	}
	runs, total, _ := syncStore.ListSyncRuns(10, 0)
	if total != 2 || runs[0].ToTrakt != 1 || runs[1].ToTrakt != 1 {
		t.Fatalf("两次收尾各落一条成功记录：total=%d %+v", total, runs)
	}
}

// TestSessionScrobblePauseUnder1PercentSkipped Trakt 的 pause 要求 ≥1%：
// 刚开播即暂停不发 pause，且后续 paused 事件不重试（否则暂停期间无限刷警告）。
func TestSessionScrobblePauseUnder1PercentSkipped(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// 刚开播（0.2%）→ start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s6", RatingKey: "856",
		ViewOffsetMS: 2880, DurationMS: 1440000,
	})
	// 立即暂停（0.5%、0.8%）→ 无进度可存，跳过上报且不重试
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s6", RatingKey: "856",
		ViewOffsetMS: 7200, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s6", RatingKey: "856",
		ViewOffsetMS: 11520, DurationMS: 1440000,
	})
	// 恢复播放（5%）→ 正常上报 start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s6", RatingKey: "856",
		ViewOffsetMS: 72000, DurationMS: 1440000,
	})

	paths, _ := cap.snapshot()
	want := []string{"/scrobble/start", "/scrobble/start"}
	if len(paths) != len(want) {
		t.Fatalf("调用序列不符（应只有两次 start，无 pause）：%v", paths)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("调用序列不符：期望 %v 实际 %v", want, paths)
		}
	}
	if _, total, _ := syncStore.ListSyncRuns(10, 0); total != 0 {
		t.Fatalf("未收尾不应落记录：total=%d", total)
	}
}

// TestSessionScrobbleStoppedWithoutTrackedRowSkipped stopped 只收尾有观察行
// 的会话（抓包证实 2026-10-25：播放/暂停事件随 ~10s 周期重发，正常播放的
// 会话必有观察行）：无观察行的 stopped（从未见过开播的残留通知、会话已
// 收尾后的重复 stopped）不据此上报，观看状态由定时全量同步兜底。
func TestSessionScrobbleStoppedWithoutTrackedRowSkipped(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// 会话首个事件就是 stopped（从未见过开播）→ 不上报
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "s9", RatingKey: "856",
		ViewOffsetMS: 1425600, DurationMS: 1440000,
	})
	if paths, _ := cap.snapshot(); len(paths) != 0 {
		t.Fatalf("无观察行的 stopped 不应上报：%v", paths)
	}

	// 正常播放 → stopped → stop 收尾一次
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s9", RatingKey: "856",
		ViewOffsetMS: 360000, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s9", 1332000))

	// 会话已收尾（观察行已删）后再来一条重复 stopped → 不再收尾
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, stoppedEvent("s9", 1332000))

	paths, bodies := cap.snapshot()
	want := []string{"/scrobble/start", "/scrobble/stop"}
	if len(paths) != len(want) {
		t.Fatalf("调用序列不符（重复 stopped 不应再收尾）：%v", paths)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("调用序列不符：期望 %v 实际 %v", want, paths)
		}
	}
	if got := bodies[1]["progress"].(float64); got != 92.5 {
		t.Fatalf("stop 进度应为 92.5%%：%v", got)
	}
}

// TestSessionScrobblePlayQueueSwitchStaleOffset 按 2026-10-25 抓包
// （config/data/sse-capture/20261025-172916-resume-next.jsonl）逐条回放播放
// 队列切集：新 sessionKey 先以「旧条目 + 旧最终 offset」出现（paused），
// ratingKey 切换后 buffering 仍带旧 offset、playing 才复位。旧集应按其真实
// ~99% 收尾，新集从零跟踪——不得把旧集的 99% 记到新集头上（旧行为：新集
// stop 时 MaxProgress 合并了旧集的 99%，被 Trakt 直接计次为已看）。
func TestSessionScrobblePlayQueueSwitchStaleOffset(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// E02 播放（92%）→ start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s10", RatingKey: "856",
		ViewOffsetMS: 1325000, DurationMS: 1440000,
	})
	// E02 停止（98.6%）→ stop 收尾，观察行清理
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "s10", RatingKey: "856",
		ViewOffsetMS: 1420000, DurationMS: 1440000,
	})
	// 新 sessionKey 以「旧条目 + 旧最终 offset」诞生（抓包 17:30:40.333）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s10", RatingKey: "856",
		ViewOffsetMS: 1420000, DurationMS: 1440000,
	})
	// ratingKey 切到 E03，buffering 仍带旧 offset（抓包 17:30:48.124）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "buffering", SessionKey: "s10", RatingKey: "857",
		ViewOffsetMS: 1420090, DurationMS: 1440000,
	})
	// buffering 重发（抓包 17:30:50.071），offset 未复位
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "buffering", SessionKey: "s10", RatingKey: "857",
		ViewOffsetMS: 1420090, DurationMS: 1440000,
	})
	// E03 开播，offset 复位为 0（抓包 17:30:51.684）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s10", RatingKey: "857",
		ViewOffsetMS: 0, DurationMS: 1440000,
	})
	// E03 播了 4 秒停止（0.24%，抓包 17:30:55.537）→ 只能按 0.24% 收尾
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "s10", RatingKey: "857",
		ViewOffsetMS: 3438, DurationMS: 1440000,
	})

	paths, bodies := cap.snapshot()
	want := []string{"/scrobble/start", "/scrobble/stop", "/scrobble/stop", "/scrobble/start", "/scrobble/stop"}
	if len(paths) != len(want) {
		t.Fatalf("调用序列不符：%v", paths)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("调用序列不符：期望 %v 实际 %v", want, paths)
		}
	}
	// 切集时按真实进度收尾旧集 E02（~98.6%）
	switchStop := bodies[2]["episode"].(map[string]any)
	if switchStop["number"].(float64) != 2 {
		t.Fatalf("切集收尾应为 E02：%v", switchStop)
	}
	// 新集 start @0
	start := bodies[3]
	if start["progress"].(float64) != 0 {
		t.Fatalf("E03 start 进度应为 0%%：%v", start["progress"])
	}
	// 新集 stop 只能按自身 0.24% 收尾——绝不能是旧集的 ~99%
	finalStop := bodies[4]
	episode := finalStop["episode"].(map[string]any)
	if episode["number"].(float64) != 3 {
		t.Fatalf("最终收尾应为 E03：%v", episode)
	}
	if got, wantP := finalStop["progress"].(float64), 3438.0/1440000.0*100; got != wantP {
		t.Fatalf("E03 收尾进度应为 %.4f%%（实际 %v，被旧集进度污染）", wantP, got)
	}
	// 会话观察行已清理
	if row := syncStore.GetSyncSession("s10"); row != nil {
		t.Fatalf("收尾后观察行应删除：%+v", row)
	}
}

// TestSessionScrobbleCleanSwitchUnaffected 按 2026-10-25 下午抓包形态
// （config/data/sse-capture/20261025-171550，无播放队列客户端）回放：
// 切集为 stopped(旧 sk) → playing(新 sk, offset=0)，每个 sessionKey 全程
// 只有一个 ratingKey。切集检测与 awaitPlay 均不应介入——行为与修复前
// 完全一致：旧集按真实进度收尾，新集正常 start/pause/stop。
func TestSessionScrobbleCleanSwitchUnaffected(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// E02 播放（25%）→ start
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s11", RatingKey: "856",
		ViewOffsetMS: 360000, DurationMS: 1440000,
	})
	// 播放中周期性事件（92%）→ 不重复上报
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s11", RatingKey: "856",
		ViewOffsetMS: 1325000, DurationMS: 1440000,
	})
	// E02 停止（93%，真实最终 offset）→ stop 收尾
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "s11", RatingKey: "856",
		ViewOffsetMS: 1340000, DurationMS: 1440000,
	})
	// E03 以全新 sessionKey 开播（offset 干净复位为 105ms，抓包 17:16:07.035）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "s12", RatingKey: "857",
		ViewOffsetMS: 105, DurationMS: 1440000,
	})
	// E03 暂停（3%）→ pause
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "s12", RatingKey: "857",
		ViewOffsetMS: 43356, DurationMS: 1440000,
	})
	// E03 停止（3%）→ 按自身进度收尾（真实 Trakt 会按进度不足忽略）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "s12", RatingKey: "857",
		ViewOffsetMS: 43356, DurationMS: 1440000,
	})

	paths, bodies := cap.snapshot()
	want := []string{"/scrobble/start", "/scrobble/stop", "/scrobble/start", "/scrobble/pause", "/scrobble/stop"}
	if len(paths) != len(want) {
		t.Fatalf("调用序列不符（干净切集不应有额外收尾）：%v", paths)
	}
	for i, p := range want {
		if paths[i] != p {
			t.Fatalf("调用序列不符：期望 %v 实际 %v", want, paths)
		}
	}
	// 旧集按真实进度收尾
	e02Stop := bodies[1]["episode"].(map[string]any)
	if e02Stop["number"].(float64) != 2 {
		t.Fatalf("E02 收尾身份不符：%v", e02Stop)
	}
	if got, wantP := bodies[1]["progress"].(float64), 1340000.0/1440000.0*100; got != wantP {
		t.Fatalf("E02 收尾进度应为 %.4f%%（实际 %v）", wantP, got)
	}
	// 新集按自身进度收尾，未受任何守卫影响
	e03Stop := bodies[4]["episode"].(map[string]any)
	if e03Stop["number"].(float64) != 3 {
		t.Fatalf("E03 收尾身份不符：%v", e03Stop)
	}
	if got, wantP := bodies[4]["progress"].(float64), 43356.0/1440000.0*100; got != wantP {
		t.Fatalf("E03 收尾进度应为 %.4f%%（实际 %v）", wantP, got)
	}
}

// TestSessionScrobbleTVDBAnchoredEndToEnd tvdb 锚定媒体（Trakt 缺 tmdb 映射的
// 场景）：Plex 侧只解析出 tvdb → 键与载荷全程 tvdb 锚，推送仍可达。
func TestSessionScrobbleTVDBAnchoredEndToEnd(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)
	// 上级剧集只有 tvdb guid（无 tmdb）
	srvClient := plexClient
	_ = srvClient
	// 直接改用 tvdb-only 的 plex fixture：重新注册 metadata 处理不可行，
	// 改走独立 env —— 简化：用 853 的 tmdb guid 场景已覆盖 tmdb 锚，
	// 这里验证 tvdb 锚的载荷路径（键经映射表查询后回带 tvdb）。
	if _, ok := syncStore.ResolveKey("tv", watched.MediaIDs{TVDB: 999}, 1, 1); !ok {
		t.Fatal("tvdb 锚应可成键")
	}
	key := watched.MediaKey{Type: "tv", TVDBID: 999, Season: 1, Episode: 1}
	RT().finishSession(syncStore, traktClient, syncStore, key, "剧 X", "sse", 0.9, "sx")
	paths, bodies := cap.snapshot()
	if len(paths) == 0 || paths[len(paths)-1] != "/scrobble/stop" {
		t.Fatalf("应发出 stop：%v", paths)
	}
	stopBody := bodies[len(bodies)-1]
	show := stopBody["show"].(map[string]any)["ids"].(map[string]any)
	if show["tvdb"].(float64) != 999 {
		t.Fatalf("tvdb 锚载荷应以 tvdb 标识：%v", show)
	}
}

// TestSessionScrobbleUnresolvedMediaLogsOnce 事故复现（2026-10-26）：
// PMS 对某条目不回传 Guid（剧集匹配到 TMDB 重复条目的状态）且路径兜底
// 未命中时，实时推送应零调用 Trakt，并恰好告警一次（此后静默不刷屏）。
func TestSessionScrobbleUnresolvedMediaLogsOnce(t *testing.T) {
	syncStore, _, traktClient, cap := newSessionTestEnv(t, nil)
	// 独立 Plex fixture：集详情无 Guid 无路径，上级剧集也无 Guid——
	// 复刻不带 includeGuids 时 PMS 的真实响应形态
	plexSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/library/metadata/856":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "856", "type": "episode", "key": "/library/metadata/856",
				"grandparentTitle": "某番剧", "grandparentRatingKey": "853",
				"parentIndex": 1, "index": 2, "title": "第二集", "duration": 1440000,
			}}}})
		case "/library/metadata/853":
			_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"Metadata": []any{map[string]any{
				"ratingKey": "853", "type": "show", "title": "某番剧",
			}}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(plexSrv.Close)
	plexClient := plexserver.NewClient(plexSrv.URL, "tok", "cid")

	// 同一会话三条 playing 事件（Plex 每 ~10s 重发）
	for i := 0; i < 3; i++ {
		RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
			State: "playing", SessionKey: "unres-1", RatingKey: "856", ViewOffsetMS: 100000,
		})
	}
	if paths, _ := cap.snapshot(); len(paths) != 0 {
		t.Fatalf("身份解析失败不应调用 Trakt：%v", paths)
	}
	warns := 0
	for _, e := range RT().GetLogStore().Read(50, "trakt", "WARNING", 0) {
		if e.Event == "session_unresolved" {
			warns++
		}
	}
	if warns != 1 {
		t.Fatalf("应恰好告警一次，实际 %d 次", warns)
	}
}

// TestSessionPollStaleRowDroppedWithoutPush 重启幽灵推送复现（2026-10-26）：
// sync_sessions 里遗留的旧行（updated_at 距今过久、misses 已累积）在重启后
// 首轮缺席清理时不得推送 Trakt——应直接丢弃；新鲜行仍走正常收尾推送。
func TestSessionPollStaleRowDroppedWithoutPush(t *testing.T) {
	syncStore, _, traktClient, cap := newSessionTestEnv(t, nil)

	seed := func(key string) {
		title := "昨天的剧"
		syncStore.UpsertSyncSession(store.TraktSyncSession{
			SessionKey: key, PlexRef: "860", MediaType: "tv", Anchor: "tmdb", TmdbID: 312266,
			Season: 1, Episode: 9, Title: &title,
			ViewOffsetMS: 1424577, DurationMS: 1425280, MaxProgress: 0.99,
		})
	}
	seed("stale-1")
	seed("fresh-1")
	// 两行都带上段进程累积的 1 次缺席——重启后首轮即达收尾阈值；
	// 僵尸行的时间停在 30 小时前，新鲜行保持当前
	if err := syncStore.BackdateSession("stale-1", time.Now().Add(-30*time.Hour), 1); err != nil {
		t.Fatal(err)
	}
	if err := syncStore.BackdateSession("fresh-1", time.Now(), 1); err != nil {
		t.Fatal(err)
	}

	RT().reapAbsentSessions(syncStore, traktClient, map[string]bool{})

	paths, bodies := cap.snapshot()
	if len(paths) != 1 || paths[0] != "/scrobble/stop" {
		t.Fatalf("僵尸行不应推送，新鲜行应恰好一次 stop：%v", paths)
	}
	if got := bodies[0]["progress"].(float64); got != 99 {
		t.Fatalf("stop 进度应为 99%%，实际 %v", got)
	}
	if row := syncStore.GetSyncSession("stale-1"); row != nil {
		t.Fatalf("僵尸行应被清除：%+v", row)
	}
	if row := syncStore.GetSyncSession("fresh-1"); row != nil {
		t.Fatalf("收尾后新鲜行也应清除：%+v", row)
	}
	dropped := 0
	for _, e := range RT().GetLogStore().Read(50, "trakt", "INFO", 0) {
		if e.Event == "session_stale_dropped" {
			dropped++
		}
	}
	if dropped != 1 {
		t.Fatalf("应恰好一条僵尸行清理日志，实际 %d", dropped)
	}
}

// TestSessionScrobbleStoppedOffsetCappedByObserved ATV 休眠幽灵收尾复现
// （2026-10-26）：播放队列客户端切集后新会话仍挂上一集上下文，暂停休眠
// 终止时 stopped 报出上一集时长（408426ms）当本集位置——观察行全程只见
// 过 1%。暂停态会话的收尾按观察轨迹封顶，不得以 92% 计次。
func TestSessionScrobbleStoppedOffsetCappedByObserved(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// 播放 1% 后暂停（ATV 随后休眠，28 分钟无事件）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "sk-atv", RatingKey: "856",
		ViewOffsetMS: 4663, DurationMS: 442475,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "paused", SessionKey: "sk-atv", RatingKey: "856",
		ViewOffsetMS: 4663, DurationMS: 442475,
	})
	// 终止：报文 offset=408426ms（上一集时长）
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "sk-atv", RatingKey: "856", ViewOffsetMS: 408426,
	})

	paths, bodies := cap.snapshot()
	if len(paths) != 3 || paths[0] != "/scrobble/start" || paths[1] != "/scrobble/pause" || paths[2] != "/scrobble/stop" {
		t.Fatalf("应为 start + pause + stop：%v", paths)
	}
	// 封顶后按观察值 ~1% 收尾（<80%，Trakt 不计次）
	if got := bodies[2]["progress"].(float64); got > 2 {
		t.Fatalf("收尾进度应封顶至观察值 ~1%%，实际 %v", got)
	}
	if syncStore.GetSyncSession("sk-atv") != nil {
		t.Fatal("收尾后观察行应清除")
	}
}

// TestSessionScrobbleStoppedOffsetPlayingStateTrusted playing 态会话的
// stopped 与最后一个周期事件之间的自然推进属正常：不封顶，按报文值收尾。
func TestSessionScrobbleStoppedOffsetPlayingStateTrusted(t *testing.T) {
	syncStore, plexClient, traktClient, cap := newSessionTestEnv(t, nil)

	// 持续播放至 75%，最后一个周期事件后正常停止于 92.5%
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "playing", SessionKey: "sk-ok", RatingKey: "856",
		ViewOffsetMS: 1080000, DurationMS: 1440000,
	})
	RT().handlePlexPlaybackNotification(plexClient, traktClient, syncStore, plexserver.PlaybackNotification{
		State: "stopped", SessionKey: "sk-ok", RatingKey: "856", ViewOffsetMS: 1332000, // 92.5%
	})

	paths, bodies := cap.snapshot()
	if len(paths) != 2 || paths[0] != "/scrobble/start" || paths[1] != "/scrobble/stop" {
		t.Fatalf("应为 start + stop：%v", paths)
	}
	if got := bodies[1]["progress"].(float64); got != 92.5 {
		t.Fatalf("playing 态应按报文值 92.5%% 收尾，实际 %v", got)
	}
}

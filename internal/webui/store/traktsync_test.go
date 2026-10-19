package store

import (
	"path/filepath"
	"testing"

	"meta2cloud/internal/syncengine"
	"meta2cloud/internal/watched"
)

func openTestTraktSyncStore(t *testing.T) *TraktSyncStore {
	t.Helper()
	s, err := OpenTraktSyncStore(filepath.Join(t.TempDir(), "traktsync.db"))
	if err != nil {
		t.Fatalf("打开存储失败：%v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestShadowRoundTrip(t *testing.T) {
	s := openTestTraktSyncStore(t)
	key := watched.MediaKey{Type: "tv", TMDBID: 100, Season: 1, Episode: 2}
	watchedTrue := true
	watchedFalse := false
	sh := syncengine.Shadow{
		Key: key, PlexRef: "rating-1",
		PlexSeen: true, PlexWatched: &watchedTrue, PlexPlays: 2, PlexLastAt: 1700000000,
		TraktSeen: true, TraktWatched: &watchedFalse, TraktPlays: 0, TraktLastAt: 0,
		SessionPushedAt: 1700000100,
	}
	if err := s.SaveShadow([]syncengine.Shadow{sh}); err != nil {
		t.Fatalf("SaveShadow 出错：%v", err)
	}
	got, ok, err := s.GetShadow(key)
	if err != nil || !ok {
		t.Fatalf("GetShadow 失败：ok=%v err=%v", ok, err)
	}
	if got.PlexRef != "rating-1" || !got.PlexSeen || got.PlexWatched == nil || !*got.PlexWatched ||
		got.PlexPlays != 2 || got.PlexLastAt != 1700000000 ||
		got.TraktWatched == nil || *got.TraktWatched || got.SessionPushedAt != 1700000100 {
		t.Fatalf("影子往返不符：%+v", got)
	}

	// upsert 覆盖
	sh.PlexPlays = 3
	if err := s.SaveShadow([]syncengine.Shadow{sh}); err != nil {
		t.Fatalf("SaveShadow 出错：%v", err)
	}
	got, _, _ = s.GetShadow(key)
	if got.PlexPlays != 3 {
		t.Fatalf("upsert 后 PlexPlays=%d", got.PlexPlays)
	}

	all, err := s.AllShadow()
	if err != nil || len(all) != 1 {
		t.Fatalf("AllShadow 结果：n=%d err=%v", len(all), err)
	}
}

func TestUpdateTraktWatchedForwardOnly(t *testing.T) {
	s := openTestTraktSyncStore(t)
	key := watched.MediaKey{Type: "movie", TMDBID: 200}

	// 行不存在时自动建行
	if err := s.UpdateTraktWatched(key, 1000, 1000); err != nil {
		t.Fatalf("UpdateTraktWatched 出错：%v", err)
	}
	got, ok, _ := s.GetShadow(key)
	if !ok || got.TraktWatched == nil || !*got.TraktWatched || got.TraktLastAt != 1000 || got.SessionPushedAt != 1000 {
		t.Fatalf("建行语义不符：%+v", got)
	}

	// 只进不退：更旧的时间戳不覆盖
	if err := s.UpdateTraktWatched(key, 500, 500); err != nil {
		t.Fatalf("UpdateTraktWatched 出错：%v", err)
	}
	got, _, _ = s.GetShadow(key)
	if got.TraktLastAt != 1000 || got.SessionPushedAt != 1000 {
		t.Fatalf("时间戳应只进不退：%+v", got)
	}
}

func TestGetShadowByPlexRef(t *testing.T) {
	s := openTestTraktSyncStore(t)
	sh := syncengine.Shadow{
		Key: watched.MediaKey{Type: "movie", TMDBID: 300}, PlexRef: "ref-xyz",
	}
	if err := s.SaveShadow([]syncengine.Shadow{sh}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := s.GetShadowByPlexRef("ref-xyz")
	if err != nil || !ok || got.Key.TMDBID != 300 {
		t.Fatalf("按 ref 反查失败：ok=%v got=%+v err=%v", ok, got, err)
	}
	if _, ok, _ := s.GetShadowByPlexRef("nope"); ok {
		t.Fatal("未命中应返回 false")
	}
}

func TestSyncRunsLifecycle(t *testing.T) {
	s := openTestTraktSyncStore(t)
	runID, err := s.StartSyncRun("manual")
	if err != nil || runID == 0 {
		t.Fatalf("StartSyncRun：id=%d err=%v", runID, err)
	}
	if err := s.InsertRunItem(runID, syncengine.RunItem{
		Key:       watched.MediaKey{Type: "movie", TMDBID: 1, Season: 0, Episode: 0},
		Title:     "测试片",
		Direction: "to_trakt", Action: "watched", Success: true,
	}); err != nil {
		t.Fatalf("InsertRunItem 出错：%v", err)
	}
	if err := s.FinishSyncRun(runID, syncengine.RunSummary{ToTrakt: 1, ToPlex: 2, Conflicts: 1}); err != nil {
		t.Fatalf("FinishSyncRun 出错：%v", err)
	}

	runs, total, err := s.ListSyncRuns(10, 0)
	if err != nil || total != 1 || len(runs) != 1 {
		t.Fatalf("ListSyncRuns：total=%d n=%d err=%v", total, len(runs), err)
	}
	if runs[0].ToTrakt != 1 || runs[0].ToPlex != 2 || runs[0].Conflicts != 1 || runs[0].FinishedAt == nil {
		t.Fatalf("run 统计不符：%+v", runs[0])
	}
	if last := s.GetLastFinishedSyncRun(); last == nil || last.ID != runID {
		t.Fatalf("GetLastFinishedSyncRun 不符：%+v", last)
	}
	items, err := s.ListRunItems(runID, 10)
	if err != nil || len(items) != 1 || items[0].Direction != "to_trakt" || !items[0].Success {
		t.Fatalf("ListRunItems 不符：n=%d err=%v", len(items), err)
	}
}

func TestFailStaleRunningRuns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "traktsync.db")
	s, err := OpenTraktSyncStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.StartSyncRun("schedule")
	// 不调用 FinishSyncRun，模拟进程崩溃；重开存储应收尾
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s2, err := OpenTraktSyncStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	runs, _, _ := s2.ListSyncRuns(10, 0)
	if len(runs) != 1 || runs[0].FinishedAt == nil || runs[0].Error == nil {
		t.Fatalf("遗留 run 应被收尾为失败：%+v", runs)
	}
}

func TestSyncSessions(t *testing.T) {
	s := openTestTraktSyncStore(t)
	title := "播放中的电影"
	s.UpsertSyncSession(TraktSyncSession{
		SessionKey: "sess-1", PlexRef: "r1", MediaType: "movie", TmdbID: 1,
		Title: &title, ViewOffsetMS: 100, DurationMS: 200, MaxProgress: 0.5,
	})
	row := s.GetSyncSession("sess-1")
	if row == nil || row.TmdbID != 1 || row.MaxProgress != 0.5 {
		t.Fatalf("会话行不符：%+v", row)
	}
	// max_progress 取历史最大
	s.UpsertSyncSession(TraktSyncSession{
		SessionKey: "sess-1", PlexRef: "r1", MediaType: "movie", TmdbID: 1,
		ViewOffsetMS: 50, DurationMS: 200, MaxProgress: 0.25,
	})
	row = s.GetSyncSession("sess-1")
	if row.MaxProgress != 0.5 {
		t.Fatalf("max_progress 应取历史最大：%+v", row)
	}

	if misses := s.IncrementSessionMiss("sess-1"); misses != 1 {
		t.Fatalf("miss 计数=%d", misses)
	}
	// 再次 upsert 应清零 miss
	s.UpsertSyncSession(TraktSyncSession{SessionKey: "sess-1", PlexRef: "r1", MediaType: "movie", TmdbID: 1})
	if row := s.GetSyncSession("sess-1"); row.Misses != 0 {
		t.Fatalf("upsert 应清零 miss：%+v", row)
	}

	s.DeleteSyncSession("sess-1")
	if row := s.GetSyncSession("sess-1"); row != nil {
		t.Fatalf("删除后应不存在：%+v", row)
	}
}

// TestPurgeLegacySentinelRuns plex_activity 遗留记录连同明细启动即清理。
func TestPurgeLegacySentinelRuns(t *testing.T) {
	s := openTestTraktSyncStore(t)
	runA, _ := s.StartSyncRun("plex_activity")
	runB, _ := s.StartSyncRun("manual")
	_ = s.InsertRunItem(runA, syncengine.RunItem{Title: "旧哨兵", Direction: "to_trakt", Action: "watched", Success: true})
	_ = s.InsertRunItem(runB, syncengine.RunItem{Title: "正常", Direction: "to_plex", Action: "watched", Success: true})
	_ = s.FinishSyncRun(runA, syncengine.RunSummary{ToTrakt: 1})
	_ = s.FinishSyncRun(runB, syncengine.RunSummary{ToPlex: 1})

	s.purgeLegacySentinelRuns()

	runs, total, err := s.ListSyncRuns(10, 0)
	if err != nil || total != 1 || len(runs) != 1 {
		t.Fatalf("清理后应仅剩 1 条：total=%d n=%d err=%v", total, len(runs), err)
	}
	if runs[0].ID != runB {
		t.Fatalf("留下的应是 manual 记录：%+v", runs[0])
	}
	if items, _ := s.ListRunItems(runA, 10); len(items) != 0 {
		t.Fatalf("plex_activity 明细应一并清理：n=%d", len(items))
	}
}

// TestRecordSessionPushRunAndFilter 实时推送落记录；零变化/尚无明细的 run 不进列表，
// 落下首条明细后（进行中）即显示。
func TestRecordSessionPushRunAndFilter(t *testing.T) {
	s := openTestTraktSyncStore(t)

	emptyID, _ := s.StartSyncRun("schedule")
	_ = s.FinishSyncRun(emptyID, syncengine.RunSummary{})
	runningID, _ := s.StartSyncRun("manual") // 尚无明细：不应显示

	if err := s.RecordSessionPushRun("sse", syncengine.RunItem{
		Key: watched.MediaKey{Type: "tv", TMDBID: 9, Season: 1, Episode: 2},
		Title: "剧 S01E02", Direction: "to_trakt", Action: "watched",
	}, 0.98, true, ""); err != nil {
		t.Fatalf("RecordSessionPushRun 成功记录出错：%v", err)
	}
	if err := s.RecordSessionPushRun("poll", syncengine.RunItem{
		Key: watched.MediaKey{Type: "movie", TMDBID: 8},
		Title: "片", Direction: "to_trakt", Action: "watched",
	}, 0.9, false, "Trakt 写入失败：boom"); err != nil {
		t.Fatalf("RecordSessionPushRun 失败记录出错：%v", err)
	}

	// 进行中 run 未落明细前不可见（避免「出现又消失」的观感）
	runs, total, err := s.ListSyncRuns(10, 0)
	if err != nil || total != 2 || len(runs) != 2 {
		t.Fatalf("应仅显示 2 条推送记录：total=%d n=%d err=%v", total, len(runs), err)
	}
	// 落下首条明细后（仍在进行中）即出现——「同步一条显示一条」
	if err := s.InsertRunItem(runningID, syncengine.RunItem{
		Key: watched.MediaKey{Type: "tv", TMDBID: 7, Season: 1, Episode: 1},
		Title: "另一剧 S01E01", Direction: "to_trakt", Action: "watched", Success: true,
	}); err != nil {
		t.Fatalf("InsertRunItem 出错：%v", err)
	}
	runs, total, err = s.ListSyncRuns(10, 0)
	if err != nil || total != 3 || len(runs) != 3 {
		t.Fatalf("明细落地后应显示 3 条：total=%d n=%d err=%v", total, len(runs), err)
	}
	var running *TraktSyncRun
	for i := range runs {
		if runs[i].ID == runningID {
			running = &runs[i]
		}
	}
	if running == nil || running.FinishedAt != nil {
		t.Fatalf("进行中 run 应显示：%+v", runs)
	}
	// 零变化 run 被过滤
	for _, r := range runs {
		if r.ID == emptyID {
			t.Fatal("零变化 run 不应出现在列表")
		}
	}
}

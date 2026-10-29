package store

import (
	"path/filepath"
	"testing"
)

func TestIngestRoundTrip(t *testing.T) {
	s, err := OpenIngestStore(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	season, episode := 1, 3
	id, err := s.RecordIngest(IngestRecord{
		MediaType: "tv", TmdbID: 1396, Title: "绝命毒师", OriginalTitle: "Breaking Bad",
		Year: "2008", Season: &season, Episode: &episode, EpisodeTitle: "...Bag's in the River",
		DriveFolderID: "f1", OriginalName: "Breaking.Bad.S01E03.mkv", Status: "success",
	})
	if err != nil || id == 0 {
		t.Fatalf("record err=%v id=%d", err, id)
	}
	// 失败记录
	if _, err := s.RecordIngest(IngestRecord{
		MediaType: "movie", Title: "测试", Status: "failed", ErrorMessage: "移动失败",
	}); err != nil {
		t.Fatal(err)
	}

	result := s.Query(IngestQuery{Page: 1, PageSize: 10})
	if result.Pagination["total"] != int64(2) {
		t.Errorf("total = %v", result.Pagination["total"])
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %d", len(result.Items))
	}
	// 过滤
	tvOnly := s.Query(IngestQuery{MediaType: "tv"})
	if len(tvOnly.Items) != 1 || tvOnly.Items[0].Title != "绝命毒师" {
		t.Errorf("tvOnly = %+v", tvOnly.Items)
	}
	failed := s.Query(IngestQuery{Status: "failed"})
	if len(failed.Items) != 1 || failed.Items[0].ErrorMessage != "移动失败" {
		t.Errorf("failed = %+v", failed.Items)
	}
	// 标题年份匹配
	if m := s.FindMatchByTitleYear("tv", "绝命毒师", "2008"); m == nil {
		t.Error("按标题年份应找到记录")
	}
	if m := s.FindMatchByTitleYear("tv", "完全不同", "2008"); m != nil {
		t.Error("不同标题不应匹配")
	}
	// 统计
	stats := s.Stats(7)
	if stats.Total != 2 || stats.TVEpisodes != 1 {
		t.Errorf("stats = %+v", stats)
	}
}

func TestLogStoreFlow(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenLogStore(dir, 7, "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.Write("library", "item_removed", "SUCCESS", "媒体项已移出媒体库", map[string]any{"id": "x"})
	s.Write("pipeline", "run_failed", "error", "整理失败", nil)

	items := s.Read(100, "", "", 0)
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	// id 倒序（最新在前）
	if items[0].Category != "pipeline" {
		t.Errorf("order = %v", items[0])
	}
	if items[1].Level != "SUCCESS" {
		t.Errorf("level = %q", items[1].Level)
	}
	// 过滤
	pipelineOnly := s.Read(100, "pipeline", "", 0)
	if len(pipelineOnly) != 1 {
		t.Errorf("category filter = %d", len(pipelineOnly))
	}
	errorOnly := s.Read(100, "", "ERROR", 0)
	if len(errorOnly) != 1 {
		t.Errorf("level filter (大小写归一) = %d", len(errorOnly))
	}
	// sinceID
	maxID := s.MaxID()
	if newer := s.Read(100, "", "", maxID); len(newer) != 0 {
		t.Errorf("sinceMax 应为空, got %d", len(newer))
	}
	// 摘要
	summary := s.Summary(10)
	if summary["total"] != 2 {
		t.Errorf("summary = %v", summary)
	}

}

func TestU115AutoStoreFlow(t *testing.T) {
	s, err := OpenU115AutoStore(filepath.Join(t.TempDir(), "u115_auto.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 首次观察（未完成）
	st := s.UpsertObservation("k1", "hash1", "任务A", 1, 55.5, nil, nil, false)
	if st == nil || st.Status != 1 || st.FinishedSeenAt != nil {
		t.Fatalf("首次观察 = %+v", st)
	}
	// 完成
	st = s.UpsertObservation("k1", "hash1", "任务A", 2, 100.0, nil, nil, true)
	if st.FinishedSeenAt == nil {
		t.Fatal("完成时间未记录")
	}
	// 触发整理
	s.MarkTriggered("k1")
	last := s.GetLastTriggered()
	if last == nil || last.TaskKey != "k1" {
		t.Fatalf("lastTriggered = %+v", last)
	}
	// 回退到未完成 → finished 清空
	st = s.UpsertObservation("k1", "hash1", "任务A", 1, 60.0, nil, nil, false)
	if st.FinishedSeenAt != nil {
		t.Error("未完成时 finished_seen_at 应清空")
	}
}

func TestRSSStoreFlow(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	s, err := OpenRSSStore(dbPath, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// 创建订阅
	tmdbID := int64(3065)
	sub, err := s.CreateSubscription(map[string]any{
		"name": "测试番", "media_title": "测试番", "media_type": "tv", "tmdb_id": tmdbID,
		"poster_url": "https://image.tmdb.org/t/p/w500/p.jpg",
		"site":       "mikan", "rss_url": "https://mikanani.me/RSS/Bangumi?bangumiId=3065",
		"subgroup_name": "", "season_number": 1, "start_episode": 1,
		"keyword_all": `["测试"]`, "push_target": "u115",
	})
	if err != nil || sub == nil {
		t.Fatalf("create err=%v sub=%v", err, sub)
	}
	// 重复创建应报错
	if _, err := s.CreateSubscription(map[string]any{
		"name": "测试番2", "media_title": "测试番", "media_type": "tv", "tmdb_id": tmdbID,
		"site": "mikan", "rss_url": "https://mikanani.me/RSS/Bangumi?bangumiId=3065",
		"subgroup_name": "", "season_number": 1, "start_episode": 1,
		"keyword_all": "[]", "push_target": "u115",
	}); err != ErrDuplicateSubscription {
		t.Errorf("重复订阅 err = %v", err)
	}
	// poster 归一化为路径
	if sub.PosterURL == nil || *sub.PosterURL != "/p.jpg" {
		t.Errorf("poster = %v", sub.PosterURL)
	}
	// 更新（改起始集数 → 不再重复）
	updated, err := s.UpdateSubscription(sub.ID, map[string]any{"start_episode": 5})
	if err != nil || updated == nil || updated.StartEpisode != 5 {
		t.Fatalf("update err=%v updated=%+v", err, updated)
	}
	// 命中记录 + 去重
	season, episode := 1, 2
	if _, err := s.RecordHit(map[string]any{
		"subscription_id": sub.ID, "episode_title": "第2集",
		"season_number": season, "episode_number": episode,
		"dedupe_key": "s1e2", "pushed_to": "u115", "push_status": "success",
	}); err != nil {
		t.Fatal(err)
	}
	if !s.HitExists(sub.ID, "s1e2") {
		t.Error("hit 应存在")
	}
	if _, err := s.RecordHit(map[string]any{
		"subscription_id": sub.ID, "episode_title": "重复",
		"dedupe_key": "s1e2", "pushed_to": "u115", "push_status": "success",
	}); err == nil {
		t.Error("重复 dedupe_key 应报错")
	}
	if n := s.CountHits(sub.ID); n != 1 {
		t.Errorf("count = %d", n)
	}
	// joined 列表（同库 JOIN tmdb_media/library_media）
	joined := s.ListSubscriptionsJoined()
	if len(joined) != 1 {
		t.Fatalf("joined = %d", len(joined))
	}
	if joined[0]["hit_count"] != int64(1) {
		t.Errorf("hit_count = %v", joined[0]["hit_count"])
	}
	// 标记检查/命中
	s.MarkChecked(sub.ID, "", "")
	s.MarkMatched(sub.ID, "")
	got, _ := s.GetSubscription(sub.ID)
	if got.LastCheckedAt == nil || got.LastMatchedAt == nil {
		t.Error("标记未生效")
	}
	// 删除
	if !s.DeleteSubscription(sub.ID) {
		t.Error("删除失败")
	}
}

// TestIngestQueryKeywordEscapesLike 关键词搜索应转义 LIKE 通配符：
// "%" 与 "_" 按字面匹配，不被当作通配符匹配全表。
func TestIngestQueryKeywordEscapesLike(t *testing.T) {
	s, err := OpenIngestStore(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.RecordIngest(IngestRecord{
		MediaType: "tv", TmdbID: 1396, Title: "绝命毒师", OriginalTitle: "Breaking Bad",
		Year: "2008", Status: "success",
	}); err != nil {
		t.Fatal(err)
	}
	// 通配符字面量：库中不含 % 或 _ 字符的记录不应被命中
	for _, kw := range []string{"%", "_", "%毒%"} {
		if got := s.Query(IngestQuery{Keyword: kw}); len(got.Items) != 0 {
			t.Errorf("关键词 %q 命中 %d 条，应为 0（通配符须按字面匹配）", kw, len(got.Items))
		}
	}
	// 普通关键词与中间匹配仍正常
	if got := s.Query(IngestQuery{Keyword: "绝命"}); len(got.Items) != 1 {
		t.Errorf("普通关键词命中 %d 条, want 1", len(got.Items))
	}
}

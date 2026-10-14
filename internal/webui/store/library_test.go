package store

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *LibraryStore {
	t.Helper()
	s, err := OpenLibraryStore(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestTmdbDetailRoundTrip(t *testing.T) {
	s := newTestStore(t)
	data := map[string]any{"id": 27205.0, "title": "Inception", "poster_path": "/in.jpg", "vote_average": 8.4}
	if err := s.UpsertTmdbDetail("movie", 27205, "zh-CN", data, float64(time.Now().Unix())+3600); err != nil {
		t.Fatal(err)
	}
	entry := s.GetTmdbEntry("movie", 27205)
	if entry == nil {
		t.Fatal("实体未写入")
	}
	if entry.Title != "Inception" || entry.VoteAverage != 8.4 {
		t.Errorf("entry = %+v", entry)
	}
}

func TestTmdbSeasonEpisodeMerge(t *testing.T) {
	s := newTestStore(t)
	expires := float64(time.Now().Unix()) + 3600
	_ = s.UpsertTmdbDetail("tv", 1396, "zh-CN", map[string]any{"name": "绝命毒师"}, expires)
	_ = s.UpsertTmdbSeasonDetail(1396, "zh-CN", 1, map[string]any{"name": "Season 1"}, expires)
	_ = s.UpsertTmdbEpisodeDetail(1396, "zh-CN", 1, 3, map[string]any{"name": "ep3"}, expires)

	entry := s.GetTmdbEntry("tv", 1396)
	if entry == nil || entry.Title != "绝命毒师" {
		t.Fatalf("detail 被季节/集写入覆盖: %+v", entry)
	}
	if entry.SeasonDetailsJSON["1"] == nil {
		t.Error("season 详情缺失")
	}
	if entry.EpisodeDetails["1:3"] == nil {
		t.Error("episode 详情缺失")
	}
}

func TestTmdbCacheAdapter(t *testing.T) {
	s := newTestStore(t)
	cache := NewTmdbCache(s)

	// detail 写入与读取
	cache.Set("/movie/27205", map[string]any{"id": 27205.0, "title": "Inception"}, "zh-CN")
	if got, ok := cache.Get("/movie/27205", "zh-CN"); !ok || got["title"] != "Inception" {
		t.Fatalf("detail 缓存未命中: %v %v", got, ok)
	}
	// 搜索路径不缓存
	cache.Set("/search/movie?query=x", map[string]any{"results": []any{}}, "zh-CN")
	if _, ok := cache.Get("/search/movie?query=x", "zh-CN"); ok {
		t.Error("搜索路径不应缓存")
	}
	// 季/集
	cache.Set("/tv/1396", map[string]any{"name": "绝命毒师"}, "zh-CN")
	cache.Set("/tv/1396/season/1", map[string]any{"name": "S1"}, "zh-CN")
	cache.Set("/tv/1396/season/1/episode/3", map[string]any{"name": "ep3"}, "zh-CN")
	if got, ok := cache.Get("/tv/1396/season/1", "zh-CN"); !ok || got["name"] != "S1" {
		t.Errorf("season 缓存未命中: %v", got)
	}
	if got, ok := cache.Get("/tv/1396/season/1/episode/3", "zh-CN"); !ok || got["name"] != "ep3" {
		t.Errorf("episode 缓存未命中: %v", got)
	}
	// 语言不匹配
	if _, ok := cache.Get("/tv/1396", "en-US"); ok {
		t.Error("语言不匹配不应命中")
	}
	// 过期
	_ = s.UpsertTmdbDetail("movie", 999, "zh-CN", map[string]any{"title": "old"}, float64(time.Now().Unix())-10)
	if _, ok := cache.Get("/movie/999", "zh-CN"); ok {
		t.Error("过期条目不应命中")
	}
}

func TestLibrarySnapshotFlow(t *testing.T) {
	s := newTestStore(t)
	if snap := s.GetSnapshot("updated_at", "desc"); snap != nil {
		t.Fatal("未扫描时不应有快照")
	}
	movies := []map[string]any{
		{"drive_folder_id": "f1", "media_type": "movie", "tmdb_id": 27205, "title": "Inception", "year": "2010",
			"poster_url": "https://image.tmdb.org/t/p/w500/in.jpg", "folder_modified_time": 100},
	}
	tv := []map[string]any{
		{"drive_folder_id": "f2", "media_type": "tv", "tmdb_id": 1396, "title": "绝命毒师", "year": "2008",
			"total_episodes": 62, "in_library_episodes": 50},
	}
	diff := s.SaveSnapshot(movies, tv)
	if diff.TotalMovies != 1 || diff.TotalTV != 1 || diff.NewMovies != 1 {
		t.Fatalf("diff = %+v", diff)
	}
	snap := s.GetSnapshot("folder_modified_time", "desc")
	if snap == nil || snap.TotalMovies != 1 || snap.TotalTV != 1 {
		t.Fatalf("snap = %+v", snap)
	}
	// poster_url 规范化为路径存储，hydrate 后恢复完整 URL
	if v := snap.Movies[0]["poster_url"]; v != "https://image.tmdb.org/t/p/w500/in.jpg" {
		t.Errorf("poster_url = %v", v)
	}
	// 再保存一次：新增为 0
	diff2 := s.SaveSnapshot(movies, tv)
	if diff2.NewMovies != 0 {
		t.Errorf("重复保存 new_movies = %d, want 0", diff2.NewMovies)
	}
	// 统计
	stats := s.GetStats()
	if stats["total_movies"] != 1 || stats["completion_rate"] != 80.6 && stats["completion_rate"] != 80.7 {
		t.Errorf("stats = %v", stats)
	}
}

func TestMarkMissingAndPatch(t *testing.T) {
	s := newTestStore(t)
	s.SaveIncremental([]map[string]any{
		{"drive_folder_id": "a", "media_type": "movie", "tmdb_id": 1, "title": "A"},
		{"drive_folder_id": "b", "media_type": "movie", "tmdb_id": 2, "title": "B"},
	}, nil)
	if n := s.MarkMissingFolders([]string{"a"}); n != 1 {
		t.Errorf("markMissing = %d, want 1", n)
	}
	if snap := s.GetSnapshot("", ""); snap.TotalMovies != 1 {
		t.Errorf("in_library 过滤失败: %+v", snap)
	}
	if !s.PatchItem("a", map[string]any{"title": "A2"}) {
		t.Fatal("patch 失败")
	}
	if item := s.GetLibraryItemByFolderID("a"); item["title"] != "A2" {
		t.Errorf("patch 后 title = %v", item["title"])
	}
}

func TestTitlesMatch(t *testing.T) {
	if !TitlesMatch("Breaking Bad", []string{"Breaking  Bad"}) {
		t.Error("忽略空白后应匹配")
	}
	if !TitlesMatch("绝命毒师", []string{"绝命毒师 (2008)"}) {
		t.Error("年份后缀应被剥离")
	}
	if TitlesMatch("ABC", []string{"XYZ"}) {
		t.Error("不同标题不应匹配")
	}
	// 短于 4 字符不做包含匹配
	if TitlesMatch("AB", []string{"ABCD"}) {
		t.Error("短标题包含匹配应被禁止")
	}
}

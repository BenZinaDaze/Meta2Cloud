package store

import (
	"path/filepath"
	"testing"
	"time"

	"meta2cloud/internal/titlematch"
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
	if page := s.GetSnapshotPage(SnapshotQuery{Page: 1, PageSize: 10}); page != nil {
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
	page := s.GetSnapshotPage(SnapshotQuery{SortBy: "folder_modified_time", SortOrder: "desc", Page: 1, PageSize: 10})
	if page == nil || page.TotalMovies != 1 || page.TotalTV != 1 {
		t.Fatalf("page = %+v", page)
	}
	// poster_url 规范化为路径存储，hydrate 后恢复完整 URL
	if v := page.Movies[0]["poster_url"]; v != "https://image.tmdb.org/t/p/w500/in.jpg" {
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

// TestGetStatsDedupMultiFolder 同一实体多目录（历史译名差异）时统计按实体去重，
// 集数取各目录最大值，不因重复目录虚增。
func TestGetStatsDedupMultiFolder(t *testing.T) {
	s := newTestStore(t)
	tv := []map[string]any{
		{"drive_folder_id": "f1", "media_type": "tv", "tmdb_id": 1396, "title": "绝命毒师",
			"total_episodes": 62, "in_library_episodes": 50},
		// 同 tmdb_id 的第二个目录（另一译名）
		{"drive_folder_id": "f2", "media_type": "tv", "tmdb_id": 1396, "title": "Breaking Bad 绝命毒师",
			"total_episodes": 62, "in_library_episodes": 30},
	}
	s.SaveSnapshot(nil, tv)
	stats := s.GetStats()
	if stats["total_tv_shows"] != 1 {
		t.Errorf("total_tv_shows = %v, want 1（多目录按实体去重）", stats["total_tv_shows"])
	}
	if stats["total_episodes_in_library"] != int64(50) {
		t.Errorf("total_episodes_in_library = %v, want 50（取各目录最大值，不虚增）", stats["total_episodes_in_library"])
	}
}

func TestMarkMissingAndPatch(t *testing.T) {
	s := newTestStore(t)
	s.SaveIncremental([]map[string]any{
		{"drive_folder_id": "a", "media_type": "movie", "tmdb_id": 1, "title": "A"},
		{"drive_folder_id": "b", "media_type": "movie", "tmdb_id": 2, "title": "B"},
	}, nil)
	if n, err := s.MarkMissingFolders([]string{"a"}); err != nil || n != 1 {
		t.Errorf("markMissing = %d, %v, want 1, nil", n, err)
	}
	if page := s.GetSnapshotPage(SnapshotQuery{MediaType: "movie", Page: 1, PageSize: 10}); page.TotalMovies != 1 {
		t.Errorf("in_library 过滤失败: %+v", page)
	}
	if !s.PatchItem("a", map[string]any{"title": "A2"}) {
		t.Fatal("patch 失败")
	}
	if item := s.GetLibraryItemByFolderID("a"); item["title"] != "A2" {
		t.Errorf("patch 后 title = %v", item["title"])
	}
}

func TestGetSnapshotPage(t *testing.T) {
	s := newTestStore(t)
	if page := s.GetSnapshotPage(SnapshotQuery{Page: 1, PageSize: 10}); page != nil {
		t.Fatal("未扫描时不应有快照页")
	}

	movies := []map[string]any{
		{"drive_folder_id": "m1", "media_type": "movie", "tmdb_id": 101, "title": "阿凡达", "year": "2009", "folder_modified_time": 300},
		{"drive_folder_id": "m2", "media_type": "movie", "tmdb_id": 102, "title": "盗梦空间", "year": "2010", "folder_modified_time": 200},
		{"drive_folder_id": "m3", "media_type": "movie", "tmdb_id": 103, "title": "星际穿越", "year": "2014", "folder_modified_time": 100},
	}
	tv := []map[string]any{
		// 同一剧集两个目录（译名差异）：集数应取合并最大值 12
		{"drive_folder_id": "t1", "media_type": "tv", "tmdb_id": 201, "title": "令和的斑小姐", "year": "2026", "in_library_episodes": 11, "folder_modified_time": 50},
		{"drive_folder_id": "t2", "media_type": "tv", "tmdb_id": 201, "title": "令和妖神斑小姐", "year": "2026", "in_library_episodes": 12, "folder_modified_time": 60},
	}
	s.SaveSnapshot(movies, tv)

	// 第 1 页（每页 2 条）：电影按 folder_modified_time 倒序
	p1 := s.GetSnapshotPage(SnapshotQuery{SortBy: "folder_modified_time", SortOrder: "desc", Page: 1, PageSize: 2})
	if p1 == nil || len(p1.Movies) != 2 || len(p1.TvShows) != 0 {
		t.Fatalf("第 1 页应为 2 部电影: %+v", p1)
	}
	if p1.Movies[0]["title"] != "阿凡达" || p1.Movies[1]["title"] != "盗梦空间" {
		t.Fatalf("第 1 页排序错误: %v %v", p1.Movies[0]["title"], p1.Movies[1]["title"])
	}
	if p1.TotalMovies != 3 || p1.TotalTV != 1 {
		t.Fatalf("去重总数错误: movies=%d tv=%d", p1.TotalMovies, p1.TotalTV)
	}

	// 第 2 页：剧集（同 tmdb 两目录合并为一张卡，集数取最大）
	p2 := s.GetSnapshotPage(SnapshotQuery{SortBy: "folder_modified_time", SortOrder: "desc", Page: 2, PageSize: 2})
	if len(p2.TvShows) != 1 {
		t.Fatalf("第 2 页应为 1 部剧集: %+v", p2)
	}
	if got := jsonxInt(p2.TvShows[0], "in_library_episodes"); got != 12 {
		t.Fatalf("同 tmdb 目录集数应取最大值 12, got %v", got)
	}

	// 类型过滤：只看电影
	pm := s.GetSnapshotPage(SnapshotQuery{MediaType: "movie", SortBy: "folder_modified_time", SortOrder: "desc", Page: 1, PageSize: 2})
	if len(pm.Movies) != 2 || len(pm.TvShows) != 0 {
		t.Fatalf("类型过滤错误: %+v", pm)
	}

	// 关键词检索（标题）
	pk := s.GetSnapshotPage(SnapshotQuery{Keyword: "盗梦", SortBy: "folder_modified_time", SortOrder: "desc", Page: 1, PageSize: 10})
	if pk.TotalMovies != 1 || len(pk.Movies) != 1 || pk.Movies[0]["title"] != "盗梦空间" {
		t.Fatalf("关键词检索错误: %+v", pk)
	}
}

func jsonxInt(m map[string]any, key string) int {
	if v, ok := m[key].(float64); ok {
		return int(v)
	}
	if v, ok := m[key].(int); ok {
		return v
	}
	return 0
}

func TestBatchTmdbEntriesAndFolderGroups(t *testing.T) {
	s := newTestStore(t)
	expires := float64(time.Now().Unix()) + 3600
	_ = s.UpsertTmdbDetail("tv", 201, "zh-CN", map[string]any{"name": "令和的斑小姐"}, expires)
	_ = s.UpsertTmdbDetail("movie", 101, "zh-CN", map[string]any{"title": "阿凡达"}, expires)

	entries := s.GetTmdbEntriesByKeys([]string{"tv:201", "movie:101", "tv:999"})
	if len(entries) != 2 {
		t.Fatalf("应命中 2 个实体, got %d", len(entries))
	}
	if entries["tv:201"].Title != "令和的斑小姐" || entries["movie:101"].Title != "阿凡达" {
		t.Fatalf("实体内容错误: %+v", entries)
	}

	movies := []map[string]any{
		{"drive_folder_id": "m1", "media_type": "movie", "tmdb_id": 101, "title": "阿凡达", "year": "2009"},
	}
	tv := []map[string]any{
		{"drive_folder_id": "t1", "media_type": "tv", "tmdb_id": 201, "title": "令和的斑小姐", "year": "2026"},
		{"drive_folder_id": "t2", "media_type": "tv", "tmdb_id": 201, "title": "令和妖神斑小姐", "year": "2026"},
	}
	s.SaveSnapshot(movies, tv)

	groups := s.ListFolderIDsGroupedByTmdb()
	if got := groups["tv:201"]; len(got) != 2 {
		t.Fatalf("tv:201 应聚合 2 个目录: %v", got)
	}
	if len(groups["movie:101"]) != 1 {
		t.Fatalf("movie:101 应有 1 个目录: %v", groups)
	}
}

func TestMediaParentIDPersistence(t *testing.T) {
	s := newTestStore(t)
	// 带 drive_parent_id 的载荷：写入后可经 ListClassifiableItems 读回
	s.SaveIncremental([]map[string]any{
		{"drive_folder_id": "f1", "drive_parent_id": "cat-anime", "media_type": "tv", "tmdb_id": 1, "title": "A"},
	}, nil)
	refs := s.ListClassifiableItems()
	if len(refs) != 1 || refs[0].ParentID != "cat-anime" {
		t.Fatalf("parent_id 应为 cat-anime: %+v", refs)
	}
	// 移动后更新
	s.UpdateMediaParentID("f1", "cat-normal")
	if refs = s.ListClassifiableItems(); refs[0].ParentID != "cat-normal" {
		t.Fatalf("更新后 parent_id 应为 cat-normal: %+v", refs)
	}
	// 不带父信息的载荷（如旧数据回放）不应清掉已有值
	s.SaveIncremental([]map[string]any{
		{"drive_folder_id": "f1", "media_type": "tv", "tmdb_id": 1, "title": "A"},
	}, nil)
	if refs = s.ListClassifiableItems(); refs[0].ParentID != "cat-normal" {
		t.Fatalf("空父信息载荷不应清掉已有 parent_id: %+v", refs)
	}
}

func TestTitlesMatchContainmentGuard(t *testing.T) {
	// 衍生条目长后缀：不得因包含关系绑到正篇的库记录
	if titlematch.TitlesMatch("空之境界 未来福音 extra chorus", []string{"空之境界 未来福音"}) {
		t.Fatal("长后缀衍生条目不应匹配正篇")
	}
	// 数字缀（年份/序号）仍匹配
	if !titlematch.TitlesMatch("学生会也有洞 2026", []string{"学生会也有洞！"}) {
		t.Fatal("数字缀应匹配")
	}
	if !titlematch.TitlesMatch("斗罗大陆", []string{"斗罗大陆2"}) {
		t.Fatal("序号后缀应匹配")
	}
	// 短缀词仍匹配
	if !titlematch.TitlesMatch("斑小姐", []string{"令和的斑小姐"}) {
		t.Fatal("短缀词应匹配")
	}
}

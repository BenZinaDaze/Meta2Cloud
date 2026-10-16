package store

import (
	"path/filepath"
	"testing"
)

// newTitlePinTestStore 在同一 library.db 上打开实体库与入库历史库。
func newTitlePinTestStore(t *testing.T) (*LibraryStore, *IngestStore) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.db")
	ls, err := OpenLibraryStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ls.Close() })
	is, err := OpenIngestStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { is.Close() })
	return ls, is
}

func TestResolveTitlePinFallback(t *testing.T) {
	ls, _ := newTitlePinTestStore(t)

	// 无钉扎、无入库历史 → 用 fallback 落库
	title, year, err := ls.ResolveTitlePin("tv", 100, "初始译名", "2026")
	if err != nil || title != "初始译名" || year != "2026" {
		t.Fatalf("resolve = %q, %q, %v", title, year, err)
	}
	pin, ok := ls.GetTitlePin("tv", 100)
	if !ok || pin.PinnedTitle != "初始译名" || pin.Source != "tmdb" {
		t.Fatalf("pin = %+v, ok=%v", pin, ok)
	}

	// 已有钉扎 → 永不被覆盖
	title, _, err = ls.ResolveTitlePin("tv", 100, "新译名", "2026")
	if err != nil || title != "初始译名" {
		t.Fatalf("已有钉扎应沿用初始名，got %q, %v", title, err)
	}

	// fallback 为空且无任何历史 → 不落库
	title, _, err = ls.ResolveTitlePin("tv", 200, "", "")
	if err != nil || title != "" {
		t.Fatalf("应返回空且不报错，got %q, %v", title, err)
	}
	if _, ok := ls.GetTitlePin("tv", 200); ok {
		t.Fatal("不应落库")
	}
}

func TestResolveTitlePinFromIngestHistory(t *testing.T) {
	ls, is := newTitlePinTestStore(t)

	// 两条入库记录：最早一条才是初始整理名
	if _, err := is.RecordIngest(IngestRecord{
		MediaType: "tv", TmdbID: 300, Title: "旧译名", Year: "2025",
		Status: "success", IngestedAt: "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := is.RecordIngest(IngestRecord{
		MediaType: "tv", TmdbID: 300, Title: "新译名", Year: "2025",
		Status: "success", IngestedAt: "2025-06-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	title, year, err := ls.ResolveTitlePin("tv", 300, "实时译名", "2025")
	if err != nil || title != "旧译名" || year != "2025" {
		t.Fatalf("应取最早入库记录的标题，got %q/%q, %v", title, year, err)
	}
	pin, _ := ls.GetTitlePin("tv", 300)
	if pin.Source != "ingest_history" {
		t.Fatalf("source = %q", pin.Source)
	}

	// 失败状态的入库记录不参与
	if _, err := is.RecordIngest(IngestRecord{
		MediaType: "tv", TmdbID: 301, Title: "失败记录", Status: "failed",
		IngestedAt: "2025-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	title, _, err = ls.ResolveTitlePin("tv", 301, "兜底译名", "")
	if err != nil || title != "兜底译名" {
		t.Fatalf("失败记录不应参与，got %q, %v", title, err)
	}
}

func TestAdoptTitlePin(t *testing.T) {
	ls, _ := newTitlePinTestStore(t)

	if _, _, err := ls.ResolveTitlePin("movie", 400, "旧译名", "2010"); err != nil {
		t.Fatal(err)
	}
	if err := ls.AdoptTitlePin("movie", 400, "新译名", "2011"); err != nil {
		t.Fatal(err)
	}
	pin, ok := ls.GetTitlePin("movie", 400)
	if !ok || pin.PinnedTitle != "新译名" || pin.PinnedYear != "2011" || pin.Source != "manual" {
		t.Fatalf("pin = %+v, ok=%v", pin, ok)
	}
	// 后续 Resolve 不再覆盖手动钉扎
	title, _, _ := ls.ResolveTitlePin("movie", 400, "另一译名", "")
	if title != "新译名" {
		t.Fatalf("手动钉扎应稳定，got %q", title)
	}
	// 无效参数
	if err := ls.AdoptTitlePin("movie", 0, "x", ""); err == nil {
		t.Fatal("tmdb_id=0 应报错")
	}
}

func TestListDuplicateFolderGroups(t *testing.T) {
	ls, _ := newTitlePinTestStore(t)

	mk := func(folderID string, tmdbID int64) {
		ls.upsertLibraryItem(map[string]any{
			"drive_folder_id": folderID, "media_type": "tv", "tmdb_id": tmdbID,
			"title": "剧", "year": "2025",
		}, utcNow(), 0)
	}
	// 同一媒体两个目录 → 一组；单目录媒体不出现
	mk("folderA", 500)
	mk("folderB", 500)
	mk("folderC", 600)

	groups, err := ls.ListDuplicateFolderGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	g := groups[0]
	if g.MediaType != "tv" || g.TmdbID != 500 {
		t.Fatalf("group = %+v", g)
	}
	if len(g.FolderIDs) != 2 {
		t.Fatalf("folder_ids = %v", g.FolderIDs)
	}
}

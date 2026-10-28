// 整理流程注入入库历史存储的回归测试。
//
// 背景：doRunPipeline 构造 pipeline 时从未传 IngestStore，
// recordIngest 在 store 为 nil 时静默返回——RSS 下载完成后
// 转移+改名成功，入库历史却始终没有登记。
package webui

import (
	"net/http"
	"path/filepath"
	"testing"

	"meta2cloud/internal/pipeline"
	"meta2cloud/internal/webui/store"
)

// TestIsLoopbackRequest 环回地址判定（webhook 未配置密钥时的兜底限制）。
func TestIsLoopbackRequest(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "[::1]:9000", "localhost:1"} {
		if !isLoopbackRequest(&http.Request{RemoteAddr: addr}) {
			t.Errorf("isLoopbackRequest(%q) = false, want true", addr)
		}
	}
	for _, addr := range []string{"192.168.1.5:80", "10.0.0.2:1234", "8.8.8.8:53"} {
		if isLoopbackRequest(&http.Request{RemoteAddr: addr}) {
			t.Errorf("isLoopbackRequest(%q) = true, want false", addr)
		}
	}
}

// TestIngestStoreAdapterRecords 验证适配器把 pipeline.IngestRecord
// 正确写入 SQLite，且字段（含季/集、文件夹 ID）不丢。
func TestIngestStoreAdapterRecords(t *testing.T) {
	is, err := store.OpenIngestStore(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer is.Close()

	season, episode := 1, 7
	adapter := ingestStoreAdapter{is}
	err = adapter.RecordIngest(pipeline.IngestRecord{
		MediaType:     "tv",
		TmdbID:        12345,
		Title:         "葬送的芙莉莲",
		OriginalTitle: "Sousou no Frieren",
		Year:          "2023",
		Season:        &season,
		Episode:       &episode,
		EpisodeTitle:  "魔法使葬送的旅途",
		PosterPath:    "/poster.jpg",
		DriveFolderID: "folder-1",
		OriginalName:  "[SubGroup] Sousou no Frieren - 07 [1080p].mkv",
		Status:        "success",
	})
	if err != nil {
		t.Fatal(err)
	}

	result := is.Query(store.IngestQuery{})
	if len(result.Items) != 1 {
		t.Fatalf("期望 1 条入库记录，实际 %d", len(result.Items))
	}
	item := result.Items[0]
	for key, want := range map[string]any{
		"media_type": "tv", "tmdb_id": int64(12345), "title": "葬送的芙莉莲",
		"original_title": "Sousou no Frieren", "year": "2023",
		"season": int64(1), "episode": int64(7), "episode_title": "魔法使葬送的旅途",
		"poster_path": "/poster.jpg", "drive_folder_id": "folder-1",
		"status": "success", "original_name": "[SubGroup] Sousou no Frieren - 07 [1080p].mkv",
	} {
		if got := item[key]; got != want {
			t.Errorf("%s = %v，期望 %v", key, got, want)
		}
	}
}

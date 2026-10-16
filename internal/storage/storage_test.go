package storage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/u115"
)

func int64Ptr(v int64) *int64 { return &v }

func strPtrHelper(s string) *string { return &s }

// ── 类型转换（对应 Python TestPan115Provider） ─────────────

func TestToCloudFileVideo(t *testing.T) {
	pf := u115.File{
		ID:       "9876543",
		Name:     "movie.mkv",
		Category: "1",
		Size:     int64Ptr(1024 * 1024 * 700),
		PickCode: strPtrHelper("xyz789"),
		SHA1:     strPtrHelper("abcdef"),
		ParentID: strPtrHelper("folder123"),
	}
	cf := ToCloudFile(pf)

	if cf.ID != "9876543" || cf.Name != "movie.mkv" {
		t.Errorf("基础字段错误: %+v", cf)
	}
	if cf.FileType != cloudfile.TypeFile {
		t.Error("应为文件类型")
	}
	if !cf.IsVideo() {
		t.Error("按扩展名应识别为视频")
	}
	if cf.Size == nil || *cf.Size != 1024*1024*700 {
		t.Error("size 转换错误")
	}
	if cf.ParentID != "folder123" || len(cf.Parents) != 1 || cf.Parents[0] != "folder123" {
		t.Errorf("parent 转换错误: %q %v", cf.ParentID, cf.Parents)
	}
	if cf.MimeType != "" {
		t.Error("115 不提供 MIME，应为空")
	}
	if cf.Extra["pick_code"] != "xyz789" || cf.Extra["sha1"] != "abcdef" {
		t.Errorf("extra 转换错误: %v", cf.Extra)
	}
}

func TestToCloudFileFolder(t *testing.T) {
	cf := ToCloudFile(u115.File{ID: "111222", Name: "剧集目录", Category: "0"})
	if !cf.IsFolder() || cf.IsVideo() {
		t.Error("category=0 应为文件夹且不是视频")
	}
}

// ── 递归遍历（对应 Python test_list_all_recursive_uses_full_listing） ──

func TestWalkAllOrderAndFullListing(t *testing.T) {
	// mock 115 服务端：根目录返回 [folder1, file1]，folder1 返回 [file2]
	// 断言完整分页（offset 翻页）与 yield 顺序 folder1 → file2 → file1
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cid := r.URL.Query().Get("cid")
		offset := r.URL.Query().Get("offset")
		var data []map[string]any
		switch {
		case cid == "0" && offset == "0":
			data = []map[string]any{
				{"fid": "folder1", "fn": "Season 01", "fc": "0"},
				{"fid": "file1", "fn": "movie.mkv", "fc": "1"},
			}
		case cid == "folder1" && offset == "0":
			data = []map[string]any{
				{"fid": "file2", "fn": "episode.mkv", "fc": "1"},
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": data})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := u115.NewClient("test",
		u115.WithToken(u115.Token{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, RefreshTime: time.Now().Unix()}),
		u115.WithQPS(1000),
	)
	client.SetTestBaseURL(server.URL)

	provider := NewPan115ProviderWithClient(client)
	var order []string
	err := cloudfile.WalkAll(provider, "root", 10, func(f cloudfile.CloudFile) error {
		order = append(order, f.ID)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"folder1", "file2", "file1"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %s, want %s", i, order[i], want[i])
		}
	}
}

// ── 注册表 ──────────────────────────────────────────────────

func TestRegistry(t *testing.T) {
	providers := ListProviders()
	found := false
	for _, p := range providers {
		if p == "pan115" {
			found = true
		}
	}
	if !found {
		t.Error("内置应包含 pan115")
	}

	if _, err := GetProvider("google_drive", &config.Config{}); err == nil {
		t.Error("google_drive 已移除，应返回未知 Provider 错误")
	}
	if _, err := GetProvider("nonexistent_cloud", &config.Config{}); err == nil {
		t.Error("未知 Provider 应报错")
	}
}

// ── 上传分片大小 ────────────────────────────────────────────

func TestDeterminePartSize(t *testing.T) {
	if got := determinePartSize(1024); got != ossPartPreferredSize {
		t.Errorf("小文件应使用首选分片 %d, got %d", ossPartPreferredSize, got)
	}
	// 超过 10000 片时向上调整
	huge := int64(ossPartPreferredSize)*10000 + 1
	got := determinePartSize(huge)
	if int64(got)*10000 < huge {
		t.Errorf("大文件分片数量应 ≤ 10000, partSize=%d", got)
	}
}

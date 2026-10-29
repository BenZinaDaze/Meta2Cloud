// librarydelete_test.go —— 媒体联动删除端到端测试：
// 全流程成功 / 条目不存在 404 / 网盘删除失败本地不动 / 目录已不存在幂等 / 联动取消订阅 / 删除预览。
package webui

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"meta2cloud/internal/webui/store"
)

// libraryDeleteEnv 联动删除测试环境。
type libraryDeleteEnv struct {
	rt        *Runtime
	ts        *httptest.Server
	token     string
	fp        *fakeWebProvider
	showID    string   // 网盘剧集目录 ID（库条目主键）
	fileIDs   []string // 目录下的网盘文件 ID
	strmLocal []string // 已写入的本地 .strm 路径
}

// newLibraryDeleteServer 构造测试环境：假网盘 + 库条目 + STRM 记录/本地文件 + 活跃订阅。
func newLibraryDeleteServer(t *testing.T) *libraryDeleteEnv {
	t.Helper()
	rootDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rootDir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootDir, "config", "config.yaml"),
		[]byte("webui:\n  username: admin\n  password: secret123\nu115:\n  root_folder_id: \"root\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := InitRuntime(rootDir)
	rt.ResetJWTSecretCache()

	// 假网盘：root/剧集/删除之剧 (2024)/Season 01/两集视频
	fp := newFakeWebProvider()
	root := fp.addFolder("root", "0")
	tvCat := fp.addFolder("剧集", root.ID)
	show := fp.addFolder("删除之剧 (2024)", tvCat.ID)
	season := fp.addFolder("Season 01", show.ID)
	f1 := fp.addFile("删除之剧 - S01E01.mkv", season.ID)
	f2 := fp.addFile("删除之剧 - S01E02.mkv", season.ID)
	rt.provider = fp.provider()

	// 库条目
	rt.GetLibraryStore().SaveIncremental([]map[string]any{
		{"drive_folder_id": show.ID, "media_type": "tv", "tmdb_id": int64(312507), "title": "删除之剧", "year": "2024"},
	}, nil)

	// STRM 记录 + 本地 .strm 文件
	env := &libraryDeleteEnv{rt: rt, fp: fp, showID: show.ID, fileIDs: []string{f1.ID, f2.ID}}
	strmStore := rt.GetStrmStore()
	for i, fileID := range env.fileIDs {
		local := filepath.Join(rootDir, "strm", "删除之剧", fmt.Sprintf("S01E%02d.strm", i+1))
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(local, []byte("http://strm/test"), 0o644); err != nil {
			t.Fatal(err)
		}
		size := int64(100 + i)
		strmStore.UpsertStrmFile(store.StrmFile{
			PickCode:  fmt.Sprintf("pc%d", i+1),
			FileID:    fileID,
			PanPath:   fmt.Sprintf("/删除之剧 - S01E%02d.mkv", i+1),
			LocalPath: local,
			FileName:  fmt.Sprintf("删除之剧 - S01E%02d.mkv", i+1),
			Size:      &size,
		})
		env.strmLocal = append(env.strmLocal, local)
	}

	// 活跃订阅（media_type+tmdb_id 与库条目一致）
	if _, err := rt.GetRSSStore().CreateSubscription(map[string]any{
		"name": "删除之剧订阅", "media_title": "删除之剧", "media_type": "tv", "tmdb_id": int64(312507),
		"site": "mikan", "rss_url": "https://mikanani.me/RSS/Bangumi?bangumiId=3333&subgroupid=583",
		"subgroup_name": "TestGroup", "season_number": 1, "start_episode": 1,
		"keyword_all": "[]", "push_target": "u115",
	}); err != nil {
		t.Fatal(err)
	}

	server := NewServer(rt)
	t.Cleanup(server.Close)
	env.ts = httptest.NewServer(server.Handler())
	t.Cleanup(env.ts.Close)
	_, out := login(t, env.ts, "admin", "secret123")
	env.token = out["token"].(string)
	return env
}

// assertLocalUntouched 断言本地数据一概未动（网盘目录/库条目/订阅/STRM/删除日志）。
func assertLocalUntouched(t *testing.T, env *libraryDeleteEnv) {
	t.Helper()
	if _, err := env.fp.GetFile(env.showID); err != nil {
		t.Errorf("网盘目录不应被删除: %v", err)
	}
	if env.rt.GetLibraryStore().GetLibraryItemByFolderID(env.showID) == nil {
		t.Error("库条目不应被删除")
	}
	if subs := env.rt.GetRSSStore().ListSubscriptions(); len(subs) != 1 {
		t.Errorf("订阅不应被取消, got %d", len(subs))
	}
	if n := env.rt.GetStrmStore().CountStrmFiles(); n != 2 {
		t.Errorf("STRM 记录不应被清理, got %d", n)
	}
	for _, p := range env.strmLocal {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("本地 STRM 文件不应被删除 %s: %v", p, err)
		}
	}
	if items := env.rt.GetIngestStore().Query(store.IngestQuery{Status: "deleted"}).Items; len(items) != 0 {
		t.Errorf("不应写入删除日志, got %v", items)
	}
}

// 全流程成功：网盘目录删除 + STRM 清理 + 库条目删除 + 删除日志 + 联动取消订阅。
func TestLibraryMediaDeleteFullFlow(t *testing.T) {
	env := newLibraryDeleteServer(t)

	code, out := authedRequest(t, env.ts, env.token, "DELETE",
		"/api/library/media/"+env.showID+"?cancel_subscription=true", nil)
	if code != 200 {
		t.Fatalf("delete code = %d: %v", code, out)
	}
	if out["removed_strm"] != float64(2) || out["cancelled_subscriptions"] != float64(1) {
		t.Errorf("delete 响应 = %v", out)
	}

	// 网盘目录已删
	if _, err := env.fp.GetFile(env.showID); err == nil {
		t.Error("网盘目录应已删除")
	}
	// STRM 记录与本地文件已清
	if n := env.rt.GetStrmStore().CountStrmFiles(); n != 0 {
		t.Errorf("STRM 记录应清空, got %d", n)
	}
	for _, p := range env.strmLocal {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("本地 STRM 文件应已删除 %s", p)
		}
	}
	// 库条目已删
	if env.rt.GetLibraryStore().GetLibraryItemByFolderID(env.showID) != nil {
		t.Error("库条目应已删除")
	}
	// 订阅已取消（删行而非停用）
	if subs := env.rt.GetRSSStore().ListSubscriptions(); len(subs) != 0 {
		t.Errorf("订阅应已取消, got %d", len(subs))
	}
	// 删除日志：status='deleted'，title/tmdb_id/drive_folder_id 照抄
	items := env.rt.GetIngestStore().Query(store.IngestQuery{Status: "deleted"}).Items
	if len(items) != 1 {
		t.Fatalf("删除日志应 1 条, got %d", len(items))
	}
	item := items[0]
	if item.Title != "删除之剧" || item.DriveFolderID != env.showID || item.TmdbID != 312507 {
		t.Errorf("删除日志字段 = %+v", item)
	}
}

// 条目不存在 → 404。
func TestLibraryMediaDeleteNotFound(t *testing.T) {
	env := newLibraryDeleteServer(t)

	code, _ := authedRequest(t, env.ts, env.token, "DELETE", "/api/library/media/no-such-folder", nil)
	if code != 404 {
		t.Errorf("delete code = %d, want 404", code)
	}
	code, _ = authedRequest(t, env.ts, env.token, "GET", "/api/library/media/no-such-folder", nil)
	if code != 404 {
		t.Errorf("preview code = %d, want 404", code)
	}
}

// 网盘删除失败 → 即刻中止，本地一概未动。
func TestLibraryMediaDeleteCloudFailureAborts(t *testing.T) {
	env := newLibraryDeleteServer(t)
	env.fp.failDelete(env.showID)

	code, out := authedRequest(t, env.ts, env.token, "DELETE",
		"/api/library/media/"+env.showID+"?cancel_subscription=true", nil)
	if code != 500 {
		t.Fatalf("delete code = %d, want 500: %v", code, out)
	}
	assertLocalUntouched(t, env)
}

// 目录已被外部删除 → 幂等返回 200，删除库条目并记删除日志。
// 真实网盘上已删目录无法再遍历（ListAllFiles 报「未找到」），fileIDs 为空集，
// 因此本次请求内无法按 FileID 清理 STRM 记录与本地文件——这部分留待
// after-delete 的 STRM 对账兜底清理，测试如实断言该语义。
func TestLibraryMediaDeleteIdempotentWhenFolderGone(t *testing.T) {
	env := newLibraryDeleteServer(t)
	// 目录已在别处被删（库条目与 STRM 记录/本地文件仍在）
	if err := env.fp.DeleteFile(env.showID); err != nil {
		t.Fatal(err)
	}

	code, out := authedRequest(t, env.ts, env.token, "DELETE", "/api/library/media/"+env.showID, nil)
	if code != 200 {
		t.Fatalf("delete code = %d: %v", code, out)
	}
	if out["removed_strm"] != float64(0) {
		t.Errorf("目录已不可遍历，本次请求内不应清理 STRM, got %v", out)
	}
	// STRM 记录与本地文件本次请求内保留（目录已无法遍历），由 after-delete 对账清理
	if n := env.rt.GetStrmStore().CountStrmFiles(); n != 2 {
		t.Errorf("STRM 记录应保留待对账, got %d", n)
	}
	for _, p := range env.strmLocal {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("本地 STRM 文件应保留待对账 %s: %v", p, err)
		}
	}
	if env.rt.GetLibraryStore().GetLibraryItemByFolderID(env.showID) != nil {
		t.Error("库条目应已删除")
	}
	if items := env.rt.GetIngestStore().Query(store.IngestQuery{Status: "deleted"}).Items; len(items) != 1 {
		t.Errorf("删除日志应 1 条, got %d", len(items))
	}
}

// 不带 cancel_subscription 时订阅保留；再次删除因条目已不存在返回 404。
func TestLibraryMediaDeleteKeepsSubscriptionWithoutFlag(t *testing.T) {
	env := newLibraryDeleteServer(t)

	code, out := authedRequest(t, env.ts, env.token, "DELETE", "/api/library/media/"+env.showID, nil)
	if code != 200 {
		t.Fatalf("delete code = %d: %v", code, out)
	}
	if out["cancelled_subscriptions"] != float64(0) {
		t.Errorf("未带标志不应取消订阅, got %v", out)
	}
	if subs := env.rt.GetRSSStore().ListSubscriptions(); len(subs) != 1 {
		t.Errorf("订阅应保留, got %d", len(subs))
	}

	// 条目已删，再次删除 → 404
	code, _ = authedRequest(t, env.ts, env.token, "DELETE", "/api/library/media/"+env.showID, nil)
	if code != 404 {
		t.Errorf("re-delete code = %d, want 404", code)
	}
}

// 删除预览：标题/类型/tmdb_id/受影响 STRM 数/匹配订阅。
func TestLibraryMediaDeletePreview(t *testing.T) {
	env := newLibraryDeleteServer(t)

	code, out := authedRequest(t, env.ts, env.token, "GET", "/api/library/media/"+env.showID, nil)
	if code != 200 {
		t.Fatalf("preview code = %d: %v", code, out)
	}
	if out["title"] != "删除之剧" || out["media_type"] != "tv" || out["tmdb_id"] != float64(312507) {
		t.Errorf("preview 基础字段 = %v", out)
	}
	if out["strm_count"] != float64(2) {
		t.Errorf("strm_count = %v, want 2", out["strm_count"])
	}
	subs, _ := out["subscriptions"].([]any)
	if len(subs) != 1 {
		t.Fatalf("subscriptions = %v, want 1 条", out["subscriptions"])
	}
	sub := subs[0].(map[string]any)
	if sub["name"] != "删除之剧订阅" || sub["tmdb_id"] != float64(312507) {
		t.Errorf("订阅摘要 = %v", sub)
	}
}

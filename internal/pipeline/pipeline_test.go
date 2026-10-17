package pipeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/mediaparser"
	"meta2cloud/internal/tmdb"
)

// ── 内存版 Fake Provider ────────────────────────────────────

type fakeProvider struct {
	mu       sync.Mutex
	children map[string][]cloudfile.CloudFile // parentID → children
	byID     map[string]cloudfile.CloudFile
	nextID   int
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{
		children: map[string][]cloudfile.CloudFile{},
		byID:     map[string]cloudfile.CloudFile{},
	}
}

func (f *fakeProvider) id() string {
	f.nextID++
	return fmt.Sprintf("f%d", f.nextID)
}

func (f *fakeProvider) ProviderName() string { return "fake" }

func (f *fakeProvider) put(cf cloudfile.CloudFile) {
	f.children[cf.ParentID] = append(f.children[cf.ParentID], cf)
	f.byID[cf.ID] = cf
}

func (f *fakeProvider) AddFolder(name, parentID string) cloudfile.CloudFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := cloudfile.CloudFile{ID: f.id(), Name: name, FileType: cloudfile.TypeFolder, ParentID: parentID, Parents: []string{parentID}}
	f.put(cf)
	return cf
}

func (f *fakeProvider) AddFile(name, parentID string) cloudfile.CloudFile {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := cloudfile.CloudFile{ID: f.id(), Name: name, FileType: cloudfile.TypeFile, ParentID: parentID, Parents: []string{parentID}}
	f.put(cf)
	return cf
}

func (f *fakeProvider) ListFiles(folderID string, pageSize int) ([]cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []cloudfile.CloudFile{}
	out = append(out, f.children[folderID]...)
	return out, nil
}

func (f *fakeProvider) ListAllFiles(folderID string) ([]cloudfile.CloudFile, error) {
	return f.ListFiles(folderID, 1000)
}

func (f *fakeProvider) GetFile(fileID string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if cf, ok := f.byID[fileID]; ok {
		return cf, nil
	}
	return cloudfile.CloudFile{}, fmt.Errorf("未找到文件：%s", fileID)
}

func (f *fakeProvider) ReadText(file cloudfile.CloudFile) (string, error) {
	return "", nil
}

func (f *fakeProvider) FindFile(name string, folderID string) (*cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cf := range f.children[folderID] {
		if cf.Name == name {
			out := cf
			return &out, nil
		}
	}
	return nil, nil
}

func (f *fakeProvider) RenameFile(fileID, newName string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf, ok := f.byID[fileID]
	if !ok {
		return cloudfile.CloudFile{}, fmt.Errorf("not found")
	}
	cf.Name = newName
	f.byID[fileID] = cf
	for parent, list := range f.children {
		for i := range list {
			if list[i].ID == fileID {
				list[i].Name = newName
				f.children[parent] = list
			}
		}
	}
	return cf, nil
}

func (f *fakeProvider) MoveFile(fileID, newFolderID, newName string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf, ok := f.byID[fileID]
	if !ok {
		return cloudfile.CloudFile{}, fmt.Errorf("not found")
	}
	// 从旧父目录移除
	oldList := f.children[cf.ParentID]
	var kept []cloudfile.CloudFile
	for _, item := range oldList {
		if item.ID != fileID {
			kept = append(kept, item)
		}
	}
	f.children[cf.ParentID] = kept
	// 加入新目录
	cf.ParentID = newFolderID
	cf.Parents = []string{newFolderID}
	if newName != "" {
		cf.Name = newName
	}
	f.byID[fileID] = cf
	f.children[newFolderID] = append(f.children[newFolderID], cf)
	return cf, nil
}

func (f *fakeProvider) UploadText(content, name, parentID, _ string, _ bool) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := cloudfile.CloudFile{ID: f.id(), Name: name, FileType: cloudfile.TypeFile, ParentID: parentID, Parents: []string{parentID}}
	f.put(cf)
	return cf, nil
}

func (f *fakeProvider) UploadBytes(content []byte, name, parentID, _ string, _ bool) (cloudfile.CloudFile, error) {
	return f.UploadText(string(content), name, parentID, "", true)
}

func (f *fakeProvider) TrashFile(fileID string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf, ok := f.byID[fileID]
	if !ok {
		return cloudfile.CloudFile{}, fmt.Errorf("not found")
	}
	cf.Trashed = true
	f.byID[fileID] = cf
	return cf, nil
}

func (f *fakeProvider) DeleteFile(fileID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byID, fileID)
	return nil
}

func (f *fakeProvider) CreateFolder(name, parentID string) (cloudfile.CloudFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := cloudfile.CloudFile{ID: f.id(), Name: name, FileType: cloudfile.TypeFolder, ParentID: parentID, Parents: []string{parentID}}
	f.put(cf)
	return cf, nil
}

// ── TMDB mock ───────────────────────────────────────────────

func newTmdbServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/tv"):
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 1396.0, "name": "Breaking Bad", "first_air_date": "2008-01-20"},
			}})
		case strings.HasPrefix(r.URL.Path, "/search/movie"):
			json.NewEncoder(w).Encode(map[string]any{"results": []any{
				map[string]any{"id": 27205.0, "title": "Inception", "release_date": "2010-07-16"},
			}})
		case strings.Contains(r.URL.Path, "/season/"):
			if strings.Contains(r.URL.Path, "/episode/") {
				json.NewEncoder(w).Encode(map[string]any{
					"id": 62085.0, "name": "...And the Bag's in the River",
					"air_date": "2008-02-10", "season_number": 1.0, "episode_number": 3.0,
					"vote_average": 8.2, "still_path": "/still.jpg",
				})
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id": 3572.0, "name": "Season 1", "air_date": "2008-01-20",
				"poster_path": "/season1.jpg",
			})
		case strings.HasPrefix(r.URL.Path, "/tv/"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": 1396.0, "name": "绝命毒师", "original_name": "Breaking Bad",
				"first_air_date": "2008-01-20", "overview": "化学老师",
				"poster_path": "/bb.jpg", "backdrop_path": "/bbd.jpg",
				"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
				"alternative_titles": map[string]any{"results": []any{}},
				"translations":       map[string]any{"translations": []any{}},
				"external_ids":       map[string]any{"tvdb_id": 81189.0},
				"genres":             []any{},
			})
		case strings.HasPrefix(r.URL.Path, "/movie/"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": 27205.0, "title": "盗梦空间", "original_title": "Inception",
				"release_date": "2010-07-16", "overview": "梦境",
				"poster_path": "/in.jpg", "backdrop_path": "/ind.jpg", "runtime": 148.0,
				"credits":            map[string]any{"crew": []any{}, "cast": []any{}},
				"alternative_titles": map[string]any{"titles": []any{}},
				"translations":       map[string]any{"translations": []any{}},
				"external_ids":       map[string]any{"imdb_id": "tt1375666"},
				"genres":             []any{},
			})
		default:
			w.WriteHeader(404)
		}
	}))
}

func newTestConfig() *config.Config {
	cfg := config.Default()
	cfg.U115.DownloadFolderID = "scan"
	cfg.U115.RootFolderID = "root"
	cfg.Tmdb.APIKey = "test"
	return cfg
}

// ── 字幕匹配 ────────────────────────────────────────────────

func TestParseSubtitleTags(t *testing.T) {
	m := &SubtitleMatcher{}
	cases := []struct {
		in   string
		name string
		tags []string
	}{
		{"Movie.2024.chs.forced", "Movie.2024", []string{"zh-CN", "forced"}},
		{"Movie.2024[chs]", "Movie.2024", []string{"zh-CN"}},
		{"The.English.Game.S01E01.eng", "The.English.Game.S01E01", []string{"en"}},
		{"剧名.简体&繁体", "剧名.简体&繁体", nil},
	}
	for _, c := range cases {
		name, tags := m.ParseSubtitleTags(c.in)
		if name != c.name {
			t.Errorf("ParseSubtitleTags(%q) name = %q, want %q", c.in, name, c.name)
		}
		if len(tags) != len(c.tags) {
			t.Errorf("ParseSubtitleTags(%q) tags = %v, want %v", c.in, tags, c.tags)
			continue
		}
		for i := range tags {
			if tags[i] != c.tags[i] {
				t.Errorf("ParseSubtitleTags(%q) tags[%d] = %s, want %s", c.in, i, tags[i], c.tags[i])
			}
		}
	}
}

func TestMatchSubtitleToVideo(t *testing.T) {
	m := &SubtitleMatcher{}
	video := cloudfile.CloudFile{ID: "v1", Name: "Show.S01E01.1080p.WEB-DL.mkv", FileType: cloudfile.TypeFile}
	sub := cloudfile.CloudFile{ID: "s1", Name: "Show.S01E01.chs.srt", FileType: cloudfile.TypeFile}
	match := m.MatchSubtitleToVideo(sub, video)
	if match == nil {
		t.Fatal("应匹配成功")
	}
	if match.TargetName != "Show.S01E01.1080p.WEB-DL.zh-CN.srt" {
		t.Errorf("target = %q", match.TargetName)
	}
	// 前缀匹配：0.7 + 0.3 * (13/25)，与 Python 版一致
	if match.Confidence < 0.83 || match.Confidence > 0.85 {
		t.Errorf("confidence = %v, want ~0.8375", match.Confidence)
	}
	// 精确匹配置信度 1.0（视频名不含 - _ 时，标签剥离后完全一致）
	exactVideo := cloudfile.CloudFile{ID: "v2", Name: "Show.S01E01.1080p.mkv", FileType: cloudfile.TypeFile}
	exactSub := cloudfile.CloudFile{ID: "s3", Name: "Show.S01E01.1080p.chs.srt", FileType: cloudfile.TypeFile}
	if match := m.MatchSubtitleToVideo(exactSub, exactVideo); match == nil || match.Confidence != 1.0 {
		t.Errorf("精确匹配应为 1.0: %+v", match)
	}
	// 季级字幕不应匹配单集视频
	seasonSub := cloudfile.CloudFile{ID: "s2", Name: "Show.chs.srt", FileType: cloudfile.TypeFile}
	if match := m.MatchSubtitleToVideo(seasonSub, video); match != nil {
		t.Errorf("季级字幕不应匹配单集视频: %+v", match)
	}
}

// ── Pipeline 端到端 ─────────────────────────────────────────

func TestPipelineTVFlow(t *testing.T) {
	server := newTmdbServer(t)
	defer server.Close()

	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")
	video := provider.AddFile("Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG.mkv", scan.ID)
	sub := provider.AddFile("Breaking.Bad.S01E03.chs.srt", scan.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID
	tmdbClient := tmdb.NewClient("key", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(server.URL)

	p := New(provider, cfg, Options{
		TmdbClient: tmdbClient,
		SkipImages: true,
	})
	summary := p.Run()

	if summary.OKCount != 1 || summary.FailedCount != 0 {
		t.Fatalf("summary = %+v", summary)
	}

	// 目标目录：root/绝命毒师 (2008)/Season 01/
	top, err := provider.FindFile("绝命毒师 (2008)", "root")
	if err != nil || top == nil {
		t.Fatalf("剧名文件夹未创建: %v", err)
	}
	season, err := provider.FindFile("Season 01", top.ID)
	if err != nil || season == nil {
		t.Fatalf("Season 文件夹未创建: %v", err)
	}
	moved, _ := provider.FindFile("绝命毒师 - S01E03.mkv", season.ID)
	if moved == nil {
		t.Fatal("视频未移动到目标文件夹（含标准改名）")
	}
	_ = video
	// NFO 同名
	nfoFile, _ := provider.FindFile("绝命毒师 - S01E03.nfo", season.ID)
	if nfoFile == nil {
		t.Fatal("单集 NFO 未上传")
	}
	tvshow, _ := provider.FindFile("tvshow.nfo", top.ID)
	if tvshow == nil {
		t.Fatal("tvshow.nfo 未上传到剧名文件夹")
	}
	seasonNfo, _ := provider.FindFile("season.nfo", season.ID)
	if seasonNfo == nil {
		t.Fatal("season.nfo 未上传到 Season 文件夹")
	}
	// 字幕跟随移动并改名
	movedSub, _ := provider.FindFile("绝命毒师 - S01E03.zh-CN.srt", season.ID)
	if movedSub == nil {
		t.Fatal("字幕未移动")
	}
	_ = sub
}

func TestPipelineMovieFlow(t *testing.T) {
	server := newTmdbServer(t)
	defer server.Close()

	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")
	provider.AddFile("Inception.2010.1080p.BluRay.x264.DTS-HD.Ma.mkv", scan.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID
	tmdbClient := tmdb.NewClient("key", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(server.URL)

	p := New(provider, cfg, Options{TmdbClient: tmdbClient, SkipImages: true})
	summary := p.Run()
	if summary.OKCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	movieFolder, _ := provider.FindFile("盗梦空间 (2010)", "root")
	if movieFolder == nil {
		t.Fatal("电影文件夹未创建")
	}
	moved, _ := provider.FindFile("盗梦空间 (2010).mkv", movieFolder.ID)
	if moved == nil {
		t.Fatal("电影未移动（标准命名 盗梦空间 (2010).mkv）")
	}
}

func TestPipelineDryRun(t *testing.T) {
	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")
	provider.AddFile("Inception.2010.1080p.BluRay.x264.mkv", scan.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID

	p := New(provider, cfg, Options{DryRun: true, SkipTmdb: true})
	summary := p.Run()
	if summary.OKCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	// dry-run 不应创建任何文件夹
	if existing, _ := provider.FindFile("Inception (2010)", "root"); existing != nil {
		t.Fatal("dry-run 不应创建文件夹")
	}
	if list, _ := provider.ListFiles(scan.ID, 100); len(list) != 1 {
		t.Fatal("dry-run 不应移动文件")
	}
}

func TestBuildCleanName(t *testing.T) {
	ep := 3
	if got := buildCleanName(map[string]any{"name": "绝命毒师"}, true, 1, &ep, ".mkv"); got != "绝命毒师 - S01E03.mkv" {
		t.Errorf("tv cleanName = %q", got)
	}
	if got := buildCleanName(map[string]any{"title": "盗梦空间", "release_date": "2010-07-16"}, false, 0, nil, ".mkv"); got != "盗梦空间 (2010).mkv" {
		t.Errorf("movie cleanName = %q", got)
	}
}

// ── 标题钉扎 ────────────────────────────────────────────────

// fakeTitlePinStore 内存版标题钉扎存储。
type fakeTitlePinStore struct {
	mu   sync.Mutex
	pins map[string][2]string
}

func newFakeTitlePinStore(pins map[string][2]string) *fakeTitlePinStore {
	return &fakeTitlePinStore{pins: pins}
}

func titlePinKey(mediaType string, tmdbID int64) string {
	return fmt.Sprintf("%s:%d", mediaType, tmdbID)
}

func (f *fakeTitlePinStore) GetTitlePin(mediaType string, tmdbID int64) (string, string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.pins[titlePinKey(mediaType, tmdbID)]
	if !ok {
		return "", "", false
	}
	return p[0], p[1], true
}

func (f *fakeTitlePinStore) ResolveTitlePin(mediaType string, tmdbID int64, fallbackTitle, fallbackYear string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := titlePinKey(mediaType, tmdbID)
	if p, ok := f.pins[k]; ok {
		return p[0], p[1], nil
	}
	if fallbackTitle == "" {
		return "", "", nil
	}
	f.pins[k] = [2]string{fallbackTitle, fallbackYear}
	return fallbackTitle, fallbackYear, nil
}

// fakeDuplicateSource 内存版重复目录分组来源。
type fakeDuplicateSource struct {
	groups []DuplicateFolderGroup
}

func (f *fakeDuplicateSource) DuplicateFolderGroups() []DuplicateFolderGroup {
	return f.groups
}

func TestPipelineTitlePinReuse(t *testing.T) {
	server := newTmdbServer(t)
	defer server.Close()

	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")
	provider.AddFile("Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG.mkv", scan.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID
	tmdbClient := tmdb.NewClient("key", "zh-CN", "", 5, nil)
	tmdbClient.SetTestBaseURL(server.URL)

	// TMDB 当前译名为「绝命毒师」，钉扎名「旧译名」→ 目录沿用钉扎名
	p := New(provider, cfg, Options{
		TmdbClient: tmdbClient,
		SkipImages: true,
		TitlePinStore: newFakeTitlePinStore(map[string][2]string{
			"tv:1396": {"旧译名", "2008"},
		}),
	})
	summary := p.Run()
	if summary.OKCount != 1 {
		t.Fatalf("summary = %+v", summary)
	}

	top, err := provider.FindFile("旧译名 (2008)", "root")
	if err != nil || top == nil {
		t.Fatalf("未使用钉扎目录名: %v", err)
	}
	season, _ := provider.FindFile("Season 01", top.ID)
	if season == nil {
		t.Fatal("Season 文件夹未创建")
	}
	if moved, _ := provider.FindFile("旧译名 - S01E03.mkv", season.ID); moved == nil {
		t.Fatal("文件名未使用钉扎名")
	}
	if other, _ := provider.FindFile("绝命毒师 (2008)", "root"); other != nil {
		t.Fatal("不应创建 TMDB 新译名目录")
	}
}

func TestPipelineMergeDuplicateFolders(t *testing.T) {
	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")

	// 媒体库根目录下同一剧集的两个目录（历史译名差异）
	oldFolder := provider.AddFolder("旧译名 (2025)", "root")
	oldSeason := provider.AddFolder("Season 01", oldFolder.ID)
	provider.AddFile("旧译名 - S01E01.mkv", oldSeason.ID)
	provider.AddFile("tvshow.nfo", oldFolder.ID)
	newFolder := provider.AddFolder("新译名 (2025)", "root")
	newSeason := provider.AddFolder("Season 01", newFolder.ID)
	provider.AddFile("新译名 - S01E02.mkv", newSeason.ID)
	provider.AddFile("tvshow.nfo", newFolder.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID
	p := New(provider, cfg, Options{
		SkipTmdb:      true,
		TitlePinStore: newFakeTitlePinStore(map[string][2]string{"tv:501": {"旧译名", "2025"}}),
		DuplicateFolders: &fakeDuplicateSource{groups: []DuplicateFolderGroup{{
			MediaType: "tv", TmdbID: 501, Folders: []DuplicateFolderInfo{{ID: oldFolder.ID}, {ID: newFolder.ID}},
		}}},
	})
	p.Run()

	// 重复目录文件已并入钉扎目录
	if f, _ := provider.FindFile("新译名 - S01E02.mkv", oldSeason.ID); f == nil {
		t.Fatal("重复目录文件未并入钉扎目录")
	}
	// 钉扎目录原文件仍在
	if f, _ := provider.FindFile("旧译名 - S01E01.mkv", oldSeason.ID); f == nil {
		t.Fatal("钉扎目录原文件丢失")
	}
	// 重复目录已回收
	cf, err := provider.GetFile(newFolder.ID)
	if err != nil || !cf.Trashed {
		t.Fatalf("重复目录未回收: %v", err)
	}
	// 钉扎目录本身未回收
	if cf, _ := provider.GetFile(oldFolder.ID); cf.Trashed {
		t.Fatal("钉扎目录不应被回收")
	}
}

func TestPipelineMergeSkippedWhenDisabled(t *testing.T) {
	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")
	oldFolder := provider.AddFolder("旧译名 (2025)", "root")
	newFolder := provider.AddFolder("新译名 (2025)", "root")
	provider.AddFile("新译名 - S01E02.mkv", newFolder.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID
	disabled := false
	cfg.Pipeline.MergeDuplicateFolders = &disabled

	p := New(provider, cfg, Options{
		SkipTmdb:      true,
		TitlePinStore: newFakeTitlePinStore(map[string][2]string{"tv:501": {"旧译名", "2025"}}),
		DuplicateFolders: &fakeDuplicateSource{groups: []DuplicateFolderGroup{{
			MediaType: "tv", TmdbID: 501, Folders: []DuplicateFolderInfo{{ID: oldFolder.ID}, {ID: newFolder.ID}},
		}}},
	})
	p.Run()

	if f, _ := provider.FindFile("新译名 - S01E02.mkv", newFolder.ID); f == nil {
		t.Fatal("开关关闭时不应移动文件")
	}
}

// 静态确认 MetaInfo 解析被 pipeline 正确使用
var _ = mediaparser.MetaInfo

func TestPipelineMergeDerivesPinFromOldestFolder(t *testing.T) {
	provider := newFakeProvider()
	scan := provider.AddFolder("扫描", "0")

	// 无钉扎记录、无入库历史：钉扎名应取组内最旧目录的实际名称
	oldFolder := provider.AddFolder("旧译名 (2025)", "root")
	oldSeason := provider.AddFolder("Season 01", oldFolder.ID)
	provider.AddFile("旧译名 - S01E01.mkv", oldSeason.ID)
	newFolder := provider.AddFolder("新译名 (2025)", "root")
	newSeason := provider.AddFolder("Season 01", newFolder.ID)
	provider.AddFile("新译名 - S01E02.mkv", newSeason.ID)

	cfg := newTestConfig()
	cfg.U115.DownloadFolderID = scan.ID
	pinStore := newFakeTitlePinStore(map[string][2]string{})
	p := New(provider, cfg, Options{
		SkipTmdb:      true,
		TitlePinStore: pinStore,
		DuplicateFolders: &fakeDuplicateSource{groups: []DuplicateFolderGroup{{
			MediaType: "tv", TmdbID: 502,
			Folders: []DuplicateFolderInfo{{ID: oldFolder.ID}, {ID: newFolder.ID}},
		}}},
	})
	p.Run()

	if f, _ := provider.FindFile("新译名 - S01E02.mkv", oldSeason.ID); f == nil {
		t.Fatal("文件未并入最旧目录")
	}
	if cf, _ := provider.GetFile(newFolder.ID); !cf.Trashed {
		t.Fatal("较新的重复目录未回收")
	}
	if title, year, ok := pinStore.GetTitlePin("tv", 502); !ok || title != "旧译名" || year != "2025" {
		t.Fatalf("钉扎未按最旧目录落库: %q %q %v", title, year, ok)
	}
}

func TestBuildNotifyCaption(t *testing.T) {
	cfg := newTestConfig()
	provider := newFakeProvider()
	p := New(provider, cfg, Options{
		SkipTmdb: true,
		OnVideoIngested: func(ProcessResult) error { return nil },
	})
	ep1, ep2 := 1, 2
	base := notifyItem{
		Title: "学生会也有洞", Year: "2026", IsTV: true,
		Season: 1, Rating: 7.7, Country: "日番 · 日本",
		Genres:      []string{"动画", "喜剧"},
		SeasonTotal: 12, Overview: "高一水之江梅有着极其高超的文学造诣。",
		StrmOK: true,
	}
	items := []notifyItem{
		func() notifyItem { it := base; it.Episode = &ep1; return it }(),
		func() notifyItem { it := base; it.Episode = &ep2; return it }(),
	}
	caption := p.buildNotifyCaption(base, items)

	for _, want := range []string{
		"✅ 入库完成 · 学生会也有洞 (2026)  S01E01-E02",
		"📺 S01E01-E02 · 本季共 12 集",
		"⭐ 7.7 · 日番 · 日本",
		"📦 入库 2 个文件 · STRM 已生成",
		"#学生会也有洞 #动画 #喜剧",
		"<blockquote expandable>高一水之江梅有着极其高超的文学造诣。</blockquote>",
	} {
		if !strings.Contains(caption, want) {
			t.Errorf("caption 缺少 %q:\n%s", want, caption)
		}
	}
}

func TestSeasonSegmentsSparseEpisodes(t *testing.T) {
	e1, e3, e5 := 1, 3, 5
	items := []notifyItem{
		{Season: 1, Episode: &e1, SeasonTotal: 12},
		{Season: 1, Episode: &e3, SeasonTotal: 12},
		{Season: 1, Episode: &e5, SeasonTotal: 12},
	}
	segs := seasonSegments(items)
	if len(segs) != 1 || segs[0].Range != "S01E01、E03、E05" {
		t.Fatalf("segs = %+v", segs)
	}
	if segs[0].Total != 12 {
		t.Fatalf("Total = %d", segs[0].Total)
	}
}

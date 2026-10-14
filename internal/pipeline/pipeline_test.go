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

// 静态确认 MetaInfo 解析被 pipeline 正确使用
var _ = mediaparser.MetaInfo

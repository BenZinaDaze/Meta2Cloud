package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/webui/store"
)

// addVideoWithPickCode 向 fake 网盘添加带 pick_code 与大小的视频文件。
func (f *fakeWebProvider) addVideoWithPickCode(name, parentID, pickCode string, size int64) cloudfile.CloudFile {
	cf := f.addFile(name, parentID)
	cf.Extra = map[string]any{"pick_code": pickCode}
	f.mu.Lock()
	defer f.mu.Unlock()
	cf.Size = &size
	f.byID[cf.ID] = cf
	for i := range f.children[parentID] {
		if f.children[parentID][i].ID == cf.ID {
			f.children[parentID][i] = cf
		}
	}
	return cf
}

// replacePickCode 模拟网盘重新上传：替换已有文件的 pick_code 与大小（不新增条目）。
func (f *fakeWebProvider) replacePickCode(fileID, pickCode string, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cf := f.byID[fileID]
	cf.Extra = map[string]any{"pick_code": pickCode}
	cf.Size = &size
	f.byID[fileID] = cf
	for i := range f.children[cf.ParentID] {
		if f.children[cf.ParentID][i].ID == fileID {
			f.children[cf.ParentID][i] = cf
		}
	}
}

// initStrmTestRuntime 初始化隔离的运行时并写入带 url_secret 的配置。
func initStrmTestRuntime(t *testing.T, urlSecret string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("创建配置目录失败：%v", err)
	}
	secretLine := ""
	if urlSecret != "" {
		secretLine = "  url_secret: " + urlSecret + "\n"
	}
	cfgYAML := "strm:\n  enabled: true\n" + secretLine
	if err := os.WriteFile(filepath.Join(root, "config", "config.yaml"), []byte(cfgYAML), 0o644); err != nil {
		t.Fatalf("写入配置失败：%v", err)
	}
	InitRuntime(root)
}

func testStrmConfig() *config.Config {
	cfg := config.Default()
	cfg.Strm.Enabled = true
	cfg.Strm.URLBase = "http://192.168.1.10:38765"
	return cfg
}

func TestBuildStrmURL(t *testing.T) {
	// 大写 pickcode 应统一转小写（与服务端验签逻辑一致）
	u := BuildStrmURL("http://host:38765/", "", "Abc123DEF456ghi78", "电影.mkv")
	want := "http://host:38765/strm/redirect?pickcode=abc123def456ghi78&file_name=" + "%E7%94%B5%E5%BD%B1.mkv"
	if u != want {
		t.Fatalf("BuildStrmURL = %q, want %q", u, want)
	}

	signed := BuildStrmURL("http://host:38765", "s3cret", "abc123def456ghi78", "")
	if !strings.Contains(signed, "sign="+StrmSign("s3cret", "abc123def456ghi78")) {
		t.Fatalf("签名不匹配：%q", signed)
	}
	// 构造与验签必须基于同一（小写）形式：大写入参生成的 sign 应等于小写形式的 sign
	if !strings.Contains(BuildStrmURL("http://h", "s3cret", "ABC123DEF456GHI78", ""),
		"sign="+StrmSign("s3cret", "abc123def456ghi78")) {
		t.Fatalf("大写 pickcode 的签名应与小写形式一致")
	}
	if StrmSign("s3cret", "ABC123DEF456GHI78") == StrmSign("s3cret", "abc123def456ghi78") {
		t.Fatalf("不同 pickcode 的签名不应相同")
	}
}

func TestContentDisposition(t *testing.T) {
	if got := contentDisposition("movie.mkv"); got != `attachment; filename="movie.mkv"` {
		t.Fatalf("ASCII filename = %q", got)
	}
	got := contentDisposition("电影.mkv")
	if !strings.HasPrefix(got, "attachment; filename*=UTF-8''") {
		t.Fatalf("非 ASCII filename = %q", got)
	}
}

func TestDirectURLExpiry(t *testing.T) {
	exp := time.Now().Unix() + 3600
	raw := "https://cdn.115.com/x/%E7%94%B5%E5%BD%B1.mkv?t=" + strconv.FormatInt(exp, 10) + "&u=abc"
	got := directURLExpiry(raw)
	want := time.Unix(exp, 0).Add(-strmURLExpiryMargin)
	if got.Unix() != want.Unix() {
		t.Fatalf("directURLExpiry = %v, want %v", got, want)
	}
	fallback := directURLExpiry("https://cdn.115.com/x/a.mkv")
	if !fallback.After(time.Now()) {
		t.Fatalf("t 缺失时应回退未来时间：%v", fallback)
	}
}

func TestURLFileName(t *testing.T) {
	if got := urlFileName("https://cdn.115.com/x/%E7%94%B5%E5%BD%B1.mkv?t=1&u=a"); got != "电影.mkv" {
		t.Fatalf("urlFileName = %q", got)
	}
}

func TestPickCodeValidation(t *testing.T) {
	if !pickCodeRe.MatchString("Abc123DEF456ghi78") {
		t.Fatalf("17 位字母数字应通过")
	}
	for _, bad := range []string{"", "short", "abc123DEF456ghi789", "含中文abc1234567890", "abc123def456ghi7!"} {
		if pickCodeRe.MatchString(bad) {
			t.Fatalf("%q 不应通过校验", bad)
		}
	}
}

func TestHandleStrmRedirectBadParams(t *testing.T) {
	initStrmTestRuntime(t, "s3cret")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/strm/redirect?pickcode=bad", nil)
	handleStrmRedirect(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("无效 pickcode 应 400，得到 %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/strm/redirect?pickcode=Abc123DEF456ghi78&sign=wrong", nil)
	handleStrmRedirect(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("签名错误应 403，得到 %d", rr.Code)
	}
}

func TestHandleStrmRedirectCacheHit(t *testing.T) {
	initStrmTestRuntime(t, "s3cret")
	pickCode := "abc123def456ghi78"
	expires := time.Now().Add(time.Hour)
	strmLinkCache.set(pickCode, "test-ua",
		"https://cdn.115.com/x/%E7%94%B5%E5%BD%B1.mkv?t="+strconv.FormatInt(time.Now().Unix()+7200, 10), expires)

	rr := httptest.NewRecorder()
	sign := StrmSign("s3cret", pickCode)
	req := httptest.NewRequest(http.MethodGet,
		"/strm/redirect?pickcode="+pickCode+"&sign="+sign, nil)
	req.Header.Set("User-Agent", "test-ua")
	handleStrmRedirect(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("缓存命中应 302，得到 %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "115.com") {
		t.Fatalf("Location 异常：%q", loc)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "filename*=UTF-8''") || !strings.Contains(cd, "mkv") {
		t.Fatalf("Content-Disposition 应含直链文件名：%q", cd)
	}
}

func TestSyncMappingFlow(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	fp := newFakeWebProvider()
	movieRoot := fp.addFolder("电影", "0")
	titleDir := fp.addFolder("流浪地球 2 (2023)", movieRoot.ID)
	video := fp.addVideoWithPickCode("流浪地球 2.mkv", titleDir.ID, "abc123def456ghi78", 1<<30)
	// 字幕等非视频文件不应生成 strm
	fp.addFile("流浪地球 2.ass", titleDir.ID)
	// BDMV 目录应跳过
	bdmv := fp.addFolder("BDMV", titleDir.ID)
	fp.addVideoWithPickCode("00000.m2ts", bdmv.ID, "bbb123def456ghi78", 1<<20)

	cfg := testStrmConfig()
	localRoot := filepath.Join(tmp, "media")
	mapping := config.StrmMapping{PanFolderID: movieRoot.ID, LocalPath: localRoot}

	created, updated, removed, skipped, err := syncMapping(fp, st, cfg, mapping, 0)
	if err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}
	if created != 1 || updated != 0 || removed != 0 || skipped != 0 {
		t.Fatalf("首次同步统计异常：created=%d updated=%d removed=%d skipped=%d", created, updated, removed, skipped)
	}
	strmPath := filepath.Join(localRoot, "流浪地球 2 (2023)", "流浪地球 2.strm")
	data, err := os.ReadFile(strmPath)
	if err != nil {
		t.Fatalf("strm 未生成：%v", err)
	}
	if !strings.Contains(string(data), "/strm/redirect?pickcode=abc123def456ghi78") {
		t.Fatalf("strm 内容异常：%q", string(data))
	}
	if !strings.Contains(string(data), "file_name=") {
		t.Fatalf("strm 应携带 file_name：%q", string(data))
	}

	// 二次同步：全部跳过
	created, updated, removed, skipped, err = syncMapping(fp, st, cfg, mapping, 0)
	if err != nil {
		t.Fatalf("二次同步失败：%v", err)
	}
	if created != 0 || updated != 0 || removed != 0 || skipped != 1 {
		t.Fatalf("二次同步应全部跳过：created=%d updated=%d removed=%d skipped=%d", created, updated, removed, skipped)
	}

	// pick_code 变化（网盘重新上传）：应重写
	fp.replacePickCode(video.ID, "new123def456ghi78", 2<<30)
	created, updated, _, _, err = syncMapping(fp, st, cfg, mapping, 0)
	if err != nil {
		t.Fatalf("三次同步失败：%v", err)
	}
	if created != 0 || updated != 1 {
		t.Fatalf("pick_code 变化应产生 updated：created=%d updated=%d", created, updated)
	}

	// 网盘删除文件：同步应清理本地 strm
	fp.mu.Lock()
	delete(fp.byID, video.ID)
	fp.children[movieRoot.ID] = nil
	fp.mu.Unlock()
	_, _, removed, _, err = syncMapping(fp, st, cfg, mapping, 0)
	if err != nil {
		t.Fatalf("清理同步失败：%v", err)
	}
	if removed != 1 {
		t.Fatalf("应清理 1 条失效 strm，实际 %d", removed)
	}
	if _, err := os.Stat(strmPath); !os.IsNotExist(err) {
		t.Fatalf("失效 strm 应被删除")
	}
	if st.CountStrmFiles() != 0 {
		t.Fatalf("记录应被清空，实际 %d", st.CountStrmFiles())
	}
}

func TestSyncRewritesWhenURLBaseChanges(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	fp := newFakeWebProvider()
	root := fp.addFolder("电影", "0")
	fp.addVideoWithPickCode("a.mkv", root.ID, "abc123def456ghi78", 100)

	cfg := testStrmConfig()
	localRoot := filepath.Join(tmp, "media")
	mapping := config.StrmMapping{PanFolderID: root.ID, LocalPath: localRoot}

	if _, _, _, _, err := syncMapping(fp, st, cfg, mapping, 0); err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}

	// 全部跳过
	_, _, _, skipped, err := syncMapping(fp, st, cfg, mapping, 0)
	if err != nil || skipped != 1 {
		t.Fatalf("第二次同步应跳过：skipped=%d err=%v", skipped, err)
	}

	// 改 url_base 后同步：内容不一致应重写（updated）
	cfg.Strm.URLBase = "http://10.0.0.5:39000"
	created, updated, _, _, err := syncMapping(fp, st, cfg, mapping, 0)
	if err != nil {
		t.Fatalf("改地址后同步失败：%v", err)
	}
	if created != 0 || updated != 1 {
		t.Fatalf("url_base 变更应重写：created=%d updated=%d", created, updated)
	}
	data, err := os.ReadFile(filepath.Join(localRoot, "a.strm"))
	if err != nil {
		t.Fatalf("读取 strm 失败：%v", err)
	}
	if !strings.Contains(string(data), "10.0.0.5:39000") {
		t.Fatalf("strm 内容应更新为新地址：%q", string(data))
	}

	// 改 url_secret 后同理
	cfg.Strm.URLSecret = "s3cret"
	_, updated, _, _, err = syncMapping(fp, st, cfg, mapping, 0)
	if err != nil || updated != 1 {
		t.Fatalf("url_secret 变更应重写：updated=%d err=%v", updated, err)
	}
}

func TestUpsertSkipsWhenFileMissingOnDisk(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	fp := newFakeWebProvider()
	root := fp.addFolder("电影", "0")
	fp.addVideoWithPickCode("a.mkv", root.ID, "abc123def456ghi78", 100)

	cfg := testStrmConfig()
	localRoot := filepath.Join(tmp, "media")
	mapping := config.StrmMapping{PanFolderID: root.ID, LocalPath: localRoot}

	if _, _, _, _, err := syncMapping(fp, st, cfg, mapping, 0); err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}
	strmPath := filepath.Join(localRoot, "a.strm")
	if err := os.Remove(strmPath); err != nil {
		t.Fatalf("删除本地 strm 失败：%v", err)
	}
	// 本地文件丢失时，即使记录未变也应补写
	created, _, _, _, err := syncMapping(fp, st, cfg, mapping, 0)
	if err != nil {
		t.Fatalf("补写同步失败：%v", err)
	}
	if created != 1 {
		t.Fatalf("本地文件丢失应补写（created=%d）", created)
	}
	if _, err := os.Stat(strmPath); err != nil {
		t.Fatalf("strm 应被补写：%v", err)
	}
}

func TestLocateMappingParentChain(t *testing.T) {
	fp := newFakeWebProvider()
	tvRoot := fp.addFolder("电视剧", "0")
	showDir := fp.addFolder("三体 (2023)", tvRoot.ID)
	seasonDir := fp.addFolder("Season 01", showDir.ID)

	cfg := testStrmConfig()
	cfg.Strm.Mappings = []config.StrmMapping{{PanFolderID: tvRoot.ID, LocalPath: "/mnt/tv"}}

	mapping, relPath, ok := locateMapping(fp, cfg, seasonDir)
	if !ok {
		t.Fatalf("应能定位映射")
	}
	if mapping.LocalPath != "/mnt/tv" {
		t.Fatalf("映射错误：%+v", mapping)
	}
	if relPath != "三体 (2023)/Season 01" {
		t.Fatalf("相对路径错误：%q", relPath)
	}

	// 目标文件夹本身就是映射根
	if m, rel, ok := locateMapping(fp, cfg, tvRoot); !ok || m.LocalPath != "/mnt/tv" || rel != "" {
		t.Fatalf("映射根本身应命中且相对路径为空：%+v %q %v", m, rel, ok)
	}

	// 不在映射内的目录
	other := fp.addFolder("其他", "0")
	if _, _, ok := locateMapping(fp, cfg, other); ok {
		t.Fatalf("映射外的目录不应命中")
	}
}

func TestStrmStoreRuns(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	runID := st.StartSyncRun("manual")
	if runID == 0 {
		t.Fatalf("run ID 应非零")
	}
	st.InsertSyncFileDetail(runID, "a.mkv", "/a.mkv", "/m/a.strm", "created", true, "")
	st.FinishSyncRun(runID, 1, 0, 0, 5, "")

	runs, total := st.ListSyncRuns(10, 0)
	if total != 1 || len(runs) != 1 {
		t.Fatalf("run 列表异常：total=%d len=%d", total, len(runs))
	}
	if runs[0].Created != 1 || runs[0].Skipped != 5 || runs[0].FinishedAt == nil {
		t.Fatalf("run 统计异常：%+v", runs[0])
	}
	files := st.ListSyncFiles(runID, 10)
	if len(files) != 1 || files[0].Action != "created" || !files[0].Success {
		t.Fatalf("文件明细异常：%+v", files)
	}
	if last := st.GetLastFinishedSyncRun(); last == nil || last.ID != runID {
		t.Fatalf("最近完成 run 异常")
	}
}

package webui

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"meta2cloud/internal/cloudfile"
	"meta2cloud/internal/config"
	"meta2cloud/internal/u115"
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

// fetchCount 记录 FetchBytes 调用次数（测试断言增量下载）。
var fetchCount atomic.Int64

// FetchBytes 假下载：内容长度与网盘声明的文件大小一致（模拟真实下载）。
func (f *fakeWebProvider) FetchBytes(pickCode string, limit int64) ([]byte, error) {
	fetchCount.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	size := len("SIDECAR:") + len(pickCode)
	for _, cf := range f.byID {
		if pc, _ := cf.Extra["pick_code"].(string); pc == pickCode && cf.Size != nil {
			size = int(*cf.Size)
			break
		}
	}
	if int64(size) > limit {
		size = int(limit)
	}
	out := make([]byte, size)
	copy(out, "SIDECAR:"+pickCode)
	return out, nil
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
func initStrmTestRuntime(t *testing.T, urlSecret string) *Runtime {
	t.Helper()
	// 不用 t.TempDir：全局 RT 单例的监听线程可能在清理窗口内惰性重建
	// config/data，与 RemoveAll 竞态会误判测试失败；此处清理尽力而为。
	root, err := os.MkdirTemp("", "meta2cloud-strm-test")
	if err != nil {
		t.Fatalf("创建临时目录失败：%v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
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
	return InitRuntime(root)
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
	rt := initStrmTestRuntime(t, "s3cret")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/strm/redirect?pickcode=bad", nil)
	testServer(rt).handleStrmRedirect(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("无效 pickcode 应 400，得到 %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/strm/redirect?pickcode=Abc123DEF456ghi78&sign=wrong", nil)
	testServer(rt).handleStrmRedirect(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("签名错误应 403，得到 %d", rr.Code)
	}
}

func TestHandleStrmRedirectCacheHit(t *testing.T) {
	rt := initStrmTestRuntime(t, "s3cret")
	pickCode := "abc123def456ghi78"
	expires := time.Now().Add(time.Hour)
	strmLinkCache.set(pickCode, "test-ua",
		"https://cdn.115.com/x/%E7%94%B5%E5%BD%B1.mkv?t="+strconv.FormatInt(time.Now().Unix()+7200, 10), expires)

	rr := httptest.NewRecorder()
	sign := StrmSign("s3cret", pickCode)
	req := httptest.NewRequest(http.MethodGet,
		"/strm/redirect?pickcode="+pickCode+"&sign="+sign, nil)
	req.Header.Set("User-Agent", "test-ua")
	testServer(rt).handleStrmRedirect(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("缓存命中应 302，得到 %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); !strings.Contains(loc, "115.com") {
		t.Fatalf("Location 异常：%q", loc)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "filename*=UTF-8''") || !strings.Contains(cd, "mkv") {
		t.Fatalf("Content-Disposition 应含直链文件名：%q", cd)
	}

	// 播放记录异步落库，轮询等待
	st := rt.GetStrmStore()
	if st == nil {
		t.Fatalf("STRM 存储不可用")
	}
	var logs []store.StrmPlaybackLog
	for i := 0; i < 50; i++ {
		logs, _ = st.ListPlaybackLogs(10, 0)
		if len(logs) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(logs) != 1 {
		t.Fatalf("应记录 1 条播放日志，实际 %d", len(logs))
	}
	l := logs[0]
	if l.PickCode != pickCode || !l.CacheHit || l.Status != http.StatusFound {
		t.Fatalf("播放日志字段异常：%+v", l)
	}
	if l.UserAgent == nil || *l.UserAgent != "test-ua" {
		t.Fatalf("UA 未记录：%v", l.UserAgent)
	}

	// 签名失败也应记录（403）
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/strm/redirect?pickcode="+pickCode+"&sign=bad", nil)
	req.Header.Set("User-Agent", "test-ua")
	testServer(rt).handleStrmRedirect(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("错误签名应 403，得到 %d", rr.Code)
	}
	for i := 0; i < 50; i++ {
		logs, _ = st.ListPlaybackLogs(10, 0)
		if len(logs) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(logs) != 2 || logs[0].Status != http.StatusForbidden {
		t.Fatalf("签名失败应记录 403 日志：%+v", logs)
	}
}

func TestPlaybackStoreRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	ua := "Infuse-Direct/8.1"
	st.InsertPlaybackLog("abc123def456ghi78", ptrStr("a.mkv"), &ua, false, 1234, 302, "")
	st.InsertPlaybackLog("abc123def456ghi78", ptrStr("a.mkv"), &ua, true, 0, 302, "")
	st.InsertPlaybackLog("bbb123def456ghi78", nil, nil, false, 900, 502, "115 授权失效")

	logs, total := st.ListPlaybackLogs(10, 0)
	if total != 3 || len(logs) != 3 {
		t.Fatalf("记录数异常：total=%d len=%d", total, len(logs))
	}
	if logs[0].Status != 502 || logs[0].Error == nil {
		t.Fatalf("最新记录应为 502 失败：%+v", logs[0])
	}
	if !logs[1].CacheHit || logs[1].DurationMs != 0 {
		t.Fatalf("缓存命中记录异常：%+v", logs[1])
	}
	if logs[2].DurationMs != 1234 || logs[2].CacheHit {
		t.Fatalf("直链延迟未记录：%+v", logs[2])
	}

	stats := st.GetPlaybackStats()
	if stats.Total != 3 || stats.Today != 3 {
		t.Fatalf("统计异常：total=%d today=%d", stats.Total, stats.Today)
	}
	if stats.AvgLatencyMs != 1234 {
		t.Fatalf("平均延迟异常（仅实时获取且成功的 302 参与统计）：%v", stats.AvgLatencyMs)
	}
	if stats.CacheHitRate <= 0 || stats.CacheHitRate >= 1 {
		t.Fatalf("缓存命中率异常：%v", stats.CacheHitRate)
	}
}

func ptrStr(s string) *string { return &s }

// countingTransport 计数并返回固定 downurl 响应的假传输层。
type countingTransport struct {
	mu    sync.Mutex
	count int
	resp  string
}

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.count++
	t.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(t.resp)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func (t *countingTransport) calls() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

func fakeDownurlResp(pickCode string) string {
	return `{"state":true,"data":{"` + pickCode + `":{"url":{"url":"https://cdn.115.com/x/a.mkv?t=99999999999"}}}}`
}

func TestStrmFetchLimiter(t *testing.T) {
	rt := initStrmTestRuntime(t, "")
	resetStrmLimiter()
	t.Cleanup(resetStrmLimiter)

	pickA := "abc123def456ghi78"
	pickB := "bbb123def456ghi78"
	doReq := func(pickCode string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/strm/redirect?pickcode="+pickCode, nil)
		req.Header.Set("User-Agent", "limiter-ua")
		testServer(rt).handleStrmRedirect(rr, req)
		return rr
	}

	// 1) 失败（无 token client → 授权错误）→ 负缓存
	rt.u115Client = u115.NewClient("test")
	if rr := doReq(pickA); rr.Code != http.StatusBadGateway {
		t.Fatalf("无 token 应 502，得到 %d", rr.Code)
	}
	// 2) 换成正常 client，同 pickcode 再请求：应命中负缓存，不打 115
	transport := &countingTransport{resp: fakeDownurlResp(pickA)}
	goodToken := u115.Token{AccessToken: "x", ExpiresIn: 86400, RefreshTime: time.Now().Unix()}
	rt.u115Client = u115.NewClient("test", u115.WithToken(goodToken), u115.WithHTTPClient(&http.Client{Transport: transport}))
	if rr := doReq(pickA); rr.Code != http.StatusBadGateway {
		t.Fatalf("负缓存应返回同状态 502，得到 %d", rr.Code)
	}
	if n := transport.calls(); n != 0 {
		t.Fatalf("负缓存命中不应请求 115，实际 %d 次", n)
	}

	// 3) 不同 pickcode：正常 302，且 t 极大 → 直链缓存生效
	if rr := doReq(pickB); rr.Code != http.StatusFound {
		t.Fatalf("正常请求应 302，得到 %d（body=%s）", rr.Code, rr.Body.String())
	}
	if n := transport.calls(); n != 1 {
		t.Fatalf("应恰好请求 1 次 115，实际 %d", n)
	}
	if rr := doReq(pickB); rr.Code != http.StatusFound {
		t.Fatalf("直链缓存命中应 302")
	}
	if n := transport.calls(); n != 1 {
		t.Fatalf("直链缓存命中不应再请求 115，实际 %d 次", n)
	}

	// 4) 排队超时：占满信号量（向缓冲 channel 填充令牌）后请求应快速 503
	originWait := strmFetchWait
	strmFetchWait = 50 * time.Millisecond
	t.Cleanup(func() { strmFetchWait = originWait })
	strmFetchSem <- struct{}{}
	strmFetchSem <- struct{}{}
	rr := doReq("ccc123def456ghi78")
	<-strmFetchSem
	<-strmFetchSem
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("信号量占满应 503，得到 %d", rr.Code)
	}
	if ra := rr.Header().Get("Retry-After"); ra == "" {
		t.Fatalf("503 应带 Retry-After")
	}
}

func TestSyncMappingFlow(t *testing.T) {
	rt := InitRuntime(t.TempDir())
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

	created, updated, removed, skipped, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
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
	created, updated, removed, skipped, err = rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil {
		t.Fatalf("二次同步失败：%v", err)
	}
	if created != 0 || updated != 0 || removed != 0 || skipped != 1 {
		t.Fatalf("二次同步应全部跳过：created=%d updated=%d removed=%d skipped=%d", created, updated, removed, skipped)
	}

	// pick_code 变化（网盘重新上传）：应重写
	fp.replacePickCode(video.ID, "new123def456ghi78", 2<<30)
	created, updated, _, _, err = rt.syncMapping(fp, st, cfg, mapping, 0, nil)
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
	_, _, removed, _, err = rt.syncMapping(fp, st, cfg, mapping, 0, nil)
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

func TestSyncSidecarFlow(t *testing.T) {
	rt := InitRuntime(t.TempDir())
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	fp := newFakeWebProvider()
	movieRoot := fp.addFolder("电影", "0")
	titleDir := fp.addFolder("流浪地球 2 (2023)", movieRoot.ID)
	fp.addVideoWithPickCode("流浪地球 2.mkv", titleDir.ID, "abc123def456ghi78", 1<<30)
	sub := fp.addVideoWithPickCode("流浪地球 2.zh-CN.srt", titleDir.ID, "sub123def456ghi78", 25)
	fp.addVideoWithPickCode("poster.jpg", titleDir.ID, "pic123def456ghi78", 25)

	cfg := testStrmConfig()
	// 默认白名单仅含字幕；显式加入 jpg 以覆盖图片类伴随文件
	cfg.Strm.SidecarExt = append(cfg.Strm.SidecarExt, "jpg")
	localRoot := filepath.Join(tmp, "media")
	mapping := config.StrmMapping{PanFolderID: movieRoot.ID, LocalPath: localRoot}

	fetchCount.Store(0)
	created, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}
	if created != 3 {
		t.Fatalf("首次同步应产出 strm+字幕+图片 3 项，实际 %d", created)
	}
	if n := fetchCount.Load(); n != 2 {
		t.Fatalf("应下载 2 个伴随文件，实际 %d", n)
	}
	subData, err := os.ReadFile(filepath.Join(localRoot, "流浪地球 2 (2023)", "流浪地球 2.zh-CN.srt"))
	if err != nil || string(subData) != "SIDECAR:sub123def456ghi78" {
		t.Fatalf("字幕内容异常：%v %q", err, string(subData))
	}
	if _, err := os.Stat(filepath.Join(localRoot, "流浪地球 2 (2023)", "poster.jpg")); err != nil {
		t.Fatalf("图片未下载：%v", err)
	}

	// 二次同步：全部跳过，不重复下载
	fetchCount.Store(0)
	created, updated, _, skipped, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil {
		t.Fatalf("二次同步失败：%v", err)
	}
	if created != 0 || updated != 0 || skipped != 3 {
		t.Fatalf("二次同步应全部跳过：created=%d updated=%d skipped=%d", created, updated, skipped)
	}
	if n := fetchCount.Load(); n != 0 {
		t.Fatalf("增量同步不应重复下载，实际 %d 次", n)
	}

	// 字幕在网盘更新（大小变化）→ 重新下载（updated）
	fp.replacePickCode(sub.ID, "sub123def456ghi78", 4096)
	_, updated, _, _, err = rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil {
		t.Fatalf("三次同步失败：%v", err)
	}
	if updated != 1 {
		t.Fatalf("字幕更新应产生 updated，实际 %d", updated)
	}

	// 网盘删除字幕 → 同步清理本地字幕（.strm 保留）
	// 先放入一个「播放器刮削产物」（无 DB 记录），验证不会被清理
	foreign := filepath.Join(localRoot, "流浪地球 2 (2023)", "fanart.jpg")
	if err := os.WriteFile(foreign, []byte("emby-scratch"), 0o644); err != nil {
		t.Fatalf("写入刮削产物失败：%v", err)
	}
	fp.mu.Lock()
	fp.children[titleDir.ID] = filterOut(fp.children[titleDir.ID], sub.ID)
	delete(fp.byID, sub.ID)
	fp.mu.Unlock()
	created, updated, removed, skipped, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil {
		t.Fatalf("清理同步失败：%v", err)
	}
	if removed != 1 || skipped != 2 {
		t.Fatalf("应清理 1 条字幕记录：removed=%d skipped=%d created=%d updated=%d", removed, skipped, created, updated)
	}
	if _, err := os.Stat(filepath.Join(localRoot, "流浪地球 2 (2023)", "流浪地球 2.zh-CN.srt")); !os.IsNotExist(err) {
		t.Fatalf("本地字幕应被清理")
	}
	if _, err := os.Stat(filepath.Join(localRoot, "流浪地球 2 (2023)", "流浪地球 2.strm")); err != nil {
		t.Fatalf("strm 应保留：%v", err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("播放器刮削产物（无记录文件）不应被清理：%v", err)
	}
}

func filterOut(items []cloudfile.CloudFile, id string) []cloudfile.CloudFile {
	out := items[:0]
	for _, it := range items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	return out
}

func TestSyncRewritesWhenURLBaseChanges(t *testing.T) {
	rt := InitRuntime(t.TempDir())
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

	if _, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil); err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}

	// 全部跳过
	_, _, _, skipped, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil || skipped != 1 {
		t.Fatalf("第二次同步应跳过：skipped=%d err=%v", skipped, err)
	}

	// 改 url_base 后同步：内容不一致应重写（updated）
	cfg.Strm.URLBase = "http://10.0.0.5:39000"
	created, updated, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
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
	_, updated, _, _, err = rt.syncMapping(fp, st, cfg, mapping, 0, nil)
	if err != nil || updated != 1 {
		t.Fatalf("url_secret 变更应重写：updated=%d err=%v", updated, err)
	}
}

func TestUpsertSkipsWhenFileMissingOnDisk(t *testing.T) {
	rt := InitRuntime(t.TempDir())
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

	if _, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil); err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}
	strmPath := filepath.Join(localRoot, "a.strm")
	if err := os.Remove(strmPath); err != nil {
		t.Fatalf("删除本地 strm 失败：%v", err)
	}
	// 本地文件丢失时，即使记录未变也应补写
	created, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil)
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
	rt := InitRuntime(t.TempDir())
	fp := newFakeWebProvider()
	tvRoot := fp.addFolder("电视剧", "0")
	showDir := fp.addFolder("三体 (2023)", tvRoot.ID)
	seasonDir := fp.addFolder("Season 01", showDir.ID)

	cfg := testStrmConfig()
	cfg.Strm.Mappings = []config.StrmMapping{{PanFolderID: tvRoot.ID, LocalPath: "/mnt/tv"}}

	// 无链条：走网络回退逐级向上查询
	mapping, relPath, ok := rt.locateMapping(fp, cfg, seasonDir, nil)
	if !ok {
		t.Fatalf("应能定位映射")
	}
	if mapping.LocalPath != "/mnt/tv" {
		t.Fatalf("映射错误：%+v", mapping)
	}
	if relPath != "三体 (2023)/Season 01" {
		t.Fatalf("相对路径错误：%q", relPath)
	}

	// 管线传链：本地匹配映射根并拼接相对路径，全程零网络调用。
	// 用一个 GetFile 必失败的 Provider 证明链条命中时不发任何反查请求
	mapping, relPath, ok = rt.locateMapping(&failGetFileProvider{fakeWebProvider: fp}, cfg, seasonDir,
		[]cloudfile.CloudFile{
			{ID: tvRoot.ID, Name: "电视剧", FileType: cloudfile.TypeFolder},
			{ID: showDir.ID, Name: "三体 (2023)", FileType: cloudfile.TypeFolder},
			{ID: seasonDir.ID, Name: "Season 01", FileType: cloudfile.TypeFolder},
		})
	if !ok {
		t.Fatalf("传链模式应能定位映射")
	}
	if mapping.LocalPath != "/mnt/tv" {
		t.Fatalf("传链映射错误：%+v", mapping)
	}
	if relPath != "三体 (2023)/Season 01" {
		t.Fatalf("传链相对路径错误：%q", relPath)
	}

	// 目标文件夹本身就是映射根
	if m, rel, ok := rt.locateMapping(fp, cfg, tvRoot, nil); !ok || m.LocalPath != "/mnt/tv" || rel != "" {
		t.Fatalf("映射根本身应命中且相对路径为空：%+v %q %v", m, rel, ok)
	}

	// 不在映射内的目录
	other := fp.addFolder("其他", "0")
	if _, _, ok := rt.locateMapping(fp, cfg, other, nil); ok {
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

// 映射本地位置变更后，旧位置的记录与文件应由全局孤儿对账清理
func TestSweepOrphanStrmFiles(t *testing.T) {
	rt := InitRuntime(t.TempDir())
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	fp := newFakeWebProvider()
	movieRoot := fp.addFolder("电影", "0")
	titleDir := fp.addFolder("流浪地球 2 (2023)", movieRoot.ID)
	fp.addVideoWithPickCode("流浪地球 2.mkv", titleDir.ID, "abc123def456ghi78", 1<<30)
	cfg := testStrmConfig()

	oldRoot := filepath.Join(tmp, "old")
	newRoot := filepath.Join(tmp, "new")
	mapping := config.StrmMapping{PanFolderID: movieRoot.ID, LocalPath: oldRoot}
	newMapping := config.StrmMapping{PanFolderID: movieRoot.ID, LocalPath: newRoot}

	// 旧位置同步 → 生成 1 条记录与 1 个文件
	if _, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, nil); err != nil {
		t.Fatalf("旧位置同步失败：%v", err)
	}
	if st.CountStrmFiles() != 1 {
		t.Fatalf("应有 1 条记录，got %d", st.CountStrmFiles())
	}
	oldFile := filepath.Join(oldRoot, "流浪地球 2 (2023)", "流浪地球 2.strm")
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("旧位置文件未生成：%v", err)
	}

	// 换位置同步：新位置生成；孤儿对账清掉旧位置记录与文件
	runID := st.StartSyncRun("test")
	if _, _, _, _, err := rt.syncMapping(fp, st, cfg, newMapping, runID, nil); err != nil {
		t.Fatalf("新位置同步失败：%v", err)
	}
	if orphans := rt.sweepOrphanStrmFiles(st, []config.StrmMapping{newMapping}, runID); orphans != 1 {
		t.Fatalf("应清理 1 条孤儿，got %d", orphans)
	}
	if st.CountStrmFiles() != 1 {
		t.Fatalf("对账后应只剩新位置 1 条，got %d", st.CountStrmFiles())
	}
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Fatal("旧位置文件应被删除")
	}
	if _, err := os.Stat(filepath.Join(newRoot, "流浪地球 2 (2023)", "流浪地球 2.strm")); err != nil {
		t.Fatalf("新位置文件应保留：%v", err)
	}
	// 旧位置空目录链应被清理
	if _, err := os.Stat(oldRoot); !os.IsNotExist(err) {
		t.Fatal("旧位置空目录应被清理")
	}
}

// failGetFileProvider GetFile 恒失败的假网盘（验证传链模式下不发任何反查请求）。
type failGetFileProvider struct {
	*fakeWebProvider
}

func (f *failGetFileProvider) GetFile(string) (cloudfile.CloudFile, error) {
	return cloudfile.CloudFile{}, fmt.Errorf("不应发起 GetFile 反查")
}

// 路径型映射：映射值为网盘根起算的相对路径，与链条节点名字路径前缀匹配
func TestLocateMappingByPathChain(t *testing.T) {
	rt := InitRuntime(t.TempDir())
	fp := newFakeWebProvider()
	media := fp.addFolder("Media", "0")
	tvDir := fp.addFolder("剧集", media.ID)
	showDir := fp.addFolder("三体 (2023)", tvDir.ID)
	seasonDir := fp.addFolder("Season 01", showDir.ID)

	cfg := testStrmConfig()
	cfg.Strm.Mappings = []config.StrmMapping{{PanFolderID: "Media/剧集", LocalPath: "/mnt/tv"}}
	chain := []cloudfile.CloudFile{
		{ID: media.ID, Name: "Media", FileType: cloudfile.TypeFolder},
		{ID: tvDir.ID, Name: "剧集", FileType: cloudfile.TypeFolder},
		{ID: showDir.ID, Name: "三体 (2023)", FileType: cloudfile.TypeFolder},
		{ID: seasonDir.ID, Name: "Season 01", FileType: cloudfile.TypeFolder},
	}

	// 链上「Media/剧集」节点命中映射路径，相对路径取其后的名字
	m, rel, ok := rt.locateMapping(&failGetFileProvider{fakeWebProvider: fp}, cfg, seasonDir, chain)
	if !ok || m.LocalPath != "/mnt/tv" || rel != "三体 (2023)/Season 01" {
		t.Fatalf("路径型映射匹配错误：%+v %q %v", m, rel, ok)
	}

	// 映射指向更深的剧名目录
	cfg.Strm.Mappings = []config.StrmMapping{{PanFolderID: "Media/剧集/三体 (2023)", LocalPath: "/mnt/santi"}}
	m, rel, ok = rt.locateMapping(&failGetFileProvider{fakeWebProvider: fp}, cfg, seasonDir, chain)
	if !ok || m.LocalPath != "/mnt/santi" || rel != "Season 01" {
		t.Fatalf("深路径映射匹配错误：%+v %q %v", m, rel, ok)
	}
}

func testStrmCategories() *config.CategoriesConfig {
	return &config.CategoriesConfig{
		Dirs:  config.CategoryDirs{TV: "剧集", Movie: "电影", Other: "其他"},
		Movie: []config.CategoryRule{{Name: "动漫电影"}, {Name: "普通电影"}},
		TV:    []config.CategoryRule{{Name: "动漫"}},
	}
}

func TestStrmChangesAddAndList(t *testing.T) {
	sc := newStrmChanges(testStrmCategories())
	sc.add("电影/动漫电影/玲芽之旅 (2022)")
	sc.add("剧集/动漫/葬送的芙莉莲/Season 01")
	sc.add("电影/普通电影/某种未分类片名")
	sc.add("电影/其他未知目录名/xyz.strm")
	got := sc.list()
	want := map[string]bool{"动漫": true, "动漫电影": true, "普通电影": true, "电影": true, "剧集": true}
	if len(got) != len(want) {
		t.Fatalf("分类收集异常：%v，期望 %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("分类收集异常：多余的 %q（全部 %v）", g, got)
		}
	}
	// nil 接收者安全
	var nilChanges *strmChanges
	nilChanges.add("电影/动漫电影/x")
	if nilChanges.list() != nil {
		t.Fatalf("nil 收集器应返回 nil")
	}
}

func TestSyncMappingCollectsChangedCategories(t *testing.T) {
	rt := InitRuntime(t.TempDir())
	tmp := t.TempDir()
	st, err := store.OpenStrmStore(filepath.Join(tmp, "strm.db"))
	if err != nil {
		t.Fatalf("打开 strm.db 失败：%v", err)
	}
	defer st.Close()

	fp := newFakeWebProvider()
	movieRoot := fp.addFolder("电影", "0")
	catDir := fp.addFolder("动漫电影", movieRoot.ID)
	titleDir := fp.addFolder("千与千寻 (2001)", catDir.ID)
	fp.addVideoWithPickCode("千与千寻.mkv", titleDir.ID, "cat123def456ghi78", 1<<30)

	cfg := testStrmConfig()
	localRoot := filepath.Join(tmp, "media")
	mapping := config.StrmMapping{PanFolderID: movieRoot.ID, LocalPath: localRoot}

	changes := newStrmChanges(testStrmCategories())
	if _, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, changes); err != nil {
		t.Fatalf("首次同步失败：%v", err)
	}
	if got := changes.list(); len(got) != 1 || got[0] != "动漫电影" {
		t.Fatalf("应收集到分类 动漫电影，得到 %v", got)
	}

	// 无变更的二次同步不应产生分类
	changes2 := newStrmChanges(testStrmCategories())
	if _, _, _, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, changes2); err != nil {
		t.Fatalf("二次同步失败：%v", err)
	}
	if got := changes2.list(); len(got) != 0 {
		t.Fatalf("无变更同步不应收集分类，得到 %v", got)
	}

	// 网盘侧删除源文件 → sweep 清 strm，应收集分类
	fp.mu.Lock()
	delete(fp.byID, titleDir.ID)
	fp.children[catDir.ID] = nil
	fp.mu.Unlock()
	changes3 := newStrmChanges(testStrmCategories())
	if _, _, removed, _, err := rt.syncMapping(fp, st, cfg, mapping, 0, changes3); err != nil {
		t.Fatalf("删除对账同步失败：%v", err)
	} else if removed != 1 {
		t.Fatalf("应清理 1 个 strm，得到 removed=%d", removed)
	}
	if got := changes3.list(); len(got) != 1 || got[0] != "动漫电影" {
		t.Fatalf("删除对账应收集分类 动漫电影，得到 %v", got)
	}
}

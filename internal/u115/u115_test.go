package u115

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ── PKCE（RFC 7636 附录 B 测试向量） ───────────────────────

func TestGenerateCodeChallenge(t *testing.T) {
	// RFC 7636: verifier "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	// → challenge "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	got := GenerateCodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk")
	if got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("challenge = %q, 与 RFC 7636 测试向量不符", got)
	}
}

func TestGenerateCodeVerifierLength(t *testing.T) {
	if _, err := GenerateCodeVerifier(10); err == nil {
		t.Error("长度 10 应报错（要求 43~128）")
	}
	if _, err := GenerateCodeVerifier(200); err == nil {
		t.Error("长度 200 应报错（要求 43~128）")
	}
	v, err := GenerateCodeVerifier(64)
	if err != nil || len(v) != 64 {
		t.Errorf("长度 64 应成功且长度一致, err=%v len=%d", err, len(v))
	}
}

// ── 分享链接解析 ────────────────────────────────────────────

func TestParseShareURL(t *testing.T) {
	cases := []struct {
		url      string
		code     string
		password string
	}{
		{"https://115.com/s/abc123?password=xyz", "abc123", "xyz"},
		{"https://115cdn.com/s/abc123", "abc123", ""},
		{"https://115.com/s/abc123#extra", "abc123", ""},
		{"https://115.com/notshare/abc", "", ""},
	}
	for _, c := range cases {
		code, password := ParseShareURL(c.url)
		if code != c.code || password != c.password {
			t.Errorf("ParseShareURL(%q) = (%q, %q), want (%q, %q)", c.url, code, password, c.code, c.password)
		}
	}
}

// ── 上传哈希 ────────────────────────────────────────────────

func TestComputeUploadHashesBytes(t *testing.T) {
	// 小于 128MB 时 preid == fileid（全量哈希）
	data := []byte("hello world panoply")
	h := ComputeUploadHashesBytes(data)
	if h.FileSize != int64(len(data)) {
		t.Errorf("FileSize = %d, want %d", h.FileSize, len(data))
	}
	if h.FileID != h.PreID {
		t.Errorf("小于 128MB 时 PreID 应等于 FileID: %s vs %s", h.FileID, h.PreID)
	}
	// 已知向量：sha1("hello") = aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d
	h2 := ComputeUploadHashesBytes([]byte("hello"))
	if h2.FileID != "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d" {
		t.Errorf("FileID = %s, sha1 已知向量不符", h2.FileID)
	}
}

func TestComputeSignValBytes(t *testing.T) {
	// "0-4" 范围即 "hello" 的 SHA1 大写
	got, err := ComputeSignValBytes([]byte("hello world"), "0-4")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ToUpper("aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d")
	if got != want {
		t.Errorf("SignVal = %s, want %s", got, want)
	}
	if _, err := ComputeSignValBytes([]byte("x"), "5-3"); err == nil {
		t.Error("非法范围应报错")
	}
}

// ── 模型解析 ────────────────────────────────────────────────

func TestFileFromListItem(t *testing.T) {
	raw := map[string]any{
		"fid": "9876543", "fn": "movie.mkv", "fc": "1",
		"fs": 734003200.0, "upt": 1704067200.0, "pc": "xyz789",
	}
	f := FileFromListItem(raw)
	if f.ID != "9876543" || f.Name != "movie.mkv" || f.Category != "1" {
		t.Errorf("基础字段解析错误: %+v", f)
	}
	if f.IsFolder() {
		t.Error("category=1 应为文件")
	}
	if f.Size == nil || *f.Size != 734003200 {
		t.Error("size 解析错误")
	}
	if f.PickCode == nil || *f.PickCode != "xyz789" {
		t.Error("pick_code 解析错误")
	}
}

func TestTokenExpiry(t *testing.T) {
	tok := Token{
		AccessToken:  "a",
		RefreshToken: "r",
		ExpiresIn:    100,
		RefreshTime:  time.Now().Unix() - 200,
	}
	if !tok.IsExpired() {
		t.Error("200 秒前签发、有效期 100 秒的 token 应已过期")
	}
	fresh := Token{AccessToken: "a", RefreshToken: "r", ExpiresIn: 3600, RefreshTime: time.Now().Unix()}
	if fresh.IsExpired() {
		t.Error("刚签发的 token 不应过期")
	}
}

// ── 客户端请求管线（httptest 模拟） ────────────────────────

// newTestClient 构造指向 mock 服务器的客户端（高 QPS 避免测试慢）。
func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	c := NewClient("test-client",
		WithToken(Token{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 3600, RefreshTime: time.Now().Unix()}),
		WithQPS(1000),
		WithCooldownSeconds(1),
		WithHTTPClient(server.Client()),
	)
	// 将 API 域名替换为测试服务器
	c.testingBaseURL = server.URL
	return c, server
}

func TestListFilesRequestAndParse(t *testing.T) {
	var gotAuth, gotQuery string
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]any{
			"state": true,
			"code":  0,
			"data": []map[string]any{
				{"fid": "111", "fn": "Season 01", "fc": "0"},
				{"fid": "222", "fn": "movie.mkv", "fc": "1", "fs": 1024.0, "pc": "pc1"},
			},
		})
	}))
	defer server.Close()

	files, err := client.ListFiles("0", 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer at" {
		t.Errorf("Authorization = %q, want Bearer at", gotAuth)
	}
	for _, want := range []string{"cid=0", "limit=1000", "offset=0", "show_dir=1"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q 缺少 %q", gotQuery, want)
		}
	}
	if len(files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(files))
	}
	if !files[0].IsFolder() || files[0].Name != "Season 01" {
		t.Errorf("目录解析错误: %+v", files[0])
	}
	if files[1].IsFolder() || files[1].Name != "movie.mkv" {
		t.Errorf("文件解析错误: %+v", files[1])
	}
}

func TestBusinessError(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"state": false, "code": 990001, "message": "参数错误"})
	}))
	defer server.Close()

	_, err := client.ListFiles("0", 10, 0)
	if err == nil {
		t.Fatal("业务失败应返回错误")
	}
	ae, ok := err.(*APIError)
	if !ok {
		t.Fatalf("错误类型应为 *APIError, got %T", err)
	}
	if ae.Kind != KindAPI || ae.CodeValue() != 990001 {
		t.Errorf("kind/code = %v/%d, want KindAPI/990001", ae.Kind, ae.CodeValue())
	}
	if !strings.Contains(ae.Error(), "参数错误") {
		t.Errorf("错误信息应包含原始 message: %v", ae)
	}
}

func TestAuthError(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"state": false, "code": 40140119, "message": "token 失效"})
	}))
	defer server.Close()

	_, err := client.ListFiles("0", 10, 0)
	if !IsAuth(err) {
		t.Errorf("40140119 应识别为认证错误: %v", err)
	}
}

func TestCooldownOn429(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer server.Close()

	_, err := client.ListFiles("0", 10, 0)
	if !IsRateLimit(err) {
		t.Fatalf("429 应识别为限流错误: %v", err)
	}
	// 冷却中：立即再次请求应直接报限流，不发起 HTTP
	if _, err := client.ListFiles("0", 10, 0); !IsRateLimit(err) {
		t.Errorf("冷却期内应直接限流: %v", err)
	}
}

func TestRetryOn5xx(t *testing.T) {
	var calls atomic.Int32
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(502)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": []any{}})
	}))
	defer server.Close()

	if _, err := client.ListFiles("0", 10, 0); err != nil {
		t.Fatalf("5xx 后重试应成功: %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("calls = %d, want 2", got)
	}
}

func TestListAllFilesPagination(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		var data []map[string]any
		if offset == "0" {
			for i := 0; i < 2; i++ {
				data = append(data, map[string]any{"fid": "f" + string(rune('a'+i)), "fn": "n", "fc": "1"})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"state": true, "code": 0, "data": data})
	}))
	defer server.Close()

	// limit=2 时第一页满页 → 翻页 → 第二页空 → 停止
	files, err := client.ListAllFiles("0", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("len(files) = %d, want 2", len(files))
	}
}

func TestGetDownloadURLParse(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"state": true,
			"code":  0,
			"data": map[string]any{
				"pc1": map[string]any{
					"url": map[string]any{"url": "https://dl.example.com/file.mkv"},
				},
			},
		})
	}))
	defer server.Close()

	urlStr, err := client.GetDownloadURL("pc1")
	if err != nil {
		t.Fatal(err)
	}
	if urlStr != "https://dl.example.com/file.mkv" {
		t.Errorf("url = %q", urlStr)
	}
}

// TestCreateDeviceCodeNumericTime 115 可能以数字类型下发 authDeviceCode 的
// time 字段（服务端变更）：必须容错解析为字符串，否则轮询签名报 key invalid。
func TestCreateDeviceCodeNumericTime(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"state":true,"data":{"uid":"abc123","time":1792145872,"sign":"sig001","qrcode":"https://115.com/scan/dg-x"}}`))
	}))
	defer server.Close()

	session, err := client.CreateDeviceCode("verifier-123")
	if err != nil {
		t.Fatal(err)
	}
	if session.TimeValue != "1792145872" {
		t.Errorf("数字型 time 应解析为字符串，实际 %q", session.TimeValue)
	}
	if session.UID != "abc123" || session.Sign != "sig001" || session.Qrcode != "https://115.com/scan/dg-x" {
		t.Errorf("会话字段异常：%+v", session)
	}

	// 字符串型 time 同样兼容
	client2, server2 := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"state":true,"data":{"uid":"u2","time":"1792145999","sign":"s2","qrcode":"https://115.com/scan/dg-y"}}`))
	}))
	defer server2.Close()
	session2, err := client2.CreateDeviceCode("v2")
	if err != nil {
		t.Fatal(err)
	}
	if session2.TimeValue != "1792145999" {
		t.Errorf("字符串型 time 应原样返回，实际 %q", session2.TimeValue)
	}
}

// TestFetchBytesUsesClientUA 直链与 UA 绑定：FetchBytes 拉取内容必须携带与
// 申请直链相同的 User-Agent，否则 CDN 返回 403。
func TestFetchBytesUsesClientUA(t *testing.T) {
	var srvURL string
	var downUA, fileUA string
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open/ufile/downurl":
			downUA = r.Header.Get("User-Agent")
			json.NewEncoder(w).Encode(map[string]any{
				"state": true,
				"data": map[string]any{
					"pc1": map[string]any{
						"url": map[string]any{"url": srvURL + "/file"},
					},
				},
			})
		case "/file":
			fileUA = r.Header.Get("User-Agent")
			_, _ = w.Write([]byte("subtitle-content"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	srvURL = server.URL

	data, err := client.FetchBytes("pc1", 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "subtitle-content" {
		t.Errorf("content = %q", string(data))
	}
	if downUA == "" {
		t.Fatal("downurl 请求应携带 User-Agent")
	}
	if fileUA != downUA {
		t.Errorf("拉取内容 UA 应与申请直链一致：downurl=%q file=%q", downUA, fileUA)
	}
}

// TestIsAuthWrapped 授权错误被 %w 层层包装后仍应被 IsAuth 识别（调用方会包装 provider 错误）。
func TestIsAuthWrapped(t *testing.T) {
	base := &APIError{Kind: KindAuth, Message: "token 失效"}
	wrapped := fmt.Errorf("列举网盘目录失败：%w", fmt.Errorf("映射同步失败：%w", base))
	if !IsAuth(wrapped) {
		t.Error("包装后的授权错误应仍被 IsAuth 识别")
	}
	if IsAuth(fmt.Errorf("普通网络错误")) || IsAuth(nil) {
		t.Error("普通错误/nil 不应被识别为授权错误")
	}
}

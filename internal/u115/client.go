// Package u115 是 115 开放平台独立接口层（对应 Python 版 u115pan 包）。
//
// 设计目标：
//   - 独立于主程序流程，只做尽量薄的 HTTP 包装
//   - 覆盖扫码授权、基础文件操作、下载、上传前置接口、空间查询、云下载
//   - 内置 QPS 预约与冷却限流， goroutine 安全
package u115

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	passportAPI = "https://passportapi.115.com"
	qrcodeAPI   = "https://qrcodeapi.115.com"
	proAPI      = "https://proapi.115.com"
	webAPI      = "https://webapi.115.com"

	defaultUserAgent = "W115Storage/2.0"
	cookieUserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.102 Safari/537.36 MicroMessenger/6.8.0(0x16080000) NetType/WIFI MiniProgramEnv/Mac MacWechat/WMPF XWEB/30626"
	cookieReferer    = "https://servicewechat.com/wx2c744c010a61b0fa/94/page-frame.html"

	// 二次上传认证需要读取的 preid 前缀大小。
	preidSize = 128 * 1024 * 1024
)

var successCodes = map[float64]bool{0: true, 20004: true}

var authErrorCodes = map[float64]bool{
	40140116: true,
	40140119: true,
	40140120: true,
	40140125: true,
}

// Client 115 开放平台客户端。
type Client struct {
	ClientID string

	tokenMu        sync.Mutex
	token          *Token
	TokenPath      string
	OnTokenUpdated func(c *Client, t Token)

	HTTP      *http.Client
	userAgent string

	apiQPS       float64
	downloadQPS  float64
	CooldownSecs int

	rateMu         sync.Mutex
	nextAPIAt      time.Time
	nextDownloadAt time.Time
	cooldownUntil  time.Time

	webCookieMu sync.Mutex
	webCookie   string

	// testingBaseURL 非空时覆盖所有 API 基础地址（仅测试使用）。
	testingBaseURL string
}

// Option 客户端可选项。
type Option func(*Client)

// WithToken 设置初始 token。
func WithToken(t Token) Option { return func(c *Client) { c.token = &t } }

// WithTokenPath 设置 token 持久化路径。
func WithTokenPath(p string) Option { return func(c *Client) { c.TokenPath = p } }

// WithOnTokenUpdated 设置 token 更新回调。
func WithOnTokenUpdated(fn func(c *Client, t Token)) Option {
	return func(c *Client) { c.OnTokenUpdated = fn }
}

// WithTimeout 设置 HTTP 超时（秒）。
func WithTimeout(seconds int) Option {
	return func(c *Client) { c.HTTP.Timeout = time.Duration(seconds) * time.Second }
}

// WithUserAgent 覆盖默认 User-Agent。
func WithUserAgent(ua string) Option { return func(c *Client) { c.userAgent = ua } }

// WithQPS 分别设置普通接口与下载接口的 QPS。
func WithQPS(apiQPS, downloadQPS float64) Option {
	return func(c *Client) { c.apiQPS = apiQPS; c.downloadQPS = downloadQPS }
}

// WithCooldownSeconds 设置风控冷却时长（秒）。
func WithCooldownSeconds(sec int) Option { return func(c *Client) { c.CooldownSecs = sec } }

// WithHTTPClient 注入自定义 http.Client（测试用）。
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.HTTP = hc } }

// NewClient 构造客户端。默认 api_qps=3、download_qps=1、冷却 1 小时。
func NewClient(clientID string, opts ...Option) *Client {
	c := &Client{
		ClientID:     clientID,
		HTTP:         &http.Client{Timeout: 30 * time.Second},
		apiQPS:       3.0,
		downloadQPS:  1.0,
		CooldownSecs: 3600,
		userAgent:    defaultUserAgent,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewClientFromTokenFile 从本地 token JSON 构造客户端。
func NewClientFromTokenFile(clientID, tokenPath string, opts ...Option) (*Client, error) {
	token, ok, err := LoadToken(tokenPath)
	if err != nil {
		return nil, err
	}
	opts = append(opts, WithTokenPath(tokenPath))
	if ok {
		opts = append(opts, WithToken(token))
	}
	return NewClient(clientID, opts...), nil
}

// ── token 管理 ──────────────────────────────────────────────

func (c *Client) persistToken(token Token) {
	if c.TokenPath == "" {
		return
	}
	if err := SaveToken(token, c.TokenPath); err != nil {
		// 持久化失败不阻断流程，与 Python 版一致（异常向上抛但调用处宽容）
		_ = err
	}
}

func (c *Client) emitTokenUpdated(token Token) {
	if c.OnTokenUpdated != nil {
		c.OnTokenUpdated(c, token)
	}
}

// GetValidAccessToken 获取可用 access_token，必要时自动刷新。
func (c *Client) GetValidAccessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token == nil {
		return "", &APIError{Kind: KindAuth, Message: "未配置 115 token，请先完成授权"}
	}
	if c.token.IsExpired() {
		if _, err := c.refreshTokenLocked(); err != nil {
			return "", err
		}
	}
	return c.token.AccessToken, nil
}

// RefreshToken 刷新 access_token，并更新本地 token 存储。
func (c *Client) RefreshToken() (Token, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	return c.refreshTokenLocked()
}

// refreshTokenLocked 需已持有 tokenMu。
func (c *Client) refreshTokenLocked() (Token, error) {
	if c.token == nil {
		return Token{}, &APIError{Kind: KindAuth, Message: "没有可刷新的 token"}
	}
	payload, err := c.request(http.MethodPost, passportAPI, "/open/refreshToken", requestOpts{
		form: url.Values{"refresh_token": {c.token.RefreshToken}},
	})
	if err != nil {
		return Token{}, err
	}
	data := jMap(payload, "data")
	token := Token{
		AccessToken:  jString(data, "access_token"),
		RefreshToken: jString(data, "refresh_token"),
		ExpiresIn:    jInt(data, "expires_in", 0),
		RefreshTime:  time.Now().Unix(),
	}
	c.token = &token
	c.persistToken(token)
	c.emitTokenUpdated(token)
	return token, nil
}

// ── 限流 ────────────────────────────────────────────────────

func (c *Client) enterCooldown() {
	until := time.Now().Add(time.Duration(c.CooldownSecs) * time.Second)
	c.rateMu.Lock()
	if until.After(c.cooldownUntil) {
		c.cooldownUntil = until
	}
	if until.After(c.nextAPIAt) {
		c.nextAPIAt = until
	}
	if until.After(c.nextDownloadAt) {
		c.nextDownloadAt = until
	}
	c.rateMu.Unlock()
}

func rateLimitError(message string, code *float64, payload map[string]any) *APIError {
	return &APIError{Kind: KindRateLimit, Message: message, Code: code, Payload: payload}
}

func (c *Client) sleepRateLimit(isDownload bool) error {
	c.rateMu.Lock()
	now := time.Now()
	if now.Before(c.cooldownUntil) {
		retryAfter := int(c.cooldownUntil.Sub(now).Seconds()) + 1
		c.rateMu.Unlock()
		return rateLimitError(fmt.Sprintf("115 接口冷却中，请 %d 秒后再试", retryAfter), strPtrFloat(429), map[string]any{
			"retry_after": retryAfter,
		})
	}
	qps := c.apiQPS
	nextAt := c.nextAPIAt
	if isDownload {
		qps = c.downloadQPS
		nextAt = c.nextDownloadAt
	}
	reserveAt := now
	if nextAt.After(reserveAt) {
		reserveAt = nextAt
	}
	wait := reserveAt.Sub(now)
	if qps > 0 {
		reserveAt = reserveAt.Add(time.Duration(float64(time.Second) / qps))
	}
	if isDownload {
		c.nextDownloadAt = reserveAt
	} else {
		c.nextAPIAt = reserveAt
	}
	c.rateMu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
	return nil
}

func strPtrFloat(f float64) *float64 { return &f }

// ── 请求核心 ────────────────────────────────────────────────

type requestOpts struct {
	params         url.Values // GET 查询串
	form           url.Values // POST 表单体
	authRequired   bool
	allowCode20004 bool
	isDownload     bool
	maxAttempts    int    // 0 → 默认 3
	userAgent      string // 非空时覆盖客户端默认 User-Agent（115 直链与 UA 绑定）
}

func (c *Client) request(method, baseURL, pathStr string, opts requestOpts) (map[string]any, error) {
	if c.testingBaseURL != "" {
		baseURL = c.testingBaseURL
	}
	target := baseURL + pathStr
	maxAttempts := opts.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	delay := 1.0

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.sleepRateLimit(opts.isDownload); err != nil {
			return nil, err
		}

		payload, retryable, err := c.doRequest(method, target, opts)
		if err == nil {
			return payload, nil
		}
		lastErr = err
		// 业务错误（限流 / 认证 / 业务码失败）doRequest 已标 retryable=false，
		// 这里只重试网络层错误与 5xx（与 Python 版一致）
		if attempt >= maxAttempts || !retryable {
			return nil, err
		}
		jitter := 0.0
		var b [1]byte
		if _, err := rand.Read(b[:]); err == nil {
			jitter = float64(b[0]) / 255.0
		}
		time.Sleep(time.Duration((delay + jitter) * float64(time.Second)))
		delay *= 2
	}
	return nil, lastErr
}

// doRequest 执行单次 HTTP 请求并解析业务码。
// retryable 指示网络层错误是否值得重试。
func (c *Client) doRequest(method, target string, opts requestOpts) (payload map[string]any, retryable bool, err error) {
	var body io.Reader
	if opts.form != nil {
		body = strings.NewReader(opts.form.Encode())
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, false, err
	}
	if opts.params != nil {
		req.URL.RawQuery = opts.params.Encode()
	}
	if opts.userAgent != "" {
		req.Header.Set("User-Agent", opts.userAgent)
	} else {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if opts.form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if opts.authRequired {
		token, err := c.GetValidAccessToken()
		if err != nil {
			return nil, false, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err // 网络错误可重试
	}
	defer resp.Body.Close()

	raw, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		c.enterCooldown()
		return nil, false, rateLimitError("115 接口返回 HTTP 429，进入冷却", strPtrFloat(429), map[string]any{
			"status_code": 429,
		})
	}
	if resp.StatusCode >= 500 {
		return nil, true, &APIError{Message: "HTTP " + strconv.Itoa(resp.StatusCode)}
	}
	if readErr != nil {
		return nil, true, readErr
	}

	payload = map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			if resp.StatusCode >= 400 {
				return nil, false, &APIError{Message: "HTTP " + strconv.Itoa(resp.StatusCode)}
			}
			return nil, true, &APIError{Message: "响应不是合法 JSON: " + err.Error()}
		}
	}
	if resp.StatusCode >= 400 {
		// 4xx/5xx 非 JSON 或业务失败：若响应提示"请求异常需要重试"则重试（与 Python 版一致）
		message := jString(payload, "message")
		if message == "" {
			message = jString(payload, "msg")
		}
		retry := strings.Contains(message, "请求异常需要重试")
		return nil, retry, &APIError{Message: "HTTP " + strconv.Itoa(resp.StatusCode), Payload: payload}
	}

	message := jString(payload, "message")
	if message == "" {
		message = jString(payload, "msg")
	}
	if strings.Contains(message, "已达到当前访问上限") {
		c.enterCooldown()
		code, _ := jNumber(payload, "code")
		return nil, false, rateLimitError("115 接口提示达到访问上限，进入冷却", &code, payload)
	}

	codeVal, hasCode := jNumber(payload, "code")
	state, hasState := payload["state"].(bool)

	ok := false
	if hasCode {
		ok = successCodes[codeVal]
		if !opts.allowCode20004 && codeVal == 20004 {
			ok = false
		}
	} else if hasState && state {
		ok = true
	}
	if ok {
		return payload, false, nil
	}

	if hasCode && authErrorCodes[codeVal] {
		statePtr := statePtrOf(payload)
		return nil, false, &APIError{Kind: KindAuth, Message: orDefault(message, "115 授权失败"), Code: &codeVal, State: statePtr, Payload: payload}
	}
	codePtr := (*float64)(nil)
	if hasCode {
		codePtr = &codeVal
	}
	return nil, false, &APIError{Message: orDefault(message, "115 接口返回失败"), Code: codePtr, State: statePtrOf(payload), Payload: payload}
}

func statePtrOf(payload map[string]any) *bool {
	if v, ok := payload["state"].(bool); ok {
		return &v
	}
	return nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// ── 扫码授权 ────────────────────────────────────────────────

// CreateDeviceCode 创建扫码登录二维码与会话参数。
func (c *Client) CreateDeviceCode(codeVerifier string) (DeviceCodeSession, error) {
	if codeVerifier == "" {
		v, err := GenerateCodeVerifier(64)
		if err != nil {
			return DeviceCodeSession{}, err
		}
		codeVerifier = v
	}
	payload, err := c.request(http.MethodPost, passportAPI, "/open/authDeviceCode", requestOpts{
		form: url.Values{
			"client_id":             {c.ClientID},
			"code_challenge":        {GenerateCodeChallenge(codeVerifier)},
			"code_challenge_method": {"sha256"},
		},
	})
	if err != nil {
		return DeviceCodeSession{}, err
	}
	data := jMap(payload, "data")
	return DeviceCodeSession{
		Qrcode:       jString(data, "qrcode"),
		UID:          jString(data, "uid"),
		TimeValue:    jString(data, "time"),
		Sign:         jString(data, "sign"),
		CodeVerifier: codeVerifier,
	}, nil
}

// GetQrcodeStatus 轮询扫码状态。
func (c *Client) GetQrcodeStatus(session DeviceCodeSession) (QrcodeStatus, error) {
	payload, err := c.request(http.MethodGet, qrcodeAPI, "/get/status/", requestOpts{
		params: url.Values{
			"uid":  {session.UID},
			"time": {session.TimeValue},
			"sign": {session.Sign},
		},
	})
	if err != nil {
		return QrcodeStatus{}, err
	}
	data := jMap(payload, "data")
	return QrcodeStatus{
		Status:  int(jInt(data, "status", 0)),
		Message: jString(data, "msg"),
		Raw:     payload,
	}, nil
}

// ExchangeDeviceToken 使用扫码登录会话换取 token。
func (c *Client) ExchangeDeviceToken(session DeviceCodeSession) (Token, error) {
	payload, err := c.request(http.MethodPost, passportAPI, "/open/deviceCodeToToken", requestOpts{
		form: url.Values{
			"uid":           {session.UID},
			"code_verifier": {session.CodeVerifier},
		},
	})
	if err != nil {
		return Token{}, err
	}
	data := jMap(payload, "data")
	token := Token{
		AccessToken:  jString(data, "access_token"),
		RefreshToken: jString(data, "refresh_token"),
		ExpiresIn:    jInt(data, "expires_in", 0),
		RefreshTime:  time.Now().Unix(),
	}
	c.tokenMu.Lock()
	c.token = &token
	c.tokenMu.Unlock()
	c.persistToken(token)
	c.emitTokenUpdated(token)
	return token, nil
}

// ── 文件列举与查找 ──────────────────────────────────────────

// ListFiles 列出单个目录下一页内容。
func (c *Client) ListFiles(cid string, limit, offset int) ([]File, error) {
	payload, err := c.request(http.MethodGet, proAPI, "/open/ufile/files", requestOpts{
		params: url.Values{
			"cid":      {cid},
			"limit":    {strconv.Itoa(limit)},
			"offset":   {strconv.Itoa(offset)},
			"cur":      {"True"},
			"show_dir": {"1"},
		},
		authRequired: true,
	})
	if err != nil {
		return nil, err
	}
	items := jMapSlice(payload, "data")
	files := make([]File, 0, len(items))
	for _, item := range items {
		files = append(files, FileFromListItem(item))
	}
	return files, nil
}

// ListAllFiles 自动翻页列出目录全部内容。
func (c *Client) ListAllFiles(cid string, limit int) ([]File, error) {
	var results []File
	offset := 0
	for {
		batch, err := c.ListFiles(cid, limit, offset)
		if err != nil {
			return nil, err
		}
		results = append(results, batch...)
		if len(batch) < limit {
			break
		}
		offset += limit
	}
	return results, nil
}

// GetPathInfo 按完整路径查询文件或目录详情；不存在时返回 (zero, false), nil。
func (c *Client) GetPathInfo(path string) (File, bool, error) {
	payload, err := c.request(http.MethodPost, proAPI, "/open/folder/get_info", requestOpts{
		form:         url.Values{"path": {path}},
		authRequired: true,
	})
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) && (ae.CodeValue() == 20018 || ae.CodeValue() == 50018) {
			return File{}, false, nil
		}
		return File{}, false, err
	}
	data := jMap(payload, "data")
	if data == nil || jString(data, "file_id") == "" {
		return File{}, false, nil
	}
	return FileFromPathInfo(data), true, nil
}

// Exists 判断路径是否存在。
func (c *Client) Exists(path string) (bool, error) {
	_, ok, err := c.GetPathInfo(path)
	return ok, err
}

// GetFileByID 尝试按文件 ID 获取详情。
// 115 开放平台没有稳定的按 ID 直查接口，这里使用两段式回退：
//  1. search 快路径，兼容按名称传入或搜索恰好命中的场景
//  2. 在给定 roots 下递归扫描 file_id
//
// 返回 (zero, false), nil 表示未找到。
func (c *Client) GetFileByID(fileID string, roots []string, maxDepth int) (File, bool, error) {
	if maxDepth <= 0 {
		maxDepth = 12
	}
	if results, err := c.Search(fileID, 1, 0, "", "", ""); err == nil {
		for _, item := range results {
			if item.ID == fileID {
				return item, true, nil
			}
		}
	}

	scanRoots := roots
	if len(scanRoots) == 0 {
		scanRoots = []string{"0"}
	}
	seen := map[string]bool{}
	for _, root := range scanRoots {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		found, ok, err := c.findFileIDUnderRoot(fileID, root, maxDepth, 0)
		if err != nil {
			return File{}, false, err
		}
		if ok {
			return found, true, nil
		}
	}
	return File{}, false, nil
}

func (c *Client) findFileIDUnderRoot(fileID, rootID string, maxDepth, depth int) (File, bool, error) {
	if depth > maxDepth {
		return File{}, false, nil
	}
	items, err := c.ListFiles(rootID, 1000, 0)
	if err != nil {
		return File{}, false, err
	}
	for _, item := range items {
		if item.ID == fileID {
			return item, true, nil
		}
		if item.IsFolder() {
			found, ok, err := c.findFileIDUnderRoot(fileID, item.ID, maxDepth, depth+1)
			if err != nil {
				return File{}, false, err
			}
			if ok {
				return found, true, nil
			}
		}
	}
	return File{}, false, nil
}

// Search 搜索文件或目录。cid / fc / fileType / suffix 为空串时忽略。
func (c *Client) Search(searchValue string, limit, offset int, cid, fc, suffix string) ([]File, error) {
	params := url.Values{
		"search_value": {searchValue},
		"limit":        {strconv.Itoa(limit)},
		"offset":       {strconv.Itoa(offset)},
	}
	if cid != "" {
		params.Set("cid", cid)
	}
	if fc != "" {
		params.Set("fc", fc)
	}
	if suffix != "" {
		params.Set("suffix", suffix)
	}
	payload, err := c.request(http.MethodGet, proAPI, "/open/ufile/search", requestOpts{
		params:       params,
		authRequired: true,
	})
	if err != nil {
		return nil, err
	}
	items := jMapSlice(payload, "data")
	files := make([]File, 0, len(items))
	for _, item := range items {
		files = append(files, FileFromSearchItem(item))
	}
	return files, nil
}

// ── 文件夹操作 ──────────────────────────────────────────────

// CreateFolder 在父目录下创建文件夹；若已存在也视为成功。返回新目录 ID。
func (c *Client) CreateFolder(pid, fileName string) (string, error) {
	payload, err := c.request(http.MethodPost, proAPI, "/open/folder/add", requestOpts{
		form: url.Values{
			"pid":       {pid},
			"file_name": {fileName},
		},
		authRequired:   true,
		allowCode20004: true,
	})
	if err != nil {
		return "", err
	}
	data := jMap(payload, "data")
	if fid := jStringPtr(data, "file_id"); fid != nil {
		return *fid, nil
	}
	return "", nil
}

// EnsureFolderPath 按绝对路径确保目录存在（path 必须以 / 开头）。
func (c *Client) EnsureFolderPath(path string) (File, error) {
	if !strings.HasPrefix(path, "/") {
		return File{}, errors.New("目录路径必须以 / 开头")
	}
	parts := make([]string, 0, 8)
	for _, part := range strings.Split(path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		if root, ok, err := c.GetPathInfo("/"); err != nil {
			return File{}, err
		} else if ok {
			return root, nil
		}
		return File{ID: "0", Name: "/", Category: "0"}, nil
	}

	parentID := "0"
	currentPath := ""
	var currentInfo File
	var ok bool
	var err error
	for _, part := range parts {
		currentPath += "/" + part
		currentInfo, ok, err = c.GetPathInfo(currentPath)
		if err != nil {
			return File{}, err
		}
		if !ok {
			if _, err := c.CreateFolder(parentID, part); err != nil {
				return File{}, err
			}
			currentInfo, ok, err = c.GetPathInfo(currentPath)
			if err != nil {
				return File{}, err
			}
			if !ok {
				return File{}, NewAPIError("创建目录失败：" + currentPath)
			}
		}
		parentID = currentInfo.ID
	}
	return currentInfo, nil
}

// ── 文件修改 ────────────────────────────────────────────────

// Delete 删除文件或目录。
func (c *Client) Delete(fileID string) error {
	_, err := c.request(http.MethodPost, proAPI, "/open/ufile/delete", requestOpts{
		form:         url.Values{"file_ids": {fileID}},
		authRequired: true,
	})
	return err
}

// Rename 重命名文件或目录。
func (c *Client) Rename(fileID, fileName string) error {
	_, err := c.request(http.MethodPost, proAPI, "/open/ufile/update", requestOpts{
		form: url.Values{
			"file_id":   {fileID},
			"file_name": {fileName},
		},
		authRequired: true,
	})
	return err
}

// Copy 复制文件或目录到目标目录。
func (c *Client) Copy(fileID, pid string) error {
	_, err := c.request(http.MethodPost, proAPI, "/open/ufile/copy", requestOpts{
		form: url.Values{
			"file_id": {fileID},
			"pid":     {pid},
		},
		authRequired: true,
	})
	return err
}

// Move 移动文件或目录到目标目录。
func (c *Client) Move(fileID, toCid string) error {
	_, err := c.request(http.MethodPost, proAPI, "/open/ufile/move", requestOpts{
		form: url.Values{
			"file_ids": {fileID},
			"to_cid":   {toCid},
		},
		authRequired: true,
	})
	return err
}

// ── 下载 ────────────────────────────────────────────────────

// GetDownloadURL 根据 pick_code 获取真实下载地址。
func (c *Client) GetDownloadURL(pickCode string) (string, error) {
	return c.GetDownloadURLWithUA(pickCode, "")
}

// GetDownloadURLWithUA 根据 pick_code 获取真实下载地址，并以 userAgent 作为本次请求的
// User-Agent。115 直链与请求时的 UA 绑定：302 播放场景必须透传播放器的 UA，
// 否则播放器带自身 UA 访问直链会被 115 拒绝（403）。userAgent 为空时使用客户端默认 UA。
func (c *Client) GetDownloadURLWithUA(pickCode, userAgent string) (string, error) {
	payload, err := c.request(http.MethodPost, proAPI, "/open/ufile/downurl", requestOpts{
		form:         url.Values{"pick_code": {pickCode}},
		authRequired: true,
		isDownload:   true,
		userAgent:    userAgent,
	})
	if err != nil {
		return "", err
	}
	return downloadURLFromPayload(payload)
}

// downloadURLFromPayload 解析 downurl 响应：data -> {pick_code: {url: {url: ...}}}。
func downloadURLFromPayload(payload map[string]any) (string, error) {
	if data := jMap(payload, "data"); data != nil {
		for _, value := range data {
			if d, ok := value.(map[string]any); ok {
				if u := jMap(d, "url"); u != nil && jString(u, "url") != "" {
					return jString(u, "url"), nil
				}
			}
		}
	}
	for _, value := range payload {
		if d, ok := value.(map[string]any); ok {
			if u := jMap(d, "url"); u != nil && jString(u, "url") != "" {
				return jString(u, "url"), nil
			}
		}
	}
	return "", &APIError{Message: "未从 downurl 响应中解析到下载地址", Payload: payload}
}

// GetHTTPClient 暴露底层 http.Client（下载等直接使用）。
func (c *Client) GetHTTPClient() *http.Client { return c.HTTP }

// DownloadFile 下载文件到本地，异常时删除残留文件。
// pickCode 为空时按 path 查询详情获取 pick_code。
func (c *Client) DownloadFile(pickCode, path, destPath string) (string, error) {
	if pickCode == "" {
		if path == "" {
			return "", errors.New("pickCode 与 path 至少需要提供一个")
		}
		info, ok, err := c.GetPathInfo(path)
		if err != nil {
			return "", err
		}
		if !ok || info.PickCode == nil || *info.PickCode == "" {
			return "", NewAPIError("无法获取文件详情或 pick_code：" + path)
		}
		pickCode = *info.PickCode
	}

	downloadURL, err := c.GetDownloadURL(pickCode)
	if err != nil {
		return "", err
	}
	if dir := fileDir(destPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}

	resp, err := c.HTTP.Get(downloadURL)
	if err != nil {
		_ = os.Remove(destPath)
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		_ = os.Remove(destPath)
		return "", NewAPIError("下载失败：HTTP " + strconv.Itoa(resp.StatusCode))
	}
	out, err := os.Create(destPath)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(out, resp.Body)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(destPath)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destPath)
		return "", closeErr
	}
	return destPath, nil
}

func fileDir(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "."
}

// ── 上传前置接口 ────────────────────────────────────────────

// ComputeUploadHashes 计算上传所需全量与 128MB 前缀 SHA1。
func ComputeUploadHashes(filePath string) (UploadHashes, error) {
	fh, err := os.Open(filePath)
	if err != nil {
		return UploadHashes{}, err
	}
	defer fh.Close()

	sha1All := sha1.New()
	sha1Pre := sha1.New()
	buf := make([]byte, 1024*1024)
	var size int64
	remainingPre := int64(preidSize)

	for {
		n, readErr := fh.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			size += int64(n)
			sha1All.Write(chunk)
			if remainingPre > 0 {
				feed := chunk
				if int64(len(feed)) > remainingPre {
					feed = feed[:remainingPre]
				}
				sha1Pre.Write(feed)
				remainingPre -= int64(len(feed))
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return UploadHashes{}, readErr
		}
	}
	return UploadHashes{
		FileSize: size,
		FileID:   hex.EncodeToString(sha1All.Sum(nil)),
		PreID:    hex.EncodeToString(sha1Pre.Sum(nil)),
	}, nil
}

// ComputeUploadHashesBytes 直接对内存数据计算上传特征值（NFO/图片等小文件免落盘）。
func ComputeUploadHashesBytes(data []byte) UploadHashes {
	sha1All := sha1.Sum(data)
	preEnd := len(data)
	if preEnd > preidSize {
		preEnd = preidSize
	}
	sha1Pre := sha1.Sum(data[:preEnd])
	return UploadHashes{
		FileSize: int64(len(data)),
		FileID:   hex.EncodeToString(sha1All[:]),
		PreID:    hex.EncodeToString(sha1Pre[:]),
	}
}

// ComputeSignVal 按 signCheck（"start-end"）指定的字节范围计算二次认证 SHA1（大写）。
func ComputeSignVal(filePath, signCheck string) (string, error) {
	parts := strings.SplitN(signCheck, "-", 2)
	if len(parts) != 2 {
		return "", errors.New("非法 sign_check：" + signCheck)
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", err
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", err
	}
	length := end - start + 1
	if length <= 0 {
		return "", errors.New("非法 sign_check：" + signCheck)
	}
	fh, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer fh.Close()
	if _, err := fh.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(fh, length))
	if err != nil {
		return "", err
	}
	sum := sha1.Sum(data)
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

// ComputeSignValBytes 对内存数据按 signCheck 范围计算二次认证 SHA1（大写）。
func ComputeSignValBytes(data []byte, signCheck string) (string, error) {
	parts := strings.SplitN(signCheck, "-", 2)
	if len(parts) != 2 {
		return "", errors.New("非法 sign_check：" + signCheck)
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", err
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", err
	}
	length := end - start + 1
	if length <= 0 || start >= int64(len(data)) {
		return "", errors.New("非法 sign_check：" + signCheck)
	}
	if end > int64(len(data))-1 {
		end = int64(len(data)) - 1
	}
	sum := sha1.Sum(data[start : end+1])
	return strings.ToUpper(hex.EncodeToString(sum[:])), nil
}

// BuildUploadTarget 将目录 ID 转换为上传接口要求的 target 参数。
func BuildUploadTarget(cid string) string { return "U_1_" + cid }

// InitUploadArgs 初始化上传参数；二次认证时补充 PickCode / SignKey / SignVal。
type InitUploadArgs struct {
	FileName string
	FileSize int64
	Cid      string
	FileID   string // 全量 SHA1
	PreID    string // 前缀 SHA1
	PickCode string
	SignKey  string
	SignVal  string
}

// InitUpload 初始化上传。
func (c *Client) InitUpload(args InitUploadArgs) (UploadInitInfo, error) {
	form := url.Values{
		"file_name": {args.FileName},
		"file_size": {strconv.FormatInt(args.FileSize, 10)},
		"target":    {BuildUploadTarget(args.Cid)},
		"fileid":    {args.FileID},
		"preid":     {args.PreID},
	}
	if args.PickCode != "" {
		form.Set("pick_code", args.PickCode)
	}
	if args.SignKey != "" {
		form.Set("sign_key", args.SignKey)
	}
	if args.SignVal != "" {
		form.Set("sign_val", args.SignVal)
	}
	payload, err := c.request(http.MethodPost, proAPI, "/open/upload/init", requestOpts{
		form:         form,
		authRequired: true,
	})
	if err != nil {
		return UploadInitInfo{}, err
	}
	raw := jMap(payload, "data")
	info := UploadInitInfo{
		Bucket:    jStringPtr(raw, "bucket"),
		Object:    jStringPtr(raw, "object"),
		Callback:  jMap(raw, "callback"),
		SignCheck: jStringPtr(raw, "sign_check"),
		PickCode:  jStringPtr(raw, "pick_code"),
		SignKey:   jStringPtr(raw, "sign_key"),
		Code:      jIntPtr(raw, "code"),
		Status:    jIntPtr(raw, "status"),
		Raw:       raw,
	}
	if fid := jStringPtr(raw, "file_id"); fid != nil {
		info.FileID = fid
	}
	return info, nil
}

// GetUploadToken 获取对象存储上传临时凭证。
func (c *Client) GetUploadToken() (UploadToken, error) {
	payload, err := c.request(http.MethodGet, proAPI, "/open/upload/get_token", requestOpts{
		authRequired: true,
	})
	if err != nil {
		return UploadToken{}, err
	}
	data := jMap(payload, "data")
	return UploadToken{
		Endpoint:        jString(data, "endpoint"),
		AccessKeyID:     jString(data, "AccessKeyId"),
		AccessKeySecret: jString(data, "AccessKeySecret"),
		SecurityToken:   jString(data, "SecurityToken"),
		Raw:             data,
	}, nil
}

// ResumeUpload 查询断点续传信息。
func (c *Client) ResumeUpload(fileSize int64, cid, fileID, pickCode string) (ResumeInfo, error) {
	payload, err := c.request(http.MethodPost, proAPI, "/open/upload/resume", requestOpts{
		form: url.Values{
			"file_size": {strconv.FormatInt(fileSize, 10)},
			"target":    {BuildUploadTarget(cid)},
			"fileid":    {fileID},
			"pick_code": {pickCode},
		},
		authRequired: true,
	})
	if err != nil {
		return ResumeInfo{}, err
	}
	data := jMap(payload, "data")
	return ResumeInfo{Callback: jMap(data, "callback"), Raw: data}, nil
}

// ── 空间与回收站 ────────────────────────────────────────────

// GetSpaceInfo 获取总空间与剩余空间。
func (c *Client) GetSpaceInfo() (SpaceInfo, error) {
	payload, err := c.request(http.MethodGet, proAPI, "/open/user/info", requestOpts{
		authRequired: true,
	})
	if err != nil {
		return SpaceInfo{}, err
	}
	data := jMap(payload, "data")
	rtInfo := jMap(data, "rt_space_info")
	total := jInt(jMap(rtInfo, "all_total"), "size", 0)
	remain := jInt(jMap(rtInfo, "all_remain"), "size", 0)
	return SpaceInfo{TotalSize: total, RemainSize: remain, Raw: data}, nil
}

// ListRecycleBin 列出回收站内容。
func (c *Client) ListRecycleBin(limit, offset int) ([]RecycleBinItem, error) {
	payload, err := c.request(http.MethodGet, proAPI, "/open/rb/list", requestOpts{
		params: url.Values{
			"limit":  {strconv.Itoa(limit)},
			"offset": {strconv.Itoa(offset)},
		},
		authRequired: true,
	})
	if err != nil {
		return nil, err
	}
	items := jMapSlice(payload, "data")
	results := make([]RecycleBinItem, 0, len(items))
	for _, item := range items {
		results = append(results, RecycleBinItem{
			ID:       firstNonEmpty(jString(item, "fid"), jString(item, "file_id")),
			Name:     firstNonEmpty(jString(item, "fn"), jString(item, "file_name")),
			Category: jStringPtr(item, "fc"),
			Size:     jIntPtr(item, "fs"),
			Raw:      item,
		})
	}
	return results, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── Cookie API: 分享转存 ────────────────────────────────────

// SetCookie 设置 Cookie 用于 Web API 认证。
func (c *Client) SetCookie(cookie string) {
	c.webCookieMu.Lock()
	c.webCookie = strings.TrimSpace(cookie)
	c.webCookieMu.Unlock()
}

// SetTestBaseURL 覆盖 API 基础地址（仅测试使用）。
func (c *Client) SetTestBaseURL(baseURL string) { c.testingBaseURL = baseURL }

// HasCookie 当前是否已配置可用的 Web API Cookie。
func (c *Client) HasCookie() bool {
	c.webCookieMu.Lock()
	defer c.webCookieMu.Unlock()
	return c.webCookie != ""
}

func (c *Client) webRequest(method, pathStr string, params, form url.Values) (map[string]any, error) {
	if !c.HasCookie() {
		return nil, &APIError{Kind: KindAuth, Message: "未设置 115 Web API Cookie"}
	}

	target := webAPI + pathStr
	if c.testingBaseURL != "" {
		target = c.testingBaseURL + pathStr
	}
	delay := 1.0
	maxAttempts := 3
	if params == nil {
		params = url.Values{}
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.sleepRateLimit(false); err != nil {
			return nil, err
		}
		var body io.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		}
		req, err := http.NewRequest(method, target, body)
		if err != nil {
			return nil, err
		}
		req.URL.RawQuery = params.Encode()
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("User-Agent", cookieUserAgent)
		req.Header.Set("Referer", cookieReferer)
		c.webCookieMu.Lock()
		req.Header.Set("Cookie", c.webCookie)
		c.webCookieMu.Unlock()

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if attempt == maxAttempts {
				return nil, err
			}
			time.Sleep(time.Duration(delay * float64(time.Second)))
			delay = math.Min(delay*2, 8.0)
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			resp.Body.Close()
			c.enterCooldown()
			return nil, rateLimitError("115 Web API 返回 HTTP 429，进入冷却", strPtrFloat(429), map[string]any{
				"status_code": 429,
			})
		}
		raw, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 500 {
			if attempt == maxAttempts {
				return nil, &APIError{Message: "HTTP " + strconv.Itoa(resp.StatusCode)}
			}
			time.Sleep(time.Duration(delay * float64(time.Second)))
			delay = math.Min(delay*2, 8.0)
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		payload := map[string]any{}
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &payload); err != nil {
				return nil, &APIError{Message: "响应不是合法 JSON: " + err.Error()}
			}
		}
		if resp.StatusCode >= 400 {
			return nil, &APIError{Message: "HTTP " + strconv.Itoa(resp.StatusCode), Payload: payload}
		}
		if !jBool(payload, "state", false) {
			message := jString(payload, "error")
			if message == "" {
				message = "请求失败"
			}
			errno, _ := jNumber(payload, "errno")
			if errno == -1 {
				return nil, &APIError{Kind: KindAuth, Message: message, Code: &errno, Payload: payload}
			}
			return nil, &APIError{Message: message, Code: &errno, Payload: payload}
		}
		return payload, nil
	}
	return nil, NewAPIError("115 Web API 请求失败")
}

// ParseShareURL 解析分享链接，提取分享码和密码。
// 支持格式：https://115.com/s/{shareCode}?password={password}
func ParseShareURL(shareURL string) (shareCode, password string) {
	parsed, err := url.Parse(shareURL)
	if err != nil {
		return "", ""
	}
	var parts []string
	for _, part := range strings.Split(parsed.Path, "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	if len(parts) >= 2 && strings.EqualFold(parts[0], "s") {
		shareCode = parts[1]
	}
	password = parsed.Query().Get("password")
	return shareCode, password
}

// GetShareInfo 获取分享链接信息。cid 为子目录 ID，根目录传空。
func (c *Client) GetShareInfo(shareCode, receiveCode, cid string, offset, limit int) (ShareInfo, []ShareItem, error) {
	if !c.HasCookie() {
		return ShareInfo{
			ShareCode:   shareCode,
			ReceiveCode: receiveCode,
			Raw:         map[string]any{"error": "未设置 115 Web API Cookie"},
		}, nil, nil
	}
	if limit <= 0 {
		limit = 100
	}
	payload, err := c.webRequest(http.MethodGet, "/share/snap", url.Values{
		"share_code":   {shareCode},
		"receive_code": {receiveCode},
		"offset":       {strconv.Itoa(offset)},
		"limit":        {strconv.Itoa(limit)},
		"cid":          {cid},
	}, nil)
	if err != nil {
		return ShareInfo{}, nil, err
	}
	data := jMap(payload, "data")
	userInfo := jMap(data, "userinfo")
	shareInfo := jMap(data, "shareinfo")

	info := ShareInfo{
		ShareCode:    shareCode,
		ReceiveCode:  receiveCode,
		Title:        jString(shareInfo, "share_title"),
		FileSize:     jInt(shareInfo, "file_size", 0),
		ReceiveCount: jInt(shareInfo, "receive_count", 0),
		CreateTime:   jStringPtr(shareInfo, "create_time"),
		ExpireTime:   jStringPtr(shareInfo, "expire_time"),
		UserID:       jString(userInfo, "user_id"),
		UserName:     jString(userInfo, "user_name"),
		SnapID:       jString(shareInfo, "snap_id"),
		Raw:          data,
	}

	items := make([]ShareItem, 0)
	for _, item := range jMapSlice(data, "list") {
		_, isFolder := item["cid"]
		var modified *string
		if t := jString(item, "t"); t != "" {
			modified = &t
		}
		items = append(items, ShareItem{
			ID:           firstNonEmpty(jString(item, "cid"), jString(item, "fid")),
			Name:         jString(item, "n"),
			Size:         jInt(item, "s", 0),
			IsFolder:     isFolder,
			ModifiedTime: modified,
			Fid:          jStringPtr(item, "fid"),
			Raw:          item,
		})
	}
	return info, items, nil
}

// ReceiveShare 转存分享文件到网盘。fileIDs 为转存 ID 列表。
func (c *Client) ReceiveShare(shareCode, receiveCode string, fileIDs []string, targetCid string) (ReceiveResult, error) {
	if !c.HasCookie() {
		return ReceiveResult{Error: "未设置 115 Web API Cookie"}, nil
	}
	if targetCid == "" {
		targetCid = "0"
	}
	payload, err := c.webRequest(http.MethodPost, "/share/receive", nil, url.Values{
		"cid":          {targetCid},
		"share_code":   {shareCode},
		"receive_code": {receiveCode},
		"file_id":      {strings.Join(fileIDs, ",")},
	})
	if err != nil {
		var ae *APIError
		if errors.As(err, &ae) {
			return ReceiveResult{Error: ae.Error(), Raw: map[string]any{"code": ae.CodeValue()}}, nil
		}
		return ReceiveResult{Error: err.Error()}, nil
	}
	data := jMap(payload, "data")
	return ReceiveResult{
		Success:     true,
		FolderCount: jInt(data, "recv_folder_count", 0),
		FileCount:   jInt(data, "recv_file_count", 0),
		TotalSize:   jInt(data, "receive_size", 0),
		Title:       jString(data, "receive_title"),
		Raw:         data,
	}, nil
}

// ReceiveShareAll 转存分享链接中的全部文件（默认处理第一页，最多 100 条）。
func (c *Client) ReceiveShareAll(shareCode, receiveCode, targetCid, cid string) (ReceiveResult, error) {
	_, items, err := c.GetShareInfo(shareCode, receiveCode, cid, 0, 100)
	if err != nil {
		return ReceiveResult{}, err
	}
	if len(items) == 0 {
		return ReceiveResult{Error: "分享链接中没有文件"}, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.FileID())
	}
	return c.ReceiveShare(shareCode, receiveCode, ids, targetCid)
}

// GetUserInfo 获取用户信息（验证 Cookie 有效性）。
func (c *Client) GetUserInfo() (map[string]any, error) {
	if !c.HasCookie() {
		return map[string]any{}, nil
	}
	payload, err := c.webRequest(http.MethodGet, "/files/index_info", nil, nil)
	if err != nil {
		return nil, err
	}
	return jMap(payload, "data"), nil
}

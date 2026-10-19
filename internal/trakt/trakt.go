// Package trakt 是 Trakt API v2 独立接口层：设备码 OAuth、token 持久化与
// 自动刷新、观看历史读写（/sync/watched、/sync/history[/remove]）。
//
// 设计与 internal/u115 对齐：薄 HTTP 包装、goroutine 安全、内置最小请求间隔节流
// （Trakt 限额 1000 次/5 分钟，同步走批量接口后调用量很低，250ms 间隔仅作兜底）。
package trakt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultAPIBase = "https://api.trakt.tv"
	// defaultAuthBase OAuth 端点（设备码认证、token 交换与刷新）专用主机。
	// 官方文档明确要求：所有 OAuth 请求使用 auth.trakt.tv，不要用 API 主机名。
	defaultAuthBase = "https://auth.trakt.tv"
	// DefaultVerificationURL 设备码激活页。
	DefaultVerificationURL = defaultAuthBase + "/activate"
	// minInterval 相邻请求最小间隔（429 时进入冷却并尊重 Retry-After）。
	minInterval = 250 * time.Millisecond
)

// Token OAuth token 持久化模型（与 u115.Token 同形；Trakt access_token 有效期 90 天）。
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshTime  int64  `json:"refresh_time"`
}

// ExpiresAt 过期时间戳（秒）。
func (t Token) ExpiresAt() int64 { return t.RefreshTime + t.ExpiresIn }

// IsExpired 是否已过期（预留 60 秒时钟偏移）。
func (t Token) IsExpired() bool { return time.Now().Unix()+60 >= t.ExpiresAt() }

// DeviceCode 设备码配对会话。
type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int64  `json:"expires_in"`
	Interval        int64  `json:"interval"`
}

// PollState 设备码轮询状态。
type PollState int

const (
	PollPending PollState = iota // 等待用户确认（authorization_pending / slow_down）
	PollExpired                  // 设备码过期（expired_token）
	PollDenied                   // 用户拒绝（denied）
)

// PollResult 一次设备码轮询结果：Token 仅在 Confirmed 时有效。
type PollResult struct {
	State   PollState
	Message string
	Token   *Token
}

// Confirmed 用户已确认授权。
func (r PollResult) Confirmed() bool { return r.Token != nil }

// ── ID 模型 ─────────────────────────────────────────────────

// IDs Trakt 媒体外部 ID 集合（同步只需 tmdb，其余原样透传便于排障）。
type IDs struct {
	Trakt int64  `json:"trakt"`
	Slug  string `json:"slug"`
	TMDB  int64  `json:"tmdb"`
	IMDB  string `json:"imdb"`
	TVDB  int64  `json:"tvdb"`
}

// Media Trakt 媒体条目。
type Media struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
	IDs   IDs    `json:"ids"`
}

// WatchedMovie GET /sync/watched/movies 条目。
type WatchedMovie struct {
	Plays         int   `json:"plays"`
	LastWatchedAt Zeit  `json:"last_watched_at"`
	Movie         Media `json:"movie"`
}

// WatchedEpisode GET /sync/watched/shows 的集条目。
type WatchedEpisode struct {
	Number        int  `json:"number"`
	Plays         int  `json:"plays"`
	LastWatchedAt Zeit `json:"last_watched_at"`
}

// WatchedSeason 按季分组的集。
type WatchedSeason struct {
	Number   int              `json:"number"`
	Episodes []WatchedEpisode `json:"episodes"`
}

// WatchedShow GET /sync/watched/shows 条目。
type WatchedShow struct {
	Plays         int             `json:"plays"`
	LastWatchedAt Zeit            `json:"last_watched_at"`
	Show          Media           `json:"show"`
	Seasons       []WatchedSeason `json:"seasons"`
}

// Zeit Trakt 时间字段（ISO8601，含毫秒）。
type Zeit string

// Unix 解析为 unix 秒；解析失败返回 0。
func (z Zeit) Unix() int64 {
	if z == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, string(z)); err == nil {
		return t.Unix()
	}
	return 0
}

// ── 客户端 ──────────────────────────────────────────────────

// Client Trakt API v2 客户端。
// Trakt 现为 PKCE 模式、不再签发 Client Secret：设备码流程与 token 刷新
// 均只携带 client_id，无 secret 参数。
type Client struct {
	ClientID  string
	TokenPath string

	HTTP *http.Client

	rateMu         sync.Mutex
	nextAt         time.Time
	cooldown       time.Time
	tokenMu        sync.Mutex
	token          *Token
	OnTokenUpdated func(t Token)

	// testingAPIBase / testingAuthBase 非空时覆盖 API / OAuth 基础地址（仅测试使用）。
	testingAPIBase  string
	testingAuthBase string
}

// Option 客户端可选项。
type Option func(*Client)

// WithToken 设置初始 token。
func WithToken(t Token) Option { return func(c *Client) { c.token = &t } }

// WithTokenPath 设置 token 持久化路径。
func WithTokenPath(p string) Option { return func(c *Client) { c.TokenPath = p } }

// WithTimeout 设置 HTTP 超时（秒）。
func WithTimeout(seconds int) Option {
	return func(c *Client) { c.HTTP.Timeout = time.Duration(seconds) * time.Second }
}

// WithHTTPClient 注入自定义 http.Client（测试用）。
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.HTTP = hc } }

// NewClient 构造客户端。
func NewClient(clientID string, opts ...Option) *Client {
	c := &Client{
		ClientID: clientID,
		HTTP:     &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// NewClientFromTokenFile 从本地 token JSON 构造客户端（文件缺失不算错误）。
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

// HasToken 是否已持有（可能过期的）token。
func (c *Client) HasToken() bool {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	return c.token != nil
}

// ── token 持久化 ────────────────────────────────────────────

// SaveToken 将 token 写入本地 JSON 文件（0o600，与 115 token 同惯例）。
func SaveToken(token Token, tokenPath string) error {
	if dir := filepath.Dir(tokenPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenPath, data, 0o600)
}

// LoadToken 从本地 JSON 文件读取 token，不存在返回 (zero, false, nil)。
func LoadToken(tokenPath string) (Token, bool, error) {
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return Token{}, false, nil
		}
		return Token{}, false, err
	}
	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return Token{}, false, err
	}
	return token, token.AccessToken != "", nil
}

func (c *Client) persistToken(token Token) {
	if c.TokenPath == "" {
		return
	}
	_ = SaveToken(token, c.TokenPath)
}

// GetValidAccessToken 获取可用 access_token，必要时自动刷新。
func (c *Client) GetValidAccessToken() (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token == nil {
		return "", fmt.Errorf("未配置 Trakt token，请先完成设备码授权")
	}
	if c.token.IsExpired() {
		if err := c.refreshTokenLocked(); err != nil {
			return "", err
		}
	}
	return c.token.AccessToken, nil
}

// RefreshToken 刷新 access_token 并更新本地存储。
func (c *Client) RefreshToken() error {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	return c.refreshTokenLocked()
}

func (c *Client) refreshTokenLocked() error {
	if c.token == nil || c.token.RefreshToken == "" {
		return fmt.Errorf("没有可刷新的 Trakt token")
	}
	var out Token
	// redirect_uri 须与应用登记一致：我们的授权走设备码流程（无浏览器回调），
	// 该值仅出现在刷新请求；Trakt Development 应用只接受 localhost 系回调，
	// 故固定为 http://localhost（创建应用时 Redirect URI 按此填写）。
	if err := c.postUnauthed("/oauth/token", map[string]any{
		"refresh_token": c.token.RefreshToken,
		"client_id":     c.ClientID,
		"grant_type":    "refresh_token",
		"redirect_uri":  "http://localhost",
	}, &out); err != nil {
		return err
	}
	if out.AccessToken == "" {
		return fmt.Errorf("Trakt token 刷新失败：响应缺少 access_token")
	}
	out.RefreshTime = time.Now().Unix()
	c.token = &out
	c.persistToken(out)
	if c.OnTokenUpdated != nil {
		c.OnTokenUpdated(out)
	}
	return nil
}

// ── 设备码配对 ──────────────────────────────────────────────

// CreateDeviceCode 发起设备码配对。
func (c *Client) CreateDeviceCode() (*DeviceCode, error) {
	var out DeviceCode
	if err := c.postUnauthed("/oauth/device/code", map[string]any{"client_id": c.ClientID}, &out); err != nil {
		return nil, err
	}
	if out.DeviceCode == "" || out.UserCode == "" {
		return nil, fmt.Errorf("Trakt 设备码响应不完整")
	}
	if out.VerificationURL == "" {
		out.VerificationURL = DefaultVerificationURL
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return &out, nil
}

// PollDeviceToken 轮询设备码授权结果（由前端驱动，每次调用一次）。
func (c *Client) PollDeviceToken(deviceCode string) (PollResult, error) {
	var raw struct {
		AccessToken      string `json:"access_token"`
		RefreshToken     string `json:"refresh_token"`
		ExpiresIn        int64  `json:"expires_in"`
		CreatedAt        int64  `json:"created_at"`
		Error            string `json:"error"`
		ErrorDescription string `json:"error_description"`
	}
	err := c.postRaw("/oauth/device/token", map[string]any{
		"code":      deviceCode,
		"client_id": c.ClientID,
	}, &raw)
	if err != nil {
		return PollResult{}, err
	}
	switch raw.Error {
	case "":
		if raw.AccessToken == "" {
			return PollResult{}, fmt.Errorf("Trakt 授权响应缺少 access_token")
		}
		token := Token{
			AccessToken:  raw.AccessToken,
			RefreshToken: raw.RefreshToken,
			ExpiresIn:    raw.ExpiresIn,
			RefreshTime:  time.Now().Unix(),
		}
		c.tokenMu.Lock()
		c.token = &token
		c.tokenMu.Unlock()
		c.persistToken(token)
		if c.OnTokenUpdated != nil {
			c.OnTokenUpdated(token)
		}
		return PollResult{Token: &token}, nil
	case "authorization_pending", "slow_down":
		return PollResult{State: PollPending, Message: "等待用户在 trakt.tv/activate 确认"}, nil
	case "expired_token":
		return PollResult{State: PollExpired, Message: "设备码已过期，请重新发起配对"}, nil
	case "denied":
		return PollResult{State: PollDenied, Message: "用户拒绝了授权"}, nil
	case "invalid_client":
		return PollResult{}, fmt.Errorf("Trakt 拒绝了该应用（invalid_client）：请检查 Client ID 是否填写正确，且应用属于当前登录的 Trakt 账号")
	default:
		msg := raw.ErrorDescription
		if msg == "" {
			msg = raw.Error
		}
		return PollResult{}, fmt.Errorf("Trakt 设备码轮询失败：%s", msg)
	}
}

// Disconnect 清除本地 token（Trakt 无服务端吊销端点，撤销需在官网设置操作）。
func (c *Client) Disconnect() {
	c.tokenMu.Lock()
	c.token = nil
	c.tokenMu.Unlock()
	if c.TokenPath != "" {
		_ = os.Remove(c.TokenPath)
	}
}

// ── 已看记录读取 ────────────────────────────────────────────

// SyncWatchedMovies 全量拉取电影已看记录（自动翻页；不翻页会在 ~100 条截断）。
func (c *Client) SyncWatchedMovies() ([]WatchedMovie, error) {
	var out []WatchedMovie
	pageSize := 250
	for page := 1; ; page++ {
		body, pageCount, err := c.getWithPagination(fmt.Sprintf("/sync/watched/movies?limit=%d&page=%d", pageSize, page))
		if err != nil {
			return nil, err
		}
		var items []WatchedMovie
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, err
		}
		out = append(out, items...)
		if page >= pageCount || len(items) == 0 {
			return out, nil
		}
	}
}

// SyncWatchedShows 全量拉取剧集已看记录（自动翻页）。
func (c *Client) SyncWatchedShows() ([]WatchedShow, error) {
	var out []WatchedShow
	pageSize := 250
	for page := 1; ; page++ {
		body, pageCount, err := c.getWithPagination(fmt.Sprintf("/sync/watched/shows?limit=%d&page=%d", pageSize, page))
		if err != nil {
			return nil, err
		}
		var items []WatchedShow
		if err := json.Unmarshal(body, &items); err != nil {
			return nil, err
		}
		out = append(out, items...)
		if page >= pageCount || len(items) == 0 {
			return out, nil
		}
	}
}

// ── 剧集观看进度 ────────────────────────────────────────────

// ProgressEpisode 单集观看进度。
type ProgressEpisode struct {
	Number        int  `json:"number"`
	Completed     bool `json:"completed"`
	LastWatchedAt Zeit `json:"last_watched_at"`
}

// ProgressSeason 单季观看进度。
type ProgressSeason struct {
	Number   int               `json:"number"`
	Episodes []ProgressEpisode `json:"episodes"`
}

// ShowWatchedProgress 剧集观看进度（每集是否已看的权威来源，
// 对剧级聚合记录与逐集记录都成立；reset_at 非 null 时官方建议忽略
// 早于该时刻的观看）。
type ShowWatchedProgress struct {
	Aired     int64            `json:"aired"`
	Completed int64            `json:"completed"`
	ResetAt   Zeit             `json:"reset_at"`
	Seasons   []ProgressSeason `json:"seasons"`
}

// ShowWatchedProgress 拉取单剧逐集观看进度。
func (c *Client) ShowWatchedProgress(traktID int64) (*ShowWatchedProgress, error) {
	var out ShowWatchedProgress
	if err := c.getAuthed(fmt.Sprintf("/shows/%d/progress/watched", traktID), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── 已看记录写入 ────────────────────────────────────────────

// HistoryMovie 批量历史条目（电影）。
type HistoryMovie struct {
	TMDBID    int64
	WatchedAt time.Time // 零值则不携带 watched_at
}

// HistoryEpisode 批量历史条目（剧集单集）。
type HistoryEpisode struct {
	ShowTMDBID int64
	Season     int
	Episode    int
	WatchedAt  time.Time
}

// HistoryResult 一次 /sync/history[/remove] 的结果。
type HistoryResult struct {
	Count         int // added/deleted 的电影+集总数（0 不代表失败，可能 already_exists）
	NotFoundCount int // 未匹配条目数；>0 时调用方应视本批为部分失败（影子不落盘，下轮重试）
}

// AddHistory 批量记录已看（单次 POST；调用方按 ≤100 集/请求分块）。
func (c *Client) AddHistory(movies []HistoryMovie, episodes []HistoryEpisode) (*HistoryResult, error) {
	return c.postHistory("/sync/history", movies, episodes)
}

// RemoveHistory 批量移除已看记录。
func (c *Client) RemoveHistory(movies []HistoryMovie, episodes []HistoryEpisode) (*HistoryResult, error) {
	return c.postHistory("/sync/history/remove", movies, episodes)
}

func (c *Client) postHistory(path string, movies []HistoryMovie, episodes []HistoryEpisode) (*HistoryResult, error) {
	body := map[string]any{}
	if len(movies) > 0 {
		list := make([]map[string]any, 0, len(movies))
		for _, m := range movies {
			entry := map[string]any{"ids": map[string]any{"tmdb": m.TMDBID}}
			if !m.WatchedAt.IsZero() {
				entry["watched_at"] = m.WatchedAt.UTC().Format("2006-01-02T15:04:05.000Z")
			}
			list = append(list, entry)
		}
		body["movies"] = list
	}
	if len(episodes) > 0 {
		body["shows"] = groupEpisodesByShow(episodes)
	}
	if len(body) == 0 {
		return &HistoryResult{}, nil
	}
	var raw struct {
		Added   map[string]int `json:"added"`
		Deleted map[string]int `json:"deleted"`
		// not_found 兼容两种形态：现行文档为按媒体类型分组的对象，
		// 旧版为扁平数组（{movie:...} / {show:...}）。
		NotFound json.RawMessage `json:"not_found"`
	}
	if err := c.postAuthed(path, body, &raw); err != nil {
		return nil, err
	}
	result := &HistoryResult{}
	for _, m := range raw.Added {
		result.Count += m
	}
	for _, m := range raw.Deleted {
		result.Count += m
	}
	result.NotFoundCount = countNotFound(raw.NotFound)
	return result, nil
}

// countNotFound 统计 not_found 条目数（对象/数组两形态都覆盖）。
func countNotFound(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		n := 0
		for _, v := range obj {
			var list []json.RawMessage
			if json.Unmarshal(v, &list) == nil {
				n += len(list)
			}
		}
		return n
	}
	var flat []json.RawMessage
	if err := json.Unmarshal(raw, &flat); err == nil {
		return len(flat)
	}
	// 形态未知时保守视为存在未匹配项
	return 1
}

// groupEpisodesByShow 按剧集分组为 Trakt 嵌套结构。
func groupEpisodesByShow(episodes []HistoryEpisode) []map[string]any {
	showOrder := []int64{}
	byShow := map[int64]map[int][]HistoryEpisode{}
	for _, ep := range episodes {
		if _, ok := byShow[ep.ShowTMDBID]; !ok {
			showOrder = append(showOrder, ep.ShowTMDBID)
			byShow[ep.ShowTMDBID] = map[int][]HistoryEpisode{}
		}
		byShow[ep.ShowTMDBID][ep.Season] = append(byShow[ep.ShowTMDBID][ep.Season], ep)
	}
	out := make([]map[string]any, 0, len(showOrder))
	for _, showID := range showOrder {
		seasonNums := []int{}
		for season := range byShow[showID] {
			seasonNums = append(seasonNums, season)
		}
		sortInts(seasonNums)
		seasons := make([]map[string]any, 0, len(seasonNums))
		for _, season := range seasonNums {
			eps := byShow[showID][season]
			epList := make([]map[string]any, 0, len(eps))
			for _, ep := range eps {
				entry := map[string]any{"number": ep.Episode}
				if !ep.WatchedAt.IsZero() {
					entry["watched_at"] = ep.WatchedAt.UTC().Format("2006-01-02T15:04:05.000Z")
				}
				epList = append(epList, entry)
			}
			seasons = append(seasons, map[string]any{"number": season, "episodes": epList})
		}
		out = append(out, map[string]any{
			"ids":     map[string]any{"tmdb": showID},
			"seasons": seasons,
		})
	}
	return out
}

func sortInts(nums []int) {
	for i := 1; i < len(nums); i++ {
		for j := i; j > 0 && nums[j] < nums[j-1]; j-- {
			nums[j], nums[j-1] = nums[j-1], nums[j]
		}
	}
}

// ── 用户与连通性 ────────────────────────────────────────────

// LastActivities 拉取观看数据的最后活动时间（unix 秒）。
// 返回 movies 与 shows（含集，取 shows/episodes 两者的最大值）两个时间戳，
// 官方用途：同步前比较，避免拉取尚未更新的数据。
func (c *Client) LastActivities() (movies, shows int64, err error) {
	var raw struct {
		All    string `json:"all"`
		Movies struct {
			WatchedAt string `json:"watched_at"`
		} `json:"movies"`
		Shows struct {
			WatchedAt string `json:"watched_at"`
		} `json:"shows"`
		Episodes struct {
			WatchedAt string `json:"watched_at"`
		} `json:"episodes"`
	}
	if err := c.getAuthed("/sync/last_activities", &raw); err != nil {
		return 0, 0, err
	}
	p := func(s string) int64 {
		if t, e := time.Parse(time.RFC3339, s); e == nil {
			return t.Unix()
		}
		return 0
	}
	movies = p(raw.Movies.WatchedAt)
	shows = max64i(p(raw.Shows.WatchedAt), p(raw.Episodes.WatchedAt))
	return movies, shows, nil
}

func max64i(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// Me 验证 token 有效性并返回当前用户名。
func (c *Client) Me() (string, error) {
	var raw struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if err := c.getAuthed("/users/me", &raw); err != nil {
		return "", err
	}
	if raw.Username != "" {
		return raw.Username, nil
	}
	return raw.Name, nil
}

// ── 请求核心 ────────────────────────────────────────────────

func (c *Client) base() string {
	if c.testingAPIBase != "" {
		return c.testingAPIBase
	}
	return defaultAPIBase
}

// authBase OAuth 端点基础地址（设备码认证、token 交换与刷新）。
func (c *Client) authBase() string {
	if c.testingAuthBase != "" {
		return c.testingAuthBase
	}
	return defaultAuthBase
}

// throttle 最小间隔节拍（单桶预约，与 u115.sleepRateLimit 同思路的简化版）。
func (c *Client) throttle() {
	c.rateMu.Lock()
	now := time.Now()
	reserveAt := now
	if c.nextAt.After(reserveAt) {
		reserveAt = c.nextAt
	}
	wait := reserveAt.Sub(now)
	c.nextAt = reserveAt.Add(minInterval)
	c.rateMu.Unlock()
	if wait > 0 {
		time.Sleep(wait)
	}
}

func (c *Client) enterCooldown(retryAfter time.Duration) {
	c.rateMu.Lock()
	until := time.Now().Add(retryAfter)
	if until.After(c.cooldown) {
		c.cooldown = until
	}
	if until.After(c.nextAt) {
		c.nextAt = until
	}
	c.rateMu.Unlock()
}

// do 执行一次请求（base 为目标主机，path 以 / 开头）。auth=true 时自动携带并按需刷新 Bearer token。
func (c *Client) do(method, base, path string, payload any, auth bool) ([]byte, int, http.Header, error) {
	c.rateMu.Lock()
	if wait := time.Until(c.cooldown); wait > 0 {
		c.rateMu.Unlock()
		return nil, 0, nil, fmt.Errorf("Trakt 接口冷却中，请 %d 秒后再试", int(wait.Seconds())+1)
	}
	c.rateMu.Unlock()
	c.throttle()

	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, nil, err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, base+path, body)
	if err != nil {
		return nil, 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	req.Header.Set("trakt-api-key", c.ClientID)
	if auth {
		token, err := c.GetValidAccessToken()
		if err != nil {
			return nil, 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, resp.Header, err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := 5 * time.Second
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if sec, perr := time.ParseDuration(ra + "s"); perr == nil && sec > 0 {
				retryAfter = sec
			}
		}
		c.enterCooldown(retryAfter)
		return nil, resp.StatusCode, resp.Header, fmt.Errorf("Trakt 接口返回 429，进入冷却")
	}
	return data, resp.StatusCode, resp.Header, nil
}

func (c *Client) postUnauthed(path string, payload any, out any) error {
	data, status, _, err := c.do(http.MethodPost, c.authBase(), path, payload, false)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Trakt %s 失败：HTTP %d（%s）", path, status, truncateBody(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// postRaw 同 postUnauthed，但 4xx 也尝试解析 JSON 体（设备码轮询用 error 字段传状态）。
func (c *Client) postRaw(path string, payload any, out any) error {
	data, _, _, err := c.do(http.MethodPost, c.authBase(), path, payload, false)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("Trakt %s 返回空响应", path)
	}
	return json.Unmarshal(data, out)
}

func (c *Client) postAuthed(path string, payload any, out any) error {
	data, status, _, err := c.do(http.MethodPost, c.base(), path, payload, true)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Trakt %s 失败：HTTP %d（%s）", path, status, truncateBody(data))
	}
	if out == nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) getAuthed(path string, out any) error {
	data, status, _, err := c.do(http.MethodGet, c.base(), path, nil, true)
	if err != nil {
		return err
	}
	if status >= 400 {
		return fmt.Errorf("Trakt %s 失败：HTTP %d（%s）", path, status, truncateBody(data))
	}
	return json.Unmarshal(data, out)
}

// getWithPagination GET 并解析 X-Pagination-* 头，返回响应体与总页数。
func (c *Client) getWithPagination(path string) ([]byte, int, error) {
	data, status, header, err := c.do(http.MethodGet, c.base(), path, nil, true)
	if err != nil {
		return nil, 0, err
	}
	if status >= 400 {
		return nil, 0, fmt.Errorf("Trakt %s 失败：HTTP %d（%s）", path, status, truncateBody(data))
	}
	pageCount := 1
	if v := header.Get("X-Pagination-Page-Count"); v != "" {
		if n, perr := fmt.Sscanf(v, "%d", &pageCount); perr != nil || n != 1 {
			pageCount = 1
		}
	}
	// 服务端可能不按请求的 limit 返回（上限 250），以响应页数为准翻页。
	return data, pageCount, nil
}

func truncateBody(data []byte) string {
	if len(data) > 200 {
		return string(data[:200]) + "..."
	}
	return string(data)
}

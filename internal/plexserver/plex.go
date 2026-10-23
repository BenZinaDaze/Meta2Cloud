// Package plexserver 是 Plex Media Server 独立接口层：
// PIN 配对（经 plex.tv）、库枚举（分页）、观看标记（scrobble）、会话查询。
// 所有服务端操作直连 Plex base_url，plex.tv 仅在 PIN 配对阶段使用。
package plexserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPlexTVBase = "https://plex.tv"
	defaultAuthPortal = "https://app.plex.tv/auth"
	// scrobbleIdentifier Plex 官方库操作固定标识。
	scrobbleIdentifier = "com.plexapp.plugins.library"
	// pageSize 库枚举分页大小（配合 X-Plex-Container-Start/Size）。
	pageSize = 200
)

// Client Plex Media Server 客户端。
type Client struct {
	// BaseURL Plex 服务器地址（如 http://192.168.1.10:32400）。
	BaseURL string
	// PlexTVBase plex.tv 地址（测试可覆盖）。
	PlexTVBase string
	// Token X-Plex-Token（PIN 配对获得，账号级长期有效）。
	Token string
	// ClientIdentifier 客户端标识（配对态，首次生成后持久化）。
	ClientIdentifier string

	Product string
	Version string

	HTTP *http.Client
}

// Option 客户端可选项。
type Option func(*Client)

// WithHTTPClient 注入自定义 http.Client（测试用）。
func WithHTTPClient(hc *http.Client) Option { return func(c *Client) { c.HTTP = hc } }

// WithTimeout 设置 HTTP 超时（秒）。
func WithTimeout(seconds int) Option {
	return func(c *Client) { c.HTTP.Timeout = time.Duration(seconds) * time.Second }
}

// NewClient 构造客户端。
func NewClient(baseURL, token, clientIdentifier string, opts ...Option) *Client {
	c := &Client{
		BaseURL:          strings.TrimRight(baseURL, "/"),
		PlexTVBase:       defaultPlexTVBase,
		Token:            token,
		ClientIdentifier: clientIdentifier,
		Product:          "Meta2Cloud",
		Version:          "1.0",
		HTTP:             &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ── 数据模型 ────────────────────────────────────────────────

// Section 媒体库分区。
type Section struct {
	Key   string `json:"key"`
	Type  string `json:"type"` // movie | show | artist | photo
	Title string `json:"title"`
}

// GuidEntry 外部 GUID（如 tmdb://123、imdb://tt...）。
type GuidEntry struct {
	ID string `json:"id"`
}

// MediaInfo 媒体文件信息（取 Part.file 用于路径兜底解析）。
type MediaInfo struct {
	Part []PartInfo `json:"Part"`
}

// PartInfo 文件分片。
type PartInfo struct {
	File string `json:"file"`
}

// Metadata Plex 条目（电影/集/会话共用；字段缺席=零值）。
type Metadata struct {
	RatingKey        string      `json:"ratingKey"`
	Key              string      `json:"key"`  // 如 /library/metadata/{show}/children/...（集条目）
	Type             string      `json:"type"` // movie | episode | show ...
	Title            string      `json:"title"`
	Year             int         `json:"year"`
	Index            int         `json:"index"`            // 集号（episode）
	ParentIndex      int         `json:"parentIndex"`      // 季号（episode 的上级季）
	GrandparentTitle string      `json:"grandparentTitle"` // 剧名（episode）
	Guid             string      `json:"guid"`
	GuidList         []GuidEntry `json:"Guid"`
	Media            []MediaInfo `json:"Media"`

	ViewCount    int `json:"viewCount"`
	LastViewedAt int `json:"lastViewedAt"`

	// 会话字段
	SessionKey string `json:"sessionKey"`
	ViewOffset int    `json:"viewOffset"`
	Duration   int    `json:"duration"`
}

// TMDBGuid 从 Guid 列表提取 tmdb://id；无则返回 0。
func (m *Metadata) TMDBGuid() int64 {
	for _, g := range m.GuidList {
		if id, ok := parseTMDBGuid(g.ID); ok {
			return id
		}
	}
	// 旧代理把外部 id 拼进主 guid（如 "com.plexapp.agents.imdb://tt1375666?lang=en" 亦可能是 tmdb://）
	if id, ok := parseTMDBGuid(m.Guid); ok {
		return id
	}
	return 0
}

func parseTMDBGuid(guid string) (int64, bool) {
	idx := strings.Index(guid, "tmdb://")
	if idx < 0 {
		return 0, false
	}
	rest := guid[idx+len("tmdb://"):]
	if end := strings.IndexAny(rest, "?/"); end >= 0 {
		rest = rest[:end]
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// FilePath 首个媒体文件路径。
func (m *Metadata) FilePath() string {
	for _, media := range m.Media {
		for _, part := range media.Part {
			if part.File != "" {
				return part.File
			}
		}
	}
	return ""
}

// IdentityResponse GET /identity 响应。
type IdentityResponse struct {
	MediaContainer struct {
		MachineIdentifier string `json:"machineIdentifier"`
		Version           string `json:"version"`
	} `json:"MediaContainer"`
}

type mediaContainer struct {
	Metadata  []Metadata `json:"Metadata"`
	Directory []Section  `json:"Directory"`
	TotalSize int        `json:"totalSize"`
}

// ── PIN 配对 ────────────────────────────────────────────────

// PIN 一次配对请求。
type PIN struct {
	ID        int    `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
}

// CreatePIN 发起 PIN 配对，返回 PIN 与用户授权链接。
func (c *Client) CreatePIN() (*PIN, string, error) {
	var pin PIN
	if err := c.plexTV(http.MethodPost, "/api/v2/pins?strong=true", &pin); err != nil {
		return nil, "", err
	}
	if pin.ID == 0 || pin.Code == "" {
		return nil, "", fmt.Errorf("plex.tv PIN 响应不完整")
	}
	authURL := fmt.Sprintf("%s#?clientID=%s&code=%s", defaultAuthPortal, url.QueryEscape(c.ClientIdentifier), url.QueryEscape(pin.Code))
	return &pin, authURL, nil
}

// PollPIN 轮询 PIN；authToken 非空表示已完成授权。
func (c *Client) PollPIN(pinID int) (*PIN, error) {
	var pin PIN
	if err := c.plexTV(http.MethodGet, "/api/v2/pins/"+strconv.Itoa(pinID), &pin); err != nil {
		return nil, err
	}
	return &pin, nil
}

// plexTV 调用 plex.tv v2 接口（JSON）。
func (c *Client) plexTV(method, path string, out any) error {
	req, err := http.NewRequest(method, c.PlexTVBase+path, nil)
	if err != nil {
		return err
	}
	c.setClientHeaders(req)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		// 与服务端/SSE 分支一致：401 归类为授权失效（errors.Is 可识别）
		return ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("plex.tv %s 失败：HTTP %d", path, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) setClientHeaders(req *http.Request) {
	req.Header.Set("X-Plex-Client-Identifier", c.ClientIdentifier)
	req.Header.Set("X-Plex-Product", c.Product)
	req.Header.Set("X-Plex-Version", c.Version)
}

// ── 服务端接口 ──────────────────────────────────────────────

// ErrUnauthorized Plex token 失效（服务端返回 401）。
// 调用方用 errors.Is 识别授权失效，与健康监测等上层逻辑解耦。
var ErrUnauthorized = errors.New("Plex 认证失败（401），请重新授权")

// get 调用 Plex 服务器（自动携带 X-Plex-Token）。
func (c *Client) get(path string, query url.Values) ([]byte, http.Header, error) {
	return c.do(http.MethodGet, path, query)
}

// post 调用 Plex 服务器写接口。
func (c *Client) post(path string, query url.Values) ([]byte, http.Header, error) {
	return c.do(http.MethodPost, path, query)
}

// do 发起对 Plex 服务器的请求（自动携带 X-Plex-Token）。
func (c *Client) do(method, path string, query url.Values) ([]byte, http.Header, error) {
	target := c.BaseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("X-Plex-Token", c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode == 401 {
		return nil, nil, ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("Plex %s 失败：HTTP %d", path, resp.StatusCode)
	}
	return data, resp.Header, nil
}

// Identity 验证连通性与 token，返回服务器信息。
func (c *Client) Identity() (*IdentityResponse, error) {
	data, _, err := c.get("/identity", nil)
	if err != nil {
		return nil, err
	}
	var out IdentityResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Sections 列出电影/剧集分区（其他类型过滤掉）。
func (c *Client) Sections() ([]Section, error) {
	data, _, err := c.get("/library/sections", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		MediaContainer mediaContainer `json:"MediaContainer"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	sections := make([]Section, 0, len(out.MediaContainer.Directory))
	for _, dir := range out.MediaContainer.Directory {
		if dir.Type == "movie" || dir.Type == "show" {
			sections = append(sections, dir)
		}
	}
	return sections, nil
}

// RefreshSection 通知 PMS 异步扫描媒体库（key 为 Sections 返回的分区 key）。
func (c *Client) RefreshSection(key string) error {
	_, _, err := c.post("/library/sections/"+key+"/refresh", nil)
	return err
}

// LibraryItems 分页枚举分区条目。plexType：1=电影，4=集（剧集叶子）。
// 带稳定排序（addedAt:asc），避免分页窗口漂移造成重复/遗漏。
func (c *Client) LibraryItems(sectionKey string, plexType int) ([]Metadata, error) {
	var out []Metadata
	for start := 0; ; start += pageSize {
		query := url.Values{}
		query.Set("type", strconv.Itoa(plexType))
		query.Set("sort", "addedAt:asc")
		query.Set("X-Plex-Container-Start", strconv.Itoa(start))
		query.Set("X-Plex-Container-Size", strconv.Itoa(pageSize))
		data, header, err := c.get("/library/sections/"+sectionKey+"/all", query)
		if err != nil {
			return nil, err
		}
		var page struct {
			MediaContainer mediaContainer `json:"MediaContainer"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, err
		}
		out = append(out, page.MediaContainer.Metadata...)
		// 响应不足一页（或 totalSize 达到）即收尾；容器头优先，body 兜底
		total := -1
		if v := header.Get("X-Plex-Container-Total-Size"); v != "" {
			if n, perr := strconv.Atoi(v); perr == nil {
				total = n
			}
		}
		if total < 0 {
			total = page.MediaContainer.TotalSize
		}
		if len(page.MediaContainer.Metadata) < pageSize || start+len(page.MediaContainer.Metadata) >= total {
			return out, nil
		}
	}
}

// Metadata 按 ratingKey 查详情（Guid/文件路径齐全，用于会话身份补解析）。
func (c *Client) Metadata(ratingKey string) (*Metadata, error) {
	data, _, err := c.get("/library/metadata/"+ratingKey, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		MediaContainer mediaContainer `json:"MediaContainer"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if len(out.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("Plex 条目不存在：%s", ratingKey)
	}
	return &out.MediaContainer.Metadata[0], nil
}

// scrobble 调用 /:/{scrobble|unscrobble}。
func (c *Client) scrobble(action, ratingKey string) error {
	query := url.Values{}
	query.Set("identifier", scrobbleIdentifier)
	query.Set("key", ratingKey)
	_, _, err := c.get("/:/"+action, query)
	return err
}

// Scrobble 标记已观看。
func (c *Client) Scrobble(ratingKey string) error { return c.scrobble("scrobble", ratingKey) }

// Unscrobble 取消已观看。
func (c *Client) Unscrobble(ratingKey string) error { return c.scrobble("unscrobble", ratingKey) }

// Sessions 当前进行中的播放会话。
func (c *Client) Sessions() ([]Metadata, error) {
	data, _, err := c.get("/status/sessions", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		MediaContainer mediaContainer `json:"MediaContainer"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out.MediaContainer.Metadata, nil
}

// ── 实时播放通知（SSE）──────────────────────────────────────

// ErrNotificationsUnsupported 该 Plex 版本未提供 SSE 通知端点（HTTP 404）。
var ErrNotificationsUnsupported = errors.New("此 Plex 版本不支持实时通知流（SSE 端点不存在）")

// PlaybackNotification 播放会话实时通知（/:/eventsource/notifications）。
type PlaybackNotification struct {
	State        string // playing | paused | buffering | stopped
	SessionKey   string
	RatingKey    string
	ViewOffsetMS int64
	DurationMS   int64
}

// StreamNotifications 订阅 Plex 实时通知流（SSE，阻塞调用）：
// 连接建立（HTTP 200）后回调一次 onOpen，之后每条播放会话通知回调一次 onEvent，
// 直至 ctx 取消或连接出错；断线重连由调用方负责。
// 注意使用独立的无超时 http.Client——常规 client 的 Timeout 会掐断长连接，
// 生命周期由 ctx 控制。filters=playing 只订阅播放事件，减少无关流量。
func (c *Client) StreamNotifications(ctx context.Context, onOpen func(), onEvent func(PlaybackNotification)) error {
	transport := c.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	streamClient := &http.Client{Transport: transport}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/:/eventsource/notifications?filters=playing", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.Token != "" {
		req.Header.Set("X-Plex-Token", c.Token)
	}
	req.Header.Set("X-Plex-Client-Identifier", c.ClientIdentifier)
	resp, err := streamClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotificationsUnsupported
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("Plex 通知流订阅失败：HTTP %d", resp.StatusCode)
	}
	if onOpen != nil {
		onOpen()
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var dataLines []string
	dispatch := func() {
		if len(dataLines) == 0 {
			return
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = nil
		for _, n := range parsePlaybackNotification(payload) {
			onEvent(n)
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		case line == "":
			dispatch()
		}
	}
	dispatch()
	return scanner.Err()
}

// parsePlaybackNotification 解析一条 SSE data（Plex 通知容器），
// 仅提取播放会话状态（type == "playing"；stopped 等状态也在此类型下）。
func parsePlaybackNotification(payload string) []PlaybackNotification {
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.UseNumber()
	var raw struct {
		Type                         string           `json:"type"`
		PlaySessionStateNotification []map[string]any `json:"PlaySessionStateNotification"`
	}
	if err := dec.Decode(&raw); err != nil || raw.Type != "playing" {
		return nil
	}
	out := make([]PlaybackNotification, 0, len(raw.PlaySessionStateNotification))
	for _, item := range raw.PlaySessionStateNotification {
		n := PlaybackNotification{
			State:        jsonStr(item["state"]),
			SessionKey:   jsonStr(item["sessionKey"]),
			RatingKey:    jsonStr(item["ratingKey"]),
			ViewOffsetMS: jsonInt(item["viewOffset"]),
			DurationMS:   jsonInt(item["duration"]),
		}
		if n.SessionKey == "" && n.RatingKey == "" {
			continue
		}
		out = append(out, n)
	}
	return out
}

// jsonStr / jsonInt Plex 通知字段兼容：不同版本可能给字符串或数字。
func jsonStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	}
	return ""
}

func jsonInt(v any) int64 {
	switch t := v.(type) {
	case string:
		n, _ := strconv.ParseInt(t, 10, 64)
		return n
	case json.Number:
		n, _ := t.Int64()
		return n
	case float64:
		return int64(t)
	}
	return 0
}

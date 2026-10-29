// strmhttp.go —— STRM 302 直链跳转服务：
// 媒体服务器/播放器访问 .strm 内容里的地址时，根据 pick_code 实时换取 115 下载直链并 302 跳转。
//
// 关键约束（参考 p115strmhelper 的实现）：
//   - 115 直链与请求时的 User-Agent 绑定，必须把播放器的 UA 透传给 115；
//   - 直链有效期由返回 URL 的 t 参数（unix 时间戳）决定，按 t-5min 缓存（key = pick_code + UA）；
//   - pick_code 本身永久有效（文件移动/改名不变），直链过期后按需重新获取即可。
//
// 路由注册在非 /api 前缀（/strm/redirect），天然豁免 JWT（播放器无法携带请求头）；
// 防滥用通过可选的 strm.url_secret 签名（HMAC 语义的 sha256 截断，常量时间比对）。
package webui

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"meta2cloud/internal/u115"
)

// strmRedirectPath 302 跳转路由（GET/HEAD/POST 均支持，媒体服务器行为不一）。
const strmRedirectPath = "/strm/redirect"

// strmURLExpiryMargin 直链缓存安全边际：t 参数到期前 5 分钟即视为过期。
const strmURLExpiryMargin = 5 * time.Minute

// pickCodeRe 115 pick_code：17 位字母数字。
var pickCodeRe = regexp.MustCompile(`^[a-zA-Z0-9]{17}$`)

// ── 直链获取限流（缓存命中不经过此层） ──────────────────────
// 媒体服务器刮削/扫描会对 302 端点高频探测，三层防护：
//  1. 单飞双检：同 pickcode+UA 并发只有一个真正请求 115
//  2. 失败负缓存：同一 pickcode 短时间内的重复失败直接返回，不打 115
//  3. 全局并发上限 + 排队超时：超出时快速 503，避免全库扫描风暴拖垮播放

const (
	strmFetchConcurrency = 2                // 同时处于「获取直链中」的最大请求数
	strmFetchWaitTimeout = 3 * time.Second  // 排队等待上限，超时快速失败
	strmNegativeCacheTTL = 30 * time.Second // 失败负缓存时长
)

// negativeEntry 负缓存条目：短时间内的重复失败直接按同状态返回。
type negativeEntry struct {
	status int
	msg    string
	until  time.Time
}

func (s *Server) strmNegativeGet(pickCode string) (negativeEntry, bool) {
	svc := s.rt.strmSvc
	svc.strmNegMu.Lock()
	defer svc.strmNegMu.Unlock()
	e, ok := svc.strmNeg[pickCode]
	if ok && time.Now().After(e.until) {
		delete(svc.strmNeg, pickCode)
		return negativeEntry{}, false
	}
	return e, ok
}

func (s *Server) strmNegativeSet(pickCode string, status int, msg string) {
	svc := s.rt.strmSvc
	svc.strmNegMu.Lock()
	defer svc.strmNegMu.Unlock()
	if len(svc.strmNeg) > 1024 {
		now := time.Now()
		for k, v := range svc.strmNeg {
			if now.After(v.until) {
				delete(svc.strmNeg, k)
			}
		}
	}
	svc.strmNeg[pickCode] = negativeEntry{status: status, msg: msg, until: time.Now().Add(svc.strmNegTTL)}
}

// resetStrmLimiter 测试用：清空直链缓存、负缓存与信号量。
func (rt *Runtime) resetStrmLimiter() {
	svc := rt.strmSvc
	svc.strmLinkCache.clear()
	svc.strmNegMu.Lock()
	svc.strmNeg = map[string]negativeEntry{}
	svc.strmNegMu.Unlock()
	svc.strmFetchSem = make(chan struct{}, strmFetchConcurrency)
}

// directLink 缓存的直链条目。
type directLink struct {
	url      string
	fileName string
	expires  time.Time
}

// directLinkCache 直链缓存（key = pick_code + "\x00" + UA）。
// 按 UA 区分缓存，避免不同播放器 UA 不同导致的直链失效。
type directLinkCache struct {
	mu      sync.Mutex
	entries map[string]directLink
}

func (c *directLinkCache) get(pickCode, ua string) (directLink, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	link, ok := c.entries[pickCode+"\x00"+ua]
	if !ok || time.Now().After(link.expires) {
		if ok {
			delete(c.entries, pickCode+"\x00"+ua)
		}
		return directLink{}, false
	}
	return link, true
}

func (c *directLinkCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]directLink{}
}

func (c *directLinkCache) set(pickCode, ua, rawURL string, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// 防止无限增长：过期项超过 2048 时整体清理一轮
	if len(c.entries) > 2048 {
		now := time.Now()
		for k, v := range c.entries {
			if now.After(v.expires) {
				delete(c.entries, k)
			}
		}
	}
	c.entries[pickCode+"\x00"+ua] = directLink{url: rawURL, fileName: urlFileName(rawURL), expires: expires}
}

// urlFileName 从直链路径提取文件名（115 直链路径含 URL 编码的原始文件名）。
func urlFileName(rawURL string) string {
	i := strings.LastIndexByte(rawURL, '/')
	if i < 0 {
		return ""
	}
	rest := rawURL[i+1:]
	if j := strings.IndexByte(rest, '?'); j >= 0 {
		rest = rest[:j]
	}
	if unescaped, err := url.PathUnescape(rest); err == nil {
		return unescaped
	}
	return rest
}

// ── HTTP 处理器 ─────────────────────────────────────────────

func (s *Server) handleStrmRedirect(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	if method := r.Method; method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "仅支持 GET/HEAD/POST")
		return
	}
	query := r.URL.Query()
	pickCode := strings.ToLower(strings.TrimSpace(query.Get("pickcode")))
	if !pickCodeRe.MatchString(pickCode) {
		// 无效参数非播放行为，不记录
		writeJSONError(w, http.StatusBadRequest, "无效的 pickcode")
		return
	}

	// 播放记录公共字段（UA 截断防御超长值）
	rawUA := r.Header.Get("User-Agent")
	if len(rawUA) > 512 {
		rawUA = rawUA[:512]
	}
	recordPlayback := func(cacheHit bool, status int, errMsg string, fileName string) {
		go func() {
			defer func() { _ = recover() }()
			st := s.rt.GetStrmStore()
			var nameArg, uaArg *string
			if fileName != "" {
				nameArg = &fileName
			}
			if rawUA != "" {
				uaArg = &rawUA
			}
			st.InsertPlaybackLog(pickCode, nameArg, uaArg, cacheHit,
				int(time.Since(start).Milliseconds()), status, errMsg)
		}()
	}

	// 可选防滥用签名
	cfg := s.rt.GetConfig()
	if cfg.Strm.URLSecret != "" {
		sign := query.Get("sign")
		expect := StrmSign(cfg.Strm.URLSecret, pickCode)
		if sign == "" || subtle.ConstantTimeCompare([]byte(sign), []byte(expect)) != 1 {
			recordPlayback(false, http.StatusForbidden, "签名无效", query.Get("file_name"))
			writeJSONError(w, http.StatusForbidden, "签名无效")
			return
		}
	}

	// UA 必须透传：115 直链与 UA 绑定
	ua := r.Header.Get("User-Agent")

	if link, ok := s.rt.strmSvc.strmLinkCache.get(pickCode, ua); ok {
		s.strmRedirect(w, r, link)
		recordPlayback(true, http.StatusFound, "", link.fileName)
		return
	}

	// 失败负缓存：同一 pickcode 短时间内重复失败直接返回，不打 115
	if neg, ok := s.strmNegativeGet(pickCode); ok {
		w.Header().Set("Retry-After", "30")
		recordPlayback(false, neg.status, neg.msg+"（负缓存）", query.Get("file_name"))
		writeJSONError(w, neg.status, neg.msg+"（近期失败，请稍后重试）")
		return
	}

	client := s.rt.GetU115Client()
	if client == nil {
		recordPlayback(false, http.StatusServiceUnavailable, "115 客户端不可用", query.Get("file_name"))
		writeJSONError(w, http.StatusServiceUnavailable, "115 客户端不可用，请先完成授权")
		return
	}

	// 全局并发上限 + 排队超时：刮削扫描风暴时快速失败，不堆积
	select {
	case s.rt.strmSvc.strmFetchSem <- struct{}{}:
		defer func() { <-s.rt.strmSvc.strmFetchSem }()
		// 双检：排队期间同 pickcode+UA 可能已被其他请求取到
		if link, ok := s.rt.strmSvc.strmLinkCache.get(pickCode, ua); ok {
			s.strmRedirect(w, r, link)
			recordPlayback(true, http.StatusFound, "", link.fileName)
			return
		}
		rawURL, err := client.GetDownloadURLWithUA(pickCode, ua)
		if err != nil {
			status, detail := s.strmUpstreamError(err)
			s.strmNegativeSet(pickCode, status, detail)
			recordPlayback(false, status, detail, query.Get("file_name"))
			s.writeStrmUpstreamError(w, err)
			return
		}
		expires := directURLExpiry(rawURL)
		s.rt.strmSvc.strmLinkCache.set(pickCode, ua, rawURL, expires)

		fileName := query.Get("file_name")
		if fileName == "" {
			fileName = urlFileName(rawURL)
		}
		s.strmRedirectWithURL(w, r, rawURL, fileName)
		recordPlayback(false, http.StatusFound, "", fileName)
		return

	case <-time.After(s.rt.strmSvc.strmFetchWait):
		w.Header().Set("Retry-After", "10")
		recordPlayback(false, http.StatusServiceUnavailable, "直链获取排队超时", query.Get("file_name"))
		writeJSONError(w, http.StatusServiceUnavailable, "直链获取排队超时（扫描限流中），请稍后重试")
	}
}

// strmUpstreamError 将 115 错误映射为 HTTP 状态码与简述。
func (s *Server) strmUpstreamError(err error) (int, string) {
	switch {
	case u115.IsRateLimit(err):
		return http.StatusServiceUnavailable, "115 限流/冷却"
	case u115.IsAuth(err):
		return http.StatusBadGateway, "115 授权失效"
	default:
		return http.StatusBadGateway, "获取直链失败"
	}
}

func (s *Server) handleStrmPlaybackHistory(w http.ResponseWriter, r *http.Request) {
	st := s.rt.GetStrmStore()
	logs, total := st.ListPlaybackLogs(s.queryIntDefault(r, "limit", 50), s.queryIntDefault(r, "offset", 0))
	items := make([]map[string]any, 0, len(logs))
	for _, l := range logs {
		items = append(items, map[string]any{
			"id": l.ID, "played_at": l.PlayedAt, "pick_code": l.PickCode,
			"file_name": l.FileName, "user_agent": l.UserAgent,
			"cache_hit": l.CacheHit, "duration_ms": l.DurationMs,
			"status": l.Status, "error": l.Error,
			"pan_path": l.PanPath, "local_path": l.LocalPath,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "items": items, "stats": st.GetPlaybackStats(),
	})
}

func (s *Server) strmRedirect(w http.ResponseWriter, r *http.Request, link directLink) {
	s.strmRedirectWithURL(w, r, link.url, link.fileName)
}

func (s *Server) strmRedirectWithURL(w http.ResponseWriter, r *http.Request, rawURL, fileName string) {
	if fileName != "" {
		w.Header().Set("Content-Disposition", contentDisposition(fileName))
	}
	// 302 按请求 UA 生成，禁止 CDN/代理缓存（缓存会让后续不同 UA 的播放器拿到失效直链）
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", rawURL)
	w.WriteHeader(http.StatusFound)
}

// contentDisposition 非 ASCII 文件名使用 RFC 5987 filename* 编码。
func contentDisposition(fileName string) string {
	if isASCII(fileName) {
		return fmt.Sprintf("attachment; filename=%q", fileName)
	}
	return "attachment; filename*=UTF-8''" + url.PathEscape(fileName)
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7f {
			return false
		}
	}
	return true
}

// directURLExpiry 从直链 query 的 t 参数（unix 秒）解析过期时间，解析失败默认 30 分钟。
func directURLExpiry(rawURL string) time.Time {
	if u, err := url.Parse(rawURL); err == nil {
		if t, err := strconv.ParseInt(u.Query().Get("t"), 10, 64); err == nil && t > 0 {
			return time.Unix(t, 0).Add(-strmURLExpiryMargin)
		}
	}
	return time.Now().Add(30 * time.Minute)
}

func (s *Server) writeStrmUpstreamError(w http.ResponseWriter, err error) {
	switch {
	case u115.IsRateLimit(err):
		w.Header().Set("Retry-After", "60")
		writeJSONError(w, http.StatusServiceUnavailable, "115 接口限流/冷却中："+err.Error())
	case u115.IsAuth(err):
		writeJSONError(w, http.StatusBadGateway, "115 授权失效："+err.Error())
	default:
		writeJSONError(w, http.StatusBadGateway, "获取 115 直链失败："+err.Error())
	}
}

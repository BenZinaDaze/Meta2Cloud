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

var strmLinkCache = &directLinkCache{entries: map[string]directLink{}}

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

func handleStrmRedirect(w http.ResponseWriter, r *http.Request) {
	if method := r.Method; method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "仅支持 GET/HEAD/POST")
		return
	}
	query := r.URL.Query()
	pickCode := strings.ToLower(strings.TrimSpace(query.Get("pickcode")))
	if !pickCodeRe.MatchString(pickCode) {
		writeJSONError(w, http.StatusBadRequest, "无效的 pickcode")
		return
	}

	// 可选防滥用签名
	cfg := RT().GetConfig()
	if cfg.Strm.URLSecret != "" {
		sign := query.Get("sign")
		expect := StrmSign(cfg.Strm.URLSecret, pickCode)
		if sign == "" || subtle.ConstantTimeCompare([]byte(sign), []byte(expect)) != 1 {
			writeJSONError(w, http.StatusForbidden, "签名无效")
			return
		}
	}

	// UA 必须透传：115 直链与 UA 绑定
	ua := r.Header.Get("User-Agent")

	if link, ok := strmLinkCache.get(pickCode, ua); ok {
		strmRedirect(w, r, link)
		return
	}

	client := RT().GetU115Client()
	if client == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "115 客户端不可用，请先完成授权")
		return
	}
	rawURL, err := client.GetDownloadURLWithUA(pickCode, ua)
	if err != nil {
		writeStrmUpstreamError(w, err)
		return
	}
	expires := directURLExpiry(rawURL)
	strmLinkCache.set(pickCode, ua, rawURL, expires)

	fileName := query.Get("file_name")
	if fileName == "" {
		fileName = urlFileName(rawURL)
	}
	strmRedirectWithURL(w, r, rawURL, fileName)
}

func strmRedirect(w http.ResponseWriter, r *http.Request, link directLink) {
	strmRedirectWithURL(w, r, link.url, link.fileName)
}

func strmRedirectWithURL(w http.ResponseWriter, r *http.Request, rawURL, fileName string) {
	if fileName != "" {
		w.Header().Set("Content-Disposition", contentDisposition(fileName))
	}
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

func writeStrmUpstreamError(w http.ResponseWriter, err error) {
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

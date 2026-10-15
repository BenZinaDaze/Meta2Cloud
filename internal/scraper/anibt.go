// anibt.go —— AniBT 资源站爬虫（对应 Python 版 anibt_spider.py）。
package scraper

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"meta2cloud/internal/jsonx"
)

const anibtBaseURL = "https://anibt.net"

const anibtUA = "Meta2Cloud/0.1 (+https://github.com/)"

var anibtTorrentHashRe = regexp.MustCompile(`/([0-9a-fA-F]{40})\.torrent(?:$|\?)`)

// AniBTSpider AniBT 爬虫。
type AniBTSpider struct {
	baseURL string
	hc      *http.Client
}

// NewAniBTSpider 构造。
func NewAniBTSpider() *AniBTSpider {
	return &AniBTSpider{
		baseURL: anibtBaseURL,
		hc:      &http.Client{Timeout: 15 * time.Second},
	}
}

// SiteID 站点标识。
func (s *AniBTSpider) SiteID() string { return "anibt" }

// SiteName 站点显示名。
func (s *AniBTSpider) SiteName() string { return "AniBT" }

// RssHosts RSS 域名后缀。
func (s *AniBTSpider) RssHosts() []string { return []string{"anibt.net"} }

// SetTestBaseURL 覆盖基础地址（仅测试使用）。
func (s *AniBTSpider) SetTestBaseURL(baseURL string) { s.baseURL = baseURL }

func (s *AniBTSpider) requestWithRetry(rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", anibtUA)
		req.Header.Set("Accept", "application/json, application/rss+xml, application/xml;q=0.9, */*;q=0.8")
		resp, err := s.hc.Do(req)
		if err == nil {
			if resp.StatusCode >= 400 {
				resp.Body.Close()
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, rawURL)
			} else {
				body, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr != nil {
					lastErr = readErr
				} else {
					return body, nil
				}
			}
		} else {
			lastErr = err
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	return nil, lastErr
}

func (s *AniBTSpider) apiGet(path string, params map[string]string) (any, error) {
	target := s.baseURL + path
	if len(params) > 0 {
		values := url.Values{}
		for k, v := range params {
			values.Set(k, v)
		}
		target += "?" + values.Encode()
	}
	body, err := s.requestWithRetry(target)
	if err != nil {
		return nil, err
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// itemsFromPayload 兼容多种响应结构：顶层数组或 data/items/results/animes/groups 键。
func itemsFromPayload(payload any) []map[string]any {
	switch v := payload.(type) {
	case []any:
		return mapSlice(v)
	case map[string]any:
		for _, key := range []string{"data", "items", "results", "animes", "groups"} {
			value, ok := v[key]
			if !ok {
				continue
			}
			if list, ok := value.([]any); ok {
				return mapSlice(list)
			}
			if nested, ok := value.(map[string]any); ok {
				if items := itemsFromPayload(nested); len(items) > 0 {
					return items
				}
			}
		}
	}
	return nil
}

func mapSlice(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if d, ok := item.(map[string]any); ok {
			out = append(out, d)
		}
	}
	return out
}

func pickTitle(item map[string]any) string {
	for _, key := range []string{"nameCn", "name_cn", "name", "titleCn", "title_cn", "title", "displayName"} {
		if v := jsonx.Str(item, key); v != "" {
			return v
		}
	}
	return ""
}

// idFromItem 依次取各 ID 字段，兼容字符串与数字两种 JSON 类型。
func idFromItem(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := jsonx.Str(item, key); v != "" {
			return v
		}
		if id := jsonx.Int(item, key, 0); id != 0 {
			return strconv.FormatInt(id, 10)
		}
	}
	return ""
}

// SearchMedia 搜索媒体（含字幕组条目）。
func (s *AniBTSpider) SearchMedia(keyword string) ([]MediaItem, error) {
	payload, err := s.apiGet("/api/bgm/search", map[string]string{"q": keyword, "limit": "5"})
	if err != nil {
		return nil, err
	}
	var items []MediaItem
	seen := map[string]bool{}
	for _, candidate := range itemsFromPayload(payload) {
		// 真实接口返回的 bgmId 是数字（如 634708），需兼容字符串与数字两种形式
		bgmID := idFromItem(candidate, "bgmId", "bangumiId", "id")
		if bgmID == "" || seen[bgmID] {
			continue
		}
		seen[bgmID] = true

		title := pickTitle(candidate)
		if title == "" {
			title = keyword
		}
		cover := firstNonEmpty3(jsonx.Str(candidate, "image"), jsonx.Str(candidate, "cover"), jsonx.Str(candidate, "poster"))
		detailURL := fmt.Sprintf("%s/anime/%s", s.baseURL, bgmID)

		items = append(items, MediaItem{
			MediaID:    bgmID,
			Name:       title + " (全部)",
			URL:        detailURL,
			CoverImage: cover,
			Site:       s.SiteID(),
			RssURL:     s.BuildRssURL(bgmID, ""),
		})
		for _, group := range s.getGroups(bgmID) {
			items = append(items, MediaItem{
				MediaID:      bgmID,
				Name:         fmt.Sprintf("%s [%s]", title, group.name),
				URL:          detailURL,
				CoverImage:   cover,
				Site:         s.SiteID(),
				SubgroupID:   group.slug,
				SubgroupName: group.name,
				RssURL:       s.BuildRssURL(bgmID, group.slug),
			})
		}
	}
	return items, nil
}

type anibtGroup struct{ slug, name string }

func (s *AniBTSpider) getGroups(bgmID string) []anibtGroup {
	payload, err := s.apiGet("/api/anime/groups", map[string]string{"bgmId": bgmID})
	if err != nil {
		return nil
	}
	var groups []anibtGroup
	seen := map[string]bool{}
	for _, item := range itemsFromPayload(payload) {
		group := item
		if nested := jsonx.Map(item, "group"); nested != nil {
			group = nested
		}
		slug := firstNonEmpty3(jsonx.Str(group, "slug"), jsonx.Str(group, "groupSlug"))
		name := firstNonEmpty3(jsonx.Str(group, "name"), jsonx.Str(group, "displayName"), slug)
		if slug == "" || name == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		groups = append(groups, anibtGroup{slug: slug, name: name})
	}
	return groups
}

// BuildRssURL 构造 RSS 订阅地址。
func (s *AniBTSpider) BuildRssURL(mediaID, subgroupID string) string {
	values := url.Values{"bgmId": {mediaID}}
	if subgroupID != "" {
		values.Set("groupSlug", subgroupID)
	}
	return fmt.Sprintf("%s/rss/anime.xml?%s", s.baseURL, values.Encode())
}

// collectItemExtTexts 按文档顺序收集每个 <item> 子树内全部后代元素的
// local 名 → 首个非空文本。encoding/xml 的 `,any` 只匹配直接子元素，
// 而 AniBT 把 infohash 嵌套在 <anibt:torrent> 容器内
// （对应 Python 版 item.iter() 的递归语义）。
func collectItemExtTexts(body []byte) []map[string]string {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var out []map[string]string
	var current map[string]string
	var stack []string
	depth := 0 // >0 表示位于某个 <item> 子树内；1 为 item 本身
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			if depth == 0 {
				if name == "item" {
					current = map[string]string{}
					depth = 1
				}
				continue
			}
			depth++
			stack = append(stack, name)
			if _, seen := current[name]; !seen {
				current[name] = "" // 占位，等待 chardata
			}
		case xml.CharData:
			if depth > 1 && len(stack) > 0 {
				tag := stack[len(stack)-1]
				if text := strings.TrimSpace(string(t)); text != "" && current[tag] == "" {
					current[tag] = text
				}
			}
		case xml.EndElement:
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 {
				out = append(out, current)
				current = nil
				stack = stack[:0]
				continue
			}
			stack = stack[:len(stack)-1]
		}
	}
	return out
}

// GetEpisodes 获取种子条目。
func (s *AniBTSpider) GetEpisodes(mediaID, subgroupID string) ([]MagnetItem, error) {
	body, err := s.requestWithRetry(s.BuildRssURL(mediaID, subgroupID))
	if err != nil {
		return nil, err
	}
	var rss struct {
		Channel struct {
			Items []rssItemXML `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(body, &rss); err != nil {
		return []MagnetItem{}, nil
	}
	extTexts := collectItemExtTexts(body)

	episodes := make([]MagnetItem, 0, len(rss.Channel.Items))
	for i, item := range rss.Channel.Items {
		var torrentURL, magnetURL, infohash string
		var fileSizeMB float64
		if item.Enclosure.URL != "" {
			if strings.HasPrefix(item.Enclosure.URL, "magnet:") {
				magnetURL = item.Enclosure.URL
			} else {
				torrentURL = item.Enclosure.URL
			}
		}
		if item.Enclosure.Length != "" && isDigits(item.Enclosure.Length) {
			var bytes int64
			fmt.Sscanf(item.Enclosure.Length, "%d", &bytes)
			fileSizeMB = float64(int64(float64(bytes)/1024/1024*100+0.5)) / 100
		}
		// 扩展标签（含嵌套元素）：magneturi / magnet / magnetlink / infohash / torrenturl
		var ext map[string]string
		if i < len(extTexts) {
			ext = extTexts[i]
		}
		for _, tag := range []string{"magneturi", "magnet", "magnetlink"} {
			if text := ext[tag]; strings.HasPrefix(text, "magnet:") {
				magnetURL = text
				break
			}
		}
		infohash = ext["infohash"]
		if torrentURL == "" {
			torrentURL = ext["torrenturl"]
		}
		if strings.HasPrefix(item.Link, "magnet:") && magnetURL == "" {
			magnetURL = item.Link
		}
		if infohash != "" && magnetURL == "" {
			magnetURL = "magnet:?xt=urn:btih:" + infohash
		}
		if torrentURL != "" && magnetURL == "" {
			if match := anibtTorrentHashRe.FindStringSubmatch(torrentURL); match != nil {
				magnetURL = "magnet:?xt=urn:btih:" + match[1]
			}
		}
		episodes = append(episodes, MagnetItem{
			Title:       item.Title,
			TorrentURL:  torrentURL,
			MagnetURL:   magnetURL,
			PublishTime: item.PubDate,
			FileSizeMB:  fileSizeMB,
			Site:        s.SiteID(),
		})
	}
	return episodes, nil
}

func firstNonEmpty3(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

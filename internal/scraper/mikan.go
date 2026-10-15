// mikan.go —— Mikan Project RSS 爬虫（对应 Python 版 mikan_spider.py）。
package scraper

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const mikanBaseURL = "https://mikan.tangbai.cc"

const mikanUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"

var bangumiIDRe = regexp.MustCompile(`/Home/Bangumi/(\d+)`)
var torrentHashRe = regexp.MustCompile(`/([0-9a-fA-F]{40})\.torrent$`)

// MikanSpider Mikan Project 爬虫。
type MikanSpider struct {
	baseURL string
	hc      *http.Client
}

// NewMikanSpider 构造。
func NewMikanSpider() *MikanSpider {
	return &MikanSpider{
		baseURL: mikanBaseURL,
		hc:      &http.Client{Timeout: 15 * time.Second},
	}
}

// SiteID 站点标识。
func (s *MikanSpider) SiteID() string { return "mikan" }

// SiteName 站点显示名。
func (s *MikanSpider) SiteName() string { return "Mikan" }

// RssHosts RSS 域名后缀。
func (s *MikanSpider) RssHosts() []string { return []string{"mikanani.me", "mikan.tangbai.cc"} }

// SetTestBaseURL 覆盖基础地址（仅测试使用）。
func (s *MikanSpider) SetTestBaseURL(baseURL string) { s.baseURL = baseURL }

func (s *MikanSpider) requestWithRetry(url string) (string, []byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return "", nil, err
		}
		req.Header.Set("User-Agent", mikanUA)
		resp, err := s.hc.Do(req)
		if err == nil {
			if resp.StatusCode >= 400 {
				resp.Body.Close()
				lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, url)
			} else {
				body := make([]byte, 0, 64*1024)
				buf := make([]byte, 32*1024)
				for {
					n, readErr := resp.Body.Read(buf)
					body = append(body, buf[:n]...)
					if readErr != nil {
						break
					}
				}
				resp.Body.Close()
				return string(body), body, nil
			}
		} else {
			lastErr = err
		}
		if attempt < 2 {
			time.Sleep(2 * time.Second)
		}
	}
	return "", nil, lastErr
}

// SearchMedia 搜索番剧（含字幕组条目）。
// 关键词必须百分号编码：mikan 服务端拒绝请求行中的原始非 ASCII 字节（HTTP 400）。
func (s *MikanSpider) SearchMedia(keyword string) ([]MediaItem, error) {
	text, _, err := s.requestWithRetry(s.baseURL + "/Home/Search?searchstr=" + url.QueryEscape(keyword))
	if err != nil {
		return nil, err
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(text))
	if err != nil {
		return nil, err
	}

	var items []MediaItem
	seen := map[string]bool{}
	doc.Find(`a[href^="/Home/Bangumi/"]`).Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		title := strings.TrimSpace(a.Text())
		if title == "" {
			title = strings.TrimSpace(a.AttrOr("title", ""))
		}
		if title == "" {
			return
		}
		match := bangumiIDRe.FindStringSubmatch(href)
		if match == nil {
			return
		}
		bgmID := match[1]
		if seen[bgmID] {
			return
		}
		seen[bgmID] = true

		// 先加"全部"条目
		items = append(items, MediaItem{
			MediaID: bgmID,
			Name:    title + " (全部)",
			URL:     s.baseURL + href,
			Site:    s.SiteID(),
			RssURL:  s.BuildRssURL(bgmID, ""),
		})
		// 该番专属的字幕组列表
		for _, sub := range s.getSubgroups(bgmID) {
			items = append(items, MediaItem{
				MediaID:      bgmID,
				Name:         fmt.Sprintf("%s [%s]", title, sub.name),
				URL:          s.baseURL + href,
				Site:         s.SiteID(),
				SubgroupID:   sub.id,
				SubgroupName: sub.name,
				RssURL:       s.BuildRssURL(bgmID, sub.id),
			})
		}
	})
	return items, nil
}

type mikanSubgroup struct{ id, name string }

// getSubgroups 从番剧详情页获取字幕组列表。
// 详情页字幕组链接形如 <a class="subgroup-name" data-anchor="#981">樱桃花字幕组</a>，
// PublishGroup ID 即为 subgroup_id，与 RSS URL 中的 subgroupid 参数一致。
func (s *MikanSpider) getSubgroups(bgmID string) []mikanSubgroup {
	text, _, err := s.requestWithRetry(s.baseURL + "/Home/Bangumi/" + bgmID)
	if err != nil {
		return nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(text))
	if err != nil {
		return nil
	}
	var subgroups []mikanSubgroup
	seen := map[string]bool{}
	doc.Find("a.subgroup-name[data-anchor]").Each(func(_ int, a *goquery.Selection) {
		subID := strings.TrimPrefix(a.AttrOr("data-anchor", ""), "#")
		subName := strings.TrimSpace(a.Text())
		if subID == "" || subName == "" || seen[subID] {
			return
		}
		seen[subID] = true
		subgroups = append(subgroups, mikanSubgroup{id: subID, name: subName})
	})
	return subgroups
}

// BuildRssURL 构造 RSS 订阅地址。
func (s *MikanSpider) BuildRssURL(mediaID, subgroupID string) string {
	rssURL := fmt.Sprintf("%s/RSS/Bangumi?bangumiId=%s", s.baseURL, mediaID)
	if subgroupID != "" {
		rssURL += "&subgroupid=" + subgroupID
	}
	return rssURL
}

// rssXML RSS XML 结构。
type rssXML struct {
	Channel struct {
		Items []rssItemXML `xml:"item"`
	} `xml:"channel"`
}

type rssItemXML struct {
	Title     string          `xml:"title"`
	PubDate   string          `xml:"pubDate"`
	Link      string          `xml:"link"`
	Enclosure rssEnclosureXML `xml:"enclosure"`
}

type rssEnclosureXML struct {
	URL    string `xml:"url,attr"`
	Length string `xml:"length,attr"`
}

// GetEpisodes 获取种子条目。
func (s *MikanSpider) GetEpisodes(mediaID, subgroupID string) ([]MagnetItem, error) {
	_, body, err := s.requestWithRetry(s.BuildRssURL(mediaID, subgroupID))
	if err != nil {
		return nil, err
	}
	var rss rssXML
	if err := xml.Unmarshal(body, &rss); err != nil {
		return []MagnetItem{}, nil
	}

	episodes := make([]MagnetItem, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		var torrentURL, magnetURL string
		var fileSizeMB float64
		if item.Enclosure.URL != "" {
			torrentURL = item.Enclosure.URL
			if match := torrentHashRe.FindStringSubmatch(torrentURL); match != nil {
				magnetURL = "magnet:?xt=urn:btih:" + match[1]
			}
			if item.Enclosure.Length != "" && isDigits(item.Enclosure.Length) {
				var bytes int64
				fmt.Sscanf(item.Enclosure.Length, "%d", &bytes)
				fileSizeMB = float64(int64(float64(bytes)/1024/1024*100+0.5)) / 100
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

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

func hostOf(rawURL string) string {
	s := rawURL
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(s, ":"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}

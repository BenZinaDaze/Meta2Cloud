// Package scraper RSS 订阅源爬虫（对应 Python 版 scraper/ 包）。
package scraper

import (
	"fmt"
	"log"
	"sort"
	"sync"
)

// MediaItem 搜索结果中的媒体条目。
type MediaItem struct {
	MediaID      string `json:"media_id"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	CoverImage   string `json:"cover_image,omitempty"`
	Site         string `json:"site"`
	SubgroupID   string `json:"subgroup_id,omitempty"`
	SubgroupName string `json:"subgroup_name,omitempty"`
	RssURL       string `json:"rss_url,omitempty"`
}

// maxSpiderBodyBytes 爬虫响应体大小上限（RSS/HTML 远小于此；源被劫持返回
// 超大响应时不至于耗尽内存）。
const maxSpiderBodyBytes = 4 << 20

// MagnetItem 种子/磁力条目。
type MagnetItem struct {
	Title       string  `json:"title"`
	TorrentURL  string  `json:"torrent_url,omitempty"`
	MagnetURL   string  `json:"magnet_url,omitempty"`
	PublishTime string  `json:"publish_time,omitempty"`
	FileSizeMB  float64 `json:"file_size_mb,omitempty"`
	Site        string  `json:"site"`
}

// Spider 站点爬虫接口。
type Spider interface {
	// SiteID 站点唯一标识（如 "mikan"）。
	SiteID() string
	// SiteName 站点显示名。
	SiteName() string
	// RssHosts 识别为该站点 RSS 的域名后缀。
	RssHosts() []string
	// SearchMedia 按关键字搜索媒体。
	SearchMedia(keyword string) ([]MediaItem, error)
	// GetEpisodes 获取指定媒体（及可选字幕组）的种子条目。
	GetEpisodes(mediaID, subgroupID string) ([]MagnetItem, error)
}

// Factory 爬虫注册中心。
type Factory struct {
	mu      sync.RWMutex
	spiders map[string]Spider
	order   []string
}

// defaultFactory 全局注册中心：包初始化即装好内置爬虫，之后只读——
// 不存在"注册前调用"的时序（Register 仅供测试替换 fake 爬虫）。
var defaultFactory = buildDefaultFactory()

// NewFactory 构造注册中心。
func NewFactory() *Factory {
	return &Factory{spiders: map[string]Spider{}}
}

// buildDefaultFactory 构造全局工厂并注册内置爬虫（mikan + anibt）。
func buildDefaultFactory() *Factory {
	f := NewFactory()
	f.Register(NewMikanSpider())
	f.Register(NewAniBTSpider())
	return f
}

// DefaultFactory 全局注册中心（内置爬虫已随包初始化装好）。
func DefaultFactory() *Factory { return defaultFactory }

// Register 注册/替换爬虫（生产代码仅包初始化调用；导出供测试注入 fake）。
func (f *Factory) Register(spider Spider) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.spiders[spider.SiteID()]; !exists {
		f.order = append(f.order, spider.SiteID())
	}
	f.spiders[spider.SiteID()] = spider
}

// GetSpider 按站点 ID 取爬虫。
func (f *Factory) GetSpider(siteID string) (Spider, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	spider, ok := f.spiders[siteID]
	if !ok {
		return nil, fmt.Errorf("Spider for site '%s' not found.", siteID)
	}
	return spider, nil
}

// GetAllSpiders 全部爬虫（按注册顺序）。
func (f *Factory) GetAllSpiders() []Spider {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]Spider, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, f.spiders[id])
	}
	return out
}

// SiteInfo 站点信息。
type SiteInfo struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	RssHosts []string `json:"rss_hosts"`
}

// ListSites 列出全部站点。
func (f *Factory) ListSites() []SiteInfo {
	sites := make([]SiteInfo, 0, 8)
	for _, spider := range f.GetAllSpiders() {
		sites = append(sites, SiteInfo{ID: spider.SiteID(), Name: spider.SiteName(), RssHosts: spider.RssHosts()})
	}
	return sites
}

// SearchAll 全站点搜索（单站点失败不影响其他）。
func (f *Factory) SearchAll(keyword string) []MediaItem {
	var results []MediaItem
	for _, spider := range f.GetAllSpiders() {
		items, err := spider.SearchMedia(keyword)
		if err != nil {
			log.Printf("Error searching %s: %v", spider.SiteID(), err)
			continue
		}
		results = append(results, items...)
	}
	return results
}

// FindSpiderByRssURL 按 RSS URL 域名匹配爬虫。
func (f *Factory) FindSpiderByRssURL(rssURL string) Spider {
	host := hostOf(rssURL)
	if host == "" {
		return nil
	}
	var candidates []Spider
	for _, spider := range f.GetAllSpiders() {
		for _, suffix := range spider.RssHosts() {
			if host == suffix || len(host) > len(suffix) && host[len(host)-len(suffix)-1:] == "."+suffix {
				candidates = append(candidates, spider)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// 取域名后缀最长的（最精确的）匹配
	sort.SliceStable(candidates, func(i, j int) bool {
		return longestHost(candidates[i]) > longestHost(candidates[j])
	})
	return candidates[0]
}

func longestHost(s Spider) int {
	longest := 0
	for _, h := range s.RssHosts() {
		if len(h) > longest {
			longest = len(h)
		}
	}
	return longest
}

package scraper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMikanSearchAndEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/Home/Search"):
			// 关键词必须已百分号编码：mikan 服务端拒绝请求行中的原始非 ASCII 字节
			for _, b := range []byte(r.URL.RawQuery) {
				if b > 127 {
					t.Errorf("searchstr 未编码，RawQuery 含非 ASCII 字节: %q", r.URL.RawQuery)
					break
				}
			}
			w.Write([]byte(`
				<html><body>
				<a href="/Home/Bangumi/3065">标题A</a>
				<a href="/Home/Bangumi/3065">重复项应去重</a>
				<a href="/Home/Bangumi/2000">标题B</a>
				</body></html>`))
		case strings.HasPrefix(r.URL.Path, "/Home/Bangumi/3065"):
			w.Write([]byte(`<a class="subgroup-name" data-anchor="#602">字幕组X</a>`))
		case strings.HasPrefix(r.URL.Path, "/Home/Bangumi/"):
			w.Write([]byte(`<html><body></body></html>`)) // 无字幕组
		case strings.HasPrefix(r.URL.Path, "/RSS/Bangumi"):
			w.Write([]byte(`<?xml version="1.0"?>
				<rss><channel>
				<item>
					<title>[GroupX] 标题 - 01 [1080p]</title>
					<pubDate>Tue, 13 Oct 2026 08:00:00 GMT</pubDate>
					<link>https://example.com/page</link>
					<enclosure url="https://mikan/Download/20260116/091b1b30d24c4d41759da1b0e24ddfffc89925c4.torrent" length="157286400" type="application/x-bittorrent"/>
				</item>
				</channel></rss>`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	spider := NewMikanSpider()
	spider.SetTestBaseURL(server.URL)

	items, err := spider.SearchMedia("标题")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 { // 标题A (全部)+[字幕组X]；标题B (全部，无字幕组)
		t.Fatalf("items = %d, want 3: %+v", len(items), items)
	}
	if items[0].Name != "标题A (全部)" || items[0].MediaID != "3065" {
		t.Errorf("items[0] = %+v", items[0])
	}
	if items[1].SubgroupID != "602" || items[1].SubgroupName != "字幕组X" {
		t.Errorf("items[1] = %+v", items[1])
	}
	if !strings.Contains(items[1].RssURL, "subgroupid=602") {
		t.Errorf("rssURL = %q", items[1].RssURL)
	}

	episodes, err := spider.GetEpisodes("3065", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 {
		t.Fatalf("episodes = %d", len(episodes))
	}
	ep := episodes[0]
	if ep.MagnetURL != "magnet:?xt=urn:btih:091b1b30d24c4d41759da1b0e24ddfffc89925c4" {
		t.Errorf("magnet = %q", ep.MagnetURL)
	}
	if ep.FileSizeMB != 150 {
		t.Errorf("fileSizeMB = %v, want 150", ep.FileSizeMB)
	}
}

func TestAniBTSearchAndEpisodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/bgm/search":
			// 真实接口的 bgmId 为数字，同时保留字符串形式覆盖兼容分支
			w.Write([]byte(`{"data": [{"bgmId": 42, "nameCn": "测试番", "image": "/cover.jpg"}, {"bgmId": "77", "nameCn": "字符串ID番"}]}`))
		case r.URL.Path == "/api/anime/groups":
			w.Write([]byte(`[{"group": {"slug": "sub-a", "name": "组A"}}]`))
		case strings.HasPrefix(r.URL.Path, "/rss/anime.xml"):
			// 模拟真实 anibt 结构：infohash 嵌套在 <anibt:torrent> 容器内
			w.Write([]byte(`<?xml version="1.0"?>
				<rss xmlns:anibt="https://anibt.net/xmlns/rss/1.0/"><channel>
				<item>
					<title>测试番 - 02</title>
					<pubDate>Tue, 13 Oct 2026 08:00:00 GMT</pubDate>
					<link>https://anibt.net/anime/42</link>
					<enclosure url="https://anibt.net/api/torrent/rel_xxx.torrent" length="104857600" type="application/x-bittorrent"/>
					<anibt:torrent>
						<link>https://anibt.net/anime/42</link>
						<infohash>e1ddc119073b1609863eae79ce26713375dd3787</infohash>
					</anibt:torrent>
				</item>
				<item>
					<title>测试番 - 01</title>
					<pubDate>Tue, 13 Oct 2026 07:00:00 GMT</pubDate>
					<link>https://anibt.net/anime/42</link>
					<magneturi>magnet:?xt=urn:btih:abcdefabcdefabcdefabcdefabcdefabcdefabcdef</magneturi>
					<enclosure url="https://anibt.net/dl/xxx" length="104857600" type="application/x-bittorrent"/>
				</item>
				</channel></rss>`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()

	spider := NewAniBTSpider()
	spider.SetTestBaseURL(server.URL)

	items, err := spider.SearchMedia("测试番")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 { // 数字 bgmId 42 + 字符串 bgmId "77"，各 (全部) + [组A]
		t.Fatalf("items = %d: %+v", len(items), items)
	}
	if items[0].MediaID != "42" {
		t.Errorf("数字 bgmId 应解析为 \"42\": items[0] = %+v", items[0])
	}
	if items[1].SubgroupID != "sub-a" {
		t.Errorf("items[1] = %+v", items[1])
	}
	if items[2].MediaID != "77" {
		t.Errorf("字符串 bgmId 应解析为 \"77\": items[2] = %+v", items[2])
	}

	episodes, err := spider.GetEpisodes("42", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episodes = %d", len(episodes))
	}
	// 第 1 条：infohash 嵌套在 <anibt:torrent> 内，应递归取到并生成 magnet
	if episodes[0].MagnetURL != "magnet:?xt=urn:btih:e1ddc119073b1609863eae79ce26713375dd3787" {
		t.Errorf("magnet = %q", episodes[0].MagnetURL)
	}
	if episodes[0].TorrentURL != "https://anibt.net/api/torrent/rel_xxx.torrent" {
		t.Errorf("torrentURL = %q", episodes[0].TorrentURL)
	}
	// 第 2 条：直接子元素 magneturi
	if episodes[1].MagnetURL != "magnet:?xt=urn:btih:abcdefabcdefabcdefabcdefabcdefabcdefabcdef" {
		t.Errorf("magnet = %q", episodes[1].MagnetURL)
	}
}

func TestFactory(t *testing.T) {
	f := RegisterDefault()
	sites := f.ListSites()
	if len(sites) < 2 {
		t.Fatalf("sites = %+v", sites)
	}
	if _, err := f.GetSpider("mikan"); err != nil {
		t.Error(err)
	}
	if _, err := f.GetSpider("nonexistent"); err == nil {
		t.Error("未知站点应报错")
	}

	// RSS 域名匹配
	if sp := f.FindSpiderByRssURL("https://mikanani.me/RSS/Bangumi?bangumiId=1"); sp == nil || sp.SiteID() != "mikan" {
		t.Errorf("mikanani.me 应匹配 mikan: %v", sp)
	}
	if sp := f.FindSpiderByRssURL("https://www.anibt.net/rss/anime.xml"); sp == nil || sp.SiteID() != "anibt" {
		t.Errorf("www.anibt.net 应匹配 anibt: %v", sp)
	}
	if sp := f.FindSpiderByRssURL("https://example.com/rss"); sp != nil {
		t.Errorf("未知域名不应匹配: %v", sp)
	}
}

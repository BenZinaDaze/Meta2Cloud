# 资源站点接入指南

本文说明如何给 Meta2Cloud 接入新的资源检索 / RSS 订阅站点。现有实现采用 spider 插件模式，入口在 `internal/scraper/`。

## 接入位置

新增站点实现放在：

```text
internal/scraper/<site>.go
```

站点需要实现 `scraper.Spider` 接口，并在 `scraper.go` 的 `RegisterDefault()` 注册：

```go
func RegisterDefault() *Factory {
	defaultFactory.Register(NewMikanSpider())
	defaultFactory.Register(NewAniBTSpider())
	defaultFactory.Register(NewExampleSpider()) // 新增
	return defaultFactory
}
```

注册后会自动参与：

- `/api/scraper/search`：资源检索聚合（按名称分组、封面聚合）。
- `/api/scraper/episodes`：按站点拉取资源条目。
- `/api/scraper/sites`：前端订阅站点下拉与 RSS URL 自动推断。
- RSS 订阅服务的 URL 解析与条目抓取。

## 必须实现的接口

`Spider` 定义在 `internal/scraper/scraper.go`：

```go
type Spider interface {
	SiteID() string            // 站点唯一标识，如 "mikan"
	SiteName() string          // 显示名
	RssHosts() []string        // 识别为该站点 RSS 的域名后缀
	SearchMedia(keyword string) ([]MediaItem, error)
	GetEpisodes(mediaID, subgroupID string) ([]MagnetItem, error)
}
```

### 约定

- **MediaItem**：搜索结果应同时产出「全部」条目与各字幕组条目（`SubgroupID`/`SubgroupName`/`RssURL`）。
- **MagnetItem**：种子条目尽量补全 `MagnetURL`（从 torrent 哈希或 infohash 扩展标签提取）与 `FileSizeMB`。
- **HTML 解析**用 `goquery`，RSS/XML 用 `encoding/xml`，参照 `mikan.go` / `anibt.go`。
- **可测试性**：提供 `SetTestBaseURL(baseURL string)` 供 `httptest` 注入，参见 `scraper_test.go`。

## 测试要求

参照 `internal/scraper/scraper_test.go`：`httptest` 模拟站点搜索/详情/RSS 三类响应，覆盖：

- 搜索去重与字幕组条目生成
- RSS 条目解析（磁力提取、体积换算）
- `FindSpiderByRssURL` 域名后缀匹配（子域名命中、未知域名返回 nil）

# 资源站点接入指南

本文说明如何给 Meta2Cloud 接入新的资源检索 / RSS 订阅站点。现有实现采用 spider 插件模式，入口在 `scraper/`。

## 接入位置

新增站点实现放在：

```text
scraper/strategies/<site>_spider.py
```

站点类需要继承 `scraper.core.base_spider.BaseSpider`，并在 `scraper/__init__.py` 注册：

```python
from scraper.strategies.example_spider import ExampleSpider

SpiderFactory.register(ExampleSpider())
```

注册后会自动参与：

- `/api/scraper/search`：资源检索聚合。
- `/api/scraper/episodes`：按站点拉取资源条目。
- `/api/scraper/sites`：前端手动订阅站点下拉与 RSS URL 自动推断。

## 必须实现的接口

`BaseSpider` 定义了两个必需方法和一个必需站点 ID：

```python
class ExampleSpider(BaseSpider):
    @property
    def site_id(self) -> str:
        return "example"

    def search_media(self, keyword: str) -> list[MediaItem]:
        ...

    def get_episodes(self, media_id: str, subgroup_id: str | None = None) -> list[MagnetItem]:
        ...
```

`site_id` 必须稳定、全小写，作为数据库订阅记录、API 参数和前端标签页的站点标识。

## 推荐实现的站点元信息

为了让手动订阅自动支持新站点，必须提供：

```python
@property
def site_name(self) -> str:
    return "Example"

@property
def rss_hosts(self) -> list[str]:
    return ["example.com", "rss.example.com"]
```

`rss_hosts` 用于前端根据 RSS URL 自动选择站点，也用于 `/api/scraper/sites` 返回支持列表。匹配规则支持完整 host 或子域后缀。

## 返回模型要求

`search_media()` 返回 `MediaItem`。关键字段：

- `media_id`：该站点内部资源 ID，例如 Mikan 的 `bangumiId`、AniBT 的 `bgmId`。
- `name`：展示名。字幕组结果建议使用 `番剧名 [字幕组名]`。
- `url`：站点详情页。
- `site`：等于 `site_id`。
- `subgroup_id`：字幕组 ID / slug，可为空。
- `subgroup_name`：字幕组展示名，可为空。
- `rss_url`：推荐填写。前端复制 RSS 和创建订阅会直接使用它。

`get_episodes()` 返回 `MagnetItem`。关键字段：

- `title`：RSS 原始标题。
- `torrent_url`：`.torrent` 下载地址，可为空。
- `magnet_url`：磁力链接，可为空，但至少应和 `torrent_url` 二选一。
- `publish_time`：发布时间字符串。
- `file_size_mb`：文件大小，单位 MB。
- `site`：等于 `site_id`。

## RSS URL 解析

订阅测试和后台轮询会通过 `webui/services/subscriptions.py` 的 `_parse_rss_url(site, rss_url)` 将 RSS URL 解析成：

```python
(media_id, subgroup_id)
```

新增站点时需要在该文件中增加解析分支，例如：

```python
def _parse_example_rss_url(rss_url: str) -> tuple[str, str | None]:
    ...

def _parse_rss_url(site: str, rss_url: str) -> tuple[str, str | None]:
    if site == "example":
        return _parse_example_rss_url(rss_url)
```

同时 `_detect_site_from_url()` 应能通过 host 识别该站点，用于“解析 RSS”按钮自动回填站点。

## 最小测试建议

为新站点添加测试文件或扩展 `test/test_anibt_scraper.py`，至少覆盖：

- `search_media()` 能返回带 `site`、`media_id`、`subgroup_id`、`rss_url` 的结果。
- `get_episodes()` 能解析真实或 mock RSS，生成 `MagnetItem`。
- `_parse_rss_url()` 能从该站 RSS URL 解析出正确 ID。
- `/api/scraper/sites` 能暴露新站点的 `id`、`name`、`rss_hosts`。

运行：

```bash
conda run -n myself python -m pytest test/test_anibt_scraper.py test/test_media_actions.py
cd frontend && npm run build
```

## 常见注意事项

- 不要在前端硬编码新站点 RSS URL。优先由 spider 返回 `rss_url`。
- 如果 RSS 同时提供完整 `magneturi` 和 `infohash`，优先保留完整 `magneturi`，避免丢失 tracker。
- 网络请求应设置超时和 User-Agent，并处理解析失败时返回空列表或明确错误。
- 后台订阅轮询会调用 `get_episodes()`，所以该方法必须可重复调用且不依赖前端状态。

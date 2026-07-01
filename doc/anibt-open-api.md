# AniBT Open API 资源检索参考

来源：https://wiki.anibt.net/docs/open-api  
用途：为 Meta2Cloud 后续实现 AniBT 资源检索、RSS 订阅、字幕组 slug 映射与 Bangumi ID 查询提供本地参考。更新时间：2026-07-01。

## 基本约定

- Base URL：`https://anibt.net`
- 公开读接口多数无需鉴权；发布类接口和私人订阅需要鉴权。
- 字幕组使用稳定 `slug`，不要依赖展示名。启动时建议拉取 `/api/subtitle-groups` 建立 `name -> slug` 映射。
- 番剧查询优先使用 Bangumi subject id，即 `bgmId`。自然语言番名应先通过 `/api/bgm/search` 转换。
- 支持 `ETag` / `If-None-Match`；轮询 RSS 时应保存上次 `ETag`，命中后服务端返回 `304 Not Modified`。

## 推荐检索流程

1. 调用 `GET /api/subtitle-groups`，缓存字幕组 `slug`、名称和统计信息。
2. 用户输入番名时调用 `GET /api/bgm/search?q=<keyword>&limit=<n>`，取得 `bgmId`。
3. 用 `GET /rss/anime.xml?bgmId=<bgmId>&groupSlug=<slug>` 拉取资源 RSS。
4. 轮询时带 `If-None-Match: <etag>`，避免重复下载完整 RSS。

## 端点速查

| 场景 | 方法与路径 | 说明 |
| --- | --- | --- |
| 字幕组目录 | `GET /api/subtitle-groups` | 公开列表，返回 slug、展示名、发布数、番剧数。 |
| 当前字幕组信息 | `GET /api/subtitle-groups/me` | 需 `Authorization: Bearer <API_KEY>`。 |
| Bangumi 搜索 | `GET /api/bgm/search?q=<keyword>&limit=<n>` | 将番名映射为 `bgmId`。 |
| 番剧 RSS 主入口 | `GET /rss/anime.xml?<query>` | 支持番剧、字幕组、分辨率、语言等过滤。 |
| 字幕组短链 RSS | `GET /rss/group/<slug>.xml` | 等价于只按字幕组过滤；路径必须以 `.xml` 结尾。 |
| 全站磁力 RSS | `GET /rss/magnets.xml` | 全站最新番剧磁力 RSS。 |
| 其他类 RSS | `GET /rss/other/<category>.xml` | `manga`、`music`、`raw`、`stage`。 |
| 私人订阅 RSS | `GET /rss/subscriptions.xml` | 需登录认证，返回“我的订阅”。 |
| 季度番剧 | `GET /api/seasons/anime` | 支持季度或 `bgmId` 精确查询。 |
| 单番剧字幕组 | `GET /api/anime/groups?bgmId=<id>` | 查询某番有哪些字幕组发布。 |
| 发布番剧资源 | `POST /api/releases/publish` | 字幕组写入端点，支持 JSON magnet 或 multipart torrent。 |
| 发布其他资源 | `POST /api/other-releases/publish` | 发布漫画、音乐、RAW、舞台剧。 |

## `/rss/anime.xml` 查询参数

至少提供 `bgmId` 或 `groupSlug` 之一，否则请求会被拒绝。

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `bgmId` | integer | Bangumi.tv subject id。别名：`bangumiId`。 |
| `groupSlug` | string | 字幕组 slug。别名：`subgroupSlug`、`subgroupid`。 |
| `resolution` | enum[] | `4K`、`2160p`、`1080p`、`720p`、`480p`、`360p`。 |
| `language` | enum[] | `CHS`、`CHT`、`JP`、`EN`。 |
| `format` | enum[] | `MKV`、`MP4`、`AVI`、`WEBM`。 |
| `subtitle` | enum[] | `EXTERNAL`、`INTERNAL`、`EMBEDDED`、`NONE`。 |
| `episode` | integer | 数字集数，精确匹配。 |
| `episodeKey` | string | 显示集数键，如 `01`、`00-01`、`BATCH`。 |
| `limit` | integer | 默认 100，上限 200。越界会截断到上限。 |

重复参数可用两种写法：

```text
?resolution=1080p&resolution=720p
?resolution=1080p,720p
```

RSS 返回标准 RSS 2.0 + BEP 36。每个 item 通常包含标题、发布时间、详情链接、磁力 enclosure、infoHash、字幕组、番剧和集数信息。

## 常用示例

```bash
# 搜索 bgmId
curl -s -G 'https://anibt.net/api/bgm/search' \
  --data-urlencode 'q=葬送的芙莉莲' \
  -d 'limit=3'

# 单番剧 + 单字幕组
curl 'https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=kirara-fantasia'

# 跨字幕组聚合一个番剧
curl 'https://anibt.net/rss/anime.xml?bgmId=543360&resolution=1080p&language=CHS'

# 字幕组全部发布
curl 'https://anibt.net/rss/anime.xml?groupSlug=kirara-fantasia&limit=50'

# 字幕组短链
curl 'https://anibt.net/rss/group/kirara-fantasia.xml'

# 合集或特典
curl 'https://anibt.net/rss/anime.xml?bgmId=543360&groupSlug=kitauji-sub&episodeKey=BATCH'

# ETag 复用
ETAG=$(curl -sI 'https://anibt.net/rss/anime.xml?bgmId=543360' | awk -F': ' 'tolower($1)=="etag"{print $2}' | tr -d '\r')
curl -sI -H "If-None-Match: $ETAG" 'https://anibt.net/rss/anime.xml?bgmId=543360'
```

## JSON 读接口

### `GET /api/subtitle-groups`

返回所有可见字幕组的精简信息。关键字段包括：

- `slug`：URL 安全稳定标识，用于 `groupSlug`。
- `name`：展示名，可能变更。
- `totalReleases`：发布总数。
- `totalAnimes`：覆盖番剧数。
- `latestReleaseAt`：最近发布时间。

缓存：`s-maxage=300`，`stale-while-revalidate=86400`，支持 ETag。

### `GET /api/bgm/search`

参数：

- `q`：必填，搜索关键词。
- `limit`：默认 5，上限 25。

响应里 `data[].bgmId` 是后续 RSS 和发布接口应使用的权威番剧 ID。生产代码应本地缓存 `bgmId`，不要每次拉 RSS 前都搜索。

### `GET /api/seasons/anime`

查询季度番剧。支持：

- 无参数：默认当前季度。
- `season=2026春` 或 `season=2026-SPRING`。
- `bgmId=543360`：精确返回单个番剧。

缓存：`s-maxage=600`，`stale-while-revalidate=86400`，限流 30 次 / 60 秒 / IP。

### `GET /api/anime/groups?bgmId=<id>`

查询某番剧下的发布字幕组和最近发布。返回的 `slug` 可直接用于 `/rss/anime.xml?groupSlug=...`。如果番剧不存在或隐藏，返回 `404 ANIME_NOT_FOUND`。

缓存：`s-maxage=300`，`stale-while-revalidate=3600`，限流 30 次 / 60 秒 / IP。

## 其他 RSS

- `/rss/group/<slug>.xml`：等价于 `/rss/anime.xml?groupSlug=<slug>`，支持 `limit`，不支持 Tag 过滤。
- `/rss/magnets.xml`：全站最新番剧磁力，支持 `limit`、关键词和常见 Tag 过滤。
- `/rss/other/<category>.xml`：其他类资源，`category` 为 `manga`、`music`、`raw`、`stage`，支持 `q`、`limit` 和排序类参数。
- `/rss/subscriptions.xml`：私人订阅 RSS，`Cache-Control: private`，需 Bearer 或 Cookie 认证，支持 ETag。

## 发布接口概要

发布接口用于字幕组自动上传资源，不是资源检索主流程，但可用于理解 AniBT 数据来源。

### `POST /api/releases/publish`

鉴权：

```text
Authorization: Bearer <YOUR_API_KEY>
```

支持 `multipart/form-data` 上传 `.torrent`，或 JSON 提交 magnet。关键参数：

- `bgmId`：推荐填写，来自 `/api/bgm/search`。
- `title`：资源标题。
- `torrent`：multipart 文件字段。
- `magnetLink` / `magnetBase64`：磁力链接。
- `resolution`、`language`、`format`、`subtitle`、`episode`、`episodeKey`：结构化检索字段。
- `trackers`、`notes`、`customTags`：附加信息。
- `preview=true`：预览模式，不正式发布。

### `POST /api/other-releases/publish`

用于非番剧资源。必填：

- `category`：`manga`、`music`、`raw`、`stage`。
- `title`：资源标题。
- `torrent`、`torrentStorageId`、`magnetLink`、`magnetBase64` 至少提供一个。

两个发布接口常见错误：`400` 参数或种子错误，`401` 未传 Bearer，`403` API Key 无效，`429` 写入频率过高。

## 缓存、限流与轮询建议

| 端点 | 默认 limit | 上限 | CDN TTL / SWR | 默认限流 |
| --- | ---: | ---: | --- | --- |
| `/rss/anime.xml` | 100 | 200 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/rss/group/<slug>.xml` | 100 | 200 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/rss/magnets.xml` | 100 | 200 | 120s / 60s | 120 次 / 60 秒 / IP |
| `/api/bgm/search` | 5 | 25 | 300s / 300s | 60 次 / 60 秒 / IP |
| `/api/subtitle-groups` | 全量 | 全量 | 300s / 86400s | 60 次 / 60 秒 / IP |
| `/api/seasons/anime` | - | - | 600s / 86400s | 30 次 / 60 秒 / IP |
| `/api/anime/groups` | - | - | 300s / 3600s | 30 次 / 60 秒 / IP |

客户端建议：

- RSS 轮询间隔建议 5-10 分钟，不要低于 1 分钟。
- 每次保存响应 `ETag`，下次带 `If-None-Match`。
- `bgmId` 和 `slug` 应长期缓存；字幕组目录可以每天或启动时刷新。
- 对多字幕组聚合，优先使用一个 RSS URL 的逗号参数或重复参数，不要为每个字幕组发独立请求。

## 接入注意事项

- `slug` 优先于字幕组 `name`；`name` 可用于展示，不应用作查询键。
- `bgmId` 优先于自然语言番名；同名、多季、剧场版容易撞车。
- `/rss/anime.xml` 会尽量将 `resolution`、`language`、`subtitle`、`format` 下推到 Typesense；`episode`、`episodeKey` 可能在 Web runtime 小窗口中过滤，极严格组合可能返回少于 `limit`。
- 路径和查询参数是公开契约；内部主键如 `organizationId` 不属于公开契约。
- 如果同时发送 `If-None-Match` 和 `If-Modified-Since`，服务端按 HTTP 规范优先使用 ETag。

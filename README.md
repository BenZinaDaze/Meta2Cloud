# Meta2Cloud

把网盘里的电影和剧集自动整理好。

它会帮你做这些事：

- 识别文件名里的片名、年份、季集号
- 自动查询 TMDB
- 自动生成 NFO
- 自动下载海报和背景图
- 自动创建电影 / 剧集目录
- 自动移动和重命名文件
- 整理完成后推送 Telegram 通知

现在支持：

- Google Drive
- 115 网盘

---

## 这适合谁

如果你平时是这样用网盘的：

- 下载完资源后先放到一个“待整理”目录
- 想把电影和剧集自动归类进媒体库
- 想让 Emby / Plex / Infuse / Kodi 更容易识别
- 不想手动改文件名、建文件夹、补海报和 NFO

那这个项目就是给你用的。

---

## 5 分钟快速开始

### 1. 准备一个配置目录

在项目目录旁边新建一个文件夹，名字随意，下面用 `meta2cloud-config/` 举例：

```bash
mkdir -p meta2cloud-config
cp config/config.example.yaml meta2cloud-config/config.yaml
cp config/parser-rules.example.yaml meta2cloud-config/parser-rules.yaml
```

这个目录以后会放：

- `config.yaml`
- `parser-rules.yaml`
- Google Drive 用的 `credentials.json` 和 `token.json`
- 115 用的 `115-token.json`

### 2. 先把程序配起来

你有两种方式：

- 直接改 `config.yaml` 和 `parser-rules.yaml`
- 先启动后，通过 Web UI 的“配置”页面来改

如果你是第一次用，最少先准备好这几项：

```yaml
storage:
  primary: "google_drive"   # 当前用哪个网盘跑：google_drive 或 pan115

webui:
  username: "admin"
  password: "你的密码"

tmdb:
  api_key: "你的 TMDB API Key"
```

然后再把你当前要用的网盘那一段配置好：

- 如果 `storage.primary` 填的是 `google_drive`，就把 `drive:` 这一段填好
- 如果 `storage.primary` 填的是 `pan115`，就把 `u115:` 这一段填好

这里最容易搞混的一点是：

- `pan115` 是填在 `storage.primary` 里的名字
- `u115:` 是 115 这部分具体配置所在的位置

也就是说，115 的正确写法是这样：

```yaml
storage:
  primary: "pan115"

u115:
  client_id: "你的 client_id"
```

后面有 Google Drive 和 115 的单独说明。

### 3. 启动

```bash
docker compose up -d
```

### 4. 打开网页

浏览器访问：

```text
http://localhost:38765
```

用你在 `config.yaml` 里填的账号密码登录。登录后，很多常用配置也可以直接在 Web UI 里修改。

### 5. 先试跑一次

如果你想先看效果、不真正移动文件：

```bash
docker exec meta2cloud python pipeline.py --dry-run
```

如果你用 115：

```bash
docker exec meta2cloud python pipeline.py --storage pan115 --dry-run
```

---

## 你最后会得到什么目录结构

### 剧集

```text
剧集根目录/
└─ Breaking Bad (2008)/
   ├─ tvshow.nfo
   ├─ poster.jpg
   ├─ fanart.jpg
   ├─ season01-poster.jpg
   └─ Season 01/
      ├─ season.nfo
      ├─ Breaking Bad - S01E01.mkv
      └─ Breaking Bad - S01E01.nfo
```

### 电影

```text
电影根目录/
└─ Inception (2010)/
   ├─ Inception (2010).mkv
   ├─ Inception (2010).nfo
   ├─ poster.jpg
   └─ fanart.jpg
```

---

## 如果你用的是 Google Drive

### 你需要准备什么

- Google Drive 的 OAuth 凭据文件 `credentials.json`
- 首次授权后生成的 `token.json`

### 怎么拿到 `credentials.json`

1. 打开 [Google Cloud Console](https://console.cloud.google.com/)
2. 进入“API 和服务”
3. 创建 OAuth 客户端 ID
4. 类型选“桌面应用”
5. 下载后保存成 `credentials.json`

### 第一次授权怎么做

把 `credentials.json` 放进 `meta2cloud-config/` 后，执行：

```bash
docker run --rm -it \
  -v $(pwd)/meta2cloud-config:/app/config \
  benz1/meta2cloud:latest \
  python pipeline.py --storage google_drive --dry-run
```

按提示在浏览器里完成授权后，会生成 `token.json`。

### `config.yaml` 里要填什么

```yaml
storage:
  primary: "google_drive"   # 这里写的是当前使用的网盘名字

drive:
  scan_folder_id: "待整理目录 ID"
  root_folder_id: "媒体库根目录 ID"
  movie_root_id: "电影目录 ID"   # 可选
  tv_root_id: "剧集目录 ID"      # 可选
```

### Google Drive 的目录 ID 怎么看

打开文件夹后，浏览器地址通常像这样：

```text
https://drive.google.com/drive/folders/1AbCdEfGhIjKlMn
```

最后那串 `1AbCdEfGhIjKlMn` 就是目录 ID。

---

## 如果你用的是 115

### 你需要准备什么

- 115 开放平台 `client_id`
- 首次授权后得到的 `115-token.json`

第一次使用时，可以直接通过 Web UI 完成 115 授权。  
授权成功后，程序会把登录信息保存到 `115-token.json`。

### `config.yaml` 里要填什么

```yaml
storage:
  primary: "pan115"   # 注意：这里写 pan115，不是 u115

u115:
  client_id: "你的 client_id"   # 115 的具体配置写在 u115: 下面
  download_folder_id: "云下载目录 ID"
  root_folder_id: "媒体库根目录 ID"
  movie_root_id: "电影目录 ID"
  tv_root_id: "剧集目录 ID"
```

### 这些目录分别是干什么的

- `download_folder_id`：程序会从这里开始扫描新资源
- `root_folder_id`：整理后的总目录
- `movie_root_id`：电影单独放哪里
- `tv_root_id`：剧集单独放哪里

如果你已经把电影和剧集分开存了，建议把 `movie_root_id` 和 `tv_root_id` 都填上。

### 115 的目录 ID 怎么找

最直接的方式就是看浏览器地址里的 `cid`。

比如你在 115 网页版打开某个文件夹，地址里如果有：

```text
...cid=3406743276128217574
```

那这个 `3406743276128217574` 就是目录 ID。

如果你不方便从浏览器看，也可以用项目里的 Web UI 或日志确认。

---

## 网页里都有什么

你平时主要会用到这些页面：

| 页面 | 你可以拿它做什么 |
|---|---|
| 登录 | 进入后台 |
| 下载大盘 | 如果你接了 Aria2，这里可以看下载状态 |
| 媒体库 | 看电影和剧集有没有整理成功 |
| 剧集详情 | 看某一部剧缺了哪几集 |
| 解析测试 | 测一下某个文件名会被识别成什么 |
| 实时日志 | 看程序当前在做什么、报了什么错 |
| 配置 | 查看和修改程序配置 |

---

## 最常用的几条命令

### 正式整理

```bash
docker exec meta2cloud python pipeline.py
```

### 先预览，不真正改动文件

```bash
docker exec meta2cloud python pipeline.py --dry-run
```

### 指定用 115 跑

```bash
docker exec meta2cloud python pipeline.py --storage pan115
```

### 跳过 TMDB

```bash
docker exec meta2cloud python pipeline.py --no-tmdb
```

### 不下载海报

```bash
docker exec meta2cloud python pipeline.py --no-images
```

### 看日志

```bash
docker logs -f meta2cloud
```

---

## 如果你想下载完自动整理

项目支持通过 `/trigger` 自动触发整理。

最常见的用法是：

- Aria2 下载完成
- Rclone 上传到网盘
- 上传脚本顺手请求一下 `/trigger`
- Meta2Cloud 自动开始整理

示例：

```bash
curl -sf -X POST http://localhost:38765/trigger \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Secret: your_webhook_secret" \
  -d '{"path": "/path/to/file"}'
```

如果你不用密钥，也可以不传 `X-Webhook-Secret`。

### 批量上传时为什么不会一直狂触发

因为支持防抖。

比如你连续上传 12 集，不会每集都立刻触发一次整理。  
你可以在配置里设置：

```yaml
telegram:
  debounce_seconds: 60
```

这样最后一集传完后，等 60 秒再统一整理一次。

---

## Telegram 通知大概长这样

### 正常整理完成

```text
📺 Breaking Bad (2008)

Season 01：
  • S01E01  试播集
  • S01E03  …And the Bag's in the River
```

一般还会带封面图。

### 没找到 TMDB

```text
📺 TMDB 未找到元数据，文件未整理
• 假面骑士强人.S01E30.1080p.mkv
• 假面骑士强人.S01E31.1080p.mkv
```

这样你能直接回到网盘里找到原文件，改完名字后再重新整理。

---

## 常见问题

### 1. 它会不会把我原来的文件删掉

正常整理是“移动到你设定好的目录”，不是直接删除。  
但你最好先用 `--dry-run` 看一遍结果。

### 2. 文件名识别错了怎么办

先去网页里的“解析测试”页面看看识别结果。  
如果识别得不对，再改 `parser-rules.yaml` 里的 `custom_words` / `custom_release_groups`，或者手动修文件名。

### 3. TMDB 没找到怎么办

通常是文件名太乱、别名太多、年份不准，或者动漫命名比较特别。  
先把文件名改得更清楚一点，再重新跑一次。

### 4. 我只想整理，不想下载海报

可以加：

```bash
python pipeline.py --no-images
```

### 5. 我只想试试看，不想真的移动文件

加：

```bash
python pipeline.py --dry-run
```

---

## 配置完整示例

如果你已经跑起来了，想进一步细调，再看这段完整配置：

```yaml
storage:
  primary: "google_drive"   # 当前使用的网盘名字：google_drive / pan115

webui:
  username: "admin"
  password: ""
  secret_key: ""
  token_expire_hours: 24
  webhook_secret: ""

tmdb:
  api_key: ""
  language: "zh-CN"
  proxy: ""
  timeout: 10

drive:
  credentials_json: "config/credentials.json"
  token_json: "config/token.json"
  scan_folder_id: ""
  root_folder_id: ""
  movie_root_id: ""
  tv_root_id: ""

u115:
  client_id: "100197847"
  token_json: "config/115-token.json"
  session_json: "config/115-device-session.json"
  download_folder_id: ""
  root_folder_id: ""
  movie_root_id: ""
  tv_root_id: ""

pipeline:
  skip_tmdb: false
  move_on_tmdb_miss: false
  dry_run: false

aria2:
  enabled: true
  host: "127.0.0.1"
  port: 6800
  path: "/jsonrpc"
  secret: ""
  secure: false

telegram:
  bot_token: ""
  chat_id: ""
  debounce_seconds: 60
```

---

## 项目结构

```text
Meta2Cloud/
├─ pipeline.py
├─ storage/
│  ├─ base.py
│  ├─ google_drive.py
│  └─ pan115.py
├─ mediaparser/
├─ drive/
├─ u115pan/
├─ webui/
├─ frontend/
├─ scripts/
├─ config/
├─ docker-compose.yml
├─ Dockerfile
└─ requirements.txt
```

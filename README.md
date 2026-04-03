# Metadata2GD

> 自动为 Google Drive 上的媒体文件查询 TMDB 元数据、生成 NFO、整理目录结构，并发送 Telegram 入库通知。

---

## 功能概览

| 功能 | 说明 |
|---|---|
| **递归扫描** | 扫描 Drive 指定文件夹（含所有子文件夹）中的视频文件 |
| **文件名解析** | 识别标题、年份、季集号、字幕组，支持自定义规则 |
| **TMDB 元数据** | 查询整剧/整片信息，TV 额外获取单集标题、简介、导演 |
| **NFO 生成** | 生成 Plex / Infuse / Emby 兼容的 `episode.nfo` / `tvshow.nfo` / `season.nfo` |
| **目录整理** | 在 Drive 按标准结构幂等创建文件夹，将文件移动并标准化命名 |
| **封面上传** | 下载 TMDB 封面图，上传 `poster.jpg` / `fanart.jpg` / `season01-poster.jpg` |
| **Telegram 通知** | 带封面图的入库通知，多集合并为一条消息，支持防抖延时 |
| **Webhook 触发** | 提供 HTTP Server，配合 Aria2 + Rclone 上传完成后自动触发 |

### Drive 目录结构

```
📁 剧集根目录/
└─ 📁 Breaking Bad (2008)/
   ├─ tvshow.nfo
   ├─ poster.jpg
   ├─ fanart.jpg
   ├─ season01-poster.jpg
   └─ 📁 Season 01/
      ├─ season.nfo
      ├─ Breaking Bad - S01E01.mkv
      └─ Breaking Bad - S01E01.nfo

📁 电影根目录/
└─ 📁 Inception (2010)/
   ├─ Inception (2010).mkv
   ├─ Inception (2010).nfo
   ├─ poster.jpg
   └─ fanart.jpg
```

---

## 部署（Docker，推荐）

### 1. 准备认证文件

在项目目录创建 `metadata2gd-config/` 文件夹，放入以下文件：

```
metadata2gd-config/
├─ config.yaml          # 主配置文件（从 config/config.yaml 复制后修改）
├─ credentials.json     # Google OAuth2 凭据（auth_mode = oauth2 时）
├─ token.json           # OAuth2 Token（首次运行后自动生成）
└─ service_account.json # Service Account JSON（auth_mode = service_account 时）
```

**获取 Google Drive API 凭据：**
- 进入 [Google Cloud Console](https://console.cloud.google.com/) → API 和服务 → 凭据
- 创建 **OAuth2 客户端 ID**（桌面应用）并下载 → 保存为 `credentials.json`
- 或创建**服务账号**并下载 JSON 密钥 → 保存为 `service_account.json`（记得将 Drive 目标文件夹共享给服务账号邮箱）

### 2. 首次 OAuth2 授权（仅 oauth2 模式）

```bash
docker run --rm -it \
  -v $(pwd)/metadata2gd-config:/app/config \
  benz1/metadata2gd:latest \
  python pipeline.py --dry-run
```

浏览器会弹出 Google 授权页面，完成后 `token.json` 自动写入。

### 3. 配置 `config.yaml`

```bash
cp config/config.yaml metadata2gd-config/config.yaml
# 然后编辑 metadata2gd-config/config.yaml
```

最少需要填写的字段：

```yaml
tmdb:
  api_key: "你的_TMDB_API_Key"   # https://www.themoviedb.org/settings/api

drive:
  scan_folder_id: "Drive_扫描文件夹_ID"  # 新文件上传到这里

organizer:
  root_folder_id: "Drive_整理目标根文件夹_ID"
  movie_root_id:  "电影专用文件夹_ID"   # 可选，留空用 root_folder_id
  tv_root_id:     "剧集专用文件夹_ID"   # 可选，留空用 root_folder_id

telegram:
  bot_token: "你的_Bot_Token"   # 从 @BotFather 获取
  chat_id:   "你的_Chat_ID"
```

> **获取 Drive 文件夹 ID**：在 Drive 网页版打开文件夹，URL 末尾即为 ID
> `https://drive.google.com/drive/folders/1AbCdEfGhIjKlMn` → ID = `1AbCdEfGhIjKlMn`

### 4. 启动服务

```bash
docker compose up -d metadata2gd
```

查看日志：

```bash
docker logs -f metadata2gd
```

---

## 配置参考

完整配置项说明（`config.yaml`）：

```yaml
# ── TMDB ──────────────────────────────────────────
tmdb:
  api_key: ""          # TMDB v3 API Key（必填）
  language: "zh-CN"   # 返回语言，支持 zh-CN / zh-TW / en-US / ja-JP 等
  proxy: ""            # HTTP 代理，示例：http://127.0.0.1:7890
  timeout: 10          # 请求超时（秒）

# ── 解析器 ─────────────────────────────────────────
parser:
  custom_words:
    # 屏蔽词：从标题中删除
    # - "国语配音"
    # 替换词：旧词 => 新词（支持正则）
    # - "OVA => SP"
    # 集偏移：前缀 <> 后缀 >> EP+偏移量
    # - "第 <> 集 >> EP-1"

  custom_release_groups:
    # 追加到内置字幕组列表（内置已含数百个主流字幕组）
    # - "MyFansub"

# ── Google Drive ───────────────────────────────────
drive:
  auth_mode: "oauth2"               # "oauth2" 或 "service_account"
  credentials_json: "config/credentials.json"
  token_json: "config/token.json"
  service_account_json: "config/service_account.json"
  scan_folder_id: ""                # 扫描的源文件夹 ID（必填）

# ── 整理器 ─────────────────────────────────────────
organizer:
  root_folder_id: ""   # 整理目标根文件夹 ID（必填）
  movie_root_id:  ""   # 电影专用子目录 ID（可选）
  tv_root_id:     ""   # 剧集专用子目录 ID（可选）

# ── 流水线 ─────────────────────────────────────────
pipeline:
  skip_tmdb: false          # true = 只整理文件夹，不查 TMDB / 不生成 NFO
  move_on_tmdb_miss: true   # TMDB 找不到时是否仍然移动文件
  dry_run: false            # true = 只打印计划，不实际操作 Drive

# ── Telegram ───────────────────────────────────────
telegram:
  bot_token: ""
  chat_id: ""
  debounce_seconds: 60   # 防抖延时（秒）。多集批量入库时，最后一次触发
                         # 后等待该时间再运行，所有集合并为一条通知。
                         # 设为 0 关闭防抖（立即触发）。
```

---

## 使用方式

### 方式一：手动触发（一次性整理）

```bash
# 正式运行
docker exec metadata2gd python pipeline.py

# 预览（不实际操作 Drive）
docker exec metadata2gd python pipeline.py --dry-run

# 跳过 TMDB，只整理文件夹
docker exec metadata2gd python pipeline.py --no-tmdb

# 跳过图片下载
docker exec metadata2gd python pipeline.py --no-images
```

### 方式二：Webhook 自动触发（配合 Aria2 + Rclone）

`metadata2gd` 容器内运行 HTTP Server，监听 `POST /trigger`。

在 Rclone 的 `upload.sh`（P3TERX aria2 方案）中，上传完成后自动调用：

```bash
curl -sf -X POST http://localhost:46562/trigger \
     -H "Content-Type: application/json" \
     -d '{"path": "/path/to/uploaded/file"}'
```

**防抖机制**：配置 `debounce_seconds: 60` 后，批量上传13集时，最后一集上传完毕 60 秒后才触发一次 pipeline，TG 只收到一条包含所有集数的通知。

**健康检查：**

```bash
curl http://localhost:46562/health
# {"status": "ok"}
```

---

## Telegram 通知示例

### 正常入库

```
📺 Breaking Bad (2008)

Season 01：
  • S01E01  试播集
  • S01E03  …And the Bag's in the River
```

附带季封面图。

### TMDB 未找到元数据

```
📺 TMDB 未找到元数据，文件未整理
• 假面骑士强人.S01E30.1080p.mkv
• 假面骑士强人.S01E31.1080p.mkv

请检查文件名后手动触发重新整理
```

显示原始文件名，方便在 Drive 中定位文件。

---

## 与 Aria2-Pro + Rclone 配合使用

`docker-compose.yml` 已包含 `aria2-pro` 和 `metadata2gd` 两个服务，均使用 `network_mode: host`，通过 `localhost:46562` 互相通信。

### 整体流程

```
Aria2 下载完成
  → Rclone 上传到 Drive（upload.sh）
    → POST /trigger 通知 metadata2gd
      → 防抖等待（debounce_seconds）
        → Pipeline 扫描 Drive
          → TMDB 查询 → NFO 生成 → 文件整理
            → Telegram 入库通知
```

### 替换 upload.sh

P3TERX 的 [aria2.conf](https://github.com/P3TERX/aria2.conf) 方案使用 `upload.sh` 在 Rclone 上传完成后执行自定义逻辑。需要将本仓库提供的 `upload.sh` 复制到 aria2 配置目录，替换原版文件。

```bash
# 将 upload.sh 复制到 aria2 配置目录（覆盖原版）
cp upload.sh aria2-config/upload.sh
```

> **注意**：本仓库的 `upload.sh` 基于 P3TERX 原版修改，在上传完成后额外调用了 `RUN_METADATA2GD` 函数触发整理流水线，其余逻辑与原版完全一致。

**新增的关键函数（upload.sh 第 93–118 行）：**

```bash
RUN_METADATA2GD() {
    local WEBHOOK_URL="http://localhost:46562/trigger"

    # 仅在上传成功时触发
    [[ "${UPLOAD_SUCCESS}" != "1" ]] && return 0

    curl -sf --max-time 10 \
        -X POST "${WEBHOOK_URL}" \
        -H "Content-Type: application/json" \
        -d "{\"path\": \"${REMOTE_PATH}\"}"
}
```

上传失败时不会触发整理，避免处理不完整文件。

### 一键启动全套服务

```bash
docker compose up -d
```

---

## 本地开发

```bash
# 安装依赖
pip install -r requirements.txt

# 运行流水线（需已有 config/config.yaml 和认证文件）
python pipeline.py --dry-run

# 构建镜像
docker build -t benz1/metadata2gd:latest .
```

# Metadata2GD

> 自动为 Google Drive 上的媒体文件查询 TMDB 元数据、生成 NFO、整理目录结构，并发送 Telegram 入库通知。  
> 现已内置 **Web UI**，可在浏览器中可视化查看媒体库状态。

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
| **Webhook 触发** | HTTP 接口 `POST /trigger`，配合 Aria2 + Rclone 上传完成后自动触发 |
| **Web UI** | 浏览器可视化媒体库、原生 Aria2 下载大盘、实时流式日志及解析沙盒 |
| **JWT 登录鉴权** | Web UI 及 API 均受 JWT 保护，支持 Webhook Secret 独立校验 |
| **TMDB 本地缓存** | SQLite 缓存减少重复请求，支持 TTL 自动过期与负缓存 |

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
├─ config.yaml          # 主配置文件（从 config/config.example.yaml 复制后修改）
├─ credentials.json     # Google OAuth2 凭据
└─ token.json           # OAuth2 Token（首次运行后自动生成）
```

**获取 Google Drive API 凭据：**
- 进入 [Google Cloud Console](https://console.cloud.google.com/) → API 和服务 → 凭据
- 创建 **OAuth2 客户端 ID**（桌面应用）并下载 → 保存为 `credentials.json`

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
cp config/config.example.yaml metadata2gd-config/config.yaml
# 然后编辑 metadata2gd-config/config.yaml
```

最少需要填写的字段：

```yaml
webui:
  username: "admin"          # Web UI 登录用户名
  password: "your_password"  # Web UI 登录密码（必填，否则无法登录）
  webhook_secret: ""         # /trigger 端点密钥，留空则不校验（内网可留空）

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
docker compose up -d
```

查看日志：

```bash
docker logs -f metadata2gd
```

### 5. 访问 Web UI

服务启动后，浏览器访问：

```
http://localhost:38765
```

使用 `config.yaml` 中配置的用户名和密码登录。

---

## Web UI

Web UI 基于 React + Vite 构建，提供以下页面：

| 页面 | 功能 |
|---|---|
| **登录** | JWT 鉴权登录，Token 默认 24 小时有效 |
| **下载大盘** | 集成 Aria2 RPC 提供原生下载中心，支持批量控制、多状态队列、全局限速统计与搜索过滤 |
| **媒体库** | 电影 / 剧集卡片展示，含 TMDB 海报、评分、概述 |
| **剧集详情** | 按季展示每集入库状态，区分已入库（✅）与未入库 |
| **解析测试** | 可视化交互沙盒，可实时验证自定义解析规则、字幕组切分与 TMDB 匹配结果 |
| **实时日志** | 流式呈现后端 Pipeline 运行日志，提供带有系统级弹窗的沉浸式自动化观测体验 |
| **配置** | 查看全局服务端配置状态，附带一键发起重建全盘快照功能 |

媒体库数据来自本地 SQLite 快照（`config/data/library.db`），无需每次访问都扫描 Drive。点击「刷新媒体库」时才重新扫描 Drive 并更新快照。

---

## 配置参考

完整配置项说明（`config.yaml`）：

```yaml
# ── Web UI & 认证 ──────────────────────────────────
webui:
  username: "admin"          # 登录用户名，默认 admin
  password: ""               # 登录密码（必填，留空将无法登录）
secret_key: ""             # JWT 签名密钥；留空则自动生成并保存到 config/data/.jwt_secret
                             # 生产环境建议指定固定值，避免重启后 Token 失效
  token_expire_hours: 24     # JWT Token 有效期（小时）
  webhook_secret: ""         # /trigger 端点密钥；留空则不校验（仅内网部署时可留空）

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
  credentials_json: "config/credentials.json"
  token_json: "config/token.json"
  scan_folder_id: ""                # 扫描的源文件夹 ID（必填）

# ── 整理器 ─────────────────────────────────────────
organizer:
  root_folder_id: ""   # 整理目标根文件夹 ID（必填）
  movie_root_id:  ""   # 电影专用子目录 ID（可选）
  tv_root_id:     ""   # 剧集专用子目录 ID（可选）

# ── 流水线 ─────────────────────────────────────────
pipeline:
  skip_tmdb: false          # true = 只整理文件夹，不查 TMDB / 不生成 NFO
  move_on_tmdb_miss: false  # TMDB 找不到时是否仍然移动文件 (默认 false 保证安全)
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

## API 接口

所有 `/api/*` 接口均需在请求头中携带 JWT Token（登录后获得）：

```
Authorization: Bearer <token>
```

| 方法 | 路径 | 说明 |
|---|---|---|
| `POST` | `/api/auth/login` | 登录，返回 JWT token（无需鉴权） |
| `GET` | `/api/auth/me` | 验证 Token，返回当前用户名 |
| `POST` | `/api/auth/logout` | 登出（前端清除 Token） |
| `GET` | `/api/library` | 从本地快照读取完整媒体库 |
| `POST` | `/api/library/refresh` | 扫描 Drive，更新本地快照 |
| `GET` | `/api/library/movies` | 仅获取电影列表（实时扫描） |
| `GET` | `/api/library/tv` | 仅获取剧集列表（实时扫描） |
| `GET` | `/api/tv/{tmdb_id}` | 单部剧集详情（含季集入库状态） |
| `GET` | `/api/stats` | 统计信息（总数、完成率等） |
| `GET` | `/api/cache/stats` | TMDB 缓存使用情况 |
| `POST` | `/api/cache/evict` | 手动清理过期缓存 |
| `POST` | `/trigger` | Webhook：触发 pipeline（需 Webhook Secret） |
| `GET` | `/api/pipeline/status` | 查询 pipeline 运行状态（需登录） |
| `GET` | `/health` | 健康检查接口（无需鉴权） |

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

`metadata2gd` 容器内运行 FastAPI Server，监听 `POST /trigger`（端口 `38765`）。

在 Rclone 的 `upload.sh`（P3TERX aria2 方案）中，上传完成后自动调用：

```bash
curl -sf -X POST http://localhost:38765/trigger \
     -H "Content-Type: application/json" \
     -H "X-Webhook-Secret: your_webhook_secret" \
     -d '{"path": "/path/to/uploaded/file"}'
```

若 `webui.webhook_secret` 配置为空，则无需传 `X-Webhook-Secret` 头（适合内网部署）。

**防抖机制**：配置 `debounce_seconds: 60` 后，批量上传 13 集时，最后一集上传完毕 60 秒后才触发一次 pipeline，Telegram 只收到一条包含所有集数的通知。

**健康检查：**

```bash
curl http://localhost:38765/health
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

`docker-compose.yml` 已包含 `aria2-pro` 和 `metadata2gd` 两个服务，均使用 `network_mode: host`，通过 `localhost:38765` 互相通信。

### 整体流程

```
Aria2 下载完成
  → Rclone 上传到 Drive（upload.sh）
    → POST /trigger 通知 metadata2gd
      → 防抖等待（debounce_seconds）
        → Pipeline 扫描 Drive
          → TMDB 查询 → NFO 生成 → 文件整理
            → Telegram 入库通知
              → 自动刷新 Web UI 快照
```

### 替换 upload.sh

P3TERX 的 [aria2.conf](https://github.com/P3TERX/aria2.conf) 方案使用 `upload.sh` 在 Rclone 上传完成后执行自定义逻辑。将本仓库提供的示例脚本复制后修改：

```bash
# 复制示例脚本
cp scripts/upload.example.sh aria2-config/upload.sh

# 编辑 upload.sh，填入你的 webhook_secret（与 config.yaml 保持一致）
# METADATA2GD_WEBHOOK_SECRET="your_webhook_secret_here"
```

> **注意**：`scripts/upload.example.sh` 基于 P3TERX 原版修改，在上传完成后额外调用了 `RUN_METADATA2GD` 函数触发整理流水线，其余逻辑与原版完全一致。

**新增的关键函数（upload.sh）：**

```bash
RUN_METADATA2GD() {
    local WEBHOOK_URL="http://localhost:38765/trigger"
    local WEBHOOK_SECRET="${METADATA2GD_WEBHOOK_SECRET}"

    # 仅在上传成功时触发
    [[ "${UPLOAD_SUCCESS}" != "1" ]] && return 0

    local CURL_ARGS=(-sf --max-time 10 -X POST "${WEBHOOK_URL}" \
        -H "Content-Type: application/json" \
        -d "{\"path\": \"${REMOTE_PATH}\"}")

    # 有密钥时加入鉴权 header
    [[ -n "${WEBHOOK_SECRET}" ]] && CURL_ARGS+=(-H "X-Webhook-Secret: ${WEBHOOK_SECRET}")

    curl "${CURL_ARGS[@]}"
}
```

上传失败时不会触发整理，避免处理不完整文件。

### 一键启动全套服务

```bash
docker compose up -d
```

---

## 本地开发

### 后端

```bash
# 安装依赖
pip install -r requirements.txt

# 运行流水线（需已有 config/config.yaml 和认证文件）
python pipeline.py --dry-run

# 启动 Web UI 后端
uvicorn webui.api:app --host 0.0.0.0 --port 38765 --reload
```

### 前端

```bash
cd frontend

# 安装依赖
npm install

# 开发模式（代理到 localhost:38765）
npm run dev

# 构建生产包
npm run build
# 产物输出到 frontend/dist/，由后端静态托管
```

### 构建镜像

```bash
# 构建前端
cd frontend && npm run build && cd ..

# 构建 Docker 镜像
docker build -t benz1/metadata2gd:latest .
```

---

## 项目结构

```
Metadata2GD/
├─ pipeline.py              # 核心整理流水线
├─ mediaparser/             # 文件名解析模块
│  └─ config.py             # 配置类（含 WebAuthConfig）
├─ drive/                   # Google Drive 客户端
├─ webui/                   # Web UI 后端（FastAPI）
│  ├─ api.py                # REST API + Webhook + 鉴权
│  ├─ library_store.py      # SQLite 媒体库快照存储
│  └─ tmdb_cache.py         # TMDB 响应 SQLite 缓存
├─ frontend/                # Web UI 前端（React + Vite）
│  └─ src/
│     ├─ components/        # LoginPage / LibraryPage / ConfigPage 等
│     └─ api.js             # 前端 API 封装
├─ config/
│  └─ config.example.yaml   # 配置文件模板（不含真实密钥）
├─ scripts/
│  ├─ upload.example.sh     # Rclone 上传脚本示例（不含真实密钥）
│  ├─ build.sh              # 镜像构建脚本
│  ├─ backfill_nfo.py       # 补录 NFO 工具
│  └─ fix_existing.py       # 修复已有文件命名工具
├─ config/
│  └─ data/                 # 运行时数据（自动创建，不提交 git）
│  ├─ library.db            # 媒体库快照
│  ├─ tmdb_cache.db         # TMDB 缓存
│  └─ .jwt_secret           # 自动生成的 JWT 密钥
├─ docker-compose.yml
├─ Dockerfile
└─ requirements.txt
```

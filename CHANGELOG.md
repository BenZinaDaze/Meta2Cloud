# 更新日志

本文件记录 Meta2Cloud 每个版本的显著变更，格式参考
[Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。
发版操作手册见 [doc/release.md](doc/release.md)。

> 注：早期 v1.0.2 ~ v1.1.7 的原始提交历史曾被重写（快照式平行历史），
> 其条目由版本树比对还原；实际功能均已包含于主线 v1.1.8 及之后版本。

## [v2.3.1] - 2026-10-17

### ✨ 新增

- 通知模块导出复用 + 分集折叠块 + 标签净化 (`8dc52f1`)
- 富入库通知（整理 + STRM 合并为一条） (`4dc7a6b`)
- Telegram 测试通知 + 配置页 tab 重排 (`f8331e5`)
- 文件管理文件夹支持复制 ID，修改时间列居中 (`6d197f5`)

### 🐛 修复

- 重复目录合并支持从最旧目录名推导钉扎名，跳过原因可见 (`00e86d8`)

### ♻️ 优化重构

- 按职责拆分 pipeline.go（1639 → 941 行） (`af89472`)


## [v2.3.0] - 2026-10-17

### ✨ 新增

- 引入 solid-glass 玻璃材质主题（保留潮汐绿配色） (`44555b5`)
- 115 未授权时隐藏不可用入口，授权状态随导航自动复查 (`e7743cf`)
- 媒体详情直链 /movies/:tmdbId、/tv/:tmdbId，入库记录联动详情 (`27a1517`)
- TMDB 译名钉扎防目录分叉，自动合并重复目录并新增整理名称页 (`695aba3`)

### ♻️ 优化重构

- 引入 react-router 替换 state 驱动的静态页面切换 (`9639da4`)


## [v2.2.1] - 2026-10-16

### 🐛 修复

- authDeviceCode 数字型 time 解析为空导致扫码授权 key invalid (40199002) (`fc3cd46`)
- 发版脚本非交互环境自动确认推送，修复 read EOF 静默中止漏推送 (`aee0930`)

### 📝 文档

- README 补充 STRM 直链播放说明，删除废弃的 standalone compose 配置 (`7025cbb`)
- compose 预置 STRM 输出目录卷（注释），标注 local_path 须为容器内路径 (`7f86281`)


## [v2.2.0] - 2026-10-16

### ✨ 新增

- STRM 播放记录、共享 QPS 限流与伴随文件下载 (`f0e168c`)
- 115 网盘 STRM 生成与 302 直链播放 (`2069c20`)


## [v2.1.2] - 2026-10-16

### 🐛 修复

- 整理流程未注入入库历史存储，RSS 下载整理后入库记录缺失 (`2c31d70`)

### ♻️ 优化重构

- 删除 aria2 集成，推送与下载入口收敛为 115 云下载 (`1c14b97`)

### 🧹 杂项

- 登录页移除账号密码配置路径提示 (`792e1a7`)


## [v2.1.1] - 2026-10-15

### 🐛 修复

- 订阅创建/更新静默丢失 tmdb_id 与 poster_url (`576a935`)

### 🧹 杂项

- 建立完整发版流程 (`c5aa885`)


## [v1.0.1] - 2026-04-06

### 新增

- implement TMDB-based global scraper search with detailed metadata and resource detection UI (`e547731`)
- implement responsive Sidebar component with navigation and download status tracking (`aa5ce9b`)
- implement sidebar component and add CI/CD infrastructure for Docker and build automation (`3487d2c`)
- implement frontend API layer and core management components for Aria2 downloads and pipeline control (`17aee2e`)
- initialize frontend project structure with Tailwind CSS, sidebar navigation, and core UI components (`0d2afde`)
- implement media metadata parsing engine and add detail modal UI for library management (`ad36110`)
- add logs and downloads management pages with real-time pipeline status monitoring and API integration (`0e1f65a`)
- initialize API module for web UI communication (`0330c78`)
- add API module for webui backend communication (`c9d9720`)
- add GitHub Actions workflow for automated Docker image publishing (`3322cb7`)
- （cache）整理流程优先读取本地 TMDB 缓存，并将剧集缓存时长调整为 6 天 (`e284a8a`)
- 下载详情文件支持一键打开解析测试 前端： - 下载详情文件列表新增解析按钮 - 点击文件后使用文件名打开解析测试弹窗 - 解析测试弹窗支持接收初始文件名并自动执行解析 (`2300e6e`)
- （webui）新增解析测试弹窗并展示 TMDB 预览 (`330f917`)
- 将 aria2 连接状态收敛为侧边栏小圆点提示 (`f1f7330`)
- 新增结构化日志中心并支持按天轮转保留 (`ddaad8b`)
- 新增原生 aria2 下载管理页并优化交互 前端： - 侧边栏新增“下载管理”及下载中/等待中/已停止子项 - 新增下载管理页面，支持链接/Torrent 添加、任务列表与详情弹窗 - aria2 状态改为全局轮询，侧边栏显示任务数量与连… (`96095f4`)
- 增强 DriveClient 的请求执行，添加重试机制；更新前端标题和图标；新增 BrandMark 组件 (`ca548e5`)
- 新增 WebUI 前端 + 重构后端架构 (`af2b721`)
- 防抖合并推送 + 修复 TMDB 未命中通知 pipeline: - 递归扫描 scan_folder_id 子文件夹（原仅扫顶层） - _NotifyItem 新增 original_name 字段，no_tmdb 通知改用原始文件名 … (`08bbd2e`)

### 修复

- 更新 dockerfile 和目录 (`5e53b28`)
- （webui）修复解析测试弹窗并展示 TMDB 预览 (`babefd8`)

### 优化重构

- refresh frontend layout and rename pipeline settings (`177b3e9`)

### 文档

- 更新文档 (`ec87ef7`)
- （config）add debounce_seconds to template config 在 config/.config.yaml 模板文件的 telegram 段新增 debounce_seconds 配置项及说明注释（默认 6… (`97437a6`)
- readme.md (`551ef7e`)

### 杂项

- 端口由 8765 改为 38765，新增 data 目录持久化挂载 (`9a7ef4c`)

### 其他

- init (`10e9721`)

## [v1.0.2] - 2026-04-06

### 新增

- add Sidebar component with navigation, download status, and version checking (`378da3e`)

### 修复

- 更新dockerfile，更新目录 (`0c87cb1`)

### 文档

- 更新文档 (`5a3a5fc`)
- readme.md (`967ff21`)

### 杂项

- update Dockerfile base image and dependencies (`38c1aff`)

### 其他

- Refresh frontend layout and rename pipeline settings (`eeefe0d`)

## [v1.0.3] - 2026-04-07

### 新增

- Mikan 资源检索支持中文/日文别名回退搜索 (`734ec99`)
- 重构爬虫搜索结果页，支持按字幕组逐组加载与折叠展开 (`06f2423`)

### 修复

- 点击「检索资源」时强制重新搜索，不再使用缓存 (`a779813`)

## [v1.0.4] - 2026-04-07

### 新增

- Mikan 别名搜索改为按需请求，新增专用 TMDB 别名接口 (`4bbbf95`)

## [v1.0.5] - 2026-04-08

### 修复

- 新增 TMDB 别名实时接口，主标题搜索失败时自动回退别名检索 (`cce643e`)

## [v1.0.6] - 2026-04-08

### 新增

- 重构资源检索结果页面，支持按番剧标题 Tab 分页及字幕组两层分类展示 (`6a08f9b`)
- 新增新番列表页面，点击卡片跳转资源检索自动 TMDB 搜索 (`324ea1e`)

## [v1.0.7] - 2026-04-09

### 新增

- 支持单条目媒体元数据独立刷新与 TMDB 自动搜索补偿 (`697a51e`)

### 修复

- 单独刷新只有tmdbid的问题 (`b4b2c78`)
- 删除服务账号认证方式，只保留OAuth2 方式 (`97c10ef`)

## [v1.1.0] - 2026-04-15

### 新增

- 115自动整理 (`ae40c1c`)
- 接入 pan115 存储抽象并统一项目命名为 Meta2Cloud (`eb9eb9f`)
- 新增115授权和115云下载 (`6b7230c`)
- 增加aira2开关，增加授权状态 (`00c6551`)

### 修复

- 优化番剧列表 (`410bb07`)
- 115授权 (`fa43d66`)
- qrcode (`d73b27a`)
- docker build (`48da8b9`)
- rename (`402cd84`)
- bug - App.jsx 里把 aria2Enabled 初始值从 true 改成“未知态” - 在未知态下不轮询、不发 overview 请求 (`f914616`)
- config缓存 (`fb79ebe`)
- 删除一些不必要脚本 (`ab176ca`)

### 文档

- readme (`a2ac00f`)

### 其他

- feat: 调整媒体库目录配置并完善 aria2/115 下载交互 - 移除 organizer 配置，统一将媒体库目录映射归入 drive - 新增 115 云下载目录与媒体库目录配置，并同步更新示例配置与 README - 调整配置页… (`61424a2`)

## [v1.1.1] - 2026-04-15

### 修复

- qps (`d291eca`)

## [v1.1.2] - 2026-04-16

### 修复

- 115 token runtime (`5caf693`)
- clear task (`cc79ba0`)
- webui (`9758420`)
- session (`c75f9b4`)

## [v1.1.3] - 2026-04-16

### 新增

- 自定义规则页面 (`425f41b`)

## [v1.1.4] - 2026-04-16

### 新增

- 新增夸克网盘(Quark Cloud)存储后端 (`3f55262`)
- 移动端适配 (`4bc1e9a`)

### 修复

- api (`56f8e8e`)

## [v1.1.5] - 2026-04-17

### 新增

- 订阅 (`964af07`)

### 修复

- 数据库优化，订阅前端优化 (`7c4df53`)

## [v1.1.6] - 2026-04-17

### 修复

- 空值保护 (`460b6b0`)
- 解析测试页面 (`82a0690`)

## [v1.1.7] - 2026-04-17

### 修复

- UI (`728d2fa`)

## [v1.1.8] - 2026-04-18

### 新增

- 115 转存 api 测试完成 (`7b386ae`)
- 订阅 (`7eba456`)
- 移动端适配 (`a963bc9`)
- 调整媒体库目录配置并完善 aria2/115 下载交互 (`80d2c5f`)
- 增加 aria2 开关和授权状态 (`003cd88`)

### 修复

- 优化别名搜索过滤逻辑 (`b6eb3e9`)
- UI (`d7626cf`)
- 空值保护 (`42a71c8`)
- 解析测试页面 (`28e2cf5`)
- api (`35888f6`)
- 115 token runtime (`c0e7a81`)
- clear task (`b34d859`)
- webui (`5a74e8c`)
- session (`635858c`)
- qps (`9d7ac5b`)
- 115 授权 (`5e5fc22`)
- qrcode (`cdd0f3a`)
- docker build (`2f5f1ec`)
- rename (`58dc2d4`)
- config 缓存 (`2101d86`)
- 更新 dockerfile 和目录 (`5e53b28`)

### 优化重构

- refresh frontend layout and rename pipeline settings (`177b3e9`)

### 文档

- readme (`f087add`)
- 更新文档 (`ec87ef7`)
- readme.md (`551ef7e`)

## [v1.1.9] - 2026-04-18

### 修复

- 添加 TG 通知调试日志 (`2b97d5e`)

## [v1.1.10] - 2026-04-18

### 修复

- 优化 mikan RSS 订阅请求稳定性 (`42c1804`)

## [v1.1.11] - 2026-04-19

### 新增

- 根据配置状态动态显示侧边栏菜单项 (`5695378`)

### 修复

- 检查 TMDB 缓存数据完整性，缺失字段时重新请求 (`939a68f`)

## [v1.1.12] - 2026-04-19

### 修复

- 改进 115 自动整理轮询失败的错误日志 (`06afa43`)

### 优化重构

- 将 pipeline 和 organizer 移入 core 模块 (`024837c`)

### 文档

- 更新 README 中 pipeline 命令为 python -m core (`d236755`)

## [v1.1.13] - 2026-04-22

### 新增

- 在移动端导航中显示订阅列表入口 (`5b85a0f`)

### 杂项

- 迁移到 uv 包管理器 (`3034d3a`)

## [v1.1.14] - 2026-04-23

### 新增

- 添加同名视频替换配置功能 (`927f0d9`)
- 添加独立部署的 docker-compose.standalone.yml (`a405e93`)

## [v1.2.0] - 2026-04-25

### 优化重构

- 使用 TypeScript 重构前端并替换旧版本 (`63a442a`)
- 将颜色系统迁移至 OKLCH 并合并重复变量 (`511559b`)

## [v1.2.1] - 2026-04-25

### 优化重构

- 将订阅删除确认从原生 confirm 替换为 AlertDialog 组件 (`44d2298`)

### 样式

- 优化 Sonner Toast 组件的主题适配 (`6710b87`)

## [v1.2.2] - 2026-04-25

### 新增

- 添加独立字幕整理脚本 (`f3aedc4`)
- 添加外挂字幕自动整理功能 (`8eebc23`)

## [v1.2.3] - 2026-04-26

### 修复

- 优化 115 网盘认证错误处理和扫码流程 (`2a10581`)

### 文档

- 更新网易爆米花支持平台为全平台 (`82009f7`)
- 更新截图、添加致谢和 GPL v3 许可证 (`f385fff`)
- 添加技术栈图标到 README (`2603487`)
- 重写 README 面向用户，推荐直连网盘播放器 (`135964b`)

## [v1.2.4] - 2026-04-26

### 新增

- 添加入库历史记录功能和本地媒体库同步脚本 (`5a0ea8f`)
- 添加手动创建订阅功能和 RSS 解析 (`ce79fd8`)

### 样式

- 为 toast 组件添加关闭按钮 (`6ea6e74`)
- 优化订阅卡片顶部标签背景样式 (`8414a38`)

## [v1.2.5] - 2026-04-27

### 修复

- 修复移动端新番列表加载失败问题 (`b7cde64`)
- 优化后端异常日志，网络错误不再 dump 完整 traceback (`e4bf37c`)
- 将 Docker 构建缓存从 GHA cache 切换到 Docker Hub registry (`f265511`)

### 样式

- 优化下载任务卡片中的删除按钮布局 (`07c9766`)

## [v1.2.6] - 2026-04-28

### 修复

- 刷新 TV 元数据时保留入库状态 (`72adc2b`)
- 修复媒体库刷新按钮无法更新总集数的问题 (`fd3d598`)

### 优化重构

- 统一 seasons 数据生成逻辑，修复刷新元数据丢失 episodes 问题 (`cf7d24b`)

## [v1.2.7] - 2026-04-28

### 新增

- 媒体库支持排序和增量刷新 (`7709eac`)

### 样式

- 排序下拉框替换为 Radix Select，统一去掉 focus 高亮并调整按钮高度 (`1b1b864`)

## [v1.2.8] - 2026-04-29

### 新增

- WebSocket 实时推送 pipeline 日志，日志存储迁移到 SQLite (`243316b`)

### 修复

- Docker 构建改用 npm 替代 pnpm (`59a4e25`)

### 优化重构

- API 路由改为 REST 风格 (`62f844a`)

### 样式

- 移除数字输入框的调节按钮 (`13491f6`)

### CI

- 升级 GitHub Actions 到 Node 24 兼容版本，清理旧前端存档 (`82ea977`)

## [v1.2.9] - 2026-04-30

### 新增

- 新增云盘文件管理页面，支持浏览、搜索、排序、批量操作 (`f71ba8c`)

### 修复

- 修复订阅卡片悬停动画时图片边缘露出白边 (`f21e898`)

## [v1.2.10] - 2026-05-07

### 修复

- 统一处理不同云盘返回的 modified_time 格式 (`f29d1bc`)

## [v1.2.11] - 2026-05-07

### 新增

- 支持配置 TMDB 图片代理域名 (`ffc7bed`)

## [v1.2.12] - 2026-05-09

### 新增

- 添加跳过元数据上传配置项 (`2ce7d27`)
- 添加返回顶部按钮并修复页面滚动布局 (`f1443c8`)

### 优化重构

- 移除一次性数据库迁移代码 (`19a23b8`)

## [v1.2.13] - 2026-05-09

### 修复

- 跳过元数据上传仅跳过单集 NFO，保留剧集/季 NFO 和封面 (`e67a393`)
- 剧集增量扫描使用 Season 文件夹修改时间 (`aa87ca7`)

## [v1.2.14] - 2026-05-10

### 新增

- 添加移出媒体库功能并优化前端加载 (`1041e2f`)

## [v1.2.15] - 2026-05-10

### 修复

- skip_metadata_upload 改为仅上传 tvshow.nfo (`0d9221c`)

## [v1.2.16] - 2026-05-11

### 修复

- skip_metadata_upload 电影仍上传同名 NFO (`da60ae3`)

## [v1.2.17] - 2026-05-12

### 修复

- 防止订阅重复创建及保存防抖 (`9157349`)

## [v1.2.18] - 2026-05-12

### 新增

- 添加 MediaVault 集成，整理成功后触发目录监控 (`8769f6e`)

## [v1.2.19] - 2026-05-12

### 修复

- 移除 NFO 中的 language 字段 (`72dd535`)

## [v1.2.20] - 2026-05-13

### 新增

- 多源 TMDB 身份解析 & skip_metadata_upload 跳过全部元数据 (`7f0e62d`)

### 修复

- 多行规则编辑器防抖优化，避免光标跳转 (`67286ae`)

## [v1.2.21] - 2026-05-16

### 新增

- 添加修正识别功能，支持重新绑定 TMDB 条目并重命名目录 (`deb8146`)

## [v1.2.22] - 2026-05-16

### 修复

- 优化 Dockerfile 镜像标签和启动命令 (`fef216a`)
- fill_seasons_episodes 保留已有 in_library 标记并重算 in_library_count (`7d37580`)
- Bangumi Calendar 代理捕获上游异常，返回 502 而非 500 (`379f74e`)

## [v1.3] - 2026-07-01

### 新增

- 接入 AniBT 资源检索并清理 Quark 残留 (`728da1b`)

### 文档

- 115 docs (`68b3d2e`)

## [v1.3.1] - 2026-07-02

### 修复

- 刷新媒体信息时绕过过期季集缓存 (`24e7ded`)

## [v2.0.0] - 2026-10-14

### 新增

- **[破坏性]** 移除 Python 后端——Go 重构完成 (`c1559d6`)
- （frontend）移除 Google Drive 配置界面 (`8bd98e5`)
- （go）WebUI Aria2 完整下载器服务 + MediaVault 测试 + TMDB 季详情 (`db7a836`)
- （go）WebUI 后台监听——115 云下载自动整理 + RSS 订阅轮询 (`12f1ccc`)
- （go）WebUI 文件管理——列表/详情/建目录/改名/移动/批量操作 (`6746f32`)
- （go）WebUI 媒体操作服务——刷新/纠错/TMDB/爬虫/番剧日历 (`48bc80a`)
- （go）WebUI RSS 订阅服务 + Aria2 最小 RPC (`4938523`)
- （go）WebUI 115 服务——扫码授权/二维码/云下载全链路 (`09e48d5`)
- （go）WebUI 配置服务——读写/校验/解析测试 (`aca68c3`)
- （go）WebUI 扫描服务与流程调度——媒体库刷新/扫描/webhook/日志/入库历史 (`890d2a0`)
- （go）WebUI 剩余存储层——入库历史/应用日志/115自动整理/RSS订阅 (`75e858e`)
- （go）cmd/meta2cloud 入口(serve/pipeline)+进度文档更新 (`489b0b8`)
- （go）WebUI 服务核心——运行时单例、JWT 认证、HTTP 骨架 (`70aec46`)
- （go）WebUI 存储层——媒体实体库 + TMDB 实体缓存 (`8e973cb`)
- （go）移植 RSS 订阅爬虫层(mikan + anibt) (`f1b512a`)
- （go）移植核心整理流程——organizer、pipeline、字幕匹配、图片上传 (`8c7817c`)
- （go）移植 TMDB 客户端与 NFO 生成器 (`bdfb392`)
- （go）移植 mediaparser 文件名解析层 (`9080efa`)
- （go）移植存储层——cloudfile 抽象、115 客户端、Provider 与配置 (`428b97c`)

### 修复

- 新番集数增长后 total_episodes 使用旧值(12/7 荒谬显示) (`80c97c6`)
- refresh-item 响应数组字段 null 崩溃(前端 null.length) (`31bb8bb`)
- 非官方译名识别 + 同剧多目录集数聚合 (`a8687f8`)
- （go）订阅卡片 TMDB 标题缓存空窗补全 (`376b4db`)
- （go）/api/logs 返回结构与 Python 对齐 + 修复空库 null 响应 (`36cc375`)
- （go）补全 /api/logs/live 实时日志 WebSocket 端点 (`e12f4a9`)

### 文档

- 全部文档更新为 Go 后端 (`f3342c2`)
- 更新 Go 重构进度——存储/解析/TMDB/NFO 层完成 (`9848a3e`)

### 构建

- Docker 切换为 Go 后端多阶段构建 (`5b6cb86`)

### 杂项

- 删除 docs/ + 修复 CI 构建链路 + 版本号注入 (`03b4e46`)
- gitignore 添加 Go 编译产物 (`97ecbf7`)

### 其他

- 清零 Python 残留 + 修复 TMDB query 覆盖 bug + 冗余合并 (`d96309d`)

## [v2.0.1] - 2026-10-14

### 修复

- 同 tmdb_id 多目录在媒体库合并为一张卡片 (`df2e7ef`)
- （readme）AniBT 并非 2.0 新功能(v1.3 已存在) + 提供 Google Drive 末版 compose (`9c8f95b`)

## [v2.0.2] - 2026-10-15

### 修复

- RSS 订阅 115 推送下载成 .torrent 种子文件而非媒体文件 (`222f26a`)
- 增量刷新同 tmdb_id 兄弟目录联动重扫 (`b203822`)

## [v2.1.0] - 2026-10-15

### 新增

- 前端 UI 全面重构 —— 中性暗底 + 亮绿主色设计系统 (`f0601b7`)

### 修复

- 媒体库卡片收录进度条区分完成态（100% 主题绿，进行中青绿） (`d100863`)
- 下载与离线任务进度条补齐成功态配色（100% 显示主题绿） (`2e53d38`)
- 进度条强调色改为青绿，与主色按钮拉开视觉层次 (`18d4437`)
- 进度条强调色由黄绿改为主题绿（--lime 令牌与辉光同色相） (`d5eaddb`)
- 媒体库排序筛选下拉被 w-full 撑满整行，与搜索栏不协调 (`cb13d57`)
- 资源检索同名聚合只展示先注册站点的字幕组结果 (`32d05b1`)
- anibt 搜索结果 bgmId 为数字时被静默跳过 (`3157b59`)
- mikan 搜索关键词未 URL 编码导致服务端返回 HTTP 400 (`a5548b6`)

### 优化重构

- 合并各页面重复的 formatBytes/formatTime 到 lib 共享实现 (`ea8f297`)

### 杂项

- 清理 api 客户端与类型中无引用的死代码 (`c124a33`)
- 移除未使用的 UI 原语组件与 @base-ui/react 依赖 (`4c75986`)


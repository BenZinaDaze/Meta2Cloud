# 更新日志

本文件记录 Panoply 每个版本的显著变更，格式参考
[Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。
发版操作手册见 [doc/release.md](doc/release.md)。

> 注：早期 v1.0.2 ~ v1.1.7 的原始提交历史曾被重写（快照式平行历史），
> 其条目由版本树比对还原；实际功能均已包含于主线 v1.1.8 及之后版本。

## [v2.8.0] - 2026-10-30

### 🏷️ 项目更名 Meta2Cloud → Panoply

- 全量更名：代码、模块路径（`panoply`）、二进制（`panoply serve`）、Docker 镜像（`benz1/panoply`）、文档与 Web UI 标题
- 存量用户迁移：Docker 用户将镜像与容器名改为 `benz1/panoply`、数据卷 `panoply-config`（沿用原目录内容即可）；二进制用户改用 `./panoply serve`
- 历史镜像 `benz1/meta2cloud` 各版本保留可拉取；`docker-compose.gdrive.yml` 的 v1.3.1 仍指向旧名镜像（Docker Hub 上仅存的历史发布）
- GitHub 仓库同步更名为 `BenZinaDaze/Panoply`，旧地址自动重定向

### 🧹 杂项

- 移除遗留的 .python-version（pyenv 残留） (`4297be2`)

## [v2.7.0] - 2026-10-29

### 🐛 修复

- RSS 轮询对 store 短暂不可用加 nil 防护（测试反复 InitRuntime 交错窗口防 panic） (`dc9dc92`)
- 修复 StrmSyncPage 初始加载丢失（usePolling 重构误删），补齐四页冒烟测试 (`c0bd519`)
- Plex SSE 常驻循环加 panic 自愈——记录后节流重启，不再带崩进程 (`0adaa48`)
- 消除残余包级运行态——watcherState 全部并入 Runtime，修复 Close 后会话轮询停不住 (`efdbb20`)
- 修复三处系统性吞错——组织失败/扫描失败/库标记失败不再静默 (`dfdaf5b`)
- 配置解析失败快速暴露——Load 严格报错 + 启动校验 cron，运行中沿用最近可用配置 (`93699b6`)
- SIGTERM/SIGINT 优雅关闭——HTTP 排空在途请求，停止后台任务并关闭存储句柄 (`004c59f`)

### ♻️ 优化重构

- trakt 域下沉——33 个方法自 Runtime 下沉到 traktService（批次 4） (`7c49c28`)
- library 域下沉——14 个扫描/解析方法自 Runtime 下沉到 libraryService（批次 3） (`860f045`)
- pipeline 域下沉——pipelineService 吸收调度状态与 4 方法（Runtime 拆分批次 2） (`2555422`)
- library_media 写入边界类型化——LibraryMediaRow 替代 map 载荷 (`d4ac884`)
- STRM 域 21 个方法自 Runtime 下沉到 strmService（Runtime 拆分收尾批次 1） (`ff35fed`)
- toast 统一为直连 sonner，删除 onToast 全链路下钻 (`5f8ccd4`)
- DashboardPage 892→312 行——卡片族外迁到 components/dashboard/ (`278cb82`)
- WatchedSyncPage 714→260 行——明细/忽略/运行表抽到 components/watched/，StatCard 收敛 5 处副本 (`1854439`)
- ConfigPage 863→360 行——9 个配置 tab 外迁为自包含 Section 组件 (`e0cb5f6`)
- ingest 行扫描类型化——IngestRecordRow 替代 map[string]any (`848c755`)
- 剩余 map 负载 struct 化——u115 OAuth/自动整理状态、ingest 统计 (`113b429`)
- Telegram 传输层独立成 internal/telegram 包，webui 不再为发消息依赖 pipeline (`96351c2`)
- 新增 usePolling 声明式轮询 hook，收敛六个页面各自手写的 setInterval (`dc00a60`)
- 新增共享字符串工具，收敛 8 处复制的 firstNonEmpty/orDefault 变体 (`3ab5244`)
- 新增共享 HTTP 基础设施——节流器与带重试的 GET 抓取，收敛五套手写实现 (`a7d2cf3`)
- 仪表盘契约——GetStats/PipelineStatus struct 化，DashboardPage 内联类型收编共享类型 (`2deffae`)
- 115 离线域契约 struct 化——任务/分页/配额/容量负载即类型源 (`276f65f`)
- STRM 同步域契约 struct 化——status/runs/files/播放历史负载即类型源 (`d790c26`)
- 观看同步域契约 struct 化——后端负载即类型源，前端收编共享类型并泛型化 (`ae2af96`)
- schema 版本化迁移框架（schema_versions 表）替换三处手写补列 (`32af744`)
- SaveIncremental 单事务化——原子生效 + 批量提速，错误上抛 (`12222a7`)
- STRM 直链缓存并入 strmService，观看同步运行态收敛为 traktState（分域批次 3/3） (`8156eeb`)
- health/classify/pipeline 状态收敛为域结构体，修复 runID 递增丢失（分域批次 2/3） (`b7633b2`)
- 后台调度状态收敛为 schedulerState（Runtime 分域批次 1/3） (`8650b3e`)
- 健康状态机/分类树缓存/cron 调度器/订阅预取并入 Runtime（DI-2 批次 C） (`9fc4277`)
- STRM 直链缓存与整理管线调度状态并入 Runtime（DI-2 批次 B） (`6179dd0`)
- 观看同步运行态并入 Runtime（DI-2 批次 A） (`3cf24c1`)
- 移除全局单例，运行时实例化 + 依赖注入完成（DI-3 S终） (`c1755c8`)
- JWT 签名校验与 STRM 钩子服务归 Runtime（DI-3 S2g-5） (`f886820`)
- JWT 密钥/健康上报/目录解析等基础设施归 Runtime（DI-3 S2g-4） (`e868f0d`)
- 消除全部业务代码 RT() 调用（DI-3 S2g-3） (`a0b7f71`)
- watcher 后台调度归 Runtime（DI-3 S2g-2） (`a1bb68f`)
- pipelinesvc 处理器方法化，appLog 归 Runtime（DI-3 S2g-1） (`e376204`)
- trakt 域处理器方法化（DI-3 S2f） (`2817cc5`)
- STRM 域处理器方法化（DI-3 S2e） (`9feecad`)
- subscriptionsvc/u115svc/miscroutes 处理器方法化（DI-3 S2d） (`7104afb`)
- 媒体域处理器方法化（DI-3 S2c） (`3bfb7c1`)
- filemanager 处理器方法化，mergedConfigDict 归属 Runtime（DI-3 S2b） (`3fa95cc`)
- configsvc 处理器方法化（DI-3 S2a） (`7e03140`)
- Server 持有 *Runtime，server.go 全部处理器方法化（DI-3 S1） (`a731cc6`)
- 存储单例启动时 fail-fast 打开，删除全部 handler 判空样板 (`176db75`)
- 清理 deadcode 核实的无用符号（约 25 个 / 300+ 行） (`99b54a7`)
- pan115 受管 Provider 装配下沉 internal/storage (`4f585d0`)
- 1739 行 traktsvc.go 按职责拆分，4 组会话读写收敛为泛型助手 (`341bdba`)
- 115 与观看同步配置区块拆为自包含 section 组件 (`d04ecf1`)
- 配置页表单基础组件抽离到 components/config/fields.tsx (`51a54e4`)
- ConfigPage 8 组 busy/message 状态对收敛为 useAsyncAction hook (`8cdeb74`)
- 快照保存事务化，删除 GetSnapshot 双实现，标题匹配拆独立包 (`137d251`)

### 🧹 杂项

- 引入 MSW mock 层——页面冒烟测试与 api 拦截器测试的基础设施 (`6cd8362`)
- 默认值常量化——DefaultWebUIUsername/DefaultPlexTimeoutSecs 收敛三处硬编码 (`90cc03d`)
- 引入 Vitest + Testing Library 单元测试框架（47 用例） (`5a958a6`)
- 删除 DI 迁移后的遗留符号（getLibraryStoreLocked/SetTmdbClientForTest） (`f95932c`)


## [v2.6.7] - 2026-10-28

### ✨ 新增

- 登录接口增加失败限速（按 IP 锁定防暴力破解） (`99e3d2b`)

### 🐛 修复

- /trigger webhook 密钥缺失时自动生成，未配置时仅允许本机触发 (`7ee87ce`)
- 凭据不再随错误日志外泄，token 文件权限写入即收紧 0600 (`90f2baf`)
- 种子链接服务端下载增加外呼地址校验（防 SSRF） (`297b16f`)
- 发版脚本前端构建显式注入版本号，本机 dist 版本不再滞后一版 (`9324d5a`)
- 入库历史关键词转义 LIKE 通配符，爬虫响应体增加大小上限 (`9f9107f`)

### ♻️ 优化重构

- 配置接口仅对 JWT 签名密钥脱敏（界面显示不变），tmdb.api_key 拆入 secrets.yaml (`6737c51`)

## [v2.6.6] - 2026-10-28

### ✨ 新增

- WebUI 密码改 bcrypt 哈希存储，敏感配置拆入 secrets.yaml（0600） (`4982659`)


## [v2.6.5] - 2026-10-27

### 🐛 修复

- 停止进度封顶只对暂停态会话生效，playing 态自然推进不封顶 (`9ba6544`)
- stopped 收尾进度按观察轨迹封顶，不采信休眠会话的遗留 offset (`49e7843`)

### ♻️ 优化重构

- 剧级指纹缓存跳过逐剧进度请求，同步异常补 Telegram 提醒 (`a01da60`)


## [v2.6.4] - 2026-10-27

### 🐛 修复

- 会话清理丢弃重启前遗留的僵尸行，不再幽灵推送 Trakt (`a446fe8`)
- 元数据请求补 includeGuids，Plex 身份解析失败不再静默 (`2220795`)

### 🧹 杂项

- 多平台构建改原生交叉编译，前端只构建一次 (`295f707`)


## [v2.6.3] - 2026-10-26

### 🐛 修复

- 修正识别同步改写入库记录到新 TMDB 身份 (`bfcbfd8`)
- 修正识别按新标题同步重命名目录内视频与字幕 (`6133ead`)


## [v2.6.2] - 2026-10-25

### 🐛 修复

- 会话 scrobble 播放队列切集不再把上一集进度记到下一集 (`8a233fc`)
- 会话 scrobble 409 去重不落同步记录，SSE 路径补会话标题 (`de521e8`)

### 🧹 杂项

- 增加 Plex SSE 实时通知抓包脚本（SSE 排查先抓包再改码） (`cc7f202`)


## [v2.6.1] - 2026-10-25

### 🐛 修复

- 会话 scrobble 暂停按 Trakt 有效进度区间（1%~80%）分派，终止无效重试 (`51663eb`)


## [v2.6.0] - 2026-10-24

### ✨ 新增

- 线程启停事件按功能区分（watcher_started 拆分为专属事件名） (`8e77317`)
- 统一前后端日志出口，appLog 镜像到进程 stdout (`51c486c`)
- 日志页 WebSocket 实时推送扩展到全部类别 (`d50e1c3`)
- start/pause 上报成功记 INFO 日志 (`b004d37`)
- 身份多 ID 归一化与 scrobble 三段式实时推送 (`7edd6bb`)
- 同步记录相邻同媒体同结果折叠展示 (`29a17f2`)

### 🐛 修复

- 修复 SSE 播放事件解析与集级身份解析，推送失败加退避 (`1f8fe90`)
- 爬虫工厂改为包初始化装配，消除订阅轮询误报 Spider not found (`185a8d1`)

### ♻️ 优化重构

- cron 调度随配置开关启停，开关未开不启动轮询 (`189723f`)

### 🎨 样式

- 线程启动日志去掉调度方式后缀，保持口径一致 (`e15d2d2`)
- 日志文案去术语化，改用使用者视角的平实描述 (`d31c356`)


## [v2.5.1] - 2026-10-23

### ✨ 新增

- 服务配置失效/恢复的 Telegram 通知（115/Plex/Trakt/订阅源） (`37af4f4`)
- 轮询定时统一改为 cron 表达式（115/RSS/STRM/Trakt 同步） (`e3a828a`)

### 🐛 修复

- 通知 Plex 刷新补应用日志并覆盖联动删除场景 (`fd7ba6e`)


## [v2.5.0] - 2026-10-23

### ✨ 新增

- 联动删除媒体条目（网盘目录 + STRM + 库条目 + 订阅） (`a08da41`)
- 同步变更后通知 Plex 刷新对应分类媒体库 (`8626b0b`)
- 识别词引擎对齐 MoviePilot v3 words.py (`39dc1a7`)

### 🐛 修复

- 解析测试返回 apply_words 字段 (`4eab087`)
- anitomy 补齐 anitopy 括号语义，副标题季集识别对齐 MoviePilot (`76d19a4`)
- mikan 域名切换至 mikanani.me，备用 mikanime.tv (`c7f86b9`)
- 升级 Go 至 1.27.3 修复 CVE-2026-111943 (`0f3c019`)

### 📝 文档

- 精简 CLAUDE.md，删除可从代码推导的内容 (`599310c`)


## [v2.4.3] - 2026-10-22

### 🐛 修复

- 容器默认以 UID/GID 1000 运行，使媒体服务器可删除生成的 STRM (`81ea2a8`)
- 定时同步前探测 Plex 可达性，避免同 compose 重启竞态 (`4d64cdb`)

### 📝 文档

- 按一站式影音平台定位重写文档并更新截图 (`c2da312`)
- 润色 v2.4.2 更新日志 (`80aedb2`)

### 🧹 杂项

- gitignore 的 panoply 模式锚定根目录，避免误伤 cmd/panoply (`18df876`)


## [v2.4.2] - 2026-10-21

### ✨ 新增

- 观看同步忽略列表与 not_found 精确归因：Trakt 未收录的媒体（如新番缺 TMDB 映射）可一键忽略止损，失败明细只连坐真正未匹配的条目并标明具体身份 (`bc5121c`)

### 🐛 修复

- 实时推送明细的 success 未随推送结果写入，成功推送点开后显示为无原因的「失败」（启动时自动修正历史数据）(`acb075e`)

### ♻️ 优化重构

- 会话轮询改为纯兜底：SSE 在线时不并行轮询，SSE 关闭/断开时自动接管 (`09270ff`)
- 移除 Trakt 活动监视器：last_activities 延迟导致自身推送被误判为外部变化而空跑全量；Trakt 侧手动标记的同步由定时全量兜底 (`0cb58af`)
- 观看同步设置区改为独立面板分组（Trakt 授权 / Plex 连接 / 同步设置）(`cb5cf01`)

### 🎨 样式

- 全站用户文案统一：省略号/术语（电视剧→剧集、入库历史→入库记录）/口语化表述，清理 4 处 Google Drive 残留文案 (`1d818a2`)

### 📝 文档

- 润色 v2.4.1 更新日志 (`285e7cb`)


## [v2.4.1] - 2026-10-20

### ✨ 新增

- 同步记录统一展示实时推送：播放推送（成功/失败）独立成条，带媒体、进度、来源与失败原因；零变化的全量同步不再显示 (`1348450`)

### 🐛 修复

- 移除 Plex 观看哨兵：其依赖的「最近观看」排序在真实环境不稳定，会误触发每分钟一轮的同步风暴 (`6930158`)

### 📝 文档

- 注释中的「影子」术语改为「同步状态」，日志与界面文案去口语化 (`d8ce04d`)


## [v2.4.0] - 2026-10-19

### ✨ 新增

- 新增仪表盘页 (`162487c`)
- Plex 实时推送（SSE）与观看哨兵 (`53777d9`)
- Plex↔Trakt 观看状态双向同步 (`6a5f1be`)

### 🐛 修复

- 补齐 STRM 同步记录的触发来源中文标签 (`8453133`)
- 修复 STRM 同步崩溃遗留记录收尾失效 (`cbe1c52`)

### ♻️ 优化重构

- 移除 Dry Run 与 NFO/封面上传，元数据交由播放器管理 (`ecbc9fe`)

### 🎨 样式

- 整理名称页大屏适配设计 token (`57780a5`)
- gofmt 全仓 (`5c8dab1`)


## [v2.3.4] - 2026-10-19

### 🐛 修复

- 衍生条目误识别的防护与自愈 (`7fd8845`)


## [v2.3.3] - 2026-10-18

### ✨ 新增

- 修正识别二合一归类与全库重新归类强化 (`bb5b3e2`)

### 🐛 修复

- GetFile 与配置根目录解析互调成环导致栈溢出 (`58ce7c7`)
- 文件列表项解析补提取 parent_id (`c943e5c`)


## [v2.3.2] - 2026-10-18

### ✨ 新增

- 播放记录相邻同项折叠分组，直链列更名延迟 (`4bd3e41`)
- 目录配置统一为网盘根起算的相对路径 (`e3e70cf`)
- 整理名称支持删除记录，tmdb 标签直跳 TMDB 官网 (`916acab`)
- 目录配置改存目录名，选择器直选网盘目录 (`a3d2f0b`)
- 一键整理增加二次确认框 + 目录配置防呆 (`9d6f9cb`)
- 首次启动自动生成默认配置三件套 + 分类页嵌套层级与折叠 (`3c6df93`)
- **[破坏性]** 媒体分类布局常开，一级分类取代剧集根/电影根 (`a6e5fc5`)
- 保存媒体分类时预创建整棵分类目录树（幂等） (`f21e0aa`)
- 媒体两级分类布局（剧集/电影/其他 + 二级分类），独立配置文件与配置页 (`bcdf1b0`)

### 🐛 修复

- 目录选择器面包屑前端自维护，修复双根目录与子目录显示 ID (`a84f7e1`)
- 同步末尾全局孤儿对账，映射换位置后自动清理旧位置残留 (`0e804a2`)
- 重复目录合并前增加同剧名校验，防止误识别媒体被错误合并 (`7bc4a36`)
- 后台监听线程随 Server.Close 停止，根治测试 TempDir 清理竞态 (`a175f8d`)

### ♻️ 优化重构

- 媒体库与整理名称查询分页化/批量化，消除大库卡顿 (`63aad44`)
- 整理提速——STRM 钩子传链免反查、移动去回查、空目录两阶段清理 (`5026d2e`)
- 移除 pipeline CLI 子命令，整理统一走 WebAPI 触发 (`16d90bc`)
- 分类配置独立为侧边栏子菜单页面，移除启用开关 (`b8c5e37`)

### 🎨 样式

- 用户可见文案去「钉扎」术语，统一为「整理名称」 (`2d004e0`)


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
- 接入 pan115 存储抽象并统一项目命名为 Panoply (`eb9eb9f`)
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
- （go）cmd/panoply 入口(serve/pipeline)+进度文档更新 (`489b0b8`)
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


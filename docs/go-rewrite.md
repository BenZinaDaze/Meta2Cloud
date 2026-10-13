# Go 重构进度跟踪

**状态**: ✅ 重构完成 | **分支**: `rewrite/go-backend` | Google Drive 支持已随重构移除(Python 代码已全部删除)
**模块名**: `meta2cloud` | **Go**: 1.27 | **依赖**: yaml.v3(已用), golang-jwt/jwt/v5、modernc.org/sqlite(待用)

## 模块状态

| Python 源 | Go 目标 | 状态 | 行数(Python) | 备注 |
|---|---|---|---|---|
| `storage/base.py` | `internal/cloudfile` | ✅ 完成+测试 | 262 | CloudFile + Provider 接口;新增 ListAllFiles 接口方法 |
| `u115pan/client.py` | `internal/u115/client.go` | ✅ 完成+测试 | 1080 | 限流/冷却/重试/OAuth/文件/上传/分享转存;OSS 上传改用内存哈希免落盘 |
| `u115pan/models.py` | `internal/u115/models.go` | ✅ 完成+测试 | 258 | PKCE 用 RFC 7636 测试向量验证 |
| `u115pan/auth.py` | `internal/u115/models.go` | ✅ 完成+测试 | 47 | |
| `u115pan/errors.py` | `internal/u115/errors.go` | ✅ 完成 | 36 | Go 用 Kind 枚举替代异常继承 |
| `u115pan/offline.py` | `internal/u115/offline.go` | ✅ 完成 | 341 | 云下载 |
| `u115pan/runtime.py` | 待移植 | ⬜ | 141 | 共享 client 单例管理,webui 层移植时处理 |
| `storage/pan115.py` | `internal/storage/pan115.go` | ✅ 完成+测试 | 634 | OSS 分片上传用手写 V1 签名替代 oss2 |
| `storage/__init__.py` | `internal/storage/registry.go` | ✅ 完成+测试 | 62 | 仅注册 pan115;google_drive 返回未知错误 |
| `storage/google_drive.py` | **不移植** | 🗑️ | 194 | Google Drive 移除 |
| `drive/`(整个包) | **不移植** | 🗑️ | 604 | Google Drive 移除 |
| `mediaparser/config.py` | `internal/config` | ✅ 完成 | 407 | 删除 DriveConfig;默认 primary=pan115 |
| `mediaparser/tmdb.py` | `internal/tmdb` | ✅ 完成+测试 | 431 | 搜索/详情/名称匹配/429 重试;Cache 为接口,webui 层实现 |
| `mediaparser/tmdb_image.py` | `internal/tmdb`(ImageURL) + `internal/config` | ✅ | 35 | |
| (新增) | `internal/jsonx` | ✅ | — | 动态 JSON 工具,多包共用 |
| `mediaparser/metainfo.py` | `internal/mediaparser/metainfo.go` | ✅ 完成+测试 | 194 | 入口 MetaInfo/MetaInfoPath + 内嵌 ID 标签 |
| `mediaparser/meta_video.py` | `internal/mediaparser/metavideo.go` | ✅ 完成+测试 | 605 | lookaround 用 regexp2;体积正则保留 lookahead 语义 |
| `mediaparser/meta_anime.py` | `internal/mediaparser/metaanime.go` | ✅ 完成+测试 | 228 | anitopy 以 anitomy.go 子集实现 |
| `mediaparser/meta_base.py` | `internal/mediaparser/metabase.go` | ✅ 完成+测试 | 448 | 副标题中文季集解析(regexp2 lookaround) |
| `mediaparser/string_utils.py` | `internal/mediaparser/stringutils.go` | ✅ 完成+测试 | 79 | cn2an/zhconv/拼音为子集实现;繁简对照表 173 字符对 |
| `mediaparser/words.py` / `tokens.py` / `types.py` | `internal/mediaparser` | ✅ 完成+测试 | 200 | |
| `mediaparser/release_group.py` | `internal/mediaparser/releasegroup.go` | ✅ 完成+测试 | 92 | lookbehind/lookahead 用 regexp2 |
| `mediaparser/streaming_platform.py` | `internal/mediaparser/streamingplatform.go` | ✅ | 105 | 全表移植 |
| `mediaparser/infopath.py` | `internal/mediaparser/infopath.go` | ✅ | 37 | |
| `nfo/generator.py` | `internal/nfo` | ✅ 完成+测试 | 482 | 手写 XML 构建器(CDATA/属性);转义行为与 minidom 对齐 |
| `nfo/image_uploader.py` | `internal/nfo` | ⬜ 待开始 | ? | |
| `core/pipeline.py` | `internal/pipeline` | ✅ 完成+测试 | 944 | 端到端: TV/电影/字幕/dry-run/替换模式/TG 通知 |
| `core/organizer.py` | `internal/organizer` | ✅ 完成+测试 | 190 | 目录整理 |
| `core/subtitle_matcher.py` | `internal/pipeline/subtitle.go` | ✅ 完成+测试 | 279 | |
| `scraper/`(mikan/anibt) | `internal/scraper` | ✅ 完成+测试 | 456 | goquery HTML 解析 |
| `webui/app.py`+`core/`+auth+基础路由 | `internal/webui` | ✅ 完成+测试 | ~500 | 运行时/JWT/ServeMux/SPA 兜底;登录+library+stats |
| `webui/library_store.py`+`tmdb_cache.py` | `internal/webui/store` | ✅ 完成+测试 | 933 | 表结构与 Python 完全一致,复用既有 library.db |
| `core/__main__.py`+服务入口 | `cmd/meta2cloud` | ✅ 可运行 | 98+ | serve/pipeline 子命令,二进制冒烟通过 |
| 前端 `frontend/` | 不变 | — | — | 保持 API 兼容 |
| Docker / compose | 待更新 | ⬜ | — | 多阶段构建 Go 二进制 |

## 关键设计决策

1. **存储抽象保留**: `internal/cloudfile.Provider` 接口是核心扩展点
2. **Google Drive 移除**: registry 只注册 pan115;config 无 drive 段;旧配置 `primary: google_drive` 会得到明确报错
3. **OSS 上传**: 手写 V1 签名(3 个操作),避免引入 oss2 等价大依赖;上传哈希直接内存计算免临时文件
4. **测试策略**: httptest 模拟 115 服务端,穿透完整请求管线(比 Python 版 mock 更真实);Python 测试用例逐一移植
5. **依赖最小化**: 目前仅 yaml.v3;JWT 与 SQLite 在 webui 层移植时引入
6. **Python 代码保留**: 移植完成前不删,作为行为参照;全部移植完成后统一清理

## API 兼容性要点(webui 移植时)

- 后端端口 38765,前端 `/api` 代理到此
- `/api/drive/*` 两个端点**不实现**(Google Drive 移除)
- 前端无任何 `/api/drive` 调用点(已验证),ConfigPage 的 Google Drive 卡片需在前端移除
- JWT Bearer 认证语义保持

## 移植顺序(后续)

1. mediaparser(最难: 正则+中文处理) → 2. tmdb → 3. nfo → 4. organizer → 5. pipeline → 6. scraper → 7. webui(最大) → 8. cmd 入口 → 9. 前端清理 Google Drive UI → 10. Docker → 11. 删除 Python 代码


## 完成收尾(2026-10)

- 前端: ConfigPage 删除 Drive 标签页/OAuth/字段编辑,存储默认值 pan115
- Dockerfile: Go 多阶段构建(golang:1.27 静态编译 + debian-slim 运行)
- 删除全部 Python 代码(22,557 行): core/webui/storage/u115pan/mediaparser/nfo/scraper/drive/test + pyproject/uv.lock
- scripts: dev.sh(并联 Go+Vite)/test.sh(go test) 重写;upload.example.sh 保留(/trigger 语义不变)
- CLAUDE.md/AGENTS.md/README.md/config.example.yaml 全部更新为 Go 版
- 冒烟验证: 真实前端 SPA + 登录 + stats/config API 正常

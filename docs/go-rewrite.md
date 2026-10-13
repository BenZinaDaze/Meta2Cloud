# Go 重构进度跟踪

**分支**: `rewrite/go-backend` | **策略**: 自底向上移植,Google Drive 支持不再实现
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
| `mediaparser/metainfo.py` | `internal/mediaparser` | ⬜ 待开始 | 194 | 入口 MetaInfo/MetaInfoPath |
| `mediaparser/meta_video.py` | `internal/mediaparser` | ⬜ 待开始 | 605 | 8 处 lookaround 正则 → regexp2 |
| `mediaparser/meta_anime.py` | `internal/mediaparser` | ⬜ 待开始 | 228 | anitopy 需自行实现子集 |
| `mediaparser/meta_base.py` | `internal/mediaparser` | ⬜ 待开始 | ? | |
| `mediaparser/string_utils.py` | `internal/mediaparser` | ⬜ 待开始 | ? | cn2an/zhconv 需自行实现 |
| `mediaparser/words.py` / `tokens.py` / `types.py` | `internal/mediaparser` | ⬜ 待开始 | ? | |
| `mediaparser/release_group.py` | `internal/mediaparser` | ⬜ 待开始 | ? | 1 处 lookaround |
| `mediaparser/streaming_platform.py` | `internal/mediaparser` | ⬜ 待开始 | ? | 纯映射表,简单 |
| `mediaparser/tmdb.py` | `internal/tmdb` | ⬜ 待开始 | ? | API 客户端 |
| `mediaparser/tmdb_image.py` | `internal/config`(部分) | ✅ | 35 | normalize 已并入 config;URL 构建待移入 tmdb 包 |
| `mediaparser/infopath.py` | `internal/mediaparser` | ⬜ 待开始 | ? | |
| `nfo/generator.py` | `internal/nfo` | ⬜ 待开始 | ? | XML 生成 |
| `nfo/image_uploader.py` | `internal/nfo` | ⬜ 待开始 | ? | |
| `core/pipeline.py` | `internal/pipeline` | ⬜ 待开始 | 944 | 主流程编排 |
| `core/organizer.py` | `internal/organizer` | ⬜ 待开始 | ~250 | 目录整理 |
| `core/subtitle_matcher.py` | `internal/pipeline` | ⬜ 待开始 | ? | |
| `scraper/`(mikan 等) | `internal/scraper` | ⬜ 待开始 | ? | RSS 抓取 |
| `webui/app.py` + `routes/` + `services/` | `internal/webui` | ⬜ 待开始 | ~8000 | FastAPI → net/http;JWT+SQLite |
| `webui/library_store.py` / `tmdb_cache.py` 等 store | `internal/webui/store` | ⬜ 待开始 | ? | modernc.org/sqlite |
| `core/__main__.py` | `cmd/meta2cloud`(CLI) | ⬜ 待开始 | ~120 | `--dry-run` 等参数 |
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

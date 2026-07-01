# 云存储接入指南

本文说明如何为 Meta2Cloud 接入新的云存储后端。项目通过 `storage.base.StorageProvider` 抽象网盘能力，整理流程、媒体库扫描和 WebUI 文件管理都只依赖这个统一接口。

## 接入位置

新增 Provider 建议放在：

```text
storage/<provider>.py
```

实现后在 `storage/__init__.py` 导入并注册：

```python
from storage.example import ExampleProvider

register_provider("example", ExampleProvider)
```

`register_provider()` 的名称必须和配置里的 `storage.primary` 一致。

## 必须实现的接口

Provider 需要继承 `StorageProvider`：

```python
from storage.base import StorageProvider, CloudFile, FileType

class ExampleProvider(StorageProvider):
    @property
    def provider_name(self) -> str:
        return "example"

    @classmethod
    def from_config(cls, cfg) -> "ExampleProvider":
        ...
```

必须实现的方法：

- `list_files(folder_id="root", page_size=100)`：列举直接子项。
- `get_file(file_id)`：读取文件 / 文件夹元数据。
- `read_text(file)`：读取文本文件内容，主要用于 NFO 等。
- `find_file(name, folder_id=None)`：在目录下按精确名称查找。
- `rename_file(file_id, new_name)`：重命名。
- `move_file(file_id, new_folder_id, new_name=None)`：移动，可同时重命名。
- `upload_text(content, name, parent_id=None, mime_type="text/xml", overwrite=False)`：上传文本。
- `upload_bytes(content, name, parent_id=None, mime_type="image/jpeg", overwrite=True)`：上传二进制。
- `trash_file(file_id)`：移入回收站。
- `delete_file(file_id)`：彻底删除。
- `create_folder(name, parent_id=None)`：创建目录。

可选覆盖：

- `list_media_files(folder_id)`：默认由 `list_files()` 过滤视频；平台支持服务端过滤时建议覆盖。
- `list_all_recursive(folder_id, max_depth)`：默认递归调用 `list_files()`；大目录平台可优化。
- `get_or_create_folder(name, parent_id)`：默认先 `find_file()` 再 `create_folder()`。

## 统一数据模型

所有 Provider 返回 `CloudFile`：

```python
CloudFile(
    id="...",
    name="Video.mkv",
    file_type=FileType.FILE,
    size=123,
    modified_time="2026-07-01T12:00:00Z",
    parent_id="...",
    parents=["..."],
    mime_type="video/x-matroska",
    trashed=False,
    extra={"pick_code": "..."},
)
```

注意：

- `id` 必须是字符串，即使平台原始 ID 是数字。
- 文件夹使用 `FileType.FOLDER`，文件使用 `FileType.FILE`。
- `parent_id` 尽量填写；WebUI 面包屑和移动操作会用到。
- 平台特有字段放入 `extra`，不要扩展公共模型字段。
- `mime_type` 或扩展名会影响 `is_video` / `is_subtitle` 判断。

## 配置接入

需要在 `mediaparser/config.py` 中增加对应配置 dataclass，并接入 `Config.from_dict()`：

```python
@dataclass
class ExampleConfig:
    token_json: str = "config/example-token.json"
    scan_folder_id: str = ""
    root_folder_id: str = ""
    movie_root_id: str = ""
    tv_root_id: str = ""

    @classmethod
    def from_dict(cls, d: dict) -> "ExampleConfig":
        return cls(...)
```

同时：

- 在 `Config` 上增加字段，例如 `example: ExampleConfig`。
- 在 `config/config.example.yaml` 增加示例配置段。
- 更新 `StorageConfig.primary` 注释中的可选值。
- 如果该存储有扫描目录、媒体库根目录、电影目录、剧集目录，需要更新：
  - `Config.active_scan_folder_id()`
  - `Config.active_root_folder_id()`
  - `Config.active_movie_root_id()`
  - `Config.active_tv_root_id()`

## Provider 创建与运行时注册

常规 Provider 只需要实现 `from_config()`，运行时会通过：

```python
storage.get_provider(cfg.storage.primary, cfg)
```

创建实例。

特殊情况：如果 Provider 需要复用 WebUI 的 OAuth runtime 或动态客户端，可以像 115 一样在 `webui/core/runtime.py` 中特殊处理：

```python
if cfg.storage.primary == "pan115":
    _storage_provider = Pan115Provider.from_client_getter(_u115_client)
else:
    _storage_provider = get_provider(cfg.storage.primary, cfg)
```

只有确实需要共享运行时状态时才加特殊分支；普通 Provider 优先走注册表。

## WebUI 文件管理注意事项

`webui/services/file_manager.py` 会把空目录 ID 或 `"root"` 规范化成平台根目录。新增平台如果根目录不是 `"root"`，需要更新 `_get_root_id()`：

```python
return {
    "google_drive": "root",
    "pan115": "0",
    "example": "root",
}.get(provider_name, "root")
```

如果平台不支持回收站、永久删除、覆盖上传或移动重命名，应在 Provider 内做兼容处理，并对不可支持的操作抛出明确异常。

## 最小测试建议

新增测试建议放在 `test/test_storage.py` 或单独的 `test/test_<provider>_storage.py`：

- `from_config()` 能从 `Config.from_dict()` 创建 Provider。
- 平台原始文件能正确转换成 `CloudFile`。
- `list_files()`、`find_file()`、`get_file()` 返回统一字段。
- `create_folder()`、`rename_file()`、`move_file()` 参数语义符合统一接口。
- `upload_text()` / `upload_bytes()` 的 `overwrite` 行为符合预期。
- `delete_file()` / `trash_file()` 对不支持场景有明确错误。

运行：

```bash
conda run -n myself python -m pytest test/test_storage.py
```

如果改动影响前端文件管理或配置页面，还应运行：

```bash
cd frontend && npm run build
```

## 接入检查清单

- 新增 `storage/<provider>.py` 并实现 `StorageProvider`。
- 在 `storage/__init__.py` 注册 provider。
- 在 `mediaparser/config.py` 增加配置类和 active folder helper 分支。
- 在 `config/config.example.yaml` 写明配置示例。
- 必要时更新 `webui/core/runtime.py` 和 `webui/services/file_manager.py`。
- 添加测试并确认不会破坏 `google_drive`、`pan115` 现有行为。

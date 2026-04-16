"""
storage —— 统一云存储抽象层

所有网盘 Provider 实现统一的 StorageProvider ABC，
Pipeline / Organizer / WebUI 只依赖此层，无需关心底层网盘实现。

新增网盘只需 3 步：
  1. 实现 StorageProvider ABC（如 storage/onedrive.py）
  2. 在此文件注册：register_provider("onedrive", OneDriveProvider)
  3. 在 config.example.yaml 添加配置段

用法：
    from storage import get_provider, CloudFile, StorageProvider

    provider = get_provider("google_drive", cfg)
    files = provider.list_files(folder_id="xxx")
"""

from storage.base import CloudFile, FileType, StorageProvider

# ── Provider 注册表 ───────────────────────────────────────

_REGISTRY: dict[str, type[StorageProvider]] = {}


def register_provider(name: str, cls: type[StorageProvider]) -> None:
    """注册一个存储 Provider 类"""
    _REGISTRY[name] = cls


def get_provider(name: str, cfg) -> StorageProvider:
    """
    根据名称和配置创建 Provider 实例。

    参数：
        name : Provider 名称，如 "google_drive"、"pan115"
        cfg  : Config 对象（mediaparser.Config）
    """
    if name not in _REGISTRY:
        available = ", ".join(_REGISTRY.keys()) or "(无)"
        raise ValueError(f"未知的存储 Provider: {name!r}，可用: {available}")
    return _REGISTRY[name].from_config(cfg)


def list_providers() -> list[str]:
    """列出所有已注册的 Provider 名称"""
    return list(_REGISTRY.keys())


# ── 自动注册内置 Provider ─────────────────────────────────

from storage.google_drive import GoogleDriveProvider  # noqa: E402
from storage.pan115 import Pan115Provider  # noqa: E402
from storage.quark import QuarkProvider  # noqa: E402

register_provider("google_drive", GoogleDriveProvider)
register_provider("pan115", Pan115Provider)
register_provider("quark", QuarkProvider)

__all__ = [
    "CloudFile",
    "FileType",
    "StorageProvider",
    "GoogleDriveProvider",
    "Pan115Provider",
    "QuarkProvider",
    "get_provider",
    "list_providers",
    "register_provider",
]

"""
u115pan —— 115 开放平台独立接口层

说明：
  - 本目录仅提供独立外部接口封装
  - 当前未接入主程序
  - 目录名已使用合法 Python 包名 `u115pan`
"""

from .auth import (
    generate_code_challenge,
    generate_code_verifier,
    load_token,
    save_token,
)
from .client import Pan115Client
from .errors import Pan115ApiError, Pan115AuthError, Pan115Error, Pan115RateLimitError
from .models import (
    DeviceCodeSession,
    Pan115File,
    Pan115SpaceInfo,
    Pan115Token,
    QrcodeStatus,
    RecycleBinItem,
    ResumeInfo,
    UploadHashes,
    UploadInitInfo,
    UploadToken,
)
from .offline import (
    OfflineClient,
    OfflineQuotaInfo,
    OfflineTask,
    OfflineTaskList,
    OfflineUrlAddResult,
    QuotaExpireInfo,
    QuotaPackage,
    TorrentFileItem,
    TorrentInfo,
)
from .runtime import U115RuntimeManager, get_runtime_manager

__all__ = [
    "Pan115ApiError",
    "Pan115AuthError",
    "Pan115Client",
    "Pan115Error",
    "Pan115File",
    "Pan115RateLimitError",
    "Pan115SpaceInfo",
    "Pan115Token",
    "OfflineClient",
    "OfflineQuotaInfo",
    "OfflineTask",
    "OfflineTaskList",
    "OfflineUrlAddResult",
    "DeviceCodeSession",
    "QuotaExpireInfo",
    "QuotaPackage",
    "QrcodeStatus",
    "RecycleBinItem",
    "ResumeInfo",
    "TorrentFileItem",
    "TorrentInfo",
    "U115RuntimeManager",
    "get_runtime_manager",
    "UploadHashes",
    "UploadInitInfo",
    "UploadToken",
    "generate_code_challenge",
    "generate_code_verifier",
    "load_token",
    "save_token",
]

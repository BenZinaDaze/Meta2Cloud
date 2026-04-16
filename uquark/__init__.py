"""
uquark —— 夸克网盘独立接口层

提供夸克网盘 API 封装，支持文件管理、搜索、上传、下载等操作。
"""

from .auth import QuarkAuth, load_token, save_token
from .client import QuarkClient
from .errors import QuarkApiError, QuarkAuthError, QuarkError
from .models import (
    QuarkFile,
    QuarkSpaceInfo,
    QuarkToken,
)
from .runtime import QuarkRuntimeManager, get_runtime_manager

__all__ = [
    "QuarkApiError",
    "QuarkAuth",
    "QuarkAuthError",
    "QuarkClient",
    "QuarkError",
    "QuarkFile",
    "QuarkRuntimeManager",
    "QuarkSpaceInfo",
    "QuarkToken",
    "get_runtime_manager",
    "load_token",
    "save_token",
]
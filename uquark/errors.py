"""
uquark/errors.py —— 夸克网盘异常定义
"""

from __future__ import annotations

from typing import Any, Optional


class QuarkError(Exception):
    """夸克网盘客户端基础异常"""


class QuarkApiError(QuarkError):
    """夸克网盘业务接口异常"""

    def __init__(
        self,
        message: str,
        *,
        code: Optional[int] = None,
        payload: Optional[dict[str, Any]] = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.payload = payload or {}


class QuarkAuthError(QuarkApiError):
    """认证或令牌异常"""

"""
u115pan/errors.py —— 115 开放平台异常定义
"""

from __future__ import annotations

from typing import Any, Optional


class Pan115Error(Exception):
    """115 客户端基础异常"""


class Pan115ApiError(Pan115Error):
    """115 业务接口异常"""

    def __init__(
        self,
        message: str,
        *,
        code: Optional[int] = None,
        state: Optional[bool] = None,
        payload: Optional[dict[str, Any]] = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.state = state
        self.payload = payload or {}


class Pan115RateLimitError(Pan115ApiError):
    """命中 115 风控或限流"""


class Pan115AuthError(Pan115ApiError):
    """认证或令牌异常"""

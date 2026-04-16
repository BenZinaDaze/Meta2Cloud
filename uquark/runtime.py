"""
uquark/runtime.py —— 夸克网盘客户端运行时管理
"""

from __future__ import annotations

import os
import threading
from dataclasses import dataclass
from typing import Optional

from .client import QuarkClient
from .models import QuarkToken


@dataclass
class _ClientEntry:
    client: QuarkClient
    token_mtime_ns: Optional[int]


class QuarkRuntimeManager:
    """按 token_path 维护与 token 文件状态同步的共享 Quark client。"""

    def __init__(self) -> None:
        self._lock = threading.RLock()
        self._clients: dict[str, _ClientEntry] = {}

    @staticmethod
    def _normalize_token_path(token_path: str) -> str:
        return os.path.abspath(token_path)

    @staticmethod
    def _safe_mtime_ns(token_path: str) -> Optional[int]:
        try:
            return os.stat(token_path).st_mtime_ns
        except FileNotFoundError:
            return None

    def _needs_reload(self, token_path: str) -> bool:
        entry = self._clients.get(token_path)
        if entry is None:
            return True
        return self._safe_mtime_ns(token_path) != entry.token_mtime_ns

    def get_client(self, *, token_path: str) -> QuarkClient:
        """获取共享 client；若 token 文件发生变化则自动重建。"""
        with self._lock:
            normalized = self._normalize_token_path(token_path)
            if self._needs_reload(normalized):
                client = QuarkClient.from_token_file(
                    token_path=normalized,
                    on_token_updated=self._handle_client_token_updated,
                )
                self._clients[normalized] = _ClientEntry(
                    client=client,
                    token_mtime_ns=self._safe_mtime_ns(normalized),
                )
            return self._clients[normalized].client

    def invalidate(self, *, token_path: Optional[str] = None) -> None:
        """主动失效共享 client。"""
        with self._lock:
            if token_path is None:
                self._clients.clear()
                return
            normalized = self._normalize_token_path(token_path)
            self._clients.pop(normalized, None)

    def sync_token(
        self,
        *,
        token_path: str,
        token: Optional[QuarkToken] = None,
    ) -> QuarkClient:
        """同步 token 到共享 client。"""
        with self._lock:
            client = self.get_client(token_path=token_path)
            if token is not None:
                client.token = token
            normalized = self._normalize_token_path(token_path)
            self._clients[normalized] = _ClientEntry(
                client=client,
                token_mtime_ns=self._safe_mtime_ns(normalized),
            )
            return client

    def _handle_client_token_updated(
        self,
        client: QuarkClient,
        token: QuarkToken,
    ) -> None:
        """当共享 client 完成 refresh/exchange 后，自动更新 manager 的内存视图。"""
        with self._lock:
            if not client.token_path:
                return
            normalized = self._normalize_token_path(client.token_path)
            self._clients[normalized] = _ClientEntry(
                client=client,
                token_mtime_ns=self._safe_mtime_ns(normalized),
            )


_SHARED_RUNTIME_MANAGER = QuarkRuntimeManager()


def get_runtime_manager() -> QuarkRuntimeManager:
    """返回进程级共享的 Quark runtime manager。"""
    return _SHARED_RUNTIME_MANAGER

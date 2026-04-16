"""
u115pan/runtime.py —— 115 客户端运行时管理

职责：
  - 统一管理进程内共享的 Pan115Client
  - 监测 token 文件路径 / client_id / mtime 变化
  - 在授权成功或 token 文件外部更新后，确保后续请求拿到最新 client
"""

from __future__ import annotations

import os
import threading
from dataclasses import dataclass
from typing import Optional

from .client import Pan115Client
from .models import Pan115Token


@dataclass
class _ClientEntry:
    client: Pan115Client
    token_mtime_ns: Optional[int]


class U115RuntimeManager:
    """按 (client_id, resolved_token_path) 维护与 token 文件状态同步的共享 115 client。"""

    def __init__(self) -> None:
        self._lock = threading.RLock()
        self._clients: dict[tuple[str, str], _ClientEntry] = {}

    @staticmethod
    def _normalize_token_path(token_path: str) -> str:
        return os.path.abspath(token_path)

    @staticmethod
    def _safe_mtime_ns(token_path: str) -> Optional[int]:
        try:
            return os.stat(token_path).st_mtime_ns
        except FileNotFoundError:
            return None

    def _make_key(self, client_id: str, token_path: str) -> tuple[str, str]:
        return (str(client_id), self._normalize_token_path(token_path))

    def _needs_reload(self, key: tuple[str, str]) -> bool:
        entry = self._clients.get(key)
        if entry is None:
            return True
        return self._safe_mtime_ns(key[1]) != entry.token_mtime_ns

    def get_client(self, *, client_id: str, token_path: str) -> Pan115Client:
        """获取共享 client；若配置或 token 文件发生变化则自动重建。"""
        with self._lock:
            key = self._make_key(client_id, token_path)
            resolved_token_path = key[1]
            if self._needs_reload(key):
                client = Pan115Client.from_token_file(
                    client_id=key[0],
                    token_path=resolved_token_path,
                    on_token_updated=self._handle_client_token_updated,
                )
                self._clients[key] = _ClientEntry(
                    client=client,
                    token_mtime_ns=self._safe_mtime_ns(resolved_token_path),
                )
            return self._clients[key].client

    def invalidate(self, *, client_id: Optional[str] = None, token_path: Optional[str] = None) -> None:
        """主动失效共享 client；未传参数时清空全部缓存。"""
        with self._lock:
            if client_id is None and token_path is None:
                self._clients.clear()
                return

            if client_id is not None and token_path is not None:
                key = self._make_key(client_id, token_path)
                self._clients.pop(key, None)
                return

            normalized_path = self._normalize_token_path(token_path) if token_path is not None else None
            keys_to_remove = [
                key for key in self._clients
                if (client_id is None or key[0] == client_id)
                and (normalized_path is None or key[1] == normalized_path)
            ]
            for key in keys_to_remove:
                self._clients.pop(key, None)

    def sync_token(
        self,
        *,
        client_id: str,
        token_path: str,
        token: Optional[Pan115Token] = None,
    ) -> Pan115Client:
        """
        在授权成功或 token 刷新后同步共享 client 的内存状态。

        如果当前共享 client 与给定 client_id/token_path 不匹配，则重建；
        否则直接更新内存中的 token，并刷新 token 文件 mtime 记录。
        """
        with self._lock:
            key = self._make_key(client_id, token_path)
            client = self.get_client(client_id=client_id, token_path=token_path)
            if token is not None:
                client.token = token
                client.token_path = key[1]
            self._clients[key] = _ClientEntry(
                client=client,
                token_mtime_ns=self._safe_mtime_ns(key[1]),
            )
            return client

    def _handle_client_token_updated(
        self,
        client: Pan115Client,
        token: Pan115Token,
    ) -> None:
        """
        当共享 client 自己完成 refresh/exchange 后，自动更新 manager 的内存视图。
        """
        with self._lock:
            if not client.token_path:
                return
            key = self._make_key(client.client_id, client.token_path)
            client.token_path = key[1]
            self._clients[key] = _ClientEntry(
                client=client,
                token_mtime_ns=self._safe_mtime_ns(key[1]),
            )


_SHARED_RUNTIME_MANAGER = U115RuntimeManager()


def get_runtime_manager() -> U115RuntimeManager:
    """返回进程级共享的 115 runtime manager。"""
    return _SHARED_RUNTIME_MANAGER

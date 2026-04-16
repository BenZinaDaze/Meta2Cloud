"""
webui/tmdb_cache.py —— 基于 tmdb_media 的实体缓存适配层

对外仍保留 `get/is_failed/set/set_failed/evict_expired/stats` 接口，
但底层不再以请求 path 作为主键，而是回写到 `tmdb_media`。
"""

from __future__ import annotations

import logging
import os
import time
from typing import Optional

from webui.library_store import get_library_store

logger = logging.getLogger(__name__)

_DEFAULT_TTL = {
    "movie": 7 * 86400,
    "tv": 6 * 86400,
    "season": 6 * 86400,
    "episode": 6 * 86400,
    "default": 1 * 86400,
}


def _pick_ttl(path: str) -> int:
    for key, ttl in _DEFAULT_TTL.items():
        if f"/{key}" in path and key != "default":
            return ttl
    return _DEFAULT_TTL["default"]


def _parse_path(path: str) -> Optional[dict]:
    normalized = (path or "").split("?", 1)[0]
    parts = [part for part in normalized.strip("/").split("/") if part]
    if len(parts) < 2:
        return None
    if parts[0] not in {"movie", "tv"}:
        return None
    try:
        tmdb_id = int(parts[1])
    except ValueError:
        return None
    parsed = {
        "media_type": parts[0],
        "tmdb_id": tmdb_id,
        "season_number": None,
        "episode_number": None,
        "resource_type": "detail",
    }
    if len(parts) >= 4 and parts[2] == "season":
        try:
            parsed["season_number"] = int(parts[3])
        except ValueError:
            return None
        parsed["resource_type"] = "season"
    if len(parts) >= 6 and parts[4] == "episode":
        try:
            parsed["episode_number"] = int(parts[5])
        except ValueError:
            return None
        parsed["resource_type"] = "episode"
    return parsed


class TmdbCache:
    def __init__(self, db_path: str):
        os.makedirs(os.path.dirname(db_path) if os.path.dirname(db_path) else ".", exist_ok=True)
        self._store = get_library_store()
        logger.info("TMDB 实体缓存启动 ：%s", db_path)

    def get(self, path: str, language: str = "") -> Optional[dict]:
        parsed = _parse_path(path)
        if parsed is None:
            return None
        entry = self._store.get_tmdb_entry(parsed["media_type"], parsed["tmdb_id"])
        if entry is None:
            return None
        if language and entry.get("language") not in {"", language}:
            return None
        if time.time() > float(entry.get("expires_at") or 0):
            return None
        if parsed["resource_type"] == "detail":
            return entry.get("raw_json") or None
        if parsed["resource_type"] == "season":
            season_details = entry.get("season_details_json") or {}
            return season_details.get(str(parsed["season_number"]))
        episode_details = entry.get("episode_details_json") or {}
        episode_key = f"{parsed['season_number']}:{parsed['episode_number']}"
        return episode_details.get(episode_key)

    def is_failed(self, path: str, language: str = "") -> bool:
        return False

    def set(self, path: str, data: dict, language: str = "", ttl: Optional[int] = None) -> None:
        parsed = _parse_path(path)
        if parsed is None or data is None:
            return
        ttl = ttl if ttl is not None else _pick_ttl(path)
        expires_at = time.time() + ttl
        if parsed["resource_type"] == "detail":
            self._store.upsert_tmdb_detail(
                media_type=parsed["media_type"],
                tmdb_id=parsed["tmdb_id"],
                language=language,
                data=data,
                expires_at=expires_at,
            )
            return
        if parsed["resource_type"] == "season":
            self._store.upsert_tmdb_season_detail(
                tmdb_id=parsed["tmdb_id"],
                language=language,
                season_number=int(parsed["season_number"]),
                data=data,
                expires_at=expires_at,
            )
            return
        self._store.upsert_tmdb_episode_detail(
            tmdb_id=parsed["tmdb_id"],
            language=language,
            season_number=int(parsed["season_number"]),
            episode_number=int(parsed["episode_number"]),
            data=data,
            expires_at=expires_at,
        )

    def set_failed(self, path: str, language: str = "", ttl: int = 300) -> None:
        logger.debug("TMDB 失败冷却已忽略（实体缓存模式）：%s", path)

    def evict_expired(self) -> int:
        return self._store.evict_tmdb_expired()

    def stats(self) -> dict:
        return self._store.tmdb_stats()

    def close(self):
        return None

"""
webui/tmdb_cache.py —— SQLite 缓存层，减少对 TMDB API 的重复请求

表结构（自动创建）：
  tmdb_cache(
    path       TEXT PRIMARY KEY,  -- TMDB API 路径，如 /tv/12345
    language   TEXT,              -- 请求语言（不同语言分开缓存）
    payload    TEXT,              -- JSON 响应体
    expires_at REAL               -- Unix 时间戳，过期则重新请求
  )

使用方式：
  from webui.tmdb_cache import TmdbCache
  cache = TmdbCache("webui/tmdb_cache.db")
  data = cache.get("/tv/12345", language="zh-CN")
  if data is None:
      data = fetch_from_tmdb(...)
      cache.set("/tv/12345", data, language="zh-CN", ttl=86400)
"""

import json
import logging
import os
import sqlite3
import time
from typing import Optional

logger = logging.getLogger(__name__)

# 各类路径的默认 TTL（秒）
_DEFAULT_TTL = {
    "movie":   7 * 86400,   # 电影详情  7 天
    "tv":      6 * 86400,   # 剧集详情  6 天
    "season":  6 * 86400,   # 季详情    6 天
    "episode": 6 * 86400,   # 集详情    6 天
    "default": 1 * 86400,
}

# 请求失败后的冷却时间（秒），期间不再重复请求 TMDB
_FAILURE_TTL = 5 * 60   # 5 分钟
_FAILURE_SENTINEL = "__FAILED__"  # 负缓存标记值

_CREATE_SQL = """
CREATE TABLE IF NOT EXISTS tmdb_cache (
    path       TEXT    NOT NULL,
    language   TEXT    NOT NULL DEFAULT '',
    payload    TEXT    NOT NULL,
    expires_at REAL    NOT NULL,
    PRIMARY KEY (path, language)
);
CREATE INDEX IF NOT EXISTS idx_expires ON tmdb_cache(expires_at);
"""


def _pick_ttl(path: str) -> int:
    for key, ttl in _DEFAULT_TTL.items():
        if f"/{key}" in path and key != "default":
            return ttl
    return _DEFAULT_TTL["default"]


class TmdbCache:
    def __init__(self, db_path: str):
        os.makedirs(os.path.dirname(db_path) if os.path.dirname(db_path) else ".", exist_ok=True)
        self._db_path = db_path
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.executescript(_CREATE_SQL)
        self._conn.commit()
        logger.info("TMDB 缓存启动：%s", db_path)

    # ── 读缓存 ──────────────────────────────────────────

    def get(self, path: str, language: str = "") -> Optional[dict]:
        """从缓存读取，过期或不存在返回 None；负缓存也返回 None（调用方用 is_failed 区分）"""
        try:
            cur = self._conn.execute(
                "SELECT payload, expires_at FROM tmdb_cache WHERE path=? AND language=?",
                (path, language),
            )
            row = cur.fetchone()
            if row is None:
                return None
            payload, expires_at = row
            if time.time() > expires_at:
                logger.debug("缓存过期：%s", path)
                return None
            if payload == _FAILURE_SENTINEL:
                return None   # 负缓存：仍在冷却中
            return json.loads(payload)
        except Exception as e:
            logger.warning("缓存读取失败 %s: %s", path, e)
            return None

    def is_failed(self, path: str, language: str = "") -> bool:
        """检查该路径是否处于失败冷却期（负缓存有效）"""
        try:
            cur = self._conn.execute(
                "SELECT payload, expires_at FROM tmdb_cache WHERE path=? AND language=?",
                (path, language),
            )
            row = cur.fetchone()
            if row is None:
                return False
            payload, expires_at = row
            return payload == _FAILURE_SENTINEL and time.time() <= expires_at
        except Exception:
            return False

    # ── 写缓存 ──────────────────────────────────────────

    def set(self, path: str, data: dict, language: str = "", ttl: Optional[int] = None) -> None:
        """写入缓存，ttl 为 None 时按路径自动选择"""
        if data is None:
            return
        if ttl is None:
            ttl = _pick_ttl(path)
        self._upsert(path, language, json.dumps(data, ensure_ascii=False), time.time() + ttl)

    def set_failed(self, path: str, language: str = "", ttl: int = _FAILURE_TTL) -> None:
        """写入负缓存（标记请求失败），冷却期内不再请求 TMDB"""
        self._upsert(path, language, _FAILURE_SENTINEL, time.time() + ttl)
        logger.debug("负缓存写入（冷却 %ds）：%s", ttl, path)

    def _upsert(self, path: str, language: str, payload: str, expires_at: float) -> None:
        try:
            self._conn.execute(
                """
                INSERT INTO tmdb_cache(path, language, payload, expires_at)
                VALUES (?, ?, ?, ?)
                ON CONFLICT(path, language) DO UPDATE SET
                    payload    = excluded.payload,
                    expires_at = excluded.expires_at
                """,
                (path, language, payload, expires_at),
            )
            self._conn.commit()
        except Exception as e:
            logger.warning("缓存写入失败 %s: %s", path, e)

    # ── 工具 ────────────────────────────────────────────

    def evict_expired(self) -> int:
        """清理过期条目，返回删除数量"""
        cur = self._conn.execute(
            "DELETE FROM tmdb_cache WHERE expires_at < ?", (time.time(),)
        )
        self._conn.commit()
        count = cur.rowcount
        if count:
            logger.info("已清理 %d 条过期缓存", count)
        return count

    def stats(self) -> dict:
        """返回缓存统计信息"""
        cur = self._conn.execute(
            "SELECT COUNT(*), SUM(CASE WHEN expires_at >= ? THEN 1 ELSE 0 END) FROM tmdb_cache",
            (time.time(),),
        )
        total, valid = cur.fetchone()
        return {"total": total or 0, "valid": valid or 0, "expired": (total or 0) - (valid or 0)}

    def close(self):
        self._conn.close()

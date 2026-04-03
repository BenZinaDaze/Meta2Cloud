"""
webui/library_store.py —— Google Drive 媒体库快照存储

将每次 Drive 扫描结果存入 SQLite，后续读取直接从本地 DB 获取，
无需每次都访问 Google Drive API。

表结构：
  library_snapshot(
    id          INTEGER PRIMARY KEY,
    scanned_at  TEXT,      -- ISO8601 时间戳
    movies_json TEXT,      -- JSON 数组
    tv_json     TEXT,      -- JSON 数组
    movie_count INTEGER,
    tv_count    INTEGER
  )
"""

import json
import logging
import os
import sqlite3
from datetime import datetime, timezone
from typing import Optional

logger = logging.getLogger(__name__)

_DDL = """
CREATE TABLE IF NOT EXISTS library_snapshot (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    scanned_at  TEXT    NOT NULL,
    movies_json TEXT    NOT NULL DEFAULT '[]',
    tv_json     TEXT    NOT NULL DEFAULT '[]',
    movie_count INTEGER NOT NULL DEFAULT 0,
    tv_count    INTEGER NOT NULL DEFAULT 0
);
"""

_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_DB_PATH = os.path.join(_ROOT, "data", "library.db")


class LibraryStore:
    def __init__(self, db_path: str = _DB_PATH):
        os.makedirs(os.path.dirname(db_path) if os.path.dirname(db_path) else ".", exist_ok=True)
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.executescript(_DDL)
        self._conn.commit()
        logger.info("媒体库存储启动：%s", db_path)

    # ── 读取 ────────────────────────────────────────────

    def get_snapshot(self) -> Optional[dict]:
        """返回最新快照，未扫描过则返回 None"""
        cur = self._conn.execute(
            "SELECT scanned_at, movies_json, tv_json, movie_count, tv_count "
            "FROM library_snapshot ORDER BY id DESC LIMIT 1"
        )
        row = cur.fetchone()
        if row is None:
            return None
        scanned_at, movies_json, tv_json, movie_count, tv_count = row
        return {
            "scanned_at": scanned_at,
            "movies": json.loads(movies_json),
            "tv_shows": json.loads(tv_json),
            "total_movies": movie_count,
            "total_tv": tv_count,
        }

    def last_scanned(self) -> Optional[str]:
        """返回最后扫描时间（ISO8601），从未扫描返回 None"""
        cur = self._conn.execute(
            "SELECT scanned_at FROM library_snapshot ORDER BY id DESC LIMIT 1"
        )
        row = cur.fetchone()
        return row[0] if row else None

    # ── 写入 ────────────────────────────────────────────

    def save_snapshot(self, movies: list, tv_shows: list) -> dict:
        """
        保存新快照，并与上一次对比，返回变更统计：
        {
          "new_movies": int,       新增电影数
          "new_tv": int,           新增剧集数
          "total_movies": int,
          "total_tv": int,
          "scanned_at": str,
        }
        """
        old = self.get_snapshot()
        # old snapshot 来自 JSON，已是 dict；movies/tv_shows 是 Pydantic MediaItem 对象
        old_movie_ids = {m["tmdb_id"] for m in (old["movies"] if old else []) if m.get("tmdb_id")}
        old_tv_ids    = {t["tmdb_id"] for t in (old["tv_shows"] if old else []) if t.get("tmdb_id")}

        new_movie_ids = {m.tmdb_id for m in movies   if m.tmdb_id}
        new_tv_ids    = {t.tmdb_id for t in tv_shows if t.tmdb_id}

        new_movies_count = len(new_movie_ids - old_movie_ids)
        new_tv_count     = len(new_tv_ids - old_tv_ids)

        now = datetime.now(timezone.utc).isoformat()

        self._conn.execute(
            "INSERT INTO library_snapshot(scanned_at, movies_json, tv_json, movie_count, tv_count) "
            "VALUES (?, ?, ?, ?, ?)",
            (
                now,
                json.dumps([m if isinstance(m, dict) else m.model_dump() for m in movies],
                           ensure_ascii=False),
                json.dumps([t if isinstance(t, dict) else t.model_dump() for t in tv_shows],
                           ensure_ascii=False),
                len(movies),
                len(tv_shows),
            ),
        )
        self._conn.commit()

        # 只保留最近 5 次快照，防止数据库无限增长
        self._conn.execute(
            "DELETE FROM library_snapshot WHERE id NOT IN "
            "(SELECT id FROM library_snapshot ORDER BY id DESC LIMIT 5)"
        )
        self._conn.commit()

        logger.info(
            "媒体库快照已保存：%d 部电影，%d 部剧集（新增 %d / %d）",
            len(movies), len(tv_shows), new_movies_count, new_tv_count,
        )
        return {
            "new_movies": new_movies_count,
            "new_tv": new_tv_count,
            "total_movies": len(movies),
            "total_tv": len(tv_shows),
            "scanned_at": now,
        }

    def close(self):
        self._conn.close()


# 全局单例
_store: Optional[LibraryStore] = None


def get_library_store() -> LibraryStore:
    global _store
    if _store is None:
        _store = LibraryStore()
    return _store

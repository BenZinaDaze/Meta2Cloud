"""
webui/ingest_store.py —— 入库历史记录存储
"""

from __future__ import annotations

import logging
import os
import sqlite3
from datetime import datetime, timedelta, timezone
from typing import Any, Optional

logger = logging.getLogger(__name__)

_DDL = """
CREATE TABLE IF NOT EXISTS ingest_history (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    media_type      TEXT    NOT NULL,
    tmdb_id         INTEGER NOT NULL DEFAULT 0,
    title           TEXT    NOT NULL,
    original_title  TEXT    NOT NULL DEFAULT '',
    year            TEXT    NOT NULL DEFAULT '',
    season          INTEGER DEFAULT NULL,
    episode         INTEGER DEFAULT NULL,
    episode_title   TEXT    NOT NULL DEFAULT '',
    poster_path     TEXT    NOT NULL DEFAULT '',
    drive_folder_id TEXT    NOT NULL DEFAULT '',
    original_name   TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL DEFAULT 'success',
    error_message   TEXT    NOT NULL DEFAULT '',
    ingested_at     TEXT    NOT NULL,
    created_at      TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_ingest_history_media_type
ON ingest_history(media_type);

CREATE INDEX IF NOT EXISTS idx_ingest_history_tmdb_id
ON ingest_history(media_type, tmdb_id);

CREATE INDEX IF NOT EXISTS idx_ingest_history_ingested_at
ON ingest_history(ingested_at DESC);
"""

_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_DB_PATH = os.path.join(_ROOT, "config", "data", "library.db")


class IngestStore:
    def __init__(self, db_path: str = _DB_PATH):
        os.makedirs(os.path.dirname(db_path) if os.path.dirname(db_path) else ".", exist_ok=True)
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA foreign_keys=ON")
        self._conn.executescript(_DDL)
        self._conn.commit()
        logger.info("入库历史存储启动：%s", db_path)

    @staticmethod
    def _utc_now() -> str:
        return datetime.now(timezone.utc).isoformat()

    def record_ingest(
        self,
        *,
        media_type: str,
        tmdb_id: int = 0,
        title: str,
        original_title: str = "",
        year: str = "",
        season: Optional[int] = None,
        episode: Optional[int] = None,
        episode_title: str = "",
        poster_path: str = "",
        drive_folder_id: str = "",
        original_name: str = "",
        status: str = "success",
        error_message: str = "",
        ingested_at: Optional[str] = None,
    ) -> int:
        now = self._utc_now()
        cursor = self._conn.execute(
            """
            INSERT INTO ingest_history(
                media_type, tmdb_id, title, original_title, year,
                season, episode, episode_title, poster_path,
                drive_folder_id, original_name, status, error_message,
                ingested_at, created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                media_type, tmdb_id, title, original_title, year,
                season, episode, episode_title, poster_path,
                drive_folder_id, original_name, status, error_message,
                ingested_at or now, now,
            ),
        )
        self._conn.commit()
        return cursor.lastrowid

    def query(
        self,
        *,
        page: int = 1,
        page_size: int = 20,
        media_type: Optional[str] = None,
        tmdb_id: Optional[int] = None,
        status: Optional[str] = None,
        start_date: Optional[str] = None,
        end_date: Optional[str] = None,
        keyword: Optional[str] = None,
    ) -> dict[str, Any]:
        offset = (max(1, page) - 1) * max(1, page_size)
        conditions: list[str] = []
        params: list[Any] = []

        if media_type:
            conditions.append("media_type = ?")
            params.append(media_type)
        if tmdb_id is not None and tmdb_id > 0:
            conditions.append("tmdb_id = ?")
            params.append(tmdb_id)
        if status:
            conditions.append("status = ?")
            params.append(status)
        if start_date:
            conditions.append("ingested_at >= ?")
            params.append(start_date)
        if end_date:
            conditions.append("ingested_at <= ?")
            params.append(end_date)
        if keyword:
            conditions.append("(title LIKE ? OR original_title LIKE ?)")
            params.extend([f"%{keyword}%", f"%{keyword}%"])

        where_clause = " AND ".join(conditions) if conditions else "1=1"

        count_row = self._conn.execute(
            f"SELECT COUNT(*) as total FROM ingest_history WHERE {where_clause}",
            params,
        ).fetchone()
        total = count_row["total"] if count_row else 0

        rows = self._conn.execute(
            f"""
            SELECT * FROM ingest_history
            WHERE {where_clause}
            ORDER BY ingested_at DESC
            LIMIT ? OFFSET ?
            """,
            params + [page_size, offset],
        ).fetchall()

        items = [dict(row) for row in rows]
        total_pages = (total + page_size - 1) // page_size if total > 0 else 1

        return {
            "items": items,
            "pagination": {
                "page": page,
                "page_size": page_size,
                "total": total,
                "total_pages": total_pages,
            },
        }

    def get_recent(self, limit: int = 10) -> list[dict[str, Any]]:
        rows = self._conn.execute(
            "SELECT * FROM ingest_history ORDER BY ingested_at DESC LIMIT ?",
            (limit,),
        ).fetchall()
        return [dict(row) for row in rows]

    def stats(self, days: int = 7) -> dict[str, Any]:
        cutoff = (datetime.now(timezone.utc) - timedelta(days=days)).isoformat()
        row = self._conn.execute(
            """
            SELECT
                COUNT(*) as total,
                COUNT(CASE WHEN media_type = 'movie' THEN 1 END) as movies,
                COUNT(CASE WHEN media_type = 'tv' THEN 1 END) as tv_episodes,
                COUNT(CASE WHEN status = 'success' THEN 1 END) as success,
                COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed,
                COUNT(CASE WHEN status = 'no_tmdb' THEN 1 END) as no_tmdb
            FROM ingest_history
            WHERE ingested_at >= ?
            """,
            (cutoff,),
        ).fetchone()

        return {
            "days": days,
            "total": row["total"] or 0,
            "movies": row["movies"] or 0,
            "tv_episodes": row["tv_episodes"] or 0,
            "success": row["success"] or 0,
            "failed": row["failed"] or 0,
            "no_tmdb": row["no_tmdb"] or 0,
        }

    def cleanup(self, retention_days: int = 30) -> int:
        cutoff = (datetime.now(timezone.utc) - timedelta(days=retention_days)).isoformat()
        cursor = self._conn.execute(
            "DELETE FROM ingest_history WHERE ingested_at < ?",
            (cutoff,),
        )
        self._conn.commit()
        return cursor.rowcount

    def close(self):
        self._conn.close()


_store: Optional[IngestStore] = None


def get_ingest_store() -> IngestStore:
    global _store
    if _store is None:
        _store = IngestStore()
    return _store

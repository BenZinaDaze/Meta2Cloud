"""
webui/library_store.py —— 实体化媒体存储

职责：
1. `tmdb_media`：缓存 TMDB 主信息，按 `(media_type, tmdb_id)` 关联。
2. `library_media`：缓存媒体库入库状态，按 `drive_folder_id` 唯一。
3. `app_state`：存放最后一次扫描时间等少量全局状态。
"""

from __future__ import annotations

import json
import logging
import os
import sqlite3
from datetime import datetime, timezone
from typing import Any, Optional

logger = logging.getLogger(__name__)
_TMDB_IMG_BASE = "https://image.tmdb.org/t/p/w500"
_TMDB_IMG_ORIG = "https://image.tmdb.org/t/p/original"

_DDL = """
CREATE TABLE IF NOT EXISTS tmdb_media (
    media_type          TEXT    NOT NULL,
    tmdb_id             INTEGER NOT NULL,
    language            TEXT    NOT NULL DEFAULT '',
    title               TEXT    NOT NULL DEFAULT '',
    original_title      TEXT    NOT NULL DEFAULT '',
    original_language   TEXT    NOT NULL DEFAULT '',
    overview            TEXT    NOT NULL DEFAULT '',
    poster_path         TEXT    NOT NULL DEFAULT '',
    backdrop_path       TEXT    NOT NULL DEFAULT '',
    release_date        TEXT    NOT NULL DEFAULT '',
    first_air_date      TEXT    NOT NULL DEFAULT '',
    status              TEXT    NOT NULL DEFAULT '',
    vote_average        REAL    NOT NULL DEFAULT 0,
    vote_count          INTEGER NOT NULL DEFAULT 0,
    raw_json            TEXT    NOT NULL DEFAULT '{}',
    season_details_json TEXT    NOT NULL DEFAULT '{}',
    episode_details_json TEXT   NOT NULL DEFAULT '{}',
    last_synced_at      TEXT    NOT NULL,
    expires_at          REAL    NOT NULL DEFAULT 0,
    PRIMARY KEY (media_type, tmdb_id)
);

CREATE TABLE IF NOT EXISTS library_media (
    drive_folder_id      TEXT    PRIMARY KEY,
    media_type           TEXT    NOT NULL,
    tmdb_id              INTEGER NOT NULL DEFAULT 0,
    title                TEXT    NOT NULL DEFAULT '',
    original_title       TEXT    NOT NULL DEFAULT '',
    year                 TEXT    NOT NULL DEFAULT '',
    poster_url           TEXT,
    backdrop_url         TEXT,
    overview             TEXT    NOT NULL DEFAULT '',
    rating               REAL    NOT NULL DEFAULT 0,
    status               TEXT    NOT NULL DEFAULT '',
    total_episodes       INTEGER,
    in_library_episodes  INTEGER,
    raw_json             TEXT    NOT NULL DEFAULT '{}',
    in_library           INTEGER NOT NULL DEFAULT 1,
    last_scanned_at      TEXT    NOT NULL,
    updated_at           TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_library_media_tmdb
ON library_media(media_type, tmdb_id)
WHERE tmdb_id > 0;

CREATE INDEX IF NOT EXISTS idx_library_media_media_type
ON library_media(media_type);

CREATE INDEX IF NOT EXISTS idx_tmdb_media_title
ON tmdb_media(title);

CREATE TABLE IF NOT EXISTS app_state (
    key    TEXT PRIMARY KEY,
    value  TEXT NOT NULL
);
"""

_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_DB_PATH = os.path.join(_ROOT, "config", "data", "library.db")
_SCAN_AT_KEY = "library.last_scanned_at"


class LibraryStore:
    def __init__(self, db_path: str = _DB_PATH):
        os.makedirs(os.path.dirname(db_path) if os.path.dirname(db_path) else ".", exist_ok=True)
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA foreign_keys=ON")
        self._conn.executescript(_DDL)
        self._conn.commit()
        logger.info("媒体实体存储启动：%s", db_path)

    @staticmethod
    def _utc_now() -> str:
        return datetime.now(timezone.utc).isoformat()

    def _set_state(self, key: str, value: str) -> None:
        self._conn.execute(
            """
            INSERT INTO app_state(key, value)
            VALUES (?, ?)
            ON CONFLICT(key) DO UPDATE SET value = excluded.value
            """,
            (key, value),
        )

    def _get_state(self, key: str) -> Optional[str]:
        row = self._conn.execute("SELECT value FROM app_state WHERE key = ?", (key,)).fetchone()
        return row["value"] if row else None

    # ── TMDB 缓存 ──────────────────────────────────────

    def get_tmdb_entry(self, media_type: str, tmdb_id: int) -> Optional[dict[str, Any]]:
        row = self._conn.execute(
            "SELECT * FROM tmdb_media WHERE media_type = ? AND tmdb_id = ?",
            (media_type, tmdb_id),
        ).fetchone()
        if row is None:
            return None
        data = dict(row)
        for key in ("raw_json", "season_details_json", "episode_details_json"):
            data[key] = json.loads(data.get(key) or "{}")
        return data

    def upsert_tmdb_detail(
        self,
        *,
        media_type: str,
        tmdb_id: int,
        language: str,
        data: dict[str, Any],
        expires_at: float,
    ) -> None:
        now = self._utc_now()
        current = self.get_tmdb_entry(media_type, tmdb_id) or {}
        self._conn.execute(
            """
            INSERT INTO tmdb_media(
                media_type, tmdb_id, language, title, original_title, original_language,
                overview, poster_path, backdrop_path, release_date, first_air_date,
                status, vote_average, vote_count, raw_json, season_details_json,
                episode_details_json, last_synced_at, expires_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
                language = excluded.language,
                title = excluded.title,
                original_title = excluded.original_title,
                original_language = excluded.original_language,
                overview = excluded.overview,
                poster_path = excluded.poster_path,
                backdrop_path = excluded.backdrop_path,
                release_date = excluded.release_date,
                first_air_date = excluded.first_air_date,
                status = excluded.status,
                vote_average = excluded.vote_average,
                vote_count = excluded.vote_count,
                raw_json = excluded.raw_json,
                season_details_json = excluded.season_details_json,
                episode_details_json = excluded.episode_details_json,
                last_synced_at = excluded.last_synced_at,
                expires_at = excluded.expires_at
            """,
            (
                media_type,
                tmdb_id,
                language,
                data.get("title") or data.get("name") or "",
                data.get("original_title") or data.get("original_name") or "",
                data.get("original_language") or "",
                data.get("overview") or "",
                data.get("poster_path") or "",
                data.get("backdrop_path") or "",
                data.get("release_date") or "",
                data.get("first_air_date") or "",
                data.get("status") or "",
                float(data.get("vote_average") or 0),
                int(data.get("vote_count") or 0),
                json.dumps(data, ensure_ascii=False),
                json.dumps(current.get("season_details_json") or {}, ensure_ascii=False),
                json.dumps(current.get("episode_details_json") or {}, ensure_ascii=False),
                now,
                expires_at,
            ),
        )
        self._conn.commit()

    def upsert_tmdb_season_detail(
        self,
        *,
        tmdb_id: int,
        language: str,
        season_number: int,
        data: dict[str, Any],
        expires_at: float,
    ) -> None:
        current = self.get_tmdb_entry("tv", tmdb_id) or {}
        seasons = current.get("season_details_json") or {}
        seasons[str(season_number)] = data
        self._conn.execute(
            """
            INSERT INTO tmdb_media(
                media_type, tmdb_id, language, raw_json, season_details_json,
                episode_details_json, last_synced_at, expires_at
            ) VALUES ('tv', ?, ?, '{}', ?, ?, ?, ?)
            ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
                language = excluded.language,
                season_details_json = excluded.season_details_json,
                episode_details_json = excluded.episode_details_json,
                last_synced_at = excluded.last_synced_at,
                expires_at = excluded.expires_at
            """,
            (
                tmdb_id,
                language,
                json.dumps(seasons, ensure_ascii=False),
                json.dumps(current.get("episode_details_json") or {}, ensure_ascii=False),
                self._utc_now(),
                expires_at,
            ),
        )
        self._conn.commit()

    def upsert_tmdb_episode_detail(
        self,
        *,
        tmdb_id: int,
        language: str,
        season_number: int,
        episode_number: int,
        data: dict[str, Any],
        expires_at: float,
    ) -> None:
        current = self.get_tmdb_entry("tv", tmdb_id) or {}
        episodes = current.get("episode_details_json") or {}
        episodes[f"{season_number}:{episode_number}"] = data
        self._conn.execute(
            """
            INSERT INTO tmdb_media(
                media_type, tmdb_id, language, raw_json, season_details_json,
                episode_details_json, last_synced_at, expires_at
            ) VALUES ('tv', ?, ?, '{}', ?, ?, ?, ?)
            ON CONFLICT(media_type, tmdb_id) DO UPDATE SET
                language = excluded.language,
                season_details_json = excluded.season_details_json,
                episode_details_json = excluded.episode_details_json,
                last_synced_at = excluded.last_synced_at,
                expires_at = excluded.expires_at
            """,
            (
                tmdb_id,
                language,
                json.dumps(current.get("season_details_json") or {}, ensure_ascii=False),
                json.dumps(episodes, ensure_ascii=False),
                self._utc_now(),
                expires_at,
            ),
        )
        self._conn.commit()

    def evict_tmdb_expired(self) -> int:
        cur = self._conn.execute(
            "DELETE FROM tmdb_media WHERE expires_at > 0 AND expires_at < ?",
            (datetime.now(timezone.utc).timestamp(),),
        )
        self._conn.commit()
        return cur.rowcount

    def tmdb_stats(self) -> dict[str, int]:
        cur = self._conn.execute(
            "SELECT COUNT(*), SUM(CASE WHEN expires_at >= ? THEN 1 ELSE 0 END) FROM tmdb_media",
            (datetime.now(timezone.utc).timestamp(),),
        )
        total, valid = cur.fetchone()
        total = total or 0
        valid = valid or 0
        return {"total": total, "valid": valid, "expired": total - valid}

    # ── 媒体库快照 ──────────────────────────────────────

    def get_snapshot(self) -> Optional[dict]:
        scanned_at = self._get_state(_SCAN_AT_KEY)
        if scanned_at is None:
            return None
        rows = self._conn.execute(
            """
            SELECT *
            FROM library_media
            ORDER BY media_type ASC, year DESC, title COLLATE NOCASE ASC
            """
        ).fetchall()
        movies: list[dict[str, Any]] = []
        tv_shows: list[dict[str, Any]] = []
        for row in rows:
            payload = json.loads(row["raw_json"] or "{}")
            target = movies if row["media_type"] == "movie" else tv_shows
            target.append(payload)
        return {
            "scanned_at": scanned_at,
            "movies": movies,
            "tv_shows": tv_shows,
            "total_movies": len(movies),
            "total_tv": len(tv_shows),
        }

    def last_scanned(self) -> Optional[str]:
        return self._get_state(_SCAN_AT_KEY)

    def save_snapshot(self, movies: list, tv_shows: list) -> dict:
        old_rows = self._conn.execute(
            "SELECT media_type, tmdb_id FROM library_media WHERE tmdb_id > 0"
        ).fetchall()
        old_movie_ids = {row["tmdb_id"] for row in old_rows if row["media_type"] == "movie"}
        old_tv_ids = {row["tmdb_id"] for row in old_rows if row["media_type"] == "tv"}

        now = self._utc_now()
        movie_payloads = [m if isinstance(m, dict) else m.model_dump() for m in movies]
        tv_payloads = [t if isinstance(t, dict) else t.model_dump() for t in tv_shows]

        new_movie_ids = {item.get("tmdb_id") for item in movie_payloads if int(item.get("tmdb_id") or 0) > 0}
        new_tv_ids = {item.get("tmdb_id") for item in tv_payloads if int(item.get("tmdb_id") or 0) > 0}

        self._conn.execute("DELETE FROM library_media")
        for payload in movie_payloads + tv_payloads:
            self._upsert_library_item(payload, now)
        self._set_state(_SCAN_AT_KEY, now)
        self._conn.commit()

        diff = {
            "new_movies": len(new_movie_ids - old_movie_ids),
            "new_tv": len(new_tv_ids - old_tv_ids),
            "total_movies": len(movie_payloads),
            "total_tv": len(tv_payloads),
            "scanned_at": now,
        }
        logger.info(
            "媒体库实体快照已保存：%d 部电影，%d 部剧集（新增 %d / %d）",
            diff["total_movies"],
            diff["total_tv"],
            diff["new_movies"],
            diff["new_tv"],
        )
        return diff

    def _upsert_library_item(self, payload: dict[str, Any], scanned_at: str) -> None:
        drive_folder_id = payload.get("drive_folder_id") or ""
        if not drive_folder_id:
            return
        self._conn.execute(
            """
            INSERT INTO library_media(
                drive_folder_id, media_type, tmdb_id, title, original_title, year,
                poster_url, backdrop_url, overview, rating, status,
                total_episodes, in_library_episodes, raw_json, in_library,
                last_scanned_at, updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
            ON CONFLICT(drive_folder_id) DO UPDATE SET
                media_type = excluded.media_type,
                tmdb_id = excluded.tmdb_id,
                title = excluded.title,
                original_title = excluded.original_title,
                year = excluded.year,
                poster_url = excluded.poster_url,
                backdrop_url = excluded.backdrop_url,
                overview = excluded.overview,
                rating = excluded.rating,
                status = excluded.status,
                total_episodes = excluded.total_episodes,
                in_library_episodes = excluded.in_library_episodes,
                raw_json = excluded.raw_json,
                in_library = excluded.in_library,
                last_scanned_at = excluded.last_scanned_at,
                updated_at = excluded.updated_at
            """,
            (
                drive_folder_id,
                payload.get("media_type") or "",
                int(payload.get("tmdb_id") or 0),
                payload.get("title") or "",
                payload.get("original_title") or "",
                payload.get("year") or "",
                payload.get("poster_url"),
                payload.get("backdrop_url"),
                payload.get("overview") or "",
                float(payload.get("rating") or 0),
                payload.get("status") or "",
                payload.get("total_episodes"),
                payload.get("in_library_episodes"),
                json.dumps(payload, ensure_ascii=False),
                scanned_at,
                scanned_at,
            ),
        )

    def patch_item(self, drive_folder_id: str, updates: dict) -> bool:
        row = self._conn.execute(
            "SELECT raw_json FROM library_media WHERE drive_folder_id = ?",
            (drive_folder_id,),
        ).fetchone()
        if row is None:
            logger.warning("patch_item: 未找到 drive_folder_id=%s", drive_folder_id)
            return False
        payload = json.loads(row["raw_json"] or "{}")
        payload.update(updates)
        self._upsert_library_item(payload, self.last_scanned() or self._utc_now())
        self._conn.commit()
        logger.info("已更新库条目 drive_folder_id=%s", drive_folder_id)
        return True

    def get_library_item_by_tmdb(self, media_type: str, tmdb_id: int) -> Optional[dict[str, Any]]:
        row = self._conn.execute(
            """
            SELECT *
            FROM library_media
            WHERE media_type = ? AND tmdb_id = ?
            LIMIT 1
            """,
            (media_type, tmdb_id),
        ).fetchone()
        if row is None:
            return None
        return json.loads(row["raw_json"] or "{}")

    def list_library_items(self, media_type: Optional[str] = None) -> list[dict[str, Any]]:
        if media_type:
            rows = self._conn.execute(
                "SELECT raw_json FROM library_media WHERE media_type = ? ORDER BY year DESC, title COLLATE NOCASE ASC",
                (media_type,),
            ).fetchall()
        else:
            rows = self._conn.execute(
                "SELECT raw_json FROM library_media ORDER BY media_type ASC, year DESC, title COLLATE NOCASE ASC"
            ).fetchall()
        return [json.loads(row["raw_json"] or "{}") for row in rows]

    def get_joined_media_item(self, media_type: str, tmdb_id: int) -> Optional[dict[str, Any]]:
        library_item = self.get_library_item_by_tmdb(media_type, tmdb_id)
        if library_item:
            return {**library_item, "in_library": True}
        tmdb_entry = self.get_tmdb_entry(media_type, tmdb_id)
        if not tmdb_entry:
            return None
        raw = tmdb_entry.get("raw_json") or {}
        poster_path = raw.get("poster_path") or tmdb_entry.get("poster_path") or ""
        backdrop_path = raw.get("backdrop_path") or tmdb_entry.get("backdrop_path") or ""
        payload = dict(raw)
        payload.update({
            "tmdb_id": tmdb_id,
            "media_type": media_type,
            "title": raw.get("title") or raw.get("name") or tmdb_entry.get("title") or "",
            "original_title": raw.get("original_title") or raw.get("original_name") or tmdb_entry.get("original_title") or "",
            "year": (raw.get("release_date") or raw.get("first_air_date") or "")[:4],
            "poster_url": f"{_TMDB_IMG_BASE}{poster_path}" if poster_path else None,
            "backdrop_url": f"{_TMDB_IMG_ORIG}{backdrop_path}" if backdrop_path else None,
            "overview": raw.get("overview") or tmdb_entry.get("overview") or "",
            "rating": round(raw.get("vote_average") or tmdb_entry.get("vote_average") or 0, 1),
            "status": raw.get("status") or tmdb_entry.get("status") or "",
            "in_library": False,
        })
        return payload

    def get_stats(self) -> dict[str, Any]:
        snapshot = self.get_snapshot()
        if snapshot is None:
            return {
                "total_movies": 0,
                "total_tv_shows": 0,
                "total_episodes_in_library": 0,
                "total_episodes_on_tmdb": 0,
                "completion_rate": 0.0,
            }
        tv_shows = snapshot["tv_shows"]
        total_eps_tmdb = sum((show.get("total_episodes") or 0) for show in tv_shows)
        total_eps_lib = sum((show.get("in_library_episodes") or 0) for show in tv_shows)
        return {
            "total_movies": snapshot["total_movies"],
            "total_tv_shows": snapshot["total_tv"],
            "total_episodes_in_library": total_eps_lib,
            "total_episodes_on_tmdb": total_eps_tmdb,
            "completion_rate": round(total_eps_lib / total_eps_tmdb * 100, 1) if total_eps_tmdb > 0 else 0.0,
        }

    def close(self):
        self._conn.close()


_store: Optional[LibraryStore] = None


def get_library_store() -> LibraryStore:
    global _store
    if _store is None:
        _store = LibraryStore()
    return _store

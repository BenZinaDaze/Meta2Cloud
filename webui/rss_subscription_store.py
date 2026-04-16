from __future__ import annotations

import os
import sqlite3
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Optional


@dataclass
class SubscriptionRecord:
    id: int
    name: str
    media_title: str
    media_type: str
    tmdb_id: Optional[int]
    poster_url: Optional[str]
    site: str
    rss_url: str
    subgroup_name: str
    season_number: int
    start_episode: int
    keyword_all: str
    push_target: str
    enabled: int
    last_checked_at: Optional[str]
    last_matched_at: Optional[str]
    last_error: Optional[str]
    created_at: str
    updated_at: str


@dataclass
class SubscriptionHitRecord:
    id: int
    subscription_id: int
    episode_title: str
    season_number: Optional[int]
    episode_number: Optional[int]
    torrent_url: Optional[str]
    magnet_url: Optional[str]
    published_at: Optional[str]
    dedupe_key: str
    pushed_to: str
    push_status: str
    push_error: Optional[str]
    created_at: str


_DDL = """
CREATE TABLE IF NOT EXISTS rss_subscriptions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    name              TEXT NOT NULL,
    media_title       TEXT NOT NULL,
    media_type        TEXT NOT NULL DEFAULT 'tv',
    tmdb_id           INTEGER,
    poster_url        TEXT,
    site              TEXT NOT NULL,
    rss_url           TEXT NOT NULL,
    subgroup_name     TEXT NOT NULL DEFAULT '',
    season_number     INTEGER NOT NULL DEFAULT 1,
    start_episode     INTEGER NOT NULL DEFAULT 1,
    keyword_all       TEXT NOT NULL DEFAULT '[]',
    push_target       TEXT NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    last_checked_at   TEXT,
    last_matched_at   TEXT,
    last_error        TEXT,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rss_subscription_hits (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_id   INTEGER NOT NULL,
    episode_title     TEXT NOT NULL,
    season_number     INTEGER,
    episode_number    INTEGER,
    torrent_url       TEXT,
    magnet_url        TEXT,
    published_at      TEXT,
    dedupe_key        TEXT NOT NULL,
    pushed_to         TEXT NOT NULL,
    push_status       TEXT NOT NULL,
    push_error        TEXT,
    created_at        TEXT NOT NULL,
    FOREIGN KEY(subscription_id) REFERENCES rss_subscriptions(id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_rss_subscription_hits_dedupe
ON rss_subscription_hits(subscription_id, dedupe_key);
"""

_SUBSCRIPTION_COLUMN_MIGRATIONS: dict[str, str] = {
    "poster_url": "ALTER TABLE rss_subscriptions ADD COLUMN poster_url TEXT",
}


class RSSSubscriptionStore:
    def __init__(self, db_path: str):
        os.makedirs(os.path.dirname(db_path) if os.path.dirname(db_path) else ".", exist_ok=True)
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA foreign_keys=ON")
        self._conn.executescript(_DDL)
        self._migrate_schema()
        self._conn.commit()

    def _migrate_schema(self) -> None:
        subscription_columns = {
            row["name"]
            for row in self._conn.execute("PRAGMA table_info(rss_subscriptions)").fetchall()
        }
        for column, ddl in _SUBSCRIPTION_COLUMN_MIGRATIONS.items():
            if column in subscription_columns:
                continue
            self._conn.execute(ddl)

    @staticmethod
    def _utc_now() -> str:
        return datetime.now(timezone.utc).isoformat()

    def _row_to_subscription(self, row: sqlite3.Row | None) -> Optional[SubscriptionRecord]:
        if row is None:
            return None
        return SubscriptionRecord(**dict(row))

    def _row_to_hit(self, row: sqlite3.Row | None) -> Optional[SubscriptionHitRecord]:
        if row is None:
            return None
        return SubscriptionHitRecord(**dict(row))

    def create_subscription(self, payload: dict[str, Any]) -> SubscriptionRecord:
        now = self._utc_now()
        cursor = self._conn.execute(
            """
            INSERT INTO rss_subscriptions(
                name, media_title, media_type, tmdb_id, poster_url, site, rss_url, subgroup_name,
                season_number, start_episode, keyword_all, push_target, enabled,
                last_checked_at, last_matched_at, last_error, created_at, updated_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?, ?)
            """,
            (
                payload["name"],
                payload["media_title"],
                payload.get("media_type") or "tv",
                payload.get("tmdb_id"),
                payload.get("poster_url"),
                payload["site"],
                payload["rss_url"],
                payload.get("subgroup_name") or "",
                payload["season_number"],
                payload["start_episode"],
                payload["keyword_all"],
                payload["push_target"],
                1 if payload.get("enabled", True) else 0,
                now,
                now,
            ),
        )
        self._conn.commit()
        return self.get_subscription(int(cursor.lastrowid))  # type: ignore[return-value]

    def list_subscriptions(self) -> list[SubscriptionRecord]:
        rows = self._conn.execute(
            """
            SELECT *
            FROM rss_subscriptions
            ORDER BY updated_at DESC, id DESC
            """
        ).fetchall()
        return [self._row_to_subscription(row) for row in rows if row is not None]

    def list_enabled_subscriptions(self) -> list[SubscriptionRecord]:
        rows = self._conn.execute(
            """
            SELECT *
            FROM rss_subscriptions
            WHERE enabled = 1
            ORDER BY updated_at DESC, id DESC
            """
        ).fetchall()
        return [self._row_to_subscription(row) for row in rows if row is not None]

    def get_subscription(self, subscription_id: int) -> Optional[SubscriptionRecord]:
        row = self._conn.execute(
            "SELECT * FROM rss_subscriptions WHERE id = ?",
            (subscription_id,),
        ).fetchone()
        return self._row_to_subscription(row)

    def update_subscription(self, subscription_id: int, payload: dict[str, Any]) -> Optional[SubscriptionRecord]:
        current = self.get_subscription(subscription_id)
        if current is None:
            return None
        merged = {
            "name": current.name,
            "media_title": current.media_title,
            "media_type": current.media_type,
            "tmdb_id": current.tmdb_id,
            "poster_url": current.poster_url,
            "site": current.site,
            "rss_url": current.rss_url,
            "subgroup_name": current.subgroup_name,
            "season_number": current.season_number,
            "start_episode": current.start_episode,
            "keyword_all": current.keyword_all,
            "push_target": current.push_target,
            "enabled": current.enabled,
        }
        merged.update(payload)
        self._conn.execute(
            """
            UPDATE rss_subscriptions
            SET name = ?, media_title = ?, media_type = ?, tmdb_id = ?, poster_url = ?, site = ?, rss_url = ?, subgroup_name = ?,
                season_number = ?, start_episode = ?, keyword_all = ?, push_target = ?, enabled = ?, updated_at = ?
            WHERE id = ?
            """,
            (
                merged["name"],
                merged["media_title"],
                merged["media_type"],
                merged.get("tmdb_id"),
                merged.get("poster_url"),
                merged["site"],
                merged["rss_url"],
                merged.get("subgroup_name") or "",
                merged["season_number"],
                merged["start_episode"],
                merged["keyword_all"],
                merged["push_target"],
                1 if merged.get("enabled") else 0,
                self._utc_now(),
                subscription_id,
            ),
        )
        self._conn.commit()
        return self.get_subscription(subscription_id)

    def delete_subscription(self, subscription_id: int) -> bool:
        cursor = self._conn.execute("DELETE FROM rss_subscriptions WHERE id = ?", (subscription_id,))
        self._conn.commit()
        return cursor.rowcount > 0

    def mark_checked(self, subscription_id: int, *, checked_at: Optional[str] = None, error: Optional[str] = None) -> None:
        self._conn.execute(
            """
            UPDATE rss_subscriptions
            SET last_checked_at = ?, last_error = ?, updated_at = ?
            WHERE id = ?
            """,
            (checked_at or self._utc_now(), error, self._utc_now(), subscription_id),
        )
        self._conn.commit()

    def mark_matched(self, subscription_id: int, *, matched_at: Optional[str] = None) -> None:
        ts = matched_at or self._utc_now()
        self._conn.execute(
            """
            UPDATE rss_subscriptions
            SET last_matched_at = ?, updated_at = ?
            WHERE id = ?
            """,
            (ts, ts, subscription_id),
        )
        self._conn.commit()

    def record_hit(self, payload: dict[str, Any]) -> SubscriptionHitRecord:
        now = self._utc_now()
        cursor = self._conn.execute(
            """
            INSERT INTO rss_subscription_hits(
                subscription_id, episode_title, season_number, episode_number, torrent_url, magnet_url,
                published_at, dedupe_key, pushed_to, push_status, push_error, created_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                payload["subscription_id"],
                payload["episode_title"],
                payload.get("season_number"),
                payload.get("episode_number"),
                payload.get("torrent_url"),
                payload.get("magnet_url"),
                payload.get("published_at"),
                payload["dedupe_key"],
                payload["pushed_to"],
                payload["push_status"],
                payload.get("push_error"),
                now,
            ),
        )
        self._conn.commit()
        return self.get_hit(int(cursor.lastrowid))  # type: ignore[return-value]

    def get_hit(self, hit_id: int) -> Optional[SubscriptionHitRecord]:
        row = self._conn.execute(
            "SELECT * FROM rss_subscription_hits WHERE id = ?",
            (hit_id,),
        ).fetchone()
        return self._row_to_hit(row)

    def hit_exists(self, subscription_id: int, dedupe_key: str) -> bool:
        row = self._conn.execute(
            """
            SELECT 1
            FROM rss_subscription_hits
            WHERE subscription_id = ? AND dedupe_key = ?
            LIMIT 1
            """,
            (subscription_id, dedupe_key),
        ).fetchone()
        return row is not None

    def list_hits(self, subscription_id: int, limit: int = 5) -> list[SubscriptionHitRecord]:
        rows = self._conn.execute(
            """
            SELECT *
            FROM rss_subscription_hits
            WHERE subscription_id = ?
            ORDER BY created_at DESC, id DESC
            LIMIT ?
            """,
            (subscription_id, limit),
        ).fetchall()
        return [self._row_to_hit(row) for row in rows if row is not None]

    def count_hits(self, subscription_id: int) -> int:
        row = self._conn.execute(
            "SELECT COUNT(*) AS count FROM rss_subscription_hits WHERE subscription_id = ?",
            (subscription_id,),
        ).fetchone()
        return int(row["count"]) if row is not None else 0

    def close(self) -> None:
        self._conn.close()

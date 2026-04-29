import json
import logging
import os
import sqlite3
import threading
from collections import Counter
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


class LogStore:
    """SQLite-based application log store with WAL mode and retention cleanup."""

    def __init__(self, directory: str, retention_days: int = 7, legacy_path: Optional[str] = None):
        self._retention_days = max(1, int(retention_days or 7))
        self._lock = threading.Lock()
        self._conn: sqlite3.Connection | None = None
        os.makedirs(directory, exist_ok=True)
        self._db_path = os.path.join(directory, "app_logs.db")
        self._init_db()
        if legacy_path and os.path.exists(legacy_path):
            self._migrate_jsonl(legacy_path)

    def set_retention_days(self, retention_days: int) -> None:
        self._retention_days = max(1, int(retention_days or 7))

    def _get_conn(self) -> sqlite3.Connection:
        if self._conn is None:
            self._conn = sqlite3.connect(self._db_path, timeout=5, check_same_thread=False)
            self._conn.execute("PRAGMA journal_mode=WAL")
            self._conn.execute("PRAGMA synchronous=NORMAL")
            self._conn.row_factory = sqlite3.Row
        return self._conn

    def _init_db(self) -> None:
        conn = self._get_conn()
        conn.execute("""
            CREATE TABLE IF NOT EXISTS app_logs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                ts TEXT NOT NULL,
                category TEXT NOT NULL DEFAULT '',
                event TEXT NOT NULL DEFAULT '',
                level TEXT NOT NULL DEFAULT 'INFO',
                message TEXT NOT NULL DEFAULT '',
                details TEXT NOT NULL DEFAULT '{}'
            )
        """)
        conn.execute("CREATE INDEX IF NOT EXISTS idx_logs_ts ON app_logs(ts)")
        conn.execute("CREATE INDEX IF NOT EXISTS idx_logs_category ON app_logs(category)")
        conn.execute("CREATE INDEX IF NOT EXISTS idx_logs_level ON app_logs(level)")

    def _migrate_jsonl(self, legacy_path: str) -> None:
        try:
            with open(legacy_path, "r", encoding="utf-8") as fh:
                records = []
                for line in fh:
                    line = line.strip()
                    if not line:
                        continue
                    try:
                        item = json.loads(line)
                        records.append((
                            item.get("ts", datetime.now(timezone.utc).isoformat()),
                            item.get("category", ""),
                            item.get("event", ""),
                            item.get("level", "INFO"),
                            item.get("message", ""),
                            json.dumps(item.get("details", {}), ensure_ascii=False),
                        ))
                    except json.JSONDecodeError:
                        continue
            if records:
                self._get_conn().executemany(
                    "INSERT INTO app_logs (ts, category, event, level, message, details) VALUES (?, ?, ?, ?, ?, ?)",
                    records,
                )
            os.remove(legacy_path)
        except OSError:
            logger.warning("迁移旧日志文件失败：%s", legacy_path)

    def write(
        self,
        *,
        category: str,
        event: str,
        level: str = "INFO",
        message: str,
        details: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        ts = datetime.now(timezone.utc).isoformat()
        details_json = json.dumps(details or {}, ensure_ascii=False)
        with self._lock:
            self._get_conn().execute(
                "INSERT INTO app_logs (ts, category, event, level, message, details) VALUES (?, ?, ?, ?, ?, ?)",
                (ts, category, event, level.upper(), message, details_json),
            )
        return {
            "ts": ts,
            "category": category,
            "event": event,
            "level": level.upper(),
            "message": message,
            "details": details or {},
        }

    def read(
        self,
        *,
        limit: int = 200,
        category: Optional[str] = None,
        level: Optional[str] = None,
        since_id: int = 0,
    ) -> List[Dict[str, Any]]:
        with self._lock:
            self._cleanup_expired()
            conditions = ["id > ?"]
            params: list = [since_id]
            if category:
                conditions.append("category = ?")
                params.append(category)
            if level:
                conditions.append("level = ?")
                params.append(level.upper())
            where = " AND ".join(conditions)
            rows = self._get_conn().execute(
                f"SELECT id, ts, category, event, level, message, details FROM app_logs WHERE {where} ORDER BY id DESC LIMIT ?",
                params + [max(1, min(limit, 1000))],
            ).fetchall()
        items: List[Dict[str, Any]] = []
        for row in rows:
            try:
                details = json.loads(row["details"])
            except json.JSONDecodeError:
                details = {}
            items.append({
                "id": row["id"],
                "ts": row["ts"],
                "category": row["category"],
                "event": row["event"],
                "level": row["level"],
                "message": row["message"],
                "details": details,
            })
        return items

    def summary(self, *, recent_limit: int = 500) -> Dict[str, Any]:
        records = self.read(limit=recent_limit)
        by_category = Counter(item.get("category") or "unknown" for item in records)
        by_level = Counter(item.get("level") or "INFO" for item in records)
        return {
            "total": len(records),
            "byCategory": dict(by_category),
            "byLevel": dict(by_level),
            "retentionDays": self._retention_days,
        }

    def max_id(self) -> int:
        with self._lock:
            row = self._get_conn().execute("SELECT MAX(id) FROM app_logs").fetchone()
            return row[0] or 0

    def _cleanup_expired(self) -> None:
        cutoff = (datetime.now().astimezone() - timedelta(days=self._retention_days)).isoformat()
        self._get_conn().execute("DELETE FROM app_logs WHERE ts < ?", (cutoff,))

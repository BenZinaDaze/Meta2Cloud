import json
import logging
import os
import threading
from collections import Counter
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

logger = logging.getLogger(__name__)


class LogStore:
    """Daily-rotated JSONL application logs with retention cleanup."""

    def __init__(self, directory: str, retention_days: int = 7, legacy_path: Optional[str] = None):
        self._directory = directory
        self._retention_days = max(1, int(retention_days or 7))
        self._legacy_path = legacy_path
        self._lock = threading.Lock()
        os.makedirs(directory, exist_ok=True)
        self._migrate_legacy_file()

    def set_retention_days(self, retention_days: int) -> None:
        self._retention_days = max(1, int(retention_days or 7))

    def _log_path_for_day(self, day: datetime) -> str:
        filename = f"{day.strftime('%Y-%m-%d')}.jsonl"
        return os.path.join(self._directory, filename)

    def _list_log_files(self) -> List[str]:
        if not os.path.exists(self._directory):
            return []
        files = []
        for name in os.listdir(self._directory):
            if name.endswith(".jsonl"):
                files.append(os.path.join(self._directory, name))
        files.sort(reverse=True)
        return files

    def _cleanup_expired(self) -> None:
        cutoff = datetime.now().astimezone() - timedelta(days=self._retention_days)
        for path in self._list_log_files():
            stem = os.path.splitext(os.path.basename(path))[0]
            try:
                file_day = datetime.strptime(stem, "%Y-%m-%d").astimezone()
            except ValueError:
                continue
            if file_day.date() < cutoff.date():
                try:
                    os.remove(path)
                except OSError:
                    logger.warning("删除过期日志失败：%s", path)

    def _migrate_legacy_file(self) -> None:
        if not self._legacy_path or not os.path.exists(self._legacy_path):
            return
        target_path = self._log_path_for_day(datetime.now().astimezone())
        try:
            with open(self._legacy_path, "r", encoding="utf-8") as src, open(target_path, "a", encoding="utf-8") as dst:
                for line in src:
                    if line.strip():
                        dst.write(line.rstrip("\n") + "\n")
            os.remove(self._legacy_path)
        except OSError:
            logger.warning("迁移旧日志文件失败：%s", self._legacy_path)

    def write(
        self,
        *,
        category: str,
        event: str,
        level: str = "INFO",
        message: str,
        details: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        record = {
            "ts": datetime.now(timezone.utc).isoformat(),
            "category": category,
            "event": event,
            "level": level.upper(),
            "message": message,
            "details": details or {},
        }
        line = json.dumps(record, ensure_ascii=False)
        with self._lock:
            self._cleanup_expired()
            path = self._log_path_for_day(datetime.now().astimezone())
            with open(path, "a", encoding="utf-8") as fh:
                fh.write(line + "\n")
        return record

    def read(
        self,
        *,
        limit: int = 200,
        category: Optional[str] = None,
        level: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        if not os.path.exists(self._directory):
            return []

        records: List[Dict[str, Any]] = []
        wanted_level = level.upper() if level else None

        with self._lock:
            self._cleanup_expired()
            for path in self._list_log_files():
                with open(path, "r", encoding="utf-8") as fh:
                    for raw in fh:
                        raw = raw.strip()
                        if not raw:
                            continue
                        try:
                            item = json.loads(raw)
                        except json.JSONDecodeError:
                            logger.warning("跳过损坏日志行")
                            continue
                        if category and item.get("category") != category:
                            continue
                        if wanted_level and item.get("level") != wanted_level:
                            continue
                        records.append(item)

        records.sort(key=lambda item: item.get("ts", ""), reverse=True)
        return records[: max(1, min(limit, 1000))]

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

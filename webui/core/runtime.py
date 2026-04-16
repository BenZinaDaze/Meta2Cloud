import logging
import os
from pathlib import Path
from typing import Any, Dict, Optional

import yaml

from drive.client import DriveClient
from mediaparser import Config
from mediaparser.release_group import ReleaseGroupsMatcher
from storage.base import StorageProvider
from storage.pan115 import Pan115Provider
from webui.u115_auto_organize_store import U115AutoOrganizeStore
import u115pan


logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("webui")

_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_ROOT_DIR = _ROOT
_CONFIG_PATH = Path(_ROOT) / "config" / "config.yaml"
_PARSER_RULES_PATH = Path(_ROOT) / "config" / "parser-rules.yaml"
_LIBRARY_DB = os.path.join(_ROOT_DIR, "config", "data", "library.db")
_APP_LOG_DIR = os.path.join(_ROOT_DIR, "config", "data", "logs")
_LEGACY_APP_LOG_DB = os.path.join(_ROOT_DIR, "config", "data", "app_logs.jsonl")
_U115_AUTO_ORGANIZE_DB = os.path.join(_ROOT_DIR, "config", "data", "u115_auto_organize.db")

_cfg: Optional[Config] = None
_client: Optional[DriveClient] = None
_storage_provider: Optional[StorageProvider] = None
_tmdb_cache = None
_config_mtime_ns: Optional[tuple[Optional[int], Optional[int]]] = None
_u115_auto_organize_store: Optional[U115AutoOrganizeStore] = None
_u115_runtime = u115pan.get_runtime_manager()


def _read_yaml_dict(path: Path) -> Dict[str, Any]:
    if not path.exists():
        return {}
    return yaml.safe_load(path.read_text(encoding="utf-8")) or {}


def _get_mtime_ns(path: Path) -> Optional[int]:
    try:
        return path.stat().st_mtime_ns
    except FileNotFoundError:
        return None


def _get_config_signature() -> tuple[Optional[int], Optional[int]]:
    return (_get_mtime_ns(_CONFIG_PATH), _get_mtime_ns(_PARSER_RULES_PATH))


def _extract_parser_rules(data: Dict[str, Any]) -> Dict[str, Any]:
    parser = data.get("parser") or {}
    if not isinstance(parser, dict):
        return {"custom_words": [], "custom_release_groups": []}
    return {
        "custom_words": list(parser.get("custom_words") or []),
        "custom_release_groups": list(parser.get("custom_release_groups") or []),
    }


def _parse_int_field(section: Dict[str, Any], key: str, label: str, minimum: int, maximum: int) -> None:
    from fastapi import HTTPException

    if key not in section or section.get(key) in (None, ""):
        return
    try:
        value = int(section[key])
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail=f"{label} 必须是整数")
    if value < minimum or value > maximum:
        raise HTTPException(status_code=400, detail=f"{label} 必须在 {minimum} 到 {maximum} 之间")
    section[key] = value


def _validate_main_config_payload(payload: Dict[str, Any]) -> Dict[str, Any]:
    normalized = dict(payload or {})
    webui = dict(normalized.get("webui") or {})
    tmdb = dict(normalized.get("tmdb") or {})
    u115 = dict(normalized.get("u115") or {})
    rss = dict(normalized.get("rss") or {})
    aria2 = dict(normalized.get("aria2") or {})
    telegram = dict(normalized.get("telegram") or {})

    _parse_int_field(webui, "token_expire_hours", "Token 有效期", 1, 8760)
    _parse_int_field(webui, "log_retention_days", "日志保留天数", 1, 365)
    _parse_int_field(tmdb, "timeout", "TMDB 请求超时", 1, 120)
    _parse_int_field(u115, "auto_organize_poll_seconds", "115 自动整理轮询间隔", 10, 600)
    _parse_int_field(u115, "auto_organize_stable_seconds", "115 完成稳定等待", 0, 600)
    _parse_int_field(rss, "poll_seconds", "RSS 订阅轮询间隔", 10, 3600)
    _parse_int_field(aria2, "port", "Aria2 RPC 端口", 1, 65535)
    _parse_int_field(telegram, "debounce_seconds", "Telegram 防抖延时", 0, 600)

    normalized["webui"] = webui
    normalized["tmdb"] = tmdb
    normalized["u115"] = u115
    normalized["rss"] = rss
    normalized["aria2"] = aria2
    normalized["telegram"] = telegram
    return normalized


def _read_merged_config_dict() -> Dict[str, Any]:
    parsed = _read_yaml_dict(_CONFIG_PATH)
    parser_rules = _read_yaml_dict(_PARSER_RULES_PATH)
    if isinstance(parser_rules, dict) and "parser" in parser_rules and isinstance(parser_rules.get("parser"), dict):
        parser_rules = parser_rules["parser"]
    parsed["parser"] = _extract_parser_rules({"parser": parser_rules if isinstance(parser_rules, dict) else {}})
    return parsed


def _invalidate_runtime_cache_if_config_changed() -> None:
    global _cfg, _client, _storage_provider, _config_mtime_ns
    mtime_ns = _get_config_signature()
    if _config_mtime_ns is None:
        _config_mtime_ns = mtime_ns
        return
    if mtime_ns != _config_mtime_ns:
        logger.info("检测到配置文件已变更，重载配置与存储 Provider 缓存")
        _cfg = None
        _client = None
        _storage_provider = None
        _u115_runtime.invalidate()
        _config_mtime_ns = mtime_ns


def _invalidate_u115_runtime_cache() -> None:
    global _storage_provider
    _u115_runtime.invalidate()
    if isinstance(_storage_provider, Pan115Provider):
        _storage_provider = None


def get_config() -> Config:
    global _cfg, _config_mtime_ns
    _invalidate_runtime_cache_if_config_changed()
    if _cfg is None:
        _cfg = Config.load()
        _config_mtime_ns = _get_config_signature()
    return _cfg


def get_drive_client() -> DriveClient:
    global _client
    _invalidate_runtime_cache_if_config_changed()
    if _client is None:
        drive_cfg = get_config().drive
        _client = DriveClient.from_oauth(
            credentials_path=drive_cfg.credentials_json,
            token_path=drive_cfg.token_json,
        )
    return _client


def _u115_client():
    from webui.services.u115 import u115_client

    return u115_client()


def get_storage_provider() -> StorageProvider:
    global _storage_provider
    _invalidate_runtime_cache_if_config_changed()
    if _storage_provider is None:
        cfg = get_config()
        if cfg.storage.primary == "pan115":
            _storage_provider = Pan115Provider.from_client_getter(_u115_client)
        else:
            from storage import get_provider

            _storage_provider = get_provider(cfg.storage.primary, cfg)
    return _storage_provider


def _get_release_group_matcher() -> ReleaseGroupsMatcher:
    return ReleaseGroupsMatcher(get_config().parser.custom_release_groups)

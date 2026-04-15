#!/usr/bin/env python3
"""
webui/api.py —— Meta2Cloud 媒体库 Web UI 后端

提供以下 REST API:
  GET /api/library                - 获取完整媒体库（电影 + 电视剧）
  GET /api/library/movies         - 只获取电影列表
  GET /api/library/tv             - 只获取电视剧列表
  POST /api/library/refresh       - 重新扫描 Drive 并更新快照
  POST /api/library/refresh-item  - 刷新单个媒体项的 NFO 和封面到 Drive
  GET /api/tv/{tmdb_id}           - 获取单部剧集详情（含季集入库状态）
  GET /api/stats                  - 获取统计信息
  GET /api/cache/stats            - 查看 TMDB 缓存使用情况
  POST /api/cache/evict           - 手动清理过期缓存

运行方式:
  conda run -n myself uvicorn webui.api:app --host 0.0.0.0 --port 38765 --reload
  # 或在项目根目录:
  conda run -n myself python -m uvicorn webui.api:app --host 0.0.0.0 --port 38765 --reload
"""

import logging
import os
import re
import subprocess
import sys
import threading
import base64
import io
import json
import uuid
import sqlite3
from dataclasses import asdict
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import unquote

import hmac
import secrets
import yaml
import requests
from datetime import datetime, timezone, timedelta
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry
from fastapi import FastAPI, HTTPException, Query, Request
from fastapi.concurrency import run_in_threadpool
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse, Response
from fastapi.staticfiles import StaticFiles
from google.oauth2.credentials import Credentials
from pydantic import BaseModel
try:
    import jwt as _pyjwt
except ImportError:
    _pyjwt = None  # type: ignore  # JWT 功能需要 PyJWT

# 将项目根目录加入路径
_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, _ROOT)

from drive.client import DriveClient
from drive.auth import SCOPES as DRIVE_SCOPES
from storage.base import StorageProvider, CloudFile
from mediaparser import Config, MetaInfo, MetaInfoPath, TmdbClient
from mediaparser.release_group import ReleaseGroupsMatcher
from nfo import NfoGenerator, ImageUploader
import u115pan
from webui.tmdb_cache import TmdbCache
from webui.library_store import get_library_store
from webui.log_store import LogStore
from webui.u115_auto_organize_store import U115AutoOrganizeStore

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("webui")

# ──────────────────────────────────────────────────────────────
# 全局初始化（懒加载）
# ──────────────────────────────────────────────────────────────

_cfg: Optional[Config] = None
_client: Optional[DriveClient] = None
_storage_provider: Optional[StorageProvider] = None
_tmdb_cache: Optional[TmdbCache] = None
_config_mtime_ns: Optional[int] = None
_u115_auto_organize_store: Optional[U115AutoOrganizeStore] = None
_u115_auto_organize_thread: Optional[threading.Thread] = None
_u115_auto_organize_stop = threading.Event()
_u115_auto_organize_last_polled_at: Optional[str] = None
_u115_auto_organize_last_poll_error: Optional[str] = None
_u115_auto_organize_last_triggered_task: Optional[Dict[str, Any]] = None
TMDB_IMG_BASE = "https://image.tmdb.org/t/p/w500"
TMDB_IMG_ORIG = "https://image.tmdb.org/t/p/original"

_ROOT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_CACHE_DB = os.path.join(_ROOT_DIR, "config", "data", "tmdb_cache.db")
_APP_LOG_DIR = os.path.join(_ROOT_DIR, "config", "data", "logs")
_LEGACY_APP_LOG_DB = os.path.join(_ROOT_DIR, "config", "data", "app_logs.jsonl")
_JWT_SECRET_FILE = os.path.join(_ROOT_DIR, "config", "data", ".jwt_secret")
_U115_AUTO_ORGANIZE_DB = os.path.join(_ROOT_DIR, "config", "data", "u115_auto_organize.db")
_jwt_secret_cache: Optional[str] = None
_log_store = LogStore(_APP_LOG_DIR, retention_days=7, legacy_path=_LEGACY_APP_LOG_DB)
_ARIA2_TASK_KEYS = [
    "gid", "status", "totalLength", "completedLength", "uploadLength",
    "downloadSpeed", "uploadSpeed", "connections", "numSeeders", "seeder",
    "errorCode", "errorMessage", "dir", "files", "bittorrent",
]
_ARIA2_GLOBAL_OPTION_KEYS = [
    "dir", "max-concurrent-downloads", "max-overall-download-limit",
    "max-overall-upload-limit", "split", "max-connection-per-server",
    "min-split-size", "continue", "max-tries", "retry-wait",
    "user-agent", "all-proxy", "seed-ratio", "seed-time", "bt-max-peers",
]


def _app_log(
    category: str,
    event: str,
    message: str,
    *,
    level: str = "INFO",
    details: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    _log_store.set_retention_days(get_config().webui.log_retention_days)
    return _log_store.write(
        category=category,
        event=event,
        level=level,
        message=message,
        details=details,
    )


def _infer_pipeline_log_level(line: str) -> str:
    text = line.lower()
    if "❌" in line or "失败" in line or "异常" in line or "error" in text:
        return "ERROR"
    if "✓" in line or "完成" in line or "已上传" in line or "移动：" in line:
        return "SUCCESS"
    if "⚠" in line or "warning" in text or "跳过" in line:
        return "WARNING"
    return "INFO"


def _get_u115_auto_organize_store() -> U115AutoOrganizeStore:
    global _u115_auto_organize_store
    if _u115_auto_organize_store is None:
        _u115_auto_organize_store = U115AutoOrganizeStore(_U115_AUTO_ORGANIZE_DB)
    return _u115_auto_organize_store


def _u115_task_key(task: u115pan.OfflineTask) -> str:
    if task.info_hash:
        return f"info_hash:{task.info_hash}"
    if task.file_id:
        return f"file_id:{task.file_id}"
    return f"name:{task.name}|add:{task.add_time or 0}|size:{task.size}"


def _poll_u115_auto_organize_once() -> None:
    global _u115_auto_organize_last_polled_at, _u115_auto_organize_last_poll_error, _u115_auto_organize_last_triggered_task
    cfg = get_config()
    _u115_auto_organize_last_polled_at = datetime.now(timezone.utc).isoformat()
    _u115_auto_organize_last_poll_error = None
    if cfg.storage.primary != "pan115":
        return
    if not cfg.u115.auto_organize_enabled:
        return
    if not cfg.u115.download_folder_id:
        return

    offline = _u115_offline_client()
    tasks = offline.get_all_tasks()
    stable_seconds = max(0, cfg.u115.auto_organize_stable_seconds)
    now = datetime.now(timezone.utc)
    store = _get_u115_auto_organize_store()
    completed_to_trigger: list[u115pan.OfflineTask] = []

    for task in tasks:
        if task.wp_path_id and str(task.wp_path_id) != str(cfg.u115.download_folder_id):
            continue

        state = store.upsert_observation(
            task_key=_u115_task_key(task),
            info_hash=task.info_hash,
            task_name=task.name,
            status=task.status,
            percent_done=task.percent_done,
            wp_path_id=task.wp_path_id,
            file_id=task.file_id,
            is_finished=task.is_finished,
        )
        if not task.is_finished or state.triggered_at or not state.finished_seen_at:
            continue

        finished_seen_at = datetime.fromisoformat(state.finished_seen_at)
        if (now - finished_seen_at).total_seconds() < stable_seconds:
            continue

        completed_to_trigger.append(task)

    if not completed_to_trigger:
        return

    debounce = get_config().telegram.debounce_seconds
    accepted = schedule_pipeline(debounce)
    if not accepted:
        _app_log(
            "u115",
            "auto_organize_deferred",
            "检测到 115 云下载任务完成，但当前整理流程忙碌，等待后续轮询重试",
            level="WARNING",
            details={
                "count": len(completed_to_trigger),
                "tasks": [
                    {
                        "info_hash": task.info_hash,
                        "name": task.name,
                        "file_id": task.file_id,
                        "wp_path_id": task.wp_path_id,
                    }
                    for task in completed_to_trigger
                ],
            },
        )
        return

    for task in completed_to_trigger:
        store.mark_triggered(_u115_task_key(task))
    _u115_auto_organize_last_triggered_task = {
        "triggered_at": datetime.now(timezone.utc).isoformat(),
        "count": len(completed_to_trigger),
        "tasks": [
            {
                "info_hash": task.info_hash,
                "name": task.name,
                "file_id": task.file_id,
                "wp_path_id": task.wp_path_id,
            }
            for task in completed_to_trigger
        ],
    }
    _app_log(
        "u115",
        "auto_organize_triggered",
        f"检测到 {len(completed_to_trigger)} 个 115 云下载任务完成，已触发自动整理",
        level="SUCCESS",
        details={
            "count": len(completed_to_trigger),
            "debounceSeconds": debounce,
            "tasks": [
                {
                    "info_hash": task.info_hash,
                    "name": task.name,
                    "file_id": task.file_id,
                    "wp_path_id": task.wp_path_id,
                }
                for task in completed_to_trigger
            ],
        },
    )


def _u115_auto_organize_loop() -> None:
    global _u115_auto_organize_last_poll_error
    logger.info("115 自动整理监听线程已启动")
    while not _u115_auto_organize_stop.is_set():
        sleep_seconds = 45
        try:
            cfg = get_config()
            sleep_seconds = max(10, cfg.u115.auto_organize_poll_seconds)
            _poll_u115_auto_organize_once()
        except (sqlite3.Error, ValueError) as exc:
            logger.warning("115 自动整理状态处理失败：%s", exc)
            _u115_auto_organize_last_poll_error = str(exc)
        except Exception as exc:
            logger.warning("115 自动整理轮询失败：%s", exc)
            _u115_auto_organize_last_poll_error = str(exc)
            _app_log(
                "u115",
                "auto_organize_poll_failed",
                "115 自动整理轮询失败",
                level="ERROR",
                details={"error": str(exc)},
            )
        _u115_auto_organize_stop.wait(timeout=sleep_seconds)
    logger.info("115 自动整理监听线程已停止")


def _ensure_u115_auto_organize_thread() -> None:
    global _u115_auto_organize_thread
    if _u115_auto_organize_thread and _u115_auto_organize_thread.is_alive():
        return
    _u115_auto_organize_stop.clear()
    thread = threading.Thread(
        target=_u115_auto_organize_loop,
        name="u115-auto-organize",
        daemon=True,
    )
    thread.start()
    _u115_auto_organize_thread = thread


def _u115_auto_organize_status_payload() -> Dict[str, Any]:
    cfg = get_config()
    store = _get_u115_auto_organize_store()
    last_triggered = store.get_last_triggered()
    last_triggered_payload = _u115_auto_organize_last_triggered_task
    if last_triggered and not last_triggered_payload:
        last_triggered_payload = {
            "triggered_at": last_triggered.triggered_at,
            "count": 1,
            "tasks": [
                {
                    "info_hash": last_triggered.info_hash,
                    "name": last_triggered.task_name,
                    "file_id": last_triggered.file_id,
                    "wp_path_id": last_triggered.wp_path_id,
                }
            ],
        }
    return {
        "enabled": bool(cfg.u115.auto_organize_enabled and cfg.storage.primary == "pan115"),
        "storage_primary": cfg.storage.primary,
        "watcher_alive": bool(_u115_auto_organize_thread and _u115_auto_organize_thread.is_alive()),
        "poll_seconds": cfg.u115.auto_organize_poll_seconds,
        "stable_seconds": cfg.u115.auto_organize_stable_seconds,
        "download_folder_id": cfg.u115.download_folder_id,
        "last_polled_at": _u115_auto_organize_last_polled_at,
        "last_poll_error": _u115_auto_organize_last_poll_error,
        "last_triggered": last_triggered_payload,
    }


# ──────────────────────────────────────────────────────────────
# JWT 工具
# ──────────────────────────────────────────────────────────────

def _get_jwt_secret() -> str:
    """JWT 密钥：配置文件 > config/data/.jwt_secret > 自动生成并持久化"""
    global _jwt_secret_cache
    if _jwt_secret_cache:
        return _jwt_secret_cache
    cfg = get_config()  # 延迟引用，避免循环导入
    if cfg.webui.secret_key:
        _jwt_secret_cache = cfg.webui.secret_key
        return _jwt_secret_cache
    # 尝试合渐加载已存在的自动生成密钥
    try:
        if os.path.exists(_JWT_SECRET_FILE):
            _jwt_secret_cache = open(_JWT_SECRET_FILE).read().strip()
            return _jwt_secret_cache
    except Exception:
        pass
    # 生成新密钥并持久化
    _jwt_secret_cache = secrets.token_hex(32)
    try:
        os.makedirs(os.path.dirname(_JWT_SECRET_FILE), exist_ok=True)
        with open(_JWT_SECRET_FILE, "w") as f:
            f.write(_jwt_secret_cache)
    except Exception as e:
        logger.warning("无法持久化 JWT 密钥：%s", e)
    return _jwt_secret_cache


def _create_token(username: str, expire_hours: int) -> str:
    if _pyjwt is None:
        raise RuntimeError("请先 pip install PyJWT")
    payload = {
        "sub": username,
        "iat": datetime.now(timezone.utc),
        "exp": datetime.now(timezone.utc) + timedelta(hours=expire_hours),
    }
    return _pyjwt.encode(payload, _get_jwt_secret(), algorithm="HS256")


def _verify_token(token: str) -> Optional[str]:
    """Return username if valid, None otherwise."""
    if _pyjwt is None:
        return None
    try:
        payload = _pyjwt.decode(token, _get_jwt_secret(), algorithms=["HS256"])
        return payload.get("sub")
    except Exception:
        return None


# ──────────────────────────────────────────────────────────────

# ──────────────────────────────────────────────────────────────
# Pipeline 触发（并入 server.py 功能）
# ──────────────────────────────────────────────────────────────

_pl_lock = threading.Lock()
_debounce_timer: threading.Timer | None = None
_pipeline_running = False


def send_telegram(token: str, chat_id: str, text: str) -> None:
    """发送 Telegram 消息，token/chat_id 为空时静默跳过。"""
    if not token or not chat_id:
        return
    try:
        requests.post(
            f"https://api.telegram.org/bot{token}/sendMessage",
            json={"chat_id": chat_id, "text": text, "parse_mode": "HTML"},
            timeout=10,
        )
    except Exception as exc:
        logger.warning("Telegram 通知发送失败：%s", exc)


def _do_refresh_library() -> dict:
    """同步刷新快照核心逻辑，可在线程或协程中调用。"""
    client   = get_storage_provider()
    cfg      = get_config()
    _app_log(
        "library",
        "refresh_scan_start",
        "开始刷新媒体库快照",
        details={
            "provider": getattr(client, "provider_name", "unknown"),
            "movie_root_id": cfg.active_movie_root_id(),
            "tv_root_id": cfg.active_tv_root_id(),
            "root_folder_id": cfg.active_root_folder_id(),
        },
    )
    movies   = scan_movies(client, cfg)
    tv_shows = scan_tv_shows(client, cfg)
    diff = get_library_store().save_snapshot(movies, tv_shows)
    _app_log(
        "library",
        "refresh_scan_finish",
        "媒体库快照刷新完成",
        level="SUCCESS",
        details=diff,
    )
    return diff


def _do_run_pipeline() -> None:
    """防抖超时后真正执行 pipeline（在独立线程中调用）。"""
    global _pipeline_running, _debounce_timer
    cfg_obj    = Config.load()
    tg_token   = cfg_obj.telegram.bot_token
    tg_chat_id = cfg_obj.telegram.chat_id
    run_id = uuid.uuid4().hex[:12]

    with _pl_lock:
        _debounce_timer  = None
        _pipeline_running = True

    _app_log("pipeline", "pipeline_start", "整理流程启动", details={"runId": run_id})
    try:
        env = os.environ.copy()
        env["PYTHONIOENCODING"] = "utf-8"
        process = subprocess.Popen(
            [sys.executable, "pipeline.py"],
            cwd=_ROOT_DIR,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            encoding="utf-8",
            errors="replace",
            env=env,
        )
        assert process.stdout is not None
        for line in process.stdout:
            line = line.rstrip()
            if not line:
                continue
            _app_log(
                "pipeline",
                "pipeline_output",
                line,
                level=_infer_pipeline_log_level(line),
                details={"runId": run_id},
            )

        returncode = process.wait()
        if returncode == 0:
            _app_log(
                "pipeline",
                "pipeline_finish",
                "整理流程完成",
                level="SUCCESS",
                details={"runId": run_id, "returncode": returncode},
            )
            try:
                _do_refresh_library()
            except Exception as re_exc:
                logger.warning("媒体库刷新异常：%s", re_exc)
                _app_log(
                    "pipeline",
                    "pipeline_refresh_failed",
                    "整理完成后刷新媒体库失败",
                    level="ERROR",
                    details={"runId": run_id, "error": str(re_exc)},
                )
        else:
            logger.error("Pipeline 退出码 %d", returncode)
            _app_log(
                "pipeline",
                "pipeline_finish",
                "整理流程失败退出",
                level="ERROR",
                details={"runId": run_id, "returncode": returncode},
            )
            send_telegram(tg_token, tg_chat_id,
                          f"❌ <b>Meta2Cloud</b>\n整理失败，退出码：<code>{returncode}</code>")
    except Exception as exc:
        logger.error("Pipeline 异常：%s", exc)
        _app_log(
            "pipeline",
            "pipeline_exception",
            "整理流程执行异常",
            level="ERROR",
            details={"runId": run_id, "error": str(exc)},
        )
        send_telegram(tg_token, tg_chat_id,
                      f"❌ <b>Meta2Cloud</b>\n异常：<code>{exc}</code>")
    finally:
        with _pl_lock:
            _pipeline_running = False


def schedule_pipeline(debounce: int) -> bool:
    """安排 pipeline 运行。debounce>0 则防抖，=0 则立即启动新线程。返回是否接受本次调度。"""
    global _debounce_timer
    with _pl_lock:
        if debounce > 0:
            if _debounce_timer is not None:
                _debounce_timer.cancel()
                logger.info("整理流程防抖计时器已重置 | 防抖：%d 秒", debounce)
            else:
                _app_log(
                    "pipeline",
                    "pipeline_schedule",
                    "整理流程已进入防抖等待",
                    details={"debounceSeconds": debounce},
                )
            t = threading.Timer(debounce, _do_run_pipeline)
            t.daemon = True
            t.start()
            _debounce_timer = t
            return True
        else:
            if _pipeline_running:
                _app_log("pipeline", "pipeline_skip_running", "整理流程已在运行，跳过本次触发", level="WARNING")
                return False
            _app_log("pipeline", "pipeline_schedule", "整理流程立即执行", details={"debounceSeconds": 0})
            threading.Thread(target=_do_run_pipeline, daemon=True).start()
            return True



def get_tmdb_cache() -> TmdbCache:
    global _tmdb_cache
    if _tmdb_cache is None:
        _tmdb_cache = TmdbCache(_CACHE_DB)
        _tmdb_cache.evict_expired()  # 启动时清一次过期条目
    return _tmdb_cache


def _invalidate_runtime_cache_if_config_changed() -> None:
    """配置文件被外部修改时，主动失效运行时缓存。"""
    global _cfg, _client, _storage_provider, _config_mtime_ns
    try:
        mtime_ns = _CONFIG_PATH.stat().st_mtime_ns
    except FileNotFoundError:
        mtime_ns = None

    if _config_mtime_ns is None:
        _config_mtime_ns = mtime_ns
        return

    if mtime_ns != _config_mtime_ns:
        logger.info("检测到 config.yaml 已变更，重载配置与存储 Provider 缓存")
        _cfg = None
        _client = None
        _storage_provider = None
        _config_mtime_ns = mtime_ns


def get_config() -> Config:
    global _cfg, _config_mtime_ns
    _invalidate_runtime_cache_if_config_changed()
    if _cfg is None:
        _cfg = Config.load()
        try:
            _config_mtime_ns = _CONFIG_PATH.stat().st_mtime_ns
        except FileNotFoundError:
            _config_mtime_ns = None
    return _cfg


def get_drive_client() -> DriveClient:
    global _client
    _invalidate_runtime_cache_if_config_changed()
    if _client is None:
        cfg = get_config()
        drive_cfg = cfg.drive
        _client = DriveClient.from_oauth(
            credentials_path=drive_cfg.credentials_json,
            token_path=drive_cfg.token_json,
        )
    return _client


def get_storage_provider() -> StorageProvider:
    """获取统一的 StorageProvider 实例（根据 config.storage.primary 选择）"""
    global _storage_provider
    _invalidate_runtime_cache_if_config_changed()
    if _storage_provider is None:
        cfg = get_config()
        from storage import get_provider
        _storage_provider = get_provider(cfg.storage.primary, cfg)
    return _storage_provider


def _get_release_group_matcher() -> ReleaseGroupsMatcher:
    return ReleaseGroupsMatcher(get_config().parser.custom_release_groups)


def _serialize_meta(meta: Any) -> Dict[str, Any]:
    payload = asdict(meta)
    payload["type"] = meta.type.to_agent() if getattr(meta, "type", None) else ""
    payload["type_label"] = meta.type.value if getattr(meta, "type", None) else ""
    payload["name"] = meta.name
    payload["season"] = meta.season
    payload["episode"] = meta.episode
    payload["season_episode"] = meta.season_episode
    payload["season_list"] = meta.season_list
    payload["episode_list"] = meta.episode_list
    payload["resource_term"] = meta.resource_term
    payload["edition"] = meta.edition
    payload["release_group"] = meta.release_group
    payload["video_term"] = meta.video_term
    payload["audio_term"] = meta.audio_term
    payload["frame_rate"] = meta.frame_rate
    return payload


def _extract_alternative_names_with_lang(info: Dict[str, Any]) -> List[Dict[str, str]]:
    """从 TMDB 原始数据提取带语言标记的别名列表，格式为 [{"name": .., "iso_639_1": ..}]。
    优先收录 alternative_titles，再收录 translations，均携带语言/地区码。
    """
    seen: set = set()
    result: List[Dict[str, str]] = []

    # 从 alternative_titles 提取（有 iso_3166_1 地区码）
    alt_titles_root = info.get("alternative_titles") or {}
    alt_titles = alt_titles_root.get("titles") or alt_titles_root.get("results") or []
    for t in alt_titles:
        name = t.get("title") or ""
        iso_3166 = t.get("iso_3166_1") or ""
        # 简单地区码 → 语言映射：CN/TW/HK/SG → zh，JP → ja
        lang_map = {"CN": "zh", "TW": "zh", "HK": "zh", "SG": "zh", "JP": "ja", "US": "en", "GB": "en"}
        iso_639 = lang_map.get(iso_3166.upper(), iso_3166.lower()[:2] if iso_3166 else "")
        if name and name not in seen:
            seen.add(name)
            result.append({"name": name, "iso_639_1": iso_639})

    # 从 translations 提取（有标准 iso_639_1 语言码）
    for tr in (info.get("translations") or {}).get("translations") or []:
        iso_639 = tr.get("iso_639_1") or ""
        data = tr.get("data") or {}
        name = data.get("title") or data.get("name") or ""
        if name and name not in seen:
            seen.add(name)
            result.append({"name": name, "iso_639_1": iso_639})

    return result


def _serialize_tmdb_result(info: Optional[Dict[str, Any]]) -> Optional[Dict[str, Any]]:
    if not info:
        return None

    media_type = info.get("media_type")
    title = info.get("title") or info.get("name") or ""
    original_title = info.get("original_title") or info.get("original_name") or ""
    release_date = info.get("release_date") or info.get("first_air_date") or ""

    res = {
        "tmdb_id": info.get("tmdb_id") or info.get("id"),
        "media_type": media_type.to_agent() if hasattr(media_type, "to_agent") else str(media_type or ""),
        "media_type_label": media_type.value if hasattr(media_type, "value") else "",
        "title": title,
        "original_title": original_title,
        "year": release_date[:4] if release_date else "",
        "release_date": release_date,
        "overview": info.get("overview") or "",
        "rating": round(info.get("vote_average") or 0, 1),
        "poster_url": TmdbClient.image_url(info.get("poster_path")),
        "backdrop_url": TmdbClient.image_url(info.get("backdrop_path")),
        "status": info.get("status") or "",
        "genres": [genre.get("name") for genre in (info.get("genres") or []) if genre.get("name")],
        "names": info.get("names") or [],
        "alternative_names": _extract_alternative_names_with_lang(info),
        "directors": [person.get("name") for person in (info.get("directors") or []) if person.get("name")],
        "actors": [person.get("name") for person in (info.get("actors") or []) if person.get("name")][:8],
        "season_count": info.get("number_of_seasons"),
        "episode_count": info.get("number_of_episodes"),
    }

    if "seasons" in info:
        # 伪造虚假的 season 数据（其中只有总数而没有入库）
        seasons_data = []
        for s in info["seasons"]:
            sn = s.get("season_number")
            if sn is None:
                continue
            count = s.get("episode_count", 0)
            episodes = [{"episode_number": i, "in_library": False} for i in range(1, count + 1)]
            seasons_data.append({
                "season_number": sn,
                "season_name": s.get("name") or f"季 {sn}",
                "poster_url": TmdbClient.image_url(s.get("poster_path")),
                "air_date": s.get("air_date"),
                "total_episodes": count,
                "in_library_episodes": 0,
                "episodes": episodes,
            })
        res["seasons"] = seasons_data

    return res


def _aria2_rpc_url() -> str:
    cfg = get_config().aria2
    scheme = "https" if cfg.secure else "http"
    path = cfg.path if cfg.path.startswith("/") else f"/{cfg.path}"
    return f"{scheme}://{cfg.host}:{cfg.port}{path}"


def _ensure_aria2_enabled() -> None:
    if not get_config().aria2.enabled:
        raise HTTPException(status_code=503, detail="Aria2 集成未启用")


def _aria2_rpc_call(method: str, params: Optional[List[Any]] = None) -> Any:
    _ensure_aria2_enabled()
    cfg = get_config().aria2
    rpc_params = list(params or [])
    if cfg.secret:
        rpc_params.insert(0, f"token:{cfg.secret}")

    payload = {
        "jsonrpc": "2.0",
        "id": secrets.token_hex(8),
        "method": f"aria2.{method}",
        "params": rpc_params,
    }

    try:
        resp = _aria2_http.post(_aria2_rpc_url(), json=payload, timeout=5)
        resp.raise_for_status()
        data = resp.json()
    except requests.RequestException as exc:
        logger.warning("aria2 RPC 请求失败：%s", exc)
        _app_log(
            "download",
            "aria2_rpc_error",
            "aria2 无法连接",
            level="ERROR",
            details={"method": method, "error": str(exc)},
        )
        raise HTTPException(status_code=502, detail=f"无法连接 aria2 RPC：{exc}") from exc
    except ValueError as exc:
        logger.warning("aria2 RPC 返回非 JSON：%s", exc)
        _app_log(
            "download",
            "aria2_rpc_invalid_json",
            "aria2 返回了无效数据",
            level="ERROR",
            details={"method": method, "error": str(exc)},
        )
        raise HTTPException(status_code=502, detail="aria2 RPC 返回了无效响应") from exc

    if data.get("error"):
        message = data["error"].get("message") or "aria2 RPC 调用失败"
        code = data["error"].get("code")
        _app_log(
            "download",
            "aria2_rpc_api_error",
            f"aria2 操作失败：{message}",
            level="ERROR",
            details={"method": method, "message": message, "code": code},
        )
        raise HTTPException(status_code=400, detail=f"{message} ({code})" if code else message)

    return data.get("result")


def _aria2_guess_name(task: Dict[str, Any]) -> str:
    info = task.get("bittorrent", {}).get("info", {})
    torrent_name = info.get("name")
    if torrent_name:
        return torrent_name

    files = task.get("files") or []
    first = files[0] if files else {}
    path = first.get("path") or ""
    if path:
        task_dir = task.get("dir") or ""
        if task_dir:
            try:
                rel_path = os.path.relpath(path, task_dir)
                first_segment = rel_path.split(os.sep, 1)[0]
                if first_segment and first_segment not in {".", ".."}:
                    return first_segment
            except ValueError:
                pass
        return os.path.basename(path)

    uris = first.get("uris") or []
    uri = next((u.get("uri") for u in uris if u.get("uri")), "")
    if uri:
        parsed = urlparse(uri)
        guess = os.path.basename(parsed.path)
        if guess:
            return unquote(guess)
        return parsed.netloc or uri

    return info.get("name") or task.get("gid") or "Unnamed Task"


def _aria2_progress(task: Dict[str, Any]) -> float:
    total = int(task.get("totalLength") or 0)
    completed = int(task.get("completedLength") or 0)
    if total <= 0:
        return 0.0
    return round(completed * 100 / total, 1)


def _aria2_file_count(task: Dict[str, Any]) -> int:
    return len(task.get("files") or [])


def _aria2_normalize_task(task: Dict[str, Any]) -> Dict[str, Any]:
    total = int(task.get("totalLength") or 0)
    completed = int(task.get("completedLength") or 0)
    upload = int(task.get("uploadLength") or 0)
    first_file = (task.get("files") or [{}])[0]
    uris = first_file.get("uris") or []
    bittorrent = task.get("bittorrent") or {}
    info = bittorrent.get("info") or {}

    return {
        "gid": task.get("gid"),
        "status": task.get("status"),
        "name": _aria2_guess_name(task),
        "dir": task.get("dir") or "",
        "progress": _aria2_progress(task),
        "totalLength": total,
        "completedLength": completed,
        "uploadLength": upload,
        "downloadSpeed": int(task.get("downloadSpeed") or 0),
        "uploadSpeed": int(task.get("uploadSpeed") or 0),
        "connections": int(task.get("connections") or 0),
        "numSeeders": int(task.get("numSeeders") or 0),
        "seeder": task.get("seeder") == "true",
        "errorCode": task.get("errorCode") or "",
        "errorMessage": task.get("errorMessage") or "",
        "fileCount": _aria2_file_count(task),
        "uris": [u.get("uri") for u in uris if u.get("uri")],
        "bittorrent": {
            "name": info.get("name") or "",
            "comment": bittorrent.get("comment") or "",
            "mode": bittorrent.get("mode") or "",
        },
        "files": [
            {
                "path": f.get("path") or "",
                "length": int(f.get("length") or 0),
                "completedLength": int(f.get("completedLength") or 0),
                "selected": f.get("selected") != "false",
                "uris": [u.get("uri") for u in (f.get("uris") or []) if u.get("uri")],
            }
            for f in (task.get("files") or [])
        ],
    }


def _pagination_payload(*, page: int, page_size: int, total: int) -> Dict[str, Any]:
    total_pages = max((total + page_size - 1) // page_size, 1)
    page = max(1, min(page, total_pages))
    return {
        "page": page,
        "page_size": page_size,
        "total": total,
        "total_pages": total_pages,
        "has_prev": page > 1,
        "has_next": page < total_pages,
    }


def _aria2_flush_metadata_results(gids: List[str]) -> None:
    if not gids:
        return
    try:
        for gid in gids:
            _aria2_rpc_call("removeDownloadResult", [gid])
        logger.info("自动清理了 %d 个已完成的 [METADATA] 或 .torrent 任务", len(gids))
        _app_log(
            "download",
            "metadata_purged",
            f"已自动从队列中横扫清理并移除了 {len(gids)} 个已完成的种子/元数据任务",
            level="INFO",
            details={"count": len(gids), "gids": gids},
        )
    except Exception as e:
        logger.warning("自动清理种子/元数据任务失败：%s", e)
        _app_log(
            "download",
            "metadata_purge_failed",
            f"尝试清理已完成的种子/元数据任务失败: {e}",
            level="WARNING",
            details={"error": str(e)},
        )


def _aria2_normalize_stopped(tasks: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    normalized_stopped = []
    metadata_gids_to_remove = []
    for task in tasks:
        norm_task = _aria2_normalize_task(task)
        name = norm_task.get("name", "")
        if norm_task.get("status") == "complete" and (
            "[METADATA]" in name or bool(re.match(r"^[0-9a-fA-F]{40}\.torrent$", name))
        ):
            metadata_gids_to_remove.append(norm_task["gid"])
            continue
        normalized_stopped.append(norm_task)
    _aria2_flush_metadata_results(metadata_gids_to_remove)
    return normalized_stopped


def _aria2_fetch_waiting_slice(start: int, count: int, total: int) -> List[Dict[str, Any]]:
    if count <= 0 or start >= total:
        return []
    raw_offset = max(total - (start + count), 0)
    raw_limit = min(count, total - start)
    waiting = _aria2_rpc_call("tellWaiting", [raw_offset, raw_limit, _ARIA2_TASK_KEYS]) or []
    return [_aria2_normalize_task(task) for task in waiting][::-1]


def _aria2_fetch_stopped_slice(start: int, count: int) -> List[Dict[str, Any]]:
    if count <= 0:
        return []
    stopped = _aria2_rpc_call("tellStopped", [start, count, _ARIA2_TASK_KEYS]) or []
    return _aria2_normalize_stopped(stopped)


def _aria2_task_matches(task: Dict[str, Any], search: str) -> bool:
    if not search:
        return True
    haystacks = [
        task.get("gid") or "",
        task.get("name") or "",
        task.get("dir") or "",
        task.get("errorMessage") or "",
    ]
    for file_item in task.get("files") or []:
        haystacks.append(file_item.get("path") or "")
        for uri in file_item.get("uris") or []:
            haystacks.append(uri or "")
    return any(search in str(value).lower() for value in haystacks if value)


def _aria2_fetch_all_filtered(queue: str, search: str) -> Dict[str, Any]:
    global_stat = _aria2_rpc_call("getGlobalStat") or {}
    version = _aria2_rpc_call("getVersion") or {}
    waiting_total = int(global_stat.get("numWaiting") or 0)
    stopped_total = int(global_stat.get("numStopped") or 0)

    active = [_aria2_normalize_task(task) for task in (_aria2_rpc_call("tellActive", [_ARIA2_TASK_KEYS]) or [])][::-1]
    waiting = _aria2_fetch_waiting_slice(0, waiting_total, waiting_total)
    stopped = _aria2_fetch_stopped_slice(0, stopped_total)

    search = search.strip().lower()
    filtered = {
        "active": [task for task in active if _aria2_task_matches(task, search)],
        "waiting": [task for task in waiting if _aria2_task_matches(task, search)],
        "stopped": [task for task in stopped if _aria2_task_matches(task, search)],
    }

    if queue == "active":
        items = filtered["active"]
    elif queue == "waiting":
        items = filtered["waiting"]
    elif queue == "stopped":
        items = filtered["stopped"]
    else:
        items = filtered["active"] + filtered["waiting"] + filtered["stopped"]

    return {
        "summary": {
            "activeCount": len(filtered["active"]),
            "waitingCount": len(filtered["waiting"]),
            "stoppedCount": len(filtered["stopped"]),
            "downloadSpeed": int(global_stat.get("downloadSpeed") or 0),
            "uploadSpeed": int(global_stat.get("uploadSpeed") or 0),
            "numActive": int(global_stat.get("numActive") or 0),
            "numWaiting": waiting_total,
            "numStopped": stopped_total,
        },
        "items": items,
        "version": {
            "version": version.get("version") or "",
            "enabledFeatures": version.get("enabledFeatures") or [],
        },
    }


def _aria2_fetch_queue_items(queue: str, page: int, page_size: int, search: str = "") -> Dict[str, Any]:
    global_stat = _aria2_rpc_call("getGlobalStat") or {}
    version = _aria2_rpc_call("getVersion") or {}

    active = [_aria2_normalize_task(task) for task in (_aria2_rpc_call("tellActive", [_ARIA2_TASK_KEYS]) or [])][::-1]
    active_count = len(active)
    waiting_count = int(global_stat.get("numWaiting") or 0)
    stopped_count = int(global_stat.get("numStopped") or 0)

    queue = queue if queue in {"all", "active", "waiting", "stopped"} else "all"
    search = search.strip()
    if search:
        filtered_payload = _aria2_fetch_all_filtered(queue, search)
        pagination = _pagination_payload(page=page, page_size=page_size, total=len(filtered_payload["items"]))
        offset = (pagination["page"] - 1) * page_size
        return {
            "summary": filtered_payload["summary"],
            "items": filtered_payload["items"][offset: offset + page_size],
            "pagination": {
                **pagination,
                "queue": queue,
                "search": search,
            },
            "version": filtered_payload["version"],
        }

    if queue == "active":
        total = active_count
    elif queue == "waiting":
        total = waiting_count
    elif queue == "stopped":
        total = stopped_count
    else:
        total = active_count + waiting_count + stopped_count

    pagination = _pagination_payload(page=page, page_size=page_size, total=total)
    offset = (pagination["page"] - 1) * page_size
    items: List[Dict[str, Any]] = []

    if queue == "active":
        items = active[offset: offset + page_size]
    elif queue == "waiting":
        items = _aria2_fetch_waiting_slice(offset, page_size, waiting_count)
    elif queue == "stopped":
        items = _aria2_fetch_stopped_slice(offset, page_size)
    else:
        remaining = page_size
        current_offset = offset

        if current_offset < active_count and remaining > 0:
            take = min(remaining, active_count - current_offset)
            items.extend(active[current_offset: current_offset + take])
            remaining -= take
            current_offset = 0
        else:
            current_offset = max(current_offset - active_count, 0)

        if remaining > 0:
            if current_offset < waiting_count:
                take = min(remaining, waiting_count - current_offset)
                items.extend(_aria2_fetch_waiting_slice(current_offset, take, waiting_count))
                remaining -= take
                current_offset = 0
            else:
                current_offset = max(current_offset - waiting_count, 0)

        if remaining > 0:
            items.extend(_aria2_fetch_stopped_slice(current_offset, remaining))

    return {
        "summary": {
            "activeCount": active_count,
            "waitingCount": waiting_count,
            "stoppedCount": stopped_count,
            "downloadSpeed": int(global_stat.get("downloadSpeed") or 0),
            "uploadSpeed": int(global_stat.get("uploadSpeed") or 0),
            "numActive": int(global_stat.get("numActive") or 0),
            "numWaiting": waiting_count,
            "numStopped": stopped_count,
        },
        "items": items,
        "pagination": {
            **pagination,
            "queue": queue,
        },
        "version": {
            "version": version.get("version") or "",
            "enabledFeatures": version.get("enabledFeatures") or [],
        },
    }


# requests Session：最多重试 3 次，仅对网络级错误（SSL/连接超时）自动退避
_RETRY = Retry(
    total=3,
    backoff_factor=1.0,          # 1s → 2s → 4s
    status_forcelist=[429, 500, 502, 503, 504],
    allowed_methods=["GET"],
    raise_on_status=False,
)
_http = requests.Session()
_http.mount("https://", HTTPAdapter(max_retries=_RETRY))
_http.mount("http://",  HTTPAdapter(max_retries=_RETRY))
# aria2 RPC：不重试（失败立即返回错误，避免阻塞事件循环）
_ARIA2_NO_RETRY = Retry(total=0, raise_on_status=False)
_aria2_http = requests.Session()
_aria2_http.trust_env = False
_aria2_http.mount("https://", HTTPAdapter(max_retries=_ARIA2_NO_RETRY))
_aria2_http.mount("http://",  HTTPAdapter(max_retries=_ARIA2_NO_RETRY))


def tmdb_get(path: str, extra: Optional[dict] = None) -> Optional[dict]:
    """调用 TMDB API：优先读 SQLite 缓存，失败冷却期内跳过，否则带重试请求"""
    cfg = get_config()
    language = cfg.tmdb.language
    cache = get_tmdb_cache()

    # 构建缓存 key（路径 + 额外参数，不含 api_key）
    cache_path = path
    if extra:
        suffix = "&".join(f"{k}={v}" for k, v in sorted(extra.items()) if k != "api_key")
        if suffix:
            cache_path = f"{path}?{suffix}"

    # 1. 正常缓存命中
    cached = cache.get(cache_path, language=language)
    if cached is not None:
        logger.debug("缓存命中：%s", cache_path)
        return cached

    # 2. 负缓存：在冷却期内，直接跳过（不请求 TMDB）
    if cache.is_failed(cache_path, language=language):
        logger.debug("冷却中，跳过：%s", cache_path)
        return None

    # 3. 真正请求 TMDB（Session 内置重试）
    params = {"api_key": cfg.tmdb.api_key, "language": language}
    if extra:
        params.update(extra)
    proxy = cfg.tmdb_proxy
    proxies = {"http": proxy, "https": proxy} if proxy else None
    try:
        resp = _http.get(
            f"https://api.themoviedb.org/3{path}",
            params=params,
            timeout=cfg.tmdb.timeout,
            proxies=proxies,
        )
        if resp.status_code == 404:
            return None
        resp.raise_for_status()
        data = resp.json()
        cache.set(cache_path, data, language=language)
        logger.debug("缓存写入：%s", cache_path)
        return data
    except Exception as e:
        logger.warning("TMDB 请求失败（将冷却 5 分钟）%s: %s", path, e)
        cache.set_failed(cache_path, language=language)   # 写入负缓存
        return None


# ──────────────────────────────────────────────────────────────
# 数据模型
# ──────────────────────────────────────────────────────────────

class EpisodeStatus(BaseModel):
    episode_number: int
    episode_title: str
    air_date: str
    in_library: bool  # 是否已在 Drive 入库


class SeasonStatus(BaseModel):
    season_number: int
    season_name: str
    poster_url: Optional[str]
    episode_count: int            # TMDB 总集数
    in_library_count: int         # Drive 已入库集数
    episodes: List[EpisodeStatus]


class MediaItem(BaseModel):
    tmdb_id: int
    title: str
    original_title: str
    year: str
    media_type: str               # "movie" | "tv"
    poster_url: Optional[str]
    backdrop_url: Optional[str]
    overview: str
    rating: float
    # 电视剧特有
    seasons: Optional[List[SeasonStatus]] = None
    total_episodes: Optional[int] = None
    in_library_episodes: Optional[int] = None
    status: Optional[str] = None  # TMDB 播出状态
    # Drive 信息
    drive_folder_id: Optional[str] = None


class LibraryResponse(BaseModel):
    movies: List[MediaItem]
    tv_shows: List[MediaItem]
    total_movies: int
    total_tv: int
    scanned_at: Optional[str] = None   # 最后扫描时间
    hint: Optional[str] = None         # 未扫描时的提示语


class StatsResponse(BaseModel):
    total_movies: int
    total_tv_shows: int
    total_episodes_in_library: int
    total_episodes_on_tmdb: int
    completion_rate: float


# ──────────────────────────────────────────────────────────────
# 核心逻辑：扫描 Drive 目录 → 解析 → 比对 TMDB
# ──────────────────────────────────────────────────────────────

def _parse_tmdb_id_from_nfo(nfo_content: str) -> Optional[int]:
    """从 NFO/tvshow.nfo XML 内容提取 TMDB ID"""
    m = re.search(r"<tmdbid>(\d+)</tmdbid>", nfo_content, re.IGNORECASE)
    if m:
        return int(m.group(1))
    m = re.search(r"<uniqueid type=\"tmdb\"[^>]*>(\d+)</uniqueid>", nfo_content, re.IGNORECASE)
    if m:
        return int(m.group(1))
    return None


def _parse_episode_from_filename(filename: str) -> Optional[tuple]:
    """从文件名解析 (season, episode)，例如 S01E05 → (1, 5)"""
    m = re.search(r"[Ss](\d+)[Ee](\d+)", filename)
    if m:
        return int(m.group(1)), int(m.group(2))
    return None


def scan_movies(client: StorageProvider, cfg: Config) -> List[MediaItem]:
    """扫描电影目录，返回电影列表"""
    movie_root = cfg.active_movie_root_id()
    if not movie_root:
        return []

    movies = []
    # 列出电影根目录下的所有子文件夹（每个是一部电影）
    movie_folders = client.list_files(folder_id=movie_root, page_size=500)
    movie_folders = [f for f in movie_folders if f.is_folder]

    logger.info("扫描到 %d 个电影文件夹", len(movie_folders))

    for folder in movie_folders:
        # 在电影文件夹内找 NFO 文件
        nfo_files = [
            f for f in client.list_files(folder_id=folder.id, page_size=100)
            if f.name.endswith(".nfo") and f.name != "tvshow.nfo"
        ]
        tmdb_id = None
        tmdb_info = None

        for nfo in nfo_files:
            content = client.read_text(nfo)
            if content:
                tmdb_id = _parse_tmdb_id_from_nfo(content)
                if tmdb_id:
                    break

        if tmdb_id:
            tmdb_info = tmdb_get(f"/movie/{tmdb_id}")

        if tmdb_info:
            movies.append(MediaItem(
                tmdb_id=tmdb_id,
                title=tmdb_info.get("title") or folder.name,
                original_title=tmdb_info.get("original_title") or "",
                year=(tmdb_info.get("release_date") or "")[:4],
                media_type="movie",
                poster_url=f"{TMDB_IMG_BASE}{tmdb_info['poster_path']}" if tmdb_info.get("poster_path") else None,
                backdrop_url=f"{TMDB_IMG_ORIG}{tmdb_info['backdrop_path']}" if tmdb_info.get("backdrop_path") else None,
                overview=tmdb_info.get("overview") or "",
                rating=round(tmdb_info.get("vote_average") or 0, 1),
                drive_folder_id=folder.id,
            ))
        else:
            # 没有 NFO 或 TMDB 信息，用文件夹名估算
            name = folder.name
            year_m = re.search(r"\((\d{4})\)", name)
            year = year_m.group(1) if year_m else ""
            title = re.sub(r"\s*\(\d{4}\)\s*$", "", name).strip()
            movies.append(MediaItem(
                tmdb_id=0,
                title=title,
                original_title="",
                year=year,
                media_type="movie",
                poster_url=None,
                backdrop_url=None,
                overview="",
                rating=0.0,
                drive_folder_id=folder.id,
            ))

    return movies


def scan_tv_shows(client: StorageProvider, cfg: Config) -> List[MediaItem]:
    """扫描剧集目录，返回剧集列表（含季集入库状态）"""
    tv_root = cfg.active_tv_root_id()
    if not tv_root:
        return []

    shows = []
    # 列出剧集根目录下的所有子文件夹（每个是一部剧）
    show_folders = client.list_files(folder_id=tv_root, page_size=500)
    show_folders = [f for f in show_folders if f.is_folder]

    logger.info("扫描到 %d 个剧集文件夹", len(show_folders))

    for show_folder in show_folders:
        show_files = client.list_files(folder_id=show_folder.id, page_size=200)

        # 找 tvshow.nfo
        tvshow_nfo = next((f for f in show_files if f.name == "tvshow.nfo"), None)
        tmdb_id = None

        if tvshow_nfo:
            content = client.read_text(tvshow_nfo)
            if content:
                tmdb_id = _parse_tmdb_id_from_nfo(content)

        # 获取 TMDB 剧集详情
        tmdb_info = None
        if tmdb_id:
            tmdb_info = tmdb_get(f"/tv/{tmdb_id}")

        # 列出 Season 子文件夹
        season_folders = [f for f in show_files if f.is_folder and f.name.startswith("Season")]

        # 建立 Drive 中已有剧集的集合 {(season, episode)}
        drive_episodes: Dict[tuple, str] = {}  # (s, e) → filename
        for season_folder in season_folders:
            season_m = re.search(r"Season\s+(\d+)", season_folder.name, re.IGNORECASE)
            if not season_m:
                continue
            season_num = int(season_m.group(1))
            season_files = client.list_files(folder_id=season_folder.id, page_size=500)
            for f in season_files:
                if f.is_video:
                    ep = _parse_episode_from_filename(f.name)
                    if ep and ep[0] == season_num:
                        drive_episodes[(ep[0], ep[1])] = f.name

        # 构建季集状态
        seasons_status: List[SeasonStatus] = []
        total_eps = 0
        in_lib_eps = 0

        if tmdb_info:
            for season_raw in (tmdb_info.get("seasons") or []):
                s_num = season_raw.get("season_number", 0)
                if s_num == 0:  # 跳过 Specials（Season 0）
                    continue
                ep_count = season_raw.get("episode_count", 0)
                total_eps += ep_count

                # 获取该季详情（含每集信息）
                season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{s_num}")
                episodes_status = []
                if season_detail:
                    for ep_raw in (season_detail.get("episodes") or []):
                        ep_num = ep_raw.get("episode_number", 0)
                        in_lib = (s_num, ep_num) in drive_episodes
                        if in_lib:
                            in_lib_eps += 1
                        episodes_status.append(EpisodeStatus(
                            episode_number=ep_num,
                            episode_title=ep_raw.get("name") or f"第 {ep_num} 集",
                            air_date=ep_raw.get("air_date") or "",
                            in_library=in_lib,
                        ))
                else:
                    # TMDB 季详情获取失败，只统计文件数
                    for ep_num in range(1, ep_count + 1):
                        in_lib = (s_num, ep_num) in drive_episodes
                        if in_lib:
                            in_lib_eps += 1
                        episodes_status.append(EpisodeStatus(
                            episode_number=ep_num,
                            episode_title=f"第 {ep_num} 集",
                            air_date="",
                            in_library=in_lib,
                        ))

                s_in_lib = sum(1 for e in episodes_status if e.in_library)
                seasons_status.append(SeasonStatus(
                    season_number=s_num,
                    season_name=season_raw.get("name") or f"Season {s_num}",
                    poster_url=f"{TMDB_IMG_BASE}{season_raw['poster_path']}" if season_raw.get("poster_path") else None,
                    episode_count=len(episodes_status),
                    in_library_count=s_in_lib,
                    episodes=episodes_status,
                ))
        else:
            # 无 TMDB：只根据 Drive 文件推断
            season_map: Dict[int, List[int]] = {}
            for (s, e) in drive_episodes:
                season_map.setdefault(s, []).append(e)
            for s_num in sorted(season_map):
                eps = sorted(season_map[s_num])
                episodes_status = [
                    EpisodeStatus(
                        episode_number=ep,
                        episode_title=f"第 {ep} 集",
                        air_date="",
                        in_library=True,
                    )
                    for ep in eps
                ]
                in_lib_eps += len(eps)
                total_eps += len(eps)
                seasons_status.append(SeasonStatus(
                    season_number=s_num,
                    season_name=f"Season {s_num}",
                    poster_url=None,
                    episode_count=len(eps),
                    in_library_count=len(eps),
                    episodes=episodes_status,
                ))

        shows.append(MediaItem(
            tmdb_id=tmdb_id or 0,
            title=tmdb_info.get("name") if tmdb_info else re.sub(r"\s*\(\d{4}\)\s*$", "", show_folder.name).strip(),
            original_title=(tmdb_info.get("original_name") or "") if tmdb_info else "",
            year=((tmdb_info.get("first_air_date") or "")[:4]) if tmdb_info else (_m.group(1) if (_m := re.search(r"\((\d{4})\)", show_folder.name)) else ""),
            media_type="tv",
            poster_url=f"{TMDB_IMG_BASE}{tmdb_info['poster_path']}" if tmdb_info and tmdb_info.get("poster_path") else None,
            backdrop_url=f"{TMDB_IMG_ORIG}{tmdb_info['backdrop_path']}" if tmdb_info and tmdb_info.get("backdrop_path") else None,
            overview=(tmdb_info.get("overview") or "") if tmdb_info else "",
            rating=round((tmdb_info.get("vote_average") or 0), 1) if tmdb_info else 0.0,
            seasons=seasons_status,
            total_episodes=total_eps,
            in_library_episodes=in_lib_eps,
            status=(tmdb_info.get("status") or "") if tmdb_info else "",
            drive_folder_id=show_folder.id,
        ))

    return shows


# ──────────────────────────────────────────────────────────────
# FastAPI App
# ──────────────────────────────────────────────────────────────

app = FastAPI(
    title="Meta2Cloud 媒体库",
    description="查看 Google Drive 上的电影/电视剧入库状态",
    version="1.0.0",
)


@app.on_event("startup")
async def startup_background_watchers():
    _ensure_u115_auto_organize_thread()


@app.on_event("shutdown")
async def shutdown_background_watchers():
    _u115_auto_organize_stop.set()
    if _u115_auto_organize_store is not None:
        _u115_auto_organize_store.close()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.middleware("http")
async def auth_middleware(request: Request, call_next):
    """JWT 鉴权中间件：保护所有 /api/* 路由，公共路径除外。"""
    path = request.url.path
    # 放行路径：登录接口、webhook、非-API 路径（静态资源 / 首页）
    if (
        path == "/api/auth/login"
        or path.startswith("/trigger")
        or not path.startswith("/api/")
    ):
        return await call_next(request)

    auth = request.headers.get("Authorization", "")
    if not auth.startswith("Bearer "):
        return JSONResponse({"detail": "未授权，请先登录"}, status_code=401)
    token = auth[7:]
    username = _verify_token(token)
    if not username:
        return JSONResponse({"detail": "Token 已过期或无效，请重新登录"}, status_code=401)
    return await call_next(request)


# 挂载静态文件（前端构建产物：frontend/dist/assets/）
# Vite 默认将 JS/CSS 输出到 dist/assets，index.html 中引用 /assets/xxx
_STATIC_DIR = os.path.join(_ROOT_DIR, "frontend", "dist")
_ASSETS_DIR = os.path.join(_STATIC_DIR, "assets")
if os.path.exists(_ASSETS_DIR):
    app.mount("/assets", StaticFiles(directory=_ASSETS_DIR), name="assets")


# ──────────────────────────────────────────────────────────────
# 认证接口（公共）
# ──────────────────────────────────────────────────────────────

class LoginRequest(BaseModel):
    username: str
    password: str


@app.post("/api/auth/login")
async def auth_login(body: LoginRequest):
    """(公共) 登录接口，返回 JWT token。"""
    cfg = get_config()
    cfg_user = cfg.webui.username or "admin"
    cfg_pass = cfg.webui.password or ""

    # 时序安全比较，防止时序攻击
    user_ok = hmac.compare_digest(body.username.encode(), cfg_user.encode())
    pass_ok = hmac.compare_digest(body.password.encode(), cfg_pass.encode())

    if not (user_ok and pass_ok):
        raise HTTPException(status_code=401, detail="用户名或密码错误")

    if not cfg_pass:
        raise HTTPException(status_code=403, detail="密码未设置，请先在配置文件中设置 webui.password")

    token = _create_token(body.username, cfg.webui.token_expire_hours)
    return {"token": token, "username": body.username, "expire_hours": cfg.webui.token_expire_hours}


@app.get("/api/auth/me")
async def auth_me(request: Request):
    """(保护) 验证 token 有效性，返回当前用户名。"""
    auth = request.headers.get("Authorization", "")
    token = auth[7:] if auth.startswith("Bearer ") else ""
    username = _verify_token(token)
    if not username:
        raise HTTPException(status_code=401, detail="Token 无效")
    return {"username": username}


@app.post("/api/auth/logout")
async def auth_logout():
    """(保护) 登出接口（前端清除 token 即可，后端无态）。"""
    return {"ok": True}


@app.get("/favicon.svg")
async def serve_favicon():
    favicon_path = os.path.join(_STATIC_DIR, "favicon.svg")
    if os.path.exists(favicon_path):
        return FileResponse(favicon_path, media_type="image/svg+xml")
    return JSONResponse({"detail": "not found"}, status_code=404)


@app.get("/")
async def serve_index():
    index_path = os.path.join(_ROOT_DIR, "frontend", "dist", "index.html")
    if not os.path.exists(index_path):
        return JSONResponse(
            {"detail": "前端未构建，请先运行 npm run build（或使用 Docker 镜像）"},
            status_code=404,
        )
    return FileResponse(index_path)


@app.get("/api/library", response_model=LibraryResponse)
async def get_library():
    """从本地 SQLite 快照读取媒体库（无需访问 Drive）"""
    store = get_library_store()
    snapshot = store.get_snapshot()
    if snapshot is None:
        # 从未扫描过，返回空库并提示用户手动刷新
        return LibraryResponse(movies=[], tv_shows=[], total_movies=0, total_tv=0,
                               scanned_at=None, hint="媒体库尚未扫描，请点击「将媒体库」按鈕刷新")
    movies    = [MediaItem(**m) for m in snapshot["movies"]]
    tv_shows  = [MediaItem(**t) for t in snapshot["tv_shows"]]
    return LibraryResponse(
        movies=movies,
        tv_shows=tv_shows,
        total_movies=snapshot["total_movies"],
        total_tv=snapshot["total_tv"],
        scanned_at=snapshot["scanned_at"],
    )


@app.post("/api/library/refresh")
async def refresh_library():
    """扫描 Google Drive，更新本地快照，返回变更统计"""
    try:
        import asyncio
        diff = await asyncio.get_event_loop().run_in_executor(None, _do_refresh_library)
        return diff
    except Exception as e:
        logger.exception("刷新媒体库失败")
        _app_log(
            "library",
            "refresh_failed",
            "媒体库刷新失败",
            level="ERROR",
            details={"error": str(e)},
        )
        raise HTTPException(status_code=500, detail=str(e))


# ──────────────────────────────────────────────────────────────
# 单条目元数据刷新（NFO + 封面 → Drive）
# ──────────────────────────────────────────────────────────────

class RefreshItemRequest(BaseModel):
    tmdb_id: int
    media_type: str          # "movie" | "tv"
    drive_folder_id: str     # 媒体文件夹 ID（电影文件夹 or 剧名文件夹）
    title: Optional[str] = None   # 供 tmdb_id=0 时按名称搜索
    year: Optional[str] = None    # 供 tmdb_id=0 时按年份辅助搜索


def _do_refresh_item(tmdb_id: int, media_type: str, drive_folder_id: str,
                     title: Optional[str] = None, year: Optional[str] = None) -> dict:
    """
    重新从 TMDB 获取详情，生成 NFO 并将封面/背景图上传覆盖到当前存储后端。
    直接复用 NfoGenerator 和 ImageUploader。
    tmdb_id=0 时自动用 title+year 搜索 TMDB。
    """
    # ── tmdb_id=0：按名称搜索 ────────────────────────────
    if not tmdb_id or tmdb_id <= 0:
        if not title:
            raise ValueError("该媒体项没有 TMDB ID 且未提供标题，无法搜索")
        cfg = get_config()
        if not cfg.is_tmdb_ready():
            raise ValueError("TMDB API Key 未配置，无法搜索")
        from mediaparser.types import MediaType as MType
        mtype = MType.TV if media_type == "tv" else MType.MOVIE
        tmdb_client = TmdbClient(
            api_key=cfg.tmdb.api_key,
            language=cfg.tmdb.language,
            proxy=cfg.tmdb_proxy,
            timeout=cfg.tmdb.timeout,
        )
        logger.info("tmdb_id=0，尝试按名称搜索：%r (%s) [%s]", title, year, media_type)
        found = tmdb_client._search_by_name(title, year, mtype)
        if not found:
            raise ValueError(f"TMDB 搜索无结果：{title!r}（{media_type}），请先手动确认 TMDB ID")
        tmdb_id = found.get("tmdb_id") or found.get("id")
        logger.info("按名称找到 tmdb_id=%s：%s", tmdb_id, found.get('name') or found.get('title'))

    client = get_storage_provider()
    gen    = NfoGenerator()
    uploader = ImageUploader(client, overwrite=True)

    uploaded: list[str] = []
    errors:   list[str] = []

    def _fetch_with_credits(path: str, extra_keys: str) -> dict:
        """先尝试 append_to_response，失败时回退到基础缓存路径。"""
        info = tmdb_get(path, {"append_to_response": extra_keys})
        if not info:
            logger.info("append_to_response 失败，回退到基础缓存：%s", path)
            info = tmdb_get(path)
        if not info:
            return {}
        # credits 未随正文返回时，尝试单独补充
        if not info.get("credits"):
            credits = tmdb_get(f"{path}/credits")
            if credits:
                info["credits"] = credits
        return info

    if media_type == "movie":
        # ── 电影 ────────────────────────────────────────────
        info = _fetch_with_credits(f"/movie/{tmdb_id}", "credits,external_ids,release_dates")
        if not info:
            raise ValueError(f"TMDB 未找到电影 tmdb_id={tmdb_id}，请稍候重试")

        info["tmdb_id"] = tmdb_id
        credits = info.get("credits") or {}
        info["directors"] = [
            {"id": p["id"], "name": p["name"], "profile_path": p.get("profile_path")}
            for p in (credits.get("crew") or [])
            if p.get("job") == "Director"
        ]
        info["actors"] = [
            {"id": p["id"], "name": p["name"],
             "character": p.get("character"), "profile_path": p.get("profile_path")}
            for p in (credits.get("cast") or [])[:20]
        ]

        # 查找文件夹内现有视频文件，生成同名 NFO
        try:
            folder_files = client.list_files(folder_id=drive_folder_id, page_size=50)
            video_files = [f for f in folder_files if f.is_video]
        except Exception as e:
            logger.warning("列出文件夹内容失败，将跳过视频同名 NFO: %s", e)
            video_files = []

        if video_files:
            for vf in video_files:
                nfo_name = gen.nfo_name_for(vf.name)
                try:
                    xml = gen.generate(info, media_type=None)
                    client.upload_text(xml, nfo_name,
                                       parent_id=drive_folder_id,
                                       mime_type="text/xml", overwrite=True)
                    uploaded.append(nfo_name)
                    logger.info("已上传 %s", nfo_name)
                except Exception as e:
                    errors.append(f"{nfo_name}: {e}")
                    logger.warning("上传 NFO 失败: %s - %s", nfo_name, e)
        else:
            # 无视频文件时，退而上传 movie.nfo
            try:
                xml = gen.generate(info, media_type=None)
                client.upload_text(xml, "movie.nfo",
                                   parent_id=drive_folder_id,
                                   mime_type="text/xml", overwrite=True)
                uploaded.append("movie.nfo")
            except Exception as e:
                errors.append(f"movie.nfo: {e}")

        if info.get("poster_path"):
            try:
                uploader.upload_poster(info["poster_path"], drive_folder_id)
                uploaded.append("poster.jpg")
            except Exception as e:
                errors.append(f"poster.jpg: {e}")
        if info.get("backdrop_path"):
            try:
                uploader.upload_fanart(info["backdrop_path"], drive_folder_id)
                uploaded.append("fanart.jpg")
            except Exception as e:
                errors.append(f"fanart.jpg: {e}")

    elif media_type == "tv":
        # ── 电视剧 ──────────────────────────────────────────
        info = _fetch_with_credits(f"/tv/{tmdb_id}", "credits,external_ids,content_ratings")
        if not info:
            raise ValueError(f"TMDB 未找到剧集 tmdb_id={tmdb_id}，请稍候重试")

        info["tmdb_id"] = tmdb_id
        credits = info.get("credits") or {}
        info["directors"] = [
            {"id": p["id"], "name": p["name"], "profile_path": p.get("profile_path")}
            for p in (credits.get("crew") or [])
            if p.get("job") in ("Director", "Executive Producer")
        ][:5]
        info["actors"] = [
            {"id": p["id"], "name": p["name"],
             "character": p.get("character"), "profile_path": p.get("profile_path")}
            for p in (credits.get("cast") or [])[:20]
        ]

        try:
            xml = gen.generate_tvshow(info)
            client.upload_text(xml, "tvshow.nfo",
                               parent_id=drive_folder_id,
                               mime_type="text/xml", overwrite=True)
            uploaded.append("tvshow.nfo")
        except Exception as e:
            errors.append(f"tvshow.nfo: {e}")

        if info.get("poster_path"):
            try:
                uploader.upload_poster(info["poster_path"], drive_folder_id)
                uploaded.append("poster.jpg")
            except Exception as e:
                errors.append(f"poster.jpg: {e}")
        if info.get("backdrop_path"):
            try:
                uploader.upload_fanart(info["backdrop_path"], drive_folder_id)
                uploaded.append("fanart.jpg")
            except Exception as e:
                errors.append(f"fanart.jpg: {e}")

        # 遍历季：season.nfo + 季封面
        try:
            season_folders = [
                f for f in client.list_files(folder_id=drive_folder_id, page_size=200)
                if f.is_folder and re.match(r"Season\s*\d+", f.name, re.IGNORECASE)
            ]
        except Exception as e:
            logger.warning("列出季文件夹失败，跳过季 NFO: %s", e)
            season_folders = []

        for season_folder in season_folders:
            m = re.search(r"(\d+)", season_folder.name)
            if not m:
                continue
            s_num = int(m.group(1))
            season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{s_num}")
            if not season_detail:
                logger.info("跳过 Season %d（TMDB 无数据）", s_num)
                continue
            try:
                xml = gen.generate_season(season_detail, s_num)
                client.upload_text(xml, "season.nfo",
                                   parent_id=season_folder.id,
                                   mime_type="text/xml", overwrite=True)
                uploaded.append(f"Season {s_num}/season.nfo")
            except Exception as e:
                errors.append(f"Season {s_num}/season.nfo: {e}")
            if season_detail.get("poster_path"):
                try:
                    uploader.upload_season_poster(
                        season_detail["poster_path"], s_num, drive_folder_id
                    )
                    uploaded.append(f"season{s_num:02d}-poster.jpg")
                except Exception as e:
                    errors.append(f"season{s_num:02d}-poster.jpg: {e}")
    else:
        raise ValueError(f"不支持的 media_type: {media_type}")

    updates = {
        "tmdb_id": tmdb_id,
        "title": info.get("name") if media_type == "tv" else info.get("title"),
        "original_title": info.get("original_name") if media_type == "tv" else info.get("original_title"),
        "overview": info.get("overview") or "",
        "rating": round(info.get("vote_average") or 0, 1),
    }

    # TMDB_IMG_BASE = "https://image.tmdb.org/t/p/w500"
    # TMDB_IMG_ORIG = "https://image.tmdb.org/t/p/original"
    if info.get("poster_path"):
        updates["poster_url"] = f"https://image.tmdb.org/t/p/w500{info['poster_path']}"
    if info.get("backdrop_path"):
        updates["backdrop_url"] = f"https://image.tmdb.org/t/p/original{info['backdrop_path']}"

    if media_type == "tv":
        updates["year"] = (info.get("first_air_date") or "")[:4]
        updates["status"] = info.get("status") or ""
        if info.get("number_of_episodes") is not None:
            updates["total_episodes"] = info.get("number_of_episodes")
    else:
        updates["year"] = (info.get("release_date") or "")[:4]

    # 保留 tmdb_id 是为了兼容外部逻辑，但核心更新内容放在 updates
    return {"ok": len(errors) == 0, "uploaded": uploaded, "errors": errors, "tmdb_id": tmdb_id, "updates": updates}


@app.post("/api/library/refresh-item")
async def refresh_item(body: RefreshItemRequest):
    """（需登录）刷新单个媒体项：重新从 TMDB 获取元数据，生成并上传 NFO + 封面到当前存储后端。"""
    if not body.drive_folder_id:
        raise HTTPException(status_code=400, detail="drive_folder_id 不能为空")
    try:
        import asyncio
        result = await asyncio.get_event_loop().run_in_executor(
            None,
            lambda: _do_refresh_item(body.tmdb_id, body.media_type, body.drive_folder_id,
                                     title=body.title, year=body.year)
        )
        _app_log(
            "library",
            "refresh_item",
            f"已刷新元数据：tmdb_id={body.tmdb_id} ({body.media_type})",
            level="SUCCESS" if result["ok"] else "WARNING",
            details={
                "tmdb_id": body.tmdb_id,
                "media_type": body.media_type,
                "drive_folder_id": body.drive_folder_id,
                "uploaded": result["uploaded"],
                "errors": result["errors"],
            },
        )
        # 刷新成功后，就地更新库缓存中该条目的详情，无需全量重扫
        updates = result.get("updates", {})
        if not updates and result.get("tmdb_id"):
            updates = {"tmdb_id": result.get("tmdb_id")}
            
        if updates and body.drive_folder_id:
            try:
                get_library_store().patch_item(
                    body.drive_folder_id,
                    updates,
                )
            except Exception as _pe:
                logger.warning("patch_item 失败（不影响刷新结果）: %s", _pe)
        return result
    except ValueError as e:
        raise HTTPException(status_code=404, detail=str(e))
    except Exception as e:
        logger.exception("刷新单条目失败")
        _app_log(
            "library",
            "refresh_item_failed",
            f"刷新元数据失败：{e}",
            level="ERROR",
            details={"tmdb_id": body.tmdb_id, "error": str(e)},
        )
        raise HTTPException(status_code=500, detail=str(e))


# ──────────────────────────────────────────────────────────────
# Webhook 触发（原 server.py 功能，合并入此）
# ──────────────────────────────────────────────────────────────

@app.post("/trigger")
async def trigger_pipeline(request: Request):
    """收到 rclone/aria2 的上传完成通知，调度运行 pipeline。
    若配置了 webhook_secret，需在 Header X-Webhook-Secret 或查询参数 secret 中传入。"""
    cfg_obj = Config.load()
    # webhook_secret 校验（配置为空时跳过，适合内网只用场景）
    ws = cfg_obj.webui.webhook_secret
    if ws:
        provided = (
            request.headers.get("X-Webhook-Secret", "")
            or request.query_params.get("secret", "")
        )
        if not hmac.compare_digest(provided.encode(), ws.encode()):
            raise HTTPException(status_code=403, detail="Invalid webhook secret")

    body: dict = {}
    try:
        body = await request.json()
    except Exception:
        pass
    debounce = cfg_obj.telegram.debounce_seconds
    
    # 仅在控制台输出，不发往前端日志（防刷屏）
    logger.info("收到整理触发请求 | 来源：webhook，防抖：%s 秒", debounce)
    
    schedule_pipeline(debounce)
    return {"status": "scheduled" if debounce > 0 else "triggered"}


@app.get("/api/pipeline/status")
async def pipeline_status():
    """（需登录）查询 pipeline 运行状态。"""
    return {
        "running":  _pipeline_running,
        "debounce": _debounce_timer is not None,
    }


@app.post("/api/pipeline/trigger")
async def trigger_pipeline():
    """（需登录）手动触发 pipeline。"""
    schedule_pipeline(0)
    return {"status": "triggered"}


@app.get("/api/library/movies", response_model=List[MediaItem])
async def get_movies():
    """获取电影列表"""
    try:
        client = get_storage_provider()
        cfg = get_config()
        return scan_movies(client, cfg)
    except Exception as e:
        logger.exception("获取电影列表失败")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/library/tv", response_model=List[MediaItem])
async def get_tv_shows():
    """获取电视剧列表"""
    try:
        client = get_storage_provider()
        cfg = get_config()
        return scan_tv_shows(client, cfg)
    except Exception as e:
        logger.exception("获取电视剧列表失败")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/tv/{tmdb_id}", response_model=MediaItem)
async def get_tv_detail(tmdb_id: int):
    """获取单部剧集详情（含季集入库状态）"""
    try:
        client = get_storage_provider()
        cfg = get_config()
        shows = scan_tv_shows(client, cfg)
        for show in shows:
            if show.tmdb_id == tmdb_id:
                return show
        raise HTTPException(status_code=404, detail=f"未找到 tmdb_id={tmdb_id} 的剧集")
    except HTTPException:
        raise
    except Exception as e:
        logger.exception("获取剧集详情失败")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/stats", response_model=StatsResponse)
async def get_stats():
    """获取统计信息"""
    try:
        client = get_storage_provider()
        cfg = get_config()
        movies = scan_movies(client, cfg)
        tv_shows = scan_tv_shows(client, cfg)

        total_eps_tmdb = sum(s.total_episodes or 0 for s in tv_shows)
        total_eps_lib = sum(s.in_library_episodes or 0 for s in tv_shows)
        rate = round(total_eps_lib / total_eps_tmdb * 100, 1) if total_eps_tmdb > 0 else 0.0

        return StatsResponse(
            total_movies=len(movies),
            total_tv_shows=len(tv_shows),
            total_episodes_in_library=total_eps_lib,
            total_episodes_on_tmdb=total_eps_tmdb,
            completion_rate=rate,
        )
    except Exception as e:
        logger.exception("获取统计信息失败")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/cache/stats")
async def cache_stats():
    """查看 TMDB 缓存使用情况"""
    return get_tmdb_cache().stats()


@app.post("/api/cache/evict")
async def cache_evict():
    """手动清理过期缓存条目"""
    count = get_tmdb_cache().evict_expired()
    return {"evicted": count, "message": f"已清理 {count} 条过期缓存"}


@app.get("/api/health")
async def health():
    return {"status": "ok"}


# ──────────────────────────────────────────────────────────────
# 配置文件 API（结构化表单版）
# ──────────────────────────────────────────────────────────────

_CONFIG_PATH = Path(_ROOT) / "config" / "config.yaml"


class ConfigSaveBody(BaseModel):
    data: dict


class ParserTestBody(BaseModel):
    filename: str


class Aria2AddUriBody(BaseModel):
    uris: List[str]
    options: Optional[Dict[str, str]] = None
    position: Optional[int] = None
    title: Optional[str] = None


class Aria2AddTorrentBody(BaseModel):
    torrent: str
    uris: Optional[List[str]] = None
    options: Optional[Dict[str, str]] = None
    position: Optional[int] = None
    title: Optional[str] = None


class Aria2BatchActionBody(BaseModel):
    gids: List[str]


class U115CreateSessionBody(BaseModel):
    client_id: Optional[str] = None
    token_json: Optional[str] = None


class U115ExchangeBody(BaseModel):
    client_id: Optional[str] = None
    token_json: Optional[str] = None


class U115OfflineAddUrlsBody(BaseModel):
    urls: str
    wp_path_id: Optional[str] = None


class U115OfflineDeleteBody(BaseModel):
    info_hashes: List[str]
    del_source_file: int = 0


class U115OfflineClearBody(BaseModel):
    flag: int = 0


def _resolve_config_path(path_str: str) -> str:
    path = Path(path_str)
    if not path.is_absolute():
        path = Path(_ROOT_DIR) / path
    return str(path)


def _u115_client() -> u115pan.Pan115Client:
    cfg = get_config().u115
    return u115pan.Pan115Client.from_token_file(
        client_id=cfg.client_id,
        token_path=_resolve_config_path(cfg.token_json),
    )


def _u115_offline_client() -> u115pan.OfflineClient:
    return u115pan.OfflineClient(_u115_client())


def _serialize_u115_offline_task(task: u115pan.OfflineTask) -> Dict[str, Any]:
    return {
        "info_hash": task.info_hash,
        "name": task.name,
        "status": task.status,
        "percent_done": task.percent_done,
        "size": task.size,
        "add_time": task.add_time,
        "last_update": task.last_update,
        "file_id": task.file_id,
        "delete_file_id": task.delete_file_id,
        "url": task.url,
        "wp_path_id": task.wp_path_id,
        "is_finished": task.is_finished,
        "is_downloading": task.is_downloading,
        "is_failed": task.is_failed,
    }


def _serialize_u115_offline_quota(quota: u115pan.OfflineQuotaInfo) -> Dict[str, Any]:
    return {
        "count": quota.count,
        "used": quota.used,
        "surplus": quota.surplus,
        "packages": [
            {
                "name": item.name,
                "count": item.count,
                "used": item.used,
                "surplus": item.surplus,
                "expire_info": [
                    {
                        "surplus": exp.surplus,
                        "expire_time": exp.expire_time,
                    }
                    for exp in item.expire_info
                ],
            }
            for item in quota.packages
        ],
    }


def _save_u115_device_session(session: u115pan.DeviceCodeSession, session_path: str) -> None:
    path = Path(session_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(
            {
                "qrcode": session.qrcode,
                "uid": session.uid,
                "time_value": session.time_value,
                "sign": session.sign,
                "code_verifier": session.code_verifier,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )


def _load_u115_device_session(session_path: str) -> u115pan.DeviceCodeSession:
    path = Path(session_path)
    if not path.exists():
        raise FileNotFoundError(f"115 扫码会话文件不存在：{path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    return u115pan.DeviceCodeSession(
        qrcode=str(data["qrcode"]),
        uid=str(data["uid"]),
        time_value=str(data["time_value"]),
        sign=str(data["sign"]),
        code_verifier=str(data["code_verifier"]),
    )


def _u115_oauth_status_payload() -> Dict[str, Any]:
    cfg = get_config().u115
    token_path = _resolve_config_path(cfg.token_json)
    session_path = _resolve_config_path(cfg.session_json)

    token_exists = os.path.exists(token_path)
    session_exists = os.path.exists(session_path)
    token_valid = False
    token_expired = False
    expires_at = None
    refresh_time = None
    authorized = False
    refreshed = False

    if token_exists:
        try:
            token = u115pan.load_token(token_path)
            if token:
                refresh_time = token.refresh_time
                expires_at = token.expires_at
                token_expired = token.is_expired(skew_seconds=0)
                if token_expired:
                    try:
                        client = u115pan.Pan115Client.from_token_file(
                            client_id=cfg.client_id,
                            token_path=token_path,
                        )
                        token = client.refresh_token()
                        refreshed = True
                        refresh_time = token.refresh_time
                        expires_at = token.expires_at
                        token_expired = False
                        token_valid = True
                        authorized = True
                    except Exception:
                        token_valid = False
                        authorized = False
                else:
                    token_valid = True
                    authorized = True
        except Exception:
            authorized = False

    return {
        "client_id": cfg.client_id,
        "token_path": cfg.token_json,
        "session_path": cfg.session_json,
        "token_exists": token_exists,
        "session_exists": session_exists,
        "authorized": authorized,
        "token_valid": token_valid,
        "token_expired": token_expired,
        "refresh_time": refresh_time,
        "expires_at": expires_at,
        "refreshed": refreshed,
    }


def _drive_oauth_status_payload() -> Dict[str, Any]:
    cfg = get_config().drive
    credentials_path = _resolve_config_path(cfg.credentials_json)
    token_path = _resolve_config_path(cfg.token_json)

    credentials_exists = os.path.exists(credentials_path)
    token_exists = os.path.exists(token_path)
    authorized = False
    token_valid = False
    token_expired = False
    token_refreshable = False

    if token_exists:
        try:
            creds = Credentials.from_authorized_user_file(token_path, DRIVE_SCOPES)
            token_valid = bool(creds.valid)
            token_expired = bool(creds.expired)
            token_refreshable = bool(creds.refresh_token)
            authorized = token_valid or token_refreshable
        except Exception:
            authorized = False

    return {
        "credentials_path": cfg.credentials_json,
        "token_path": cfg.token_json,
        "credentials_exists": credentials_exists,
        "token_exists": token_exists,
        "authorized": authorized,
        "token_valid": token_valid,
        "token_expired": token_expired,
        "token_refreshable": token_refreshable,
    }


def _aria2_sanitize_options(options: Optional[Dict[str, Any]]) -> Dict[str, str]:
    sanitized: Dict[str, str] = {}
    for key, value in (options or {}).items():
        if value is None:
            continue
        sanitized[str(key)] = str(value)
    return sanitized


def _aria2_get_task(gid: str) -> Dict[str, Any]:
    task = _aria2_rpc_call("tellStatus", [gid, _ARIA2_TASK_KEYS])
    return _aria2_normalize_task(task)


def _aria2_retry_task(gid: str) -> Dict[str, Any]:
    task = _aria2_rpc_call("tellStatus", [gid, _ARIA2_TASK_KEYS])
    uris = []
    for file_item in task.get("files") or []:
        for uri_item in file_item.get("uris") or []:
            uri = uri_item.get("uri")
            if uri and uri not in uris:
                uris.append(uri)

    if not uris:
        raise HTTPException(status_code=400, detail="该任务没有可重试的原始 URI")

    options = {}
    if task.get("dir"):
        options["dir"] = task["dir"]

    new_gid = _aria2_rpc_call("addUri", [uris, options])
    return _aria2_get_task(new_gid)


@app.get("/api/config")
async def read_config():
    """读取 config.yaml，返回解析后的结构化 dict"""
    if not _CONFIG_PATH.exists():
        raise HTTPException(status_code=404, detail="config.yaml 不存在")
    raw = _CONFIG_PATH.read_text(encoding="utf-8")
    parsed = yaml.safe_load(raw) or {}
    return parsed


def _looks_like_file_path(text: str) -> bool:
    # 1. 明显的双语/分隔符（带有局部空格的斜杠）必然不是路径
    if " / " in text or " \\" in text or "\\ " in text:
        return False
        
    # 2. 明显的文件系统绝对/相对路径前缀
    if re.match(r"^([a-zA-Z]:[\\/]|\\\\|/|\./|\.\./)", text):
        return True
        
    # 3. 如果是以中括号/方括号开头，极大概率是 BT 种子命名（即使内部包含 /，也多半是双语分隔）
    # 例如：[Subs] TitleA/TitleB
    if text.startswith("[") or text.startswith("【"):
        return False
        
    # 4. 如果包含明确的季数文件夹结构（如 /Season 1/ 或 /S01/ 等）
    if re.search(r"[/\\](Season\s*\d+|S\d{2}|Specials|Extras|Featurettes|OVA)[/\\]?", text, re.IGNORECASE):
        return True

    # 5. 兜底逻辑：具备路径的斜杠特征
    if "/" in text or "\\" in text:
        return True
        
    return False

@app.post("/api/parser/test")
async def parser_test(body: ParserTestBody):
    filename = body.filename.strip()
    if not filename:
        raise HTTPException(status_code=400, detail="文件名不能为空")

    cfg = get_config()
    matcher = _get_release_group_matcher()
    parser_kwargs = {
        "custom_words": cfg.parser.custom_words,
        "release_group_matcher": matcher,
    }

    if _looks_like_file_path(filename):
        meta = MetaInfoPath(filename, **parser_kwargs)
    else:
        meta = MetaInfo(filename, isfile=True, **parser_kwargs)

    payload = _serialize_meta(meta)
    tmdb_payload = None
    if cfg.tmdb.api_key:
        try:
            tmdb_client = TmdbClient(
                api_key=cfg.tmdb.api_key,
                language=cfg.tmdb.language,
                proxy=cfg.tmdb_proxy,
                timeout=cfg.tmdb.timeout,
                cache=get_tmdb_cache(),
            )
            tmdb_payload = _serialize_tmdb_result(tmdb_client.recognize(meta))
        except Exception as exc:
            logger.warning("解析测试 TMDB 查询失败：%s", exc)

    payload["tmdb"] = tmdb_payload
    _app_log(
        "parser",
        "parse_test",
        "执行了解析测试",
        details={
            "filename": filename,
            "name": payload.get("name"),
            "type": payload.get("type"),
            "tmdbMatched": bool(tmdb_payload),
        },
    )
    return payload


@app.put("/api/config")
async def write_config(body: ConfigSaveBody):
    """接收结构化 dict，写回 config.yaml（YAML dump）"""
    new_yaml = yaml.dump(
        body.data,
        allow_unicode=True,
        default_flow_style=False,
        sort_keys=False,
        indent=2,
    )
    _CONFIG_PATH.write_text(new_yaml, encoding="utf-8")
    # 使全局缓存失效
    global _cfg, _client, _storage_provider, _config_mtime_ns
    _cfg = None
    _client = None
    _storage_provider = None
    try:
        _config_mtime_ns = _CONFIG_PATH.stat().st_mtime_ns
    except FileNotFoundError:
        _config_mtime_ns = None
    _app_log("system", "config_updated", "配置文件已更新", level="SUCCESS")
    return {"ok": True, "message": "配置已保存"}


@app.get("/api/drive/oauth/status")
async def drive_oauth_status():
    return _drive_oauth_status_payload()


@app.post("/api/drive/test")
async def drive_test_connection():
    try:
        global _client
        _client = None
        client = get_drive_client()
        about = client.about() or {}
        user = about.get("user") or {}
        quota = about.get("storageQuota") or {}
        return {
            "ok": True,
            "email": user.get("emailAddress") or "",
            "display_name": user.get("displayName") or "",
            "limit": quota.get("limit"),
            "usage": quota.get("usage"),
            "usage_in_drive": quota.get("usageInDrive"),
        }
    except Exception as exc:
        logger.exception("Google Drive 连接测试失败")
        raise HTTPException(status_code=400, detail=f"Drive 连接测试失败：{exc}") from exc


@app.get("/api/u115/oauth/status")
async def u115_oauth_status():
    return _u115_oauth_status_payload()


@app.post("/api/u115/oauth/create")
async def u115_oauth_create(body: Optional[U115CreateSessionBody] = None):
    return await run_in_threadpool(_u115_oauth_create_sync, body)


def _u115_oauth_create_sync(body: Optional[U115CreateSessionBody] = None):
    cfg = get_config().u115
    client_id = (body.client_id if body and body.client_id else cfg.client_id).strip()
    token_json = (body.token_json if body and body.token_json else cfg.token_json).strip()
    session_json = cfg.session_json.strip()
    if not client_id:
        raise HTTPException(status_code=400, detail="u115.client_id 不能为空")

    client = u115pan.Pan115Client.from_token_file(
        client_id=client_id,
        token_path=_resolve_config_path(token_json),
    )
    try:
        session = client.create_device_code()
        _save_u115_device_session(session, _resolve_config_path(session_json))
        return {
            "ok": True,
            "qrcode": session.qrcode,
            "uid": session.uid,
            "session_path": session_json,
            "status": _u115_oauth_status_payload(),
        }
    except Exception as exc:
        logger.exception("115 创建扫码会话失败")
        raise HTTPException(status_code=400, detail=f"115 创建扫码会话失败：{exc}") from exc


@app.get("/api/u115/oauth/qrcode")
async def u115_oauth_qrcode():
    return await run_in_threadpool(_u115_oauth_qrcode_sync)


def _u115_oauth_qrcode_sync():
    cfg = get_config().u115
    try:
        import qrcode

        session = _load_u115_device_session(_resolve_config_path(cfg.session_json))
        qr = qrcode.QRCode(border=2, box_size=8)
        qr.add_data(session.qrcode)
        qr.make(fit=True)
        image = qr.make_image(fill_color="black", back_color="white")
        buffer = io.BytesIO()
        image.save(buffer, format="PNG")
        return Response(content=buffer.getvalue(), media_type="image/png")
    except Exception as exc:
        logger.exception("115 二维码代理失败")
        raise HTTPException(status_code=400, detail=f"115 二维码代理失败：{exc}") from exc


@app.get("/api/u115/oauth/poll")
async def u115_oauth_poll():
    return await run_in_threadpool(_u115_oauth_poll_sync)


def _u115_oauth_poll_sync():
    cfg = get_config().u115
    try:
        client = _u115_client()
        session_path = _resolve_config_path(cfg.session_json)
        session = _load_u115_device_session(session_path)
        status = client.get_qrcode_status(session)
        exchange_error = None

        # 实际使用中，115 的扫码状态有时会停留在“已扫码，待手机确认”，
        # 但 deviceCodeToToken 已经可以成功换 token。这里在已扫码后顺手探测一次，
        # 避免前端一直卡在等待确认。
        if not status.confirmed and int(status.status) >= 1:
            try:
                client.exchange_device_token(session)
                if os.path.exists(session_path):
                    os.remove(session_path)
                return {
                    "ok": True,
                    "status": status.status,
                    "message": "已确认并完成授权",
                    "confirmed": True,
                    "authorized": True,
                    "raw": status.raw,
                }
            except Exception as exc:
                exchange_error = str(exc)

        return {
            "ok": True,
            "status": status.status,
            "message": status.message,
            "confirmed": status.confirmed,
            "raw": status.raw,
            "exchange_error": exchange_error,
        }
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("115 查询扫码状态失败")
        raise HTTPException(status_code=400, detail=f"115 查询扫码状态失败：{exc}") from exc


@app.post("/api/u115/oauth/exchange")
async def u115_oauth_exchange(body: Optional[U115ExchangeBody] = None):
    return await run_in_threadpool(_u115_oauth_exchange_sync, body)


def _u115_oauth_exchange_sync(body: Optional[U115ExchangeBody] = None):
    cfg = get_config().u115
    client_id = (body.client_id if body and body.client_id else cfg.client_id).strip()
    token_json = (body.token_json if body and body.token_json else cfg.token_json).strip()
    session_json = cfg.session_json.strip()
    if not client_id:
        raise HTTPException(status_code=400, detail="u115.client_id 不能为空")

    client = u115pan.Pan115Client.from_token_file(
        client_id=client_id,
        token_path=_resolve_config_path(token_json),
    )
    try:
        resolved_session_path = _resolve_config_path(session_json)
        session = _load_u115_device_session(resolved_session_path)
        token = client.exchange_device_token(session)
        try:
            os.remove(resolved_session_path)
        except FileNotFoundError:
            pass
        return {
            "ok": True,
            "expires_in": token.expires_in,
            "refresh_time": token.refresh_time,
            "token_path": token_json,
            "status": _u115_oauth_status_payload(),
        }
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except Exception as exc:
        logger.exception("115 换取 token 失败")
        raise HTTPException(status_code=400, detail=f"115 换取 token 失败：{exc}") from exc


@app.post("/api/u115/test")
async def u115_test_connection():
    return await run_in_threadpool(_u115_test_connection_sync)


def _u115_test_connection_sync():
    try:
        client = _u115_client()
        space = client.get_space_info()
        return {
            "ok": True,
            "total_space": space.total_size,
            "remain_space": space.remain_size,
        }
    except Exception as exc:
        logger.exception("115 连接测试失败")
        raise HTTPException(status_code=400, detail=f"115 连接测试失败：{exc}") from exc


@app.get("/api/u115/offline/overview")
async def u115_offline_overview(
    page: int = Query(1, ge=1),
):
    return await run_in_threadpool(_u115_offline_overview_sync, page)


@app.get("/api/u115/offline/auto-organize-status")
async def u115_offline_auto_organize_status():
    return await run_in_threadpool(_u115_auto_organize_status_payload)


def _u115_offline_overview_sync(page: int):
    try:
        offline = _u115_offline_client()
        quota = offline.get_quota_info()
        task_page = offline.get_task_list(page=page)
        return {
            "ok": True,
            "quota": _serialize_u115_offline_quota(quota),
            "tasks": [_serialize_u115_offline_task(task) for task in task_page.tasks],
            "pagination": {
                "page": task_page.page,
                "page_size": len(task_page.tasks),
                "total": task_page.count,
                "total_pages": max(task_page.page_count, 1),
                "has_prev": task_page.page > 1,
                "has_next": task_page.page < max(task_page.page_count, 1),
            },
        }
    except Exception as exc:
        logger.exception("115 云下载概览获取失败")
        raise HTTPException(status_code=400, detail=f"115 云下载概览获取失败：{exc}") from exc


@app.post("/api/u115/offline/add-urls")
async def u115_offline_add_urls(body: U115OfflineAddUrlsBody):
    return await run_in_threadpool(_u115_offline_add_urls_sync, body)


def _u115_offline_add_urls_sync(body: U115OfflineAddUrlsBody):
    url_lines = [line.strip() for line in body.urls.splitlines() if line.strip()]
    if not url_lines:
        raise HTTPException(status_code=400, detail="请至少输入一个下载链接")

    try:
        offline = _u115_offline_client()
        cfg = get_config().u115
        target_path_id = (
            (body.wp_path_id.strip() if body.wp_path_id else "")
            or (cfg.download_folder_id.strip() if cfg.download_folder_id else "")
            or None
        )
        results = offline.add_task_urls(url_lines, wp_path_id=target_path_id)
        return {
            "ok": True,
            "count": len(results),
            "results": [asdict(item) for item in results],
            "wp_path_id": target_path_id,
        }
    except Exception as exc:
        logger.exception("115 云下载添加链接失败")
        raise HTTPException(status_code=400, detail=f"115 云下载添加链接失败：{exc}") from exc


@app.post("/api/u115/offline/tasks/delete")
async def u115_offline_delete_tasks(body: U115OfflineDeleteBody):
    return await run_in_threadpool(_u115_offline_delete_tasks_sync, body)


def _u115_offline_delete_tasks_sync(body: U115OfflineDeleteBody):
    info_hashes = [item.strip() for item in body.info_hashes if item and item.strip()]
    if not info_hashes:
        raise HTTPException(status_code=400, detail="请至少选择一个云下载任务")

    try:
        offline = _u115_offline_client()
        for info_hash in info_hashes:
            offline.del_task(info_hash, del_source_file=body.del_source_file)
        return {"ok": True, "deleted": len(info_hashes)}
    except Exception as exc:
        logger.exception("115 云下载删除任务失败")
        raise HTTPException(status_code=400, detail=f"115 云下载删除任务失败：{exc}") from exc


@app.post("/api/u115/offline/tasks/clear")
async def u115_offline_clear_tasks(body: U115OfflineClearBody):
    return await run_in_threadpool(_u115_offline_clear_tasks_sync, body)


def _u115_offline_clear_tasks_sync(body: U115OfflineClearBody):
    try:
        offline = _u115_offline_client()
        offline.clear_tasks(body.flag)
        return {"ok": True, "flag": body.flag}
    except Exception as exc:
        logger.exception("115 云下载清空任务失败")
        raise HTTPException(status_code=400, detail=f"115 云下载清空任务失败：{exc}") from exc


@app.get("/api/logs")
async def get_logs(
    limit: int = Query(200, ge=1, le=1000),
    category: Optional[str] = None,
    level: Optional[str] = None,
):
    return await run_in_threadpool(_logs_payload, limit, category, level)


def _logs_payload(limit: int, category: Optional[str], level: Optional[str]):
    _log_store.set_retention_days(get_config().webui.log_retention_days)
    return {
        "items": _log_store.read(limit=limit, category=category, level=level),
        "summary": _log_store.summary(),
    }


@app.get("/api/aria2/overview")
async def aria2_overview(
    queue: str = Query("all"),
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=200),
    search: str = Query(""),
):
    return await run_in_threadpool(_aria2_fetch_queue_items, queue, page, page_size, search)


@app.get("/api/aria2/options")
async def aria2_options():
    global_options = _aria2_rpc_call("getGlobalOption") or {}
    return {key: global_options.get(key, "") for key in _ARIA2_GLOBAL_OPTION_KEYS}


@app.put("/api/aria2/options")
async def aria2_update_options(body: Dict[str, Any]):
    options = _aria2_sanitize_options(body)
    _aria2_rpc_call("changeGlobalOption", [options])
    _app_log(
        "download",
        "options_updated",
        "下载器全局配置已更新",
        level="SUCCESS",
        details={"keys": sorted(options.keys())},
    )
    return {"ok": True, "options": await aria2_options()}


@app.post("/api/aria2/add-uri")
async def aria2_add_uri(body: Aria2AddUriBody):
    uris = [u.strip() for u in body.uris if u and u.strip()]
    if not uris:
        raise HTTPException(status_code=400, detail="至少需要一个下载链接")

    params: List[Any] = [uris, _aria2_sanitize_options(body.options)]
    if body.position is not None:
        params.append(body.position)

    gid = _aria2_rpc_call("addUri", params)
    task = _aria2_get_task(gid)
    _app_log(
        "download",
        "task_added_uri",
        f"已推送下载：{body.title}" if body.title else "已添加链接下载任务",
        level="SUCCESS",
        details={"gid": gid, "uriCount": len(uris), "name": body.title or task.get("name")},
    )
    return {"ok": True, "task": task}


@app.post("/api/aria2/add-torrent")
async def aria2_add_torrent(body: Aria2AddTorrentBody):
    try:
        raw = base64.b64decode(body.torrent, validate=True)
    except Exception as exc:
        raise HTTPException(status_code=400, detail="Torrent 内容不是有效的 Base64") from exc

    torrent_b64 = base64.b64encode(raw).decode("ascii")
    params: List[Any] = [
        torrent_b64,
        body.uris or [],
        _aria2_sanitize_options(body.options),
    ]
    if body.position is not None:
        params.append(body.position)

    gid = _aria2_rpc_call("addTorrent", params)
    task = _aria2_get_task(gid)
    _app_log(
        "download",
        "task_added_torrent",
        f"已推送下载：{body.title}" if body.title else "已添加种子下载任务",
        level="SUCCESS",
        details={"gid": gid, "name": body.title or task.get("name")},
    )
    return {"ok": True, "task": task}


@app.post("/api/aria2/tasks/pause")
async def aria2_pause_tasks(body: Aria2BatchActionBody):
    for gid in body.gids:
        _aria2_rpc_call("pause", [gid])
    _app_log(
        "download",
        "tasks_paused",
        "下载任务已暂停",
        level="SUCCESS",
        details={"gids": body.gids, "count": len(body.gids)},
    )
    return {"ok": True}


@app.post("/api/aria2/tasks/unpause")
async def aria2_unpause_tasks(body: Aria2BatchActionBody):
    for gid in body.gids:
        _aria2_rpc_call("unpause", [gid])
    _app_log(
        "download",
        "tasks_unpaused",
        "下载任务已恢复",
        level="SUCCESS",
        details={"gids": body.gids, "count": len(body.gids)},
    )
    return {"ok": True}



@app.post("/api/aria2/tasks/remove")
async def aria2_remove_tasks(body: Aria2BatchActionBody):
    for gid in body.gids:
        task = _aria2_rpc_call("tellStatus", [gid, ["status"]])
        method = "removeDownloadResult" if task.get("status") == "complete" else "remove"
        try:
            _aria2_rpc_call(method, [gid])
        except HTTPException:
            if method != "removeDownloadResult":
                _aria2_rpc_call("removeDownloadResult", [gid])
            else:
                raise
    _app_log(
        "download",
        "tasks_removed",
        "下载任务已移除",
        level="SUCCESS",
        details={"gids": body.gids, "count": len(body.gids)},
    )
    return {"ok": True}


@app.post("/api/aria2/tasks/retry")
async def aria2_retry_tasks(body: Aria2BatchActionBody):
    tasks = [_aria2_retry_task(gid) for gid in body.gids]
    _app_log(
        "download",
        "tasks_retried",
        "下载任务已重试",
        level="SUCCESS",
        details={"sourceGids": body.gids, "newGids": [task.get("gid") for task in tasks]},
    )
    return {"ok": True, "tasks": tasks}


@app.post("/api/aria2/tasks/purge")
async def aria2_purge_tasks():
    _aria2_rpc_call("purgeDownloadResult")
    _app_log("download", "tasks_purged", "已清空已完成/已停止下载记录", level="SUCCESS")
    return {"ok": True}

# ──────────────────────────────────────────────────────────────
# Scraper / 爬虫接口
# ──────────────────────────────────────────────────────────────

try:
    from scraper.core.factory import SpiderFactory
except ImportError:
    SpiderFactory = None

@app.get("/api/tmdb/search_multi")
async def tmdb_search_multi(keyword: str):
    from mediaparser.tmdb import TmdbClient
    cfg = get_config()
    if not cfg.tmdb.api_key:
        raise HTTPException(status_code=400, detail="TMDB API Key 未配置")
    
    # Initialize TmdbClient with user config
    tmdb_client = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
        cache=get_tmdb_cache(),
    )
    
    try:
        raw_results = tmdb_client.search_raw_multi(keyword)
        # Serialize fields so frontend can render them seamlessly
        serialized = []
        for r in raw_results:
            # Skip pure person objects if any
            if r.get("media_type") not in ("movie", "tv"):
                continue
            ser = _serialize_tmdb_result(r)
            if ser:
                serialized.append(ser)
        return {"ok": True, "results": serialized}
    except Exception as e:
        logger.error("TMDB 搜索失败: %s", e)
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/api/tmdb/detail")
async def tmdb_detail(tmdb_id: int, media_type: str):
    try:
        # 同步库信息，优先使用本地缓存以避免请求 TMDB
        from webui.library_store import get_library_store
        store = get_library_store()
        snapshot = store.get_snapshot()
        
        if snapshot:
            if media_type == "movie":
                found = next((m for m in snapshot["movies"] if str(m.get("tmdb_id", "")) == str(tmdb_id)), None)
                if found:
                    found_copy = dict(found)
                    found_copy["in_library"] = True
                    return {"ok": True, "detail": found_copy}
            elif media_type == "tv":
                found = next((t for t in snapshot["tv_shows"] if str(t.get("tmdb_id", "")) == str(tmdb_id)), None)
                if found:
                    found_copy = dict(found)
                    found_copy["in_library"] = True
                    return {"ok": True, "detail": found_copy}

        # 本地库没有，则请求 TMDB (较慢，但会走缓存)
        from mediaparser.tmdb import TmdbClient
        cfg = get_config()
        if not cfg.tmdb or not cfg.tmdb.api_key:
            raise ValueError("TMDB API Key 未配置")
        tmdb_client = TmdbClient(
            api_key=cfg.tmdb.api_key,
            language=cfg.tmdb.language,
            proxy=cfg.tmdb_proxy,
            timeout=cfg.tmdb.timeout,
            cache=get_tmdb_cache(),
        )

        if media_type == "movie":
            raw = tmdb_client._get_movie_detail(tmdb_id)
        else:
            raw = tmdb_client._get_tv_detail(tmdb_id)

        ser = _serialize_tmdb_result(raw)
        if not ser:
            raise ValueError("解析详情失败")

        ser["in_library"] = False
        return {"ok": True, "detail": ser}
    except Exception as e:
        logger.error("TMDB 详情获取失败: %s", e)
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/api/scraper/search_media")
async def scraper_search_media(keyword: str):
    if not SpiderFactory:
        raise HTTPException(status_code=500, detail="Scraper module not loaded")
    results = SpiderFactory.search_all(keyword)
    
    # 聚合结果
    from collections import defaultdict
    grouped = defaultdict(list)
    for r in results:
        grouped[r.name].append({
            "site": r.site,
            "media_id": r.media_id,
            "url": r.url,
            "cover_image": r.cover_image,
            "subgroup_id": getattr(r, "subgroup_id", None)
        })

    aggregate_results = []
    for name, sources in grouped.items():
        cover_image = next((s["cover_image"] for s in sources if s["cover_image"]), None)
        aggregate_results.append({
            "name": name,
            "cover_image": cover_image,
            "sources": sources
        })

    return {"ok": True, "results": aggregate_results}

@app.get("/api/scraper/get_episodes")
async def scraper_get_episodes(site: str, media_id: str, subgroup_id: str = None):
    if not SpiderFactory:
        raise HTTPException(status_code=500, detail="Scraper module not loaded")
    try:
        spider = SpiderFactory.get_spider(site)
        episodes = spider.get_episodes(media_id, subgroup_id)
        return {"ok": True, "episodes": [e.model_dump() for e in episodes]}
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        # 网络超时、Mikan RSS 错误、XML 解析失败等：返回空列表而不是 500
        logging.getLogger("scraper").warning(f"get_episodes failed for {site}/{media_id}/{subgroup_id}: {e}")
        return {"ok": False, "episodes": [], "error": str(e)}



@app.get("/api/tmdb/alternative_names")
async def tmdb_alternative_names(tmdb_id: int, media_type: str):
    """专用：通过 TMDB ID 直接获取别名列表，不走本地库快照。
    用于资源爬虫搜索时的别名 fallback。TmdbClient 内置缓存，重复调用不会重复请求。
    """
    try:
        from mediaparser.tmdb import TmdbClient
        cfg = get_config()
        if not cfg.tmdb or not cfg.tmdb.api_key:
            raise ValueError("TMDB API Key 未配置")
        tmdb_client = TmdbClient(
            api_key=cfg.tmdb.api_key,
            language=cfg.tmdb.language,
            proxy=cfg.tmdb_proxy,
            timeout=cfg.tmdb.timeout,
            cache=get_tmdb_cache(),
        )
        media_path = "movie" if media_type == "movie" else "tv"
        data = tmdb_client._get(f"/{media_path}/{tmdb_id}/alternative_titles", use_cache=False)
        if not data:
            return {"ok": True, "alternative_names": []}
        raw_titles = data.get("titles") or data.get("results") or []
        lang_map = {"CN": "zh", "TW": "zh", "HK": "zh", "SG": "zh", "JP": "ja", "US": "en", "GB": "en"}
        seen: set = set()
        result = []
        for t in raw_titles:
            name = t.get("title") or ""
            iso_3166 = (t.get("iso_3166_1") or "").upper()
            iso_639 = lang_map.get(iso_3166, iso_3166.lower()[:2] if iso_3166 else "")
            if name and name not in seen:
                seen.add(name)
                result.append({"name": name, "iso_639_1": iso_639})
        return {"ok": True, "alternative_names": result}
    except Exception as e:
        logger.error("TMDB 别名获取失败: %s", e)
        raise HTTPException(status_code=500, detail=str(e))


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("webui.api:app", host="0.0.0.0", port=38765, reload=True)

#!/usr/bin/env python3
"""
webui/api.py —— Metadata2GD 媒体库 Web UI 后端

提供以下 REST API:
  GET /api/library          - 获取完整媒体库（电影 + 电视剧）
  GET /api/library/movies   - 只获取电影列表
  GET /api/library/tv       - 只获取电视剧列表
  GET /api/tv/{tmdb_id}     - 获取单部剧集详情（含季集入库状态）
  GET /api/stats            - 获取统计信息
  GET /api/cache/stats      - 查看 TMDB 缓存使用情况
  POST /api/cache/evict     - 手动清理过期缓存

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
import uuid
from dataclasses import asdict
from pathlib import Path
from typing import Any, Dict, List, Optional
from urllib.parse import unquote, urlparse

import hmac
import secrets
import yaml
import requests
from datetime import datetime, timezone, timedelta
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry
from fastapi import FastAPI, HTTPException, Query, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import FileResponse, JSONResponse
from fastapi.staticfiles import StaticFiles
from pydantic import BaseModel
try:
    import jwt as _pyjwt
except ImportError:
    _pyjwt = None  # type: ignore  # JWT 功能需要 PyJWT

# 将项目根目录加入路径
_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path.insert(0, _ROOT)

from drive.client import DriveClient
from mediaparser import Config, MetaInfo, MetaInfoPath, TmdbClient
from mediaparser.release_group import ReleaseGroupsMatcher
from webui.tmdb_cache import TmdbCache
from webui.library_store import get_library_store
from webui.log_store import LogStore

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
_tmdb_cache: Optional[TmdbCache] = None
TMDB_IMG_BASE = "https://image.tmdb.org/t/p/w500"
TMDB_IMG_ORIG = "https://image.tmdb.org/t/p/original"

_ROOT_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
_CACHE_DB = os.path.join(_ROOT_DIR, "config", "data", "tmdb_cache.db")
_APP_LOG_DIR = os.path.join(_ROOT_DIR, "config", "data", "logs")
_LEGACY_APP_LOG_DB = os.path.join(_ROOT_DIR, "config", "data", "app_logs.jsonl")
_JWT_SECRET_FILE = os.path.join(_ROOT_DIR, "config", "data", ".jwt_secret")
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
    _app_log("library", "refresh_scan_start", "开始刷新媒体库快照")
    client   = get_drive_client()
    cfg      = get_config()
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
                          f"❌ <b>Metadata2GD</b>\n整理失败，退出码：<code>{returncode}</code>")
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
                      f"❌ <b>Metadata2GD</b>\n异常：<code>{exc}</code>")
    finally:
        with _pl_lock:
            _pipeline_running = False


def schedule_pipeline(debounce: int) -> None:
    """安排 pipeline 运行。debounce>0 则防抖，=0 则立即启动新线程。"""
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
        else:
            if _pipeline_running:
                _app_log("pipeline", "pipeline_skip_running", "整理流程已在运行，跳过本次触发", level="WARNING")
                return
            _app_log("pipeline", "pipeline_schedule", "整理流程立即执行", details={"debounceSeconds": 0})
            threading.Thread(target=_do_run_pipeline, daemon=True).start()



def get_tmdb_cache() -> TmdbCache:
    global _tmdb_cache
    if _tmdb_cache is None:
        _tmdb_cache = TmdbCache(_CACHE_DB)
        _tmdb_cache.evict_expired()  # 启动时清一次过期条目
    return _tmdb_cache


def get_config() -> Config:
    global _cfg
    if _cfg is None:
        _cfg = Config.load()
    return _cfg


def get_drive_client() -> DriveClient:
    global _client
    if _client is None:
        cfg = get_config()
        drive_cfg = cfg.drive
        _client = DriveClient.from_oauth(
            credentials_path=drive_cfg.credentials_json,
            token_path=drive_cfg.token_json,
        )
    return _client


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


def _aria2_rpc_call(method: str, params: Optional[List[Any]] = None) -> Any:
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


def _aria2_fetch_task_lists() -> Dict[str, List[Dict[str, Any]]]:
    active = _aria2_rpc_call("tellActive", [_ARIA2_TASK_KEYS]) or []
    waiting = _aria2_rpc_call("tellWaiting", [0, 1000, _ARIA2_TASK_KEYS]) or []
    stopped = _aria2_rpc_call("tellStopped", [0, 1000, _ARIA2_TASK_KEYS]) or []
    
    normalized_stopped = []
    metadata_gids_to_remove = []
    
    for t in stopped:
        norm_t = _aria2_normalize_task(t)
        name = norm_t.get("name", "")
        if norm_t.get("status") == "complete" and (
            "[METADATA]" in name or bool(re.match(r"^[0-9a-fA-F]{40}\.torrent$", name))
        ):
            metadata_gids_to_remove.append(norm_t["gid"])
        else:
            normalized_stopped.append(norm_t)
            
    if metadata_gids_to_remove:
        try:
            for gid in metadata_gids_to_remove:
                _aria2_rpc_call("removeDownloadResult", [gid])
            logger.info("自动清理了 %d 个已完成的 [METADATA] 或 .torrent 任务", len(metadata_gids_to_remove))
            _app_log(
                "download",
                "metadata_purged",
                f"已自动从队列中横扫清理并移除了 {len(metadata_gids_to_remove)} 个已完成的种子/元数据任务",
                level="INFO",
                details={"count": len(metadata_gids_to_remove), "gids": metadata_gids_to_remove}
            )
        except Exception as e:
            logger.warning("自动清理种子/元数据任务失败：%s", e)
            _app_log(
                "download",
                "metadata_purge_failed",
                f"尝试清理已完成的种子/元数据任务失败: {e}",
                level="WARNING",
                details={"error": str(e)}
            )

    # 翻转 active 和 waiting 列表，使得新添加的任务出现在前端顶部（aria2 默认加在尾部）
    # stopped 列表 aria2 原生就是按“最近停止”逆序的，所以保持原状即可
    return {
        "active": [_aria2_normalize_task(t) for t in active][::-1],
        "waiting": [_aria2_normalize_task(t) for t in waiting][::-1],
        "stopped": normalized_stopped,
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


def _read_drive_file_content(client: DriveClient, file_id: str) -> Optional[str]:
    """从 Drive 读取文本文件内容"""
    try:
        svc = client._svc
        request = svc.files().get_media(fileId=file_id)
        content = client._execute(request)
        if isinstance(content, bytes):
            return content.decode("utf-8", errors="ignore")
        return str(content)
    except Exception as e:
        logger.warning("读取 Drive 文件失败 %s: %s", file_id, e)
        return None


def scan_movies(client: DriveClient, cfg: Config) -> List[MediaItem]:
    """扫描电影目录，返回电影列表"""
    movie_root = cfg.organizer.movie_root_id or cfg.organizer.root_folder_id
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
            content = _read_drive_file_content(client, nfo.id)
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


def scan_tv_shows(client: DriveClient, cfg: Config) -> List[MediaItem]:
    """扫描剧集目录，返回剧集列表（含季集入库状态）"""
    tv_root = cfg.organizer.tv_root_id or cfg.organizer.root_folder_id
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
            content = _read_drive_file_content(client, tvshow_nfo.id)
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
    title="Metadata2GD 媒体库",
    description="查看 Google Drive 上的电影/电视剧入库状态",
    version="1.0.0",
)

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
        client = get_drive_client()
        cfg = get_config()
        return scan_movies(client, cfg)
    except Exception as e:
        logger.exception("获取电影列表失败")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/library/tv", response_model=List[MediaItem])
async def get_tv_shows():
    """获取电视剧列表"""
    try:
        client = get_drive_client()
        cfg = get_config()
        return scan_tv_shows(client, cfg)
    except Exception as e:
        logger.exception("获取电视剧列表失败")
        raise HTTPException(status_code=500, detail=str(e))


@app.get("/api/tv/{tmdb_id}", response_model=MediaItem)
async def get_tv_detail(tmdb_id: int):
    """获取单部剧集详情（含季集入库状态）"""
    try:
        client = get_drive_client()
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
        client = get_drive_client()
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
    # 使全局 Config 缓存失效
    global _cfg
    _cfg = None
    _app_log("system", "config_updated", "配置文件已更新", level="SUCCESS")
    return {"ok": True, "message": "配置已保存"}


@app.get("/api/logs")
async def get_logs(
    limit: int = Query(200, ge=1, le=1000),
    category: Optional[str] = None,
    level: Optional[str] = None,
):
    _log_store.set_retention_days(get_config().webui.log_retention_days)
    return {
        "items": _log_store.read(limit=limit, category=category, level=level),
        "summary": _log_store.summary(),
    }


@app.get("/api/aria2/overview")
async def aria2_overview():
    tasks = _aria2_fetch_task_lists()
    global_stat = _aria2_rpc_call("getGlobalStat") or {}
    version = _aria2_rpc_call("getVersion") or {}

    return {
        "summary": {
            "activeCount": len(tasks["active"]),
            "waitingCount": len(tasks["waiting"]),
            "stoppedCount": len(tasks["stopped"]),
            "downloadSpeed": int(global_stat.get("downloadSpeed") or 0),
            "uploadSpeed": int(global_stat.get("uploadSpeed") or 0),
            "numActive": int(global_stat.get("numActive") or 0),
            "numWaiting": int(global_stat.get("numWaiting") or 0),
            "numStopped": int(global_stat.get("numStopped") or 0),
        },
        "tasks": tasks,
        "version": {
            "version": version.get("version") or "",
            "enabledFeatures": version.get("enabledFeatures") or [],
        },
    }


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

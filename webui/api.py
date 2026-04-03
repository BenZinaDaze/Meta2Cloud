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
  conda run -n myself uvicorn webui.api:app --host 0.0.0.0 --port 8765 --reload
  # 或在项目根目录:
  conda run -n myself python -m uvicorn webui.api:app --host 0.0.0.0 --port 8765 --reload
"""

import logging
import os
import re
import subprocess
import sys
import threading
from pathlib import Path
from typing import Any, Dict, List, Optional

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
from mediaparser import Config
from webui.tmdb_cache import TmdbCache
from webui.library_store import get_library_store

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
_CACHE_DB = os.path.join(_ROOT_DIR, "data", "tmdb_cache.db")
_JWT_SECRET_FILE = os.path.join(_ROOT_DIR, "data", ".jwt_secret")
_jwt_secret_cache: Optional[str] = None


# ──────────────────────────────────────────────────────────────
# JWT 工具
# ──────────────────────────────────────────────────────────────

def _get_jwt_secret() -> str:
    """JWT 密钥：配置文件 > data/.jwt_secret > 自动生成并持久化"""
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
    client   = get_drive_client()
    cfg      = get_config()
    movies   = scan_movies(client, cfg)
    tv_shows = scan_tv_shows(client, cfg)
    return get_library_store().save_snapshot(movies, tv_shows)


def _do_run_pipeline() -> None:
    """防抖超时后真正执行 pipeline（在独立线程中调用）。"""
    global _pipeline_running, _debounce_timer
    cfg_obj    = Config.load()
    tg_token   = cfg_obj.telegram.bot_token
    tg_chat_id = cfg_obj.telegram.chat_id

    with _pl_lock:
        _debounce_timer  = None
        _pipeline_running = True

    logger.info("防抖超时，Pipeline 启动")
    try:
        result = subprocess.run(
            [sys.executable, "pipeline.py"],
            cwd=_ROOT_DIR,
            capture_output=False,
        )
        if result.returncode == 0:
            logger.info("Pipeline 完成 ✓")
            try:
                diff = _do_refresh_library()
                logger.info("媒体库已自动刷新 ✓  +%d / -%d",
                            diff.get("added", 0), diff.get("removed", 0))
            except Exception as re_exc:
                logger.warning("媒体库刷新异常：%s", re_exc)
        else:
            logger.error("Pipeline 退出码 %d", result.returncode)
            send_telegram(tg_token, tg_chat_id,
                          f"❌ <b>Metadata2GD</b>\n整理失败，退出码：<code>{result.returncode}</code>")
    except Exception as exc:
        logger.error("Pipeline 异常：%s", exc)
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
                logger.info("防抖计时器已重置，重新等待 %d 秒...", debounce)
            else:
                logger.info("收到首次触发，%d 秒后运行 Pipeline...", debounce)
            t = threading.Timer(debounce, _do_run_pipeline)
            t.daemon = True
            t.start()
            _debounce_timer = t
        else:
            if _pipeline_running:
                logger.info("Pipeline 正在运行，跳过本次触发")
                return
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
        if drive_cfg.auth_mode == "service_account":
            _client = DriveClient.from_service_account(drive_cfg.service_account_json)
        else:
            _client = DriveClient.from_oauth(
                credentials_path=drive_cfg.credentials_json,
                token_path=drive_cfg.token_json,
            )
    return _client


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
        content = request.execute()
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


# 挂载静态文件（前端构建产物：frontend/dist/）
_STATIC_DIR = os.path.join(_ROOT_DIR, "frontend", "dist")
if os.path.exists(_STATIC_DIR):
    app.mount("/static", StaticFiles(directory=_STATIC_DIR), name="static")


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


@app.get("/")
async def serve_index():
    index_path = os.path.join(os.path.dirname(__file__), "index.html")
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
    logger.info("收到触发请求  path=%s  debounce=%ds", body.get("path", ""), debounce)
    schedule_pipeline(debounce)
    return {"status": "scheduled" if debounce > 0 else "triggered"}


@app.get("/trigger/status")
async def trigger_status():
    """查询 pipeline 运行状态。"""
    return {
        "running":  _pipeline_running,
        "debounce": _debounce_timer is not None,
    }

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


@app.get("/api/config")
async def read_config():
    """读取 config.yaml，返回解析后的结构化 dict"""
    if not _CONFIG_PATH.exists():
        raise HTTPException(status_code=404, detail="config.yaml 不存在")
    raw = _CONFIG_PATH.read_text(encoding="utf-8")
    parsed = yaml.safe_load(raw) or {}
    return parsed


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
    logger.info("config.yaml 已更新（结构化）")
    return {"ok": True, "message": "配置已保存"}


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("webui.api:app", host="0.0.0.0", port=8765, reload=True)

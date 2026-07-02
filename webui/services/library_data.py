import re
from datetime import datetime
from typing import Dict, List, Optional, Set

from storage.base import StorageProvider
from mediaparser import Config, TmdbClient
from webui.ingest_store import get_ingest_store
from webui.library_store import get_library_store
from webui.schemas.library import EpisodeStatus, MediaItem, SeasonStatus
from webui.services.tmdb_service import get_tmdb_cache
from webui.services.tmdb_service import tmdb_get, tmdb_image_url
from webui.core.runtime import logger


def _normalize_modified_time(value) -> int:
    """将 CloudFile.modified_time 统一转换为秒级 Unix 时间戳。"""
    if value in (None, ""):
        return 0

    if isinstance(value, (int, float)):
        return int(value)

    if isinstance(value, str):
        raw = value.strip()
        if not raw:
            return 0
        if raw.isdigit():
            return int(raw)
        try:
            iso = raw[:-1] + "+00:00" if raw.endswith("Z") else raw
            return int(datetime.fromisoformat(iso).timestamp())
        except ValueError:
            logger.warning("无法解析 modified_time=%r，按 0 处理", value)
            return 0

    logger.warning("未知的 modified_time 类型 %s，按 0 处理", type(value).__name__)
    return 0


def build_seasons_status(
    tmdb_id: int,
    tmdb_info: dict,
    drive_episodes: Optional[Set[tuple]] = None,
    tmdb_use_cache: bool = True,
) -> tuple[List[SeasonStatus], int, int]:
    """
    根据 TMDB 信息构建 seasons 状态列表。

    Args:
        tmdb_id: TMDB ID
        tmdb_info: TMDB 剧集详情
        drive_episodes: 已入库的剧集集合 {(season, episode)}

    Returns:
        (seasons_status, total_episodes, in_library_episodes)
    """
    if drive_episodes is None:
        drive_episodes = set()

    seasons_status: List[SeasonStatus] = []
    total_eps = 0
    in_lib_eps = 0

    for season_raw in tmdb_info.get("seasons") or []:
        season_num = season_raw.get("season_number")
        if season_num is None:
            continue

        ep_count = season_raw.get("episode_count", 0)
        total_eps += ep_count

        season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{season_num}", use_cache=tmdb_use_cache)
        episodes_status: List[EpisodeStatus] = []

        if season_detail:
            for ep_raw in season_detail.get("episodes") or []:
                ep_num = ep_raw.get("episode_number", 0)
                in_lib = (season_num, ep_num) in drive_episodes
                if in_lib:
                    in_lib_eps += 1
                episodes_status.append(
                    EpisodeStatus(
                        episode_number=ep_num,
                        episode_title=ep_raw.get("name") or f"第 {ep_num} 集",
                        air_date=ep_raw.get("air_date") or "",
                        in_library=in_lib,
                    )
                )
        else:
            for ep_num in range(1, ep_count + 1):
                in_lib = (season_num, ep_num) in drive_episodes
                if in_lib:
                    in_lib_eps += 1
                episodes_status.append(
                    EpisodeStatus(
                        episode_number=ep_num,
                        episode_title=f"第 {ep_num} 集",
                        air_date="",
                        in_library=in_lib,
                    )
                )

        s_in_lib = sum(1 for ep in episodes_status if ep.in_library)
        seasons_status.append(
            SeasonStatus(
                season_number=season_num,
                season_name=season_raw.get("name") or f"Season {season_num}",
                poster_url=tmdb_image_url(season_raw.get("poster_path"), size="w500"),
                episode_count=len(episodes_status),
                in_library_count=s_in_lib,
                episodes=episodes_status,
            )
        )

    return seasons_status, total_eps, in_lib_eps


def fill_seasons_episodes(
    tmdb_id: int,
    existing_seasons: List[dict],
    tmdb_use_cache: bool = True,
) -> List[dict]:
    """
    补充已有 seasons 数据中缺失的 episodes 详情。

    Args:
        tmdb_id: TMDB ID
        existing_seasons: 已有的 seasons 数据（可能缺少 episodes）

    Returns:
        完整的 seasons 数据（包含 episodes）
    """
    seasons_data = []
    for season in existing_seasons:
        season_number = season.get("season_number")
        if season_number is None:
            continue

        existing_episode_flags = {}
        for ep in season.get("episodes") or []:
            if not isinstance(ep, dict):
                continue
            ep_num = ep.get("episode_number")
            if ep_num is None:
                continue
            existing_episode_flags[int(ep_num)] = bool(ep.get("in_library", False))

        season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{season_number}", use_cache=tmdb_use_cache)
        episodes = []

        if season_detail and season_detail.get("episodes"):
            for ep in season_detail["episodes"]:
                ep_num = ep.get("episode_number", 0)
                episodes.append({
                    "episode_number": ep_num,
                    "episode_title": ep.get("name") or f"第 {ep_num} 集",
                    "air_date": ep.get("air_date") or "",
                    "in_library": existing_episode_flags.get(int(ep_num), bool(ep.get("in_library", False))),
                })
        else:
            count = season.get("episode_count", 0)
            episodes = [
                {
                    "episode_number": i,
                    "episode_title": f"第 {i} 集",
                    "air_date": "",
                    "in_library": existing_episode_flags.get(i, False),
                }
                for i in range(1, count + 1)
            ]

        in_library_count = sum(1 for ep in episodes if ep.get("in_library"))

        seasons_data.append({
            "season_number": season_number,
            "season_name": season.get("season_name") or f"季 {season_number}",
            "poster_url": season.get("poster_url"),
            "episode_count": len(episodes),
            "in_library_count": in_library_count,
            "episodes": episodes,
        })

    return seasons_data


def parse_tmdb_id_from_nfo(nfo_content: str) -> Optional[int]:
    match = re.search(r"<tmdbid>(\d+)</tmdbid>", nfo_content, re.IGNORECASE)
    if match:
        return int(match.group(1))
    match = re.search(r"<uniqueid type=\"tmdb\"[^>]*>(\d+)</uniqueid>", nfo_content, re.IGNORECASE)
    if match:
        return int(match.group(1))
    return None


def parse_episode_from_filename(filename: str) -> Optional[tuple]:
    match = re.search(r"[Ss](\d+)[Ee](\d+)", filename)
    if match:
        return int(match.group(1)), int(match.group(2))
    return None


def _extract_title_year(name: str) -> tuple[str, str]:
    year_match = re.search(r"\((\d{4})\)", name or "")
    year = year_match.group(1) if year_match else ""
    title = re.sub(r"\s*\(\d{4}\)\s*$", "", name or "").strip()
    return title, year


def _tmdb_detail_path(media_type: str, tmdb_id: int) -> str:
    return f"/tv/{tmdb_id}" if media_type == "tv" else f"/movie/{tmdb_id}"


def _resolve_tmdb_info(media_type: str, tmdb_id: int) -> Optional[dict]:
    if not tmdb_id or tmdb_id <= 0:
        return None
    store = get_library_store()
    tmdb_entry = store.get_tmdb_entry(media_type, tmdb_id)
    if tmdb_entry and tmdb_entry.get("raw_json"):
        return tmdb_entry["raw_json"]
    return tmdb_get(_tmdb_detail_path(media_type, tmdb_id))


def _search_tmdb_identity(media_type: str, title: str, year: str, cfg: Config) -> tuple[int, Optional[dict]]:
    if not title or not cfg.is_tmdb_ready():
        return 0, None
    from mediaparser.types import MediaType as MType

    tmdb_client = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
        cache=get_tmdb_cache(),
    )
    mtype = MType.TV if media_type == "tv" else MType.MOVIE
    found = tmdb_client._search_by_name(title, year or None, mtype)
    if not found:
        return 0, None
    tmdb_id = int(found.get("tmdb_id") or found.get("id") or 0)
    return tmdb_id, found if tmdb_id > 0 else None


def _resolve_media_identity(media_type: str, drive_folder_id: str, folder_name: str, cfg: Config) -> tuple[int, Optional[dict]]:
    store = get_library_store()
    existing = store.get_library_item_by_folder_id(drive_folder_id)
    if existing:
        tmdb_id = int(existing.get("tmdb_id") or 0)
        if tmdb_id > 0:
            return tmdb_id, _resolve_tmdb_info(media_type, tmdb_id)

    title, year = _extract_title_year(folder_name)
    if title:
        matched = store.find_library_item_by_title_year(media_type, title, year)
        if matched:
            tmdb_id = int(matched.get("tmdb_id") or 0)
            if tmdb_id > 0:
                return tmdb_id, _resolve_tmdb_info(media_type, tmdb_id)

        tmdb_cached = store.find_tmdb_entry_by_title_year(media_type, title, year)
        if tmdb_cached:
            tmdb_id = int(tmdb_cached.get("tmdb_id") or 0)
            if tmdb_id > 0:
                return tmdb_id, tmdb_cached.get("raw_json") or _resolve_tmdb_info(media_type, tmdb_id)

        ingest_match = get_ingest_store().find_match_by_title_year(
            media_type=media_type,
            title=title,
            year=year,
        )
        if ingest_match:
            tmdb_id = int(ingest_match.get("tmdb_id") or 0)
            if tmdb_id > 0:
                return tmdb_id, _resolve_tmdb_info(media_type, tmdb_id)

        tmdb_id, tmdb_info = _search_tmdb_identity(media_type, title, year, cfg)
        if tmdb_id > 0 and tmdb_info:
            return tmdb_id, tmdb_info

    return 0, None


def _scan_single_movie_folder(client: StorageProvider, folder, cfg: Config) -> Optional[MediaItem]:
    mtime = _normalize_modified_time(folder.modified_time)
    tmdb_id, tmdb_info = _resolve_media_identity("movie", folder.id, folder.name, cfg)
    if tmdb_info:
        return MediaItem(
            tmdb_id=int(tmdb_id),
            title=tmdb_info.get("title") or folder.name,
            original_title=tmdb_info.get("original_title") or "",
            year=(tmdb_info.get("release_date") or "")[:4],
            media_type="movie",
            poster_url=tmdb_image_url(tmdb_info.get("poster_path"), size="w500"),
            backdrop_url=tmdb_image_url(tmdb_info.get("backdrop_path")),
            overview=tmdb_info.get("overview") or "",
            rating=round(tmdb_info.get("vote_average") or 0, 1),
            drive_folder_id=folder.id,
            folder_modified_time=mtime,
        )
    title, year = _extract_title_year(folder.name)
    return MediaItem(
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
        folder_modified_time=mtime,
    )


def scan_movies(client: StorageProvider, cfg: Config) -> List[MediaItem]:
    movie_root = cfg.active_movie_root_id()
    if not movie_root:
        return []
    movies = []
    movie_folders = [f for f in client.list_files(folder_id=movie_root, page_size=500) if f.is_folder]
    logger.info("扫描到 %d 个电影文件夹", len(movie_folders))
    for folder in movie_folders:
        movie = _scan_single_movie_folder(client, folder, cfg)
        if movie:
            movies.append(movie)
    return movies


def _tv_leaf_modified_time(show_folder, show_files: List) -> int:
    """剧集增量时间按 Season 叶子目录计算，缺失时退回剧名目录。"""
    show_mtime = _normalize_modified_time(show_folder.modified_time)
    season_mtimes = [
        _normalize_modified_time(item.modified_time)
        for item in show_files
        if item.is_folder and item.name.startswith("Season")
    ]
    return max(season_mtimes, default=show_mtime)


def _scan_single_tv_folder(
    client: StorageProvider,
    cfg: Config,
    show_folder,
    show_files: Optional[List] = None,
    tmdb_use_cache: bool = True,
) -> Optional[MediaItem]:
    show_files = show_files or client.list_files(folder_id=show_folder.id, page_size=200)
    mtime = _tv_leaf_modified_time(show_folder, show_files)
    tmdb_id, tmdb_info = _resolve_media_identity("tv", show_folder.id, show_folder.name, cfg)
    season_folders = [f for f in show_files if f.is_folder and f.name.startswith("Season")]
    drive_episodes: Set[tuple] = set()
    for season_folder in season_folders:
        season_match = re.search(r"Season\s+(\d+)", season_folder.name, re.IGNORECASE)
        if not season_match:
            continue
        season_num = int(season_match.group(1))
        season_files = client.list_files(folder_id=season_folder.id, page_size=500)
        for file in season_files:
            if file.is_video:
                episode = parse_episode_from_filename(file.name)
                if episode and episode[0] == season_num:
                    drive_episodes.add((episode[0], episode[1]))
    seasons_status: List[SeasonStatus] = []
    total_eps = 0
    in_lib_eps = 0
    if tmdb_info:
        seasons_status, total_eps, in_lib_eps = build_seasons_status(
            tmdb_id,
            tmdb_info,
            drive_episodes,
            tmdb_use_cache=tmdb_use_cache,
        )
    else:
        season_map: Dict[int, List[int]] = {}
        for season_num, ep_num in drive_episodes:
            season_map.setdefault(season_num, []).append(ep_num)
        for season_num in sorted(season_map):
            episodes = sorted(season_map[season_num])
            episodes_status = [EpisodeStatus(episode_number=ep, episode_title=f"第 {ep} 集", air_date="", in_library=True) for ep in episodes]
            in_lib_eps += len(episodes)
            total_eps += len(episodes)
            seasons_status.append(
                SeasonStatus(
                    season_number=season_num,
                    season_name=f"Season {season_num}",
                    poster_url=None,
                    episode_count=len(episodes),
                    in_library_count=len(episodes),
                    episodes=episodes_status,
                )
            )
    return MediaItem(
        tmdb_id=tmdb_id or 0,
        title=tmdb_info.get("name") if tmdb_info else re.sub(r"\s*\(\d{4}\)\s*$", "", show_folder.name).strip(),
        original_title=(tmdb_info.get("original_name") or "") if tmdb_info else "",
        year=((tmdb_info.get("first_air_date") or "")[:4]) if tmdb_info else (_m.group(1) if (_m := re.search(r"\((\d{4})\)", show_folder.name)) else ""),
        media_type="tv",
        poster_url=tmdb_image_url(tmdb_info.get("poster_path"), size="w500") if tmdb_info else None,
        backdrop_url=tmdb_image_url(tmdb_info.get("backdrop_path")) if tmdb_info else None,
        overview=(tmdb_info.get("overview") or "") if tmdb_info else "",
        rating=round((tmdb_info.get("vote_average") or 0), 1) if tmdb_info else 0.0,
        seasons=seasons_status,
        total_episodes=total_eps,
        in_library_episodes=in_lib_eps,
        status=(tmdb_info.get("status") or "") if tmdb_info else "",
        drive_folder_id=show_folder.id,
        folder_modified_time=mtime,
    )


def scan_tv_shows(client: StorageProvider, cfg: Config, tmdb_use_cache: bool = True) -> List[MediaItem]:
    tv_root = cfg.active_tv_root_id()
    if not tv_root:
        return []
    shows = []
    show_folders = [f for f in client.list_files(folder_id=tv_root, page_size=500) if f.is_folder]
    logger.info("扫描到 %d 个剧集文件夹", len(show_folders))
    for show_folder in show_folders:
        show = _scan_single_tv_folder(client, cfg, show_folder, tmdb_use_cache=tmdb_use_cache)
        if show:
            shows.append(show)
    return shows


def scan_movies_incremental(
    client: StorageProvider,
    cfg: Config,
    stored_mtimes: dict[str, int],
) -> tuple[List[MediaItem], dict[str, int]]:
    movie_root = cfg.active_movie_root_id()
    if not movie_root:
        return [], {}
    movies: List[MediaItem] = []
    current_mtimes: dict[str, int] = {}
    movie_folders = [f for f in client.list_files(folder_id=movie_root, page_size=500) if f.is_folder]
    logger.info("增量扫描到 %d 个电影文件夹", len(movie_folders))
    for folder in movie_folders:
        current_mtime = _normalize_modified_time(folder.modified_time)
        current_mtimes[folder.id] = current_mtime
        stored_mtime = stored_mtimes.get(folder.id, 0)
        if stored_mtime > 0 and current_mtime > 0 and current_mtime <= stored_mtime:
            continue
        movie = _scan_single_movie_folder(client, folder, cfg)
        if movie:
            movies.append(movie)
    skipped = len(movie_folders) - len(movies)
    logger.info("电影增量扫描：%d 跳过，%d 更新", skipped, len(movies))
    return movies, current_mtimes


def scan_tv_shows_incremental(
    client: StorageProvider,
    cfg: Config,
    stored_mtimes: dict[str, int],
) -> tuple[List[MediaItem], dict[str, int]]:
    tv_root = cfg.active_tv_root_id()
    if not tv_root:
        return [], {}
    shows: List[MediaItem] = []
    current_mtimes: dict[str, int] = {}
    show_folders = [f for f in client.list_files(folder_id=tv_root, page_size=500) if f.is_folder]
    logger.info("增量扫描到 %d 个剧集文件夹", len(show_folders))
    for show_folder in show_folders:
        show_files = client.list_files(folder_id=show_folder.id, page_size=200)
        current_mtime = _tv_leaf_modified_time(show_folder, show_files)
        current_mtimes[show_folder.id] = current_mtime
        stored_mtime = stored_mtimes.get(show_folder.id, 0)
        if stored_mtime > 0 and current_mtime > 0 and current_mtime <= stored_mtime:
            continue
        show = _scan_single_tv_folder(client, cfg, show_folder, show_files=show_files)
        if show:
            shows.append(show)
    skipped = len(show_folders) - len(shows)
    logger.info("剧集增量扫描：%d 跳过，%d 更新", skipped, len(shows))
    return shows, current_mtimes

import re
from typing import Dict, List, Optional, Set

from storage.base import StorageProvider
from mediaparser import Config
from webui.schemas.library import EpisodeStatus, MediaItem, SeasonStatus
from webui.services.tmdb_service import TMDB_IMG_BASE, TMDB_IMG_ORIG, tmdb_get
from webui.core.runtime import logger


def build_seasons_status(
    tmdb_id: int,
    tmdb_info: dict,
    drive_episodes: Optional[Set[tuple]] = None,
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

        season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{season_num}")
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
                poster_url=f"{TMDB_IMG_BASE}{season_raw['poster_path']}" if season_raw.get("poster_path") else None,
                episode_count=len(episodes_status),
                in_library_count=s_in_lib,
                episodes=episodes_status,
            )
        )

    return seasons_status, total_eps, in_lib_eps


def fill_seasons_episodes(
    tmdb_id: int,
    existing_seasons: List[dict],
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

        season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{season_number}")
        episodes = []

        if season_detail and season_detail.get("episodes"):
            for ep in season_detail["episodes"]:
                episodes.append({
                    "episode_number": ep.get("episode_number", 0),
                    "episode_title": ep.get("name") or f"第 {ep.get('episode_number', 0)} 集",
                    "air_date": ep.get("air_date") or "",
                    "in_library": ep.get("in_library", False),
                })
        else:
            count = season.get("episode_count", 0)
            episodes = [
                {"episode_number": i, "episode_title": f"第 {i} 集", "air_date": "", "in_library": False}
                for i in range(1, count + 1)
            ]

        seasons_data.append({
            "season_number": season_number,
            "season_name": season.get("season_name") or f"季 {season_number}",
            "poster_url": season.get("poster_url"),
            "episode_count": len(episodes),
            "in_library_count": season.get("in_library_count", 0),
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


def _scan_single_movie_folder(client: StorageProvider, folder) -> Optional[MediaItem]:
    mtime = int(folder.modified_time) if folder.modified_time else 0
    nfo_files = [f for f in client.list_files(folder_id=folder.id, page_size=100) if f.name.endswith(".nfo") and f.name != "tvshow.nfo"]
    tmdb_id = None
    tmdb_info = None
    for nfo in nfo_files:
        content = client.read_text(nfo)
        if content:
            tmdb_id = parse_tmdb_id_from_nfo(content)
            if tmdb_id:
                break
    if tmdb_id:
        tmdb_info = tmdb_get(f"/movie/{tmdb_id}")
    if tmdb_info:
        return MediaItem(
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
            folder_modified_time=mtime,
        )
    name = folder.name
    year_match = re.search(r"\((\d{4})\)", name)
    year = year_match.group(1) if year_match else ""
    title = re.sub(r"\s*\(\d{4}\)\s*$", "", name).strip()
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
        movie = _scan_single_movie_folder(client, folder)
        if movie:
            movies.append(movie)
    return movies


def _scan_single_tv_folder(client: StorageProvider, show_folder) -> Optional[MediaItem]:
    mtime = int(show_folder.modified_time) if show_folder.modified_time else 0
    show_files = client.list_files(folder_id=show_folder.id, page_size=200)
    tvshow_nfo = next((f for f in show_files if f.name == "tvshow.nfo"), None)
    tmdb_id = None
    if tvshow_nfo:
        content = client.read_text(tvshow_nfo)
        if content:
            tmdb_id = parse_tmdb_id_from_nfo(content)
    tmdb_info = tmdb_get(f"/tv/{tmdb_id}") if tmdb_id else None
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
            tmdb_id, tmdb_info, drive_episodes
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
        poster_url=f"{TMDB_IMG_BASE}{tmdb_info['poster_path']}" if tmdb_info and tmdb_info.get("poster_path") else None,
        backdrop_url=f"{TMDB_IMG_ORIG}{tmdb_info['backdrop_path']}" if tmdb_info and tmdb_info.get("backdrop_path") else None,
        overview=(tmdb_info.get("overview") or "") if tmdb_info else "",
        rating=round((tmdb_info.get("vote_average") or 0), 1) if tmdb_info else 0.0,
        seasons=seasons_status,
        total_episodes=total_eps,
        in_library_episodes=in_lib_eps,
        status=(tmdb_info.get("status") or "") if tmdb_info else "",
        drive_folder_id=show_folder.id,
        folder_modified_time=mtime,
    )


def scan_tv_shows(client: StorageProvider, cfg: Config) -> List[MediaItem]:
    tv_root = cfg.active_tv_root_id()
    if not tv_root:
        return []
    shows = []
    show_folders = [f for f in client.list_files(folder_id=tv_root, page_size=500) if f.is_folder]
    logger.info("扫描到 %d 个剧集文件夹", len(show_folders))
    for show_folder in show_folders:
        show = _scan_single_tv_folder(client, show_folder)
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
        current_mtime = int(folder.modified_time) if folder.modified_time else 0
        current_mtimes[folder.id] = current_mtime
        stored_mtime = stored_mtimes.get(folder.id, 0)
        if stored_mtime > 0 and current_mtime > 0 and current_mtime <= stored_mtime:
            continue
        movie = _scan_single_movie_folder(client, folder)
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
        current_mtime = int(show_folder.modified_time) if show_folder.modified_time else 0
        current_mtimes[show_folder.id] = current_mtime
        stored_mtime = stored_mtimes.get(show_folder.id, 0)
        if stored_mtime > 0 and current_mtime > 0 and current_mtime <= stored_mtime:
            continue
        show = _scan_single_tv_folder(client, show_folder)
        if show:
            shows.append(show)
    skipped = len(show_folders) - len(shows)
    logger.info("剧集增量扫描：%d 跳过，%d 更新", skipped, len(shows))
    return shows, current_mtimes

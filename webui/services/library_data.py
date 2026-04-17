import re
from typing import Dict, List, Optional

from storage.base import StorageProvider
from mediaparser import Config
from webui.schemas.library import EpisodeStatus, MediaItem, SeasonStatus
from webui.services.tmdb_service import TMDB_IMG_BASE, TMDB_IMG_ORIG, tmdb_get
from webui.core.runtime import logger


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


def scan_movies(client: StorageProvider, cfg: Config) -> List[MediaItem]:
    movie_root = cfg.active_movie_root_id()
    if not movie_root:
        return []
    movies = []
    movie_folders = [f for f in client.list_files(folder_id=movie_root, page_size=500) if f.is_folder]
    logger.info("扫描到 %d 个电影文件夹", len(movie_folders))
    for folder in movie_folders:
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
            movies.append(
                MediaItem(
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
                )
            )
        else:
            name = folder.name
            year_match = re.search(r"\((\d{4})\)", name)
            year = year_match.group(1) if year_match else ""
            title = re.sub(r"\s*\(\d{4}\)\s*$", "", name).strip()
            movies.append(
                MediaItem(
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
                )
            )
    return movies


def scan_tv_shows(client: StorageProvider, cfg: Config) -> List[MediaItem]:
    tv_root = cfg.active_tv_root_id()
    if not tv_root:
        return []
    shows = []
    show_folders = [f for f in client.list_files(folder_id=tv_root, page_size=500) if f.is_folder]
    logger.info("扫描到 %d 个剧集文件夹", len(show_folders))
    for show_folder in show_folders:
        show_files = client.list_files(folder_id=show_folder.id, page_size=200)
        tvshow_nfo = next((f for f in show_files if f.name == "tvshow.nfo"), None)
        tmdb_id = None
        if tvshow_nfo:
            content = client.read_text(tvshow_nfo)
            if content:
                tmdb_id = parse_tmdb_id_from_nfo(content)
        tmdb_info = tmdb_get(f"/tv/{tmdb_id}") if tmdb_id else None
        season_folders = [f for f in show_files if f.is_folder and f.name.startswith("Season")]
        drive_episodes: Dict[tuple, str] = {}
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
                        drive_episodes[(episode[0], episode[1])] = file.name
        seasons_status: List[SeasonStatus] = []
        total_eps = 0
        in_lib_eps = 0
        if tmdb_info:
            for season_raw in (tmdb_info.get("seasons") or []):
                season_num = season_raw.get("season_number")
                if season_num is None:
                    continue
                ep_count = season_raw.get("episode_count", 0)
                total_eps += ep_count
                season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{season_num}")
                episodes_status = []
                if season_detail:
                    for ep_raw in (season_detail.get("episodes") or []):
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
                        episodes_status.append(EpisodeStatus(episode_number=ep_num, episode_title=f"第 {ep_num} 集", air_date="", in_library=in_lib))
                s_in_lib = sum(1 for episode in episodes_status if episode.in_library)
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
        shows.append(
            MediaItem(
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
            )
        )
    return shows

from dataclasses import asdict
from typing import Any, Dict, List, Optional

import requests
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

from mediaparser import TmdbClient
from webui.tmdb_cache import TmdbCache
from webui.core.app_logging import app_log
from webui.core.runtime import _LIBRARY_DB, get_config, logger


TMDB_IMG_BASE = "https://image.tmdb.org/t/p/w500"
TMDB_IMG_ORIG = "https://image.tmdb.org/t/p/original"

_RETRY = Retry(
    total=3,
    backoff_factor=1.0,
    status_forcelist=[429, 500, 502, 503, 504],
    allowed_methods=["GET"],
    raise_on_status=False,
)
_http = requests.Session()
_http.mount("https://", HTTPAdapter(max_retries=_RETRY))
_http.mount("http://", HTTPAdapter(max_retries=_RETRY))

_tmdb_cache: Optional[TmdbCache] = None


def get_tmdb_cache() -> TmdbCache:
    global _tmdb_cache
    if _tmdb_cache is None:
        _tmdb_cache = TmdbCache(_LIBRARY_DB)
        _tmdb_cache.evict_expired()
    return _tmdb_cache


def serialize_meta(meta: Any) -> Dict[str, Any]:
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


def extract_alternative_names_with_lang(info: Dict[str, Any]) -> List[Dict[str, str]]:
    seen: set = set()
    result: List[Dict[str, str]] = []
    alt_titles_root = info.get("alternative_titles") or {}
    alt_titles = alt_titles_root.get("titles") or alt_titles_root.get("results") or []
    for title in alt_titles:
        name = title.get("title") or ""
        iso_3166 = title.get("iso_3166_1") or ""
        lang_map = {"CN": "zh", "TW": "zh", "HK": "zh", "SG": "zh", "JP": "ja", "US": "en", "GB": "en"}
        iso_639 = lang_map.get(iso_3166.upper(), iso_3166.lower()[:2] if iso_3166 else "")
        if name and name not in seen:
            seen.add(name)
            result.append({"name": name, "iso_639_1": iso_639})
    for translation in (info.get("translations") or {}).get("translations") or []:
        iso_639 = translation.get("iso_639_1") or ""
        data = translation.get("data") or {}
        name = data.get("title") or data.get("name") or ""
        if name and name not in seen:
            seen.add(name)
            result.append({"name": name, "iso_639_1": iso_639})
    return result


def serialize_tmdb_result(info: Optional[Dict[str, Any]]) -> Optional[Dict[str, Any]]:
    if not info:
        return None
    media_type = info.get("media_type")
    title = info.get("title") or info.get("name") or ""
    original_title = info.get("original_title") or info.get("original_name") or ""
    release_date = info.get("release_date") or info.get("first_air_date") or ""
    result = {
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
        "alternative_names": extract_alternative_names_with_lang(info),
        "directors": [person.get("name") for person in (info.get("directors") or []) if person.get("name")],
        "actors": [person.get("name") for person in (info.get("actors") or []) if person.get("name")][:8],
        "season_count": info.get("number_of_seasons"),
        "episode_count": info.get("number_of_episodes"),
    }
    if "seasons" in info:
        seasons_data = []
        for season in info["seasons"]:
            season_number = season.get("season_number")
            if season_number is None:
                continue
            count = season.get("episode_count", 0)
            episodes = [{"episode_number": i, "in_library": False} for i in range(1, count + 1)]
            seasons_data.append(
                {
                    "season_number": season_number,
                    "season_name": season.get("name") or f"季 {season_number}",
                    "poster_url": TmdbClient.image_url(season.get("poster_path")),
                    "air_date": season.get("air_date"),
                    "total_episodes": count,
                    "in_library_episodes": 0,
                    "episodes": episodes,
                }
            )
        result["seasons"] = seasons_data
    return result


def tmdb_get(path: str, extra: Optional[dict] = None) -> Optional[dict]:
    cfg = get_config()
    language = cfg.tmdb.language
    cache = get_tmdb_cache()
    cache_path = path
    if extra:
        suffix = "&".join(f"{k}={v}" for k, v in sorted(extra.items()) if k != "api_key")
        if suffix:
            cache_path = f"{path}?{suffix}"
    cached = cache.get(cache_path, language=language)
    if cached is not None:
        logger.debug("缓存命中：%s", cache_path)
        return cached
    if cache.is_failed(cache_path, language=language):
        logger.debug("冷却中，跳过：%s", cache_path)
        return None
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
    except Exception as exc:
        logger.warning("TMDB 请求失败（将冷却 5 分钟）%s: %s", path, exc)
        cache.set_failed(cache_path, language=language)
        return None

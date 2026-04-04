"""
TmdbClient —— 独立的 TMDB 元数据获取模块。

用法：
    from mediaparser.tmdb import TmdbClient
    from mediaparser import MetaInfo

    tmdb = TmdbClient(api_key="your_key", language="zh-CN")
    meta = MetaInfo("Breaking.Bad.S01E03.1080p.BluRay.HEVC")
    info = tmdb.recognize(meta)
    print(info)
"""
import logging
import re
import time
from typing import Optional, List, Dict, Any
from urllib.parse import urlencode

import requests
import zhconv

from mediaparser.meta_base import MetaBase
from mediaparser.string_utils import StringUtils
from mediaparser.types import MediaType

logger = logging.getLogger(__name__)

TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p/original"
_RETRY_WAIT = 5  # 429 限流等待秒数


class TmdbClient:
    """
    TMDB v3 API 客户端。
    直接调用 REST API，无任何框架依赖。
    """

    BASE_URL = "https://api.themoviedb.org/3"

    def __init__(
        self,
        api_key: str,
        language: str = "zh-CN",
        proxy: Optional[str] = None,
        timeout: int = 10,
        cache: Optional[Any] = None,
    ):
        """
        :param api_key:  TMDB API Key（v3）
        :param language: 返回语言，默认 zh-CN
        :param proxy:    代理地址，如 "http://127.0.0.1:7890"
        :param timeout:  请求超时秒数
        :param cache:    可选缓存对象，需要实现 get/is_failed/set/set_failed
        """
        self._api_key = api_key
        self._language = language
        self._timeout = timeout
        self._cache = cache
        self._session = requests.Session()
        if proxy:
            self._session.proxies = {"http": proxy, "https": proxy}
        self._session.headers.update({"Accept": "application/json"})

    # ── 核心入口 ─────────────────────────────────────────

    def recognize(self, meta: MetaBase) -> Optional[Dict[str, Any]]:
        """
        根据解析结果查询 TMDB 并返回完整媒体信息字典。

        :param meta: MetaInfo / MetaInfoPath 的解析结果
        :return: TMDB 信息字典，包含 media_type / tmdb_id / title 等字段；找不到返回 None
        """
        # 1. 有内嵌 tmdbid 直接查
        if meta.tmdbid:
            info = self.get_by_id(meta.type, meta.tmdbid)
            if info:
                return info

        # 2. 按名称搜索
        names = self._build_name_list(meta)
        mtype = meta.type if meta.type != MediaType.UNKNOWN else None

        for name in names:
            info = self._search_by_name(name, meta.year, mtype, meta.begin_season)
            if info:
                return info

        return None

    def get_by_id(self, mtype: Optional[MediaType], tmdbid: int) -> Optional[Dict[str, Any]]:
        """
        通过 TMDB ID 获取完整信息。
        mtype 为 None 时同时尝试 movie 和 tv。
        """
        if mtype == MediaType.MOVIE:
            return self._get_movie_detail(tmdbid)
        elif mtype == MediaType.TV:
            return self._get_tv_detail(tmdbid)
        else:
            # 类型未知，先试电影再试电视剧
            info = self._get_movie_detail(tmdbid)
            if not info:
                info = self._get_tv_detail(tmdbid)
            return info

    def get_season_detail(self, tmdbid: int, season: int) -> Optional[Dict]:
        """获取某季的完整信息（含所有集）"""
        return self._get("/tv/%d/season/%d" % (tmdbid, season))

    def get_episode_detail(self, tmdbid: int, season: int, episode: int) -> Optional[Dict]:
        """获取某集的完整信息"""
        return self._get("/tv/%d/season/%d/episode/%d" % (tmdbid, season, episode))

    # ── 搜索逻辑 ─────────────────────────────────────────

    def _search_by_name(
        self,
        name: str,
        year: Optional[str],
        mtype: Optional[MediaType],
        season: Optional[int] = None,
    ) -> Optional[Dict[str, Any]]:
        """
        按名称搜索，优先精确匹配，降级 multi 搜索。
        """
        if not name:
            return None

        year_range = self._year_range(year)

        if mtype == MediaType.MOVIE:
            for y in year_range:
                info = self._match_movie(name, y)
                if info:
                    return info
        elif mtype == MediaType.TV:
            for y in year_range:
                info = self._match_tv(name, y)
                if info:
                    return info
        else:
            # 类型未知：先电影再电视剧再 multi
            for y in year_range:
                info = self._match_movie(name, y)
                if info:
                    return info
            for y in year_range:
                info = self._match_tv(name, y)
                if info:
                    return info
            info = self._match_multi(name)
            if info:
                return info

        return None

    def _match_movie(self, name: str, year: Optional[str]) -> Optional[Dict]:
        params = {"query": name}
        if year:
            params["year"] = year
        results = self._search("movie", params)
        if not results:
            return None
        results = sorted(results, key=lambda x: x.get("release_date", "") or "", reverse=True)
        fast_skipped = []  # 年份不匹配的跳过，主字段不中的放进来做 fallback
        for r in results:
            r_year = (r.get("release_date") or "")[:4]
            if year and r_year != year:
                continue
            # 主字段快速匹配
            if self._name_match(name, [r.get("title"), r.get("original_title")]):
                return self._get_movie_detail(r["id"])
            fast_skipped.append(r)
        # 降级：对前 3 条拉完整详情，比对译名/别名
        for r in fast_skipped[:3]:
            detail = self._get_movie_detail(r["id"])
            if detail and self._name_match(name, detail.get("names", [])):
                return detail
        return None

    def _match_tv(self, name: str, year: Optional[str]) -> Optional[Dict]:
        params = {"query": name}
        if year:
            params["first_air_date_year"] = year
        results = self._search("tv", params)
        if not results:
            return None
        results = sorted(results, key=lambda x: x.get("first_air_date", "") or "", reverse=True)
        for r in results:
            r_year = (r.get("first_air_date") or "")[:4]
            if year and r_year != year:
                continue
            # 主字段快速匹配
            if self._name_match(name, [r.get("name"), r.get("original_name")]):
                return self._get_tv_detail(r["id"])
            # 降级：拉完整详情，比对全部译名/别名
            detail = self._get_tv_detail(r["id"])
            if detail and self._name_match(name, detail.get("names", [])):
                return detail
        return None

    def _match_multi(self, name: str) -> Optional[Dict]:
        results = self._search("multi", {"query": name})
        if not results:
            return None
        results = sorted(
            results,
            key=lambda x: (
                "1" if x.get("media_type") == "movie" else "0"
            ) + (x.get("release_date") or x.get("first_air_date") or ""),
            reverse=True,
        )
        for r in results:
            media_type = r.get("media_type")
            if media_type == "movie":
                if self._name_match(name, [r.get("title"), r.get("original_title")]):
                    return self._get_movie_detail(r["id"])
                detail = self._get_movie_detail(r["id"])
                if detail and self._name_match(name, detail.get("names", [])):
                    return detail
            elif media_type == "tv":
                if self._name_match(name, [r.get("name"), r.get("original_name")]):
                    return self._get_tv_detail(r["id"])
                detail = self._get_tv_detail(r["id"])
                if detail and self._name_match(name, detail.get("names", [])):
                    return detail
        return None

    # ── 详情获取 ─────────────────────────────────────────

    def _get_movie_detail(self, tmdbid: int) -> Optional[Dict]:
        data = self._get(
            "/movie/%d" % tmdbid,
            extra_params={"append_to_response": "credits,alternative_titles,translations,external_ids"},
        )
        if not data:
            return None
        data["media_type"] = MediaType.MOVIE
        data["tmdb_id"] = data.get("id")
        data["names"] = self._extract_names(data, MediaType.MOVIE)
        data["directors"] = self._extract_directors(data)
        data["actors"] = self._extract_actors(data)
        return data

    def _get_tv_detail(self, tmdbid: int) -> Optional[Dict]:
        data = self._get(
            "/tv/%d" % tmdbid,
            extra_params={"append_to_response": "credits,alternative_titles,translations,external_ids"},
        )
        if not data:
            return None
        data["media_type"] = MediaType.TV
        data["tmdb_id"] = data.get("id")
        data["names"] = self._extract_names(data, MediaType.TV)
        data["directors"] = self._extract_directors(data)
        data["actors"] = self._extract_actors(data)
        return data

    # ── 名称匹配工具 ─────────────────────────────────────

    @staticmethod
    def _name_match(query: str, candidates: List[Optional[str]]) -> bool:
        """忽略大小写和特殊字符的名称比较"""
        q = re.sub(r"[^a-zA-Z0-9\u4e00-\u9fff]", "", query).upper()
        for c in candidates:
            if not c:
                continue
            c_clean = re.sub(r"[^a-zA-Z0-9\u4e00-\u9fff]", "", c).upper()
            if q == c_clean:
                return True
        return False

    @staticmethod
    def _extract_names(data: dict, mtype: MediaType) -> List[str]:
        names = []
        if mtype == MediaType.MOVIE:
            for t in (data.get("alternative_titles") or {}).get("titles") or []:
                n = t.get("title")
                if n and n not in names:
                    names.append(n)
        else:
            for t in (data.get("alternative_titles") or {}).get("results") or []:
                n = t.get("title")
                if n and n not in names:
                    names.append(n)
        for tr in (data.get("translations") or {}).get("translations") or []:
            key = "title" if mtype == MediaType.MOVIE else "name"
            n = (tr.get("data") or {}).get(key)
            if n and n not in names:
                names.append(n)
        return names

    @staticmethod
    def _extract_directors(data: dict) -> List[dict]:
        crew = (data.get("credits") or {}).get("crew") or []
        return [
            {"id": p["id"], "name": p.get("name"), "profile_path": p.get("profile_path")}
            for p in crew
            if p.get("job") == "Director"
        ]

    @staticmethod
    def _extract_actors(data: dict) -> List[dict]:
        cast = (data.get("credits") or {}).get("cast") or []
        return [
            {
                "id": p["id"],
                "name": p.get("name"),
                "character": p.get("character"),
                "profile_path": p.get("profile_path"),
            }
            for p in cast[:20]  # 只取前 20
        ]

    # ── 名称列表构建 ─────────────────────────────────────

    @staticmethod
    def _build_name_list(meta: MetaBase) -> List[str]:
        names = []
        for n in [meta.cn_name, meta.en_name]:
            if not n:
                continue
            if n not in names:
                names.append(n)
            # 繁体 → 简体
            simplified = zhconv.convert(n, "zh-hans")
            if simplified != n and simplified not in names:
                names.append(simplified)
        return names

    @staticmethod
    def _year_range(year: Optional[str]) -> List[Optional[str]]:
        if not year:
            return [None]
        try:
            y = int(year)
            return [year, str(y - 1), str(y + 1)]
        except ValueError:
            return [year]

    # ── HTTP 基础方法 ────────────────────────────────────

    def _search(self, endpoint: str, params: dict) -> List[dict]:
        data = self._get("/search/%s" % endpoint, extra_params=params)
        if not data:
            return []
        return data.get("results") or []

    def _get(self, path: str, extra_params: Optional[dict] = None) -> Optional[dict]:
        params = {"api_key": self._api_key, "language": self._language}
        if extra_params:
            params.update(extra_params)
        cache_path = self._build_cache_path(path, extra_params)

        if self._cache is not None:
            cached = self._cache.get(cache_path, language=self._language)
            if cached is not None:
                logger.debug("TMDB 缓存命中：%s", cache_path)
                return cached
            if self._cache.is_failed(cache_path, language=self._language):
                logger.debug("TMDB 负缓存命中，跳过请求：%s", cache_path)
                return None

        url = self.BASE_URL + path
        for attempt in range(3):
            try:
                resp = self._session.get(url, params=params, timeout=self._timeout)
                if resp.status_code == 429:
                    wait = int(resp.headers.get("Retry-After", _RETRY_WAIT))
                    logger.warning("TMDB 限流，等待 %ds..." % wait)
                    time.sleep(wait)
                    continue
                if resp.status_code == 404:
                    return None
                resp.raise_for_status()
                data = resp.json()
                if self._cache is not None:
                    self._cache.set(cache_path, data, language=self._language)
                return data
            except requests.RequestException as e:
                logger.error("TMDB 请求失败 [%s %s]: %s" % (path, attempt + 1, e))
                if attempt < 2:
                    time.sleep(2)
        if self._cache is not None:
            self._cache.set_failed(cache_path, language=self._language)
        return None

    @staticmethod
    def _build_cache_path(path: str, extra_params: Optional[dict] = None) -> str:
        if not extra_params:
            return path
        # 与 WebUI 的缓存 key 规则保持一致，确保同一份 SQLite 可复用。
        suffix = urlencode(
            [(k, v) for k, v in sorted(extra_params.items()) if k != "api_key"],
            doseq=True,
        )
        return f"{path}?{suffix}" if suffix else path

    # ── 便利工具 ─────────────────────────────────────────

    @staticmethod
    def image_url(path: Optional[str]) -> Optional[str]:
        """将 TMDB 图片相对路径转为完整 URL"""
        if not path:
            return None
        return TMDB_IMAGE_BASE + path

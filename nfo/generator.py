"""
nfo/generator.py —— Plex / Infuse / Kodi 兼容 NFO 生成器（对齐 MoviePilot 标准）

NFO 格式：XBMC/Kodi XML，Plex Local Media Assets 和 Infuse 均支持。

生成类型：
    电影  → <movie>           与视频文件同名
    单集  → <episodedetails>  与视频文件同名，支持单集真实标题/简介
    整剧  → <tvshow>          放在剧名文件夹根目录（tvshow.nfo）
    季    → <season>          放在 Season 子文件夹（season.nfo）

相比初版的提升（对齐 MoviePilot TmdbScraper）：
    ✓ 全面切换 minidom 构建，<plot>/<outline>/<overview> 使用 CDATA
    ✓ 新增 <premiered>（首播/上映日期）
    ✓ 新增 <outline>（与 <plot> 相同，Kodi 系媒体服兼容）
    ✓ <actor> 增加 <type>Actor</type> / <tmdbid> / <profile> 人物链接
    ✓ <director> 增加 tmdbid 属性
    ✓ uniqueid 优先级修正：有 IMDB 时 IMDB 为 default="true"
    ✓ tvshow.nfo 的 <season>/<episode> 改为 -1（NFO 规范整剧约定）
    ✓ 新增 generate_season()：生成 season.nfo（<season> 根标签）
    ✓ 单集 NFO 支持接受 episode_detail 字典（单集真实标题/简介/导演/客串）
"""

import os
from typing import Any, Dict, List, Optional
from xml.dom import minidom

from mediaparser.types import MediaType

TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p/original"
TMDB_PERSON_BASE = "https://www.themoviedb.org/person/"


class NfoGenerator:
    """
    将 TmdbClient 返回的字典转为 NFO XML 字符串。

    用法：
        gen = NfoGenerator()

        # 电影
        xml = gen.generate(movie_info, MediaType.MOVIE)

        # 单集（可选传入单集详情以获得精确元数据）
        xml = gen.generate(show_info, MediaType.TV, episode_detail=ep_detail)

        # 整剧 NFO（tvshow.nfo）
        xml = gen.generate_tvshow(show_info)

        # 季 NFO（season.nfo）
        xml = gen.generate_season(season_detail, season_number=1)

        # 得到 NFO 文件名
        name = gen.nfo_name_for("Breaking.Bad.S01E03.mkv")
        # → "Breaking.Bad.S01E03.nfo"
    """

    # ── 公共接口 ────────────────────────────────────────────────────

    def generate(
        self,
        tmdb_info: Dict[str, Any],
        media_type: Optional[MediaType] = None,
        episode_detail: Optional[Dict[str, Any]] = None,
    ) -> str:
        """
        生成单集/电影 NFO XML。

        :param tmdb_info:      TmdbClient.recognize() 返回的字典
        :param media_type:     显式指定类型；None 时从 tmdb_info["media_type"] 读取
        :param episode_detail: TmdbClient.get_episode_detail() 返回的单集详情（TV 专用）
        :return:               格式化 UTF-8 XML 字符串
        """
        mtype = media_type or tmdb_info.get("media_type")
        if mtype == MediaType.TV:
            return self._build_episode_nfo(tmdb_info, episode_detail)
        else:
            return self._build_movie_nfo(tmdb_info)

    def generate_tvshow(self, tmdb_info: Dict[str, Any]) -> str:
        """
        生成 tvshow.nfo（<tvshow> 根标签）。

        Infuse 需要在剧名文件夹根目录放此文件才能识别为电视剧系列。
        Plex 也读取它补充剧集库元数据。
        """
        doc = minidom.Document()
        root = doc.createElement("tvshow")
        doc.appendChild(root)

        title = tmdb_info.get("name") or tmdb_info.get("title") or ""
        orig_title = tmdb_info.get("original_name") or tmdb_info.get("original_title") or ""
        year = self._extract_year(tmdb_info, MediaType.TV)
        overview = tmdb_info.get("overview") or ""

        self._add(doc, root, "title", title)
        self._add(doc, root, "originaltitle", orig_title)
        self._add(doc, root, "showtitle", title)
        self._add(doc, root, "year", year)
        self._add(doc, root, "premiered", tmdb_info.get("first_air_date") or "")
        self._add_cdata(doc, root, "plot", overview)
        self._add_cdata(doc, root, "outline", overview)
        self._add(doc, root, "rating", str(tmdb_info.get("vote_average") or ""))
        self._add(doc, root, "votes", str(tmdb_info.get("vote_count") or ""))

        # 整剧约定：-1 表示不指定（Kodi/NFO 规范）
        self._add(doc, root, "season", "-1")
        self._add(doc, root, "episode", "-1")

        # MPAA 内容分级
        if cr := (tmdb_info.get("content_ratings") or {}).get("results"):
            rating = next((r["rating"] for r in cr if r.get("iso_3166_1") == "US"), None)
            if rating:
                self._add(doc, root, "mpaa", rating)

        # IDs
        self._add_ids_tv(doc, root, tmdb_info)

        # 类型
        for genre in (tmdb_info.get("genres") or []):
            self._add(doc, root, "genre", genre.get("name") or "")

        # 制作国
        self._add_countries(doc, root, tmdb_info)

        self._add(doc, root, "language", tmdb_info.get("original_language") or "")

        # 导演
        self._add_directors(doc, root, tmdb_info.get("directors") or [])

        # 演员
        self._add_actors(doc, root, tmdb_info.get("actors") or [])

        # 封面 / 背景图
        self._add_images(doc, root, tmdb_info)

        return self._to_xml(doc)

    def generate_season(
        self,
        season_detail: Dict[str, Any],
        season_number: int,
    ) -> str:
        """
        生成 season.nfo（<season> 根标签）。

        :param season_detail: TmdbClient.get_season_detail() 返回的字典
        :param season_number: 季号（1/2/3…）
        """
        doc = minidom.Document()
        root = doc.createElement("season")
        doc.appendChild(root)

        overview = season_detail.get("overview") or ""
        air_date = season_detail.get("air_date") or ""
        title = season_detail.get("name") or f"Season {season_number}"

        self._add_cdata(doc, root, "plot", overview)
        self._add_cdata(doc, root, "outline", overview)
        self._add(doc, root, "title", title)
        self._add(doc, root, "premiered", air_date)
        self._add(doc, root, "releasedate", air_date)
        self._add(doc, root, "year", air_date[:4] if air_date else "")
        self._add(doc, root, "seasonnumber", str(season_number))

        # 季封面 URL（放进 NFO，Plex 可读）
        if poster := season_detail.get("poster_path"):
            self._add(doc, root, "thumb", TMDB_IMAGE_BASE + poster)

        return self._to_xml(doc)

    @staticmethod
    def nfo_name_for(video_filename: str) -> str:
        """视频文件名 → 同名 NFO 文件名（改扩展名为 .nfo）"""
        stem, _ = os.path.splitext(video_filename)
        return stem + ".nfo"

    @staticmethod
    def season_poster_name(season: int) -> str:
        """返回 Plex/Infuse 标准的季封面文件名，如 season01-poster.jpg"""
        if season == 0:
            return "season-specials-poster.jpg"
        return f"season{str(season).zfill(2)}-poster.jpg"

    # ── 电影 NFO ────────────────────────────────────────────────────

    def _build_movie_nfo(self, info: Dict[str, Any]) -> str:
        doc = minidom.Document()
        root = doc.createElement("movie")
        doc.appendChild(root)

        overview = info.get("overview") or ""

        self._add(doc, root, "title", info.get("title") or info.get("name") or "")
        self._add(doc, root, "originaltitle", info.get("original_title") or info.get("original_name") or "")
        self._add(doc, root, "premiered", info.get("release_date") or "")
        self._add(doc, root, "year", self._extract_year(info, MediaType.MOVIE))
        self._add_cdata(doc, root, "plot", overview)
        self._add_cdata(doc, root, "outline", overview)
        self._add(doc, root, "tagline", info.get("tagline") or "")
        self._add(doc, root, "rating", str(info.get("vote_average") or ""))
        self._add(doc, root, "votes", str(info.get("vote_count") or ""))

        # 时长
        if runtime := info.get("runtime"):
            self._add(doc, root, "runtime", str(runtime))

        # MPAA 内容分级
        if rd := (info.get("release_dates") or {}).get("results"):
            for country in rd:
                if country.get("iso_3166_1") == "US":
                    for rel in (country.get("release_dates") or []):
                        if cert := rel.get("certification"):
                            self._add(doc, root, "mpaa", cert)
                            break

        # IDs（IMDB 优先作为 default）
        self._add_ids_movie(doc, root, info)

        # 类型
        for genre in (info.get("genres") or []):
            self._add(doc, root, "genre", genre.get("name") or "")

        # 制作国
        for country in (info.get("production_countries") or []):
            self._add(doc, root, "country", country.get("name") or "")

        self._add(doc, root, "language", info.get("original_language") or "")

        # 导演
        self._add_directors(doc, root, info.get("directors") or [])

        # 演员
        self._add_actors(doc, root, info.get("actors") or [])

        # 封面 / 背景图
        self._add_images(doc, root, info)

        return self._to_xml(doc)

    # ── 单集 NFO ────────────────────────────────────────────────────

    def _build_episode_nfo(
        self,
        show_info: Dict[str, Any],
        episode_detail: Optional[Dict[str, Any]] = None,
    ) -> str:
        """
        生成 <episodedetails> NFO。
        若提供 episode_detail，优先使用单集的真实标题/简介/导演/客串演员。
        """
        doc = minidom.Document()
        root = doc.createElement("episodedetails")
        doc.appendChild(root)

        ep = episode_detail or {}
        show_title = show_info.get("name") or show_info.get("title") or ""

        # 单集标题：优先使用真实单集名，没有时用「第N集」，避免与 showtitle 重复
        # Infuse 会同时显示 showtitle 和 title，两者相同会出现重叠
        ep_name = ep.get("name") or ""
        if not ep_name:
            ep_num = show_info.get("_episode") or ep.get("episode_number")
            ep_title = f"第{ep_num}集" if ep_num else show_title
        else:
            ep_title = ep_name
        self._add(doc, root, "showtitle", show_title)
        self._add(doc, root, "title", ep_title)
        self._add(doc, root, "originaltitle", ep.get("name") or show_info.get("original_name") or "")

        # 年份：优先单集播出日期
        air_date = ep.get("air_date") or show_info.get("first_air_date") or ""
        self._add(doc, root, "aired", air_date)
        self._add(doc, root, "premiered", air_date)
        self._add(doc, root, "year", air_date[:4] if air_date else "")

        # 评分：优先单集评分
        rating = ep.get("vote_average") or show_info.get("vote_average") or ""
        self._add(doc, root, "rating", str(rating))

        # 简介：优先单集简介
        ep_overview = ep.get("overview") or show_info.get("overview") or ""
        self._add_cdata(doc, root, "plot", ep_overview)
        self._add_cdata(doc, root, "outline", ep_overview)

        # 季/集号
        season = show_info.get("_season") or ep.get("season_number")
        episode = show_info.get("_episode") or ep.get("episode_number")
        if season is not None:
            self._add(doc, root, "season", str(season))
        if episode is not None:
            self._add(doc, root, "episode", str(episode))

        # IDs（单集用单集自己的 TMDB ID）
        ep_tmdb_id = ep.get("id")  # 单集自己的 TMDB ID
        show_tmdb_id = show_info.get("tmdb_id") or show_info.get("id")
        ext_ids = show_info.get("external_ids") or {}
        tvdb = ext_ids.get("tvdb_id") or ""

        if ep_tmdb_id:
            # 单集使用单集 ID（对齐 MoviePilot 行为）
            self._add(doc, root, "tmdbid", str(ep_tmdb_id))
            uid = doc.createElement("uniqueid")
            uid.setAttribute("type", "tmdb")
            uid.setAttribute("default", "true")
            uid.appendChild(doc.createTextNode(str(ep_tmdb_id)))
            root.appendChild(uid)
        elif show_tmdb_id:
            self._add(doc, root, "tmdbid", str(show_tmdb_id))
            uid = doc.createElement("uniqueid")
            uid.setAttribute("type", "tmdb")
            uid.setAttribute("default", "true")
            uid.appendChild(doc.createTextNode(str(show_tmdb_id)))
            root.appendChild(uid)

        if tvdb:
            self._add(doc, root, "tvdbid", str(tvdb))
            uid2 = doc.createElement("uniqueid")
            uid2.setAttribute("type", "tvdb")
            uid2.appendChild(doc.createTextNode(str(tvdb)))
            root.appendChild(uid2)

        # 类型
        for genre in (show_info.get("genres") or []):
            self._add(doc, root, "genre", genre.get("name") or "")

        # 制作国
        self._add_countries(doc, root, show_info)
        self._add(doc, root, "language", show_info.get("original_language") or "")

        # 导演：优先单集导演（crew with job=Director）
        if ep:
            ep_directors = [
                {"id": p.get("id"), "name": p.get("name"), "profile_path": p.get("profile_path")}
                for p in (ep.get("crew") or [])
                if p.get("job") == "Director"
            ]
            self._add_directors(doc, root, ep_directors or (show_info.get("directors") or []))
        else:
            self._add_directors(doc, root, show_info.get("directors") or [])

        # 演员：单集客串演员 + 整剧主演
        if ep:
            guest_stars = [
                {
                    "id": p.get("id"), "name": p.get("name"),
                    "character": p.get("character"), "profile_path": p.get("profile_path"),
                }
                for p in (ep.get("guest_stars") or [])
                if p.get("known_for_department") == "Acting"
            ]
            self._add_actors(doc, root, guest_stars + (show_info.get("actors") or []))
        else:
            self._add_actors(doc, root, show_info.get("actors") or [])

        # 封面：优先单集剧照（still_path），回退整剧 poster
        still = ep.get("still_path")
        if still:
            self._add(doc, root, "thumb", TMDB_IMAGE_BASE + still)
        elif poster := show_info.get("poster_path"):
            self._add(doc, root, "thumb", TMDB_IMAGE_BASE + poster)

        return self._to_xml(doc)

    # ── 共享辅助 ────────────────────────────────────────────────────

    def _add_ids_movie(self, doc, root, info: Dict):
        """电影 ID 块：IMDB 有时优先 default=true"""
        tmdb_id = str(info.get("tmdb_id") or info.get("id") or "")
        ext_ids = info.get("external_ids") or {}
        imdb = ext_ids.get("imdb_id") or info.get("imdb_id") or ""

        self._add(doc, root, "tmdbid", tmdb_id)
        if imdb:
            self._add(doc, root, "imdbid", imdb)

        has_imdb = bool(imdb)
        if tmdb_id:
            uid = doc.createElement("uniqueid")
            uid.setAttribute("type", "tmdb")
            uid.setAttribute("default", "false" if has_imdb else "true")
            uid.appendChild(doc.createTextNode(tmdb_id))
            root.appendChild(uid)
        if has_imdb:
            uid2 = doc.createElement("uniqueid")
            uid2.setAttribute("type", "imdb")
            uid2.setAttribute("default", "true")
            uid2.appendChild(doc.createTextNode(imdb))
            root.appendChild(uid2)

    def _add_ids_tv(self, doc, root, info: Dict):
        """电视剧 ID 块"""
        tmdb_id = str(info.get("tmdb_id") or info.get("id") or "")
        ext_ids = info.get("external_ids") or {}
        tvdb = str(ext_ids.get("tvdb_id") or "")

        self._add(doc, root, "tmdbid", tmdb_id)
        if tvdb:
            self._add(doc, root, "tvdbid", tvdb)

        if tmdb_id:
            uid = doc.createElement("uniqueid")
            uid.setAttribute("type", "tmdb")
            uid.setAttribute("default", "true")
            uid.appendChild(doc.createTextNode(tmdb_id))
            root.appendChild(uid)
        if tvdb:
            uid2 = doc.createElement("uniqueid")
            uid2.setAttribute("type", "tvdb")
            uid2.appendChild(doc.createTextNode(tvdb))
            root.appendChild(uid2)

    def _add_directors(self, doc, root, directors: List[Dict]):
        """添加 <director tmdbid="...">Name</director>"""
        for d in directors:
            el = doc.createElement("director")
            el.setAttribute("tmdbid", str(d.get("id") or ""))
            el.appendChild(doc.createTextNode(d.get("name") or ""))
            root.appendChild(el)

    def _add_actors(self, doc, root, actors: List[Dict]):
        """添加完整 <actor> 块：name/type/role/tmdbid/thumb/profile"""
        for actor in actors:
            xactor = doc.createElement("actor")
            self._add(doc, xactor, "name", actor.get("name") or "")
            self._add(doc, xactor, "type", "Actor")
            self._add(doc, xactor, "role", actor.get("character") or "")
            self._add(doc, xactor, "tmdbid", str(actor.get("id") or ""))
            if profile_path := actor.get("profile_path"):
                self._add(doc, xactor, "thumb", TMDB_IMAGE_BASE + profile_path)
            if actor_id := actor.get("id"):
                self._add(doc, xactor, "profile", f"{TMDB_PERSON_BASE}{actor_id}")
            root.appendChild(xactor)

    def _add_countries(self, doc, root, info: Dict):
        for country in (info.get("production_countries") or info.get("origin_country") or []):
            if isinstance(country, str):
                self._add(doc, root, "country", country)
            else:
                self._add(doc, root, "country", country.get("name") or "")

    def _add_images(self, doc, root, info: Dict):
        if poster := info.get("poster_path"):
            self._add(doc, root, "thumb", TMDB_IMAGE_BASE + poster)
        if backdrop := info.get("backdrop_path"):
            fanart = doc.createElement("fanart")
            self._add(doc, fanart, "thumb", TMDB_IMAGE_BASE + backdrop)
            root.appendChild(fanart)

    # ── 底层工具 ────────────────────────────────────────────────────

    @staticmethod
    def _add(doc, parent, tag: str, text: str):
        """添加子元素（text 空时跳过）"""
        if not text or not str(text).strip():
            return
        el = doc.createElement(tag)
        el.appendChild(doc.createTextNode(str(text)))
        parent.appendChild(el)

    @staticmethod
    def _add_cdata(doc, parent, tag: str, text: str):
        """添加带 CDATA 的子元素（安全转义 <>&）"""
        el = doc.createElement(tag)
        el.appendChild(doc.createCDATASection(text or ""))
        parent.appendChild(el)

    @staticmethod
    def _extract_year(info: Dict[str, Any], mtype: MediaType) -> str:
        date = (
            info.get("release_date") if mtype == MediaType.MOVIE
            else info.get("first_air_date")
        ) or ""
        return date[:4] if date else ""

    @staticmethod
    def _to_xml(doc: minidom.Document) -> str:
        """序列化为带 XML 声明的格式化字符串"""
        pretty = doc.toprettyxml(indent="  ", encoding=None)
        lines = pretty.split("\n")
        # 替换首行声明为标准格式
        lines[0] = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        # 删除多余空行
        lines = [l for l in lines if l.strip()]
        return "\n".join(lines) + "\n"

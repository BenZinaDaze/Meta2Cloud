"""
MetaBase —— 媒体元数据基类，存放所有解析结果字段及副标题中文格式解析。
来源：MoviePilot app/core/meta/metabase.py（移除 app.log / app.schemas 依赖）
"""
import logging
import traceback
from dataclasses import dataclass
from typing import Union, Optional, List

import cn2an
import regex as re

from mediaparser.string_utils import StringUtils
from mediaparser.types import MediaType

logger = logging.getLogger(__name__)

# 媒体文件扩展名
RMT_MEDIAEXT = [
    ".mp4", ".mkv", ".ts", ".iso", ".rmvb", ".avi", ".mov", ".mpeg",
    ".mpg", ".wmv", ".3gp", ".asf", ".m4v", ".flv", ".m2ts", ".strm", ".tp", ".f4v",
]
RMT_SUBEXT = [".srt", ".ass", ".ssa", ".sub"]
RMT_AUDIOEXT = [".mp3", ".flac", ".wav", ".aac", ".ogg", ".m4a", ".wma"]
MEDIA_EXTS = RMT_MEDIAEXT + RMT_SUBEXT + RMT_AUDIOEXT


@dataclass
class MetaBase:
    """媒体信息基类"""
    isfile: bool = False
    title: str = ""
    org_string: Optional[str] = None
    subtitle: Optional[str] = None
    type: MediaType = MediaType.UNKNOWN
    cn_name: Optional[str] = None
    en_name: Optional[str] = None
    year: Optional[str] = None
    total_season: int = 0
    begin_season: Optional[int] = None
    end_season: Optional[int] = None
    total_episode: int = 0
    begin_episode: Optional[int] = None
    end_episode: Optional[int] = None
    part: Optional[str] = None
    resource_type: Optional[str] = None
    resource_effect: Optional[str] = None
    resource_pix: Optional[str] = None
    resource_team: Optional[str] = None
    customization: Optional[str] = None
    web_source: Optional[str] = None
    video_encode: Optional[str] = None
    audio_encode: Optional[str] = None
    apply_words: Optional[List[str]] = None
    tmdbid: int = None
    doubanid: str = None
    fps: Optional[int] = None

    # 副标题解析辅助
    _subtitle_flag: bool = False
    _title_episodel_re = r"Episode\s+(\d{1,4})"
    _subtitle_season_re = r"(?<![全共]\s*)[第\s]+([0-9一二三四五六七八九十S\-]+)\s*季(?!\s*[全共])"
    _subtitle_season_all_re = r"[全共]\s*([0-9一二三四五六七八九十]+)\s*季"
    _subtitle_episode_re = r"(?<![全共]\s*)[第\s]+([0-9一二三四五六七八九十百零EP]+)\s*[集话話期幕](?!\s*[全共])"
    _subtitle_episode_between_re = r"[第]*\s*([0-9一二三四五六七八九十百零]+)\s*[集话話期幕]?\s*-\s*第*\s*([0-9一二三四五六七八九十百零]+)\s*[集话話期幕]"
    _subtitle_episode_all_re = r"([0-9一二三四五六七八九十百零]+)\s*集\s*全|[全共]\s*([0-9一二三四五六七八九十百零]+)\s*[集话話期幕]"

    def __init__(self, title: str, subtitle: str = None, isfile: bool = False):
        if not title:
            return
        self.org_string = title.strip()
        self.subtitle = subtitle.strip() if subtitle else None
        self.isfile = isfile

    # ── 名称属性 ──────────────────────────────────────────
    @property
    def name(self) -> str:
        if self.cn_name and StringUtils.is_all_chinese(self.cn_name):
            return self.cn_name
        elif self.en_name:
            return self.en_name
        elif self.cn_name:
            return self.cn_name
        return ""

    @name.setter
    def name(self, name: str):
        if StringUtils.is_all_chinese(name):
            self.cn_name = name
        else:
            self.en_name = name
            self.cn_name = None

    # ── 季属性 ───────────────────────────────────────────
    @property
    def season(self) -> str:
        if self.begin_season is not None:
            return (
                "S%s" % str(self.begin_season).rjust(2, "0")
                if self.end_season is None
                else "S%s-S%s" % (
                    str(self.begin_season).rjust(2, "0"),
                    str(self.end_season).rjust(2, "0"),
                )
            )
        return "S01" if self.type == MediaType.TV else ""

    @property
    def sea(self) -> str:
        return self.season if self.begin_season is not None else ""

    @property
    def season_seq(self) -> str:
        if self.begin_season is not None:
            return str(self.begin_season)
        return "1" if self.type == MediaType.TV else ""

    @property
    def season_list(self) -> List[int]:
        if self.begin_season is None:
            return [1] if self.type == MediaType.TV else []
        if self.end_season is not None:
            return list(range(self.begin_season, self.end_season + 1))
        return [self.begin_season]

    # ── 集属性 ───────────────────────────────────────────
    @property
    def episode(self) -> str:
        if self.begin_episode is not None:
            return (
                "E%s" % str(self.begin_episode).rjust(2, "0")
                if self.end_episode is None
                else "E%s-E%s" % (
                    str(self.begin_episode).rjust(2, "0"),
                    str(self.end_episode).rjust(2, "0"),
                )
            )
        return ""

    @property
    def episode_list(self) -> List[int]:
        if self.begin_episode is None:
            return []
        if self.end_episode is not None:
            return list(range(self.begin_episode, self.end_episode + 1))
        return [self.begin_episode]

    @property
    def episodes(self) -> str:
        return "E%s" % "E".join(str(ep).rjust(2, "0") for ep in self.episode_list)

    @property
    def episode_seqs(self) -> str:
        eps = self.episode_list
        if not eps:
            return ""
        return str(eps[0]) if len(eps) == 1 else "%s-%s" % (eps[0], eps[-1])

    @property
    def episode_seq(self) -> str:
        eps = self.episode_list
        return str(eps[0]) if eps else ""

    @property
    def season_episode(self) -> str:
        if self.type == MediaType.TV:
            s, e = self.season, self.episode
            if s and e:
                return "%s %s" % (s, e)
            return s or e
        return ""

    # ── 资源属性 ─────────────────────────────────────────
    @property
    def resource_term(self) -> str:
        parts = [self.resource_type, self.resource_effect, self.resource_pix]
        return " ".join(p for p in parts if p)

    @property
    def edition(self) -> str:
        parts = [self.resource_type, self.resource_effect]
        return " ".join(p for p in parts if p)

    @property
    def release_group(self) -> str:
        return self.resource_team or ""

    @property
    def video_term(self) -> str:
        return self.video_encode or ""

    @property
    def audio_term(self) -> str:
        return self.audio_encode or ""

    @property
    def frame_rate(self) -> Optional[int]:
        return self.fps or None

    # ── 包含判断 ─────────────────────────────────────────
    def is_in_season(self, season: Union[list, int, str]) -> bool:
        if isinstance(season, list):
            meta_s = (
                list(range(self.begin_season, self.end_season + 1))
                if self.end_season is not None
                else ([self.begin_season] if self.begin_season is not None else [1])
            )
            return set(meta_s).issuperset(set(season))
        if self.end_season is not None:
            return self.begin_season <= int(season) <= self.end_season
        return int(season) == (self.begin_season if self.begin_season is not None else 1)

    def is_in_episode(self, episode: Union[list, int, str]) -> bool:
        if isinstance(episode, list):
            meta_e = (
                list(range(self.begin_episode, self.end_episode + 1))
                if self.end_episode is not None
                else [self.begin_episode]
            )
            return set(meta_e).issuperset(set(episode))
        if self.end_episode is not None:
            return self.begin_episode <= int(episode) <= self.end_episode
        return int(episode) == self.begin_episode

    # ── Setter ───────────────────────────────────────────
    def set_season(self, sea: Union[list, int, str]):
        if not sea:
            return
        if isinstance(sea, list):
            if len(sea) == 1 and str(sea[0]).isdigit():
                self.begin_season = int(sea[0])
                self.end_season = None
            elif len(sea) > 1 and str(sea[0]).isdigit() and str(sea[-1]).isdigit():
                self.begin_season = int(sea[0])
                self.end_season = int(sea[-1])
        elif str(sea).isdigit():
            self.begin_season = int(sea)
            self.end_season = None

    def set_episode(self, ep: Union[list, int, str]):
        if not ep:
            return
        if isinstance(ep, list):
            if len(ep) == 1 and str(ep[0]).isdigit():
                self.begin_episode = int(ep[0])
                self.end_episode = None
            elif len(ep) > 1 and str(ep[0]).isdigit() and str(ep[-1]).isdigit():
                self.begin_episode = int(ep[0])
                self.end_episode = int(ep[-1])
                self.total_episode = (self.end_episode - self.begin_episode) + 1
        elif str(ep).isdigit():
            self.begin_episode = int(ep)
            self.end_episode = None

    def set_episodes(self, begin: int, end: int):
        if begin:
            self.begin_episode = begin
        if end:
            self.end_episode = end
        if self.begin_episode and self.end_episode:
            self.total_episode = (self.end_episode - self.begin_episode) + 1

    # ── Merge ────────────────────────────────────────────
    def merge(self, meta: "MetaBase"):
        if self.type == MediaType.UNKNOWN and meta.type != MediaType.UNKNOWN:
            self.type = meta.type
        if not self.name:
            self.cn_name = meta.cn_name
            self.en_name = meta.en_name
        if not self.year:
            self.year = meta.year
        if self.type == MediaType.TV and self.begin_season is None:
            self.begin_season = meta.begin_season
            self.end_season = meta.end_season
            self.total_season = meta.total_season
        if self.type == MediaType.TV and self.begin_episode is None:
            self.begin_episode = meta.begin_episode
            self.end_episode = meta.end_episode
            self.total_episode = meta.total_episode
        if not self.resource_type:
            self.resource_type = meta.resource_type
        if not self.resource_pix:
            self.resource_pix = meta.resource_pix
        if not self.resource_team:
            self.resource_team = meta.resource_team
        if not self.customization:
            self.customization = meta.customization
        if not self.resource_effect:
            self.resource_effect = meta.resource_effect
        if not self.video_encode:
            self.video_encode = meta.video_encode
        if not self.audio_encode:
            self.audio_encode = meta.audio_encode
        if not self.fps:
            self.fps = meta.fps
        if not self.part:
            self.part = meta.part
        if not self.tmdbid and meta.tmdbid:
            self.tmdbid = meta.tmdbid
        if not self.doubanid and meta.doubanid:
            self.doubanid = meta.doubanid

    # ── 副标题中文格式解析 ──────────────────────────────
    def init_subtitle(self, title_text: str):
        if not title_text:
            return
        title_text = f" {title_text} "
        if re.search(r"%s" % self._title_episodel_re, title_text, re.IGNORECASE):
            m = re.search(r"%s" % self._title_episodel_re, title_text, re.IGNORECASE)
            if m:
                try:
                    episode = int(m.group(1))
                except Exception:
                    return
                if episode >= 10000:
                    return
                if self.begin_episode is None:
                    self.begin_episode = episode
                    self.total_episode = 1
                self.type = MediaType.TV
                self._subtitle_flag = True
        elif re.search(r"[全第季集话話期幕]", title_text, re.IGNORECASE):
            # 全x季
            m = re.search(r"%s" % self._subtitle_season_all_re, title_text, re.IGNORECASE)
            if m:
                season_all = m.group(1) or m.group(2)
                if season_all and self.begin_season is None and self.begin_episode is None:
                    try:
                        self.total_season = int(cn2an.cn2an(season_all.strip(), mode="smart"))
                    except Exception:
                        return
                    self.begin_season = 1
                    self.end_season = self.total_season
                    self.type = MediaType.TV
                    self._subtitle_flag = True
                return
            # 第x季
            m = re.search(r"%s" % self._subtitle_season_re, title_text, re.IGNORECASE)
            if m:
                seasons = m.group(1)
                if seasons:
                    seasons = seasons.upper().replace("S", "").strip()
                else:
                    return
                try:
                    end_season = None
                    if "-" in seasons:
                        parts = seasons.split("-")
                        begin_season = int(cn2an.cn2an(parts[0].strip(), mode="smart"))
                        if len(parts) > 1:
                            end_season = int(cn2an.cn2an(parts[1].strip(), mode="smart"))
                    else:
                        begin_season = int(cn2an.cn2an(seasons, mode="smart"))
                except Exception:
                    return
                if begin_season and begin_season > 100:
                    return
                if end_season and end_season > 100:
                    return
                if self.begin_season is None and isinstance(begin_season, int):
                    self.begin_season = begin_season
                    self.total_season = 1
                if (self.begin_season is not None and self.end_season is None
                        and isinstance(end_season, int) and end_season != self.begin_season):
                    self.end_season = end_season
                    self.total_season = (self.end_season - self.begin_season) + 1
                self.type = MediaType.TV
                self._subtitle_flag = True
            # 第x-x集
            m = re.search(r"%s" % self._subtitle_episode_between_re, title_text, re.IGNORECASE)
            if m:
                groups = m.groups()
                if groups:
                    begin_ep, end_ep = groups[0], groups[1]
                else:
                    return
                try:
                    begin_ep = int(cn2an.cn2an(begin_ep.strip(), mode="smart"))
                    end_ep = int(cn2an.cn2an(end_ep.strip(), mode="smart"))
                except Exception:
                    return
                if begin_ep and begin_ep >= 10000:
                    return
                if end_ep and end_ep >= 10000:
                    return
                if self.begin_episode is None and isinstance(begin_ep, int):
                    self.begin_episode = begin_ep
                    self.total_episode = 1
                if (self.begin_episode is not None and self.end_episode is None
                        and isinstance(end_ep, int) and end_ep != self.begin_episode):
                    self.end_episode = end_ep
                    self.total_episode = (self.end_episode - self.begin_episode) + 1
                self.type = MediaType.TV
                self._subtitle_flag = True
                return
            # 第x集
            m = re.search(r"%s" % self._subtitle_episode_re, title_text, re.IGNORECASE)
            if m:
                episodes = m.group(1)
                if episodes:
                    episodes = episodes.upper().replace("E", "").replace("P", "").strip()
                else:
                    return
                try:
                    end_ep = None
                    if "-" in episodes:
                        parts = episodes.split("-")
                        begin_ep = int(cn2an.cn2an(parts[0].strip(), mode="smart"))
                        if len(parts) > 1:
                            end_ep = int(cn2an.cn2an(parts[1].strip(), mode="smart"))
                    else:
                        begin_ep = int(cn2an.cn2an(episodes, mode="smart"))
                except Exception:
                    return
                if begin_ep and begin_ep >= 10000:
                    return
                if end_ep and end_ep >= 10000:
                    return
                if self.begin_episode is None and isinstance(begin_ep, int):
                    self.begin_episode = begin_ep
                    self.total_episode = 1
                if (self.begin_episode is not None and self.end_episode is None
                        and isinstance(end_ep, int) and end_ep != self.begin_episode):
                    self.end_episode = end_ep
                    self.total_episode = (self.end_episode - self.begin_episode) + 1
                self.type = MediaType.TV
                self._subtitle_flag = True
                return
            # x集全
            m = re.search(r"%s" % self._subtitle_episode_all_re, title_text, re.IGNORECASE)
            if m:
                episode_all = m.group(1) or m.group(2)
                if episode_all and self.begin_episode is None:
                    try:
                        self.total_episode = int(cn2an.cn2an(episode_all.strip(), mode="smart"))
                    except Exception:
                        return
                    self.type = MediaType.TV
                    self._subtitle_flag = True

    # ── 序列化 ───────────────────────────────────────────
    def to_dict(self) -> dict:
        d = vars(self).copy()
        d["type"] = self.type.value if self.type else None
        d["season_episode"] = self.season_episode
        d["edition"] = self.edition
        d["name"] = self.name
        d["episode_list"] = self.episode_list
        return d

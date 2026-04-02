"""
MetaAnime —— 动漫文件名解析器。
来源：MoviePilot app/core/meta/metaanime.py（移除 app.* 依赖）
"""
import re
import logging
import traceback
from typing import Optional

import anitopy
import zhconv

from mediaparser.meta_base import MetaBase
from mediaparser.release_group import ReleaseGroupsMatcher
from mediaparser.string_utils import StringUtils
from mediaparser.types import MediaType

logger = logging.getLogger(__name__)


class MetaAnime(MetaBase):
    """识别动漫"""

    _anime_no_words = ["CHS&CHT", "MP4", "GB MP4", "WEB-DL"]
    _name_nostring_re = (
        r"S\d{2}\s*-\s*S\d{2}|S\d{2}|\s+S\d{1,2}|EP?\d{2,4}\s*-\s*EP?\d{2,4}"
        r"|EP?\d{2,4}|\s+EP?\d{1,4}|\s+GB"
    )
    _fps_re = r"(\d{2,3})(?=FPS)"

    def __init__(
        self,
        title: str,
        subtitle: str = None,
        isfile: bool = False,
        release_group_matcher: Optional[ReleaseGroupsMatcher] = None,
    ):
        super().__init__(title, subtitle, isfile)
        if not title:
            return
        original_title = title
        try:
            anitopy_info_origin = anitopy.parse(title)
            title = self.__prepare_title(title)
            anitopy_info = anitopy.parse(title)
            if anitopy_info:
                # 名称
                name = anitopy_info.get("anime_title")
                if not name or name in self._anime_no_words or (len(name) < 5 and not StringUtils.is_chinese(name)):
                    anitopy_info = anitopy.parse("[ANIME]" + title)
                    if anitopy_info:
                        name = anitopy_info.get("anime_title")
                if not name or name in self._anime_no_words or (len(name) < 5 and not StringUtils.is_chinese(name)):
                    nm = re.search(r"\[(.+?)]", title)
                    if nm and nm.group(1):
                        name = nm.group(1).strip()
                if name:
                    _split_flag = True
                    if name.find("/") != -1:
                        parts = name.split("/")
                        if StringUtils.is_chinese(parts[0]):
                            self.cn_name = parts[0]
                            self.en_name = parts[1] if len(parts) > 1 else None
                            _split_flag = False
                        elif StringUtils.is_chinese(parts[-1]):
                            self.cn_name = parts[-1]
                            self.en_name = parts[0] if len(parts) > 1 else None
                            _split_flag = False
                        else:
                            name = parts[-1]
                    if _split_flag:
                        last_type = ""
                        for word in name.split():
                            if not word:
                                continue
                            if word.endswith("]"):
                                word = word[:-1]
                            if word.isdigit():
                                if last_type == "cn":
                                    self.cn_name = "%s %s" % (self.cn_name or "", word)
                                elif last_type == "en":
                                    self.en_name = "%s %s" % (self.en_name or "", word)
                            elif StringUtils.is_chinese(word):
                                self.cn_name = "%s %s" % (self.cn_name or "", word)
                                last_type = "cn"
                            else:
                                self.en_name = "%s %s" % (self.en_name or "", word)
                                last_type = "en"
                if self.cn_name:
                    _, self.cn_name, _, _, _, _ = StringUtils.get_keyword(self.cn_name)
                    if self.cn_name:
                        self.cn_name = re.sub(r"%s" % self._name_nostring_re, "", self.cn_name, flags=re.IGNORECASE).strip()
                if self.en_name:
                    self.en_name = re.sub(r"%s" % self._name_nostring_re, "", self.en_name, flags=re.IGNORECASE).strip().title()
                # 年份
                yr = anitopy_info.get("anime_year")
                if str(yr).isdigit():
                    self.year = str(yr)
                # 季号
                anime_season = anitopy_info.get("anime_season")
                if isinstance(anime_season, list):
                    begin_s = anime_season[0]
                    end_s = anime_season[-1] if len(anime_season) > 1 else None
                elif anime_season:
                    begin_s, end_s = anime_season, None
                else:
                    begin_s = end_s = None
                if begin_s:
                    self.begin_season = int(begin_s)
                    if end_s and int(end_s) != self.begin_season:
                        self.end_season = int(end_s)
                        self.total_season = (self.end_season - self.begin_season) + 1
                    else:
                        self.total_season = 1
                    self.type = MediaType.TV
                # 集号
                episode_number = anitopy_info.get("episode_number")
                if isinstance(episode_number, list):
                    begin_e = episode_number[0]
                    end_e = episode_number[-1] if len(episode_number) > 1 else None
                elif episode_number:
                    begin_e, end_e = episode_number, None
                else:
                    begin_e = end_e = None
                if begin_e:
                    try:
                        self.begin_episode = int(begin_e)
                        if end_e and int(end_e) != self.begin_episode:
                            self.end_episode = int(end_e)
                            self.total_episode = (self.end_episode - self.begin_episode) + 1
                        else:
                            self.total_episode = 1
                    except Exception as err:
                        logger.debug(f"解析集数失败：{err} - {traceback.format_exc()}")
                        self.begin_episode = None
                        self.end_episode = None
                    self.type = MediaType.TV
                # 类型
                if not self.type:
                    anime_type = anitopy_info.get("anime_type")
                    if isinstance(anime_type, list):
                        anime_type = anime_type[0]
                    if anime_type and anime_type.upper() == "TV":
                        self.type = MediaType.TV
                    else:
                        self.type = MediaType.MOVIE
                # 分辨率
                self.resource_pix = anitopy_info.get("video_resolution")
                if isinstance(self.resource_pix, list):
                    self.resource_pix = self.resource_pix[0]
                if self.resource_pix:
                    if re.search(r"x", self.resource_pix, re.IGNORECASE):
                        self.resource_pix = re.split(r"[Xx]", self.resource_pix)[-1] + "p"
                    else:
                        self.resource_pix = self.resource_pix.lower()
                    if str(self.resource_pix).isdigit():
                        self.resource_pix = f"{self.resource_pix}p"
                # 字幕组
                _matcher = release_group_matcher or ReleaseGroupsMatcher()
                self.resource_team = (
                    _matcher.match(title=original_title)
                    or anitopy_info_origin.get("release_group")
                    or None
                )
                # 视频/音频编码
                self.video_encode = anitopy_info.get("video_term")
                if isinstance(self.video_encode, list):
                    self.video_encode = self.video_encode[0]
                self.audio_encode = anitopy_info.get("audio_term")
                if isinstance(self.audio_encode, list):
                    self.audio_encode = self.audio_encode[0]
                # 帧率
                self.__init_anime_fps(original_title)
                # 副标题
                self.init_subtitle(self.org_string)
                if not self._subtitle_flag and self.subtitle:
                    self.init_subtitle(self.subtitle)
            if not self.type:
                self.type = MediaType.TV
        except Exception as e:
            logger.error(f"解析动漫信息失败：{e} - {traceback.format_exc()}")

    def __init_anime_fps(self, original_title: str):
        res = re.search(rf"({self._fps_re})", original_title, re.IGNORECASE)
        if res and res.group(1) and res.group(1).isdigit():
            self.fps = int(res.group(1))

    @staticmethod
    def __prepare_title(title: str) -> str:
        if not title:
            return title
        title = title.replace("【", "[").replace("】", "]").strip()
        match = re.search(r"新番|月?番|[日美国][漫剧]", title)
        if match and match.span()[1] < len(title) - 1:
            title = re.sub(r".*番.|.*[日美国][漫剧].", "", title)
        elif match:
            title = title[: title.rfind("[")]
        first_item = title.split("]")[0]
        if first_item and re.search(
            r"[动漫画纪录片电影视连续剧集日美韩中港台海外亚洲华语大陆综艺原盘高清]{2,}|TV|Animation|Movie|Documentar|Anime",
            zhconv.convert(first_item, "zh-hans"),
            re.IGNORECASE,
        ):
            title = re.sub(r"^[^]]*]", "", title).strip()
        title = re.sub(r"[0-9.]+\s*[MGT]i?B(?![A-Z]+)", "", title, flags=re.IGNORECASE)
        title = re.sub(r"\[TV\s+(\d{1,4})", r"[\1", title, flags=re.IGNORECASE)
        title = re.sub(r"\[4k]", "2160p", title, flags=re.IGNORECASE)
        names = title.split("]")
        if len(names) > 1 and title.find("- ") == -1:
            titles = []
            for name in names:
                if not name:
                    continue
                lc = ""
                if name.startswith("["):
                    lc = "["
                    name = name[1:]
                if name and name.find("/") != -1:
                    last = name.split("/")[-1].strip()
                    titles.append("%s%s" % (lc, last if last else name.split("/")[0].strip()))
                elif name:
                    if StringUtils.is_chinese(name) and not StringUtils.is_all_chinese(name):
                        if not re.search(r"\[\d+", name, re.IGNORECASE):
                            name = re.sub(r"[\d|#:：\-()（）\u4e00-\u9fff]", "", name).strip()
                        if not name or name.strip().isdigit():
                            continue
                    titles.append("%s%s" % (lc, name.strip()) if name != "[" else "")
            return "]".join(titles)
        return title

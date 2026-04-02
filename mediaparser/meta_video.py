"""
MetaVideo —— 普通影视文件名/种子名解析器。
来源：MoviePilot app/core/meta/metavideo.py（移除 app.* 依赖）
"""
import re
from typing import Optional

from Pinyin2Hanzi import is_pinyin

from mediaparser.meta_base import MetaBase, MEDIA_EXTS
from mediaparser.release_group import ReleaseGroupsMatcher
from mediaparser.string_utils import StringUtils
from mediaparser.streaming_platform import StreamingPlatforms
from mediaparser.tokens import Tokens
from mediaparser.types import MediaType


class MetaVideo(MetaBase):
    """识别电影、电视剧"""

    _stop_name_flag = False
    _stop_cnname_flag = False
    _last_token = ""
    _last_token_type = ""
    _continue_flag = True
    _unknown_name_str = ""
    _source = ""
    _effect: list = []

    # ── 正则 ─────────────────────────────────────────────
    _season_re = r"S(\d{3})|^S(\d{1,3})$|S(\d{1,3})E"
    _episode_re = r"EP?(\d{2,4})$|^EP?(\d{1,4})$|^S\d{1,2}EP?(\d{1,4})$|S\d{2}EP?(\d{2,4})"
    _part_re = r"(^PART[0-9ABI]{0,2}$|^CD[0-9]{0,2}$|^DVD[0-9]{0,2}$|^DISK[0-9]{0,2}$|^DISC[0-9]{0,2}$)"
    _roman_numerals = r"^(?=[MDCLXVI])M*(C[MD]|D?C{0,3})(X[CL]|L?X{0,3})(I[XV]|V?I{0,3})$"
    _source_re = r"^BLURAY$|^HDTV$|^UHDTV$|^HDDVD$|^WEBRIP$|^DVDRIP$|^BDRIP$|^BLU$|^WEB$|^BD$|^HDRip$|^REMUX$|^UHD$"
    _effect_re = r"^SDR$|^HDR\d*$|^DOLBY$|^DOVI$|^DV$|^3D$|^REPACK$|^HLG$|^HDR10(\+|Plus)$|^EDR$|^HQ$"
    _resources_type_re = r"%s|%s" % (_source_re, _effect_re)
    _name_no_begin_re = r"^[\[【].+?[\]】]"
    _name_no_chinese_re = r".*版|.*字幕"
    _name_se_words = ["共", "第", "季", "集", "话", "話", "期"]
    _name_movie_words = ["剧场版", "劇場版", "电影版", "電影版"]
    _name_nostring_re = (
        r"^PTS|^JADE|^AOD|^CHC|^[A-Z]{1,4}TV[\-0-9UVHDK]*"
        r"|HBO$|\s+HBO|\d{1,2}th|\d{1,2}bit|NETFLIX|AMAZON|IMAX|^3D|\s+3D|^BBC\s+|\s+BBC|BBC$|DISNEY\+?|XXX|\s+DC$"
        r"|[第\s共]+[0-9一二三四五六七八九十\-\s]+季"
        r"|[第\s共]+[0-9一二三四五六七八九十百零\-\s]+[集话話]"
        r"|连载|日剧|美剧|电视剧|动画片|动漫|欧美|西德|日韩|超高清|高清|无水印|下载|蓝光|翡翠台|梦幻天堂·龙网|★?\d*月?新番"
        r"|最终季|合集|[多中国英葡法俄日韩德意西印泰台港粤双文语简繁体特效内封官译外挂]+字幕|版本|出品|台版|港版|\w+字幕组|\w+字幕社"
        r"|未删减版|UNCUT$|UNRATE$|WITH EXTRAS$|RERIP$|SUBBED$|PROPER$|REPACK$|SEASON$|EPISODE$|Complete$|Extended$|Extended Version$"
        r"|S\d{2}\s*-\s*S\d{2}|S\d{2}|\s+S\d{1,2}|EP?\d{2,4}\s*-\s*EP?\d{2,4}|EP?\d{2,4}|\s+EP?\d{1,4}"
        r"|CD[\s.]*[1-9]|DVD[\s.]*[1-9]|DISK[\s.]*[1-9]|DISC[\s.]*[1-9]"
        r"|[248]K|\d{3,4}[PIX]+"
        r"|CD[\s.]*[1-9]|DVD[\s.]*[1-9]|DISK[\s.]*[1-9]|DISC[\s.]*[1-9]|\s+GB"
    )
    _resources_pix_re = r"^[SBUHD]*(\d{3,4}[PI]+)|\d{3,4}X(\d{3,4})"
    _resources_pix_re2 = r"(^[248]+K)"
    _video_encode_re = r"^(H26[45])$|^(x26[45])$|^AVC$|^HEVC$|^VC\d?$|^MPEG\d?$|^Xvid$|^DivX$|^AV1$|^HDR\d*$|^AVS(\+|[23])$"
    _audio_encode_re = r"^DTS\d?$|^DTSHD$|^DTSHDMA$|^Atmos$|^TrueHD\d?$|^AC3$|^\dAudios?$|^DDP\d?$|^DD\+\d?$|^DD\d?$|^LPCM\d?$|^AAC\d?$|^FLAC\d?$|^HD\d?$|^MA\d?$|^HR\d?$|^Opus\d?$|^Vorbis\d?$|^AV[3S]A$"
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
        self._source = ""
        self._effect = []
        self._index = 0
        self._stop_name_flag = False
        self._stop_cnname_flag = False
        self._last_token = ""
        self._last_token_type = ""
        self._continue_flag = True
        self._unknown_name_str = ""

        # 纯数字文件名
        if isfile and title.isdigit() and len(title) < 5:
            self.begin_episode = int(title)
            self.type = MediaType.TV
            return

        # Season xx / Sxx 格式直接返回
        season_full = re.search(r"^(?:Season\s+|S)(\d{1,3})$", title, re.IGNORECASE)
        if season_full:
            self.type = MediaType.TV
            self.begin_season = int(season_full.group(1))
            self.total_season = 1
            return

        # 预处理
        title = re.sub(r"%s" % self._name_no_begin_re, "", title, count=1)
        title = re.sub(r"([\s.]+)(\d{4})-(\d{4})", r"\1\2", title)
        title = re.sub(r"[0-9.]+\s*[MGT]i?B(?![A-Z]+)", "", title, flags=re.IGNORECASE)
        title = re.sub(r"\d{4}[\s._-]\d{1,2}[\s._-]\d{1,2}", "", title)

        tokens = Tokens(title)
        streaming_platforms = StreamingPlatforms()

        token = tokens.get_next()
        while token:
            self._index += 1
            self.__init_part(token, tokens)
            if self._continue_flag:
                self.__init_name(token)
            if self._continue_flag:
                self.__init_year(token)
            if self._continue_flag:
                self.__init_resource_pix(token)
            if self._continue_flag:
                self.__init_season(token)
            if self._continue_flag:
                self.__init_episode(token)
            if self._continue_flag:
                self.__init_resource_type(token)
            if self._continue_flag:
                self.__init_web_source(token, tokens, streaming_platforms)
            if self._continue_flag:
                self.__init_video_encode(token)
            if self._continue_flag:
                self.__init_audio_encode(token)
            if self._continue_flag:
                self.__init_fps(token)
            token = tokens.get_next()
            self._continue_flag = True

        if self._effect:
            self._effect.reverse()
            self.resource_effect = " ".join(self._effect)
        if self._source:
            self.resource_type = self._source.strip()

        # 原盘 DIY
        if self.resource_type and "BluRay" in self.resource_type:
            if (self.subtitle and re.findall(r"D[Ii]Y", self.subtitle)) or re.findall(r"-D[Ii]Y@", original_title):
                self.resource_type = f"{self.resource_type} DIY"

        # 副标题
        self.init_subtitle(self.org_string)
        if not self._subtitle_flag and self.subtitle:
            self.init_subtitle(self.subtitle)

        # 清理名称
        self.cn_name = self.__fix_name(self.cn_name)
        self.en_name = StringUtils.str_title(self.__fix_name(self.en_name))

        if self.part and self.part.upper() == "PART":
            self.part = None

        # 拼音→中文
        if not self.cn_name and self.en_name and self.subtitle:
            if self.__is_pinyin(self.en_name):
                cn = self.__get_title_from_description(self.subtitle)
                if cn and len(cn) == len(self.en_name.split()):
                    self.cn_name = cn

        # 字幕组
        _matcher = release_group_matcher or ReleaseGroupsMatcher()
        self.resource_team = _matcher.match(title=original_title) or None

    # ── 私有解析方法 ─────────────────────────────────────

    @staticmethod
    def __get_title_from_description(description: str) -> Optional[str]:
        if not description:
            return None
        titles = re.split(r"[\s/|]+", description)
        if StringUtils.is_chinese(titles[0]):
            return titles[0]
        return None

    @staticmethod
    def __is_pinyin(name_str: Optional[str]) -> bool:
        if not name_str:
            return False
        for n in name_str.lower().split():
            if not is_pinyin(n):
                return False
        return True

    def __fix_name(self, name: Optional[str]) -> Optional[str]:
        if not name:
            return name
        name = re.sub(r"%s" % self._name_nostring_re, "", name, flags=re.IGNORECASE).strip()
        name = re.sub(r"\s+", " ", name)
        if (
            name.isdecimal()
            and int(name) < 1800
            and not self.year
            and not self.begin_season
            and not self.resource_pix
            and not self.resource_type
            and not self.audio_encode
            and not self.video_encode
        ):
            if self.begin_episode is None:
                self.begin_episode = int(name)
                name = None
            elif self.is_in_episode(int(name)) and not self.begin_season:
                name = None
        return name

    def __init_name(self, token: Optional[str]):
        if not token:
            return
        if self._unknown_name_str:
            if not self.cn_name:
                if not self.en_name:
                    self.en_name = self._unknown_name_str
                elif self._unknown_name_str != self.year:
                    self.en_name = "%s %s" % (self.en_name, self._unknown_name_str)
                self._last_token_type = "enname"
            self._unknown_name_str = ""
        if self._stop_name_flag:
            return
        if token.upper() == "AKA":
            self._continue_flag = False
            self._stop_name_flag = True
            return
        if token in self._name_se_words:
            self._last_token_type = "name_se_words"
            return
        if StringUtils.is_chinese(token):
            self._last_token_type = "cnname"
            if not self.cn_name:
                self.cn_name = token
            elif not self._stop_cnname_flag:
                if re.search(r"%s" % "|".join(self._name_movie_words), token, flags=re.IGNORECASE) or (
                    not re.search(r"%s" % self._name_no_chinese_re, token, flags=re.IGNORECASE)
                    and not re.search(r"%s" % "|".join(self._name_se_words), token, flags=re.IGNORECASE)
                ):
                    self.cn_name = "%s %s" % (self.cn_name, token)
                self._stop_cnname_flag = True
        else:
            is_roman = re.search(self._roman_numerals, token)
            if token.isdigit() or is_roman:
                if self._last_token_type == "name_se_words":
                    return
                if self.name:
                    if token.startswith("0"):
                        return
                    if token.isdigit():
                        try:
                            int(token)
                        except ValueError:
                            return
                    if not is_roman and self._last_token_type == "cnname" and int(token) < 1900:
                        return
                    if (token.isdigit() and len(token) < 4) or is_roman:
                        if self._last_token_type == "cnname":
                            self.cn_name = "%s %s" % (self.cn_name, token)
                        elif self._last_token_type == "enname":
                            self.en_name = "%s %s" % (self.en_name, token)
                        self._continue_flag = False
                    elif token.isdigit() and len(token) == 4:
                        if not self._unknown_name_str:
                            self._unknown_name_str = token
                else:
                    if not self._unknown_name_str:
                        self._unknown_name_str = token
            elif re.search(r"%s" % self._season_re, token, re.IGNORECASE):
                if self.en_name and re.search(r"SEASON$", self.en_name, re.IGNORECASE):
                    self.en_name += " "
                self._stop_name_flag = True
                return
            elif (
                re.search(r"%s" % self._episode_re, token, re.IGNORECASE)
                or re.search(r"(%s)" % self._resources_type_re, token, re.IGNORECASE)
                or re.search(r"%s" % self._resources_pix_re, token, re.IGNORECASE)
            ):
                self._stop_name_flag = True
                return
            else:
                if ".%s".lower() % token in MEDIA_EXTS:
                    return
                if self.en_name:
                    self.en_name = "%s %s" % (self.en_name, token)
                else:
                    self.en_name = token
                self._last_token_type = "enname"

    def __init_part(self, token: str, tokens: Tokens):
        if not self.name:
            return
        if not any([self.year, self.begin_season, self.begin_episode, self.resource_pix, self.resource_type]):
            return
        m = re.search(r"%s" % self._part_re, token, re.IGNORECASE)
        if m:
            if not self.part:
                self.part = m.group(1)
            nxt = tokens.cur()
            if nxt and (
                (nxt.isdigit() and (len(nxt) == 1 or (len(nxt) == 2 and nxt.startswith("0"))))
                or nxt.upper() in ["A", "B", "C", "I", "II", "III"]
            ):
                self.part = "%s%s" % (self.part, nxt)
                tokens.get_next()
            self._last_token_type = "part"
            self._continue_flag = False

    def __init_year(self, token: str):
        if not self.name:
            return
        if not token.isdigit() or len(token) != 4 or not (1900 < int(token) < 2050):
            return
        if self.year:
            if self.en_name:
                self.en_name = "%s %s" % (self.en_name.strip(), self.year)
            elif self.cn_name:
                self.cn_name = "%s %s" % (self.cn_name, self.year)
        elif self.en_name and re.search(r"SEASON$", self.en_name, re.IGNORECASE):
            self.en_name += " "
        self.year = token
        self._last_token_type = "year"
        self._continue_flag = False
        self._stop_name_flag = True

    def __init_resource_pix(self, token: str):
        if not self.name:
            return
        res = re.findall(r"%s" % self._resources_pix_re, token, re.IGNORECASE)
        if res:
            self._last_token_type = "pix"
            self._continue_flag = False
            self._stop_name_flag = True
            pix = None
            for pixs in res:
                if isinstance(pixs, tuple):
                    pix = next((p for p in pixs if p), None)
                else:
                    pix = pixs
                if pix and not self.resource_pix:
                    self.resource_pix = pix.lower()
                    break
            if self.resource_pix and self.resource_pix.isdigit() and self.resource_pix[-1] not in "kpi":
                self.resource_pix = "%sp" % self.resource_pix
        else:
            res2 = re.search(r"%s" % self._resources_pix_re2, token, re.IGNORECASE)
            if res2:
                self._last_token_type = "pix"
                self._continue_flag = False
                self._stop_name_flag = True
                if not self.resource_pix:
                    self.resource_pix = res2.group(1).lower()

    def __init_season(self, token: str):
        res = re.findall(r"%s" % self._season_re, token, re.IGNORECASE)
        if res:
            self._last_token_type = "season"
            self.type = MediaType.TV
            self._stop_name_flag = True
            self._continue_flag = True
            for se in res:
                if isinstance(se, tuple):
                    se_t = next((s for s in se if s and str(s).isdigit()), None)
                    if se_t:
                        se = int(se_t)
                    else:
                        break
                else:
                    se = int(se)
                if self.begin_season is None:
                    self.begin_season = se
                    self.total_season = 1
                elif se > self.begin_season:
                    self.end_season = se
                    self.total_season = (self.end_season - self.begin_season) + 1
                    if self.isfile and self.total_season > 1:
                        self.end_season = None
                        self.total_season = 1
        elif token.isdigit():
            try:
                int(token)
            except ValueError:
                return
            if self._last_token_type == "SEASON" and self.begin_season is None and len(token) < 3:
                self.begin_season = int(token)
                self.total_season = 1
                self._last_token_type = "season"
                self._stop_name_flag = True
                self._continue_flag = False
                self.type = MediaType.TV
        elif token.upper() == "SEASON" and self.begin_season is None:
            self._last_token_type = "SEASON"
        elif self.type == MediaType.TV and self.begin_season is None:
            self.begin_season = 1

    def __init_episode(self, token: str):
        res = re.findall(r"%s" % self._episode_re, token, re.IGNORECASE)
        if res:
            self._last_token_type = "episode"
            self._continue_flag = False
            self._stop_name_flag = True
            self.type = MediaType.TV
            for se in res:
                if isinstance(se, tuple):
                    se_t = next((s for s in se if s and str(s).isdigit()), None)
                    if se_t:
                        se = int(se_t)
                    else:
                        break
                else:
                    se = int(se)
                if self.begin_episode is None:
                    self.begin_episode = se
                    self.total_episode = 1
                elif se > self.begin_episode:
                    self.end_episode = se
                    self.total_episode = (self.end_episode - self.begin_episode) + 1
                    if self.isfile and self.total_episode > 2:
                        self.end_episode = None
                        self.total_episode = 1
        elif token.isdigit():
            try:
                int(token)
            except ValueError:
                return
            if (
                self.begin_episode is not None
                and self.end_episode is None
                and len(token) < 5
                and int(token) > self.begin_episode
                and self._last_token_type == "episode"
            ):
                self.end_episode = int(token)
                self.total_episode = (self.end_episode - self.begin_episode) + 1
                if self.isfile and self.total_episode > 2:
                    self.end_episode = None
                    self.total_episode = 1
                self._continue_flag = False
                self.type = MediaType.TV
            elif (
                self.begin_episode is None
                and 1 < len(token) < 4
                and self._last_token_type not in ("year", "videoencode")
                and token != self._unknown_name_str
            ):
                self.begin_episode = int(token)
                self.total_episode = 1
                self._last_token_type = "episode"
                self._continue_flag = False
                self._stop_name_flag = True
                self.type = MediaType.TV
            elif self._last_token_type == "EPISODE" and self.begin_episode is None and len(token) < 5:
                self.begin_episode = int(token)
                self.total_episode = 1
                self._last_token_type = "episode"
                self._continue_flag = False
                self._stop_name_flag = True
                self.type = MediaType.TV
        elif token.upper() == "EPISODE":
            self._last_token_type = "EPISODE"

    def __init_resource_type(self, token: str):
        if not self.name:
            return
        if token.upper() == "DL" and self._last_token_type == "source" and self._last_token == "WEB":
            self._source = "WEB-DL"
            self._continue_flag = False
            return
        if token.upper() == "RAY" and self._last_token_type == "source" and self._last_token == "BLU":
            self._source = "UHD BluRay" if self._source == "UHD" else "BluRay"
            self._continue_flag = False
            return
        if token.upper() == "WEBDL":
            self._source = "WEB-DL"
            self._continue_flag = False
            return
        if token.upper() == "REMUX" and self._source == "BluRay":
            self._source = "BluRay REMUX"
            self._continue_flag = False
            return
        if token.upper() == "BLURAY" and self._source == "UHD":
            self._source = "UHD BluRay"
            self._continue_flag = False
            return
        src = re.search(r"(%s)" % self._source_re, token, re.IGNORECASE)
        if src:
            self._last_token_type = "source"
            self._continue_flag = False
            self._stop_name_flag = True
            if not self._source:
                self._source = src.group(1)
                self._last_token = self._source.upper()
            return
        eff = re.search(r"(%s)" % self._effect_re, token, re.IGNORECASE)
        if eff:
            self._last_token_type = "effect"
            self._continue_flag = False
            self._stop_name_flag = True
            effect = eff.group(1)
            if effect not in self._effect:
                self._effect.append(effect)
            self._last_token = effect.upper()

    def __init_web_source(self, token: str, tokens: Tokens, streaming_platforms: StreamingPlatforms):
        if not self.name:
            return
        platform_name = None
        query_range = 1
        prev_token = tokens.tokens[self._index - 2] if 0 <= self._index - 2 < len(tokens.tokens) else None
        next_token = tokens.peek()

        if streaming_platforms.is_streaming_platform(token):
            platform_name = streaming_platforms.get_streaming_platform_name(token)
        else:
            for adjacent_token, is_next in [(prev_token, False), (next_token, True)]:
                if not adjacent_token or platform_name:
                    continue
                for separator in [" ", "-"]:
                    combined = f"{token}{separator}{adjacent_token}" if is_next else f"{adjacent_token}{separator}{token}"
                    if streaming_platforms.is_streaming_platform(combined):
                        platform_name = streaming_platforms.get_streaming_platform_name(combined)
                        query_range = 2
                        if is_next:
                            tokens.get_next()
                        break

        if not platform_name:
            return

        web_tokens = {"WEB", "DL", "WEBDL", "WEBRIP"}
        start_index = max(0, self._index - query_range - query_range)
        end_index = min(len(tokens.tokens), self._index - 1 + query_range + 1)
        if any(t and t.upper() in web_tokens for t in tokens.tokens[start_index:end_index]):
            self.web_source = platform_name
            self._continue_flag = False

    def __init_video_encode(self, token: str):
        if not self.name:
            return
        if not any([self.year, self.resource_pix, self.resource_type, self.begin_season, self.begin_episode]):
            return
        res = re.search(r"(%s)" % self._video_encode_re, token, re.IGNORECASE)
        if res:
            self._continue_flag = False
            self._stop_name_flag = True
            self._last_token_type = "videoencode"
            if not self.video_encode:
                if res.group(2):
                    self.video_encode = res.group(2).upper()
                elif res.group(3):
                    self.video_encode = res.group(3).lower()
                else:
                    self.video_encode = res.group(1).upper()
                self._last_token = self.video_encode
            elif self.video_encode == "10bit":
                self.video_encode = f"{res.group(1).upper()} 10bit"
                self._last_token = res.group(1).upper()
        elif token.upper() in ("H", "X"):
            self._continue_flag = False
            self._stop_name_flag = True
            self._last_token_type = "videoencode"
            self._last_token = token.upper() if token.upper() == "H" else token.lower()
        elif token in ("264", "265") and self._last_token_type == "videoencode" and self._last_token in ("H", "X"):
            self.video_encode = "%s%s" % (self._last_token, token)
        elif token.isdigit() and self._last_token_type == "videoencode" and self._last_token in ("VC", "MPEG"):
            self.video_encode = "%s%s" % (self._last_token, token)
        elif token.upper() == "10BIT":
            self._last_token_type = "videoencode"
            if not self.video_encode:
                self.video_encode = "10bit"
            else:
                self.video_encode = f"{self.video_encode} 10bit"

    def __init_audio_encode(self, token: str):
        if not self.name:
            return
        if not any([self.year, self.resource_pix, self.resource_type, self.begin_season, self.begin_episode]):
            return
        res = re.search(r"(%s)" % self._audio_encode_re, token, re.IGNORECASE)
        if res:
            self._continue_flag = False
            self._stop_name_flag = True
            self._last_token_type = "audioencode"
            self._last_token = res.group(1).upper()
            if not self.audio_encode:
                self.audio_encode = res.group(1)
            else:
                sep = "-" if self.audio_encode.upper() == "DTS" else " "
                self.audio_encode = f"{self.audio_encode}{sep}{res.group(1)}"
        elif token.isdigit() and self._last_token_type == "audioencode":
            if self.audio_encode:
                if self._last_token.isdigit():
                    self.audio_encode = "%s.%s" % (self.audio_encode, token)
                elif self.audio_encode[-1].isdigit():
                    self.audio_encode = "%s %s.%s" % (self.audio_encode[:-1], self.audio_encode[-1], token)
                else:
                    self.audio_encode = "%s %s" % (self.audio_encode, token)
            self._last_token = token

    def __init_fps(self, token: str):
        if not self.name:
            return
        res = re.search(rf"({self._fps_re})", token, re.IGNORECASE)
        if res and res.group(1) and res.group(1).isdigit():
            self._continue_flag = False
            self._stop_name_flag = True
            self._last_token_type = "fps"
            self.fps = int(res.group(1))
            self._last_token = f"{self.fps}FPS"

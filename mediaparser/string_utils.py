"""
StringUtils —— 仅保留解析器实际用到的字符串工具方法。
来源：MoviePilot app/utils/string.py（精简版）
"""
import re
from typing import Optional, Union

import cn2an


class StringUtils:

    @staticmethod
    def is_chinese(word: Union[str, list]) -> bool:
        """判断是否含有中文"""
        if not word:
            return False
        if isinstance(word, list):
            word = " ".join(word)
        return bool(re.compile(r"[\u4e00-\u9fff]").search(word))

    @staticmethod
    def is_all_chinese(word: str) -> bool:
        """判断是否全是中文（空格忽略）"""
        for ch in word:
            if ch == " ":
                continue
            if "\u4e00" <= ch <= "\u9fff":
                continue
            return False
        return True

    @staticmethod
    def is_japanese(word: str) -> bool:
        return bool(re.compile(r"[\u3040-\u309F\u30A0-\u30FF]").search(word))

    @staticmethod
    def str_title(s: Optional[str]) -> Optional[str]:
        """大写首字母兼容 None"""
        return s.title() if s else s

    @staticmethod
    def get_keyword(content: str):
        """
        从搜索关键字中拆分年份、季、集、类型。
        返回 (mtype, key_word, season_num, episode_num, year, content)
        """
        from mediaparser.types import MediaType
        if not content:
            return None, None, None, None, None, None

        mtype = MediaType.TV if re.search(r"^(电视剧|动漫|\s+电视剧|\s+动漫)", content) else None
        content = re.sub(r"^(电影|电视剧|动漫|\s+电影|\s+电视剧|\s+动漫)", "", content).strip()

        season_num = None
        episode_num = None
        season_re = re.search(r"第\s*([0-9一二三四五六七八九十]+)\s*季", content, re.IGNORECASE)
        if season_re:
            mtype = MediaType.TV
            season_num = int(cn2an.cn2an(season_re.group(1), mode="smart"))

        episode_re = re.search(r"第\s*([0-9一二三四五六七八九十百零]+)\s*集", content, re.IGNORECASE)
        if episode_re:
            mtype = MediaType.TV
            episode_num = int(cn2an.cn2an(episode_re.group(1), mode="smart"))
            if episode_num and not season_num:
                season_num = 1

        year_re = re.search(r"[\s(]+(\d{4})[\s)]*", content)
        year = year_re.group(1) if year_re else None

        key_word = re.sub(
            r"第\s*[0-9一二三四五六七八九十]+\s*季|第\s*[0-9一二三四五六七八九十百零]+\s*集|[\s(]+(\d{4})[\s)]*",
            "",
            content,
            flags=re.IGNORECASE,
        ).strip()
        key_word = re.sub(r"\s+", " ", key_word) if key_word else year

        return mtype, key_word, season_num, episode_num, year, content

"""
WordsMatcher —— 自定义识别词预处理（屏蔽词/替换词/集偏移）。
来源：MoviePilot app/core/meta/words.py（移除数据库依赖）

用法：
    # 不带自定义词
    matcher = WordsMatcher()
    result = matcher.prepare("标题")

    # 带自定义词（来自配置文件/数据库/用户输入）
    matcher = WordsMatcher(custom_words=["屏蔽词1", "旧词 => 新词", "前 <> 后 >> EP+2"])
    result = matcher.prepare("标题")
"""
import re
import logging
from typing import List, Optional, Tuple

import cn2an

logger = logging.getLogger(__name__)


class WordsMatcher:
    """
    自定义识别词处理
    词条格式：
        - 屏蔽词：  keyword
        - 替换词：  old => new
        - 集偏移：  before <> after >> EP+N  或  EP-N
        - 组合：    old => new && before <> after >> EP+N
    """

    def __init__(self, custom_words: Optional[List[str]] = None):
        """
        :param custom_words: 自定义识别词条列表，每行一个词条
        """
        self._word_list: List[str] = [w.strip() for w in (custom_words or []) if w and w.strip()]

    def prepare(self, title: str) -> Tuple[str, List[str]]:
        """
        预处理标题：依次应用所有词条规则。
        :param title: 原始标题
        :return: (处理后的标题, 命中的词条列表)
        """
        if not title or not self._word_list:
            return title, []

        applied: List[str] = []
        for word in self._word_list:
            if "&&" in word:
                # 组合：替换+偏移
                rep_part, offset_part = word.split("&&", 1)
                title, hit = self._apply_replace(title, rep_part.strip())
                if hit:
                    applied.append(rep_part.strip())
                title, hit = self._apply_offset(title, offset_part.strip())
                if hit:
                    applied.append(offset_part.strip())
            elif "=>" in word:
                title, hit = self._apply_replace(title, word)
                if hit:
                    applied.append(word)
            elif "<>" in word and ">>" in word:
                title, hit = self._apply_offset(title, word)
                if hit:
                    applied.append(word)
            else:
                # 屏蔽词
                new_title = re.sub(r"%s" % re.escape(word), "", title, flags=re.IGNORECASE)
                if new_title != title:
                    applied.append(word)
                    title = new_title

        return title.strip(), applied

    @staticmethod
    def _apply_replace(title: str, word: str) -> Tuple[str, bool]:
        """应用替换词规则"""
        parts = word.split("=>", 1)
        if len(parts) != 2:
            return title, False
        old_w, new_w = parts[0].strip(), parts[1].strip()
        if not old_w:
            return title, False
        try:
            new_title = re.sub(old_w, new_w, title, flags=re.IGNORECASE)
        except re.error:
            new_title = title.replace(old_w, new_w)
        return new_title, new_title != title

    @staticmethod
    def _apply_offset(title: str, word: str) -> Tuple[str, bool]:
        """应用集偏移规则"""
        # 格式：before_word <> after_word >> EP+N 或 EP-N
        m = re.match(r"^(.+?)\s*<>\s*(.+?)\s*>>\s*EP([+-]\d+)$", word, re.IGNORECASE)
        if not m:
            return title, False
        before, after, offset_str = m.group(1).strip(), m.group(2).strip(), m.group(3)
        try:
            offset = int(offset_str)
        except ValueError:
            return title, False

        # 在标题中找到 before ... 数字 ... after 的结构
        pattern = r"(%s)\s*(\d+)\s*(%s)" % (re.escape(before), re.escape(after))
        match = re.search(pattern, title, re.IGNORECASE)
        if not match:
            return title, False

        try:
            ep_num = int(match.group(2)) + offset
            if ep_num < 0:
                ep_num = 0
        except ValueError:
            return title, False

        new_title = title[: match.start(2)] + str(ep_num) + title[match.end(2):]
        return new_title, True


def is_anime(title: str) -> bool:
    """
    判断是否为动漫格式（简单规则：带方括号形如 [字幕组] Title [编号]...）
    与 MoviePilot app/core/metainfo.py 中 is_anime() 逻辑一致。
    """
    if not title:
        return False
    anime_re = r"^\[.+\].+|\d{4}年?.{0,5}[番漫]|新番|动漫|動漫"
    return bool(re.search(anime_re, title, re.IGNORECASE))

"""
MetaInfo / MetaInfoPath —— 解析入口，对外暴露的主要接口。
来源：MoviePilot app/core/metainfo.py（移除 app.* 依赖）

用法：
    from mediaparser import MetaInfo, MetaInfoPath

    # 从字符串（种子名/文件名）解析
    meta = MetaInfo("Breaking.Bad.S01E03.1080p.BluRay.HEVC")
    print(meta.name, meta.season, meta.episode)

    # 从文件路径解析（会合并父目录信息）
    from pathlib import Path
    meta = MetaInfoPath(Path("/media/TV/Breaking Bad/Season 1/S01E03.mkv"))
"""
import re
import logging
from pathlib import Path
from typing import Optional, List, Union

from mediaparser.meta_base import MetaBase, MEDIA_EXTS
from mediaparser.meta_video import MetaVideo
from mediaparser.meta_anime import MetaAnime
from mediaparser.words import WordsMatcher, is_anime
from mediaparser.release_group import ReleaseGroupsMatcher
from mediaparser.types import MediaType
from mediaparser.infopath import should_use_parent_title_for_file_stem, clear_parsed_title_for_parent_merge

logger = logging.getLogger(__name__)


def MetaInfo(
    title: str,
    subtitle: str = None,
    isfile: bool = False,
    custom_words: Optional[List[str]] = None,
    release_group_matcher: Optional[ReleaseGroupsMatcher] = None,
) -> MetaBase:
    """
    从标题字符串创建元数据对象。

    :param title:    媒体标题或文件名（带或不带后缀均可）
    :param subtitle: 副标题/描述（可选，用于补充季集信息）
    :param isfile:   是否为文件名
    :param custom_words:  自定义识别词列表（替换/屏蔽/集偏移）
    :param release_group_matcher: 自定义字幕组匹配器（可选，不传则用内置规则）
    :return: MetaBase 子类实例（MetaVideo 或 MetaAnime）
    """
    if not title:
        return MetaVideo("", subtitle=subtitle, isfile=isfile)

    # 1. 自定义词预处理
    matcher = WordsMatcher(custom_words=custom_words)
    title, applied_words = matcher.prepare(title)

    # 2. 提取内嵌 ID 标签
    meta_tmdbid, meta_doubanid, meta_type, meta_season, meta_episode = _find_metainfo(title)
    title = _strip_metainfo_tag(title)

    # 3. 去掉媒体文件后缀
    if isfile:
        for ext in MEDIA_EXTS:
            if title.lower().endswith(ext):
                title = title[: -len(ext)]
                break

    # 4. 选择解析器
    if is_anime(title):
        meta = MetaAnime(title, subtitle=subtitle, isfile=isfile, release_group_matcher=release_group_matcher)
    else:
        meta = MetaVideo(title, subtitle=subtitle, isfile=isfile, release_group_matcher=release_group_matcher)

    # 5. 回填内嵌信息
    meta.apply_words = applied_words
    if meta_tmdbid:
        meta.tmdbid = meta_tmdbid
    if meta_doubanid:
        meta.doubanid = meta_doubanid
    if meta_type:
        meta.type = meta_type
    if meta_season is not None and meta.begin_season is None:
        meta.begin_season = meta_season
        meta.total_season = 1
    if meta_episode is not None and meta.begin_episode is None:
        meta.begin_episode = meta_episode
        meta.total_episode = 1

    return meta


def MetaInfoPath(
    path: Union[str, Path],
    custom_words: Optional[List[str]] = None,
    release_group_matcher: Optional[ReleaseGroupsMatcher] = None,
) -> MetaBase:
    """
    从文件路径创建元数据对象（文件名 + 父目录 + 上上级目录 merge）。

    :param path: 文件路径（str 或 Path）
    :param custom_words: 自定义识别词列表
    :param release_group_matcher: 自定义字幕组匹配器
    :return: MetaBase 子类实例
    """
    if not path:
        return MetaVideo("")

    path = Path(path)

    # 从文件名解析（去掉后缀）
    stem = path.stem
    file_meta = MetaInfo(
        title=stem,
        subtitle=None,
        isfile=True,
        custom_words=custom_words,
        release_group_matcher=release_group_matcher,
    )

    # 父目录
    parent_name = path.parent.name if path.parent != path else ""
    if parent_name:
        # 若文件名仅含辅助中文词（简繁/字幕等），改用父目录标题
        if should_use_parent_title_for_file_stem(stem, parent_name, file_meta):
            clear_parsed_title_for_parent_merge(file_meta)

        dir_meta = MetaInfo(
            title=parent_name,
            subtitle=None,
            isfile=False,
            custom_words=custom_words,
            release_group_matcher=release_group_matcher,
        )
        file_meta.merge(dir_meta)

    # 上上级目录
    root_name = path.parent.parent.name if path.parent.parent != path.parent else ""
    if root_name and root_name != parent_name:
        root_meta = MetaInfo(
            title=root_name,
            subtitle=None,
            isfile=False,
            custom_words=custom_words,
            release_group_matcher=release_group_matcher,
        )
        file_meta.merge(root_meta)

    return file_meta


# ── 内嵌 ID 标签提取 ────────────────────────────────────────────

_TAG_PATTERNS = [
    # {[tmdbid=xxx;type=xxx;s=x;e=x]}
    re.compile(
        r"\{?\[tmdbid=(?P<tmdbid>\d+)(?:;type=(?P<type>[a-zA-Z]+))?(?:;s=(?P<s>\d+))?(?:;e=(?P<e>\d+))?]\}?",
        re.IGNORECASE,
    ),
    # [tmdbid=xxx] / [tmdb=xxx]
    re.compile(r"\[tmdb(?:id)?=(?P<tmdbid>\d+)]", re.IGNORECASE),
    # {tmdbid=xxx} / {tmdb=xxx}
    re.compile(r"\{tmdb(?:id)?=(?P<tmdbid>\d+)}", re.IGNORECASE),
    # [doubanid=xxx]
    re.compile(r"\[douban(?:id)?=(?P<doubanid>\d+)]", re.IGNORECASE),
]


def _find_metainfo(title: str):
    """
    从标题中提取内嵌 TMDB/豆瓣 ID 及类型标签。
    返回 (tmdbid, doubanid, media_type, season, episode)
    """
    tmdbid = doubanid = media_type = season = episode = None
    for pat in _TAG_PATTERNS:
        m = pat.search(title)
        if not m:
            continue
        d = m.groupdict()
        if d.get("tmdbid") and not tmdbid:
            tmdbid = int(d["tmdbid"])
        if d.get("doubanid") and not doubanid:
            doubanid = str(d["doubanid"])
        if d.get("type") and not media_type:
            media_type = MediaType.from_agent(d["type"])
        if d.get("s") and season is None:
            season = int(d["s"])
        if d.get("e") and episode is None:
            episode = int(d["e"])
    return tmdbid, doubanid, media_type, season, episode


def _strip_metainfo_tag(title: str) -> str:
    """删除标题中的 ID 标签"""
    for pat in _TAG_PATTERNS:
        title = pat.sub("", title)
    return title.strip()

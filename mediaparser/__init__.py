"""
mediaparser —— 独立的媒体文件名解析库

从 MoviePilot 项目提取，无数据库/Web 依赖，可直接用于其他程序。

快速使用：
    from mediaparser import MetaInfo, MetaInfoPath

    meta = MetaInfo("Breaking.Bad.S01E03.1080p.BluRay.HEVC-TTG")
    print(meta.name)         # "Breaking Bad"
    print(meta.season)       # "S01"
    print(meta.episode)      # "E03"
    print(meta.resource_pix) # "1080p"
    print(meta.resource_type)# "BluRay"
    print(meta.video_encode) # "HEVC"
    print(meta.resource_team)# "TTG"
"""

from mediaparser.metainfo import MetaInfo, MetaInfoPath
from mediaparser.meta_base import MetaBase
from mediaparser.types import MediaType
from mediaparser.words import WordsMatcher
from mediaparser.release_group import ReleaseGroupsMatcher
from mediaparser.tmdb import TmdbClient
from mediaparser.config import Config, DriveConfig, OrganizerConfig, PipelineConfig

__all__ = [
    "MetaInfo",
    "MetaInfoPath",
    "MetaBase",
    "MediaType",
    "WordsMatcher",
    "ReleaseGroupsMatcher",
    "TmdbClient",
    "Config",
    "DriveConfig",
    "OrganizerConfig",
    "PipelineConfig",
]

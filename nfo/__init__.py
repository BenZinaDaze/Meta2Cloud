"""
nfo —— Plex / Infuse / Kodi 兼容的 NFO 文件生成模块

NFO 文件格式遵循 XBMC/Kodi XML 约定（Plex 和 Infuse 均支持）。

快速使用：
    from nfo import NfoGenerator, ImageUploader
    from mediaparser.types import MediaType

    gen = NfoGenerator()
    xml = gen.generate(tmdb_info, media_type=MediaType.MOVIE)
    xml_tv = gen.generate_tvshow(tmdb_info)
    xml_season = gen.generate_season(season_detail, season_number=1)
    nfo_name = gen.nfo_name_for("Inception.2010.1080p.mkv")

    uploader = ImageUploader(client)
    uploader.upload_poster(poster_path="/poster.jpg", folder_id="1AbC...")
    uploader.upload_season_poster(poster_path="/s1.jpg", season=1, folder_id="1AbC...")
"""

from nfo.generator import NfoGenerator
from nfo.image_uploader import ImageUploader

__all__ = ["NfoGenerator", "ImageUploader"]

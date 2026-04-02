"""
nfo/image_uploader.py —— 从 TMDB 下载图片并上传到 Google Drive

支持的图片类型（对齐 Plex / Infuse / MoviePilot 标准）：
    poster.jpg         媒体文件夹根目录（电影或剧名文件夹）
    fanart.jpg         媒体文件夹根目录
    season01-poster.jpg  剧名文件夹根目录（季封面）
    episode-thumb.jpg  Season 子文件夹（单集剧照，可选）
"""

import logging
from pathlib import Path
from typing import Optional

import requests

from drive.client import DriveClient, DriveFile

logger = logging.getLogger(__name__)

TMDB_IMAGE_BASE = "https://image.tmdb.org/t/p/original"

# MIME 类型映射
_EXT_MIME = {
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".png": "image/png",
    ".webp": "image/webp",
}


class ImageUploader:
    """
    从 TMDB 图片 CDN 下载图片，并上传到指定的 Drive 文件夹。

    用法：
        uploader = ImageUploader(client, session=requests.Session())
        uploader.upload_poster(poster_path="/poster.jpg", folder_id="1AbC...")
        uploader.upload_fanart(backdrop_path="/backdrop.jpg", folder_id="1AbC...")
        uploader.upload_season_poster(poster_path="/season1.jpg", season=1, folder_id="1AbC...")
        uploader.upload_episode_thumb(still_path="/still.jpg", folder_id="1AbC...")
    """

    def __init__(
        self,
        client: DriveClient,
        session: Optional[requests.Session] = None,
        timeout: int = 15,
        overwrite: bool = True,
    ):
        self._client = client
        self._session = session or requests.Session()
        self._timeout = timeout
        self._overwrite = overwrite

    # ── 公共接口 ────────────────────────────────────────────────────

    def upload_poster(
        self,
        poster_path: Optional[str],
        folder_id: str,
        filename: str = "poster.jpg",
    ) -> Optional[DriveFile]:
        """
        下载并上传 poster（封面）。

        :param poster_path: TMDB 图片路径，如 "/poster.jpg"
        :param folder_id:   目标文件夹 ID（电影文件夹 or 剧名文件夹）
        :param filename:    Drive 上的文件名（默认 poster.jpg）
        """
        return self._download_and_upload(poster_path, filename, folder_id)

    def upload_fanart(
        self,
        backdrop_path: Optional[str],
        folder_id: str,
        filename: str = "fanart.jpg",
    ) -> Optional[DriveFile]:
        """
        下载并上传 fanart（背景图）。

        :param backdrop_path: TMDB backdrop_path，如 "/backdrop.jpg"
        :param folder_id:     目标文件夹 ID
        :param filename:      Drive 上的文件名（默认 fanart.jpg）
        """
        return self._download_and_upload(backdrop_path, filename, folder_id)

    def upload_season_poster(
        self,
        poster_path: Optional[str],
        season: int,
        folder_id: str,
    ) -> Optional[DriveFile]:
        """
        下载并上传季封面。

        命名规则（Plex / Infuse 标准）：
            Season 1  → season01-poster.jpg
            Season 0  → season-specials-poster.jpg

        :param poster_path: TMDB 季 poster_path
        :param season:      季号（0 = 特典）
        :param folder_id:   剧名文件夹 ID（不是 Season 子文件夹）
        """
        from nfo.generator import NfoGenerator
        filename = NfoGenerator.season_poster_name(season)
        return self._download_and_upload(poster_path, filename, folder_id)

    def upload_episode_thumb(
        self,
        still_path: Optional[str],
        folder_id: str,
        filename: str = "episode-thumb.jpg",
    ) -> Optional[DriveFile]:
        """
        下载并上传单集剧照（still_path）。

        :param still_path: TMDB 单集 still_path
        :param folder_id:  Season 文件夹 ID
        :param filename:   Drive 上的文件名
        """
        return self._download_and_upload(still_path, filename, folder_id)

    # ── 内部实现 ────────────────────────────────────────────────────

    def _download_and_upload(
        self,
        tmdb_path: Optional[str],
        filename: str,
        folder_id: str,
    ) -> Optional[DriveFile]:
        """下载 TMDB 图片并上传到 Drive。失败时返回 None。"""
        if not tmdb_path:
            return None

        url = TMDB_IMAGE_BASE + tmdb_path
        mime_type = self._guess_mime(filename)

        # 下载
        data = self._fetch(url)
        if not data:
            return None

        # 上传
        try:
            uploaded = self._client.upload_bytes(
                content=data,
                name=filename,
                parent_id=folder_id,
                mime_type=mime_type,
                overwrite=self._overwrite,
            )
            logger.debug("图片已上传：%s → [%s]", filename, uploaded.id[:12])
            return uploaded
        except Exception as e:
            logger.warning("图片上传失败 [%s]: %s", filename, e)
            return None

    def _fetch(self, url: str) -> Optional[bytes]:
        """HTTP 下载图片，失败返回 None。"""
        try:
            resp = self._session.get(url, timeout=self._timeout)
            resp.raise_for_status()
            return resp.content
        except Exception as e:
            logger.warning("图片下载失败 [%s]: %s", url, e)
            return None

    @staticmethod
    def _guess_mime(filename: str) -> str:
        ext = Path(filename).suffix.lower()
        return _EXT_MIME.get(ext, "image/jpeg")

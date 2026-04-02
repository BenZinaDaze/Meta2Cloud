"""
organizer.py —— 媒体文件夹整理器

功能：
  解析媒体文件名  →  生成符合 MoviePilot 规范的目录路径
  →  在 Google Drive 按需创建（幂等）文件夹

目录结构规范（与 MoviePilot 默认格式一致）：
  电影：  {片名} ({年份})/
  剧集：  {剧名} ({年份})/Season {季号}/

用法：
    from organizer import MediaOrganizer
    from drive import DriveClient

    client = DriveClient.from_oauth("config/credentials.json", "config/token.json")
    org = MediaOrganizer(client, root_folder_id="1AbCd...")

    # 从文件名整理
    folder = org.ensure_folder_for("Breaking.Bad.S01E03.1080p.mkv")
    print(folder)   # 📁 'Season 1' [1SH1h3iGo...]

    # 只获取路径字符串（不操作 Drive）
    path = org.folder_path_for("Inception.2010.1080p.mkv")
    print(path)     # "Inception (2010)"
"""

import logging
from pathlib import Path
from typing import Optional, Union

from drive.client import DriveClient, DriveFile
from mediaparser import MetaInfo, MetaInfoPath, MediaType

logger = logging.getLogger(__name__)


class MediaOrganizer:
    """
    媒体文件夹整理器

    参数：
        client        : DriveClient 实例
        root_folder_id: Drive 根文件夹 ID（整理目标的顶层目录）
        movie_root_id : 电影专用子目录 ID，None 则使用 root_folder_id
        tv_root_id    : 剧集专用子目录 ID，None 则使用 root_folder_id
        dry_run       : True 时只计算路径，不实际创建 Drive 文件夹
    """

    def __init__(
        self,
        client: DriveClient,
        root_folder_id: str,
        movie_root_id: Optional[str] = None,
        tv_root_id: Optional[str] = None,
        dry_run: bool = False,
    ):
        self._client = client
        self._root_id = root_folder_id
        self._movie_root_id = movie_root_id or root_folder_id
        self._tv_root_id = tv_root_id or root_folder_id
        self._dry_run = dry_run

    # ── 公共接口 ───────────────────────────────────────────────────────────────

    def folder_path_for(self, name: str, isfile: bool = True) -> str:
        """
        根据文件名（或目录名）计算目标文件夹路径字符串，不操作 Drive。

        参数：
            name   : 文件名（例如 "Breaking.Bad.S01E03.mkv"）或目录名
            isfile : 是否为文件名（含扩展名）

        返回：相对路径字符串，例如 "Breaking Bad (2008)/Season 1"
              解析失败或名称为空时返回 ""
        """
        meta = MetaInfo(name, isfile=isfile)
        return self._build_path(meta)

    def folder_path_for_meta(self, meta) -> str:
        """
        直接通过 MetaInfo 计算目标文件夹路径字符串
        """
        return self._build_path(meta)

    def folder_path_for_path(self, path: Union[str, Path]) -> str:
        """
        根据文件路径（合并父目录信息）计算目标文件夹路径字符串，不操作 Drive。

        参数：
            path : 文件完整路径

        返回：相对路径字符串
        """
        meta = MetaInfoPath(path)
        return self._build_path(meta)

    def ensure_folder_for(self, name: str, isfile: bool = True) -> Optional[DriveFile]:
        """
        根据文件名在 Drive 中按需创建文件夹（幂等），返回叶子文件夹。

        参数：
            name   : 文件名（例如 "Breaking.Bad.S01E03.mkv"）
            isfile : 是否为文件名

        返回：叶子文件夹的 DriveFile；解析失败或 dry_run 时返回 None
        """
        meta = MetaInfo(name, isfile=isfile)
        return self._ensure_folders(meta, label=name)

    def ensure_folder_for_meta(self, meta, label: str = "") -> Optional[DriveFile]:
        """
        直接通过 MetaInfo 在 Drive 中按需创建文件夹（幂等），返回叶子文件夹。
        """
        return self._ensure_folders(meta, label=label)

    def ensure_folder_for_path(self, path: Union[str, Path]) -> Optional[DriveFile]:
        """
        根据文件路径（合并父目录信息）在 Drive 中按需创建文件夹（幂等）。

        参数：
            path : 文件完整路径

        返回：叶子文件夹的 DriveFile
        """
        meta = MetaInfoPath(path)
        return self._ensure_folders(meta, label=str(path))

    # ── 内部实现 ───────────────────────────────────────────────────────────────

    @staticmethod
    def _build_path(meta) -> str:
        """根据 MetaBase 拼装路径字符串（不含根目录）。"""
        if not meta or not meta.name:
            return ""

        # 顶层目录：片名 (年份)
        top = meta.name
        if meta.year:
            top = f"{top} ({meta.year})"

        if meta.type == MediaType.TV:
            # Season NN（两位零填充，Emby/Plex/Infuse 三者标准）
            season_num = int(meta.season_seq or 1)
            return f"{top}/Season {season_num:02d}"
        else:
            # 电影或未知类型，只有一层
            return top

    def _ensure_folders(self, meta, label: str = "") -> Optional[DriveFile]:
        """在 Drive 中幂等创建目录链，返回叶子文件夹。"""
        if not meta or not meta.name:
            logger.warning(f"解析失败，跳过：{label!r}")
            return None

        media_type = meta.type
        is_tv = media_type == MediaType.TV

        # 选择基准目录
        base_id = self._tv_root_id if is_tv else self._movie_root_id

        # 顶层目录名
        top_name = meta.name
        if meta.year:
            top_name = f"{top_name} ({meta.year})"

        season_int = int(meta.season_seq or 1)
        logger.info(
            f"[{'TV' if is_tv else 'Movie'}] {label!r} → /{top_name}"
            + (f"/Season {season_int:02d}" if is_tv else "")
        )

        if self._dry_run:
            logger.info("dry_run=True，跳过 Drive 操作")
            return None

        # 第一层：片名/剧名 (年份)
        top_folder = self._client.get_or_create_folder(top_name, parent_id=base_id)
        logger.debug(f"  顶层文件夹：{top_folder}")

        if not is_tv:
            return top_folder

        # 第二层（仅剧集）：Season NN（两位零填充，三平台统一标准）
        season_name = f"Season {season_int:02d}"
        season_folder = self._client.get_or_create_folder(
            season_name, parent_id=top_folder.id
        )
        logger.debug(f"  季文件夹：{season_folder}")
        return season_folder

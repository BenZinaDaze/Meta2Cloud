"""
storage/base.py —— 网盘存储抽象层

定义 StorageProvider ABC 和 CloudFile 统一数据模型。
所有网盘 Provider 需要实现此 ABC，
Pipeline / Organizer / WebUI 只依赖此接口。
"""

from __future__ import annotations

import os
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from enum import Enum
from typing import Iterator, List, Optional


class FileType(Enum):
    """通用文件类型"""
    FILE = "file"
    FOLDER = "folder"


# 常见视频扩展名（跨网盘通用判断）
_VIDEO_EXTENSIONS = {
    ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv",
    ".ts", ".m2ts", ".webm", ".rmvb", ".rm", ".mpg",
    ".mpeg", ".vob", ".iso", ".3gp",
}


@dataclass
class CloudFile:
    """
    统一的云文件/文件夹描述。

    所有网盘 Provider 返回此类型，屏蔽各平台字段差异。

    字段说明：
        id            : 文件唯一 ID（各平台格式不同，均为字符串）
        name          : 文件名（含扩展名）
        file_type     : FILE 或 FOLDER
        size          : 文件大小（字节），文件夹为 None
        modified_time : 修改时间（ISO8601 字符串或 Unix 时间戳字符串）
        parent_id     : 父目录 ID（部分平台可能为空）
        parents       : 父目录 ID 列表（兼容 Google Drive 多父目录场景）
        mime_type     : MIME 类型（部分平台可能为空）
        trashed       : 是否在回收站
        extra         : 平台特有字段（pick_code, sha1 等）
    """
    id: str
    name: str
    file_type: FileType
    size: Optional[int] = None
    modified_time: Optional[str] = None
    parent_id: Optional[str] = None
    parents: List[str] = field(default_factory=list)
    mime_type: Optional[str] = None
    trashed: bool = False
    extra: dict = field(default_factory=dict)

    @property
    def is_folder(self) -> bool:
        return self.file_type == FileType.FOLDER

    @property
    def is_video(self) -> bool:
        """按 MIME 或扩展名判断是否为视频（跨平台通用）"""
        if self.is_folder:
            return False
        if self.mime_type and self.mime_type.startswith("video/"):
            return True
        return self.extension in _VIDEO_EXTENSIONS

    @property
    def is_subtitle(self) -> bool:
        """判断是否为字幕文件"""
        _SUBTITLE_EXTENSIONS = {".srt", ".ass", ".ssa"}
        return not self.is_folder and self.extension in _SUBTITLE_EXTENSIONS

    @property
    def extension(self) -> str:
        _, ext = os.path.splitext(self.name)
        return ext.lower()

    def __repr__(self) -> str:
        tag = "📁" if self.is_folder else ("🎬" if self.is_video else "📄")
        size_str = f" {self.size // 1024 // 1024}MB" if self.size else ""
        return f"{tag} {self.name!r} [{self.id[:12]}...]{size_str}"


class StorageProvider(ABC):
    """
    网盘存储 Provider 抽象基类。

    所有网盘实现都需要提供以下标记为 @abstractmethod 的方法。
    带默认实现的方法可以被子类覆盖以提供更高效的平台特有实现。

    命名约定：
      - 参数统一使用 folder_id（而非 Google 的 parent_id 或 115 的 cid/pid）
      - 返回值统一使用 CloudFile
    """

    @property
    @abstractmethod
    def provider_name(self) -> str:
        """返回 Provider 名称，如 'google_drive', 'pan115', 'onedrive'"""
        ...

    @classmethod
    @abstractmethod
    def from_config(cls, cfg) -> "StorageProvider":
        """从 Config 对象构造 Provider 实例"""
        ...

    # ── 列举 ─────────────────────────────────────────────

    @abstractmethod
    def list_files(
        self,
        folder_id: str = "root",
        page_size: int = 100,
    ) -> List[CloudFile]:
        """列举文件夹直接子项（不递归）"""
        ...

    def list_media_files(
        self,
        folder_id: str = "root",
    ) -> List[CloudFile]:
        """
        列举文件夹内所有视频文件。

        默认实现：列举全部再过滤。
        子类可覆盖以使用更高效的平台 API。
        """
        return [f for f in self.list_files(folder_id, page_size=1000) if f.is_video]

    def list_all_recursive(
        self,
        folder_id: str = "root",
        max_depth: int = 10,
        _depth: int = 0,
    ) -> Iterator[CloudFile]:
        """
        递归遍历文件夹（生成器）。

        注意：对于超大目录可能产生大量请求，请谨慎使用。
        """
        if _depth > max_depth:
            return
        items = self.list_files(folder_id, page_size=1000)
        for item in items:
            yield item
            if item.is_folder:
                yield from self.list_all_recursive(
                    folder_id=item.id,
                    max_depth=max_depth,
                    _depth=_depth + 1,
                )

    # ── 查找 ─────────────────────────────────────────────

    @abstractmethod
    def get_file(self, file_id: str) -> CloudFile:
        """获取单个文件的元数据"""
        ...

    @abstractmethod
    def read_text(self, file: CloudFile) -> Optional[str]:
        """读取文本文件内容，失败时返回 None 或抛异常"""
        ...

    @abstractmethod
    def find_file(
        self,
        name: str,
        folder_id: Optional[str] = None,
    ) -> Optional[CloudFile]:
        """按精确文件名查找（返回第一个匹配，没有返回 None）"""
        ...

    def exists(self, name: str, folder_id: Optional[str] = None) -> bool:
        """检查指定名称的文件是否存在"""
        return self.find_file(name, folder_id) is not None

    # ── 修改 ─────────────────────────────────────────────

    @abstractmethod
    def rename_file(self, file_id: str, new_name: str) -> CloudFile:
        """重命名文件或文件夹"""
        ...

    @abstractmethod
    def move_file(
        self,
        file_id: str,
        new_folder_id: str,
        new_name: Optional[str] = None,
    ) -> CloudFile:
        """移动文件到另一个文件夹，可同时重命名"""
        ...

    # ── 上传 ─────────────────────────────────────────────

    @abstractmethod
    def upload_text(
        self,
        content: str,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "text/xml",
        overwrite: bool = False,
    ) -> CloudFile:
        """上传文本内容为文件（适合 NFO 等小文件）"""
        ...

    @abstractmethod
    def upload_bytes(
        self,
        content: bytes,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "image/jpeg",
        overwrite: bool = True,
    ) -> CloudFile:
        """上传二进制内容为文件（适合图片等非文本文件）"""
        ...

    # ── 删除 ─────────────────────────────────────────────

    @abstractmethod
    def trash_file(self, file_id: str) -> CloudFile:
        """移动到回收站（可恢复）"""
        ...

    @abstractmethod
    def delete_file(self, file_id: str) -> None:
        """彻底删除文件（不可恢复）"""
        ...

    # ── 文件夹 ───────────────────────────────────────────

    @abstractmethod
    def create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> CloudFile:
        """创建文件夹"""
        ...

    def get_or_create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> CloudFile:
        """如果文件夹不存在则创建，否则返回已有的（默认实现）"""
        existing = self.find_file(name, folder_id=parent_id)
        if existing and existing.is_folder:
            return existing
        return self.create_folder(name, parent_id)

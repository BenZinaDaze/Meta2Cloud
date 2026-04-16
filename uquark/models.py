"""
uquark/models.py —— 夸克网盘数据模型
"""

from __future__ import annotations

import os
import time
from dataclasses import asdict, dataclass, field
from typing import Any, Optional


@dataclass
class QuarkToken:
    """夸克网盘 access_token / refresh_token 持久化模型"""

    access_token: str
    refresh_token: str
    expires_in: int
    refresh_time: int = field(default_factory=lambda: int(time.time()))
    uid: str = ""

    @property
    def expires_at(self) -> int:
        return self.refresh_time + int(self.expires_in)

    def is_expired(self, *, skew_seconds: int = 60) -> bool:
        return int(time.time()) + skew_seconds >= self.expires_at

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "QuarkToken":
        return cls(
            access_token=str(data["access_token"]),
            refresh_token=str(data["refresh_token"]),
            expires_in=int(data["expires_in"]),
            refresh_time=int(data.get("refresh_time", time.time())),
            uid=str(data.get("uid", "")),
        )


@dataclass
class QuarkFile:
    """夸克网盘文件或目录的通用描述"""

    id: str
    name: str
    size: Optional[int] = None
    modified_time: Optional[int] = None
    created_time: Optional[int] = None
    parent_id: Optional[str] = None
    pdir_fid: Optional[str] = None
    filetype: Optional[int] = None  # 1=文件夹, 2=文件
    mime_type: Optional[str] = None
    suffix: Optional[str] = None
    sha1: Optional[str] = None
    md5: Optional[str] = None
    block_list: Optional[list] = None
    drive_id: Optional[str] = None
    status: Optional[int] = None  # 1=正常, 2=回收站, 3=彻底删除
    raw: dict[str, Any] = field(default_factory=dict)

    @property
    def is_folder(self) -> bool:
        return self.filetype == 1

    @property
    def is_file(self) -> bool:
        return self.filetype == 2

    @property
    def extension(self) -> str:
        _, ext = os.path.splitext(self.name)
        return ext.lower()

    @property
    def is_video(self) -> bool:
        """判断是否为视频文件"""
        video_extensions = {
            ".mp4", ".mkv", ".avi", ".mov", ".wmv", ".flv",
            ".ts", ".m2ts", ".webm", ".rmvb", ".rm", ".mpg",
            ".mpeg", ".vob", ".iso", ".3gp",
        }
        return self.extension in video_extensions

    @property
    def trashed(self) -> bool:
        return self.status == 2

    @classmethod
    def from_list_item(cls, raw: dict[str, Any]) -> "QuarkFile":
        """从文件列表项构造"""
        return cls(
            id=str(raw.get("fid", "")),
            name=str(raw.get("fname", "")),
            size=int(raw["size"]) if raw.get("size") not in (None, "") else None,
            modified_time=int(raw["mtime"]) if raw.get("mtime") not in (None, "") else None,
            created_time=int(raw["ctime"]) if raw.get("ctime") not in (None, "") else None,
            parent_id=str(raw.get("pdir_fid", "")) if raw.get("pdir_fid") not in (None, "") else None,
            pdir_fid=str(raw.get("pdir_fid", "")) if raw.get("pdir_fid") not in (None, "") else None,
            filetype=int(raw.get("type", 0)),
            mime_type=raw.get("mime_type"),
            suffix=raw.get("suffix"),
            sha1=raw.get("sha1"),
            md5=raw.get("md5"),
            block_list=raw.get("block_list"),
            drive_id=raw.get("drive_id"),
            status=int(raw.get("status", 1)),
            raw=raw,
        )

    @classmethod
    def from_search_item(cls, raw: dict[str, Any]) -> "QuarkFile":
        """从搜索结果构造"""
        return cls(
            id=str(raw.get("fid", "")),
            name=str(raw.get("fname", "")),
            size=int(raw["size"]) if raw.get("size") not in (None, "") else None,
            modified_time=int(raw["mtime"]) if raw.get("mtime") not in (None, "") else None,
            created_time=int(raw["ctime"]) if raw.get("ctime") not in (None, "") else None,
            parent_id=str(raw.get("pdir_fid", "")) if raw.get("pdir_fid") not in (None, "") else None,
            pdir_fid=str(raw.get("pdir_fid", "")) if raw.get("pdir_fid") not in (None, "") else None,
            filetype=int(raw.get("type", 0)),
            mime_type=raw.get("mime_type"),
            suffix=raw.get("suffix"),
            sha1=raw.get("sha1"),
            md5=raw.get("md5"),
            block_list=raw.get("block_list"),
            drive_id=raw.get("drive_id"),
            status=int(raw.get("status", 1)),
            raw=raw,
        )

    @classmethod
    def from_detail(cls, raw: dict[str, Any]) -> "QuarkFile":
        """从文件详情构造"""
        return cls(
            id=str(raw.get("file_id", "")),
            name=str(raw.get("file_name", "")),
            size=int(raw["size"]) if raw.get("size") not in (None, "") else None,
            modified_time=int(raw.get("updated_at")) if raw.get("updated_at") not in (None, "") else None,
            created_time=int(raw.get("created_at")) if raw.get("created_at") not in (None, "") else None,
            parent_id=str(raw.get("parent_id", "")) if raw.get("parent_id") not in (None, "") else None,
            pdir_fid=str(raw.get("parent_id", "")) if raw.get("parent_id") not in (None, "") else None,
            filetype=1 if raw.get("type") == "folder" else 2,
            mime_type=raw.get("mime_type"),
            suffix=raw.get("suffix"),
            sha1=raw.get("sha1"),
            md5=raw.get("md5"),
            block_list=raw.get("block_list"),
            drive_id=raw.get("drive_id"),
            status=int(raw.get("status", 1)),
            raw=raw,
        )


@dataclass
class QuarkSpaceInfo:
    """空间使用信息"""

    total_size: int
    used_size: int
    remain_size: int
    raw: dict[str, Any] = field(default_factory=dict)

    @property
    def total_gb(self) -> float:
        return self.total_size / (1024 ** 3)

    @property
    def used_gb(self) -> float:
        return self.used_size / (1024 ** 3)

    @property
    def remain_gb(self) -> float:
        return self.remain_size / (1024 ** 3)

    @property
    def used_percent(self) -> float:
        if self.total_size == 0:
            return 0.0
        return self.used_size / self.total_size * 100

"""
u115pan/models.py —— 115 开放平台通用数据模型
"""

from __future__ import annotations

import os
import time
from dataclasses import asdict, dataclass, field
from typing import Any, Optional


@dataclass
class Pan115Token:
    """115 access_token / refresh_token 持久化模型"""

    access_token: str
    refresh_token: str
    expires_in: int
    refresh_time: int = field(default_factory=lambda: int(time.time()))

    @property
    def expires_at(self) -> int:
        return self.refresh_time + int(self.expires_in)

    def is_expired(self, *, skew_seconds: int = 60) -> bool:
        return int(time.time()) + skew_seconds >= self.expires_at

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> "Pan115Token":
        return cls(
            access_token=str(data["access_token"]),
            refresh_token=str(data["refresh_token"]),
            expires_in=int(data["expires_in"]),
            refresh_time=int(data.get("refresh_time") or time.time()),
        )


@dataclass
class DeviceCodeSession:
    """扫码登录会话"""

    qrcode: str
    uid: str
    time_value: str
    sign: str
    code_verifier: str


@dataclass
class QrcodeStatus:
    """扫码轮询状态"""

    status: int
    message: str = ""
    raw: dict[str, Any] = field(default_factory=dict)

    @property
    def confirmed(self) -> bool:
        return self.status == 2


@dataclass
class Pan115File:
    """115 文件或目录的通用描述"""

    id: str
    name: str
    category: str
    size: Optional[int] = None
    modified_time: Optional[int] = None
    pick_code: Optional[str] = None
    sha1: Optional[str] = None
    parent_id: Optional[str] = None
    ico: Optional[str] = None
    raw: dict[str, Any] = field(default_factory=dict)

    @property
    def is_folder(self) -> bool:
        return str(self.category) == "0"

    @property
    def is_file(self) -> bool:
        return not self.is_folder

    @property
    def extension(self) -> str:
        _, ext = os.path.splitext(self.name)
        return ext.lower()

    @classmethod
    def from_list_item(cls, raw: dict[str, Any]) -> "Pan115File":
        return cls(
            id=str(raw.get("fid", "")),
            name=str(raw.get("fn", "")),
            category=str(raw.get("fc", "")),
            size=int(raw["fs"]) if raw.get("fs") not in (None, "") else None,
            modified_time=int(raw["upt"]) if raw.get("upt") not in (None, "") else None,
            pick_code=raw.get("pc"),
            ico=raw.get("ico"),
            raw=raw,
        )

    @classmethod
    def from_path_info(cls, raw: dict[str, Any]) -> "Pan115File":
        return cls(
            id=str(raw.get("file_id", "")),
            name=str(raw.get("file_name", "")),
            category=str(raw.get("file_category", "")),
            size=int(raw["size_byte"]) if raw.get("size_byte") not in (None, "") else None,
            modified_time=int(raw["utime"]) if raw.get("utime") not in (None, "") else None,
            pick_code=raw.get("pick_code"),
            sha1=raw.get("sha1"),
            parent_id=str(raw.get("parent_id")) if raw.get("parent_id") is not None else None,
            ico=raw.get("ico"),
            raw=raw,
        )

    @classmethod
    def from_search_item(cls, raw: dict[str, Any]) -> "Pan115File":
        return cls(
            id=str(raw.get("file_id", "")),
            name=str(raw.get("file_name", "")),
            category=str(raw.get("file_category", "")),
            size=int(raw["file_size"]) if raw.get("file_size") not in (None, "") else None,
            modified_time=int(raw["user_utime"]) if raw.get("user_utime") not in (None, "") else None,
            pick_code=raw.get("pick_code"),
            sha1=raw.get("sha1"),
            parent_id=str(raw.get("parent_id")) if raw.get("parent_id") is not None else None,
            ico=raw.get("ico"),
            raw=raw,
        )


@dataclass
class Pan115SpaceInfo:
    """空间使用信息"""

    total_size: int
    remain_size: int
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class UploadHashes:
    """上传前特征值"""

    file_size: int
    fileid: str
    preid: str


@dataclass
class UploadInitInfo:
    """初始化上传返回"""

    bucket: Optional[str]
    object_name: Optional[str]
    callback: Optional[dict[str, Any]]
    sign_check: Optional[str]
    pick_code: Optional[str]
    sign_key: Optional[str]
    code: Optional[int]
    status: Optional[int]
    file_id: Optional[str]
    raw: dict[str, Any] = field(default_factory=dict)

    @property
    def is_fast_upload(self) -> bool:
        return self.status == 2

    @property
    def requires_second_check(self) -> bool:
        return self.code in (700, 701) and bool(self.sign_check)


@dataclass
class UploadToken:
    """对象存储临时凭证"""

    endpoint: str
    access_key_id: str
    access_key_secret: str
    security_token: str
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class ResumeInfo:
    """断点续传信息"""

    callback: Optional[dict[str, Any]]
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class RecycleBinItem:
    """回收站条目"""

    id: str
    name: str
    category: Optional[str] = None
    size: Optional[int] = None
    raw: dict[str, Any] = field(default_factory=dict)

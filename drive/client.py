"""
drive/client.py —— Google Drive 文件操作封装

主要功能：
  - list_files()        列举文件夹内容（可过滤类型/名称）
  - list_media_files()  只返回视频文件
  - get_file()          获取单个文件元数据
  - find_file()         按名称查找文件
  - rename_file()       重命名文件
  - move_file()         移动文件到其他文件夹
  - upload_text()       上传文本内容（如 NFO）
  - upload_bytes()      上传二进制内容（如图片）
  - upload_file()       上传本地文件
  - delete_file()       删除文件（彻底删除）
  - trash_file()        移到回收站
  - create_folder()     创建文件夹
  - exists()            检查文件/文件夹是否存在
"""

import io
import os
from dataclasses import dataclass, field
from typing import Iterator, List, Optional

from googleapiclient.errors import HttpError
from googleapiclient.http import MediaFileUpload, MediaIoBaseUpload

# Drive API 返回的字段集合
_FILE_FIELDS = "id, name, mimeType, size, modifiedTime, parents, trashed"

# 常见视频 MIME 类型前缀
_VIDEO_MIME_PREFIXES = ("video/",)

# Drive 文件夹 MIME
_FOLDER_MIME = "application/vnd.google-apps.folder"


@dataclass
class DriveFile:
    """Drive 文件/文件夹的简化描述"""

    id: str
    name: str
    mime_type: str
    size: Optional[int] = None          # 文件夹为 None
    modified_time: Optional[str] = None  # ISO8601 字符串
    parents: List[str] = field(default_factory=list)
    trashed: bool = False

    @property
    def is_folder(self) -> bool:
        return self.mime_type == _FOLDER_MIME

    @property
    def is_video(self) -> bool:
        return any(self.mime_type.startswith(p) for p in _VIDEO_MIME_PREFIXES)

    @property
    def extension(self) -> str:
        _, ext = os.path.splitext(self.name)
        return ext.lower()

    def __repr__(self) -> str:
        tag = "📁" if self.is_folder else ("🎬" if self.is_video else "📄")
        size_str = f" {self.size // 1024 // 1024}MB" if self.size else ""
        return f"{tag} {self.name!r} [{self.id[:12]}...]{size_str}"

    @classmethod
    def _from_raw(cls, raw: dict) -> "DriveFile":
        return cls(
            id=raw["id"],
            name=raw["name"],
            mime_type=raw.get("mimeType", ""),
            size=int(raw["size"]) if raw.get("size") else None,
            modified_time=raw.get("modifiedTime"),
            parents=raw.get("parents", []),
            trashed=raw.get("trashed", False),
        )


class DriveClient:
    """
    Google Drive CRUD 客户端

    推荐通过类方法构造：
        client = DriveClient.from_service_account("config/service_account.json")
        client = DriveClient.from_oauth("config/credentials.json")
    """

    def __init__(self, service):
        """
        参数：
            service : googleapiclient.discovery.build() 返回的 Resource 对象
        """
        self._svc = service

    # ── 构造函数 ──────────────────────────────────────────────────────────────

    @classmethod
    def from_service_account(cls, json_path: str) -> "DriveClient":
        """从 Service Account JSON 文件构造客户端（推荐自动化场景）"""
        from drive.auth import DriveAuth
        return cls(DriveAuth.from_service_account(json_path))

    @classmethod
    def from_oauth(
        cls,
        credentials_path: str = "config/credentials.json",
        token_path: str = "config/token.json",
    ) -> "DriveClient":
        """从 OAuth2 凭据文件构造客户端（首次使用弹浏览器授权）"""
        from drive.auth import DriveAuth
        return cls(DriveAuth.from_oauth(credentials_path, token_path))

    # ── 列举 ──────────────────────────────────────────────────────────────────

    def list_files(
        self,
        folder_id: str = "root",
        name_contains: Optional[str] = None,
        mime_type: Optional[str] = None,
        include_trashed: bool = False,
        page_size: int = 100,
    ) -> List[DriveFile]:
        """
        列举文件夹直接子项（不递归）

        参数：
            folder_id      : 目标文件夹 ID，默认 "root"
            name_contains  : 名称包含该字符串（区分大小写，Drive q 语法）
            mime_type      : 精确匹配 MIME 类型
            include_trashed: 是否包含回收站内容
            page_size      : 每页条数（API 最大 1000）

        返回：DriveFile 列表
        """
        q_parts = [f"'{folder_id}' in parents"]
        if not include_trashed:
            q_parts.append("trashed = false")
        if mime_type:
            q_parts.append(f"mimeType = '{mime_type}'")
        if name_contains:
            # Drive q 语法：name contains '...'（大小写不敏感）
            name_contains_escaped = name_contains.replace("'", "\\'")
            q_parts.append(f"name contains '{name_contains_escaped}'")

        q = " and ".join(q_parts)
        results: List[DriveFile] = []
        page_token = None

        while True:
            resp = (
                self._svc.files()
                .list(
                    q=q,
                    fields=f"nextPageToken, files({_FILE_FIELDS})",
                    pageSize=min(page_size, 1000),
                    pageToken=page_token,
                )
                .execute()
            )
            for raw in resp.get("files", []):
                results.append(DriveFile._from_raw(raw))

            page_token = resp.get("nextPageToken")
            if not page_token or len(results) >= page_size:
                break

        return results

    def list_media_files(
        self,
        folder_id: str = "root",
        include_trashed: bool = False,
    ) -> List[DriveFile]:
        """
        列举文件夹内所有视频文件（按 mimeType 过滤，不含子文件夹）

        Drive 不支持 mimeType contains 'video/'，需要获取所有再过滤。
        """
        all_files = self.list_files(
            folder_id=folder_id,
            include_trashed=include_trashed,
            page_size=1000,
        )
        return [f for f in all_files if f.is_video]

    def list_all_recursive(
        self,
        folder_id: str = "root",
        include_trashed: bool = False,
        _depth: int = 0,
        _max_depth: int = 10,
    ) -> Iterator[DriveFile]:
        """
        递归遍历文件夹（生成器）

        注意：对于超大目录会产生大量 API 请求，请谨慎使用。
        每层递归触发一次 list_files 请求。
        """
        if _depth > _max_depth:
            return

        items = self.list_files(folder_id=folder_id, include_trashed=include_trashed, page_size=1000)
        for item in items:
            yield item
            if item.is_folder:
                yield from self.list_all_recursive(
                    folder_id=item.id,
                    include_trashed=include_trashed,
                    _depth=_depth + 1,
                    _max_depth=_max_depth,
                )

    # ── 查找 ──────────────────────────────────────────────────────────────────

    def get_file(self, file_id: str) -> DriveFile:
        """
        获取单个文件的元数据

        异常：googleapiclient.errors.HttpError（404 时文件不存在）
        """
        raw = (
            self._svc.files()
            .get(fileId=file_id, fields=_FILE_FIELDS)
            .execute()
        )
        return DriveFile._from_raw(raw)

    def find_file(
        self,
        name: str,
        folder_id: Optional[str] = None,
    ) -> Optional[DriveFile]:
        """
        按精确文件名查找（返回第一个匹配，没有返回 None）

        参数：
            name      : 完整文件名（含扩展名）
            folder_id : 限定父文件夹，None 则全盘搜索
        """
        q_parts = [f"name = '{name.replace(chr(39), chr(92)+chr(39))}'", "trashed = false"]
        if folder_id:
            q_parts.append(f"'{folder_id}' in parents")

        resp = (
            self._svc.files()
            .list(
                q=" and ".join(q_parts),
                fields=f"files({_FILE_FIELDS})",
                pageSize=1,
            )
            .execute()
        )
        files = resp.get("files", [])
        return DriveFile._from_raw(files[0]) if files else None

    def exists(self, name: str, folder_id: Optional[str] = None) -> bool:
        """检查指定名称的文件是否存在"""
        return self.find_file(name, folder_id) is not None

    # ── 修改 ──────────────────────────────────────────────────────────────────

    def rename_file(self, file_id: str, new_name: str) -> DriveFile:
        """
        重命名文件或文件夹

        返回：更新后的 DriveFile
        """
        raw = (
            self._svc.files()
            .update(
                fileId=file_id,
                body={"name": new_name},
                fields=_FILE_FIELDS,
            )
            .execute()
        )
        return DriveFile._from_raw(raw)

    def move_file(
        self,
        file_id: str,
        new_folder_id: str,
        remove_from_current: bool = True,
        new_name: Optional[str] = None,
    ) -> DriveFile:
        """
        移动文件到其他文件夹，可同时重命名（一次 API 调用）

        参数：
            file_id           : 文件 ID
            new_folder_id     : 目标文件夹 ID
            remove_from_current: True 表示从原始位置移除（剪切），False 表示复制到新位置（仍保留原位）
            new_name          : 移动后的新文件名；None 表示保持原名
        """
        # 先获取当前 parents 以便移除
        current = self.get_file(file_id)
        remove_parents = ",".join(current.parents) if remove_from_current else ""

        body = {"name": new_name} if new_name else {}
        raw = (
            self._svc.files()
            .update(
                fileId=file_id,
                body=body,
                addParents=new_folder_id,
                removeParents=remove_parents,
                fields=_FILE_FIELDS,
            )
            .execute()
        )
        return DriveFile._from_raw(raw)

    # ── 上传 ──────────────────────────────────────────────────────────────────

    def upload_text(
        self,
        content: str,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "text/xml",
        overwrite: bool = False,
    ) -> DriveFile:
        """
        上传文本内容为文件（适合 NFO、字幕等小文件）

        参数：
            content   : 文件内容字符串
            name      : 目标文件名
            parent_id : 上传到哪个文件夹（None = 根目录）
            mime_type : 默认 text/xml（NFO 用）
            overwrite : 若同名文件存在则更新内容，否则新建
        """
        metadata: dict = {"name": name}
        if parent_id:
            metadata["parents"] = [parent_id]

        media = MediaIoBaseUpload(
            io.BytesIO(content.encode("utf-8")),
            mimetype=mime_type,
            resumable=False,
        )

        # 检查是否已存在同名文件
        if overwrite:
            existing = self.find_file(name, folder_id=parent_id)
            if existing:
                raw = (
                    self._svc.files()
                    .update(
                        fileId=existing.id,
                        body={},
                        media_body=media,
                        fields=_FILE_FIELDS,
                    )
                    .execute()
                )
                return DriveFile._from_raw(raw)

        raw = (
            self._svc.files()
            .create(body=metadata, media_body=media, fields=_FILE_FIELDS)
            .execute()
        )
        return DriveFile._from_raw(raw)

    def upload_bytes(
        self,
        content: bytes,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "image/jpeg",
        overwrite: bool = True,
    ) -> DriveFile:
        """
        上传二进制内容为文件（适合图片等非文本文件）。

        参数：
            content   : 二进制数据
            name      : 目标文件名（如 poster.jpg）
            parent_id : 上传到哪个文件夹（None = 根目录）
            mime_type : 默认 image/jpeg
            overwrite : 同名文件存在时更新，否则新建
        """
        metadata: dict = {"name": name}
        if parent_id:
            metadata["parents"] = [parent_id]

        media = MediaIoBaseUpload(
            io.BytesIO(content),
            mimetype=mime_type,
            resumable=False,
        )

        if overwrite:
            existing = self.find_file(name, folder_id=parent_id)
            if existing:
                raw = (
                    self._svc.files()
                    .update(
                        fileId=existing.id,
                        body={},
                        media_body=media,
                        fields=_FILE_FIELDS,
                    )
                    .execute()
                )
                return DriveFile._from_raw(raw)

        raw = (
            self._svc.files()
            .create(body=metadata, media_body=media, fields=_FILE_FIELDS)
            .execute()
        )
        return DriveFile._from_raw(raw)

    def upload_file(
        self,
        local_path: str,
        name: Optional[str] = None,
        parent_id: Optional[str] = None,
        mime_type: Optional[str] = None,
        resumable: bool = True,
        overwrite: bool = False,
    ) -> DriveFile:
        """
        上传本地文件到 Drive

        参数：
            local_path : 本地文件路径
            name       : Drive 上的文件名，默认取 local_path 的文件名
            parent_id  : 目标文件夹 ID
            mime_type  : 不指定则自动推断
            resumable  : 大文件用分块上传（>5MB 建议 True）
            overwrite  : 同名文件存在则更新
        """
        if not os.path.exists(local_path):
            raise FileNotFoundError(f"本地文件不存在：{local_path}")

        name = name or os.path.basename(local_path)
        metadata: dict = {"name": name}
        if parent_id:
            metadata["parents"] = [parent_id]

        media = MediaFileUpload(local_path, mimetype=mime_type, resumable=resumable)

        if overwrite:
            existing = self.find_file(name, folder_id=parent_id)
            if existing:
                raw = (
                    self._svc.files()
                    .update(
                        fileId=existing.id,
                        body={},
                        media_body=media,
                        fields=_FILE_FIELDS,
                    )
                    .execute()
                )
                return DriveFile._from_raw(raw)

        raw = (
            self._svc.files()
            .create(body=metadata, media_body=media, fields=_FILE_FIELDS)
            .execute()
        )
        return DriveFile._from_raw(raw)

    # ── 删除 ──────────────────────────────────────────────────────────────────

    def trash_file(self, file_id: str) -> DriveFile:
        """移动到回收站（可恢复）"""
        raw = (
            self._svc.files()
            .update(fileId=file_id, body={"trashed": True}, fields=_FILE_FIELDS)
            .execute()
        )
        return DriveFile._from_raw(raw)

    def delete_file(self, file_id: str) -> None:
        """彻底删除文件（不可恢复！）"""
        self._svc.files().delete(fileId=file_id).execute()

    # ── 文件夹 ────────────────────────────────────────────────────────────────

    def create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> DriveFile:
        """
        创建文件夹

        参数：
            name      : 文件夹名称
            parent_id : 父文件夹 ID，None = 根目录
        """
        metadata: dict = {"name": name, "mimeType": _FOLDER_MIME}
        if parent_id:
            metadata["parents"] = [parent_id]
        raw = (
            self._svc.files()
            .create(body=metadata, fields=_FILE_FIELDS)
            .execute()
        )
        return DriveFile._from_raw(raw)

    def get_or_create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> DriveFile:
        """如果文件夹不存在则创建，否则返回已有的"""
        existing = self.find_file(name, folder_id=parent_id)
        if existing and existing.is_folder:
            return existing
        return self.create_folder(name, parent_id)

    # ── 工具 ──────────────────────────────────────────────────────────────────

    def about(self) -> dict:
        """返回当前账号信息（邮箱、配额等）"""
        return (
            self._svc.about()
            .get(fields="user, storageQuota")
            .execute()
        )

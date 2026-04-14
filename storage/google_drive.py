"""
storage/google_drive.py —— Google Drive Provider 实现

内部包装现有 drive/client.py 的 DriveClient，
将 DriveFile 映射为统一的 CloudFile。

现有 DriveClient 完全保持不变，本层只做适配。
"""

from __future__ import annotations

from typing import Iterator, List, Optional

from drive.client import DriveClient, DriveFile
from storage.base import CloudFile, FileType, StorageProvider


class GoogleDriveProvider(StorageProvider):
    """Google Drive 存储 Provider"""

    def __init__(self, client: DriveClient):
        self._client = client

    @property
    def provider_name(self) -> str:
        return "google_drive"

    @classmethod
    def from_config(cls, cfg) -> "GoogleDriveProvider":
        """从 Config 对象构造（读取 drive 配置段）"""
        drive_cfg = cfg.drive
        client = DriveClient.from_oauth(
            credentials_path=drive_cfg.credentials_json,
            token_path=drive_cfg.token_json,
        )
        return cls(client)

    @property
    def raw_client(self) -> DriveClient:
        """暴露底层 DriveClient，供需要平台特有功能的场景使用"""
        return self._client

    # ── 类型转换 ─────────────────────────────────────────

    @staticmethod
    def _to_cloud_file(df: DriveFile) -> CloudFile:
        """将 DriveFile 转换为统一的 CloudFile"""
        return CloudFile(
            id=df.id,
            name=df.name,
            file_type=FileType.FOLDER if df.is_folder else FileType.FILE,
            size=df.size,
            modified_time=df.modified_time,
            parent_id=df.parents[0] if df.parents else None,
            parents=list(df.parents),
            mime_type=df.mime_type,
            trashed=df.trashed,
        )

    # ── 列举 ─────────────────────────────────────────────

    def list_files(
        self,
        folder_id: str = "root",
        page_size: int = 100,
    ) -> List[CloudFile]:
        raw = self._client.list_files(folder_id=folder_id, page_size=page_size)
        return [self._to_cloud_file(f) for f in raw]

    def list_media_files(
        self,
        folder_id: str = "root",
    ) -> List[CloudFile]:
        raw = self._client.list_media_files(folder_id=folder_id)
        return [self._to_cloud_file(f) for f in raw]

    def list_all_recursive(
        self,
        folder_id: str = "root",
        max_depth: int = 10,
        _depth: int = 0,
    ) -> Iterator[CloudFile]:
        for df in self._client.list_all_recursive(
            folder_id=folder_id,
            _depth=_depth,
            _max_depth=max_depth,
        ):
            yield self._to_cloud_file(df)

    # ── 查找 ─────────────────────────────────────────────

    def get_file(self, file_id: str) -> CloudFile:
        return self._to_cloud_file(self._client.get_file(file_id))

    def read_text(self, file: CloudFile) -> Optional[str]:
        try:
            request = self._client._svc.files().get_media(fileId=file.id)
            content = self._client._execute(request)
            if isinstance(content, bytes):
                return content.decode("utf-8", errors="ignore")
            return str(content)
        except Exception:
            return None

    def find_file(
        self,
        name: str,
        folder_id: Optional[str] = None,
    ) -> Optional[CloudFile]:
        result = self._client.find_file(name, folder_id=folder_id)
        return self._to_cloud_file(result) if result else None

    # ── 修改 ─────────────────────────────────────────────

    def rename_file(self, file_id: str, new_name: str) -> CloudFile:
        return self._to_cloud_file(self._client.rename_file(file_id, new_name))

    def move_file(
        self,
        file_id: str,
        new_folder_id: str,
        new_name: Optional[str] = None,
    ) -> CloudFile:
        return self._to_cloud_file(
            self._client.move_file(
                file_id=file_id,
                new_folder_id=new_folder_id,
                new_name=new_name,
            )
        )

    # ── 上传 ─────────────────────────────────────────────

    def upload_text(
        self,
        content: str,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "text/xml",
        overwrite: bool = False,
    ) -> CloudFile:
        return self._to_cloud_file(
            self._client.upload_text(
                content=content,
                name=name,
                parent_id=parent_id,
                mime_type=mime_type,
                overwrite=overwrite,
            )
        )

    def upload_bytes(
        self,
        content: bytes,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "image/jpeg",
        overwrite: bool = True,
    ) -> CloudFile:
        return self._to_cloud_file(
            self._client.upload_bytes(
                content=content,
                name=name,
                parent_id=parent_id,
                mime_type=mime_type,
                overwrite=overwrite,
            )
        )

    # ── 删除 ─────────────────────────────────────────────

    def trash_file(self, file_id: str) -> CloudFile:
        return self._to_cloud_file(self._client.trash_file(file_id))

    def delete_file(self, file_id: str) -> None:
        self._client.delete_file(file_id)

    # ── 文件夹 ───────────────────────────────────────────

    def create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> CloudFile:
        return self._to_cloud_file(self._client.create_folder(name, parent_id))

    def get_or_create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> CloudFile:
        return self._to_cloud_file(
            self._client.get_or_create_folder(name, parent_id)
        )

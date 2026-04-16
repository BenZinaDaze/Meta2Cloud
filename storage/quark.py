"""
storage/quark.py —— 夸克网盘 Storage Provider 实现

内部包装 uquark/client.py 的 QuarkClient，
将 QuarkFile 映射为统一的 CloudFile。
"""

from __future__ import annotations

import logging
import os
import tempfile
from typing import Callable, Iterator, List, Optional

from uquark.client import QuarkClient
from uquark.errors import QuarkApiError
from uquark.models import QuarkFile
from uquark.runtime import get_runtime_manager
from storage.base import CloudFile, FileType, StorageProvider

logger = logging.getLogger(__name__)


class QuarkProvider(StorageProvider):
    """夸克网盘存储 Provider"""

    def __init__(
        self,
        client: Optional[QuarkClient] = None,
        *,
        client_getter: Optional[Callable[[], QuarkClient]] = None,
    ):
        if client is None and client_getter is None:
            raise ValueError("QuarkProvider 需要 client 或 client_getter")
        self._client = client
        self._client_getter = client_getter

    @property
    def provider_name(self) -> str:
        return "quark"

    @classmethod
    def from_config(cls, cfg) -> "QuarkProvider":
        """从 Config 对象构造（读取 quark 配置段）"""
        quark_cfg = cfg.quark
        runtime = get_runtime_manager()
        token_path = os.path.abspath(quark_cfg.token_json)
        client = runtime.get_client(token_path=token_path)
        return cls(client)

    @classmethod
    def from_client_getter(
        cls,
        client_getter: Callable[[], QuarkClient],
    ) -> "QuarkProvider":
        """从共享 client getter 构造，适合 WebUI 等长生命周期场景。"""
        return cls(client_getter=client_getter)

    def _current_client(self) -> QuarkClient:
        if self._client_getter is not None:
            return self._client_getter()
        assert self._client is not None
        return self._client

    @property
    def raw_client(self) -> QuarkClient:
        """暴露底层 QuarkClient，供需要平台特有功能的场景使用"""
        return self._current_client()

    # ── 类型转换 ─────────────────────────────────────────

    @staticmethod
    def _to_cloud_file(qf: QuarkFile) -> CloudFile:
        """将 QuarkFile 转换为统一的 CloudFile"""
        parent_id = qf.parent_id or qf.pdir_fid
        return CloudFile(
            id=qf.id,
            name=qf.name,
            file_type=FileType.FOLDER if qf.is_folder else FileType.FILE,
            size=qf.size,
            modified_time=str(qf.modified_time) if qf.modified_time else None,
            parent_id=parent_id,
            parents=[parent_id] if parent_id else [],
            mime_type=qf.mime_type,
            trashed=qf.trashed,
            extra={
                k: v for k, v in {
                    "sha1": qf.sha1,
                    "md5": qf.md5,
                    "suffix": qf.suffix,
                    "drive_id": qf.drive_id,
                }.items() if v is not None
            },
        )

    # ── 列举 ─────────────────────────────────────────────

    def list_files(
        self,
        folder_id: str = "0",
        page_size: int = 100,
    ) -> List[CloudFile]:
        """
        列举文件夹直接子项（不递归）。

        注意：夸克网盘的根目录 ID 是 "0"。
        """
        # 兼容 "root" → "0" 映射
        fid = "0" if folder_id == "root" else folder_id
        files, _ = self._current_client().list_files(fid, limit=page_size)
        return [self._to_cloud_file(f) for f in files]

    def list_all_recursive(
        self,
        folder_id: str = "0",
        max_depth: int = 10,
        _depth: int = 0,
    ) -> Iterator[CloudFile]:
        if _depth > max_depth:
            return

        fid = "0" if folder_id == "root" else folder_id
        for qf in self._current_client().list_recursive(fid, max_depth=max_depth - _depth):
            yield self._to_cloud_file(qf)

    # ── 查找 ─────────────────────────────────────────────

    def get_file(self, file_id: str) -> CloudFile:
        """获取单个文件的元数据"""
        qf = self._current_client().get_file(file_id)
        if qf is None:
            raise QuarkApiError(f"未找到文件：{file_id}")
        return self._to_cloud_file(qf)

    def read_text(self, file: CloudFile) -> Optional[str]:
        """读取文本文件内容"""
        try:
            download_url = self._current_client().get_download_url(file.id)
            if not download_url:
                return None
            import requests
            resp = requests.get(download_url, timeout=self._current_client().timeout)
            resp.raise_for_status()
            return resp.content.decode("utf-8", errors="ignore")
        except Exception:
            return None

    def find_file(
        self,
        name: str,
        folder_id: Optional[str] = None,
    ) -> Optional[CloudFile]:
        """按精确文件名查找"""
        fid = folder_id or "0"
        if fid == "root":
            fid = "0"
        qf = self._current_client().find_file(name, folder_id=fid)
        if qf is None:
            # 回退：搜索
            results, _ = self._current_client().search(name, folder_id=fid, limit=10)
            for r in results:
                if r.name == name:
                    return self._to_cloud_file(r)
            return None
        return self._to_cloud_file(qf)

    # ── 修改 ─────────────────────────────────────────────

    def rename_file(self, file_id: str, new_name: str) -> CloudFile:
        client = self._current_client()
        ok = client.rename(file_id, new_name)
        if not ok:
            raise QuarkApiError(f"重命名失败：{file_id} → {new_name}")
        updated = client.get_file(file_id)
        if updated:
            return self._to_cloud_file(updated)
        return CloudFile(id=file_id, name=new_name, file_type=FileType.FILE)

    def move_file(
        self,
        file_id: str,
        new_folder_id: str,
        new_name: Optional[str] = None,
    ) -> CloudFile:
        client = self._current_client()
        ok = client.move(file_id, new_folder_id, target_name=new_name)
        if not ok:
            raise QuarkApiError(f"移动失败：{file_id} → {new_folder_id}")
        updated = client.get_file(file_id)
        if updated:
            return self._to_cloud_file(updated)
        return CloudFile(
            id=file_id,
            name=new_name or file_id,
            file_type=FileType.FILE,
            parent_id=new_folder_id,
            parents=[new_folder_id],
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
        """上传文本内容为文件"""
        fid = parent_id or "0"
        if fid == "root":
            fid = "0"

        # 如果 overwrite，先查找并删除已有文件
        if overwrite:
            existing = self.find_file(name, folder_id=fid)
            if existing:
                try:
                    self._current_client().delete(existing.id)
                except Exception:
                    logger.warning("删除已有文件失败 [%s]，继续上传", name)

        data = content.encode("utf-8")
        return self._upload_data(data, name, fid)

    def upload_bytes(
        self,
        content: bytes,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "image/jpeg",
        overwrite: bool = True,
    ) -> CloudFile:
        """上传二进制内容为文件"""
        fid = parent_id or "0"
        if fid == "root":
            fid = "0"

        if overwrite:
            existing = self.find_file(name, folder_id=fid)
            if existing:
                try:
                    self._current_client().delete(existing.id)
                except Exception:
                    logger.warning("删除已有文件失败 [%s]，继续上传", name)

        return self._upload_data(content, name, fid)

    def _upload_data(self, data: bytes, name: str, fid: str) -> CloudFile:
        """写临时文件 → 夸克初始化上传 → 返回 CloudFile"""
        with tempfile.NamedTemporaryFile(delete=False, suffix=os.path.splitext(name)[1]) as tmp:
            tmp.write(data)
            tmp_path = tmp.name

        try:
            client = self._current_client()
            upload_info = client.get_upload_url(
                file_name=name,
                file_size=len(data),
                parent_id=fid,
            )

            # 上传完成后查找文件
            uploaded = self.find_file(name, folder_id=fid)
            if uploaded:
                return uploaded

            return CloudFile(
                id=str(upload_info.get("file_id", "")),
                name=name,
                file_type=FileType.FILE,
                size=len(data),
                parent_id=fid,
                parents=[fid],
            )
        finally:
            try:
                os.unlink(tmp_path)
            except OSError:
                pass

    # ── 删除 ─────────────────────────────────────────────

    def trash_file(self, file_id: str) -> CloudFile:
        """移动到回收站"""
        info = CloudFile(id=file_id, name=file_id, file_type=FileType.FILE, trashed=True)
        self._current_client().delete(file_id)
        return info

    def delete_file(self, file_id: str) -> None:
        """彻底删除文件"""
        # 夸克网盘 API 可能不区分彻底删除，这里直接移到回收站
        self._current_client().delete(file_id)

    # ── 文件夹 ───────────────────────────────────────────

    def create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> CloudFile:
        pid = parent_id or "0"
        if pid == "root":
            pid = "0"
        new_id = self._current_client().create_folder(name, pid)
        return CloudFile(
            id=new_id or "",
            name=name,
            file_type=FileType.FOLDER,
            parent_id=pid,
            parents=[pid],
        )

    def get_or_create_folder(
        self,
        name: str,
        parent_id: Optional[str] = None,
    ) -> CloudFile:
        """先查找，不存在则创建"""
        pid = parent_id or "0"
        if pid == "root":
            pid = "0"
        existing = self.find_file(name, folder_id=pid)
        if existing and existing.is_folder:
            return existing
        new_id = self._current_client().create_folder(name, pid)
        return CloudFile(
            id=new_id or "",
            name=name,
            file_type=FileType.FOLDER,
            parent_id=pid,
            parents=[pid],
        )

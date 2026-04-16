"""
storage/pan115.py —— 115 网盘 Provider 实现

内部包装现有 u115pan/client.py 的 Pan115Client，
将 Pan115File 映射为统一的 CloudFile。

现有 Pan115Client 完全保持不变，本层只做适配。

注意：upload_text / upload_bytes 需要 115 开放平台的秒传/分片上传流程，
当前实现为先写本地临时文件再调用 init_upload + 对象存储上传，
暂时标记为 NotImplementedError，后续按需实现。
"""

from __future__ import annotations

import logging
import os
import tempfile
import base64
from typing import Callable, Iterator, List, Optional

from u115pan.client import Pan115Client
from u115pan.errors import Pan115ApiError
from u115pan.models import Pan115File
from u115pan.runtime import get_runtime_manager
from storage.base import CloudFile, FileType, StorageProvider

logger = logging.getLogger(__name__)


class Pan115Provider(StorageProvider):
    """115 网盘存储 Provider"""

    def __init__(
        self,
        client: Optional[Pan115Client] = None,
        *,
        client_getter: Optional[Callable[[], Pan115Client]] = None,
    ):
        if client is None and client_getter is None:
            raise ValueError("Pan115Provider 需要 client 或 client_getter")
        self._client = client
        self._client_getter = client_getter

    @property
    def provider_name(self) -> str:
        return "pan115"

    @classmethod
    def from_config(cls, cfg) -> "Pan115Provider":
        """从 Config 对象构造（读取 u115 配置段）"""
        u115_cfg = cfg.u115
        runtime = get_runtime_manager()
        token_path = os.path.abspath(u115_cfg.token_json)
        client = runtime.get_client(
            client_id=u115_cfg.client_id,
            token_path=token_path,
        )
        return cls(client)

    @classmethod
    def from_client_getter(
        cls,
        client_getter: Callable[[], Pan115Client],
    ) -> "Pan115Provider":
        """从共享 client getter 构造，适合 WebUI 等长生命周期场景。"""
        return cls(client_getter=client_getter)

    def _current_client(self) -> Pan115Client:
        if self._client_getter is not None:
            return self._client_getter()
        assert self._client is not None
        return self._client

    @property
    def raw_client(self) -> Pan115Client:
        """暴露底层 Pan115Client，供需要平台特有功能的场景使用"""
        return self._current_client()

    # ── 类型转换 ─────────────────────────────────────────

    @staticmethod
    def _to_cloud_file(pf: Pan115File) -> CloudFile:
        """将 Pan115File 转换为统一的 CloudFile"""
        parent_id = pf.parent_id
        return CloudFile(
            id=pf.id,
            name=pf.name,
            file_type=FileType.FOLDER if pf.is_folder else FileType.FILE,
            size=pf.size,
            modified_time=str(pf.modified_time) if pf.modified_time else None,
            parent_id=parent_id,
            parents=[parent_id] if parent_id else [],
            mime_type=None,  # 115 API 不返回 MIME，依赖扩展名判断
            extra={
                k: v for k, v in {
                    "pick_code": pf.pick_code,
                    "sha1": pf.sha1,
                    "ico": pf.ico,
                }.items() if v is not None
            },
        )

    # ── 列举 ─────────────────────────────────────────────

    def list_files(
        self,
        folder_id: str = "0",
        page_size: int = 1000,
    ) -> List[CloudFile]:
        """
        列举文件夹直接子项（不递归）。

        注意：115 的根目录 ID 是 "0"，而非 "root"。
        """
        # 兼容 "root" → "0" 映射
        cid = "0" if folder_id == "root" else folder_id
        raw = self._current_client().list_files(cid=cid, limit=page_size)
        return [self._to_cloud_file(f) for f in raw]

    def list_all_recursive(
        self,
        folder_id: str = "0",
        max_depth: int = 10,
        _depth: int = 0,
    ) -> Iterator[CloudFile]:
        if _depth > max_depth:
            return

        cid = "0" if folder_id == "root" else folder_id
        items = [self._to_cloud_file(f) for f in self._current_client().list_all_files(cid=cid, limit=1000)]
        for item in items:
            yield item
            if item.is_folder:
                yield from self.list_all_recursive(
                    folder_id=item.id,
                    max_depth=max_depth,
                    _depth=_depth + 1,
                )

    # ── 查找 ─────────────────────────────────────────────

    def get_file(self, file_id: str) -> CloudFile:
        """
        获取单个文件的元数据。

        115 没有直接的 get_by_id API，使用搜索或 path_info 作为替代。
        这里用 search 按文件 ID 查找（回退到 list_files 扫描父目录）。
        """
        # 尝试通过 search 查找
        results = self._current_client().search(file_id, limit=1)
        for r in results:
            if r.id == file_id:
                return self._to_cloud_file(r)
        raise Pan115ApiError(f"未找到文件：{file_id}")

    def read_text(self, file: CloudFile) -> Optional[str]:
        pick_code = file.extra.get("pick_code")
        if not pick_code:
            return None
        try:
            client = self._current_client()
            download_url = client.get_download_url(str(pick_code))
            resp = client.session.get(download_url, timeout=client.timeout)
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
        cid = folder_id or "0"
        if cid == "root":
            cid = "0"
        results = self._current_client().search(name, cid=cid, limit=10)
        for r in results:
            if r.name == name:
                return self._to_cloud_file(r)
        # 115 搜索结果存在延迟，回退到直接列目录做精确匹配
        try:
            for item in self.list_files(folder_id=cid, page_size=1000):
                if item.name == name:
                    return item
        except Exception:
            pass
        return None

    # ── 修改 ─────────────────────────────────────────────

    def rename_file(self, file_id: str, new_name: str) -> CloudFile:
        client = self._current_client()
        client.rename(file_id, new_name)
        results = client.search(new_name, limit=20)
        for r in results:
            if r.id == file_id:
                return self._to_cloud_file(r)
        return CloudFile(id=file_id, name=new_name, file_type=FileType.FILE)

    def move_file(
        self,
        file_id: str,
        new_folder_id: str,
        new_name: Optional[str] = None,
    ) -> CloudFile:
        # 先移动
        client = self._current_client()
        client.move(file_id, to_cid=new_folder_id)
        # 再重命名（如果需要）
        if new_name:
            client.rename(file_id, new_name)
            moved = self.find_file(new_name, folder_id=new_folder_id)
            if moved:
                return moved

        for item in self.list_files(folder_id=new_folder_id, page_size=1000):
            if item.id == file_id:
                return item

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
        """
        上传文本内容为文件。

        115 的上传流程较复杂（秒传 → 分片上传），
        这里通过写入临时文件再走标准上传流程实现。
        """
        cid = parent_id or "0"
        if cid == "root":
            cid = "0"

        # 如果 overwrite，先查找并删除已有文件
        if overwrite:
            existing = self.find_file(name, folder_id=cid)
            if existing:
                try:
                    self._current_client().delete(existing.id)
                except Exception:
                    logger.warning("删除已有文件失败 [%s]，继续上传", name)

        # 写临时文件并上传
        data = content.encode("utf-8")
        return self._upload_data(data, name, cid)

    def upload_bytes(
        self,
        content: bytes,
        name: str,
        parent_id: Optional[str] = None,
        mime_type: str = "image/jpeg",
        overwrite: bool = True,
    ) -> CloudFile:
        """上传二进制内容为文件"""
        cid = parent_id or "0"
        if cid == "root":
            cid = "0"

        if overwrite:
            existing = self.find_file(name, folder_id=cid)
            if existing:
                try:
                    self._current_client().delete(existing.id)
                except Exception:
                    logger.warning("删除已有文件失败 [%s]，继续上传", name)

        return self._upload_data(content, name, cid)

    def _upload_data(self, data: bytes, name: str, cid: str) -> CloudFile:
        """写临时文件 → 115 初始化上传 → 必要时走 OSS 分片上传 → 返回 CloudFile"""
        with tempfile.NamedTemporaryFile(delete=False, suffix=os.path.splitext(name)[1]) as tmp:
            tmp.write(data)
            tmp_path = tmp.name

        try:
            hashes = Pan115Client.compute_upload_hashes(tmp_path)
            client = self._current_client()
            init_info = client.init_upload(
                file_name=name,
                file_size=hashes.file_size,
                cid=cid,
                fileid=hashes.fileid,
                preid=hashes.preid,
            )

            if init_info.is_fast_upload and init_info.file_id:
                # 秒传成功
                return CloudFile(
                    id=init_info.file_id,
                    name=name,
                    file_type=FileType.FILE,
                    size=hashes.file_size,
                    parent_id=cid,
                    parents=[cid],
                )

            if init_info.requires_second_check:
                sign_val = Pan115Client.compute_sign_val(tmp_path, init_info.sign_check)
                init_info = client.init_upload(
                    file_name=name,
                    file_size=hashes.file_size,
                    cid=cid,
                    fileid=hashes.fileid,
                    preid=hashes.preid,
                    pick_code=init_info.pick_code,
                    sign_key=init_info.sign_key,
                    sign_val=sign_val,
                )
                if init_info.is_fast_upload and init_info.file_id:
                    return CloudFile(
                        id=init_info.file_id,
                        name=name,
                        file_type=FileType.FILE,
                        size=hashes.file_size,
                        parent_id=cid,
                        parents=[cid],
                    )

            if not init_info.bucket or not init_info.object_name or not init_info.pick_code:
                raise Pan115ApiError(f"115 上传初始化信息不完整：{name}")

            self._multipart_upload(
                file_path=tmp_path,
                file_size=hashes.file_size,
                cid=cid,
                fileid=hashes.fileid,
                bucket_name=init_info.bucket,
                object_name=init_info.object_name,
                pick_code=init_info.pick_code,
                callback=init_info.callback or {},
            )

            uploaded = self.find_file(name, folder_id=cid)
            if uploaded:
                return uploaded

            return CloudFile(
                id=init_info.file_id or init_info.pick_code,
                name=name,
                file_type=FileType.FILE,
                size=hashes.file_size,
                parent_id=cid,
                parents=[cid],
                extra={"pick_code": init_info.pick_code},
            )
        finally:
            try:
                os.unlink(tmp_path)
            except OSError:
                pass

    def _multipart_upload(
        self,
        *,
        file_path: str,
        file_size: int,
        cid: str,
        fileid: str,
        bucket_name: str,
        object_name: str,
        pick_code: str,
        callback: dict,
    ) -> None:
        try:
            import oss2
            from oss2 import SizedFileAdapter, determine_part_size
            from oss2.models import PartInfo
        except ImportError as exc:
            raise Pan115ApiError("缺少 oss2 依赖，无法执行 115 分片上传") from exc

        client = self._current_client()
        token = client.get_upload_token()
        resume_info = client.resume_upload(
            file_size=file_size,
            cid=cid,
            fileid=fileid,
            pick_code=pick_code,
        )
        if resume_info.callback:
            callback = resume_info.callback

        auth = oss2.StsAuth(
            access_key_id=token.access_key_id,
            access_key_secret=token.access_key_secret,
            security_token=token.security_token,
        )
        bucket = oss2.Bucket(auth, token.endpoint, bucket_name)
        part_size = determine_part_size(file_size, preferred_size=10 * 1024 * 1024)
        upload_id = bucket.init_multipart_upload(
            object_name,
            params={"encoding-type": "url", "sequential": ""},
        ).upload_id

        parts = []
        with open(file_path, "rb") as fileobj:
            part_number = 1
            offset = 0
            while offset < file_size:
                num_to_upload = min(part_size, file_size - offset)
                result = bucket.upload_part(
                    object_name,
                    upload_id,
                    part_number,
                    data=SizedFileAdapter(fileobj, num_to_upload),
                )
                parts.append(PartInfo(part_number, result.etag))
                offset += num_to_upload
                part_number += 1

        headers = {"x-oss-forbid-overwrite": "false"}
        callback_body = callback.get("callback")
        callback_var = callback.get("callback_var")
        if callback_body:
            headers["X-oss-callback"] = base64.b64encode(
                str(callback_body).encode("utf-8")
            ).decode("utf-8")
        if callback_var:
            headers["x-oss-callback-var"] = base64.b64encode(
                str(callback_var).encode("utf-8")
            ).decode("utf-8")

        try:
            bucket.complete_multipart_upload(
                object_name,
                upload_id,
                parts,
                headers=headers,
            )
        except oss2.exceptions.OssError as exc:
            if getattr(exc, "code", None) == "FileAlreadyExists":
                logger.warning("115 OSS 上传目标已存在 [%s]", object_name)
                return
            raise Pan115ApiError(f"115 OSS 分片上传失败：{exc}") from exc

    # ── 删除 ─────────────────────────────────────────────

    def trash_file(self, file_id: str) -> CloudFile:
        """
        115 没有回收站移入 API（只有列出和清空），
        此处用 delete 代替。
        """
        info = CloudFile(id=file_id, name=file_id, file_type=FileType.FILE, trashed=True)
        self._current_client().delete(file_id)
        return info

    def delete_file(self, file_id: str) -> None:
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
        new_id = self._current_client().create_folder(pid, name)
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
        """先查找，不存在则创建（115 的 create_folder 本身支持幂等）"""
        pid = parent_id or "0"
        if pid == "root":
            pid = "0"
        # 115 的 create_folder 已经是幂等的（已存在返回已有 ID）
        new_id = self._current_client().create_folder(pid, name)
        if new_id:
            return CloudFile(
                id=new_id,
                name=name,
                file_type=FileType.FOLDER,
                parent_id=pid,
                parents=[pid],
            )
        # 回退：手动查找
        existing = self.find_file(name, folder_id=pid)
        if existing and existing.is_folder:
            return existing
        raise Pan115ApiError(f"创建文件夹失败：{name}")

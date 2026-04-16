"""
uquark/client.py —— 夸克网盘 API 客户端

基于夸克网盘开放 API 封装，提供文件管理、搜索、上传、下载等操作。
"""

from __future__ import annotations

import hashlib
import os
import random
import threading
import time
from typing import Any, Callable, Iterator, Optional
from urllib.parse import urljoin

import requests

from .auth import QuarkAuth, load_token
from .errors import QuarkApiError, QuarkAuthError
from .models import QuarkFile, QuarkSpaceInfo, QuarkToken


class QuarkClient:
    """夸克网盘客户端"""

    API_BASE = "https://drive-pc.quark.cn"
    APP_ID = "168830"
    DEVICE_ID = "pc"

    def __init__(
        self,
        token: Optional[QuarkToken] = None,
        token_path: Optional[str] = None,
        on_token_updated: Optional[Callable[[QuarkToken], None]] = None,
        timeout: int = 30,
        user_agent: str = "QuarkCloudStorage/2.0",
        session: Optional[requests.Session] = None,
        api_qps: float = 5.0,
    ) -> None:
        self.token = token
        self.token_path = token_path
        self.on_token_updated = on_token_updated
        self.timeout = timeout
        self.api_qps = api_qps

        self._next_api_at = 0.0
        self._rate_lock = threading.Lock()

        self.session = session or requests.Session()
        self.session.headers.update(
            {
                "User-Agent": user_agent,
                "Accept-Encoding": "gzip, deflate",
            }
        )

    @classmethod
    def from_token_file(
        cls,
        token_path: str,
        **kwargs: Any,
    ) -> "QuarkClient":
        """从本地 token JSON 构造客户端"""
        token = load_token(token_path)
        return cls(token=token, token_path=token_path, **kwargs)

    @property
    def auth(self) -> QuarkAuth:
        """获取认证对象"""
        return QuarkAuth(
            token=self.token,
            token_path=self.token_path,
            on_token_updated=self._handle_token_updated,
        )

    def _handle_token_updated(self, token: QuarkToken) -> None:
        """Token 更新回调"""
        self.token = token
        if self.on_token_updated:
            self.on_token_updated(token)

    def _sleep_rate_limit(self) -> None:
        wait_seconds = 0.0
        with self._rate_lock:
            now = time.time()
            wait_seconds = max(0.0, self._next_api_at - now)
            self._next_api_at = max(self._next_api_at, now) + (1.0 / self.api_qps if self.api_qps > 0 else 0.0)
        if wait_seconds > 0:
            time.sleep(wait_seconds)

    def _make_headers(self, *, auth_required: bool = True) -> dict[str, str]:
        headers: dict[str, str] = {
            "Content-Type": "application/json",
        }
        if auth_required:
            token = self.auth.get_valid_access_token()
            headers["Authorization"] = f"Bearer {token}"
        headers["X-Device-ID"] = self.DEVICE_ID
        headers["X-App-Id"] = self.APP_ID
        return headers

    def _request(
        self,
        method: str,
        path: str,
        *,
        params: Optional[dict[str, Any]] = None,
        json_body: Optional[dict[str, Any]] = None,
        data: Optional[dict[str, Any]] = None,
        auth_required: bool = True,
        max_attempts: int = 3,
    ) -> dict[str, Any]:
        url = urljoin(self.API_BASE, path)
        delay = 1.0

        for attempt in range(1, max_attempts + 1):
            self._sleep_rate_limit()
            headers = self._make_headers(auth_required=auth_required)

            try:
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    json=json_body,
                    data=data,
                    headers=headers,
                    timeout=self.timeout,
                )

                if response.status_code == 429:
                    with self._rate_lock:
                        self._next_api_at = time.time() + 60
                    raise QuarkApiError("请求过于频繁，请稍后重试", code=429)

                response.raise_for_status()
                payload = response.json()

                code = payload.get("code")
                if code == 401 or code == 403:
                    raise QuarkAuthError(
                        payload.get("message", "认证失败"),
                        code=code,
                        payload=payload,
                    )

                if code != 0:
                    raise QuarkApiError(
                        payload.get("message", "请求失败"),
                        code=code,
                        payload=payload,
                    )

                return payload

            except requests.exceptions.ConnectionError:
                if attempt >= max_attempts:
                    raise QuarkApiError("网络连接失败")
                time.sleep(delay + random.random())
                delay *= 2
            except requests.exceptions.Timeout:
                if attempt >= max_attempts:
                    raise QuarkApiError("请求超时")
                time.sleep(delay + random.random())
                delay *= 2

        raise QuarkApiError("请求失败")

    def list_files(
        self,
        folder_id: str = "0",
        *,
        limit: int = 100,
        page_token: Optional[str] = None,
    ) -> tuple[list[QuarkFile], Optional[str]]:
        """
        列出文件夹内容

        Returns:
            (files, next_page_token)
        """
        payload = self._request(
            "POST",
            "/1.0/file/search",
            json_body={
                "drive_id": 0,
                "folder_id": folder_id,
                "order_by": 1,
                "order_dir": 1,
                "limit": limit,
                "page_token": page_token,
            },
        )

        data = payload.get("data", {})
        items = data.get("list", [])
        files = [QuarkFile.from_list_item(item) for item in items]
        next_token = data.get("next_page_token")

        return files, next_token

    def list_all_files(
        self,
        folder_id: str = "0",
        *,
        limit: int = 100,
    ) -> Iterator[QuarkFile]:
        """自动翻页列出目录全部内容"""
        page_token = None
        while True:
            files, page_token = self.list_files(folder_id, limit=limit, page_token=page_token)
            yield from files
            if not page_token or len(files) < limit:
                break

    def list_recursive(
        self,
        folder_id: str = "0",
        *,
        max_depth: int = 10,
        current_depth: int = 0,
    ) -> Iterator[QuarkFile]:
        """递归遍历文件夹"""
        if current_depth > max_depth:
            return

        for file in self.list_all_files(folder_id, limit=100):
            yield file
            if file.is_folder:
                yield from self.list_recursive(
                    file.id,
                    max_depth=max_depth,
                    current_depth=current_depth + 1,
                )

    def get_file(self, file_id: str) -> Optional[QuarkFile]:
        """获取文件详情"""
        try:
            payload = self._request(
                "GET",
                f"/1.0/file/get",
                params={"file_id": file_id},
            )
            data = payload.get("data", {})
            if data:
                return QuarkFile.from_detail(data)
            return None
        except QuarkApiError:
            return None

    def find_file(
        self,
        name: str,
        folder_id: Optional[str] = None,
        limit: int = 20,
    ) -> Optional[QuarkFile]:
        """按文件名模糊搜索"""
        payload = self._request(
            "POST",
            "/1.0/file/search",
            json_body={
                "drive_id": 0,
                "folder_id": folder_id or "0",
                "file_name": name,
                "limit": limit,
            },
        )

        data = payload.get("data", {})
        items = data.get("list", [])
        for item in items:
            f = QuarkFile.from_list_item(item)
            if f.name == name:
                return f
        return None

    def search(
        self,
        keyword: str,
        *,
        folder_id: Optional[str] = None,
        file_type: Optional[int] = None,  # 1=文件夹, 2=文件
        limit: int = 20,
        page_token: Optional[str] = None,
    ) -> tuple[list[QuarkFile], Optional[str]]:
        """搜索文件"""
        body: dict[str, Any] = {
            "drive_id": 0,
            "file_name": keyword,
            "limit": limit,
            "page_token": page_token,
        }
        if folder_id:
            body["folder_id"] = folder_id
        if file_type is not None:
            body["type"] = file_type

        payload = self._request("POST", "/1.0/file/search", json_body=body)

        data = payload.get("data", {})
        items = data.get("list", [])
        files = [QuarkFile.from_list_item(item) for item in items]
        next_token = data.get("next_page_token")

        return files, next_token

    def create_folder(self, name: str, parent_id: str = "0") -> str:
        """创建文件夹，返回新文件夹 ID"""
        payload = self._request(
            "POST",
            "/1.0/file",
            json_body={
                "drive_id": 0,
                "parent_id": parent_id,
                "name": name,
                "type": "folder",
            },
        )
        data = payload.get("data", {})
        return str(data.get("file_id", ""))

    def rename(self, file_id: str, new_name: str) -> bool:
        """重命名文件或文件夹"""
        try:
            self._request(
                "PUT",
                f"/1.0/file/{file_id}",
                json_body={
                    "drive_id": 0,
                    "file_id": file_id,
                    "name": new_name,
                },
            )
            return True
        except QuarkApiError:
            return False

    def move(
        self,
        file_id: str,
        target_folder_id: str,
        target_name: Optional[str] = None,
    ) -> bool:
        """移动文件到目标文件夹"""
        body: dict[str, Any] = {
            "drive_id": 0,
            "file_id": file_id,
            "to_parent_id": target_folder_id,
        }
        if target_name:
            body["name"] = target_name

        try:
            self._request(
                "POST",
                "/1.0/file/update",
                json_body=body,
            )
            return True
        except QuarkApiError:
            return False

    def copy(
        self,
        file_id: str,
        target_folder_id: str,
        target_name: Optional[str] = None,
    ) -> bool:
        """复制文件到目标文件夹"""
        body: dict[str, Any] = {
            "drive_id": 0,
            "file_id": file_id,
            "to_parent_id": target_folder_id,
        }
        if target_name:
            body["name"] = target_name

        try:
            self._request(
                "POST",
                "/1.0/file/copy",
                json_body=body,
            )
            return True
        except QuarkApiError:
            return False

    def delete(self, file_id: str) -> bool:
        """删除文件到回收站"""
        try:
            self._request(
                "DELETE",
                f"/1.0/file/{file_id}",
                json_body={"drive_id": 0, "file_id": file_id},
            )
            return True
        except QuarkApiError:
            return False

    def get_download_url(self, file_id: str) -> Optional[str]:
        """获取文件下载链接"""
        try:
            payload = self._request(
                "GET",
                "/1.0/file/download",
                params={"file_id": file_id},
            )
            data = payload.get("data", {})
            return data.get("download_url") or data.get("url")
        except QuarkApiError:
            return None

    def get_upload_url(
        self,
        file_name: str,
        file_size: int,
        parent_id: str = "0",
        *,
        chunk_size: int = 10485760,  # 10MB
    ) -> dict[str, Any]:
        """获取上传预签名 URL"""
        payload = self._request(
            "POST",
            "/1.0/file/upload",
            json_body={
                "drive_id": 0,
                "parent_id": parent_id,
                "name": file_name,
                "size": file_size,
                "type": "file",
            },
        )
        return payload.get("data", {})

    def upload_file(
        self,
        file_path: str,
        file_name: str,
        parent_id: str = "0",
    ) -> Optional[str]:
        """
        上传本地文件到云盘，返回 file_id

        流程：获取上传URL → PUT上传文件 → 确认完成
        """
        file_size = os.path.getsize(file_path)
        upload_info = self.get_upload_url(file_name, file_size, parent_id)

        upload_url = upload_info.get("upload_url")
        file_id = upload_info.get("file_id")

        if not upload_url:
            raise QuarkApiError("未获取到上传URL", payload=upload_info)

        # PUT 文件内容到预签名 URL
        with open(file_path, "rb") as f:
            resp = self.session.put(upload_url, data=f, timeout=self.timeout * 10)
            resp.raise_for_status()

        # 确认上传完成
        if file_id:
            self.upload_complete(file_id, parent_id)

        return file_id

    def upload_complete(
        self,
        file_id: str,
        parent_id: str = "0",
    ) -> bool:
        """通知上传完成"""
        try:
            self._request(
                "POST",
                "/1.0/file/upload/complete",
                json_body={
                    "drive_id": 0,
                    "file_id": file_id,
                    "parent_id": parent_id,
                },
            )
            return True
        except QuarkApiError:
            return False

    def get_space_info(self) -> QuarkSpaceInfo:
        """获取空间使用信息"""
        payload = self._request("GET", "/1.0/space/info")
        data = payload.get("data", {})
        return QuarkSpaceInfo(
            total_size=int(data.get("total_size", 0)),
            used_size=int(data.get("used_size", 0)),
            remain_size=int(data.get("remain_size", 0)),
            raw=data,
        )

    def get_path_info(self, path: str) -> Optional[QuarkFile]:
        """
        按完整路径查询文件或目录详情。

        注意：夸克网盘 API 不支持直接按路径查询，需要逐级遍历。
        """
        if not path.startswith("/"):
            path = "/" + path

        parts = [p for p in path.split("/") if p]
        if not parts:
            return None

        parent_id = "0"
        current_file: Optional[QuarkFile] = None

        for part in parts:
            found = self.find_file(part, folder_id=parent_id)
            if not found:
                return None
            current_file = found
            parent_id = found.id

        return current_file

    @staticmethod
    def compute_file_hash(file_path: str) -> str:
        """计算文件 MD5（用于秒传）"""
        md5 = hashlib.md5()
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(8192), b""):
                md5.update(chunk)
        return md5.hexdigest()

    @staticmethod
    def compute_sha1(file_path: str) -> str:
        """计算文件 SHA1"""
        sha1 = hashlib.sha1()
        with open(file_path, "rb") as f:
            for chunk in iter(lambda: f.read(8192), b""):
                sha1.update(chunk)
        return sha1.hexdigest()

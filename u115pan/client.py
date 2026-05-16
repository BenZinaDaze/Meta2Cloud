"""
u115pan/client.py —— 115 开放平台通用外部接口封装

设计目标：
  - 独立于当前主程序，不接入 drive / pipeline 流程
  - 覆盖扫码授权、基础文件操作、下载、上传前置接口、空间查询
  - 保持尽量薄的 HTTP 包装，方便后续接入或二次扩展
"""

from __future__ import annotations

import hashlib
import os
import random
import threading
import time
from typing import Any, Callable, Optional
from urllib.parse import parse_qs, unquote, urljoin, urlparse

import requests

from .auth import (
    generate_code_challenge,
    generate_code_verifier,
    load_token,
    save_token,
)
from .errors import Pan115ApiError, Pan115AuthError, Pan115RateLimitError
from .models import (
    DeviceCodeSession,
    Pan115File,
    Pan115SpaceInfo,
    Pan115Token,
    QrcodeStatus,
    ReceiveResult,
    RecycleBinItem,
    ResumeInfo,
    ShareInfo,
    ShareItem,
    UploadHashes,
    UploadInitInfo,
    UploadToken,
)


class Pan115Client:
    """115 开放平台客户端"""

    PASSPORT_API = "https://passportapi.115.com"
    QRCODE_API = "https://qrcodeapi.115.com"
    PRO_API = "https://proapi.115.com"

    SUCCESS_CODES = {0, 20004}
    AUTH_ERROR_CODES = {
        40140116,
        40140119,
        40140120,
        40140125,
    }

    def __init__(
        self,
        client_id: str,
        *,
        token: Optional[Pan115Token] = None,
        token_path: Optional[str] = None,
        on_token_updated: Optional[Callable[["Pan115Client", Pan115Token], None]] = None,
        timeout: int = 30,
        user_agent: str = "W115Storage/2.0",
        session: Optional[requests.Session] = None,
        api_qps: float = 3.0,
        download_qps: float = 1.0,
        cooldown_seconds: int = 3600,
    ) -> None:
        self.client_id = str(client_id)
        self.token = token
        self.token_path = token_path
        self.on_token_updated = on_token_updated
        self.timeout = timeout
        self.session = session or requests.Session()
        self.api_qps = api_qps
        self.download_qps = download_qps
        self.cooldown_seconds = cooldown_seconds
        self._web_cookie: Optional[str] = None

        self._next_api_at = 0.0
        self._next_download_at = 0.0
        self._cooldown_until = 0.0
        self._rate_lock = threading.Lock()

        self.session.headers.update(
            {
                "User-Agent": user_agent,
                "Accept-Encoding": "gzip, deflate",
            }
        )

    @classmethod
    def from_token_file(
        cls,
        client_id: str,
        token_path: str,
        **kwargs: Any,
    ) -> "Pan115Client":
        """从本地 token JSON 构造客户端"""
        return cls(client_id=client_id, token=load_token(token_path), token_path=token_path, **kwargs)

    def _persist_token(self) -> None:
        if self.token and self.token_path:
            save_token(self.token, self.token_path)

    def _emit_token_updated(self, token: Pan115Token) -> None:
        if self.on_token_updated is not None:
            self.on_token_updated(self, token)

    def _enter_cooldown(self) -> None:
        cooldown_until = time.time() + self.cooldown_seconds
        with self._rate_lock:
            self._cooldown_until = cooldown_until
            self._next_api_at = max(self._next_api_at, cooldown_until)
            self._next_download_at = max(self._next_download_at, cooldown_until)

    def _sleep_rate_limit(self, *, is_download: bool) -> None:
        wait_seconds = 0.0
        with self._rate_lock:
            now = time.time()
            if now < self._cooldown_until:
                retry_after = max(1, int(self._cooldown_until - now))
                raise Pan115RateLimitError(
                    f"115 接口冷却中，请 {retry_after} 秒后再试",
                    code=429,
                    payload={
                        "cooldown_until": self._cooldown_until,
                        "retry_after": retry_after,
                    },
                )

            next_at = self._next_download_at if is_download else self._next_api_at
            reserve_at = max(now, next_at)
            wait_seconds = max(0.0, reserve_at - now)

            qps = self.download_qps if is_download else self.api_qps
            interval = 1.0 / qps if qps > 0 else 0.0
            reserved_next = reserve_at + interval
            if is_download:
                self._next_download_at = reserved_next
            else:
                self._next_api_at = reserved_next

        if wait_seconds > 0:
            time.sleep(wait_seconds)

    def _make_headers(
        self,
        *,
        auth_required: bool = False,
        content_type_form: bool = True,
    ) -> dict[str, str]:
        headers: dict[str, str] = {}
        if content_type_form:
            headers["Content-Type"] = "application/x-www-form-urlencoded"
        if auth_required:
            token = self.get_valid_access_token()
            headers["Authorization"] = f"Bearer {token}"
        return headers

    def _should_retry(self, exc: Exception, payload: Optional[dict[str, Any]] = None) -> bool:
        if isinstance(exc, requests.HTTPError):
            response = exc.response
            if response is not None and response.status_code >= 500:
                return True
        if payload:
            message = str(payload.get("message") or payload.get("msg") or "")
            if "请求异常需要重试" in message:
                return True
        return isinstance(exc, (requests.ConnectionError, requests.Timeout))

    def _request(
        self,
        method: str,
        base_url: str,
        path: str,
        *,
        params: Optional[dict[str, Any]] = None,
        data: Optional[dict[str, Any]] = None,
        json_body: Optional[dict[str, Any]] = None,
        auth_required: bool = False,
        allow_code_20004: bool = False,
        is_download: bool = False,
        max_attempts: int = 3,
    ) -> dict[str, Any]:
        url = urljoin(base_url, path)
        delay = 1.0
        for attempt in range(1, max_attempts + 1):
            self._sleep_rate_limit(is_download=is_download)
            headers = self._make_headers(auth_required=auth_required, content_type_form=json_body is None)
            try:
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    data=data,
                    json=json_body,
                    headers=headers,
                    timeout=self.timeout,
                )
                if response.status_code == 429:
                    self._enter_cooldown()
                    raise Pan115RateLimitError(
                        "115 接口返回 HTTP 429，进入冷却",
                        code=429,
                        payload={"status_code": 429},
                    )

                response.raise_for_status()
                payload = response.json()

                message = str(payload.get("message") or payload.get("msg") or "")
                if "已达到当前访问上限" in message:
                    self._enter_cooldown()
                    raise Pan115RateLimitError(
                        "115 接口提示达到访问上限，进入冷却",
                        code=payload.get("code"),
                        payload=payload,
                    )

                code = payload.get("code")
                state = payload.get("state")
                success_codes = set(self.SUCCESS_CODES)
                if not allow_code_20004:
                    success_codes.discard(20004)

                if code in success_codes or (code is None and state is True):
                    return payload

                if code in self.AUTH_ERROR_CODES:
                    raise Pan115AuthError(message or "115 授权失败", code=code, state=state, payload=payload)

                raise Pan115ApiError(message or "115 接口返回失败", code=code, state=state, payload=payload)
            except Pan115RateLimitError:
                raise
            except Pan115ApiError:
                raise
            except Exception as exc:
                payload = None
                if isinstance(exc, requests.HTTPError) and exc.response is not None:
                    try:
                        payload = exc.response.json()
                    except ValueError:
                        payload = None
                if attempt >= max_attempts or not self._should_retry(exc, payload):
                    if isinstance(exc, requests.HTTPError):
                        detail = f"HTTP {exc.response.status_code}" if exc.response is not None else "HTTP 请求失败"
                        raise Pan115ApiError(detail, payload=payload) from exc
                    raise
                time.sleep(delay + random.random())
                delay *= 2

        raise Pan115ApiError("115 请求失败")

    def get_valid_access_token(self) -> str:
        """获取可用 access_token，必要时自动刷新"""
        if not self.token:
            raise Pan115AuthError("未配置 115 token，请先完成授权")

        if self.token.is_expired():
            self.refresh_token()
        return self.token.access_token

    def create_device_code(self, *, code_verifier: Optional[str] = None) -> DeviceCodeSession:
        """创建扫码登录二维码与会话参数"""
        code_verifier = code_verifier or generate_code_verifier()
        code_challenge = generate_code_challenge(code_verifier)
        payload = self._request(
            "POST",
            self.PASSPORT_API,
            "/open/authDeviceCode",
            data={
                "client_id": self.client_id,
                "code_challenge": code_challenge,
                "code_challenge_method": "sha256",
            },
        )
        data = payload["data"]
        return DeviceCodeSession(
            qrcode=str(data["qrcode"]),
            uid=str(data["uid"]),
            time_value=str(data["time"]),
            sign=str(data["sign"]),
            code_verifier=code_verifier,
        )

    def get_qrcode_status(self, login_session: DeviceCodeSession) -> QrcodeStatus:
        """轮询扫码状态"""
        payload = self._request(
            "GET",
            self.QRCODE_API,
            "/get/status/",
            params={
                "uid": login_session.uid,
                "time": login_session.time_value,
                "sign": login_session.sign,
            },
        )
        data = payload.get("data") or {}
        return QrcodeStatus(
            status=int(data.get("status") or 0),
            message=str(data.get("msg") or ""),
            raw=payload,
        )

    def exchange_device_token(self, login_session: DeviceCodeSession) -> Pan115Token:
        """使用扫码登录会话换取 token"""
        payload = self._request(
            "POST",
            self.PASSPORT_API,
            "/open/deviceCodeToToken",
            data={
                "uid": login_session.uid,
                "code_verifier": login_session.code_verifier,
            },
        )
        token = Pan115Token.from_dict(payload["data"])
        self.token = token
        self._persist_token()
        self._emit_token_updated(token)
        return token

    def refresh_token(self) -> Pan115Token:
        """刷新 access_token，并更新本地 token 存储"""
        if not self.token:
            raise Pan115AuthError("没有可刷新的 token")
        payload = self._request(
            "POST",
            self.PASSPORT_API,
            "/open/refreshToken",
            data={"refresh_token": self.token.refresh_token},
        )
        data = payload["data"]
        token = Pan115Token(
            access_token=str(data["access_token"]),
            refresh_token=str(data["refresh_token"]),
            expires_in=int(data["expires_in"]),
            refresh_time=int(time.time()),
        )
        self.token = token
        self._persist_token()
        self._emit_token_updated(token)
        return token

    def list_files(self, cid: int | str = 0, *, limit: int = 1000, offset: int = 0) -> list[Pan115File]:
        """列出单个目录下一页内容"""
        payload = self._request(
            "GET",
            self.PRO_API,
            "/open/ufile/files",
            params={
                "cid": cid,
                "limit": limit,
                "offset": offset,
                "cur": True,
                "show_dir": 1,
            },
            auth_required=True,
        )
        return [Pan115File.from_list_item(item) for item in (payload.get("data") or [])]

    def list_all_files(self, cid: int | str = 0, *, limit: int = 1000) -> list[Pan115File]:
        """自动翻页列出目录全部内容"""
        results: list[Pan115File] = []
        offset = 0
        while True:
            batch = self.list_files(cid=cid, limit=limit, offset=offset)
            results.extend(batch)
            if len(batch) < limit:
                break
            offset += limit
        return results

    def get_path_info(self, path: str) -> Optional[Pan115File]:
        """按完整路径查询文件或目录详情；不存在时返回 None"""
        try:
            payload = self._request(
                "POST",
                self.PRO_API,
                "/open/folder/get_info",
                data={"path": path},
                auth_required=True,
            )
        except Pan115ApiError as exc:
            if exc.code in (20018, 50018):
                return None
            raise
        data = payload.get("data") or {}
        if not data or not data.get("file_id"):
            return None
        return Pan115File.from_path_info(data)

    def get_file_by_id(
        self,
        file_id: int | str,
        *,
        roots: Optional[list[int | str]] = None,
        max_depth: int = 12,
    ) -> Optional[Pan115File]:
        """
        尝试按文件 ID 获取文件/目录详情。

        115 开放平台没有稳定的按 ID 直查接口，这里使用两段式回退：
          1. 保留 search 快路径，兼容按名称传入或搜索恰好命中的场景
          2. 在给定 roots 下递归扫描 file_id

        返回 None 表示未找到。
        """
        normalized_id = str(file_id)

        results = self.search(normalized_id, limit=1)
        for item in results:
            if item.id == normalized_id:
                return item

        scan_roots = [str(root) for root in (roots or ["0"]) if str(root)]
        seen: set[str] = set()
        for root in scan_roots:
            if root in seen:
                continue
            seen.add(root)
            found = self._find_file_id_under_root(normalized_id, root, max_depth=max_depth)
            if found is not None:
                return found
        return None

    def _find_file_id_under_root(
        self,
        file_id: str,
        root_id: int | str,
        *,
        max_depth: int = 12,
        _depth: int = 0,
    ) -> Optional[Pan115File]:
        if _depth > max_depth:
            return None

        for item in self.list_files(cid=root_id, limit=1000):
            if item.id == file_id:
                return item
            if item.is_folder:
                found = self._find_file_id_under_root(
                    file_id,
                    item.id,
                    max_depth=max_depth,
                    _depth=_depth + 1,
                )
                if found is not None:
                    return found
        return None

    def exists(self, path: str) -> bool:
        """判断路径是否存在"""
        return self.get_path_info(path) is not None

    def create_folder(self, pid: int | str, file_name: str) -> Optional[str]:
        """在父目录下创建文件夹；若已存在也视为成功"""
        payload = self._request(
            "POST",
            self.PRO_API,
            "/open/folder/add",
            data={"pid": pid, "file_name": file_name},
            auth_required=True,
            allow_code_20004=True,
        )
        data = payload.get("data") or {}
        file_id = data.get("file_id")
        return str(file_id) if file_id is not None else None

    def ensure_folder_path(self, path: str) -> Pan115File:
        """
        按绝对路径确保目录存在。

        约定：
          - path 必须以 `/` 开头
          - 逐层调用 `get_path_info` / `create_folder`
        """
        if not path.startswith("/"):
            raise ValueError("目录路径必须以 / 开头")
        normalized = "/" + "/".join(part for part in path.split("/") if part)
        if normalized == "/":
            root = self.get_path_info("/")
            return root or Pan115File(id="0", name="/", category="0")

        parent_id = "0"
        current_path = ""
        current_info: Optional[Pan115File] = None
        for part in normalized.strip("/").split("/"):
            current_path = f"{current_path}/{part}"
            current_info = self.get_path_info(current_path)
            if current_info is None:
                self.create_folder(parent_id, part)
                current_info = self.get_path_info(current_path)
                if current_info is None:
                    raise Pan115ApiError(f"创建目录失败：{current_path}")
            parent_id = current_info.id
        return current_info

    def delete(self, file_id: int | str) -> None:
        """删除文件或目录"""
        self._request(
            "POST",
            self.PRO_API,
            "/open/ufile/delete",
            data={"file_ids": file_id},
            auth_required=True,
        )

    def rename(self, file_id: int | str, file_name: str) -> None:
        """重命名文件或目录"""
        self._request(
            "POST",
            self.PRO_API,
            "/open/ufile/update",
            data={"file_id": file_id, "file_name": file_name},
            auth_required=True,
        )

    def copy(self, file_id: int | str, pid: int | str) -> None:
        """复制文件或目录到目标目录"""
        self._request(
            "POST",
            self.PRO_API,
            "/open/ufile/copy",
            data={"file_id": file_id, "pid": pid},
            auth_required=True,
        )

    def move(self, file_id: int | str, to_cid: int | str) -> None:
        """移动文件或目录到目标目录"""
        self._request(
            "POST",
            self.PRO_API,
            "/open/ufile/move",
            data={"file_ids": file_id, "to_cid": to_cid},
            auth_required=True,
        )

    def search(
        self,
        search_value: str,
        *,
        limit: int = 20,
        offset: int = 0,
        cid: Optional[int | str] = None,
        fc: Optional[int] = None,
        file_type: Optional[int] = None,
        suffix: Optional[str] = None,
    ) -> list[Pan115File]:
        """搜索文件或目录"""
        params: dict[str, Any] = {
            "search_value": search_value,
            "limit": limit,
            "offset": offset,
        }
        if cid is not None:
            params["cid"] = cid
        if fc is not None:
            params["fc"] = fc
        if file_type is not None:
            params["type"] = file_type
        if suffix:
            params["suffix"] = suffix

        payload = self._request(
            "GET",
            self.PRO_API,
            "/open/ufile/search",
            params=params,
            auth_required=True,
        )
        return [Pan115File.from_search_item(item) for item in (payload.get("data") or [])]

    def get_download_url(self, pick_code: str) -> str:
        """根据 pick_code 获取真实下载地址"""
        payload = self._request(
            "POST",
            self.PRO_API,
            "/open/ufile/downurl",
            data={"pick_code": pick_code},
            auth_required=True,
            is_download=True,
        )
        data = payload.get("data")
        if isinstance(data, dict):
            for value in data.values():
                if isinstance(value, dict):
                    maybe_url = value.get("url")
                    if isinstance(maybe_url, dict) and maybe_url.get("url"):
                        return str(maybe_url["url"])
        for value in payload.values():
            if isinstance(value, dict):
                maybe_url = value.get("url")
                if isinstance(maybe_url, dict) and maybe_url.get("url"):
                    return str(maybe_url["url"])
        raise Pan115ApiError("未从 downurl 响应中解析到下载地址", payload=payload)

    def download_file(
        self,
        *,
        pick_code: Optional[str] = None,
        path: Optional[str] = None,
        dest_path: str,
        chunk_size: int = 1024 * 1024,
    ) -> str:
        """下载文件到本地，异常时删除残留文件"""
        if not pick_code:
            if not path:
                raise ValueError("pick_code 与 path 至少需要提供一个")
            info = self.get_path_info(path)
            if not info or not info.pick_code:
                raise Pan115ApiError(f"无法获取文件详情或 pick_code：{path}")
            pick_code = info.pick_code

        download_url = self.get_download_url(pick_code)
        os.makedirs(os.path.dirname(dest_path) or ".", exist_ok=True)

        try:
            with self.session.get(download_url, stream=True, timeout=self.timeout) as response:
                response.raise_for_status()
                with open(dest_path, "wb") as fh:
                    for chunk in response.iter_content(chunk_size=chunk_size):
                        if chunk:
                            fh.write(chunk)
            return dest_path
        except Exception:
            if os.path.exists(dest_path):
                os.remove(dest_path)
            raise

    @staticmethod
    def compute_upload_hashes(file_path: str, *, preid_size: int = 128 * 1024 * 1024) -> UploadHashes:
        """计算上传所需 fileid / preid"""
        sha1_all = hashlib.sha1()
        sha1_pre = hashlib.sha1()
        size = 0
        remaining_pre = preid_size

        with open(file_path, "rb") as fh:
            while True:
                chunk = fh.read(1024 * 1024)
                if not chunk:
                    break
                size += len(chunk)
                sha1_all.update(chunk)
                if remaining_pre > 0:
                    sha1_pre.update(chunk[:remaining_pre])
                    remaining_pre -= len(chunk)

        return UploadHashes(
            file_size=size,
            fileid=sha1_all.hexdigest(),
            preid=sha1_pre.hexdigest(),
        )

    @staticmethod
    def compute_sign_val(file_path: str, sign_check: str) -> str:
        """按 sign_check 指定的字节范围计算二次认证 SHA1（大写）"""
        start_str, end_str = sign_check.split("-", 1)
        start = int(start_str)
        end = int(end_str)
        length = end - start + 1
        if length <= 0:
            raise ValueError(f"非法 sign_check：{sign_check}")

        with open(file_path, "rb") as fh:
            fh.seek(start)
            data = fh.read(length)
        return hashlib.sha1(data).hexdigest().upper()

    @staticmethod
    def build_upload_target(cid: int | str) -> str:
        """将目录 ID 转换为上传接口要求的 target 参数"""
        return f"U_1_{cid}"

    def init_upload(
        self,
        *,
        file_name: str,
        file_size: int,
        cid: int | str,
        fileid: str,
        preid: str,
        pick_code: Optional[str] = None,
        sign_key: Optional[str] = None,
        sign_val: Optional[str] = None,
    ) -> UploadInitInfo:
        """初始化上传；二次认证时补充 pick_code / sign_key / sign_val"""
        data: dict[str, Any] = {
            "file_name": file_name,
            "file_size": file_size,
            "target": self.build_upload_target(cid),
            "fileid": fileid,
            "preid": preid,
        }
        if pick_code:
            data["pick_code"] = pick_code
        if sign_key:
            data["sign_key"] = sign_key
        if sign_val:
            data["sign_val"] = sign_val

        payload = self._request(
            "POST",
            self.PRO_API,
            "/open/upload/init",
            data=data,
            auth_required=True,
        )
        raw = payload.get("data") or {}
        return UploadInitInfo(
            bucket=raw.get("bucket"),
            object_name=raw.get("object"),
            callback=raw.get("callback"),
            sign_check=raw.get("sign_check"),
            pick_code=raw.get("pick_code"),
            sign_key=raw.get("sign_key"),
            code=raw.get("code"),
            status=raw.get("status"),
            file_id=str(raw.get("file_id")) if raw.get("file_id") is not None else None,
            raw=raw,
        )

    def get_upload_token(self) -> UploadToken:
        """获取对象存储上传临时凭证"""
        payload = self._request(
            "GET",
            self.PRO_API,
            "/open/upload/get_token",
            auth_required=True,
        )
        data = payload.get("data") or {}
        return UploadToken(
            endpoint=str(data["endpoint"]),
            access_key_id=str(data["AccessKeyId"]),
            access_key_secret=str(data["AccessKeySecret"]),
            security_token=str(data["SecurityToken"]),
            raw=data,
        )

    def resume_upload(
        self,
        *,
        file_size: int,
        cid: int | str,
        fileid: str,
        pick_code: str,
    ) -> ResumeInfo:
        """查询断点续传信息"""
        payload = self._request(
            "POST",
            self.PRO_API,
            "/open/upload/resume",
            data={
                "file_size": file_size,
                "target": self.build_upload_target(cid),
                "fileid": fileid,
                "pick_code": pick_code,
            },
            auth_required=True,
        )
        data = payload.get("data") or {}
        return ResumeInfo(callback=data.get("callback"), raw=data)

    def get_space_info(self) -> Pan115SpaceInfo:
        """获取总空间与剩余空间"""
        payload = self._request(
            "GET",
            self.PRO_API,
            "/open/user/info",
            auth_required=True,
        )
        data = payload.get("data") or {}
        rt_info = data.get("rt_space_info") or {}
        total = (((rt_info.get("all_total") or {}).get("size")) or 0)
        remain = (((rt_info.get("all_remain") or {}).get("size")) or 0)
        return Pan115SpaceInfo(total_size=int(total), remain_size=int(remain), raw=data)

    def list_recycle_bin(self, *, limit: int = 30, offset: int = 0) -> list[RecycleBinItem]:
        """列出回收站内容"""
        payload = self._request(
            "GET",
            self.PRO_API,
            "/open/rb/list",
            params={"limit": limit, "offset": offset},
            auth_required=True,
        )
        items = payload.get("data") or []
        results: list[RecycleBinItem] = []
        for item in items:
            results.append(
                RecycleBinItem(
                    id=str(item.get("fid") or item.get("file_id") or ""),
                    name=str(item.get("fn") or item.get("file_name") or ""),
                    category=str(item.get("fc")) if item.get("fc") is not None else None,
                    size=int(item["fs"]) if item.get("fs") not in (None, "") else None,
                    raw=item,
                )
            )
        return results

    # ── Cookie API: 分享转存 ─────────────────────────────────────

    WEB_API = "https://webapi.115.com"
    COOKIE_USER_AGENT = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.102 Safari/537.36 MicroMessenger/6.8.0(0x16080000) NetType/WIFI MiniProgramEnv/Mac MacWechat/WMPF XWEB/30626"

    def set_cookie(self, cookie: str) -> None:
        """设置 Cookie 用于 Web API 认证"""
        self._web_cookie = cookie.strip()

    def has_cookie(self) -> bool:
        """当前是否已配置可用的 Web API Cookie"""
        return bool(self._web_cookie)

    def _web_request(
        self,
        method: str,
        path: str,
        *,
        params: Optional[dict[str, Any]] = None,
        data: Optional[dict[str, Any]] = None,
        max_attempts: int = 3,
    ) -> dict[str, Any]:
        """Web API 请求（基于 Cookie 认证）"""
        if not self._web_cookie:
            raise Pan115AuthError("未设置 115 Web API Cookie")

        url = urljoin(self.WEB_API, path)
        delay = 1.0
        headers = {
            "Content-Type": "application/x-www-form-urlencoded",
            "User-Agent": self.COOKIE_USER_AGENT,
            "Referer": "https://servicewechat.com/wx2c744c010a61b0fa/94/page-frame.html",
            "Cookie": self._web_cookie,
        }

        for attempt in range(1, max_attempts + 1):
            self._sleep_rate_limit(is_download=False)
            try:
                response = self.session.request(
                    method=method,
                    url=url,
                    params=params,
                    data=data,
                    headers=headers,
                    timeout=self.timeout,
                )
                if response.status_code == 429:
                    self._enter_cooldown()
                    raise Pan115RateLimitError(
                        "115 Web API 返回 HTTP 429，进入冷却",
                        code=429,
                        payload={"status_code": 429},
                    )

                response.raise_for_status()
                payload = response.json()
                if not payload.get("state"):
                    error = payload.get("error") or "请求失败"
                    errno = payload.get("errno")
                    if errno == -1:
                        raise Pan115AuthError(error, code=errno, payload=payload)
                    raise Pan115ApiError(error, code=errno, payload=payload)
                return payload
            except Pan115RateLimitError:
                raise
            except Exception as exc:
                payload = exc.payload if isinstance(exc, Pan115ApiError) else None
                if attempt == max_attempts or not self._should_retry(exc, payload):
                    raise
                time.sleep(delay)
                delay = min(delay * 2, 8.0)

        raise Pan115ApiError("115 Web API 请求失败")

    @staticmethod
    def parse_share_url(url: str) -> tuple[str, str]:
        """
        解析分享链接，提取分享码和密码。

        支持格式：
          - https://115.com/s/{shareCode}?password={password}
          - https://115cdn.com/s/{shareCode}?password={password}

        返回: (share_code, password)
        """
        parsed = urlparse(url)
        path_parts = [part for part in parsed.path.split("/") if part]
        share_code = ""
        if len(path_parts) >= 2 and path_parts[0].lower() == "s":
            share_code = path_parts[1]

        query = parse_qs(parsed.query)
        password = query.get("password", [""])[0]

        return share_code, unquote(password)

    def get_share_info(
        self,
        share_code: str,
        receive_code: str,
        *,
        cid: str = "",
        offset: int = 0,
        limit: int = 100,
    ) -> tuple[ShareInfo, list[ShareItem]]:
        """
        获取分享链接信息。

        Args:
            share_code: 分享码
            receive_code: 提取码
            cid: 子目录ID，根目录传空
            offset: 偏移量
            limit: 每页数量

        Returns:
            (ShareInfo, list[ShareItem])
        """
        if not self.has_cookie():
            return (
                ShareInfo(
                    share_code=share_code,
                    receive_code=receive_code,
                    title="",
                    file_size=0,
                    receive_count=0,
                    raw={"error": "未设置 115 Web API Cookie"},
                ),
                [],
            )

        payload = self._web_request(
            "GET",
            "/share/snap",
            params={
                "share_code": share_code,
                "receive_code": receive_code,
                "offset": offset,
                "limit": limit,
                "cid": cid,
            },
        )
        data = payload.get("data") or {}

        user_info = data.get("userinfo") or {}
        share_info = data.get("shareinfo") or {}

        info = ShareInfo(
            share_code=share_code,
            receive_code=receive_code,
            title=str(share_info.get("share_title") or ""),
            file_size=int(share_info.get("file_size") or 0),
            receive_count=int(share_info.get("receive_count") or 0),
            create_time=share_info.get("create_time"),
            expire_time=share_info.get("expire_time"),
            user_id=str(user_info.get("user_id") or ""),
            user_name=str(user_info.get("user_name") or ""),
            snap_id=str(share_info.get("snap_id") or ""),
            raw=data,
        )

        items: list[ShareItem] = []
        for item in data.get("list") or []:
            is_folder = item.get("cid") is not None
            items.append(
                ShareItem(
                    id=str(item.get("cid") or item.get("fid") or ""),
                    name=str(item.get("n") or ""),
                    size=int(item.get("s") or 0),
                    is_folder=is_folder,
                    modified_time=str(item.get("t")) if item.get("t") else None,
                    fid=str(item.get("fid")) if item.get("fid") else None,
                    raw=item,
                )
            )

        return info, items

    def receive_share(
        self,
        share_code: str,
        receive_code: str,
        file_ids: str | list[str],
        *,
        target_cid: str = "0",
    ) -> ReceiveResult:
        """
        转存分享文件到网盘。

        Args:
            share_code: 分享码
            receive_code: 提取码
            file_ids: 文件ID列表（可以是逗号分隔的字符串或列表）
            target_cid: 目标目录ID，默认根目录

        Returns:
            ReceiveResult
        """
        if not self.has_cookie():
            return ReceiveResult(
                success=False,
                folder_count=0,
                file_count=0,
                total_size=0,
                error="未设置 115 Web API Cookie",
            )

        if isinstance(file_ids, list):
            file_ids = ",".join(file_ids)

        try:
            payload = self._web_request(
                "POST",
                "/share/receive",
                data={
                    "cid": target_cid,
                    "share_code": share_code,
                    "receive_code": receive_code,
                    "file_id": file_ids,
                },
            )
            data = payload.get("data") or {}
            return ReceiveResult(
                success=True,
                folder_count=int(data.get("recv_folder_count") or 0),
                file_count=int(data.get("recv_file_count") or 0),
                total_size=int(data.get("receive_size") or 0),
                title=str(data.get("receive_title") or ""),
                raw=data,
            )
        except Pan115ApiError as exc:
            return ReceiveResult(
                success=False,
                folder_count=0,
                file_count=0,
                total_size=0,
                error=str(exc),
                raw={"error": str(exc), "code": exc.code},
            )

    def receive_share_all(
        self,
        share_code: str,
        receive_code: str,
        *,
        target_cid: str = "0",
        cid: str = "",
    ) -> ReceiveResult:
        """
        转存分享链接中的全部文件。

        默认只处理分享根目录第一页内容（最多 100 条）。
        """
        _, items = self.get_share_info(share_code, receive_code, cid=cid)
        if not items:
            return ReceiveResult(
                success=False,
                folder_count=0,
                file_count=0,
                total_size=0,
                error="分享链接中没有文件",
            )

        file_ids = [item.file_id for item in items]
        return self.receive_share(share_code, receive_code, file_ids, target_cid=target_cid)

    def get_user_info(self) -> dict[str, Any]:
        """获取用户信息（验证 Cookie 有效性）"""
        if not self.has_cookie():
            return {}

        payload = self._web_request("GET", "/files/index_info")
        return payload.get("data") or {}

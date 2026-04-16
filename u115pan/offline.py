"""
u115pan/offline.py —— 115 云下载接口

说明：
  - 仅封装云下载相关能力
  - 通过注入 Pan115Client 复用认证、限流、错误处理
  - 当前保持独立，不接入主程序
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Optional

from .client import Pan115Client


@dataclass
class TorrentFileItem:
    """BT 种子内的单个文件条目"""

    path: str
    size: int
    wanted: int
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class TorrentInfo:
    """BT 种子解析结果"""

    file_size: int
    torrent_name: str
    file_count: int
    info_hash: str
    files: list[TorrentFileItem] = field(default_factory=list)
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class OfflineTask:
    """云下载任务"""

    info_hash: str
    name: str
    status: int
    percent_done: float = 0.0
    size: int = 0
    add_time: Optional[int] = None
    last_update: Optional[int] = None
    file_id: Optional[str] = None
    delete_file_id: Optional[str] = None
    url: Optional[str] = None
    wp_path_id: Optional[str] = None
    raw: dict[str, Any] = field(default_factory=dict)

    @property
    def is_finished(self) -> bool:
        return self.status == 2

    @property
    def is_downloading(self) -> bool:
        return self.status == 1

    @property
    def is_failed(self) -> bool:
        return self.status == -1


@dataclass
class OfflineTaskList:
    """云下载任务分页结果"""

    page: int
    page_count: int
    count: int
    tasks: list[OfflineTask] = field(default_factory=list)
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class QuotaExpireInfo:
    """配额过期明细"""

    surplus: int
    expire_time: int
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class QuotaPackage:
    """单个配额包"""

    name: str
    count: int
    used: int
    surplus: int
    expire_info: list[QuotaExpireInfo] = field(default_factory=list)
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class OfflineQuotaInfo:
    """云下载配额信息"""

    count: int
    used: int
    surplus: int
    packages: list[QuotaPackage] = field(default_factory=list)
    raw: dict[str, Any] = field(default_factory=dict)


@dataclass
class OfflineUrlAddResult:
    """添加链接任务的逐条结果"""

    state: bool
    code: int
    message: str
    url: str
    info_hash: Optional[str] = None
    raw: dict[str, Any] = field(default_factory=dict)


class OfflineClient:
    """115 云下载接口客户端"""

    def __init__(self, client: Pan115Client) -> None:
        self._client = client

    def parse_torrent(self, torrent_sha1: str, pick_code: str) -> list[TorrentInfo]:
        """解析 BT 种子内容"""
        payload = self._client._request(
            "POST",
            self._client.PRO_API,
            "/open/offline/torrent",
            data={
                "torrent_sha1": torrent_sha1,
                "pick_code": pick_code,
            },
            auth_required=True,
        )
        results: list[TorrentInfo] = []
        for item in payload.get("data") or []:
            files = [
                TorrentFileItem(
                    path=str(child.get("path") or ""),
                    size=int(child.get("size") or 0),
                    wanted=int(child.get("wanted") or 0),
                    raw=child,
                )
                for child in (item.get("torrent_filelist") or [])
            ]
            results.append(
                TorrentInfo(
                    file_size=int(item.get("file_size") or 0),
                    torrent_name=str(item.get("torrent_name") or ""),
                    file_count=int(item.get("file_count") or 0),
                    info_hash=str(item.get("info_hash") or ""),
                    files=files,
                    raw=item,
                )
            )
        return results

    def get_task_list(self, *, page: int = 1) -> OfflineTaskList:
        """获取云下载任务列表"""
        payload = self._client._request(
            "GET",
            self._client.PRO_API,
            "/open/offline/get_task_list",
            params={"page": page},
            auth_required=True,
        )
        data = payload.get("data") or {}
        tasks = [
            OfflineTask(
                info_hash=str(item.get("info_hash") or ""),
                name=str(item.get("name") or ""),
                status=int(item.get("status") or 0),
                percent_done=float(item.get("percentDone") or 0),
                size=int(item.get("size") or 0),
                add_time=int(item["add_time"]) if item.get("add_time") not in (None, "") else None,
                last_update=int(item["last_update"]) if item.get("last_update") not in (None, "") else None,
                file_id=str(item.get("file_id")) if item.get("file_id") is not None else None,
                delete_file_id=str(item.get("delete_file_id")) if item.get("delete_file_id") is not None else None,
                url=item.get("url"),
                wp_path_id=str(item.get("wp_path_id")) if item.get("wp_path_id") is not None else None,
                raw=item,
            )
            for item in (data.get("tasks") or [])
        ]
        return OfflineTaskList(
            page=int(data.get("page") or page),
            page_count=int(data.get("page_count") or 0),
            count=int(data.get("count") or len(tasks)),
            tasks=tasks,
            raw=data,
        )

    def get_all_tasks(self) -> list[OfflineTask]:
        """自动翻页拉取全部云下载任务"""
        first_page = self.get_task_list(page=1)
        tasks = list(first_page.tasks)
        for page in range(2, max(first_page.page_count, 1) + 1):
            tasks.extend(self.get_task_list(page=page).tasks)
        return tasks

    def get_quota_info(self) -> OfflineQuotaInfo:
        """获取云下载配额信息"""
        payload = self._client._request(
            "GET",
            self._client.PRO_API,
            "/open/offline/get_quota_info",
            auth_required=True,
        )
        data = payload.get("data") or {}
        packages: list[QuotaPackage] = []
        for item in (data.get("package") or []):
            expire_info = [
                QuotaExpireInfo(
                    surplus=int(exp.get("surplus") or 0),
                    expire_time=int(exp.get("expire_time") or 0),
                    raw=exp,
                )
                for exp in (item.get("expire_info") or [])
            ]
            packages.append(
                QuotaPackage(
                    name=str(item.get("name") or ""),
                    count=int(item.get("count") or 0),
                    used=int(item.get("used") or 0),
                    surplus=int(item.get("surplus") or 0),
                    expire_info=expire_info,
                    raw=item,
                )
            )
        return OfflineQuotaInfo(
            count=int(data.get("count") or 0),
            used=int(data.get("used") or 0),
            surplus=int(data.get("surplus") or 0),
            packages=packages,
            raw=data,
        )

    def add_task_urls(
        self,
        urls: list[str] | str,
        *,
        wp_path_id: Optional[str | int] = None,
    ) -> list[OfflineUrlAddResult]:
        """添加云下载链接任务；支持多个链接"""
        if isinstance(urls, str):
            url_text = urls
        else:
            url_text = "\n".join(url.strip() for url in urls if url.strip())

        data: dict[str, Any] = {"urls": url_text}
        if wp_path_id is not None:
            data["wp_path_id"] = wp_path_id

        payload = self._client._request(
            "POST",
            self._client.PRO_API,
            "/open/offline/add_task_urls",
            data=data,
            auth_required=True,
        )
        return [
            OfflineUrlAddResult(
                state=bool(item.get("state")),
                code=int(item.get("code") or 0),
                message=str(item.get("message") or ""),
                url=str(item.get("url") or ""),
                info_hash=str(item.get("info_hash")) if item.get("info_hash") is not None else None,
                raw=item,
            )
            for item in (payload.get("data") or [])
        ]

    def add_task_bt(
        self,
        *,
        info_hash: str,
        wanted: list[int] | str,
        save_path: str,
        torrent_sha1: str,
        pick_code: str,
        wp_path_id: Optional[str | int] = None,
    ) -> None:
        """添加 BT 云下载任务"""
        wanted_value = wanted if isinstance(wanted, str) else ",".join(str(i) for i in wanted)
        data: dict[str, Any] = {
            "info_hash": info_hash,
            "wanted": wanted_value,
            "save_path": save_path,
            "torrent_sha1": torrent_sha1,
            "pick_code": pick_code,
        }
        if wp_path_id is not None:
            data["wp_path_id"] = wp_path_id

        self._client._request(
            "POST",
            self._client.PRO_API,
            "/open/offline/add_task_bt",
            data=data,
            auth_required=True,
        )

    def del_task(self, info_hash: str, *, del_source_file: int = 0) -> None:
        """删除单个云下载任务"""
        self._client._request(
            "POST",
            self._client.PRO_API,
            "/open/offline/del_task",
            data={
                "info_hash": info_hash,
                "del_source_file": del_source_file,
            },
            auth_required=True,
        )

    def clear_tasks(self, flag: int = 0) -> None:
        """按类型清空云下载任务。

        flag 枚举：
          - 0: 清空已完成
          - 1: 清空全部
          - 2: 清空失败
          - 3: 清空进行中
          - 4: 清空已完成任务并清空对应源文件
          - 5: 清空全部任务并清空对应源文件
        """
        self._client._request(
            "POST",
            self._client.PRO_API,
            "/open/offline/clear_task",
            data={"flag": flag},
            auth_required=True,
        )

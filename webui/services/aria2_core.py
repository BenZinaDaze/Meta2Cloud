import os
import re
import secrets
from typing import Any, Dict, List, Optional
from urllib.parse import unquote, urlparse

import requests
from fastapi import HTTPException
from requests.adapters import HTTPAdapter
from urllib3.util.retry import Retry

from webui.core.app_logging import app_log
from webui.core.runtime import get_config, logger


_ARIA2_TASK_KEYS = [
    "gid", "status", "totalLength", "completedLength", "uploadLength",
    "downloadSpeed", "uploadSpeed", "connections", "numSeeders", "seeder",
    "errorCode", "errorMessage", "dir", "files", "bittorrent",
]
_ARIA2_GLOBAL_OPTION_KEYS = [
    "dir", "max-concurrent-downloads", "max-overall-download-limit",
    "max-overall-upload-limit", "split", "max-connection-per-server",
    "min-split-size", "continue", "max-tries", "retry-wait",
    "user-agent", "all-proxy", "seed-ratio", "seed-time", "bt-max-peers",
]
_ARIA2_NO_RETRY = Retry(total=0, raise_on_status=False)
_aria2_http = requests.Session()
_aria2_http.trust_env = False
_aria2_http.mount("https://", HTTPAdapter(max_retries=_ARIA2_NO_RETRY))
_aria2_http.mount("http://", HTTPAdapter(max_retries=_ARIA2_NO_RETRY))


def _aria2_rpc_url() -> str:
    cfg = get_config().aria2
    scheme = "https" if cfg.secure else "http"
    path = cfg.path if cfg.path.startswith("/") else f"/{cfg.path}"
    return f"{scheme}://{cfg.host}:{cfg.port}{path}"


def _ensure_aria2_enabled() -> None:
    if not get_config().aria2.enabled:
        raise HTTPException(status_code=503, detail="Aria2 集成未启用")


def aria2_rpc_call(method: str, params: Optional[List[Any]] = None) -> Any:
    _ensure_aria2_enabled()
    cfg = get_config().aria2
    rpc_params = list(params or [])
    if cfg.secret:
        rpc_params.insert(0, f"token:{cfg.secret}")
    payload = {"jsonrpc": "2.0", "id": secrets.token_hex(8), "method": f"aria2.{method}", "params": rpc_params}
    try:
        resp = _aria2_http.post(_aria2_rpc_url(), json=payload, timeout=5)
        resp.raise_for_status()
        data = resp.json()
    except requests.RequestException as exc:
        logger.warning("aria2 RPC 请求失败：%s", exc)
        app_log("download", "aria2_rpc_error", "aria2 无法连接", level="ERROR", details={"method": method, "error": str(exc)})
        raise HTTPException(status_code=502, detail=f"无法连接 aria2 RPC：{exc}") from exc
    except ValueError as exc:
        logger.warning("aria2 RPC 返回非 JSON：%s", exc)
        app_log("download", "aria2_rpc_invalid_json", "aria2 返回了无效数据", level="ERROR", details={"method": method, "error": str(exc)})
        raise HTTPException(status_code=502, detail="aria2 RPC 返回了无效响应") from exc
    if data.get("error"):
        message = data["error"].get("message") or "aria2 RPC 调用失败"
        code = data["error"].get("code")
        app_log("download", "aria2_rpc_api_error", f"aria2 操作失败：{message}", level="ERROR", details={"method": method, "message": message, "code": code})
        raise HTTPException(status_code=400, detail=f"{message} ({code})" if code else message)
    return data.get("result")


def _aria2_guess_name(task: Dict[str, Any]) -> str:
    info = task.get("bittorrent", {}).get("info", {})
    torrent_name = info.get("name")
    if torrent_name:
        return torrent_name
    files = task.get("files") or []
    first = files[0] if files else {}
    path = first.get("path") or ""
    if path:
        task_dir = task.get("dir") or ""
        if task_dir:
            try:
                rel_path = os.path.relpath(path, task_dir)
                first_segment = rel_path.split(os.sep, 1)[0]
                if first_segment and first_segment not in {".", ".."}:
                    return first_segment
            except ValueError:
                pass
        return os.path.basename(path)
    uris = first.get("uris") or []
    uri = next((u.get("uri") for u in uris if u.get("uri")), "")
    if uri:
        parsed = urlparse(uri)
        guess = os.path.basename(parsed.path)
        if guess:
            return unquote(guess)
        return parsed.netloc or uri
    return info.get("name") or task.get("gid") or "Unnamed Task"


def _aria2_progress(task: Dict[str, Any]) -> float:
    total = int(task.get("totalLength") or 0)
    completed = int(task.get("completedLength") or 0)
    if total <= 0:
        return 0.0
    return round(completed * 100 / total, 1)


def _aria2_file_count(task: Dict[str, Any]) -> int:
    return len(task.get("files") or [])


def normalize_task(task: Dict[str, Any]) -> Dict[str, Any]:
    total = int(task.get("totalLength") or 0)
    completed = int(task.get("completedLength") or 0)
    upload = int(task.get("uploadLength") or 0)
    first_file = (task.get("files") or [{}])[0]
    uris = first_file.get("uris") or []
    bittorrent = task.get("bittorrent") or {}
    info = bittorrent.get("info") or {}
    return {
        "gid": task.get("gid"),
        "status": task.get("status"),
        "name": _aria2_guess_name(task),
        "dir": task.get("dir") or "",
        "progress": _aria2_progress(task),
        "totalLength": total,
        "completedLength": completed,
        "uploadLength": upload,
        "downloadSpeed": int(task.get("downloadSpeed") or 0),
        "uploadSpeed": int(task.get("uploadSpeed") or 0),
        "connections": int(task.get("connections") or 0),
        "numSeeders": int(task.get("numSeeders") or 0),
        "seeder": task.get("seeder") == "true",
        "errorCode": task.get("errorCode") or "",
        "errorMessage": task.get("errorMessage") or "",
        "fileCount": _aria2_file_count(task),
        "uris": [u.get("uri") for u in uris if u.get("uri")],
        "bittorrent": {"name": info.get("name") or "", "comment": bittorrent.get("comment") or "", "mode": bittorrent.get("mode") or ""},
        "files": [
            {
                "path": f.get("path") or "",
                "length": int(f.get("length") or 0),
                "completedLength": int(f.get("completedLength") or 0),
                "selected": f.get("selected") != "false",
                "uris": [u.get("uri") for u in (f.get("uris") or []) if u.get("uri")],
            }
            for f in (task.get("files") or [])
        ],
    }


def _pagination_payload(*, page: int, page_size: int, total: int) -> Dict[str, Any]:
    total_pages = max((total + page_size - 1) // page_size, 1)
    page = max(1, min(page, total_pages))
    return {"page": page, "page_size": page_size, "total": total, "total_pages": total_pages, "has_prev": page > 1, "has_next": page < total_pages}


def _aria2_flush_metadata_results(gids: List[str]) -> None:
    if not gids:
        return
    try:
        for gid in gids:
            aria2_rpc_call("removeDownloadResult", [gid])
        logger.info("自动清理了 %d 个已完成的 [METADATA] 或 .torrent 任务", len(gids))
        app_log("download", "metadata_purged", f"已自动从队列中横扫清理并移除了 {len(gids)} 个已完成的种子/元数据任务", level="INFO", details={"count": len(gids), "gids": gids})
    except Exception as exc:
        logger.warning("自动清理种子/元数据任务失败：%s", exc)
        app_log("download", "metadata_purge_failed", f"尝试清理已完成的种子/元数据任务失败: {exc}", level="WARNING", details={"error": str(exc)})


def _aria2_normalize_stopped(tasks: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    normalized_stopped = []
    metadata_gids_to_remove = []
    for task in tasks:
        norm_task = normalize_task(task)
        name = norm_task.get("name", "")
        if norm_task.get("status") == "complete" and ("[METADATA]" in name or bool(re.match(r"^[0-9a-fA-F]{40}\.torrent$", name))):
            metadata_gids_to_remove.append(norm_task["gid"])
            continue
        normalized_stopped.append(norm_task)
    _aria2_flush_metadata_results(metadata_gids_to_remove)
    return normalized_stopped


def _aria2_fetch_waiting_slice(start: int, count: int, total: int) -> List[Dict[str, Any]]:
    if count <= 0 or start >= total:
        return []
    raw_offset = max(total - (start + count), 0)
    raw_limit = min(count, total - start)
    waiting = aria2_rpc_call("tellWaiting", [raw_offset, raw_limit, _ARIA2_TASK_KEYS]) or []
    return [normalize_task(task) for task in waiting][::-1]


def _aria2_fetch_stopped_slice(start: int, count: int) -> List[Dict[str, Any]]:
    if count <= 0:
        return []
    stopped = aria2_rpc_call("tellStopped", [start, count, _ARIA2_TASK_KEYS]) or []
    return _aria2_normalize_stopped(stopped)


def _aria2_task_matches(task: Dict[str, Any], search: str) -> bool:
    if not search:
        return True
    haystacks = [task.get("gid") or "", task.get("name") or "", task.get("dir") or "", task.get("errorMessage") or ""]
    for file_item in task.get("files") or []:
        haystacks.append(file_item.get("path") or "")
        for uri in file_item.get("uris") or []:
            haystacks.append(uri or "")
    return any(search in str(value).lower() for value in haystacks if value)


def _aria2_fetch_all_filtered(queue: str, search: str) -> Dict[str, Any]:
    global_stat = aria2_rpc_call("getGlobalStat") or {}
    version = aria2_rpc_call("getVersion") or {}
    waiting_total = int(global_stat.get("numWaiting") or 0)
    stopped_total = int(global_stat.get("numStopped") or 0)
    active = [normalize_task(task) for task in (aria2_rpc_call("tellActive", [_ARIA2_TASK_KEYS]) or [])][::-1]
    waiting = _aria2_fetch_waiting_slice(0, waiting_total, waiting_total)
    stopped = _aria2_fetch_stopped_slice(0, stopped_total)
    search = search.strip().lower()
    filtered = {
        "active": [task for task in active if _aria2_task_matches(task, search)],
        "waiting": [task for task in waiting if _aria2_task_matches(task, search)],
        "stopped": [task for task in stopped if _aria2_task_matches(task, search)],
    }
    items = filtered["active"] + filtered["waiting"] + filtered["stopped"] if queue == "all" else filtered[queue]
    return {
        "summary": {
            "activeCount": len(filtered["active"]),
            "waitingCount": len(filtered["waiting"]),
            "stoppedCount": len(filtered["stopped"]),
            "downloadSpeed": int(global_stat.get("downloadSpeed") or 0),
            "uploadSpeed": int(global_stat.get("uploadSpeed") or 0),
            "numActive": int(global_stat.get("numActive") or 0),
            "numWaiting": waiting_total,
            "numStopped": stopped_total,
        },
        "items": items,
        "version": {"version": version.get("version") or "", "enabledFeatures": version.get("enabledFeatures") or []},
    }


def fetch_queue_items(queue: str, page: int, page_size: int, search: str = "") -> Dict[str, Any]:
    global_stat = aria2_rpc_call("getGlobalStat") or {}
    version = aria2_rpc_call("getVersion") or {}
    active = [normalize_task(task) for task in (aria2_rpc_call("tellActive", [_ARIA2_TASK_KEYS]) or [])][::-1]
    active_count = len(active)
    waiting_count = int(global_stat.get("numWaiting") or 0)
    stopped_count = int(global_stat.get("numStopped") or 0)
    queue = queue if queue in {"all", "active", "waiting", "stopped"} else "all"
    search = search.strip()
    if search:
        filtered_payload = _aria2_fetch_all_filtered(queue, search)
        pagination = _pagination_payload(page=page, page_size=page_size, total=len(filtered_payload["items"]))
        offset = (pagination["page"] - 1) * page_size
        return {
            "summary": filtered_payload["summary"],
            "items": filtered_payload["items"][offset: offset + page_size],
            "pagination": {**pagination, "queue": queue, "search": search},
            "version": filtered_payload["version"],
        }
    total = active_count if queue == "active" else waiting_count if queue == "waiting" else stopped_count if queue == "stopped" else active_count + waiting_count + stopped_count
    pagination = _pagination_payload(page=page, page_size=page_size, total=total)
    offset = (pagination["page"] - 1) * page_size
    items: List[Dict[str, Any]] = []
    if queue == "active":
        items = active[offset: offset + page_size]
    elif queue == "waiting":
        items = _aria2_fetch_waiting_slice(offset, page_size, waiting_count)
    elif queue == "stopped":
        items = _aria2_fetch_stopped_slice(offset, page_size)
    else:
        remaining = page_size
        current_offset = offset
        if current_offset < active_count and remaining > 0:
            take = min(remaining, active_count - current_offset)
            items.extend(active[current_offset: current_offset + take])
            remaining -= take
            current_offset = 0
        else:
            current_offset = max(current_offset - active_count, 0)
        if remaining > 0:
            if current_offset < waiting_count:
                take = min(remaining, waiting_count - current_offset)
                items.extend(_aria2_fetch_waiting_slice(current_offset, take, waiting_count))
                remaining -= take
                current_offset = 0
            else:
                current_offset = max(current_offset - waiting_count, 0)
        if remaining > 0:
            items.extend(_aria2_fetch_stopped_slice(current_offset, remaining))
    return {
        "summary": {
            "activeCount": active_count,
            "waitingCount": waiting_count,
            "stoppedCount": stopped_count,
            "downloadSpeed": int(global_stat.get("downloadSpeed") or 0),
            "uploadSpeed": int(global_stat.get("uploadSpeed") or 0),
            "numActive": int(global_stat.get("numActive") or 0),
            "numWaiting": waiting_count,
            "numStopped": stopped_count,
        },
        "items": items,
        "pagination": {**pagination, "queue": queue},
        "version": {"version": version.get("version") or "", "enabledFeatures": version.get("enabledFeatures") or []},
    }

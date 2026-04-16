import base64
from typing import Any, Dict, Optional

from fastapi import HTTPException

from webui.core.app_logging import app_log
from webui.services.aria2_core import _ARIA2_GLOBAL_OPTION_KEYS, _ARIA2_TASK_KEYS, aria2_rpc_call, normalize_task


def aria2_sanitize_options(options: Optional[Dict[str, Any]]) -> Dict[str, str]:
    sanitized: Dict[str, str] = {}
    for key, value in (options or {}).items():
        if value is None:
            continue
        sanitized[str(key)] = str(value)
    return sanitized


def aria2_get_task(gid: str) -> Dict[str, Any]:
    task = aria2_rpc_call("tellStatus", [gid, _ARIA2_TASK_KEYS])
    return normalize_task(task)


def aria2_retry_task(gid: str) -> Dict[str, Any]:
    task = aria2_rpc_call("tellStatus", [gid, _ARIA2_TASK_KEYS])
    uris = []
    for file_item in task.get("files") or []:
        for uri_item in file_item.get("uris") or []:
            uri = uri_item.get("uri")
            if uri and uri not in uris:
                uris.append(uri)
    if not uris:
        raise HTTPException(status_code=400, detail="该任务没有可重试的原始 URI")
    options = {}
    if task.get("dir"):
        options["dir"] = task["dir"]
    new_gid = aria2_rpc_call("addUri", [uris, options])
    return aria2_get_task(new_gid)


def aria2_options_payload():
    global_options = aria2_rpc_call("getGlobalOption") or {}
    return {key: global_options.get(key, "") for key in _ARIA2_GLOBAL_OPTION_KEYS}


def aria2_update_options_payload(body: Dict[str, Any]):
    options = aria2_sanitize_options(body)
    aria2_rpc_call("changeGlobalOption", [options])
    app_log(
        "download",
        "options_updated",
        "下载器全局配置已更新",
        level="SUCCESS",
        details={"keys": sorted(options.keys())},
    )
    return {"ok": True, "options": aria2_options_payload()}


def aria2_add_uri_payload(body):
    uris = [uri.strip() for uri in body.uris if uri and uri.strip()]
    if not uris:
        raise HTTPException(status_code=400, detail="至少需要一个下载链接")
    params: list[Any] = [uris, aria2_sanitize_options(body.options)]
    if body.position is not None:
        params.append(body.position)
    gid = aria2_rpc_call("addUri", params)
    task = aria2_get_task(gid)
    app_log(
        "download",
        "task_added_uri",
        f"已推送下载：{body.title}" if body.title else "已添加链接下载任务",
        level="SUCCESS",
        details={"gid": gid, "uriCount": len(uris), "name": body.title or task.get("name")},
    )
    return {"ok": True, "task": task}


def aria2_add_torrent_payload(body):
    try:
        raw = base64.b64decode(body.torrent, validate=True)
    except Exception as exc:
        raise HTTPException(status_code=400, detail="Torrent 内容不是有效的 Base64") from exc
    torrent_b64 = base64.b64encode(raw).decode("ascii")
    params: list[Any] = [torrent_b64, body.uris or [], aria2_sanitize_options(body.options)]
    if body.position is not None:
        params.append(body.position)
    gid = aria2_rpc_call("addTorrent", params)
    task = aria2_get_task(gid)
    app_log(
        "download",
        "task_added_torrent",
        f"已推送下载：{body.title}" if body.title else "已添加种子下载任务",
        level="SUCCESS",
        details={"gid": gid, "name": body.title or task.get("name")},
    )
    return {"ok": True, "task": task}


def aria2_pause_tasks_payload(gids: list[str]):
    for gid in gids:
        aria2_rpc_call("pause", [gid])
    app_log("download", "tasks_paused", "下载任务已暂停", level="SUCCESS", details={"gids": gids, "count": len(gids)})
    return {"ok": True}


def aria2_unpause_tasks_payload(gids: list[str]):
    for gid in gids:
        aria2_rpc_call("unpause", [gid])
    app_log("download", "tasks_unpaused", "下载任务已恢复", level="SUCCESS", details={"gids": gids, "count": len(gids)})
    return {"ok": True}


def aria2_remove_tasks_payload(gids: list[str]):
    for gid in gids:
        task = aria2_rpc_call("tellStatus", [gid, ["status"]])
        method = "removeDownloadResult" if task.get("status") == "complete" else "remove"
        try:
            aria2_rpc_call(method, [gid])
        except HTTPException:
            if method != "removeDownloadResult":
                aria2_rpc_call("removeDownloadResult", [gid])
            else:
                raise
    app_log("download", "tasks_removed", "下载任务已移除", level="SUCCESS", details={"gids": gids, "count": len(gids)})
    return {"ok": True}


def aria2_retry_tasks_payload(gids: list[str]):
    tasks = [aria2_retry_task(gid) for gid in gids]
    app_log(
        "download",
        "tasks_retried",
        "下载任务已重试",
        level="SUCCESS",
        details={"sourceGids": gids, "newGids": [task.get("gid") for task in tasks]},
    )
    return {"ok": True, "tasks": tasks}


def aria2_purge_tasks_payload():
    aria2_rpc_call("purgeDownloadResult")
    app_log("download", "tasks_purged", "已清空已完成/已停止下载记录", level="SUCCESS")
    return {"ok": True}

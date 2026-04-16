from typing import Any

from fastapi import APIRouter, Query
from fastapi.concurrency import run_in_threadpool

from webui.schemas.aria2 import Aria2AddTorrentBody, Aria2AddUriBody, Aria2BatchActionBody
from webui.services.aria2_core import fetch_queue_items
from webui.services.aria2 import (
    aria2_add_torrent_payload,
    aria2_add_uri_payload,
    aria2_options_payload,
    aria2_pause_tasks_payload,
    aria2_purge_tasks_payload,
    aria2_remove_tasks_payload,
    aria2_retry_tasks_payload,
    aria2_unpause_tasks_payload,
    aria2_update_options_payload,
)

router = APIRouter()


@router.get("/api/aria2/overview")
async def aria2_overview(
    queue: str = Query("all"),
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=200),
    search: str = Query(""),
):
    return await run_in_threadpool(fetch_queue_items, queue, page, page_size, search)


@router.get("/api/aria2/options")
async def aria2_options():
    return aria2_options_payload()


@router.put("/api/aria2/options")
async def aria2_update_options(body: dict[str, Any]):
    return aria2_update_options_payload(body)


@router.post("/api/aria2/add-uri")
async def aria2_add_uri(body: Aria2AddUriBody):
    return aria2_add_uri_payload(body)


@router.post("/api/aria2/add-torrent")
async def aria2_add_torrent(body: Aria2AddTorrentBody):
    return aria2_add_torrent_payload(body)


@router.post("/api/aria2/tasks/pause")
async def aria2_pause_tasks(body: Aria2BatchActionBody):
    return aria2_pause_tasks_payload(body.gids)


@router.post("/api/aria2/tasks/unpause")
async def aria2_unpause_tasks(body: Aria2BatchActionBody):
    return aria2_unpause_tasks_payload(body.gids)


@router.post("/api/aria2/tasks/remove")
async def aria2_remove_tasks(body: Aria2BatchActionBody):
    return aria2_remove_tasks_payload(body.gids)


@router.post("/api/aria2/tasks/retry")
async def aria2_retry_tasks(body: Aria2BatchActionBody):
    return aria2_retry_tasks_payload(body.gids)


@router.post("/api/aria2/tasks/purge")
async def aria2_purge_tasks():
    return aria2_purge_tasks_payload()

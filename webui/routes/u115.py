from typing import Optional

from fastapi import APIRouter, Query
from fastapi.concurrency import run_in_threadpool

from webui.schemas.config import (
    U115CreateSessionBody,
    U115ExchangeBody,
    U115OfflineAddUrlsBody,
    U115OfflineClearBody,
    U115OfflineDeleteBody,
)
from webui.services.u115 import (
    u115_offline_add_urls_sync,
    u115_offline_clear_tasks_sync,
    u115_offline_delete_tasks_sync,
    u115_offline_overview_sync,
    u115_offline_quota_sync,
    u115_oauth_create_sync,
    u115_oauth_exchange_sync,
    u115_oauth_poll_sync,
    u115_oauth_qrcode_sync,
    u115_oauth_status_payload,
    u115_test_connection_sync,
)
from webui.services.watcher import u115_auto_organize_status_payload

router = APIRouter()


@router.get("/api/u115/oauth/status")
async def u115_oauth_status():
    return u115_oauth_status_payload()


@router.post("/api/u115/oauth/create")
async def u115_oauth_create(body: Optional[U115CreateSessionBody] = None):
    return await run_in_threadpool(u115_oauth_create_sync, body)


@router.get("/api/u115/oauth/qrcode")
async def u115_oauth_qrcode():
    return await run_in_threadpool(u115_oauth_qrcode_sync)


@router.get("/api/u115/oauth/poll")
async def u115_oauth_poll():
    return await run_in_threadpool(u115_oauth_poll_sync)


@router.post("/api/u115/oauth/exchange")
async def u115_oauth_exchange(body: Optional[U115ExchangeBody] = None):
    return await run_in_threadpool(u115_oauth_exchange_sync, body)


@router.post("/api/u115/test")
async def u115_test_connection():
    return await run_in_threadpool(u115_test_connection_sync)


@router.get("/api/u115/offline/overview")
async def u115_offline_overview(page: int = Query(1, ge=1)):
    return await run_in_threadpool(u115_offline_overview_sync, page)


@router.get("/api/u115/offline/auto-organize-status")
async def u115_offline_auto_organize_status():
    return await run_in_threadpool(u115_auto_organize_status_payload)


@router.get("/api/u115/offline/quota")
async def u115_offline_quota():
    return await run_in_threadpool(u115_offline_quota_sync)


@router.post("/api/u115/offline/add-urls")
async def u115_offline_add_urls(body: U115OfflineAddUrlsBody):
    return await run_in_threadpool(u115_offline_add_urls_sync, body)


@router.post("/api/u115/offline/tasks/delete")
async def u115_offline_delete_tasks(body: U115OfflineDeleteBody):
    return await run_in_threadpool(u115_offline_delete_tasks_sync, body)


@router.post("/api/u115/offline/tasks/clear")
async def u115_offline_clear_tasks(body: U115OfflineClearBody):
    return await run_in_threadpool(u115_offline_clear_tasks_sync, body)

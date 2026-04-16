from typing import Optional

from fastapi import APIRouter, Query
from fastapi.concurrency import run_in_threadpool

from webui.services.logs import logs_payload

router = APIRouter()


@router.get("/api/logs")
async def get_logs(
    limit: int = Query(200, ge=1, le=1000),
    category: Optional[str] = None,
    level: Optional[str] = None,
):
    return await run_in_threadpool(logs_payload, limit, category, level)

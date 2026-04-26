from typing import Optional

from fastapi import APIRouter, Query
from fastapi.concurrency import run_in_threadpool

from webui.ingest_store import get_ingest_store
from webui.schemas.ingest import IngestHistoryResponse, IngestStatsResponse
from webui.services.tmdb_service import TMDB_IMG_BASE

router = APIRouter()


def _enrich_poster_url(item: dict) -> dict:
    if item.get("poster_path"):
        item["poster_url"] = f"{TMDB_IMG_BASE}{item['poster_path']}"
    return item


@router.get("/api/ingest/history", response_model=IngestHistoryResponse)
async def get_ingest_history(
    page: int = Query(1, ge=1),
    page_size: int = Query(20, ge=1, le=100),
    media_type: Optional[str] = Query(None, pattern="^(movie|tv)$"),
    status: Optional[str] = Query(None, pattern="^(success|failed|no_tmdb)$"),
    start_date: Optional[str] = None,
    end_date: Optional[str] = None,
    keyword: Optional[str] = None,
):
    store = get_ingest_store()
    result = await run_in_threadpool(
        store.query,
        page=page,
        page_size=page_size,
        media_type=media_type,
        status=status,
        start_date=start_date,
        end_date=end_date,
        keyword=keyword,
    )
    items = [_enrich_poster_url(item) for item in result["items"]]
    return IngestHistoryResponse(items=items, pagination=result["pagination"])


@router.get("/api/ingest/stats", response_model=IngestStatsResponse)
async def get_ingest_stats(days: int = Query(7, ge=1, le=30)):
    store = get_ingest_store()
    return await run_in_threadpool(store.stats, days=days)


@router.get("/api/ingest/recent")
async def get_recent_ingests(limit: int = Query(10, ge=1, le=50)):
    store = get_ingest_store()
    items = await run_in_threadpool(store.get_recent, limit=limit)
    return [_enrich_poster_url(item) for item in items]

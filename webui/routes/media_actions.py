import asyncio

from fastapi import APIRouter

from webui.schemas.media_actions import RefreshItemRequest
from webui.services.media_actions import (
    cache_evict_payload,
    cache_stats_payload,
    health_payload,
    refresh_item_payload,
    scraper_get_episodes_payload,
    scraper_search_media_payload,
    tmdb_alternative_names_payload,
    tmdb_detail_payload,
    tmdb_search_multi_payload,
)

router = APIRouter()


@router.post("/api/library:refresh-item")
async def refresh_item(body: RefreshItemRequest):
    return await asyncio.get_event_loop().run_in_executor(None, lambda: refresh_item_payload(body))


@router.get("/api/cache")
async def cache_stats():
    return cache_stats_payload()


@router.post("/api/cache:evict")
async def cache_evict():
    return cache_evict_payload()


@router.get("/api/health")
async def health():
    return health_payload()


@router.get("/api/tmdb/search")
async def tmdb_search_multi(keyword: str):
    return tmdb_search_multi_payload(keyword)


@router.get("/api/tmdb/{tmdb_id}")
async def tmdb_detail(tmdb_id: int, media_type: str):
    return tmdb_detail_payload(tmdb_id, media_type)


@router.get("/api/tmdb/{tmdb_id}/alternative-names")
async def tmdb_alternative_names(tmdb_id: int, media_type: str):
    return tmdb_alternative_names_payload(tmdb_id, media_type)


@router.get("/api/scraper/search")
async def scraper_search_media(keyword: str):
    return scraper_search_media_payload(keyword)


@router.get("/api/scraper/episodes")
async def scraper_get_episodes(site: str, media_id: str, subgroup_id: str = None):
    return scraper_get_episodes_payload(site, media_id, subgroup_id)

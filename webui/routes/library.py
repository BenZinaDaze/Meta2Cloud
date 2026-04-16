import asyncio
from typing import List

from fastapi import APIRouter, HTTPException

from webui.schemas.library import LibraryResponse, MediaItem
from webui.core.app_logging import app_log
from webui.core.runtime import get_config, get_storage_provider, logger
from webui.library_store import get_library_store
from webui.services.library_data import scan_movies, scan_tv_shows
from webui.services.pipeline import _do_refresh_library

router = APIRouter()


@router.get("/api/library", response_model=LibraryResponse)
async def get_library():
    store = get_library_store()
    snapshot = store.get_snapshot()
    if snapshot is None:
        return LibraryResponse(
            movies=[],
            tv_shows=[],
            total_movies=0,
            total_tv=0,
            scanned_at=None,
            hint="媒体库尚未扫描，请点击「将媒体库」按鈕刷新",
        )
    movies = [MediaItem(**m) for m in snapshot["movies"]]
    tv_shows = [MediaItem(**t) for t in snapshot["tv_shows"]]
    return LibraryResponse(
        movies=movies,
        tv_shows=tv_shows,
        total_movies=snapshot["total_movies"],
        total_tv=snapshot["total_tv"],
        scanned_at=snapshot["scanned_at"],
    )


@router.post("/api/library/refresh")
async def refresh_library():
    try:
        diff = await asyncio.get_event_loop().run_in_executor(None, _do_refresh_library)
        return diff
    except Exception as exc:
        logger.exception("刷新媒体库失败")
        app_log(
            "library",
            "refresh_failed",
            "媒体库刷新失败",
            level="ERROR",
            details={"error": str(exc)},
        )
        raise HTTPException(status_code=500, detail=str(exc))


@router.get("/api/library/movies", response_model=List[MediaItem])
async def get_movies():
    try:
        client = get_storage_provider()
        cfg = get_config()
        return scan_movies(client, cfg)
    except Exception as exc:
        logger.exception("获取电影列表失败")
        raise HTTPException(status_code=500, detail=str(exc))


@router.get("/api/library/tv", response_model=List[MediaItem])
async def get_tv_shows():
    try:
        client = get_storage_provider()
        cfg = get_config()
        return scan_tv_shows(client, cfg)
    except Exception as exc:
        logger.exception("获取电视剧列表失败")
        raise HTTPException(status_code=500, detail=str(exc))


@router.get("/api/tv/{tmdb_id}", response_model=MediaItem)
async def get_tv_detail(tmdb_id: int):
    try:
        item = get_library_store().get_library_item_by_tmdb("tv", tmdb_id)
        if item:
            return MediaItem(**item)
        client = get_storage_provider()
        cfg = get_config()
        shows = scan_tv_shows(client, cfg)
        for show in shows:
            if show.tmdb_id == tmdb_id:
                return show
        raise HTTPException(status_code=404, detail=f"未找到 tmdb_id={tmdb_id} 的剧集")
    except HTTPException:
        raise
    except Exception as exc:
        logger.exception("获取剧集详情失败")
        raise HTTPException(status_code=500, detail=str(exc))

from fastapi import APIRouter, HTTPException

from webui.core.runtime import get_config, get_storage_provider, logger
from webui.schemas.library import StatsResponse
from webui.services.library_data import scan_movies, scan_tv_shows

router = APIRouter()


@router.get("/api/stats", response_model=StatsResponse)
async def get_stats():
    try:
        client = get_storage_provider()
        cfg = get_config()
        movies = scan_movies(client, cfg)
        tv_shows = scan_tv_shows(client, cfg)

        total_eps_tmdb = sum(show.total_episodes or 0 for show in tv_shows)
        total_eps_lib = sum(show.in_library_episodes or 0 for show in tv_shows)
        rate = round(total_eps_lib / total_eps_tmdb * 100, 1) if total_eps_tmdb > 0 else 0.0

        return StatsResponse(
            total_movies=len(movies),
            total_tv_shows=len(tv_shows),
            total_episodes_in_library=total_eps_lib,
            total_episodes_on_tmdb=total_eps_tmdb,
            completion_rate=rate,
        )
    except Exception as exc:
        logger.exception("获取统计信息失败")
        raise HTTPException(status_code=500, detail=str(exc))

from fastapi import APIRouter, HTTPException

from webui.core.runtime import logger
from webui.library_store import get_library_store
from webui.schemas.library import StatsResponse

router = APIRouter()


@router.get("/api/stats", response_model=StatsResponse)
async def get_stats():
    try:
        return StatsResponse(**get_library_store().get_stats())
    except Exception as exc:
        logger.exception("获取统计信息失败")
        raise HTTPException(status_code=500, detail=str(exc))

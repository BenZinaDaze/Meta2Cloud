import asyncio

import requests
from fastapi import APIRouter, HTTPException

from webui.core.runtime import logger

router = APIRouter()

BANGUMI_CALENDAR_URL = "https://api.bgm.tv/calendar"
USER_AGENT = "Meta2Cloud/1.0 (https://github.com/BenZinaDaze/Meta2Cloud)"


def _fetch_bangumi_calendar():
    resp = requests.get(
        BANGUMI_CALENDAR_URL,
        headers={"User-Agent": USER_AGENT},
        timeout=15.0,
    )
    resp.raise_for_status()
    return resp.json()


@router.get("/api/bangumi/calendar")
async def bangumi_calendar():
    """代理 Bangumi Calendar API，解决移动端 CORS 问题"""
    loop = asyncio.get_event_loop()
    try:
        return await loop.run_in_executor(None, _fetch_bangumi_calendar)
    except requests.HTTPError as exc:
        status_code = exc.response.status_code if exc.response is not None else 502
        logger.warning("Bangumi Calendar 上游 HTTP 错误: status=%s error=%s", status_code, exc)
        raise HTTPException(
            status_code=502,
            detail=f"Bangumi 日历服务暂时不可用（上游返回 {status_code}），请稍后重试",
        ) from exc
    except requests.RequestException as exc:
        logger.warning("Bangumi Calendar 请求失败: %s", exc)
        raise HTTPException(
            status_code=502,
            detail="Bangumi 日历服务暂时不可用，请稍后重试",
        ) from exc

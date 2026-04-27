import asyncio

import requests
from fastapi import APIRouter

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
    return await loop.run_in_executor(None, _fetch_bangumi_calendar)

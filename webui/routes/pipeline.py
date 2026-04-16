from fastapi import APIRouter, HTTPException, Request

from mediaparser import Config
from webui.services.pipeline import pipeline_status_payload, schedule_pipeline
from webui.core.runtime import logger
import hmac

router = APIRouter()


@router.post("/trigger")
async def trigger_pipeline_webhook(request: Request):
    cfg_obj = Config.load()
    ws = cfg_obj.webui.webhook_secret
    if ws:
        provided = request.headers.get("X-Webhook-Secret", "") or request.query_params.get("secret", "")
        if not hmac.compare_digest(provided.encode(), ws.encode()):
            raise HTTPException(status_code=403, detail="Invalid webhook secret")

    try:
        await request.json()
    except Exception:
        pass
    debounce = cfg_obj.telegram.debounce_seconds
    logger.info("收到整理触发请求 | 来源：webhook，防抖：%s 秒", debounce)
    schedule_pipeline(debounce)
    return {"status": "scheduled" if debounce > 0 else "triggered"}


@router.get("/api/pipeline/status")
async def pipeline_status():
    return pipeline_status_payload()


@router.post("/api/pipeline/trigger")
async def trigger_pipeline_manual():
    schedule_pipeline(0)
    return {"status": "triggered"}

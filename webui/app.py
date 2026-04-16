import os

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.staticfiles import StaticFiles

from webui.core.auth import auth_middleware
from webui.core.runtime import _ROOT_DIR
from webui.routes.aria2 import router as aria2_router
from webui.routes.auth import router as auth_router
from webui.routes.config import router as config_router
from webui.routes.library import router as library_router
from webui.routes.logs import router as logs_router
from webui.routes.media_actions import router as media_actions_router
from webui.routes.pages import router as pages_router
from webui.routes.pipeline import router as pipeline_router
from webui.routes.stats import router as stats_router
from webui.routes.subscriptions import router as subscriptions_router
from webui.routes.u115 import router as u115_router
from webui.services.watcher import shutdown_background_watchers as _shutdown_background_watchers
from webui.services.watcher import startup_background_watchers as _startup_background_watchers


app = FastAPI(
    title="Meta2Cloud 媒体库",
    description="网盘整理程序",
    version="1.0.0",
)


@app.on_event("startup")
async def startup_background_watchers():
    await _startup_background_watchers()


@app.on_event("shutdown")
async def shutdown_background_watchers():
    await _shutdown_background_watchers()


app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.middleware("http")
async def auth_http_middleware(request: Request, call_next):
    return await auth_middleware(request, call_next)


_STATIC_DIR = os.path.join(_ROOT_DIR, "frontend", "dist")
_ASSETS_DIR = os.path.join(_STATIC_DIR, "assets")
if os.path.exists(_ASSETS_DIR):
    app.mount("/assets", StaticFiles(directory=_ASSETS_DIR), name="assets")


app.include_router(auth_router)
app.include_router(aria2_router)
app.include_router(config_router)
app.include_router(pages_router)
app.include_router(library_router)
app.include_router(logs_router)
app.include_router(media_actions_router)
app.include_router(pipeline_router)
app.include_router(stats_router)
app.include_router(subscriptions_router)
app.include_router(u115_router)

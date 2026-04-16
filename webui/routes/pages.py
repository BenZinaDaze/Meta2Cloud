import os

from fastapi import APIRouter
from fastapi.responses import FileResponse, JSONResponse
from webui.core.runtime import _ROOT_DIR

router = APIRouter()


@router.get("/favicon.svg")
async def serve_favicon():
    favicon_path = os.path.join(_ROOT_DIR, "frontend", "dist", "favicon.svg")
    if os.path.exists(favicon_path):
        return FileResponse(favicon_path, media_type="image/svg+xml")
    return JSONResponse({"detail": "not found"}, status_code=404)


@router.get("/")
async def serve_index():
    index_path = os.path.join(_ROOT_DIR, "frontend", "dist", "index.html")
    if not os.path.exists(index_path):
        return JSONResponse({"detail": "前端未构建，请先运行 npm run build（或使用 Docker 镜像）"}, status_code=404)
    return FileResponse(index_path)

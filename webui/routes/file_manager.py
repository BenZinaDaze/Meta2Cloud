"""
文件管理 API 路由
"""
from typing import Optional

from fastapi import APIRouter, HTTPException, Query
from fastapi.concurrency import run_in_threadpool

from webui.schemas.file_manager import (
    CreateFileBody,
    UpdateFileBody,
    BatchActionBody,
)
from webui.services.file_manager import (
    list_files,
    get_file,
    create_folder,
    update_file,
    delete_file,
    batch_delete,
    batch_move,
)

router = APIRouter()


@router.get("/api/files")
async def api_list_files(
    folder_id: Optional[str] = Query(None),
    folder_name: Optional[str] = Query(None),
    search: Optional[str] = Query(None),
):
    """列举文件"""
    return await run_in_threadpool(list_files, folder_id, folder_name, search)


@router.get("/api/files/{file_id}")
async def api_get_file(
    file_id: str,
    include_path: bool = Query(False),
):
    """获取文件详情"""
    return await run_in_threadpool(get_file, file_id, include_path)


@router.post("/api/files")
async def api_create_file(body: CreateFileBody):
    """创建文件夹"""
    return await run_in_threadpool(create_folder, body.name, body.parent_id)


@router.patch("/api/files/{file_id}")
async def api_update_file(file_id: str, body: UpdateFileBody):
    """更新文件（重命名/移动）"""
    return await run_in_threadpool(update_file, file_id, body.name, body.parent_id)


@router.delete("/api/files/{file_id}")
async def api_delete_file(file_id: str):
    """删除文件"""
    return await run_in_threadpool(delete_file, file_id)


@router.post("/api/files/batch")
async def api_batch_action(body: BatchActionBody):
    """批量操作"""
    if body.action == "delete":
        count = await run_in_threadpool(batch_delete, body.file_ids)
        return {"action": "delete", "deleted_count": count}
    elif body.action == "move":
        if not body.parent_id:
            raise HTTPException(status_code=400, detail="move 操作需要提供 parent_id")
        count = await run_in_threadpool(batch_move, body.file_ids, body.parent_id)
        return {"action": "move", "moved_count": count}
    else:
        raise HTTPException(status_code=400, detail=f"未知操作: {body.action}")

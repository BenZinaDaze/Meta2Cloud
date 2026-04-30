from typing import List, Optional

from pydantic import BaseModel, Field


class PathBreadcrumbItem(BaseModel):
    """路径面包屑项"""
    id: str
    name: str


class CloudFileResponse(BaseModel):
    """文件信息响应"""
    id: str
    name: str
    file_type: str
    size: Optional[int] = None
    modified_time: Optional[str] = None
    parent_id: Optional[str] = None
    mime_type: Optional[str] = None
    is_video: bool = False
    is_subtitle: bool = False
    extension: str = ""
    extra: dict = Field(default_factory=dict)


class ListFilesResponse(BaseModel):
    """列举文件响应"""
    items: List[CloudFileResponse]
    folder_id: str
    folder_name: Optional[str] = None
    parent_id: Optional[str] = None
    provider: str
    root_id: str
    path: List[PathBreadcrumbItem] = []


class CreateFileBody(BaseModel):
    """创建文件/文件夹请求"""
    name: str
    parent_id: Optional[str] = None
    file_type: str = "folder"


class UpdateFileBody(BaseModel):
    """更新文件请求（重命名/移动）"""
    name: Optional[str] = None
    parent_id: Optional[str] = None


class FileDetailResponse(CloudFileResponse):
    """文件详情响应（含路径）"""
    path: List[PathBreadcrumbItem] = []


class BatchActionBody(BaseModel):
    """批量操作请求"""
    action: str = Field(..., pattern="^(delete|move)$")
    file_ids: List[str]
    parent_id: Optional[str] = None

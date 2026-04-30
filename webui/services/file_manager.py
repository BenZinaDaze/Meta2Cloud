"""
文件管理业务逻辑层
"""
import logging
from typing import List, Optional

from storage.base import CloudFile, FileType
from u115pan.errors import Pan115RateLimitError
from webui.core.runtime import get_config, get_storage_provider
from webui.schemas.file_manager import (
    CloudFileResponse,
    ListFilesResponse,
    PathBreadcrumbItem,
    FileDetailResponse,
)

logger = logging.getLogger("webui")


def _get_root_id(provider_name: str) -> str:
    """获取不同网盘的根目录 ID"""
    return {
        "google_drive": "root",
        "pan115": "0",
    }.get(provider_name, "root")


def _normalize_folder_id(folder_id: Optional[str], provider_name: str) -> str:
    """规范化文件夹 ID"""
    if not folder_id or folder_id == "root":
        return _get_root_id(provider_name)
    return folder_id


def _cloud_file_to_response(file: CloudFile) -> CloudFileResponse:
    """转换 CloudFile 为响应模型"""
    return CloudFileResponse(
        id=file.id,
        name=file.name,
        file_type=file.file_type.value,
        size=file.size,
        modified_time=file.modified_time,
        parent_id=file.parent_id,
        mime_type=file.mime_type,
        is_video=file.is_video,
        is_subtitle=file.is_subtitle,
        extension=file.extension,
        extra=file.extra,
    )


def list_files(
    folder_id: Optional[str] = None,
    folder_name: Optional[str] = None,
    search: Optional[str] = None,
) -> ListFilesResponse:
    """列举文件"""
    provider = get_storage_provider()
    cfg = get_config()
    provider_name = cfg.storage.primary
    root_id = _get_root_id(provider_name)

    normalized_folder_id = _normalize_folder_id(folder_id, provider_name)

    # 搜索模式
    if search:
        items = provider.list_files(folder_id=normalized_folder_id, page_size=1000)
        filtered = [f for f in items if search.lower() in f.name.lower()]
        response_items = [_cloud_file_to_response(f) for f in filtered]
        return ListFilesResponse(
            items=response_items,
            folder_id=normalized_folder_id,
            folder_name=f"搜索结果: {search}",
            parent_id=None,
            provider=provider_name,
            root_id=root_id,
            path=[PathBreadcrumbItem(id=root_id, name="根目录")],
        )

    # 正常列举
    items = provider.list_files(folder_id=normalized_folder_id, page_size=1000)
    response_items = [_cloud_file_to_response(f) for f in items]

    # 使用前端传来的文件夹名，或默认"根目录"
    display_name = folder_name or "根目录"

    # 构建路径面包屑
    path = _build_folder_path(provider, normalized_folder_id, root_id, folder_name)

    # 从列表中获取 parent_id（子文件的 parent_id 就是当前文件夹）
    parent_id = None
    if normalized_folder_id != root_id and items:
        parent_id = items[0].parent_id

    return ListFilesResponse(
        items=response_items,
        folder_id=normalized_folder_id,
        folder_name=display_name,
        parent_id=parent_id,
        provider=provider_name,
        root_id=root_id,
        path=path,
    )


def _build_folder_path(
    provider,
    folder_id: str,
    root_id: str,
    folder_name: Optional[str] = None,
) -> List[PathBreadcrumbItem]:
    """构建文件夹路径面包屑"""
    # 根目录
    if folder_id == root_id:
        return [PathBreadcrumbItem(id=root_id, name="根目录")]

    # 当前文件夹
    path = [PathBreadcrumbItem(id=root_id, name="根目录")]
    if folder_name:
        path.append(PathBreadcrumbItem(id=folder_id, name=folder_name))
    else:
        path.append(PathBreadcrumbItem(id=folder_id, name=folder_id))

    return path


def get_file(file_id: str, include_path: bool = False) -> FileDetailResponse:
    """获取文件详情"""
    provider = get_storage_provider()
    cfg = get_config()
    provider_name = cfg.storage.primary
    root_id = _get_root_id(provider_name)

    file = provider.get_file(file_id)
    response = _cloud_file_to_response(file)

    result = FileDetailResponse(
        **response.model_dump(),
        path=[],
    )

    if include_path:
        path = _build_path(provider, file, root_id)
        result.path = path

    return result


def _build_path(provider, file: CloudFile, root_id: str) -> List[PathBreadcrumbItem]:
    """构建路径面包屑"""
    path = []

    # 从当前文件向上追溯
    current = file
    while current:
        path.insert(0, PathBreadcrumbItem(id=current.id, name=current.name))
        if current.parent_id and current.parent_id != root_id:
            try:
                current = provider.get_file(current.parent_id)
            except Exception:
                break
        else:
            break

    # 添加根目录
    path.insert(0, PathBreadcrumbItem(id=root_id, name="根目录"))

    return path


def create_folder(name: str, parent_id: Optional[str] = None) -> CloudFileResponse:
    """创建文件夹"""
    provider = get_storage_provider()
    cfg = get_config()
    provider_name = cfg.storage.primary

    normalized_parent_id = _normalize_folder_id(parent_id, provider_name)
    folder = provider.create_folder(name, parent_id=normalized_parent_id)
    return _cloud_file_to_response(folder)


def update_file(file_id: str, name: Optional[str] = None, parent_id: Optional[str] = None) -> CloudFileResponse:
    """更新文件（重命名/移动）"""
    provider = get_storage_provider()

    if name and parent_id:
        # 同时重命名和移动
        file = provider.move_file(file_id, parent_id, new_name=name)
    elif name:
        # 仅重命名
        file = provider.rename_file(file_id, name)
    elif parent_id:
        # 仅移动
        file = provider.move_file(file_id, parent_id)
    else:
        raise ValueError("至少需要提供 name 或 parent_id")

    return _cloud_file_to_response(file)


def delete_file(file_id: str) -> None:
    """删除文件"""
    provider = get_storage_provider()
    provider.delete_file(file_id)


def batch_delete(file_ids: List[str]) -> int:
    """批量删除"""
    provider = get_storage_provider()
    count = 0
    for file_id in file_ids:
        try:
            provider.delete_file(file_id)
            count += 1
        except Pan115RateLimitError as e:
            logger.warning(f"删除操作触发限流，已处理 {count}/{len(file_ids)} 个: {e}")
            break
        except Exception as e:
            logger.warning(f"删除文件 {file_id} 失败: {e}")
    return count


def batch_move(file_ids: List[str], target_parent_id: str) -> int:
    """批量移动"""
    provider = get_storage_provider()
    count = 0
    for file_id in file_ids:
        try:
            provider.move_file(file_id, target_parent_id)
            count += 1
        except Pan115RateLimitError as e:
            logger.warning(f"移动操作触发限流，已处理 {count}/{len(file_ids)} 个: {e}")
            break
        except Exception as e:
            logger.warning(f"移动文件 {file_id} 失败: {e}")
    return count

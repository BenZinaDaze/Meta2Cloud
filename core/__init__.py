"""
core —— Meta2Cloud 核心业务逻辑

模块：
  pipeline  : 整理流水线（扫描 → 解析 → TMDB → NFO → 移动）
  organizer : 媒体文件夹整理器

用法：
  python -m core              # 运行整理流程
  python -m core --dry-run    # 预览模式
"""

from core.pipeline import Pipeline
from core.organizer import MediaOrganizer

__all__ = ["Pipeline", "MediaOrganizer"]

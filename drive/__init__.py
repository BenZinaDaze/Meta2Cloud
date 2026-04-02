"""
drive —— Google Drive 文件操作封装

支持两种认证方式：
  - Service Account（推荐用于自动化/服务器场景）
  - OAuth2（用于个人授权场景）

快速使用：
    from drive import DriveClient

    client = DriveClient.from_service_account("service_account.json")
    # 或
    client = DriveClient.from_oauth("credentials.json", "token.json")

    files = client.list_media_files(folder_id="xxx")
    client.rename_file(file_id="yyy", new_name="新名称.mkv")
    client.upload_text(content="<nfo>...</nfo>", name="movie.nfo", parent_id="xxx")
"""

from drive.auth import DriveAuth
from drive.client import DriveClient, DriveFile

__all__ = [
    "DriveAuth",
    "DriveClient",
    "DriveFile",
]

"""
drive —— Google Drive 文件操作封装（OAuth2 认证）

快速使用：
    from drive import DriveClient

    client = DriveClient.from_oauth("config/credentials.json", "config/token.json")

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

"""
drive/auth.py —— Google Drive OAuth2 认证
"""

import os
from typing import Optional

from google.oauth2.credentials import Credentials
from google.auth.transport.requests import Request
from google_auth_oauthlib.flow import InstalledAppFlow
from googleapiclient.discovery import build

# Drive 所需权限
SCOPES = ["https://www.googleapis.com/auth/drive"]


class DriveAuth:
    """Google Drive OAuth2 认证，返回 googleapiclient 的 Resource 对象"""

    @staticmethod
    def from_oauth(
        credentials_path: str = "config/credentials.json",
        token_path: str = "config/token.json",
        scopes: Optional[list] = None,
    ):
        """
        OAuth2 认证（适合个人账号）

        参数：
            credentials_path : 从 Google Cloud Console 下载的 OAuth2 JSON
            token_path       : 本地缓存 token，首次运行后自动生成

        首次使用会弹出浏览器授权页面。
        """
        scopes = scopes or SCOPES
        creds: Optional[Credentials] = None

        # 读取已有 token
        if os.path.exists(token_path):
            creds = Credentials.from_authorized_user_file(token_path, scopes)

        # token 无效或过期则刷新/重新授权
        if not creds or not creds.valid:
            if creds and creds.expired and creds.refresh_token:
                creds.refresh(Request())
            else:
                if not os.path.exists(credentials_path):
                    raise FileNotFoundError(
                        f"OAuth2 凭据文件不存在：{credentials_path}\n"
                        "请到 Google Cloud Console → API → 凭据 → 创建 OAuth2 ClientID(桌面应用)"
                    )
                flow = InstalledAppFlow.from_client_secrets_file(
                    credentials_path, scopes
                )
                creds = flow.run_local_server(port=0)

            # 保存 token 供下次复用
            with open(token_path, "w", encoding="utf-8") as fh:
                fh.write(creds.to_json())

        return build("drive", "v3", credentials=creds)

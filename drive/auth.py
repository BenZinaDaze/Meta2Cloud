"""
drive/auth.py —— Google Drive 认证

支持两种方式：
  1. Service Account  适合服务器/CI 自动化，无需用户交互
  2. OAuth2           适合个人账号，首次运行弹浏览器授权
"""

import os
from typing import Optional

from google.oauth2 import service_account
from google.oauth2.credentials import Credentials
from google.auth.transport.requests import Request
from google_auth_oauthlib.flow import InstalledAppFlow
from googleapiclient.discovery import build

# 只需要读写自己的 Drive 文件
SCOPES_FULL = ["https://www.googleapis.com/auth/drive"]
# 只能读写本应用创建的文件（权限更小，更安全）
SCOPES_FILE = ["https://www.googleapis.com/auth/drive.file"]


class DriveAuth:
    """Google Drive 认证工厂，返回 googleapiclient 的 Resource 对象"""

    @staticmethod
    def from_service_account(
        json_path: str,
        scopes: Optional[list] = None,
    ):
        """
        Service Account 认证（推荐用于自动化）

        参数：
            json_path : service_account.json 路径
            scopes    : 默认 SCOPES_FULL

        前提：
            在 Google Cloud Console 创建服务账号，下载 JSON Key，
            并在 Google Drive 将目标文件夹共享给服务账号邮箱。
        """
        if not os.path.exists(json_path):
            raise FileNotFoundError(
                f"Service Account 文件不存在：{json_path}\n"
                "请到 Google Cloud Console → IAM → 服务账号 → 创建密钥(JSON)"
            )
        scopes = scopes or SCOPES_FULL
        creds = service_account.Credentials.from_service_account_file(
            json_path, scopes=scopes
        )
        return build("drive", "v3", credentials=creds)

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
            scopes           : 默认 SCOPES_FULL

        首次使用会弹出浏览器授权页面。
        """
        scopes = scopes or SCOPES_FULL
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

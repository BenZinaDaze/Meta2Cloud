"""
uquark/auth.py —— 夸克网盘认证相关
"""

from __future__ import annotations

import json
import os
import time
from typing import Optional

from .errors import QuarkAuthError
from .models import QuarkToken


TOKEN_VERSION = 1


def save_token(token: QuarkToken, token_path: str) -> None:
    """保存 token 到文件"""
    os.makedirs(os.path.dirname(token_path) or ".", exist_ok=True)
    data = token.to_dict()
    data["_version"] = TOKEN_VERSION
    with open(token_path, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=2)


def load_token(token_path: str) -> Optional[QuarkToken]:
    """从文件加载 token"""
    if not os.path.exists(token_path):
        return None
    try:
        with open(token_path, "r", encoding="utf-8") as f:
            data = json.load(f)
        if data.get("_version") != TOKEN_VERSION:
            return None
        return QuarkToken.from_dict(data)
    except (json.JSONDecodeError, KeyError, ValueError):
        return None


class QuarkAuth:
    """夸克网盘认证管理"""

    # 夸克网盘 API 基础地址
    API_BASE = "https://drive-pc.quark.cn"

    def __init__(
        self,
        token: Optional[QuarkToken] = None,
        token_path: Optional[str] = None,
        on_token_updated: Optional[callable] = None,
    ):
        self.token = token
        self.token_path = token_path
        self.on_token_updated = on_token_updated

    def get_valid_access_token(self) -> str:
        """获取可用 access_token"""
        if not self.token:
            raise QuarkAuthError("未配置夸克 token，请先完成授权")
        if self.token.is_expired():
            self.refresh_token()
        return self.token.access_token

    def refresh_token(self) -> QuarkToken:
        """刷新 access_token"""
        if not self.token:
            raise QuarkAuthError("没有可刷新的 token")
        if not self.token.refresh_token:
            raise QuarkAuthError("没有 refresh_token，无法刷新")

        import requests

        url = f"{self.API_BASE}/1.0/oauth/token"
        data = {
            "grant_type": "refresh_token",
            "refresh_token": self.token.refresh_token,
        }
        headers = {"Content-Type": "application/x-www-form-urlencoded"}

        resp = requests.post(url, data=data, headers=headers, timeout=30)
        payload = resp.json()

        if payload.get("code") != 0:
            raise QuarkAuthError(
                payload.get("message", "刷新 token 失败"),
                code=payload.get("code"),
                payload=payload,
            )

        token_data = payload.get("data", {})
        self.token = QuarkToken(
            access_token=str(token_data["access_token"]),
            refresh_token=str(token_data["refresh_token"]),
            expires_in=int(token_data.get("expires_in", 7200)),
            refresh_time=int(time.time()),
            uid=self.token.uid or str(token_data.get("uid", "")),
        )
        self._persist_token()
        self._emit_token_updated()
        return self.token

    def _persist_token(self) -> None:
        if self.token and self.token_path:
            save_token(self.token, self.token_path)

    def _emit_token_updated(self) -> None:
        if self.on_token_updated is not None and self.token is not None:
            self.on_token_updated(self.token)

"""
u115pan/auth.py —— 115 PKCE 与 token 持久化工具
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import secrets
from typing import Optional

from .models import Pan115Token


def generate_code_verifier(length: int = 64) -> str:
    """
    生成 PKCE code_verifier。

    115 文档要求长度 43~128，默认生成 64 位。
    """
    if length < 43 or length > 128:
        raise ValueError("code_verifier 长度必须在 43~128 之间")
    return secrets.token_urlsafe(length)[:length]


def generate_code_challenge(code_verifier: str) -> str:
    """根据 code_verifier 计算 SHA256 challenge"""
    digest = hashlib.sha256(code_verifier.encode("utf-8")).digest()
    return base64.urlsafe_b64encode(digest).decode("utf-8").rstrip("=")


def save_token(token: Pan115Token, token_path: str) -> None:
    """将 token 写入本地 JSON 文件"""
    os.makedirs(os.path.dirname(token_path) or ".", exist_ok=True)
    with open(token_path, "w", encoding="utf-8") as fh:
        json.dump(token.to_dict(), fh, ensure_ascii=False, indent=2)


def load_token(token_path: str) -> Optional[Pan115Token]:
    """从本地 JSON 文件读取 token，不存在则返回 None"""
    if not os.path.exists(token_path):
        return None
    with open(token_path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    return Pan115Token.from_dict(data)

import hmac
import os
import secrets
from datetime import datetime, timedelta, timezone
from typing import Optional

from fastapi import Request
from fastapi.responses import JSONResponse

try:
    import jwt as _pyjwt
except ImportError:
    _pyjwt = None  # type: ignore


_ROOT_DIR = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
_JWT_SECRET_FILE = os.path.join(_ROOT_DIR, "config", "data", ".jwt_secret")
_jwt_secret_cache: Optional[str] = None


def _get_config():
    from webui.core.runtime import get_config

    return get_config()


def get_jwt_secret() -> str:
    global _jwt_secret_cache
    if _jwt_secret_cache:
        return _jwt_secret_cache

    cfg = _get_config()
    if cfg.webui.secret_key:
        _jwt_secret_cache = cfg.webui.secret_key
        return _jwt_secret_cache

    try:
        if os.path.exists(_JWT_SECRET_FILE):
            _jwt_secret_cache = open(_JWT_SECRET_FILE).read().strip()
            return _jwt_secret_cache
    except Exception:
        pass

    _jwt_secret_cache = secrets.token_hex(32)
    try:
        os.makedirs(os.path.dirname(_JWT_SECRET_FILE), exist_ok=True)
        with open(_JWT_SECRET_FILE, "w") as f:
            f.write(_jwt_secret_cache)
    except Exception:
        from webui.core.runtime import logger

        logger.warning("无法持久化 JWT 密钥")
    return _jwt_secret_cache


def create_token(username: str, expire_hours: int) -> str:
    if _pyjwt is None:
        raise RuntimeError("请先 pip install PyJWT")
    payload = {
        "sub": username,
        "iat": datetime.now(timezone.utc),
        "exp": datetime.now(timezone.utc) + timedelta(hours=expire_hours),
    }
    return _pyjwt.encode(payload, get_jwt_secret(), algorithm="HS256")


def verify_token(token: str) -> Optional[str]:
    if _pyjwt is None:
        return None
    try:
        payload = _pyjwt.decode(token, get_jwt_secret(), algorithms=["HS256"])
        return payload.get("sub")
    except Exception:
        return None


async def auth_middleware(request: Request, call_next):
    path = request.url.path
    if path == "/api/auth:login" or path.startswith("/trigger") or not path.startswith("/api/"):
        return await call_next(request)

    auth = request.headers.get("Authorization", "")
    if not auth.startswith("Bearer "):
        return JSONResponse({"detail": "未授权，请先登录"}, status_code=401)

    token = auth[7:]
    username = verify_token(token)
    if not username:
        return JSONResponse({"detail": "Token 已过期或无效，请重新登录"}, status_code=401)
    return await call_next(request)


def credentials_match(username: str, password: str) -> bool:
    cfg = _get_config()
    cfg_user = cfg.webui.username or "admin"
    cfg_pass = cfg.webui.password or ""
    return hmac.compare_digest(username.encode(), cfg_user.encode()) and hmac.compare_digest(
        password.encode(), cfg_pass.encode()
    )

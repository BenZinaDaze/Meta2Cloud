from fastapi import APIRouter, HTTPException, Request

from webui.core.auth import create_token, credentials_match, verify_token
from webui.core.runtime import get_config
from webui.schemas.auth import LoginRequest

router = APIRouter()


@router.post("/api/auth/login")
async def auth_login(body: LoginRequest):
    cfg = get_config()
    cfg_pass = cfg.webui.password or ""

    if not credentials_match(body.username, body.password):
        raise HTTPException(status_code=401, detail="用户名或密码错误")

    if not cfg_pass:
        raise HTTPException(status_code=403, detail="密码未设置，请先在配置文件中设置 webui.password")

    token = create_token(body.username, cfg.webui.token_expire_hours)
    return {"token": token, "username": body.username, "expire_hours": cfg.webui.token_expire_hours}


@router.get("/api/auth/me")
async def auth_me(request: Request):
    auth = request.headers.get("Authorization", "")
    token = auth[7:] if auth.startswith("Bearer ") else ""
    username = verify_token(token)
    if not username:
        raise HTTPException(status_code=401, detail="Token 无效")
    return {"username": username}


@router.post("/api/auth/logout")
async def auth_logout():
    return {"ok": True}

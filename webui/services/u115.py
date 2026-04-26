import io
import json
from dataclasses import asdict

import requests
from fastapi import HTTPException
from fastapi.responses import Response
import webui.core.runtime as runtime
from webui.core.runtime import get_config, logger
from u115pan.errors import Pan115AuthError


def resolve_config_path(path_str: str) -> str:
    path = runtime.Path(path_str)
    if not path.is_absolute():
        path = runtime.Path(runtime._ROOT_DIR) / path
    return str(path)


def u115_client():
    cfg = get_config().u115
    client = runtime._u115_runtime.get_client(client_id=cfg.client_id, token_path=resolve_config_path(cfg.token_json))
    if cfg.cookie.strip():
        client.set_cookie(cfg.cookie)
    return client


def u115_offline_client():
    return runtime.u115pan.OfflineClient(u115_client())


def serialize_u115_offline_task(task):
    return {
        "info_hash": task.info_hash,
        "name": task.name,
        "status": task.status,
        "percent_done": task.percent_done,
        "size": task.size,
        "add_time": task.add_time,
        "last_update": task.last_update,
        "file_id": task.file_id,
        "delete_file_id": task.delete_file_id,
        "url": task.url,
        "wp_path_id": task.wp_path_id,
        "is_finished": task.is_finished,
        "is_downloading": task.is_downloading,
        "is_failed": task.is_failed,
    }


def serialize_u115_offline_quota(quota):
    return {
        "count": quota.count,
        "used": quota.used,
        "surplus": quota.surplus,
        "packages": [
            {
                "name": item.name,
                "count": item.count,
                "used": item.used,
                "surplus": item.surplus,
                "expire_info": [{"surplus": exp.surplus, "expire_time": exp.expire_time} for exp in item.expire_info],
            }
            for item in quota.packages
        ],
    }


def save_u115_device_session(session, session_path: str) -> None:
    path = runtime.Path(session_path)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(
            {
                "qrcode": session.qrcode,
                "uid": session.uid,
                "time_value": session.time_value,
                "sign": session.sign,
                "code_verifier": session.code_verifier,
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )


def load_u115_device_session(session_path: str):
    path = runtime.Path(session_path)
    if not path.exists():
        raise FileNotFoundError(f"115 扫码会话文件不存在：{path}")
    data = json.loads(path.read_text(encoding="utf-8"))
    return runtime.u115pan.DeviceCodeSession(
        qrcode=str(data["qrcode"]),
        uid=str(data["uid"]),
        time_value=str(data["time_value"]),
        sign=str(data["sign"]),
        code_verifier=str(data["code_verifier"]),
    )


def u115_oauth_status_payload():
    cfg = get_config().u115
    token_path = resolve_config_path(cfg.token_json)
    session_path = resolve_config_path(cfg.session_json)
    token_exists = runtime.os.path.exists(token_path)
    session_exists = runtime.os.path.exists(session_path)
    token_valid = False
    token_expired = False
    expires_at = None
    refresh_time = None
    authorized = False
    refreshed = False
    if token_exists:
        try:
            token = runtime.u115pan.load_token(token_path)
            if token:
                refresh_time = token.refresh_time
                expires_at = token.expires_at
                token_expired = token.is_expired(skew_seconds=0)
                if token_expired:
                    try:
                        client = runtime._u115_runtime.get_client(client_id=cfg.client_id, token_path=token_path)
                        token = client.refresh_token()
                        runtime._u115_runtime.sync_token(client_id=cfg.client_id, token_path=token_path, token=token)
                        refreshed = True
                        refresh_time = token.refresh_time
                        expires_at = token.expires_at
                        token_expired = False
                        token_valid = True
                        authorized = True
                    except Exception:
                        token_valid = False
                        authorized = False
                else:
                    token_valid = True
                    authorized = True
        except Exception:
            authorized = False
    return {
        "client_id": cfg.client_id,
        "token_path": cfg.token_json,
        "session_path": cfg.session_json,
        "cookie_configured": bool(cfg.cookie.strip()),
        "token_exists": token_exists,
        "session_exists": session_exists,
        "authorized": authorized,
        "token_valid": token_valid,
        "token_expired": token_expired,
        "refresh_time": refresh_time,
        "expires_at": expires_at,
        "refreshed": refreshed,
    }


def u115_oauth_create_sync(body=None):
    cfg = get_config().u115
    client_id = (body.client_id if body and body.client_id else cfg.client_id).strip()
    token_json = (body.token_json if body and body.token_json else cfg.token_json).strip()
    session_json = cfg.session_json.strip()
    if not client_id:
        raise HTTPException(status_code=400, detail="u115.client_id 不能为空")
    client = runtime._u115_runtime.get_client(client_id=client_id, token_path=resolve_config_path(token_json))
    try:
        session = client.create_device_code()
        save_u115_device_session(session, resolve_config_path(session_json))
        return {"ok": True, "qrcode": session.qrcode, "uid": session.uid, "session_path": session_json, "status": u115_oauth_status_payload()}
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 创建扫码会话失败")
        raise HTTPException(status_code=400, detail=f"115 创建扫码会话失败：{exc}") from exc


def u115_oauth_qrcode_sync():
    cfg = get_config().u115
    try:
        import qrcode

        session = load_u115_device_session(resolve_config_path(cfg.session_json))
        qr = qrcode.QRCode(border=2, box_size=8)
        qr.add_data(session.qrcode)
        qr.make(fit=True)
        image = qr.make_image(fill_color="black", back_color="white")
        buffer = io.BytesIO()
        image.save(buffer, format="PNG")
        return Response(content=buffer.getvalue(), media_type="image/png")
    except Exception as exc:
        logger.warning(f"115 二维码生成失败: {exc}")
        raise HTTPException(status_code=400, detail=f"115 二维码生成失败：{exc}") from exc


def u115_oauth_poll_sync():
    cfg = get_config().u115
    try:
        client = u115_client()
        session_path = resolve_config_path(cfg.session_json)
        session = load_u115_device_session(session_path)
        status = client.get_qrcode_status(session)
        if status.confirmed:
            try:
                token = client.exchange_device_token(session)
                runtime._u115_runtime.sync_token(
                    client_id=cfg.client_id,
                    token_path=resolve_config_path(cfg.token_json),
                    token=token,
                )
                runtime._invalidate_u115_runtime_cache()
                try:
                    on_disk = load_u115_device_session(session_path)
                    if on_disk.uid == session.uid:
                        runtime.os.remove(session_path)
                except (FileNotFoundError, Exception):
                    pass
                return {"ok": True, "status": status.status, "message": "已确认并完成授权", "confirmed": True, "authorized": True, "raw": status.raw}
            except (requests.Timeout, requests.ConnectionError) as exc:
                logger.warning(f"115 API 网络不通: {exc}")
                return {"ok": False, "status": status.status, "message": "无法连接 115 服务器", "confirmed": True, "raw": status.raw}
            except Exception as exc:
                logger.error(f"115 换取 token 失败: {exc}")
                return {"ok": False, "status": status.status, "message": f"换取 token 失败：{exc}", "confirmed": True, "raw": status.raw}
        return {
            "ok": True,
            "status": status.status,
            "message": status.message,
            "confirmed": status.confirmed,
            "raw": status.raw,
        }
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 查询扫码状态失败")
        raise HTTPException(status_code=400, detail=f"115 查询扫码状态失败：{exc}") from exc


def u115_oauth_exchange_sync(body=None):
    cfg = get_config().u115
    client_id = (body.client_id if body and body.client_id else cfg.client_id).strip()
    token_json = (body.token_json if body and body.token_json else cfg.token_json).strip()
    session_json = cfg.session_json.strip()
    if not client_id:
        raise HTTPException(status_code=400, detail="u115.client_id 不能为空")
    client = runtime._u115_runtime.get_client(client_id=client_id, token_path=resolve_config_path(token_json))
    try:
        resolved_session_path = resolve_config_path(session_json)
        session = load_u115_device_session(resolved_session_path)
        token = client.exchange_device_token(session)
        runtime._u115_runtime.sync_token(client_id=client_id, token_path=resolve_config_path(token_json), token=token)
        runtime._invalidate_u115_runtime_cache()
        try:
            on_disk = load_u115_device_session(resolved_session_path)
            if on_disk.uid == session.uid:
                runtime.os.remove(resolved_session_path)
        except (FileNotFoundError, Exception):
            pass
        return {
            "ok": True,
            "expires_in": token.expires_in,
            "refresh_time": token.refresh_time,
            "token_path": token_json,
            "status": u115_oauth_status_payload(),
        }
    except FileNotFoundError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 换取 token 失败")
        raise HTTPException(status_code=400, detail=f"115 换取 token 失败：{exc}") from exc


def u115_test_connection_sync():
    try:
        client = u115_client()
        space = client.get_space_info()
        return {"ok": True, "total_space": space.total_size, "remain_space": space.remain_size}
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通 (proapi.115.com): {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 连接测试失败")
        raise HTTPException(status_code=400, detail=f"115 连接测试失败：{exc}") from exc


def u115_test_cookie_sync():
    cfg = get_config().u115
    if not cfg.cookie.strip():
        raise HTTPException(status_code=400, detail="u115.cookie 未配置")
    try:
        client = u115_client()
        user_info = client.get_user_info()
        if not user_info:
            raise HTTPException(status_code=400, detail="Cookie 不可用")
        return {"ok": True, **user_info}
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except HTTPException:
        raise
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通 (proapi.115.com): {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 Cookie 测试失败")
        raise HTTPException(status_code=400, detail=f"115 Cookie 测试失败：{exc}") from exc


def u115_offline_overview_sync(page: int):
    try:
        offline = u115_offline_client()
        task_page = offline.get_task_list(page=page)
        return {
            "ok": True,
            "tasks": [serialize_u115_offline_task(task) for task in task_page.tasks],
            "pagination": {
                "page": task_page.page,
                "page_size": len(task_page.tasks),
                "total": task_page.count,
                "total_pages": max(task_page.page_count, 1),
                "has_prev": task_page.page > 1,
                "has_next": task_page.page < max(task_page.page_count, 1),
            },
        }
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 云下载概览获取失败")
        raise HTTPException(status_code=400, detail=f"115 云下载概览获取失败：{exc}") from exc


def u115_offline_quota_sync():
    try:
        offline = u115_offline_client()
        quota = offline.get_quota_info()
        return {"ok": True, "quota": serialize_u115_offline_quota(quota)}
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 云下载配额获取失败")
        raise HTTPException(status_code=400, detail=f"115 云下载配额获取失败：{exc}") from exc


def u115_offline_add_urls_sync(body):
    url_lines = [line.strip() for line in body.urls.splitlines() if line.strip()]
    if not url_lines:
        raise HTTPException(status_code=400, detail="请至少输入一个下载链接")
    try:
        offline = u115_offline_client()
        cfg = get_config().u115
        target_path_id = (body.wp_path_id.strip() if body.wp_path_id else "") or (cfg.download_folder_id.strip() if cfg.download_folder_id else "") or None
        results = offline.add_task_urls(url_lines, wp_path_id=target_path_id)
        return {"ok": True, "count": len(results), "results": [asdict(item) for item in results], "wp_path_id": target_path_id}
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 云下载添加链接失败")
        raise HTTPException(status_code=400, detail=f"115 云下载添加链接失败：{exc}") from exc


def u115_offline_delete_tasks_sync(body):
    info_hashes = [item.strip() for item in body.info_hashes if item and item.strip()]
    if not info_hashes:
        raise HTTPException(status_code=400, detail="请至少选择一个云下载任务")
    try:
        offline = u115_offline_client()
        for info_hash in info_hashes:
            offline.del_task(info_hash, del_source_file=body.del_source_file)
        return {"ok": True, "deleted": len(info_hashes)}
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 云下载删除任务失败")
        raise HTTPException(status_code=400, detail=f"115 云下载删除任务失败：{exc}") from exc


def u115_offline_clear_tasks_sync(body):
    try:
        offline = u115_offline_client()
        offline.clear_tasks(body.flag)
        return {"ok": True, "flag": body.flag}
    except Pan115AuthError as exc:
        logger.warning(f"115 认证失效: {exc}")
        raise HTTPException(status_code=400, detail="115 Token 已失效，请重新扫码授权") from exc
    except (requests.Timeout, requests.ConnectionError) as exc:
        logger.warning(f"115 API 网络不通: {exc}")
        raise HTTPException(status_code=400, detail="无法连接 115 服务器，请检查网络或服务器是否在国内") from exc
    except Exception as exc:
        logger.exception("115 云下载清空任务失败")
        raise HTTPException(status_code=400, detail=f"115 云下载清空任务失败：{exc}") from exc

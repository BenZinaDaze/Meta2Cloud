from __future__ import annotations

import re
from typing import Any

import requests
from fastapi import HTTPException

from mediaparser import Config
from webui.core.app_logging import app_log
from webui.core.runtime import logger


def _summarize_data(data: Any) -> str:
    if data is None:
        return "null"
    if isinstance(data, dict):
        return f"object(keys={sorted(data.keys())})"
    if isinstance(data, list):
        return f"array(len={len(data)})"
    return type(data).__name__


def _normalize_scheme_host(scheme: str, host: str) -> tuple[str, str]:
    normalized_scheme = (scheme or "http").strip().lower()
    if normalized_scheme not in ("http", "https"):
        raise HTTPException(status_code=400, detail="MediaVault 协议仅支持 http 或 https")
    normalized_host = (host or "").strip()
    normalized_host = re.sub(r"^https?://", "", normalized_host, flags=re.IGNORECASE).rstrip("/")
    if not normalized_host:
        raise HTTPException(status_code=400, detail="MediaVault 地址不能为空")
    return normalized_scheme, normalized_host


def test_mediavault_connection_payload(body=None) -> dict[str, Any]:
    cfg = Config.load()
    mv_cfg = cfg.mediavault
    scheme = body.scheme if body and body.scheme is not None else mv_cfg.scheme
    host = body.host if body and body.host is not None else mv_cfg.host
    api_key = body.api_key if body and body.api_key is not None else mv_cfg.api_key
    timeout = body.timeout if body and body.timeout is not None else mv_cfg.timeout

    normalized_scheme, normalized_host = _normalize_scheme_host(str(scheme or ""), str(host or ""))
    api_key = str(api_key or "").strip()
    if not api_key:
        raise HTTPException(status_code=400, detail="MediaVault API Key 不能为空")

    try:
        timeout_value = int(timeout or 10)
    except (TypeError, ValueError):
        raise HTTPException(status_code=400, detail="MediaVault 请求超时必须是整数")
    if timeout_value < 1 or timeout_value > 120:
        raise HTTPException(status_code=400, detail="MediaVault 请求超时必须在 1 到 120 之间")

    endpoint = f"{normalized_scheme}://{normalized_host}/api/v1"

    try:
        response = requests.post(
            endpoint,
            params={"api_key": api_key},
            timeout=timeout_value,
        )
        try:
            payload = response.json()
        except Exception:
            payload = None

        detail = ""
        if isinstance(payload, dict):
            detail = str(payload.get("detail") or "")

        ok = response.status_code == 405 and detail == "Method Not Allowed"
        if ok:
            return {
                "ok": True,
                "message": "MediaVault 接口连通，API Key 可用",
                "status_code": response.status_code,
                "detail": detail,
            }
        raise HTTPException(
            status_code=400,
            detail=f"MediaVault 测试失败：HTTP {response.status_code}{f'，{detail}' if detail else ''}",
        )
    except HTTPException:
        raise
    except requests.RequestException as exc:
        logger.warning("MediaVault 测试请求失败：%s", exc)
        raise HTTPException(status_code=400, detail=f"MediaVault 测试失败：{exc}") from exc


def trigger_mediavault(cfg: Config, run_id: str, ok_count: int) -> bool:
    mv = cfg.mediavault
    if not mv.enabled:
        return False

    base_url = cfg.mediavault_base_url
    api_key = (mv.api_key or "").strip()
    source_dir = (mv.source_dir or "").strip()
    source_dir_set = bool(source_dir)

    if not base_url or not api_key:
        logger.warning("MediaVault 已启用但配置不完整，跳过触发")
        app_log(
            "integration",
            "mediavault_skipped_invalid_config",
            "MediaVault 已启用但配置不完整，跳过触发",
            level="WARNING",
            details={"runId": run_id, "okCount": ok_count},
        )
        return False

    endpoint = f"{base_url.rstrip('/')}/api/v1/monitor/trigger-organize"
    params = {"api_key": api_key}
    if source_dir_set:
        params["source_dir"] = source_dir

    app_log(
        "integration",
        "mediavault_trigger_start",
        "开始触发 MediaVault 整理",
        details={
            "runId": run_id,
            "okCount": ok_count,
            "baseUrl": base_url,
            "sourceDirSet": source_dir_set,
        },
    )

    try:
        response = requests.post(endpoint, params=params, timeout=mv.timeout)
        status_code = response.status_code
        try:
            payload = response.json()
        except Exception as exc:
            logger.warning("MediaVault 响应解析失败：%s", exc)
            app_log(
                "integration",
                "mediavault_trigger_failed",
                "MediaVault 响应解析失败",
                level="ERROR",
                details={
                    "runId": run_id,
                    "okCount": ok_count,
                    "statusCode": status_code,
                    "sourceDirSet": source_dir_set,
                    "error": str(exc),
                },
            )
            return False

        success = payload.get("success")
        message = str(payload.get("message") or "")
        data_summary = _summarize_data(payload.get("data"))
        if success is True:
            app_log(
                "integration",
                "mediavault_trigger_success",
                "MediaVault 整理触发成功",
                level="SUCCESS",
                details={
                    "runId": run_id,
                    "okCount": ok_count,
                    "statusCode": status_code,
                    "success": True,
                    "message": message,
                    "data": data_summary,
                    "sourceDirSet": source_dir_set,
                },
            )
            return True

        logger.warning("MediaVault 触发失败：%s", message or "success=false")
        app_log(
            "integration",
            "mediavault_trigger_failed",
            "MediaVault 整理触发失败",
            level="ERROR",
            details={
                "runId": run_id,
                "okCount": ok_count,
                "statusCode": status_code,
                "success": bool(success) if isinstance(success, bool) else None,
                "message": message,
                "data": data_summary,
                "sourceDirSet": source_dir_set,
            },
        )
        return False
    except Exception as exc:
        logger.warning("MediaVault 请求异常：%s", exc)
        app_log(
            "integration",
            "mediavault_trigger_failed",
            "MediaVault 请求异常",
            level="ERROR",
            details={
                "runId": run_id,
                "okCount": ok_count,
                "sourceDirSet": source_dir_set,
                "error": str(exc),
            },
        )
        return False

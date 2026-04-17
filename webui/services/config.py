import re

import yaml
from fastapi import HTTPException
from google.oauth2.credentials import Credentials
from webui.services.u115 import resolve_config_path
import webui.core.runtime as runtime
from webui.core.app_logging import app_log
from webui.services.tmdb_service import get_tmdb_cache, serialize_meta, serialize_tmdb_result
from webui.core.runtime import (
    _CONFIG_PATH,
    _PARSER_RULES_PATH,
    _extract_parser_rules,
    _get_config_signature,
    _get_release_group_matcher,
    _read_merged_config_dict,
    _read_yaml_dict,
    _validate_main_config_payload,
    get_config,
    get_drive_client,
    logger,
)
from mediaparser import MetaInfo, MetaInfoPath, TmdbClient


def read_config_payload():
    if not _CONFIG_PATH.exists():
        raise HTTPException(status_code=404, detail="config.yaml 不存在")
    return _read_merged_config_dict()


def read_main_config_payload():
    if not _CONFIG_PATH.exists():
        raise HTTPException(status_code=404, detail="config.yaml 不存在")
    return _read_yaml_dict(_CONFIG_PATH)


def read_parser_rules_payload():
    return _extract_parser_rules({"parser": _read_yaml_dict(_PARSER_RULES_PATH)})


def looks_like_file_path(text: str) -> bool:
    if " / " in text or " \\" in text or "\\ " in text:
        return False
    if re.match(r"^([a-zA-Z]:[\\/]|\\\\|/|\./|\.\./)", text):
        return True
    if text.startswith("[") or text.startswith("【"):
        return False
    if re.search(r"[/\\](Season\s*\d+|S\d{2}|Specials|Extras|Featurettes|OVA)[/\\]?", text, re.IGNORECASE):
        return True
    return "/" in text or "\\" in text


def parser_test_payload(filename: str):
    filename = filename.strip()
    if not filename:
        raise HTTPException(status_code=400, detail="文件名不能为空")

    cfg = get_config()
    matcher = _get_release_group_matcher()
    parser_kwargs = {
        "custom_words": cfg.parser.custom_words,
        "release_group_matcher": matcher,
    }
    if looks_like_file_path(filename):
        meta = MetaInfoPath(filename, **parser_kwargs)
    else:
        meta = MetaInfo(filename, isfile=True, **parser_kwargs)

    payload = serialize_meta(meta)
    tmdb_payload = None
    if cfg.tmdb.api_key:
        try:
            tmdb_client = TmdbClient(
                api_key=cfg.tmdb.api_key,
                language=cfg.tmdb.language,
                proxy=cfg.tmdb_proxy,
                timeout=cfg.tmdb.timeout,
                cache=get_tmdb_cache(),
            )
            tmdb_payload = serialize_tmdb_result(tmdb_client.recognize(meta))
            # 从 TMDB 结果回填 tmdbid 和 year
            if tmdb_payload:
                if tmdb_payload.get("tmdb_id") and not payload.get("tmdbid"):
                    payload["tmdbid"] = tmdb_payload["tmdb_id"]
                if tmdb_payload.get("year") and not payload.get("year"):
                    payload["year"] = tmdb_payload["year"]
        except Exception as exc:
            logger.warning("解析测试 TMDB 查询失败：%s", exc)

    payload["tmdb"] = tmdb_payload
    app_log(
        "parser",
        "parse_test",
        "执行了解析测试",
        details={
            "filename": filename,
            "name": payload.get("name"),
            "type": payload.get("type"),
            "tmdbMatched": bool(tmdb_payload),
        },
    )
    return payload


def _invalidate_runtime_cache() -> None:
    runtime._cfg = None
    runtime._client = None
    runtime._storage_provider = None
    runtime._u115_runtime.invalidate()
    runtime._config_mtime_ns = _get_config_signature()


def write_config_payload(data):
    payload = _validate_main_config_payload(dict(data or {}))
    parser_rules = _extract_parser_rules(payload)
    payload.pop("parser", None)

    _CONFIG_PATH.write_text(
        yaml.dump(payload, allow_unicode=True, default_flow_style=False, sort_keys=False, indent=2),
        encoding="utf-8",
    )
    _PARSER_RULES_PATH.write_text(
        yaml.dump(parser_rules, allow_unicode=True, default_flow_style=False, sort_keys=False, indent=2),
        encoding="utf-8",
    )
    _invalidate_runtime_cache()
    app_log("system", "config_updated", "配置文件已更新", level="SUCCESS")
    return {"ok": True, "message": "配置已保存"}


def write_main_config_payload(data):
    payload = _validate_main_config_payload(dict(data or {}))
    payload.pop("parser", None)
    _CONFIG_PATH.write_text(
        yaml.dump(payload, allow_unicode=True, default_flow_style=False, sort_keys=False, indent=2),
        encoding="utf-8",
    )
    _invalidate_runtime_cache()
    app_log("system", "main_config_updated", "主配置文件已更新", level="SUCCESS")
    return {"ok": True, "message": "主配置已保存"}


def write_parser_rules_payload(data):
    parser_rules = _extract_parser_rules({"parser": data or {}})
    _PARSER_RULES_PATH.write_text(
        yaml.dump(parser_rules, allow_unicode=True, default_flow_style=False, sort_keys=False, indent=2),
        encoding="utf-8",
    )
    _invalidate_runtime_cache()
    app_log("system", "parser_rules_updated", "文件名识别规则已更新", level="SUCCESS")
    return {"ok": True, "message": "识别规则已保存"}


def drive_oauth_status_payload():
    cfg = get_config().drive
    credentials_path = resolve_config_path(cfg.credentials_json)
    token_path = resolve_config_path(cfg.token_json)
    credentials_exists = runtime.os.path.exists(credentials_path)
    token_exists = runtime.os.path.exists(token_path)
    authorized = False
    token_valid = False
    token_expired = False
    token_refreshable = False
    if token_exists:
        try:
            from drive.auth import SCOPES as DRIVE_SCOPES
            creds = Credentials.from_authorized_user_file(token_path, DRIVE_SCOPES)
            token_valid = bool(creds.valid)
            token_expired = bool(creds.expired)
            token_refreshable = bool(creds.refresh_token)
            authorized = token_valid or token_refreshable
        except Exception:
            authorized = False
    return {
        "credentials_path": cfg.credentials_json,
        "token_path": cfg.token_json,
        "credentials_exists": credentials_exists,
        "token_exists": token_exists,
        "authorized": authorized,
        "token_valid": token_valid,
        "token_expired": token_expired,
        "token_refreshable": token_refreshable,
    }


def drive_test_connection_payload():
    try:
        runtime._client = None
        client = get_drive_client()
        about = client.about() or {}
        user = about.get("user") or {}
        quota = about.get("storageQuota") or {}
        return {
            "ok": True,
            "email": user.get("emailAddress") or "",
            "display_name": user.get("displayName") or "",
            "limit": quota.get("limit"),
            "usage": quota.get("usage"),
            "usage_in_drive": quota.get("usageInDrive"),
        }
    except Exception as exc:
        logger.exception("Google Drive 连接测试失败")
        raise HTTPException(status_code=400, detail=f"Drive 连接测试失败：{exc}") from exc

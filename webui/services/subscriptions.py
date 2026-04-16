from __future__ import annotations

import hashlib
import json
import threading
from dataclasses import asdict
from typing import Any
from urllib.parse import parse_qs, urlparse

from fastapi import HTTPException

from mediaparser import MetaInfo
from scraper.core.factory import SpiderFactory
from webui.core.app_logging import app_log
from webui.core.runtime import get_config, logger
from webui.rss_subscription_store import RSSSubscriptionStore, SubscriptionRecord
from webui.schemas.aria2 import Aria2AddUriBody
from webui.schemas.config import U115OfflineAddUrlsBody
from webui.services.aria2 import aria2_add_uri_payload
from webui.services.u115 import u115_oauth_status_payload, u115_offline_add_urls_sync


_store_lock = threading.Lock()
_store: RSSSubscriptionStore | None = None
_DB_PATH = "config/data/rss_subscriptions.db"
_DEFAULT_POLL_SECONDS = 300


def _resolve_store() -> RSSSubscriptionStore:
    global _store
    if _store is None:
        with _store_lock:
            if _store is None:
                from webui.core.runtime import _ROOT_DIR

                _store = RSSSubscriptionStore(f"{_ROOT_DIR}/{_DB_PATH}")
    return _store


def close_subscription_store() -> None:
    global _store
    with _store_lock:
        if _store is not None:
            _store.close()
            _store = None


def _normalize_keywords(keyword_all: list[str]) -> list[str]:
    seen: set[str] = set()
    normalized: list[str] = []
    for item in keyword_all:
        value = (item or "").strip().lower()
        if not value or value in seen:
            continue
        seen.add(value)
        normalized.append(value)
    return normalized


def _serialize_subscription(record: SubscriptionRecord, *, include_hits: bool = True) -> dict[str, Any]:
    store = _resolve_store()
    payload = asdict(record)
    payload["enabled"] = bool(record.enabled)
    payload["keyword_all"] = json.loads(record.keyword_all or "[]")
    payload["hit_count"] = store.count_hits(record.id)
    if include_hits:
        payload["recent_hits"] = [asdict(item) for item in store.list_hits(record.id, limit=5)]
    return payload


def _parse_mikan_rss_url(rss_url: str) -> tuple[str, str | None]:
    parsed = urlparse(rss_url)
    qs = parse_qs(parsed.query)
    media_id = (qs.get("bangumiId") or qs.get("bangumiid") or [None])[0]
    subgroup_id = (qs.get("subgroupid") or qs.get("subgroupId") or [None])[0]
    if not media_id:
        raise HTTPException(status_code=400, detail="RSS 链接缺少 bangumiId，暂不支持该格式")
    return str(media_id), str(subgroup_id) if subgroup_id else None


def _fetch_feed_items(site: str, rss_url: str) -> list[dict[str, Any]]:
    if site != "mikan":
        raise HTTPException(status_code=400, detail=f"暂不支持 RSS 站点：{site}")
    media_id, subgroup_id = _parse_mikan_rss_url(rss_url)
    spider = SpiderFactory.get_spider(site)
    episodes = spider.get_episodes(media_id, subgroup_id)
    return [episode.model_dump() for episode in episodes]


def _analyze_title(title: str) -> dict[str, Any]:
    cfg = get_config()
    meta = MetaInfo(title, custom_words=cfg.parser.custom_words)
    season_number = meta.begin_season if meta.begin_season is not None else 1
    episode_number = meta.begin_episode
    keyword_source = " ".join(
        part
        for part in [
            title,
            meta.name or "",
            meta.cn_name or "",
            meta.en_name or "",
            meta.resource_term or "",
            meta.release_group or "",
        ]
        if part
    ).lower()
    return {
        "parse_success": episode_number is not None,
        "name": meta.name or "",
        "season_number": season_number,
        "episode_number": episode_number,
        "keyword_source": keyword_source,
        "resource_team": meta.release_group or "",
        "resource_term": meta.resource_term or "",
    }


def _dedupe_key(item: dict[str, Any]) -> str:
    magnet_url = item.get("magnet_url") or ""
    torrent_url = item.get("torrent_url") or ""
    title = item.get("title") or ""
    publish_time = item.get("publish_time") or ""
    base = magnet_url or torrent_url or f"{title}|{publish_time}"
    return hashlib.sha1(base.encode("utf-8")).hexdigest()


def _filter_match_items(
    *,
    items: list[dict[str, Any]],
    season_number: int,
    start_episode: int,
    keyword_all: list[str],
) -> dict[str, Any]:
    normalized_keywords = _normalize_keywords(keyword_all)
    matches: list[dict[str, Any]] = []
    parsed_items = 0

    for item in items:
        analysis = _analyze_title(item.get("title") or "")
        if analysis["parse_success"]:
            parsed_items += 1
        episode_number = analysis["episode_number"]
        keyword_hits = [kw for kw in normalized_keywords if kw in analysis["keyword_source"]]
        all_keywords_hit = len(keyword_hits) == len(normalized_keywords)
        season_match = analysis["season_number"] == season_number
        episode_match = episode_number is not None and episode_number >= start_episode
        would_push = bool(analysis["parse_success"] and season_match and episode_match and all_keywords_hit)
        matches.append(
            {
                "title": item.get("title") or "",
                "season_number": analysis["season_number"],
                "episode_number": episode_number,
                "publish_time": item.get("publish_time"),
                "torrent_url": item.get("torrent_url"),
                "magnet_url": item.get("magnet_url"),
                "parse_success": analysis["parse_success"],
                "keyword_hits": keyword_hits,
                "all_keywords_hit": all_keywords_hit,
                "season_match": season_match,
                "episode_match": episode_match,
                "would_push": would_push,
                "dedupe_key": _dedupe_key(item),
            }
        )

    effective_matches = [item for item in matches if item["would_push"]]
    return {
        "summary": {
            "total_items": len(items),
            "parsed_items": parsed_items,
            "matched_items": len(effective_matches),
        },
        "matches": matches,
        "effective_matches": effective_matches,
    }


def test_subscription_payload(body) -> dict[str, Any]:
    items = _fetch_feed_items(body.site, body.rss_url)
    result = _filter_match_items(
        items=items,
        season_number=body.season_number,
        start_episode=body.start_episode,
        keyword_all=body.keyword_all,
    )
    return {
        "ok": True,
        "summary": result["summary"],
        "matches": result["matches"],
    }


def create_subscription_payload(body) -> dict[str, Any]:
    store = _resolve_store()
    payload = body.model_dump()
    payload["keyword_all"] = json.dumps(_normalize_keywords(body.keyword_all), ensure_ascii=False)
    record = store.create_subscription(payload)
    app_log(
        "subscription",
        "created",
        f"已创建 RSS 订阅：{record.name}",
        level="SUCCESS",
        details={"subscription_id": record.id, "site": record.site, "push_target": record.push_target},
    )
    return {"ok": True, "subscription": _serialize_subscription(record)}


def list_subscriptions_payload() -> dict[str, Any]:
    store = _resolve_store()
    return {"ok": True, "subscriptions": [_serialize_subscription(item) for item in store.list_subscriptions()]}


def get_subscription_payload(subscription_id: int) -> dict[str, Any]:
    store = _resolve_store()
    record = store.get_subscription(subscription_id)
    if record is None:
        raise HTTPException(status_code=404, detail="订阅不存在")
    return {"ok": True, "subscription": _serialize_subscription(record)}


def update_subscription_payload(subscription_id: int, body) -> dict[str, Any]:
    store = _resolve_store()
    payload = body.model_dump()
    payload["keyword_all"] = json.dumps(_normalize_keywords(body.keyword_all), ensure_ascii=False)
    record = store.update_subscription(subscription_id, payload)
    if record is None:
        raise HTTPException(status_code=404, detail="订阅不存在")
    app_log(
        "subscription",
        "updated",
        f"已更新 RSS 订阅：{record.name}",
        level="SUCCESS",
        details={"subscription_id": record.id},
    )
    return {"ok": True, "subscription": _serialize_subscription(record)}


def delete_subscription_payload(subscription_id: int) -> dict[str, Any]:
    store = _resolve_store()
    record = store.get_subscription(subscription_id)
    if record is None:
        raise HTTPException(status_code=404, detail="订阅不存在")
    store.delete_subscription(subscription_id)
    app_log(
        "subscription",
        "deleted",
        f"已删除 RSS 订阅：{record.name}",
        level="SUCCESS",
        details={"subscription_id": subscription_id},
    )
    return {"ok": True}


def _push_item(push_target: str, item: dict[str, Any]) -> None:
    url = item.get("magnet_url") or item.get("torrent_url")
    if not url:
        raise HTTPException(status_code=400, detail="RSS 条目缺少下载地址")
    if push_target == "aria2":
        aria2_add_uri_payload(Aria2AddUriBody(uris=[url], title=item.get("title")))
        return
    if push_target == "u115":
        auth = u115_oauth_status_payload()
        if not auth.get("authorized"):
            raise HTTPException(status_code=400, detail="115 未授权，无法推送云下载")
        u115_offline_add_urls_sync(U115OfflineAddUrlsBody(urls=url))
        return
    raise HTTPException(status_code=400, detail=f"未知推送目标：{push_target}")


def _check_subscription_record(record: SubscriptionRecord, *, manual: bool = False) -> dict[str, Any]:
    store = _resolve_store()
    items = _fetch_feed_items(record.site, record.rss_url)
    keyword_all = json.loads(record.keyword_all or "[]")
    result = _filter_match_items(
        items=items,
        season_number=record.season_number,
        start_episode=record.start_episode,
        keyword_all=keyword_all,
    )

    pushed = 0
    skipped = 0
    errors: list[str] = []

    for item in result["effective_matches"]:
        if store.hit_exists(record.id, item["dedupe_key"]):
            skipped += 1
            continue
        try:
            _push_item(record.push_target, item)
            store.record_hit(
                {
                    "subscription_id": record.id,
                    "episode_title": item["title"],
                    "season_number": item["season_number"],
                    "episode_number": item["episode_number"],
                    "torrent_url": item.get("torrent_url"),
                    "magnet_url": item.get("magnet_url"),
                    "published_at": item.get("publish_time"),
                    "dedupe_key": item["dedupe_key"],
                    "pushed_to": record.push_target,
                    "push_status": "success",
                    "push_error": None,
                }
            )
            pushed += 1
        except Exception as exc:
            logger.warning("RSS 订阅推送失败 | subscription=%s title=%s error=%s", record.id, item["title"], exc)
            store.record_hit(
                {
                    "subscription_id": record.id,
                    "episode_title": item["title"],
                    "season_number": item["season_number"],
                    "episode_number": item["episode_number"],
                    "torrent_url": item.get("torrent_url"),
                    "magnet_url": item.get("magnet_url"),
                    "published_at": item.get("publish_time"),
                    "dedupe_key": item["dedupe_key"],
                    "pushed_to": record.push_target,
                    "push_status": "failed",
                    "push_error": str(exc),
                }
            )
            errors.append(str(exc))

    checked_at = None
    if pushed > 0:
        store.mark_matched(record.id)
        checked_at = _resolve_store().get_subscription(record.id).last_matched_at if _resolve_store().get_subscription(record.id) else None
    store.mark_checked(record.id, error="; ".join(errors) if errors else None)

    if pushed > 0 or manual:
        app_log(
            "subscription",
            "checked",
            f"RSS 订阅检查完成：{record.name}",
            level="SUCCESS" if not errors else "WARNING",
            details={
                "subscription_id": record.id,
                "matched": result["summary"]["matched_items"],
                "pushed": pushed,
                "skipped": skipped,
                "errors": errors,
            },
        )

    return {
        "summary": result["summary"],
        "matched": result["summary"]["matched_items"],
        "pushed": pushed,
        "skipped": skipped,
        "errors": errors,
        "last_matched_at": checked_at,
    }


def check_subscription_payload(subscription_id: int) -> dict[str, Any]:
    store = _resolve_store()
    record = store.get_subscription(subscription_id)
    if record is None:
        raise HTTPException(status_code=404, detail="订阅不存在")
    result = _check_subscription_record(record, manual=True)
    refreshed = store.get_subscription(subscription_id)
    return {"ok": True, "result": result, "subscription": _serialize_subscription(refreshed)}  # type: ignore[arg-type]


def poll_subscriptions_once() -> dict[str, Any]:
    store = _resolve_store()
    records = store.list_enabled_subscriptions()
    checked = 0
    pushed = 0
    errors = 0
    for record in records:
        checked += 1
        try:
            result = _check_subscription_record(record)
            pushed += result["pushed"]
            if result["errors"]:
                errors += 1
        except Exception as exc:
            store.mark_checked(record.id, error=str(exc))
            errors += 1
            logger.warning("RSS 订阅轮询失败 | subscription=%s error=%s", record.id, exc)
    return {"checked": checked, "pushed": pushed, "errors": errors}


def subscriptions_status_payload() -> dict[str, Any]:
    store = _resolve_store()
    return {
        "enabled_count": len(store.list_enabled_subscriptions()),
        "total_count": len(store.list_subscriptions()),
        "poll_seconds": _DEFAULT_POLL_SECONDS,
    }

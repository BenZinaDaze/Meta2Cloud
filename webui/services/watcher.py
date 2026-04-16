import threading
from datetime import datetime, timezone
from typing import Any, Dict, Optional

import sqlite3
import u115pan

from webui.services.pipeline import schedule_pipeline
from webui.services.subscriptions import (
    close_subscription_store,
    get_subscription_poll_seconds,
    poll_subscriptions_once,
    subscriptions_status_payload,
)
from webui.services.u115 import u115_offline_client, u115_oauth_status_payload
import webui.core.runtime as runtime
from webui.core.app_logging import app_log
from webui.core.runtime import get_config, logger


_u115_auto_organize_thread: Optional[threading.Thread] = None
_u115_auto_organize_stop = threading.Event()
_u115_auto_organize_last_polled_at: Optional[str] = None
_u115_auto_organize_last_poll_error: Optional[str] = None
_u115_auto_organize_last_triggered_task: Optional[Dict[str, Any]] = None
_rss_subscription_thread: Optional[threading.Thread] = None
_rss_subscription_stop = threading.Event()
_rss_subscription_last_polled_at: Optional[str] = None
_rss_subscription_last_poll_error: Optional[str] = None
_rss_subscription_last_summary: Optional[Dict[str, Any]] = None
def _get_u115_auto_organize_store():
    if runtime._u115_auto_organize_store is None:
        runtime._u115_auto_organize_store = runtime.U115AutoOrganizeStore(runtime._U115_AUTO_ORGANIZE_DB)
    return runtime._u115_auto_organize_store


def _u115_task_key(task: u115pan.OfflineTask) -> str:
    if task.info_hash:
        return f"info_hash:{task.info_hash}"
    if task.file_id:
        return f"file_id:{task.file_id}"
    return f"name:{task.name}|add:{task.add_time or 0}|size:{task.size}"


def poll_u115_auto_organize_once() -> None:
    global _u115_auto_organize_last_polled_at, _u115_auto_organize_last_poll_error, _u115_auto_organize_last_triggered_task
    cfg = get_config()
    _u115_auto_organize_last_polled_at = datetime.now(timezone.utc).isoformat()
    _u115_auto_organize_last_poll_error = None
    if cfg.storage.primary != "pan115" or not cfg.u115.auto_organize_enabled or not cfg.u115.download_folder_id:
        return
    oauth_status = u115_oauth_status_payload()
    if not oauth_status.get("authorized"):
        return

    offline = u115_offline_client()
    task_page = offline.get_task_list(page=1)
    stable_seconds = max(0, cfg.u115.auto_organize_stable_seconds)
    now = datetime.now(timezone.utc)
    store = _get_u115_auto_organize_store()
    completed_to_trigger = []

    for task in task_page.tasks:
        if task.wp_path_id and str(task.wp_path_id) != str(cfg.u115.download_folder_id):
            continue
        state = store.upsert_observation(
            task_key=_u115_task_key(task),
            info_hash=task.info_hash,
            task_name=task.name,
            status=task.status,
            percent_done=task.percent_done,
            wp_path_id=task.wp_path_id,
            file_id=task.file_id,
            is_finished=task.is_finished,
        )
        if not task.is_finished or state.triggered_at or not state.finished_seen_at:
            continue
        finished_seen_at = datetime.fromisoformat(state.finished_seen_at)
        if (now - finished_seen_at).total_seconds() < stable_seconds:
            continue
        completed_to_trigger.append(task)

    if not completed_to_trigger:
        return

    debounce = cfg.telegram.debounce_seconds
    accepted = schedule_pipeline(debounce)
    if not accepted:
        app_log(
            "u115",
            "auto_organize_deferred",
            "检测到 115 云下载任务完成，但当前整理流程忙碌，等待后续轮询重试",
            level="WARNING",
            details={
                "count": len(completed_to_trigger),
                "tasks": [
                    {
                        "info_hash": task.info_hash,
                        "name": task.name,
                        "file_id": task.file_id,
                        "wp_path_id": task.wp_path_id,
                    }
                    for task in completed_to_trigger
                ],
            },
        )
        return

    for task in completed_to_trigger:
        store.mark_triggered(_u115_task_key(task))
    _u115_auto_organize_last_triggered_task = {
        "triggered_at": datetime.now(timezone.utc).isoformat(),
        "count": len(completed_to_trigger),
        "tasks": [
            {
                "info_hash": task.info_hash,
                "name": task.name,
                "file_id": task.file_id,
                "wp_path_id": task.wp_path_id,
            }
            for task in completed_to_trigger
        ],
    }
    app_log(
        "u115",
        "auto_organize_triggered",
        f"检测到 {len(completed_to_trigger)} 个 115 云下载任务完成，已触发自动整理",
        level="SUCCESS",
        details=_u115_auto_organize_last_triggered_task,
    )


def _u115_auto_organize_loop() -> None:
    global _u115_auto_organize_last_poll_error
    logger.info("115 自动整理监听线程已启动")
    while not _u115_auto_organize_stop.is_set():
        sleep_seconds = 45
        try:
            cfg = get_config()
            sleep_seconds = max(10, cfg.u115.auto_organize_poll_seconds)
            poll_u115_auto_organize_once()
        except u115pan.Pan115RateLimitError as exc:
            logger.warning("115 自动整理轮询进入冷却：%s", exc)
            _u115_auto_organize_last_poll_error = str(exc)
        except (sqlite3.Error, ValueError) as exc:
            logger.warning("115 自动整理状态处理失败：%s", exc)
            _u115_auto_organize_last_poll_error = str(exc)
        except Exception as exc:
            logger.warning("115 自动整理轮询失败：%s", exc)
            _u115_auto_organize_last_poll_error = str(exc)
            app_log(
                "u115",
                "auto_organize_poll_failed",
                "115 自动整理轮询失败",
                level="ERROR",
                details={"error": str(exc)},
            )
        _u115_auto_organize_stop.wait(timeout=sleep_seconds)
    logger.info("115 自动整理监听线程已停止")


def ensure_u115_auto_organize_thread() -> None:
    global _u115_auto_organize_thread
    if _u115_auto_organize_thread and _u115_auto_organize_thread.is_alive():
        return
    _u115_auto_organize_stop.clear()
    thread = threading.Thread(target=_u115_auto_organize_loop, name="u115-auto-organize", daemon=True)
    thread.start()
    _u115_auto_organize_thread = thread


def _rss_subscription_loop() -> None:
    global _rss_subscription_last_polled_at, _rss_subscription_last_poll_error, _rss_subscription_last_summary
    logger.info("RSS 订阅监听线程已启动")
    while not _rss_subscription_stop.is_set():
        sleep_seconds = get_subscription_poll_seconds()
        try:
            _rss_subscription_last_polled_at = datetime.now(timezone.utc).isoformat()
            _rss_subscription_last_poll_error = None
            _rss_subscription_last_summary = poll_subscriptions_once()
        except Exception as exc:
            _rss_subscription_last_poll_error = str(exc)
            logger.warning("RSS 订阅轮询失败：%s", exc)
            app_log(
                "subscription",
                "poll_failed",
                "RSS 订阅轮询失败",
                level="ERROR",
                details={"error": str(exc)},
            )
        _rss_subscription_stop.wait(timeout=sleep_seconds)
    logger.info("RSS 订阅监听线程已停止")


def ensure_rss_subscription_thread() -> None:
    global _rss_subscription_thread
    if _rss_subscription_thread and _rss_subscription_thread.is_alive():
        return
    _rss_subscription_stop.clear()
    thread = threading.Thread(target=_rss_subscription_loop, name="rss-subscription", daemon=True)
    thread.start()
    _rss_subscription_thread = thread


def u115_auto_organize_status_payload() -> Dict[str, Any]:
    cfg = get_config()
    store = _get_u115_auto_organize_store()
    last_triggered = store.get_last_triggered()
    last_triggered_payload = _u115_auto_organize_last_triggered_task
    if last_triggered and not last_triggered_payload:
        last_triggered_payload = {
            "triggered_at": last_triggered.triggered_at,
            "count": 1,
            "tasks": [
                {
                    "info_hash": last_triggered.info_hash,
                    "name": last_triggered.task_name,
                    "file_id": last_triggered.file_id,
                    "wp_path_id": last_triggered.wp_path_id,
                }
            ],
        }
    return {
        "enabled": bool(cfg.u115.auto_organize_enabled and cfg.storage.primary == "pan115"),
        "storage_primary": cfg.storage.primary,
        "authorized": bool(u115_oauth_status_payload().get("authorized")),
        "watcher_alive": bool(_u115_auto_organize_thread and _u115_auto_organize_thread.is_alive()),
        "poll_seconds": cfg.u115.auto_organize_poll_seconds,
        "stable_seconds": cfg.u115.auto_organize_stable_seconds,
        "download_folder_id": cfg.u115.download_folder_id,
        "last_polled_at": _u115_auto_organize_last_polled_at,
        "last_poll_error": _u115_auto_organize_last_poll_error,
        "last_triggered": last_triggered_payload,
    }


def rss_subscription_status_payload() -> Dict[str, Any]:
    summary = subscriptions_status_payload()
    summary.update(
        {
            "watcher_alive": bool(_rss_subscription_thread and _rss_subscription_thread.is_alive()),
            "last_polled_at": _rss_subscription_last_polled_at,
            "last_poll_error": _rss_subscription_last_poll_error,
            "last_summary": _rss_subscription_last_summary,
        }
    )
    return summary


async def startup_background_watchers():
    ensure_u115_auto_organize_thread()
    ensure_rss_subscription_thread()


async def shutdown_background_watchers():
    _u115_auto_organize_stop.set()
    _rss_subscription_stop.set()
    if runtime._u115_auto_organize_store is not None:
        runtime._u115_auto_organize_store.close()
    close_subscription_store()

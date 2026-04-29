import os
import sys
import threading
import uuid

from core.pipeline import Pipeline
from mediaparser import Config

from webui.core.app_logging import app_log
from webui.core.runtime import get_config, get_storage_provider, logger
from webui.library_store import get_library_store
from webui.services.library_data import (
    scan_movies,
    scan_movies_incremental,
    scan_tv_shows,
    scan_tv_shows_incremental,
)
from webui.services.telegram import send_telegram
from webui.websocket import get_broadcaster


_pl_lock = threading.Lock()
_debounce_timer: threading.Timer | None = None
_pipeline_running = False


def _do_refresh_library(incremental: bool = True) -> dict:
    client = get_storage_provider()
    cfg = get_config()
    store = get_library_store()
    app_log(
        "library",
        "refresh_scan_start",
        f"开始{'增量' if incremental else '全量'}刷新媒体库",
        details={
            "provider": getattr(client, "provider_name", "unknown"),
            "movie_root_id": cfg.active_movie_root_id(),
            "tv_root_id": cfg.active_tv_root_id(),
            "root_folder_id": cfg.active_root_folder_id(),
            "incremental": incremental,
        },
    )
    if incremental:
        stored_mtimes = store.get_all_folder_modified_times()
        movies, movie_mtimes = scan_movies_incremental(client, cfg, stored_mtimes)
        tv_shows, tv_mtimes = scan_tv_shows_incremental(client, cfg, stored_mtimes)
        all_mtimes = {**movie_mtimes, **tv_mtimes}
        diff = store.save_incremental(movies + tv_shows, all_mtimes)
        existing_ids = set(all_mtimes.keys())
        store.mark_missing_folders(existing_ids)
    else:
        movies = scan_movies(client, cfg)
        tv_shows = scan_tv_shows(client, cfg)
        diff = store.save_snapshot(movies, tv_shows)
    app_log(
        "library",
        "refresh_scan_finish",
        "媒体库刷新完成",
        level="SUCCESS",
        details=diff,
    )
    return diff


def _do_run_pipeline() -> None:
    global _pipeline_running, _debounce_timer
    cfg_obj = Config.load()
    tg_token = cfg_obj.telegram.bot_token
    tg_chat_id = cfg_obj.telegram.chat_id
    run_id = uuid.uuid4().hex[:12]

    with _pl_lock:
        _debounce_timer = None
        _pipeline_running = True

    app_log("pipeline", "pipeline_start", "整理流程启动", details={"runId": run_id})

    def log_callback(level: str, message: str) -> None:
        app_log(
            "pipeline",
            "pipeline_output",
            message,
            level=level,
            details={"runId": run_id},
        )
        try:
            get_broadcaster().broadcast_sync({
                "type": "log",
                "ts": None,
                "level": level,
                "message": message,
                "runId": run_id,
            })
        except Exception:
            pass

    try:
        # Ensure stdout uses UTF-8 for emoji output in thread context
        if hasattr(sys.stdout, "encoding") and (sys.stdout.encoding or "").lower() != "utf-8":
            try:
                sys.stdout.reconfigure(encoding="utf-8")
            except Exception:
                pass

        provider = get_storage_provider()
        pipe = Pipeline(
            client=provider,
            cfg=cfg_obj,
            log_callback=log_callback,
        )
        pipe.run()

        app_log(
            "pipeline",
            "pipeline_finish",
            "整理流程完成",
            level="SUCCESS",
            details={"runId": run_id},
        )
        try:
            _do_refresh_library()
        except Exception as exc:
            logger.warning("媒体库刷新异常：%s", exc)
            app_log(
                "pipeline",
                "pipeline_refresh_failed",
                "整理完成后刷新媒体库失败",
                level="ERROR",
                details={"runId": run_id, "error": str(exc)},
            )
    except Exception as exc:
        logger.error("Pipeline 异常：%s", exc)
        app_log(
            "pipeline",
            "pipeline_exception",
            "整理流程执行异常",
            level="ERROR",
            details={"runId": run_id, "error": str(exc)},
        )
        send_telegram(tg_token, tg_chat_id, f"❌ <b>Meta2Cloud</b>\n异常：<code>{exc}</code>")
    finally:
        with _pl_lock:
            _pipeline_running = False


def schedule_pipeline(debounce: int) -> bool:
    global _debounce_timer
    with _pl_lock:
        if debounce > 0:
            if _debounce_timer is not None:
                _debounce_timer.cancel()
                logger.info("整理流程防抖计时器已重置 | 防抖：%d 秒", debounce)
            else:
                app_log(
                    "pipeline",
                    "pipeline_schedule",
                    "整理流程已进入防抖等待",
                    details={"debounceSeconds": debounce},
                )
            timer = threading.Timer(debounce, _do_run_pipeline)
            timer.daemon = True
            timer.start()
            _debounce_timer = timer
            return True

        if _pipeline_running:
            app_log("pipeline", "pipeline_skip_running", "整理流程已在运行，跳过本次触发", level="WARNING")
            return False
        app_log("pipeline", "pipeline_schedule", "整理流程立即执行", details={"debounceSeconds": 0})
        threading.Thread(target=_do_run_pipeline, daemon=True).start()
        return True


def pipeline_status_payload() -> dict:
    return {"running": _pipeline_running, "debounce": _debounce_timer is not None}

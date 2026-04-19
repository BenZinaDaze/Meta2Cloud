import os
import re
import subprocess
import sys
import threading
import uuid

from mediaparser import Config

from webui.core.app_logging import app_log
from webui.core.runtime import _ROOT_DIR, get_config, get_storage_provider, logger
from webui.library_store import get_library_store
from webui.services.telegram import send_telegram
from webui.services.library_data import scan_movies, scan_tv_shows


def infer_pipeline_log_level(line: str) -> str:
    """根据 pipeline 输出行内容推断日志级别。"""
    text = line.lower()
    # 汇总行格式：✓ 成功：N    ⚠ 跳过：N    ✗ 失败：N
    # 根据实际数值判断级别：失败>0 → ERROR，跳过>0 → WARNING，否则 SUCCESS
    match = re.search(r"成功[：:]\s*(\d+).*跳过[：:]\s*(\d+).*失败[：:]\s*(\d+)", line)
    if match:
        ok, skipped, failed = int(match.group(1)), int(match.group(2)), int(match.group(3))
        if failed > 0:
            return "ERROR"
        if skipped > 0:
            return "WARNING"
        if ok > 0:
            return "SUCCESS"
        return "INFO"
    # 非汇总行：按标记判断
    if "✓" in line or "完成" in line or "已上传" in line or "移动：" in line:
        return "SUCCESS"
    if "⚠" in line or "warning" in text or "跳过" in line:
        return "WARNING"
    if "❌" in line or "失败" in line or "异常" in line or "error" in text:
        return "ERROR"
    return "INFO"


_pl_lock = threading.Lock()
_debounce_timer: threading.Timer | None = None
_pipeline_running = False


def _do_refresh_library() -> dict:
    client = get_storage_provider()
    cfg = get_config()
    app_log(
        "library",
        "refresh_scan_start",
        "开始刷新媒体库快照",
        details={
            "provider": getattr(client, "provider_name", "unknown"),
            "movie_root_id": cfg.active_movie_root_id(),
            "tv_root_id": cfg.active_tv_root_id(),
            "root_folder_id": cfg.active_root_folder_id(),
        },
    )
    movies = scan_movies(client, cfg)
    tv_shows = scan_tv_shows(client, cfg)
    diff = get_library_store().save_snapshot(movies, tv_shows)
    app_log(
        "library",
        "refresh_scan_finish",
        "媒体库快照刷新完成",
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
    try:
        env = os.environ.copy()
        env["PYTHONIOENCODING"] = "utf-8"
        process = subprocess.Popen(
            [sys.executable, "-m", "core"],
            cwd=_ROOT_DIR,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            encoding="utf-8",
            errors="replace",
            env=env,
        )
        assert process.stdout is not None
        for line in process.stdout:
            line = line.rstrip()
            if not line:
                continue
            app_log(
                "pipeline",
                "pipeline_output",
                line,
                level=infer_pipeline_log_level(line),
                details={"runId": run_id},
            )

        returncode = process.wait()
        if returncode == 0:
            app_log(
                "pipeline",
                "pipeline_finish",
                "整理流程完成",
                level="SUCCESS",
                details={"runId": run_id, "returncode": returncode},
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
        else:
            logger.error("Pipeline 退出码 %d", returncode)
            app_log(
                "pipeline",
                "pipeline_finish",
                "整理流程失败退出",
                level="ERROR",
                details={"runId": run_id, "returncode": returncode},
            )
            send_telegram(
                tg_token,
                tg_chat_id,
                f"❌ <b>Meta2Cloud</b>\n整理失败，退出码：<code>{returncode}</code>",
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

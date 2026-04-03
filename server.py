#!/usr/bin/env python3
"""
server.py —— Metadata2GD Webhook 触发服务

监听 HTTP POST /trigger，收到请求后在后台线程运行 pipeline.py。
容器内使用，供 aria2/rclone 容器在上传完成后调用。

用法：
    python server.py              # 监听 127.0.0.1:46562
    python server.py --port 9000  # 自定义端口
"""

import argparse
import json
import logging
import os
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import requests

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from mediaparser import Config

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)-8s %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger("webhook")

PIPELINE_DIR = os.path.dirname(os.path.abspath(__file__))


def send_telegram(token: str, chat_id: str, text: str) -> None:
    """发送 Telegram 消息，token/chat_id 为空时静默跳过。"""
    if not token or not chat_id:
        return
    try:
        requests.post(
            f"https://api.telegram.org/bot{token}/sendMessage",
            json={"chat_id": chat_id, "text": text, "parse_mode": "HTML"},
            timeout=10,
        )
    except Exception as e:
        logger.warning("Telegram 通知发送失败：%s", e)

# 防抖触发状态
_lock = threading.Lock()
_debounce_timer: threading.Timer | None = None   # 当前等待中的定时器
_pipeline_running = False                         # pipeline 子进程是否在运行


def _do_run_pipeline() -> None:
    """防抖超时后真正执行 pipeline（在独立线程中调用）。"""
    global _pipeline_running, _debounce_timer
    cfg = Config.load()
    tg_token = cfg.telegram.bot_token
    tg_chat_id = cfg.telegram.chat_id

    with _lock:
        _debounce_timer = None
        _pipeline_running = True

    logger.info("防抖超时，Pipeline 启动")
    try:
        result = subprocess.run(
            [sys.executable, "pipeline.py"],
            cwd=PIPELINE_DIR,
            capture_output=False,
        )
        if result.returncode == 0:
            logger.info("Pipeline 完成 ✓")
        else:
            logger.error("Pipeline 退出码 %d", result.returncode)
            send_telegram(tg_token, tg_chat_id,
                          f"❌ <b>Metadata2GD</b>\n整理失败，退出码：<code>{result.returncode}</code>")
    except Exception as e:
        logger.error("Pipeline 异常：%s", e)
        send_telegram(tg_token, tg_chat_id,
                      f"❌ <b>Metadata2GD</b>\n异常：<code>{e}</code>")
    finally:
        with _lock:
            _pipeline_running = False


def schedule_pipeline(debounce: int) -> None:
    """
    安排 pipeline 运行。

    debounce > 0：防抖模式 —— 重置/启动计时器，超时后才运行。
    debounce = 0：立即在新线程运行（与旧行为一致）。
    """
    global _debounce_timer

    with _lock:
        if debounce > 0:
            # 取消旧计时器（如果有），重置倒计时
            if _debounce_timer is not None:
                _debounce_timer.cancel()
                logger.info("防抖计时器已重置，重新等待 %d 秒...", debounce)
            else:
                logger.info("收到首次触发，%d 秒后运行 Pipeline...", debounce)
            t = threading.Timer(debounce, _do_run_pipeline)
            t.daemon = True
            t.start()
            _debounce_timer = t
        else:
            # 无防抖：立即运行（pipeline 运行期间忽略重复触发，与旧逻辑相同）
            if _pipeline_running:
                logger.info("Pipeline 正在运行，跳过本次触发")
                return
            threading.Thread(target=_do_run_pipeline, daemon=True).start()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):  # 屏蔽默认的 access log，用自己的
        pass

    def handle_error(self, request, client_address):
        pass  # 静默忽略 BrokenPipe / ConnectionReset 等客户端断开错误

    def do_GET(self):
        """健康检查"""
        if self.path == "/health":
            self._respond(200, {"status": "ok"})
        else:
            self._respond(404, {"error": "not found"})

    def do_POST(self):
        if self.path != "/trigger":
            self._respond(404, {"error": "not found"})
            return

        # 读取请求体（可选，记录 rclone 上传的路径信息）
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length) if length else b"{}"
        try:
            info = json.loads(body)
        except json.JSONDecodeError:
            info = {}

        cfg = Config.load()
        debounce = cfg.telegram.debounce_seconds
        logger.info("收到触发请求  path=%s  debounce=%ds", info.get("path", ""), debounce)
        schedule_pipeline(debounce)
        self._respond(200, {"status": "scheduled" if debounce > 0 else "triggered"})


    def _respond(self, code: int, body: dict) -> None:
        data = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main():
    parser = argparse.ArgumentParser(description="Metadata2GD Webhook Server")
    parser.add_argument("--port", type=int, default=46562)
    parser.add_argument("--host", default="127.0.0.1")
    args = parser.parse_args()

    server = HTTPServer((args.host, args.port), Handler)
    logger.info("Webhook 服务启动  http://%s:%d/trigger", args.host, args.port)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        logger.info("服务停止")


if __name__ == "__main__":
    main()

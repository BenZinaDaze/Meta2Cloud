#!/usr/bin/env python3
"""
server.py —— Metadata2GD Webhook 触发服务

监听 HTTP POST /trigger，收到请求后在后台线程运行 pipeline.py。
容器内使用，供 aria2/rclone 容器在上传完成后调用。

用法：
    python server.py              # 监听 0.0.0.0:18888
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

# 防止并发重复触发
_lock = threading.Lock()
_running = False   # pipeline 是否正在运行
_pending = False   # 运行期间是否收到新触发（运行结束后立即补跑）


def run_pipeline(extra_info: dict) -> None:
    global _running, _pending
    cfg = Config.load()
    tg_token = cfg.telegram.bot_token
    tg_chat_id = cfg.telegram.chat_id
    while True:
        logger.info("Pipeline 启动  info=%s", extra_info)
        try:
            result = subprocess.run(
                [sys.executable, "pipeline.py"],
                cwd=PIPELINE_DIR,
                capture_output=False,
            )
            if result.returncode == 0:
                logger.info("Pipeline 完成 ✓")
                # 详细入库通知由 pipeline.py 内部发出，此处不重复
            else:
                logger.error("Pipeline 退出码 %d", result.returncode)
                send_telegram(tg_token, tg_chat_id, f"❌ <b>Metadata2GD</b>\n整理失败，退出码：<code>{result.returncode}</code>")
        except Exception as e:
            logger.error("Pipeline 异常：%s", e)
            send_telegram(tg_token, tg_chat_id, f"❌ <b>Metadata2GD</b>\n异常：<code>{e}</code>")

        # 检查运行期间是否有新触发
        with _lock:
            if _pending:
                _pending = False
                logger.info("检测到待执行触发，立即补跑 Pipeline...")
                extra_info = {}
                continue   # 继续循环，再跑一次
            _running = False
            break



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
        global _running
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

        with _lock:
            if _running:
                _pending = True
                logger.info("Pipeline 正在运行，已标记为待执行（不会遗漏）")
                self._respond(202, {"status": "queued"})
                return
            _running = True

        logger.info("收到触发请求  path=%s", info.get("path", ""))
        threading.Thread(target=run_pipeline, args=(info,), daemon=True).start()
        self._respond(200, {"status": "triggered"})

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
    parser.add_argument("--host", default="0.0.0.0")
    args = parser.parse_args()

    server = HTTPServer((args.host, args.port), Handler)
    logger.info("Webhook 服务启动  http://%s:%d/trigger", args.host, args.port)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        logger.info("服务停止")


if __name__ == "__main__":
    main()

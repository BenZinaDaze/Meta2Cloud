#!/usr/bin/env python3
"""
独立测试 115 扫码授权流程。

用途：
  1. 创建扫码会话，输出二维码 URL
  2. 在控制台显示二维码
  3. 轮询扫码状态
  4. 用户确认后换取 token 并写入本地 JSON
  5. 查看本地 token 状态并验证远端授权

示例：
  python test/test_u115pan_auth.py create --client-id 100197847
  python test/test_u115pan_auth.py status
  python test/test_u115pan_auth.py exchange --token-path config/115-token.json
  python test/test_u115pan_auth.py login --client-id 100197847 --token-path config/115-token.json
  python test/test_u115pan_auth.py whoami --token-path config/115-token.json
"""

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path
from datetime import datetime, timezone

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

import u115pan


DEFAULT_CLIENT_ID = "100197847"
DEFAULT_TOKEN_PATH = "config/115-token.json"
DEFAULT_SESSION_PATH = "config/115-device-session.json"


def _session_path(path: str) -> Path:
    p = Path(path)
    if not p.is_absolute():
        p = ROOT / p
    return p


def save_device_session(session: u115pan.DeviceCodeSession, path: str) -> Path:
    session_file = _session_path(path)
    session_file.parent.mkdir(parents=True, exist_ok=True)
    with session_file.open("w", encoding="utf-8") as fh:
        json.dump(
            {
                "qrcode": session.qrcode,
                "uid": session.uid,
                "time_value": session.time_value,
                "sign": session.sign,
                "code_verifier": session.code_verifier,
            },
            fh,
            ensure_ascii=False,
            indent=2,
        )
    return session_file


def load_device_session(path: str) -> u115pan.DeviceCodeSession:
    session_file = _session_path(path)
    if not session_file.exists():
        raise FileNotFoundError(f"扫码会话文件不存在：{session_file}")
    with session_file.open("r", encoding="utf-8") as fh:
        data = json.load(fh)
    return u115pan.DeviceCodeSession(
        qrcode=str(data["qrcode"]),
        uid=str(data["uid"]),
        time_value=str(data["time_value"]),
        sign=str(data["sign"]),
        code_verifier=str(data["code_verifier"]),
    )


def build_client(client_id: str, token_path: str) -> u115pan.Pan115Client:
    return u115pan.Pan115Client.from_token_file(
        client_id=client_id,
        token_path=str(_session_path(token_path)),
    )


def _format_ts(ts: int | float | None) -> str:
    if not ts:
        return "-"
    dt = datetime.fromtimestamp(ts, tz=timezone.utc).astimezone()
    return dt.strftime("%Y-%m-%d %H:%M:%S %Z")


def print_terminal_qr(text: str) -> None:
    """
    用 qrencode 在终端显示二维码。

    若系统未安装 qrencode，则降级为只输出二维码 URL。
    """
    if not shutil.which("qrencode"):
        print("未找到 qrencode，无法在控制台显示二维码。")
        print(f"请手动打开二维码 URL: {text}")
        return

    try:
        subprocess.run(
            ["qrencode", "-t", "ANSIUTF8", text],
            check=True,
        )
    except subprocess.CalledProcessError:
        print("终端二维码渲染失败，请手动打开下面的二维码 URL：")
        print(text)


def cmd_create(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    session = client.create_device_code()
    session_file = save_device_session(session, args.session_path)

    print("已创建 115 扫码会话")
    print("终端二维码：")
    print_terminal_qr(session.qrcode)
    print(f"二维码 URL: {session.qrcode}")
    print(f"uid: {session.uid}")
    print(f"会话文件: {session_file}")
    print("下一步：")
    print(f"  1. 用 115 App 扫码并确认")
    print(f"  2. 运行: python test/test_u115pan_auth.py status --session-path {args.session_path}")
    print(f"  3. 或直接运行: python test/test_u115pan_auth.py login --client-id {args.client_id}")
    return 0


def cmd_status(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    session = load_device_session(args.session_path)
    status = client.get_qrcode_status(session)

    print(f"status={status.status}")
    print(f"message={status.message}")
    if status.confirmed:
        print("用户已确认授权，可以执行 exchange")
    return 0


def cmd_exchange(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    session = load_device_session(args.session_path)
    token = client.exchange_device_token(session)

    print("授权成功，token 已写入本地")
    print(f"token_path={_session_path(args.token_path)}")
    print(f"expires_in={token.expires_in}")
    print(f"refresh_time={token.refresh_time}")
    return 0


def cmd_login(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    session = client.create_device_code()
    session_file = save_device_session(session, args.session_path)

    print("请使用 115 App 扫码并确认")
    print("终端二维码：")
    print_terminal_qr(session.qrcode)
    print(f"二维码 URL: {session.qrcode}")
    print(f"会话文件: {session_file}")
    print(f"开始轮询，间隔 {args.interval} 秒，超时 {args.timeout} 秒")

    deadline = time.time() + args.timeout
    while time.time() < deadline:
        status = client.get_qrcode_status(session)
        print(f"[status] code={status.status} message={status.message}")
        if status.confirmed:
            token = client.exchange_device_token(session)
            print("授权成功，token 已写入本地")
            print(f"token_path={_session_path(args.token_path)}")
            print(f"expires_in={token.expires_in}")
            return 0
        time.sleep(args.interval)

    print("等待扫码确认超时")
    return 1


def cmd_whoami(args: argparse.Namespace) -> int:
    token_file = _session_path(args.token_path)
    token = u115pan.load_token(str(token_file))
    if not token:
        print(f"未找到 token 文件: {token_file}")
        return 1

    client = build_client(args.client_id, args.token_path)
    expires_at = token.refresh_time + int(token.expires_in)
    remaining = max(0, expires_at - int(time.time()))

    print("本地 token 信息")
    print(f"token_path={token_file}")
    print(f"refresh_time={_format_ts(token.refresh_time)}")
    print(f"expires_in={token.expires_in}")
    print(f"expires_at={_format_ts(expires_at)}")
    print(f"remaining_seconds={remaining}")
    print(f"access_token_prefix={token.access_token[:12]}...")
    print(f"refresh_token_prefix={token.refresh_token[:12]}...")

    try:
        space = client.get_space_info()
        print("远端验证结果")
        print("auth_ok=true")
        print(f"total_space={space.total_size}")
        print(f"remain_space={space.remain_size}")
    except Exception as exc:
        print("远端验证结果")
        print("auth_ok=false")
        print(f"error={exc}")
        return 1

    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="115 扫码授权测试脚本")
    subparsers = parser.add_subparsers(dest="command", required=True)

    def add_common_flags(p: argparse.ArgumentParser) -> None:
        p.add_argument("--client-id", default=DEFAULT_CLIENT_ID, help="115 开放平台 client_id")
        p.add_argument("--token-path", default=DEFAULT_TOKEN_PATH, help="token JSON 保存路径")
        p.add_argument("--session-path", default=DEFAULT_SESSION_PATH, help="扫码会话 JSON 保存路径")

    create = subparsers.add_parser("create", help="创建扫码会话并输出二维码 URL")
    add_common_flags(create)
    create.set_defaults(func=cmd_create)

    status = subparsers.add_parser("status", help="查询当前扫码状态")
    add_common_flags(status)
    status.set_defaults(func=cmd_status)

    exchange = subparsers.add_parser("exchange", help="使用已确认的扫码会话换取 token")
    add_common_flags(exchange)
    exchange.set_defaults(func=cmd_exchange)

    login = subparsers.add_parser("login", help="一条命令完成创建二维码、轮询、换 token")
    add_common_flags(login)
    login.add_argument("--interval", type=int, default=3, help="轮询间隔秒数")
    login.add_argument("--timeout", type=int, default=300, help="等待扫码总超时秒数")
    login.set_defaults(func=cmd_login)

    whoami = subparsers.add_parser("whoami", help="显示本地 token 信息并验证授权是否可用")
    add_common_flags(whoami)
    whoami.set_defaults(func=cmd_whoami)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return int(args.func(args))


if __name__ == "__main__":
    raise SystemExit(main())

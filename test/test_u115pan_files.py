#!/usr/bin/env python3
"""
独立测试 115 文件列表相关接口。

用途：
  1. 列出指定目录下的文件和目录
  2. 自动翻页列出整个目录
  3. 按完整路径查询文件或目录详情
  4. 搜索文件或目录

示例：
  python test/test_u115pan_files.py list --cid 0
  python test/test_u115pan_files.py list-all --cid 0
  python test/test_u115pan_files.py info --path /电影
  python test/test_u115pan_files.py search --keyword 星际穿越
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

import u115pan


DEFAULT_CLIENT_ID = "100197847"
DEFAULT_TOKEN_PATH = "config/115-token.json"


def _abs_path(path: str) -> Path:
    p = Path(path)
    if not p.is_absolute():
        p = ROOT / p
    return p


def build_client(client_id: str, token_path: str) -> u115pan.Pan115Client:
    return u115pan.Pan115Client.from_token_file(
        client_id=client_id,
        token_path=str(_abs_path(token_path)),
    )


def _print_file(item: u115pan.Pan115File) -> None:
    kind = "DIR " if item.is_folder else "FILE"
    print(
        f"[{kind}] "
        f"id={item.id} "
        f"name={item.name} "
        f"size={item.size} "
        f"pick_code={item.pick_code} "
        f"mtime={item.modified_time}"
    )


def cmd_list(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    items = client.list_files(cid=args.cid, limit=args.limit, offset=args.offset)

    print(f"cid={args.cid} count={len(items)} offset={args.offset} limit={args.limit}")
    for item in items:
        _print_file(item)
    return 0


def cmd_list_all(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    items = client.list_all_files(cid=args.cid, limit=args.limit)

    print(f"cid={args.cid} total={len(items)}")
    for item in items:
        _print_file(item)
    return 0


def cmd_info(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    item = client.get_path_info(args.path)
    if item is None:
        print(f"path not found: {args.path}")
        return 1

    print(f"path={args.path}")
    _print_file(item)
    return 0


def cmd_search(args: argparse.Namespace) -> int:
    client = build_client(args.client_id, args.token_path)
    items = client.search(
        args.keyword,
        limit=args.limit,
        offset=args.offset,
        cid=args.cid,
        fc=args.fc,
        file_type=args.file_type,
        suffix=args.suffix,
    )

    print(
        f"keyword={args.keyword} count={len(items)} "
        f"offset={args.offset} limit={args.limit}"
    )
    for item in items:
        _print_file(item)
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="115 文件列表测试脚本")
    subparsers = parser.add_subparsers(dest="command", required=True)

    def add_common_flags(p: argparse.ArgumentParser) -> None:
        p.add_argument("--client-id", default=DEFAULT_CLIENT_ID, help="115 开放平台 client_id")
        p.add_argument("--token-path", default=DEFAULT_TOKEN_PATH, help="token JSON 路径")

    list_cmd = subparsers.add_parser("list", help="列出单页目录内容")
    add_common_flags(list_cmd)
    list_cmd.add_argument("--cid", default="0", help="目录 ID，根目录传 0")
    list_cmd.add_argument("--limit", type=int, default=1000, help="单页数量")
    list_cmd.add_argument("--offset", type=int, default=0, help="分页偏移")
    list_cmd.set_defaults(func=cmd_list)

    list_all_cmd = subparsers.add_parser("list-all", help="自动翻页列出整个目录")
    add_common_flags(list_all_cmd)
    list_all_cmd.add_argument("--cid", default="0", help="目录 ID，根目录传 0")
    list_all_cmd.add_argument("--limit", type=int, default=1000, help="每页数量")
    list_all_cmd.set_defaults(func=cmd_list_all)

    info_cmd = subparsers.add_parser("info", help="按完整路径查询文件或目录详情")
    add_common_flags(info_cmd)
    info_cmd.add_argument("--path", required=True, help="完整路径，例如 /电影/星际穿越.mkv")
    info_cmd.set_defaults(func=cmd_info)

    search_cmd = subparsers.add_parser("search", help="搜索文件或目录")
    add_common_flags(search_cmd)
    search_cmd.add_argument("--keyword", required=True, help="搜索关键字")
    search_cmd.add_argument("--limit", type=int, default=20, help="单页数量")
    search_cmd.add_argument("--offset", type=int, default=0, help="分页偏移")
    search_cmd.add_argument("--cid", default=None, help="限定目录 ID")
    search_cmd.add_argument("--fc", type=int, default=None, help="1 只文件夹，2 只文件")
    search_cmd.add_argument("--file-type", type=int, default=None, help="一级分类：4 表示视频")
    search_cmd.add_argument("--suffix", default=None, help="后缀筛选，例如 mkv")
    search_cmd.set_defaults(func=cmd_search)

    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()
    return int(args.func(args))


if __name__ == "__main__":
    raise SystemExit(main())

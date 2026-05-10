#!/usr/bin/env python3
"""
删除 115 远程剧集目录中的 NFO 文件。

默认行为：
  - 递归扫描指定远程目录
  - 只匹配“单集同名 .nfo”
  - 保留 tvshow.nfo / season.nfo
  - 只预览，不实际删除

示例：
  uv run python scripts/delete_u115_episode_nfo.py --path "/剧集/绝命毒师 (2008)"
  uv run python scripts/delete_u115_episode_nfo.py --path "/剧集/绝命毒师 (2008)" --yes
  uv run python scripts/delete_u115_episode_nfo.py --path "/剧集/绝命毒师 (2008)" --all-nfo --yes
  uv run python scripts/delete_u115_episode_nfo.py --tv-root
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from mediaparser import Config
from u115pan.client import Pan115Client
from u115pan.errors import Pan115ApiError
from u115pan.models import Pan115File

RESERVED_NFO_NAMES = {"tvshow.nfo", "season.nfo"}


def resolve_local_path(path: str) -> Path:
    candidate = Path(path)
    if not candidate.is_absolute():
        candidate = ROOT / candidate
    return candidate


def build_client(cfg: Config) -> Pan115Client:
    token_path = resolve_local_path(cfg.u115.token_json)
    return Pan115Client.from_token_file(
        client_id=cfg.u115.client_id,
        token_path=str(token_path),
    )


def iter_files(
    client: Pan115Client,
    folder_id: str,
    folder_path: str,
):
    for item in client.list_all_files(cid=folder_id, limit=1000):
        item_path = join_remote_path(folder_path, item.name)
        yield item, item_path
        if item.is_folder:
            yield from iter_files(client, item.id, item_path)


def join_remote_path(parent: str, name: str) -> str:
    if parent == "/":
        return f"/{name}"
    return f"{parent.rstrip('/')}/{name}"


def should_delete_nfo(name: str, include_all_nfo: bool) -> bool:
    lower_name = name.lower()
    if not lower_name.endswith(".nfo"):
        return False
    if include_all_nfo:
        return True
    return lower_name not in RESERVED_NFO_NAMES


def scan_targets(
    client: Pan115Client,
    root: Pan115File,
    root_path: str,
    *,
    include_all_nfo: bool,
) -> list[tuple[Pan115File, str]]:
    if not root.is_folder:
        raise Pan115ApiError(f"远程路径不是目录：{root_path}")

    matches: list[tuple[Pan115File, str]] = []
    for item, item_path in iter_files(client, root.id, root_path):
        if item.is_folder:
            continue
        if should_delete_nfo(item.name, include_all_nfo):
            matches.append((item, item_path))
    return matches


def resolve_scan_root(client: Pan115Client, cfg: Config, args: argparse.Namespace) -> tuple[Pan115File, str]:
    if args.tv_root:
        tv_root_id = (cfg.u115.tv_root_id or "").strip()
        if not tv_root_id:
            raise Pan115ApiError("配置中未设置 u115.tv_root_id")
        return Pan115File(id=tv_root_id, name="tv_root", category="0"), f"/[tv_root_id:{tv_root_id}]"

    remote_path = (args.path or "").strip()
    if not remote_path:
        raise Pan115ApiError("必须提供 --path 或 --tv-root")
    if not remote_path.startswith("/"):
        raise Pan115ApiError("远程路径必须以 / 开头")

    root = client.get_path_info(remote_path)
    if root is None:
        raise Pan115ApiError(f"远程路径不存在：{remote_path}")
    return root, remote_path


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="删除 115 远程剧集目录中的 NFO 文件")
    parser.add_argument(
        "--path",
        default=None,
        help="115 远程目录绝对路径，例如 /剧集/绝命毒师 (2008)",
    )
    parser.add_argument(
        "--tv-root",
        action="store_true",
        help="直接使用配置中的 u115.tv_root_id 作为扫描根目录",
    )
    parser.add_argument(
        "--config",
        default=None,
        help="配置文件路径，默认自动查找 config/config.yaml",
    )
    parser.add_argument(
        "--all-nfo",
        action="store_true",
        help="连 tvshow.nfo 和 season.nfo 也一起删除",
    )
    parser.add_argument(
        "--yes",
        action="store_true",
        help="确认执行删除；不传时只预览",
    )
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    cfg = Config.load(args.config)
    client = build_client(cfg)

    try:
        root, root_path = resolve_scan_root(client, cfg, args)
        targets = scan_targets(
            client,
            root,
            root_path,
            include_all_nfo=args.all_nfo,
        )
    except Exception as exc:
        print(f"扫描失败：{exc}")
        return 1

    if not targets:
        print("未找到符合条件的 NFO 文件")
        return 0

    mode = "全部 NFO" if args.all_nfo else "单集 NFO（保留 tvshow.nfo / season.nfo）"
    print(f"扫描目录：{root_path}")
    print(f"匹配模式：{mode}")
    print(f"命中数量：{len(targets)}")
    for _, item_path in targets:
        print(item_path)

    if not args.yes:
        print("")
        print("当前为预览模式，未执行删除。")
        print("确认无误后，加 --yes 真正删除。")
        return 0

    deleted = 0
    failed = 0
    for item, item_path in targets:
        try:
            client.delete(item.id)
            print(f"已删除：{item_path}")
            deleted += 1
        except Exception as exc:
            print(f"删除失败：{item_path} -> {exc}")
            failed += 1

    print("")
    print(f"完成：删除 {deleted} 个，失败 {failed} 个")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())

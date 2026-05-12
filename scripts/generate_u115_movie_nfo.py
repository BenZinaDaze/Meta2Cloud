#!/usr/bin/env python3
"""
为 115 电影目录批量生成同名 NFO 文件。

默认行为：
  - 递归扫描指定 115 远程目录
  - 只处理视频文件
  - 使用文件名 + 父目录名解析影片信息，并查询 TMDB
  - 仅当识别结果为电影时生成同名 .nfo
  - 默认只预览，不实际上传

示例：
  uv run python scripts/generate_u115_movie_nfo.py --movie-root
  uv run python scripts/generate_u115_movie_nfo.py --path "/电影/星际穿越 (2014)"
  uv run python scripts/generate_u115_movie_nfo.py --movie-root --yes
  uv run python scripts/generate_u115_movie_nfo.py --movie-root --yes --overwrite
"""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from mediaparser import Config, MediaType, MetaInfoPath, TmdbClient
from mediaparser.meta_base import MEDIA_EXTS
from mediaparser.release_group import ReleaseGroupsMatcher
from nfo import NfoGenerator
from storage.pan115 import Pan115Provider
from u115pan.client import Pan115Client
from u115pan.errors import Pan115ApiError
from u115pan.models import Pan115File


@dataclass
class VideoEntry:
    file: Pan115File
    path: str


def resolve_local_path(path: str) -> Path:
    candidate = Path(path)
    if not candidate.is_absolute():
        candidate = ROOT / candidate
    return candidate


def build_client(cfg: Config, api_qps: float) -> Pan115Client:
    token_path = resolve_local_path(cfg.u115.token_json)
    client = Pan115Client.from_token_file(
        client_id=cfg.u115.client_id,
        token_path=str(token_path),
        api_qps=api_qps,
    )
    if cfg.u115.cookie.strip():
        client.set_cookie(cfg.u115.cookie)
    return client


def build_provider(client: Pan115Client) -> Pan115Provider:
    return Pan115Provider(client)


def join_remote_path(parent: str, name: str) -> str:
    if parent == "/":
        return f"/{name}"
    return f"{parent.rstrip('/')}/{name}"


def is_video_file(item: Pan115File) -> bool:
    return item.is_file and item.extension in MEDIA_EXTS


def iter_video_files(
    client: Pan115Client,
    folder_id: str,
    folder_path: str,
):
    for item in client.list_all_files(cid=folder_id, limit=1000):
        item.parent_id = folder_id
        item_path = join_remote_path(folder_path, item.name)
        if item.is_folder:
            yield from iter_video_files(client, item.id, item_path)
            continue
        if is_video_file(item):
            yield VideoEntry(file=item, path=item_path)


def resolve_scan_root(
    client: Pan115Client,
    cfg: Config,
    args: argparse.Namespace,
) -> tuple[Pan115File, str]:
    if args.movie_root:
        movie_root_id = (cfg.u115.movie_root_id or cfg.u115.root_folder_id or "").strip()
        if not movie_root_id:
            raise Pan115ApiError("配置中未设置 u115.movie_root_id 或 u115.root_folder_id")
        return Pan115File(id=movie_root_id, name="movie_root", category="0"), "/"

    remote_path = (args.path or "").strip()
    if not remote_path:
        raise Pan115ApiError("必须提供 --path 或 --movie-root")
    if not remote_path.startswith("/"):
        raise Pan115ApiError("远程路径必须以 / 开头")

    root = client.get_path_info(remote_path)
    if root is None:
        raise Pan115ApiError(f"远程路径不存在：{remote_path}")
    return root, remote_path


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="为 115 电影目录批量生成同名 NFO 文件")
    parser.add_argument(
        "--path",
        default=None,
        help="115 远程目录绝对路径，例如 /电影/星际穿越 (2014)",
    )
    parser.add_argument(
        "--movie-root",
        action="store_true",
        help="直接使用配置中的 u115.movie_root_id 作为扫描根目录",
    )
    parser.add_argument(
        "--config",
        default=None,
        help="配置文件路径，默认自动查找 config/config.yaml",
    )
    parser.add_argument(
        "--qps",
        type=float,
        default=3.0,
        help="115 API 请求速率限制，默认 3",
    )
    parser.add_argument(
        "--timeout",
        type=int,
        default=None,
        help="TMDB 请求超时秒数，默认使用配置文件中的 tmdb.timeout",
    )
    parser.add_argument(
        "--limit",
        type=int,
        default=0,
        help="最多处理多少个视频，0 表示不限制",
    )
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="覆盖已存在的同名 NFO",
    )
    parser.add_argument(
        "--force-movie",
        action="store_true",
        help="当解析器未识别出类型时，强制按电影搜索",
    )
    parser.add_argument(
        "--yes",
        action="store_true",
        help="确认执行上传；不传时只预览",
    )
    return parser


def main() -> int:
    parser = build_parser()
    args = parser.parse_args()

    cfg = Config.load(args.config)
    if not cfg.is_tmdb_ready():
        print("配置错误：tmdb.api_key 未设置")
        return 1

    client = build_client(cfg, api_qps=max(0.1, args.qps))
    provider = build_provider(client)
    tmdb = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=args.timeout or cfg.tmdb.timeout,
    )
    nfo_gen = NfoGenerator(tmdb_image_base_url=cfg.tmdb_image_base_url)
    rg_matcher = ReleaseGroupsMatcher(custom_groups=cfg.parser.custom_release_groups or None)

    try:
        root, root_path = resolve_scan_root(client, cfg, args)
        if not root.is_folder:
            raise Pan115ApiError(f"远程路径不是目录：{root_path}")
        videos = list(iter_video_files(client, root.id, root_path))
    except Exception as exc:
        print(f"扫描失败：{exc}")
        return 1

    if args.limit > 0:
        videos = videos[: args.limit]

    if not videos:
        print("未找到视频文件")
        return 0

    print(f"扫描目录：{root_path}")
    print(f"视频数量：{len(videos)}")
    print(f"执行模式：{'上传' if args.yes else '预览'}")
    print("")

    ok_count = 0
    skipped_count = 0
    failed_count = 0

    for index, entry in enumerate(videos, 1):
        file = entry.file
        nfo_name = nfo_gen.nfo_name_for(file.name)
        print(f"[{index}/{len(videos)}] {entry.path}")

        try:
            existing = provider.find_file(nfo_name, folder_id=file.parent_id)
            if existing and not args.overwrite:
                print(f"  跳过：已存在 {nfo_name}")
                skipped_count += 1
                continue

            meta = MetaInfoPath(
                entry.path,
                custom_words=cfg.parser.custom_words,
                release_group_matcher=rg_matcher,
            )
            if args.force_movie and meta.type == MediaType.UNKNOWN:
                meta.type = MediaType.MOVIE

            tmdb_info = tmdb.recognize(meta)
            if not tmdb_info:
                print("  跳过：TMDB 未识别到匹配结果")
                skipped_count += 1
                continue

            if tmdb_info.get("media_type") != MediaType.MOVIE:
                media_type = tmdb_info.get("media_type")
                print(f"  跳过：识别结果不是电影，而是 {media_type}")
                skipped_count += 1
                continue

            title = tmdb_info.get("title") or tmdb_info.get("name") or ""
            year = (tmdb_info.get("release_date") or "")[:4]
            print(f"  识别：{title} ({year or '未知年份'})")

            if not args.yes:
                print(f"  预览：将上传 {nfo_name}")
                ok_count += 1
                continue

            xml = nfo_gen.generate(tmdb_info, media_type=MediaType.MOVIE)
            provider.upload_text(
                xml,
                nfo_name,
                parent_id=file.parent_id,
                mime_type="text/xml",
                overwrite=args.overwrite,
            )
            print(f"  已上传：{nfo_name}")
            ok_count += 1
        except Exception as exc:
            print(f"  失败：{exc}")
            failed_count += 1

    print("")
    print(f"完成：成功 {ok_count}，跳过 {skipped_count}，失败 {failed_count}")
    if not args.yes:
        print("当前为预览模式，确认无误后加 --yes 真正上传。")
    return 0 if failed_count == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())

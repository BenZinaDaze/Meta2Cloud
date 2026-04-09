#!/usr/bin/env python3
"""
backfill_nfo.py —— 为已整理好的电影/剧集文件夹批量补传 NFO

使用场景：
  - pipeline.py 运行前文件夹已存在，缺少 NFO
  - Infuse / Plex 无法识别媒体，显示在「其他」
  - 需要批量更新某个根目录下所有媒体的元数据

处理逻辑：
  TV   → 扫描剧名子文件夹 → TMDB → 上传 tvshow.nfo 到剧名文件夹根目录
         （Infuse 依赖 tvshow.nfo 识别电视剧分类）
  Movie → 扫描电影子文件夹 → 找其中的视频文件 → TMDB → 上传与视频同名的 .nfo
          （Plex / Infuse 标准：video.mkv 对应 video.nfo）

用法：
  python backfill_nfo.py                        # 同时处理电影和剧集（默认）
  python backfill_nfo.py --mode tv              # 只处理剧集
  python backfill_nfo.py --mode movie           # 只处理电影
  python backfill_nfo.py --dry-run              # 只查询，不上传
  python backfill_nfo.py --overwrite            # 强制覆盖已有 NFO
  python backfill_nfo.py --tv-folder-id <ID>   # 指定 TV 根目录
  python backfill_nfo.py --movie-folder-id <ID> # 指定电影根目录
  python backfill_nfo.py --config /path         # 指定配置文件
"""

import argparse
import logging
import os
import sys
import warnings

warnings.filterwarnings("ignore")

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from drive.client import DriveClient, DriveFile
from mediaparser import Config, TmdbClient, MediaType
from nfo import NfoGenerator

logging.basicConfig(
    level=logging.WARNING,
    format="%(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("backfill")


# ─────────────────────────────────────────────────────────────────
# 工具函数
# ─────────────────────────────────────────────────────────────────

def _query_tmdb_tv(tmdb: TmdbClient, folder_name: str):
    """用文件夹名查询 TMDB（电视剧），依次尝试多种变体。"""
    for name in _name_variants(folder_name):
        info = tmdb._search_by_name(name, None, MediaType.TV)
        if info:
            return info
    return None


def _query_tmdb_movie(tmdb: TmdbClient, folder_name: str):
    """用文件夹名查询 TMDB（电影），依次尝试多种变体。"""
    # 电影文件夹名格式通常是 "片名 (年份)"，尝试提取年份
    import re
    year = None
    clean_name = folder_name
    m = re.search(r'\((\d{4})\)\s*$', folder_name)
    if m:
        year = m.group(1)
        clean_name = folder_name[:m.start()].strip()

    for name in _name_variants(clean_name):
        info = tmdb._search_by_name(name, year, MediaType.MOVIE)
        if info:
            return info
        # 不带年份再试一次
        if year:
            info = tmdb._search_by_name(name, None, MediaType.MOVIE)
            if info:
                return info
    return None


def _name_variants(name: str):
    """生成去重的名称变体列表（原始 / 点分隔 / 空格分隔）。"""
    seen = []
    for variant in [name, name.replace(" ", "."), name.replace(".", " ")]:
        if variant not in seen:
            seen.append(variant)
    return seen


def sep(title: str):
    print(f"\n{'─' * 60}")
    print(f"  {title}")
    print(f"{'─' * 60}")


# ─────────────────────────────────────────────────────────────────
# TV 补传
# ─────────────────────────────────────────────────────────────────

def backfill_tv(
    client: DriveClient,
    tmdb: TmdbClient,
    gen: NfoGenerator,
    tv_root_id: str,
    dry_run: bool = False,
    overwrite: bool = True,
) -> tuple:
    """
    为 tv_root_id 下每个剧名子文件夹补传 tvshow.nfo。

    返回：(ok, skipped, failed) 计数元组
    """
    sep(f"TV 剧集补传  →  {tv_root_id}")
    shows = [f for f in client.list_files(folder_id=tv_root_id) if f.is_folder]

    if not shows:
        print("  （未找到剧集文件夹）")
        return 0, 0, 0

    print(f"  找到 {len(shows)} 个剧集文件夹\n")
    ok = skipped = failed = 0

    for idx, show in enumerate(shows, 1):
        print(f"  [{idx}/{len(shows)}]  📁  {show.name}")

        # 检查是否已有 tvshow.nfo
        existing = client.find_file("tvshow.nfo", folder_id=show.id)
        if existing and not overwrite:
            print(f"         ℹ️   tvshow.nfo 已存在（跳过，使用 --overwrite 强制更新）")
            skipped += 1
            print()
            continue

        info = _query_tmdb_tv(tmdb, show.name)
        if not info:
            print(f"         ⚠️   TMDB 未找到，跳过")
            skipped += 1
            print()
            continue

        tmdb_title = info.get("name") or info.get("title") or ""
        tmdb_year = (info.get("first_air_date") or "")[:4]
        print(f"         TMDB：{tmdb_title} ({tmdb_year})  id={info.get('tmdb_id') or info.get('id')}")

        if dry_run:
            print(f"         tvshow.nfo：[dry-run，不上传]")
            ok += 1
        else:
            try:
                xml = gen.generate_tvshow(info)
                uploaded = client.upload_text(
                    content=xml,
                    name="tvshow.nfo",
                    parent_id=show.id,
                    mime_type="text/xml",
                    overwrite=True,
                )
                action = "更新" if existing else "新建"
                print(f"         tvshow.nfo：✓  {action}  [{uploaded.id[:12]}...]")
                ok += 1
            except Exception as e:
                logger.error("上传失败：%s", e)
                print(f"         tvshow.nfo：✗  {e}")
                failed += 1

        print()

    return ok, skipped, failed


# ─────────────────────────────────────────────────────────────────
# Movie 补传
# ─────────────────────────────────────────────────────────────────

def backfill_movie(
    client: DriveClient,
    tmdb: TmdbClient,
    gen: NfoGenerator,
    movie_root_id: str,
    dry_run: bool = False,
    overwrite: bool = True,
) -> tuple:
    """
    为 movie_root_id 下每个电影子文件夹补传与视频同名的 .nfo。

    返回：(ok, skipped, failed) 计数元组
    """
    sep(f"电影补传  →  {movie_root_id}")
    movie_dirs = [f for f in client.list_files(folder_id=movie_root_id) if f.is_folder]

    if not movie_dirs:
        print("  （未找到电影文件夹）")
        return 0, 0, 0

    print(f"  找到 {len(movie_dirs)} 个电影文件夹\n")
    ok = skipped = failed = 0

    for idx, movie_dir in enumerate(movie_dirs, 1):
        print(f"  [{idx}/{len(movie_dirs)}]  📁  {movie_dir.name}")

        # 找文件夹内的视频文件
        contents = client.list_files(folder_id=movie_dir.id, page_size=50)
        videos = [f for f in contents if f.is_video]

        if not videos:
            print(f"         ⚠️   文件夹内无视频文件，跳过")
            skipped += 1
            print()
            continue

        # 查询 TMDB（用文件夹名，比视频文件名更干净）
        info = _query_tmdb_movie(tmdb, movie_dir.name)
        if not info:
            print(f"         ⚠️   TMDB 未找到，跳过")
            skipped += 1
            print()
            continue

        tmdb_title = info.get("title") or info.get("name") or ""
        tmdb_year = (info.get("release_date") or "")[:4]
        print(f"         TMDB：{tmdb_title} ({tmdb_year})  id={info.get('tmdb_id') or info.get('id')}")

        # 为每个视频文件生成同名 NFO
        any_ok = False
        for video in videos:
            nfo_name = gen.nfo_name_for(video.name)

            # 检查是否已有同名 NFO
            existing_nfo = client.find_file(nfo_name, folder_id=movie_dir.id)
            if existing_nfo and not overwrite:
                print(f"         ℹ️   {nfo_name} 已存在（跳过）")
                continue

            if dry_run:
                action = "更新" if existing_nfo else "新建"
                print(f"         {nfo_name}：[dry-run，{action}，不上传]")
                any_ok = True
            else:
                try:
                    xml = gen.generate(info, media_type=MediaType.MOVIE)
                    uploaded = client.upload_text(
                        content=xml,
                        name=nfo_name,
                        parent_id=movie_dir.id,
                        mime_type="text/xml",
                        overwrite=True,
                    )
                    action = "更新" if existing_nfo else "新建"
                    print(f"         {nfo_name}：✓  {action}  [{uploaded.id[:12]}...]")
                    any_ok = True
                except Exception as e:
                    logger.error("上传失败：%s", e)
                    print(f"         {nfo_name}：✗  {e}")
                    failed += 1

        if any_ok:
            ok += 1

        print()

    return ok, skipped, failed


# ─────────────────────────────────────────────────────────────────
# CLI
# ─────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="为已整理好的电影/剧集文件夹批量补传 NFO（Infuse / Plex 兼容）",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例：
  python backfill_nfo.py                          # 处理电影 + 剧集（默认）
  python backfill_nfo.py --mode tv                # 只处理剧集
  python backfill_nfo.py --mode movie             # 只处理电影
  python backfill_nfo.py --dry-run                # 预览，不上传
  python backfill_nfo.py --overwrite              # 强制覆盖已有 NFO
  python backfill_nfo.py --movie-folder-id 1AbC  # 指定电影根目录
  python backfill_nfo.py --tv-folder-id 1DeF     # 指定剧集根目录
""",
    )
    parser.add_argument(
        "--mode",
        choices=["all", "tv", "movie"],
        default="all",
        help="处理模式：all（默认）/ tv / movie",
    )
    parser.add_argument(
        "--tv-folder-id",
        default=None,
        metavar="DRIVE_ID",
        help="TV 根目录 ID（覆盖 config 里的 tv_root_id）",
    )
    parser.add_argument(
        "--movie-folder-id",
        default=None,
        metavar="DRIVE_ID",
        help="电影根目录 ID（覆盖 config 里的 movie_root_id）",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="只查询 TMDB，不实际上传文件",
    )
    parser.add_argument(
        "--overwrite",
        action="store_true",
        default=True,
        help="覆盖已有 NFO（默认开启）",
    )
    parser.add_argument(
        "--no-overwrite",
        dest="overwrite",
        action="store_false",
        help="跳过已有 NFO，不覆盖",
    )
    parser.add_argument(
        "--config",
        default=None,
        metavar="PATH",
        help="配置文件路径（默认自动查找 config/config.yaml）",
    )
    parser.add_argument(
        "--verbose", "-v",
        action="store_true",
        help="输出详细日志",
    )
    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    cfg = Config.load(args.config)

    # ── 初始化 Drive ────────────────────────────────────
    drive_cfg = cfg.drive
    try:
        client = DriveClient.from_oauth(
            credentials_path=drive_cfg.credentials_json,
            token_path=drive_cfg.token_json,
        )
    except FileNotFoundError as e:
        print(f"❌  认证文件不存在：{e}")
        sys.exit(1)

    # ── 初始化 TMDB ─────────────────────────────────────
    if not cfg.is_tmdb_ready():
        print("❌  未配置 TMDB API Key（config.yaml → tmdb.api_key）")
        sys.exit(1)

    tmdb = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
    )
    gen = NfoGenerator()

    print("=" * 60)
    print("  NFO 补传工具")
    if args.dry_run:
        print("  ⚠️  DRY-RUN — 不会上传文件")
    if not args.overwrite:
        print("  ℹ️  --no-overwrite — 已有 NFO 将跳过")
    print("=" * 60)

    total_ok = total_skip = total_fail = 0

    try:
        # ── TV ──────────────────────────────────────────
        if args.mode in ("all", "tv"):
            tv_id = (
                args.tv_folder_id
                or cfg.organizer.tv_root_id
                or cfg.organizer.root_folder_id
            )
            if not tv_id:
                print("⚠️  未找到 TV 根目录（config.yaml → organizer.tv_root_id），跳过剧集")
            else:
                ok, skip, fail = backfill_tv(
                    client, tmdb, gen, tv_id,
                    dry_run=args.dry_run,
                    overwrite=args.overwrite,
                )
                total_ok += ok
                total_skip += skip
                total_fail += fail

        # ── Movie ────────────────────────────────────────
        if args.mode in ("all", "movie"):
            movie_id = (
                args.movie_folder_id
                or cfg.organizer.movie_root_id
                or cfg.organizer.root_folder_id
            )
            if not movie_id:
                print("⚠️  未找到电影根目录（config.yaml → organizer.movie_root_id），跳过电影")
            else:
                ok, skip, fail = backfill_movie(
                    client, tmdb, gen, movie_id,
                    dry_run=args.dry_run,
                    overwrite=args.overwrite,
                )
                total_ok += ok
                total_skip += skip
                total_fail += fail

    except KeyboardInterrupt:
        print("\n\n⚠️  用户中断")
        sys.exit(130)

    # ── 总结 ─────────────────────────────────────────────
    print(f"\n{'=' * 60}")
    print(f"  全部完成  ✓ 成功：{total_ok}    ⚠ 跳过：{total_skip}    ✗ 失败：{total_fail}")
    print(f"{'=' * 60}\n")


if __name__ == "__main__":
    main()

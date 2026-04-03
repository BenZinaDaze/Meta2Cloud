#!/usr/bin/env python3
"""
backfill_tvshow_nfo.py —— 为已整理好的剧集文件夹补传 tvshow.nfo

使用场景：
  - pipeline.py 运行前文件夹已存在，缺少 tvshow.nfo
  - Infuse 将剧集显示在「其他」而非「电视剧」
  - 需要批量刷新某个 TV 根目录下所有剧的元数据

流程：
  扫描 TV 根目录下的所有子文件夹（每个子文件夹 = 一部剧）
  → 用文件夹名称查询 TMDB
  → 生成 tvshow.nfo（<tvshow> 根标签，Infuse / Plex 兼容）
  → 上传到剧名文件夹根目录（overwrite=True）

用法：
  python backfill_tvshow_nfo.py                     # 处理 config 里 tv_root_id 下所有剧
  python backfill_tvshow_nfo.py --folder-id <ID>    # 指定任意 Drive 文件夹
  python backfill_tvshow_nfo.py --dry-run            # 只查询 TMDB，不上传
  python backfill_tvshow_nfo.py --config /path       # 指定配置文件
"""

import argparse
import logging
import sys
import os
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

def backfill(
    client: DriveClient,
    tmdb: TmdbClient,
    gen: NfoGenerator,
    tv_root_id: str,
    dry_run: bool = False,
) -> None:
    """
    遍历 tv_root_id 下的所有子文件夹，为每个剧补传 tvshow.nfo。

    :param client:     DriveClient 实例
    :param tmdb:       TmdbClient 实例
    :param gen:        NfoGenerator 实例
    :param tv_root_id: TV 根目录文件夹 ID
    :param dry_run:    True 时只查询 TMDB，不上传文件
    """
    print(f"\n📂 扫描 TV 根目录：{tv_root_id}")
    shows = [f for f in client.list_files(folder_id=tv_root_id) if f.is_folder]

    if not shows:
        print("  （未找到剧集文件夹）")
        return

    print(f"  找到 {len(shows)} 个剧集文件夹\n")

    ok_count = 0
    skip_count = 0
    fail_count = 0

    for idx, show in enumerate(shows, 1):
        print(f"[{idx}/{len(shows)}]  📁  {show.name}")

        # 检查是否已有 tvshow.nfo
        existing = client.find_file("tvshow.nfo", folder_id=show.id)
        if existing and not dry_run:
            print(f"       ℹ️   tvshow.nfo 已存在（将覆盖更新）")

        # 查询 TMDB
        info = _query_tmdb(tmdb, show.name)

        if not info:
            print(f"       ⚠️   TMDB 未找到，跳过")
            skip_count += 1
            print()
            continue

        tmdb_title = info.get("name") or info.get("title") or ""
        tmdb_year = (info.get("first_air_date") or "")[:4]
        tmdb_id = info.get("tmdb_id") or info.get("id")
        print(f"       TMDB：{tmdb_title} ({tmdb_year})  tmdb_id={tmdb_id}")

        # 生成 XML
        xml = gen.generate_tvshow(info)

        if dry_run:
            print(f"       tvshow.nfo：[dry-run，不上传]")
            ok_count += 1
        else:
            try:
                uploaded = client.upload_text(
                    content=xml,
                    name="tvshow.nfo",
                    parent_id=show.id,
                    mime_type="text/xml",
                    overwrite=True,
                )
                print(f"       tvshow.nfo：✓  [{uploaded.id[:12]}...]")
                ok_count += 1
            except Exception as e:
                logger.error("上传失败：%s", e)
                print(f"       tvshow.nfo：✗ 上传失败 — {e}")
                fail_count += 1

        print()

    # 摘要
    print("=" * 60)
    print(f"  完成  ✓ 成功：{ok_count}    ⚠ 跳过：{skip_count}    ✗ 失败：{fail_count}")
    print("=" * 60)


def _query_tmdb(tmdb: TmdbClient, folder_name: str):
    """
    用文件夹名称查询 TMDB（电视剧类型）。
    依次尝试：原始名称 → 以点分隔 → 以空格分隔。
    """
    candidates = [
        folder_name,
        folder_name.replace(" ", "."),
        folder_name.replace(".", " "),
    ]
    seen = []
    for name in candidates:
        if name in seen:
            continue
        seen.append(name)
        info = tmdb._search_by_name(name, None, MediaType.TV)
        if info:
            return info
    return None


# ─────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="为已整理好的剧集文件夹补传 tvshow.nfo（Infuse / Plex 兼容）",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
示例：
  python backfill_tvshow_nfo.py                      # 处理 config 里的 tv_root_id
  python backfill_tvshow_nfo.py --folder-id 1AbC...  # 指定任意文件夹
  python backfill_tvshow_nfo.py --dry-run            # 预览，不上传
""",
    )
    parser.add_argument(
        "--folder-id",
        default=None,
        metavar="DRIVE_ID",
        help="TV 根目录文件夹 ID（不填则使用 config 里的 tv_root_id 或 root_folder_id）",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="只查询 TMDB，不实际上传文件",
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

    # ── 加载配置 ────────────────────────────────────────
    cfg = Config.load(args.config)

    # ── 确定 TV 根目录 ──────────────────────────────────
    tv_root_id = (
        args.folder_id
        or cfg.organizer.tv_root_id
        or cfg.organizer.root_folder_id
    )
    if not tv_root_id:
        print("❌  未指定 TV 根目录，请在 config.yaml 设置 organizer.tv_root_id 或使用 --folder-id")
        sys.exit(1)

    # ── 初始化 Drive 客户端 ─────────────────────────────
    drive_cfg = cfg.drive
    try:
        if drive_cfg.auth_mode == "service_account":
            client = DriveClient.from_service_account(drive_cfg.service_account_json)
        else:
            client = DriveClient.from_oauth(
                credentials_path=drive_cfg.credentials_json,
                token_path=drive_cfg.token_json,
            )
    except FileNotFoundError as e:
        print(f"❌  认证文件不存在：{e}")
        sys.exit(1)

    # ── 初始化 TMDB 客户端 ──────────────────────────────
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

    # ── 运行补传 ────────────────────────────────────────
    print("=" * 60)
    print("  tvshow.nfo 补传工具")
    if args.dry_run:
        print("  ⚠️  DRY-RUN 模式 — 不会上传文件")
    print("=" * 60)

    try:
        backfill(
            client=client,
            tmdb=tmdb,
            gen=gen,
            tv_root_id=tv_root_id,
            dry_run=args.dry_run,
        )
    except KeyboardInterrupt:
        print("\n\n⚠️  用户中断")
        sys.exit(130)


if __name__ == "__main__":
    main()

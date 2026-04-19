#!/usr/bin/env python3
"""
CLI 入口：python -m core

用法：
  python -m core                  # 正式运行（使用配置文件中的存储后端）
  python -m core --dry-run        # 预览计划，不实际操作
  python -m core --storage pan115 # 使用 115 网盘作为存储后端
  python -m core --no-tmdb        # 只整理文件夹，不查 TMDB/生成 NFO
  python -m core --no-images      # 跳过图片下载上传
"""

import argparse
import logging
import sys

from mediaparser import Config
from core.pipeline import Pipeline

logging.basicConfig(
    level=logging.WARNING,
    format="%(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("pipeline")


def main():
    # 强制让 Windows 下的输出使用 utf-8 编码，防止部分 Emoji 或特殊字符在 GBK 环境下报错崩溃
    if sys.stdout.encoding.lower() != 'utf-8':
        try:
            sys.stdout.reconfigure(encoding='utf-8')
        except Exception:
            pass

    parser = argparse.ArgumentParser(
        description="Meta2Cloud — 扫描云存储媒体文件，查询 TMDB 元数据，生成 NFO，整理到目标文件夹",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
流程：
  扫描源文件夹 → 解析文件名 → TMDB 查询（整剧+单集）→ 生成 NFO
  → 创建文件夹 → 上传 NFO + tvshow.nfo + season.nfo → 下载图片 → 移动文件

示例：
  python -m core                          # 正式运行（使用配置文件中的存储后端）
  python -m core --dry-run                # 预览计划，不实际操作
  python -m core --storage pan115         # 使用 115 网盘作为存储后端
  python -m core --storage google_drive   # 使用 Google Drive
  python -m core --no-tmdb                # 只整理文件夹，不查 TMDB/生成 NFO
  python -m core --no-images              # 跳过图片下载上传
""",
    )
    parser.add_argument("--dry-run", action="store_true", help="只打印计划，不实际操作")
    parser.add_argument("--no-tmdb", action="store_true", help="跳过 TMDB 查询（不生成 NFO，只整理文件夹）")
    parser.add_argument("--no-images", action="store_true", help="跳过图片下载上传（poster/fanart）")
    parser.add_argument("--storage", default=None, metavar="NAME",
                        help="存储后端名称（google_drive / pan115），覆盖配置文件 storage.primary")
    parser.add_argument("--config", default=None, metavar="PATH", help="配置文件路径（默认自动查找 config/config.yaml）")
    parser.add_argument("--verbose", "-v", action="store_true", help="输出详细日志")
    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    cfg = Config.load(args.config)

    # 确定使用哪个存储后端：CLI --storage 优先，否则读取 config 中的 storage.primary
    storage_name = args.storage or cfg.storage.primary
    cfg.storage.primary = storage_name
    try:
        from storage import get_provider
        provider = get_provider(storage_name, cfg)
        logger.info("使用存储后端：%s", provider.provider_name)
    except FileNotFoundError as e:
        print(f"❌  认证文件不存在：{e}")
        sys.exit(1)
    except ValueError as e:
        print(f"❌  {e}")
        sys.exit(1)
    except Exception as e:
        print(f"❌  初始化存储客户端失败（{storage_name}）：{e}")
        sys.exit(1)

    pipe = Pipeline(
        client=provider,
        cfg=cfg,
        dry_run=args.dry_run,
        skip_tmdb=args.no_tmdb,
        skip_images=args.no_images,
    )
    try:
        pipe.run()
    except KeyboardInterrupt:
        print("\n\n⚠️  用户中断")
        sys.exit(130)


if __name__ == "__main__":
    main()

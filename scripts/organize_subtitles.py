#!/usr/bin/env python3
"""
独立整理字幕文件夹

用法：
    uv run python scripts/organize_subtitles.py --dry-run    # 预览
    uv run python scripts/organize_subtitles.py              # 执行
"""
from __future__ import annotations

import argparse
import os
import sys
from pathlib import Path

ROOT = Path(__file__).parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from core.subtitle_matcher import SubtitleMatcher
from core.organizer import MediaOrganizer
from mediaparser import MetaInfo, TmdbClient
from mediaparser.config import Config
from mediaparser.meta_video import MediaType
from storage.pan115 import Pan115Provider
from storage.base import CloudFile


def scan_subtitles_recursive(client, folder_id: str, depth: int = 0) -> list[CloudFile]:
    """递归扫描字幕文件"""
    if depth > 10:
        return []
    items = client.list_files(folder_id)
    subtitles = [f for f in items if f.is_subtitle]
    for f in items:
        if f.is_folder:
            subtitles.extend(scan_subtitles_recursive(client, f.id, depth + 1))
    return subtitles


def build_clean_name(tmdb_info: dict, is_tv: bool, season_num: int, episode_num: int | None, ext: str) -> str:
    """根据 TMDB 数据生成标准化文件名（从 pipeline 复制）"""
    def safe_filename(name: str) -> str:
        return "".join(c for c in name if c not in r'<>:"/\|?*').strip()

    if is_tv:
        title = tmdb_info.get("name") or tmdb_info.get("title") or "Unknown"
        ep_str = f"S{season_num:02d}E{episode_num:02d}" if episode_num else f"S{season_num:02d}"
        return f"{safe_filename(title)} - {ep_str}{ext}"
    else:
        title = tmdb_info.get("title") or tmdb_info.get("name") or "Unknown"
        year = (tmdb_info.get("release_date") or "")[:4]
        if year:
            return f"{safe_filename(title)} ({year}){ext}"
        return f"{safe_filename(title)}{ext}"


def main():
    parser = argparse.ArgumentParser(description="独立整理字幕")
    parser.add_argument("--dry-run", action="store_true", help="预览模式")
    parser.add_argument("--source", type=str, help="字幕文件夹ID")
    args = parser.parse_args()

    cfg = Config.load()
    client = Pan115Provider.from_config(cfg)
    matcher = SubtitleMatcher()
    organizer = MediaOrganizer(
        client=client,
        root_folder_id=cfg.active_root_folder_id(),
        movie_root_id=cfg.active_movie_root_id() or None,
        tv_root_id=cfg.active_tv_root_id() or None,
        dry_run=args.dry_run,
    )
    tmdb = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
    ) if cfg.is_tmdb_ready() else None

    source_id = args.source or cfg.u115.download_folder_id
    replace_existing = cfg.pipeline.replace_existing_video

    print(f"字幕源: {source_id}")
    print(f"模式: {'预览' if args.dry_run else '执行'}")
    print(f"TMDB: {'已启用' if tmdb else '未配置'}\n")

    # 扫描字幕
    print("扫描字幕...")
    subtitles = scan_subtitles_recursive(client, source_id)
    print(f"找到 {len(subtitles)} 个字幕\n")

    # 去重：同一剧集只查询一次 TMDB
    tmdb_cache: dict[str, dict] = {}

    # 整理
    for sub in subtitles:
        stem = Path(sub.name).stem
        clean_name, tags = matcher.parse_subtitle_tags(stem)

        # 解析媒体信息
        meta = MetaInfo(clean_name, isfile=True)
        if not meta.name:
            print(f"  跳过 {sub.name} -> 无法解析")
            continue

        is_tv = meta.type == MediaType.TV
        season_num = int(meta.season.replace("S", "")) if meta.season else None
        episode_num = int(meta.episode.replace("E", "")) if meta.episode else None

        # 获取 TMDB 信息
        cache_key = f"{meta.name}_{meta.type.value}"
        if cache_key in tmdb_cache:
            tmdb_info = tmdb_cache[cache_key]
        elif tmdb:
            tmdb_info = tmdb.recognize(meta)
            if tmdb_info:
                tmdb_cache[cache_key] = tmdb_info
            else:
                print(f"  跳过 {sub.name} -> TMDB 未找到 '{meta.name}'")
                continue
        else:
            tmdb_info = None

        if not tmdb_info:
            print(f"  跳过 {sub.name} -> 无 TMDB 信息")
            continue

        # 更新 meta 使用 TMDB 标题
        tmdb_title = tmdb_info.get("name") or tmdb_info.get("title") or meta.name
        meta.name = tmdb_title
        tmdb_year = (tmdb_info.get("release_date") or tmdb_info.get("first_air_date") or "")[:4]
        if tmdb_year:
            meta.year = tmdb_year

        # 确定目标文件夹
        if args.dry_run:
            # 预览模式下模拟目标路径
            if is_tv:
                target_path = f"{tmdb_title}/Season {season_num:02d}" if season_num else tmdb_title
            else:
                target_path = tmdb_title
        else:
            target_folder = organizer.ensure_folder_for_meta(meta, label=sub.name)
            if not target_folder:
                print(f"  跳过 {sub.name} -> 无法创建目标文件夹")
                continue

        # 构建标准化的视频名（用于字幕命名）
        video_name = build_clean_name(
            tmdb_info=tmdb_info,
            is_tv=is_tv,
            season_num=season_num or 1,
            episode_num=episode_num,
            ext=os.path.splitext(sub.name)[1],
        )

        # 构建字幕目标文件名
        target_subtitle_name = matcher.build_target_name(video_name, sub.extension, tags)

        if args.dry_run:
            print(f"  [预览] {sub.name} -> {target_path}/{target_subtitle_name}")
        else:
            # 检查目标位置是否已存在
            existing = client.find_file(target_subtitle_name, folder_id=target_folder.id)
            if existing:
                if existing.id == sub.id:
                    print(f"  跳过 {sub.name} -> 已就位")
                    continue
                elif replace_existing:
                    client.trash_file(existing.id)
                else:
                    print(f"  跳过 {sub.name} -> 目标已存在")
                    continue

            client.move_file(sub.id, target_folder.id, target_subtitle_name)
            print(f"  ✓ {sub.name} -> {target_folder.name}/{target_subtitle_name}")


if __name__ == "__main__":
    main()

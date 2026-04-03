#!/usr/bin/env python3
"""
fix_existing.py —— 修复 Drive 上已整理但命名不标准的媒体文件

问题根源：旧 pipeline 保留了原始垃圾文件名，导致 Infuse/Plex/Emby 无法正确识别。

修复内容：
  1. Season 文件夹改名：Season 1 → Season 01（跳过已正确的）
  2. 视频文件改名：垃圾文件名 → TMDB 标准名
       电影：Inception (2010).mkv
       剧集：Breaking Bad - S01E03.mkv
  3. NFO 文件改名：与视频文件名保持同 stem

用法：
  python fix_existing.py                # 预览计划（dry-run，不实际改名）
  python fix_existing.py --apply        # 实际执行
  python fix_existing.py --movie-only   # 只处理电影
  python fix_existing.py --tv-only      # 只处理剧集
  python fix_existing.py --no-tmdb      # 不查 TMDB（只修 Season 文件夹名）
  python fix_existing.py --config PATH  # 指定配置文件路径
"""

import argparse
import logging
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from drive.client import DriveClient, DriveFile
from mediaparser import MetaInfo, TmdbClient, Config, MediaType

logging.basicConfig(
    level=logging.WARNING,
    format="%(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("fix_existing")

# ── 视频文件扩展名列表 ─────────────────────────────────────────────────────────
_VIDEO_EXTS = {".mkv", ".mp4", ".avi", ".mov", ".ts", ".m2ts", ".wmv", ".rmvb", ".iso"}
# ── Season 文件夹名正则 ────────────────────────────────────────────────────────
_SEASON_RE = re.compile(r"^Season\s+(\d+)$", re.IGNORECASE)


# ─────────────────────────────────────────────────────────────────────────────
# 工具函数
# ─────────────────────────────────────────────────────────────────────────────

def _safe_name(s: str) -> str:
    """去掉文件名中的非法字符"""
    return re.sub(r'[\\/:*?"<>|]', "", s).strip()


def _build_clean_name(tmdb_info: dict, is_tv: bool,
                      season_num: int, episode_num: int | None,
                      ext: str) -> str:
    """与 pipeline._build_clean_name 逻辑一致"""
    if is_tv:
        title = tmdb_info.get("name") or tmdb_info.get("title") or "Unknown"
        ep_str = f"S{season_num:02d}E{episode_num:02d}" if episode_num else f"S{season_num:02d}"
        return f"{_safe_name(title)} - {ep_str}{ext}"
    else:
        title = tmdb_info.get("title") or tmdb_info.get("name") or "Unknown"
        year = (tmdb_info.get("release_date") or "")[:4]
        year_str = f" ({year})" if year else ""
        return f"{_safe_name(title)}{year_str}{ext}"


def _is_video(f: DriveFile) -> bool:
    return f.extension in _VIDEO_EXTS


def _is_nfo(f: DriveFile) -> bool:
    return f.extension == ".nfo"


# ─────────────────────────────────────────────────────────────────────────────
# 修复器
# ─────────────────────────────────────────────────────────────────────────────

class Fixer:
    def __init__(self, client: DriveClient, cfg: Config,
                 dry_run: bool, skip_tmdb: bool,
                 movie_only: bool, tv_only: bool):
        self._client = client
        self._cfg = cfg
        self._dry_run = dry_run
        self._skip_tmdb = skip_tmdb
        self._movie_only = movie_only
        self._tv_only = tv_only

        self._tmdb: TmdbClient | None = None
        if not skip_tmdb and cfg.is_tmdb_ready():
            self._tmdb = TmdbClient(
                api_key=cfg.tmdb.api_key,
                language=cfg.tmdb.language,
                proxy=cfg.tmdb_proxy,
                timeout=cfg.tmdb.timeout,
            )

        # 统计
        self.stat = {"season_renamed": 0, "file_renamed": 0, "nfo_renamed": 0,
                     "skipped": 0, "errors": 0}

    # ── 主入口 ────────────────────────────────────────────────────────────────

    def run(self):
        cfg = self._cfg
        roots = []
        if not self._tv_only:
            mid = cfg.organizer.movie_root_id or cfg.organizer.root_folder_id
            if mid:
                roots.append(("Movie", mid))
        if not self._movie_only:
            tid = cfg.organizer.tv_root_id or cfg.organizer.root_folder_id
            if tid:
                roots.append(("TV", tid))

        if not roots:
            print("❌  配置中未找到 movie_root_id / tv_root_id / root_folder_id，请检查配置")
            sys.exit(1)

        print("=" * 68)
        print("  fix_existing.py — Drive 媒体文件命名修复")
        if self._dry_run:
            print("  ⚠️  DRY-RUN 模式 — 不会实际改名")
        if self._skip_tmdb or not self._tmdb:
            print("  ℹ️  跳过 TMDB — 只修 Season 文件夹名")
        print("=" * 68)

        for label, root_id in roots:
            print(f"\n📂 扫描 {label} 根目录 [{root_id[:12]}...]")
            self._scan_root(root_id, label)

        print("\n" + "=" * 68)
        print("  完成")
        s = self.stat
        print(f"  Season 文件夹改名：{s['season_renamed']}")
        print(f"  视频文件改名：{s['file_renamed']}")
        print(f"  NFO 文件改名：{s['nfo_renamed']}")
        print(f"  跳过（已是标准名）：{s['skipped']}")
        print(f"  错误：{s['errors']}")
        print("=" * 68)

    # ── 扫描根目录 ────────────────────────────────────────────────────────────

    def _scan_root(self, root_id: str, root_label: str):
        """
        根目录里每个子文件夹是一部电影或一部剧集。
        """
        title_folders = self._client.list_files(root_id, page_size=500)
        for tf in title_folders:
            if not tf.is_folder:
                continue
            print(f"\n  📁 {tf.name}")
            # 判断是电影还是剧集：看里面有没有 Season 子文件夹
            children = self._client.list_files(tf.id, page_size=200)
            season_folders = [c for c in children if c.is_folder and _SEASON_RE.match(c.name)]

            if season_folders:
                # 剧集
                if self._movie_only:
                    continue
                for sf in season_folders:
                    self._fix_season_folder(sf, parent_id=tf.id)
            else:
                # 电影（或剧集只有顶层文件）
                if self._tv_only:
                    continue
                self._fix_files_in_folder(children, parent_id=tf.id, is_tv=False,
                                           season_num=1, episode_num=None)

    # ── 修复 Season 文件夹 ────────────────────────────────────────────────────

    def _fix_season_folder(self, sf: DriveFile, parent_id: str):
        m = _SEASON_RE.match(sf.name)
        if not m:
            return
        season_num = int(m.group(1))
        correct_name = f"Season {season_num:02d}"

        if sf.name != correct_name:
            print(f"    🔧 Season 文件夹：{sf.name!r} → {correct_name!r}", end="")
            if not self._dry_run:
                try:
                    self._client.rename_file(sf.id, correct_name)
                    print("  ✓")
                    self.stat["season_renamed"] += 1
                except Exception as e:
                    print(f"  ❌ {e}")
                    self.stat["errors"] += 1
            else:
                print("  [dry-run]")
                self.stat["season_renamed"] += 1
        else:
            print(f"    ✅ Season 文件夹：{sf.name!r} 已是标准名")

        # 修复 Season 里的文件
        children = self._client.list_files(sf.id, page_size=500)
        self._fix_files_in_folder(children, parent_id=sf.id, is_tv=True,
                                   season_num=season_num, episode_num=None)

    # ── 修复文件夹内的视频 + NFO ──────────────────────────────────────────────

    def _fix_files_in_folder(self, children: list, parent_id: str,
                              is_tv: bool, season_num: int, episode_num: int | None):
        videos = [f for f in children if _is_video(f)]
        nfos   = {os.path.splitext(f.name)[0]: f for f in children if _is_nfo(f)}

        for video in videos:
            old_stem = os.path.splitext(video.name)[0]
            ext = video.extension  # 已含 '.'

            # 解析文件名
            meta = MetaInfo(video.name, isfile=True)
            if not meta.name:
                print(f"    ⚠️  {video.name!r} — 解析失败，跳过")
                self.stat["skipped"] += 1
                continue

            ep_num = int(meta.episode_seq) if meta.episode_seq else episode_num
            s_num  = int(meta.season_seq)  if meta.season_seq  else season_num

            # 查 TMDB
            tmdb_info = None
            if self._tmdb and not self._skip_tmdb:
                tmdb_info = self._tmdb.recognize(meta)

            if tmdb_info:
                clean = _build_clean_name(tmdb_info, is_tv=is_tv,
                                          season_num=s_num, episode_num=ep_num,
                                          ext=ext)
            else:
                # 无 TMDB 数据时，用解析结果拼干净名
                if is_tv and meta.name:
                    ep_str = (f"S{s_num:02d}E{ep_num:02d}" if ep_num
                              else f"S{s_num:02d}")
                    clean = f"{_safe_name(meta.name)} - {ep_str}{ext}"
                elif meta.name:
                    year_str = f" ({meta.year})" if meta.year else ""
                    clean = f"{_safe_name(meta.name)}{year_str}{ext}"
                else:
                    print(f"    ⚠️  {video.name!r} — 无法生成干净名，跳过")
                    self.stat["skipped"] += 1
                    continue

            clean_stem = os.path.splitext(clean)[0]

            # ── 重命名视频 ──────────────────────────────────────────────────
            if video.name == clean:
                print(f"    ✅ {video.name!r} — 已是标准名")
                self.stat["skipped"] += 1
            else:
                print(f"    🔧 视频：{video.name!r}")
                print(f"         → {clean!r}", end="")
                if not self._dry_run:
                    try:
                        self._client.rename_file(video.id, clean)
                        print("  ✓")
                        self.stat["file_renamed"] += 1
                    except Exception as e:
                        print(f"  ❌ {e}")
                        self.stat["errors"] += 1
                else:
                    print("  [dry-run]")
                    self.stat["file_renamed"] += 1

            # ── 重命名对应 NFO ──────────────────────────────────────────────
            nfo_file = nfos.get(old_stem)
            if nfo_file:
                new_nfo_name = clean_stem + ".nfo"
                if nfo_file.name == new_nfo_name:
                    print(f"    ✅ {nfo_file.name!r} — NFO 已是标准名")
                else:
                    print(f"    🔧 NFO：{nfo_file.name!r}")
                    print(f"         → {new_nfo_name!r}", end="")
                    if not self._dry_run:
                        try:
                            self._client.rename_file(nfo_file.id, new_nfo_name)
                            print("  ✓")
                            self.stat["nfo_renamed"] += 1
                        except Exception as e:
                            print(f"  ❌ {e}")
                            self.stat["errors"] += 1
                    else:
                        print("  [dry-run]")
                        self.stat["nfo_renamed"] += 1


# ─────────────────────────────────────────────────────────────────────────────
# CLI 入口
# ─────────────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="修复 Drive 上已整理但命名不标准的媒体文件",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("--apply",      action="store_true", help="实际执行改名（默认 dry-run）")
    parser.add_argument("--movie-only", action="store_true", help="只处理电影")
    parser.add_argument("--tv-only",    action="store_true", help="只处理剧集")
    parser.add_argument("--no-tmdb",    action="store_true", help="跳过 TMDB 查询")
    parser.add_argument("--config",     default=None, metavar="PATH", help="配置文件路径")
    parser.add_argument("--verbose",    action="store_true", help="输出详细日志")
    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    cfg = Config.load(args.config)

    try:
        drive_cfg = cfg.drive
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
    except Exception as e:
        print(f"❌  初始化 Drive 客户端失败：{e}")
        sys.exit(1)

    fixer = Fixer(
        client=client,
        cfg=cfg,
        dry_run=not args.apply,
        skip_tmdb=args.no_tmdb,
        movie_only=args.movie_only,
        tv_only=args.tv_only,
    )
    try:
        fixer.run()
    except KeyboardInterrupt:
        print("\n\n⚠️  用户中断")
        sys.exit(130)


if __name__ == "__main__":
    main()

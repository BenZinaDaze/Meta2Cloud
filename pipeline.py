#!/usr/bin/env python3
"""
pipeline.py —— Metadata2GD 完整流水线

流程：
  1. 从 Google Drive 扫描目标文件夹中的视频文件
  2. 用 MetaInfo 解析文件名（媒体类型 / 标题 / 年份 / 季集号）
  3. 用 TmdbClient 查询整剧/整片元数据
  4. TV：额外调用 get_episode_detail() 获取单集真实标题/简介/导演/剧照
  5. 用 NfoGenerator 生成 Plex / Infuse 兼容的 NFO XML
  6. 用 MediaOrganizer 在 Drive 幂等创建目标文件夹
  7. 上传与视频同名的 NFO（单集 or 电影）
  8. TV：上传 tvshow.nfo 到剧名文件夹（每剧一次）
  9. TV：上传 season.nfo 到 Season 文件夹（每季一次）
  10.  上传 poster.jpg / fanart.jpg 封面图片
  11. TV：上传 season01-poster.jpg 季封面（每季一次）
  12. 将视频文件移动到目标文件夹

NFO 命名规则（Plex / Infuse 标准）：
  视频文件名去掉扩展名 + .nfo，例：Breaking.Bad.S01E03.mkv → Breaking.Bad.S01E03.nfo

用法：
  python pipeline.py                  # 使用 config/config.yaml，正式运行
  python pipeline.py --dry-run        # 只打印计划，不操作 Drive
  python pipeline.py --no-tmdb        # 跳过 TMDB，只整理文件夹
  python pipeline.py --no-images      # 跳过图片下载上传
  python pipeline.py --config /path   # 指定配置文件
"""

import argparse
import logging
import sys
import os
from collections import defaultdict
from dataclasses import dataclass, field
from typing import List, Optional, Set

import requests

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from drive.client import DriveClient, DriveFile
from mediaparser import MetaInfo, TmdbClient, Config, MediaType
from mediaparser.release_group import ReleaseGroupsMatcher
from nfo import NfoGenerator, ImageUploader
from organizer import MediaOrganizer
try:
    from webui.tmdb_cache import TmdbCache as _WebUiCache
except ImportError:
    _WebUiCache = None  # type: ignore

logging.basicConfig(
    level=logging.WARNING,
    format="%(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("pipeline")


# ─────────────────────────────────────────────────────────────────
# 数据结构
# ─────────────────────────────────────────────────────────────────

@dataclass
class ProcessResult:
    file: DriveFile
    status: str          # "ok" | "skipped" | "failed"
    reason: str = ""
    target_folder: Optional[DriveFile] = None
    nfo_uploaded: bool = False
    moved: bool = False


@dataclass
class _NotifyItem:
    title: str
    year: str
    is_tv: bool
    season: int
    episode: Optional[int]
    ep_title: str          # 单集标题（可为空）
    poster_path: str       # 整剧封面路径（回退用）
    season_poster_path: str = ""  # 季封面路径（TG 通知优先使用）
    tmdb_id: str = ""
    no_tmdb: bool = False  # True 表示 TMDB 未找到元数据
    original_name: str = ""  # Drive 原始文件名（用于 no_tmdb 通知）


# ─────────────────────────────────────────────────────────────────
# Pipeline
# ─────────────────────────────────────────────────────────────────

class Pipeline:
    """
    完整流水线：扫描 → 解析 → TMDB（整剧+单集）→ NFO → 文件夹 → 图片 → 移动
    """

    def __init__(
        self,
        client: DriveClient,
        cfg: Config,
        dry_run: bool = False,
        skip_tmdb: bool = False,
        skip_images: bool = False,
    ):
        self._client = client
        self._cfg = cfg
        self._dry_run = dry_run or cfg.pipeline.dry_run
        self._skip_tmdb = skip_tmdb or cfg.pipeline.skip_tmdb
        self._skip_images = skip_images

        self._organizer = MediaOrganizer(
            client=client,
            root_folder_id=cfg.organizer.root_folder_id,
            movie_root_id=cfg.organizer.movie_root_id or None,
            tv_root_id=cfg.organizer.tv_root_id or None,
            dry_run=self._dry_run,
        )

        # 写入 WebUI SQLite 缓存（同时供整理流程和后端读取复用）
        self._tmdb_write_cache = None
        if _WebUiCache is not None:
            _cache_db = os.path.join(
                os.path.dirname(os.path.abspath(__file__)), "config", "data", "tmdb_cache.db"
            )
            try:
                self._tmdb_write_cache = _WebUiCache(_cache_db)
            except Exception as _e:
                logger.debug("无法初始化 WebUI TMDB 缓存：%s", _e)

        self._tmdb: Optional[TmdbClient] = None
        if not self._skip_tmdb and cfg.is_tmdb_ready():
            self._tmdb = TmdbClient(
                api_key=cfg.tmdb.api_key,
                language=cfg.tmdb.language,
                proxy=cfg.tmdb_proxy,
                timeout=cfg.tmdb.timeout,
                cache=self._tmdb_write_cache,
            )

        self._nfo_gen = NfoGenerator()

        self._img_uploader: Optional[ImageUploader] = None
        if not self._skip_images and not self._dry_run:
            self._img_uploader = ImageUploader(client)

        # 幂等去重集合
        self._tvshow_nfo_done: Set[str] = set()   # 已上传 tvshow.nfo 的剧名文件夹 ID
        self._season_nfo_done: Set[str] = set()    # 已上传 season.nfo 的 Season 文件夹 ID
        self._poster_done: Set[str] = set()        # 已上传 poster/fanart 的文件夹 ID
        self._season_poster_done: Set[str] = set() # 已上传季封面的文件夹 ID
        # 剧名文件夹缓存：key = (tmdb_id, season) 用于季层，top key = tmdb_id 用于第一层
        self._show_folder_cache: dict = {}   # tmdb_id(str) → 剧名文件夹 DriveFile
        # 季封面缓存：key = "tmdb_id:season_num" → poster_path，避免重复 API 调用
        self._season_poster_cache: dict = {}
        # 字幕组匹配器（内置 + 自定义 custom_release_groups）
        self._rg_matcher = ReleaseGroupsMatcher(
            custom_groups=cfg.parser.custom_release_groups or None
        )
        # Telegram 入库通知收集列表
        self._notify_items: List[_NotifyItem] = []

        # 季详情本地缓存：key=f"{tmdb_id}:{season}" → dict，避免同一季第三次调用 TMDB
        self._season_detail_cache: dict = {}

    # ── 缓存工具 ──────────────────────────────────────────

    def _write_cache(self, path: str, data: dict) -> None:
        """将 TMDB 响应写入 WebUI SQLite 缓存，库不可用时静默跳过。"""
        if self._tmdb_write_cache is None or not data:
            return
        try:
            self._tmdb_write_cache.set(path, data, language=self._cfg.tmdb.language)
        except Exception as e:
            logger.debug("写入 WebUI 缓存失败 %s: %s", path, e)

    def _get_season_detail_cached(self, tmdb_id, season_num: int) -> dict:
        """获取季详情，命中内地缓存则跳过 API；否则技务后写入 SQLite 缓存和季封面缓存。"""
        _key = f"{tmdb_id}:{season_num}"
        if _key not in self._season_detail_cache:
            sd = (self._tmdb.get_season_detail(tmdb_id, season_num)
                  if self._tmdb else None) or {}
            self._season_detail_cache[_key] = sd
            if sd:
                self._write_cache(f"/tv/{tmdb_id}/season/{season_num}", sd)
            self._season_poster_cache.setdefault(_key, sd.get("poster_path") or "")
        return self._season_detail_cache[_key]

    # ── 主入口 ──────────────────────────────────────────────────

    def run(self) -> None:
        scan_folder = self._cfg.drive.scan_folder_id
        if not scan_folder:
            print("❌  配置错误：drive.scan_folder_id 未设置")
            sys.exit(1)
        if not self._cfg.organizer.root_folder_id:
            print("❌  配置错误：organizer.root_folder_id 未设置")
            sys.exit(1)

        print("=" * 68)
        print("  Metadata2GD 开始整理")
        if self._dry_run:
            print("  ⚠️  DRY-RUN 模式 — 不会操作 Drive")
        if self._skip_tmdb or not self._tmdb:
            print("  ℹ️  跳过 TMDB — 不生成 NFO")
        if self._skip_images:
            print("  ℹ️  跳过图片下载")
        print("=" * 68)

        print(f"\n📂 扫描文件夹：{scan_folder}（含子文件夹）")
        videos = [f for f in self._client.list_all_recursive(folder_id=scan_folder) if f.is_video]
        if not videos:
            print("  （未找到视频文件）")
            if scan_folder:
                print("\n🧹 检查并清理空子文件夹...")
                self._cleanup_empty_folders(scan_folder, is_root=True)
                print("  空文件夹清理完毕。")
            return

        print(f"  找到 {len(videos)} 个视频文件\n")

        results = []
        for idx, video in enumerate(videos, 1):
            result = self._process_one(video, idx, total=len(videos))
            results.append(result)

        self._print_summary(results)
        self._send_notifications()

        if scan_folder:
            print("\n🧹 检查并清理空子文件夹...")
            self._cleanup_empty_folders(scan_folder, is_root=True)
            print("  空文件夹清理完毕。")

    def _cleanup_empty_folders(self, folder_id: str, folder_name: str = "", is_root: bool = False) -> bool:
        """
        递归检查子文件夹，如果包含的内容全部移动完毕（即空文件夹），则将其移至回收站。
        """
        if self._dry_run:
            return False

        try:
            items = self._client.list_files(folder_id)
        except Exception as e:
            logger.error("获取文件夹内容失败 [%s]: %s", folder_id, e)
            return False

        is_empty = True
        for item in items:
            if item.is_folder:
                child_empty = self._cleanup_empty_folders(item.id, folder_name=item.name, is_root=False)
                if not child_empty:
                    is_empty = False
            else:
                # 存在普通文件
                is_empty = False

        if is_empty and not is_root:
            try:
                self._client.trash_file(folder_id)
                print(f"      🗑️  清理空文件夹：{folder_name}")
            except Exception as e:
                logger.error("清理空文件夹失败 [%s]: %s", folder_name, e)
                is_empty = False

        return is_empty

    def _process_one(self, video: DriveFile, idx: int, total: int) -> ProcessResult:
        print(f"[{idx}/{total}] 🎬  {video.name}")
        result = ProcessResult(file=video, status="ok")

        # ── Step 2: 解析文件名 ──────────────────────────────
        meta = MetaInfo(video.name, isfile=True,
                       custom_words=self._cfg.parser.custom_words or None,
                       release_group_matcher=self._rg_matcher)
        if not meta.name:
            msg = "解析失败（无法识别标题）"
            print(f"      ⚠️   {msg}")
            result.status = "skipped"
            result.reason = msg
            return result

        is_tv = meta.type == MediaType.TV
        media_label = "TV" if is_tv else "Movie"
        season_num = int(meta.season_seq) if meta.season_seq else 1
        episode_num = int(meta.episode_seq) if meta.episode_seq else None
        season_info = f"  S{season_num:02d}E{episode_num or '?'}" if is_tv else ""
        print(f"      解析：{meta.name} ({meta.year or '?'})  [{media_label}]{season_info}")

        # ── Step 3: TMDB 整剧/整片元数据 ───────────────────
        tmdb_info = None
        if self._tmdb:
            tmdb_info = self._tmdb.recognize(meta)
            if tmdb_info:
                tmdb_title = tmdb_info.get("title") or tmdb_info.get("name") or ""
                tmdb_id = tmdb_info.get("tmdb_id") or tmdb_info.get("id")
                tmdb_year = (tmdb_info.get("release_date") or tmdb_info.get("first_air_date") or "")[:4]
                print(f"      TMDB：{tmdb_title} ({tmdb_year})  tmdb_id={tmdb_id}")
                if is_tv:
                    tmdb_info["_season"] = season_num
                    tmdb_info["_episode"] = episode_num
                # 写入 WebUI 缓存，后端展示时直接命中
                _cache_type = "tv" if is_tv else "movie"
                self._write_cache(f"/{_cache_type}/{tmdb_id}", tmdb_info)
            else:
                print("      TMDB：未找到元数据")
                # 无论是否继续整理，都发 TG 提醒
                self._notify_items.append(_NotifyItem(
                    title=meta.name,
                    year=meta.year or "",
                    is_tv=is_tv,
                    season=season_num,
                    episode=episode_num,
                    ep_title="",
                    poster_path="",
                    tmdb_id="",
                    no_tmdb=True,
                    original_name=video.name,
                ))
                if not self._cfg.pipeline.move_on_tmdb_miss:
                    result.status = "skipped"
                    result.reason = f"TMDB 未找到，move_on_tmdb_miss={str(self._cfg.pipeline.move_on_tmdb_miss).lower()}"
                    return result

        # ── Step 4: TMDB 单集详情（TV 专用）───────────────
        episode_detail = None
        if is_tv and tmdb_info and self._tmdb and episode_num:
            tmdb_id = tmdb_info.get("tmdb_id") or tmdb_info.get("id")
            episode_detail = self._tmdb.get_episode_detail(tmdb_id, season_num, episode_num)
            if episode_detail:
                ep_title = episode_detail.get("name") or ""
                print(f"      单集：{ep_title}  (S{season_num:02d}E{episode_num:02d})")

        # ── Step 5: 更新 Meta 使用 TMDB 数据以便 Organizer 创建正确的中文目录 ──
        if tmdb_info:
            tmdb_title = tmdb_info.get("name") or tmdb_info.get("title") or meta.name
            meta.name = self._safe_filename(tmdb_title)
            tmdb_year = (tmdb_info.get("release_date") or tmdb_info.get("first_air_date") or "")[:4]
            if tmdb_year:
                meta.year = tmdb_year

        # ── Step 5.5: 生成 NFO 内容 ───────────────────────────
        nfo_content: Optional[str] = None
        nfo_name: Optional[str] = None
        clean_name: Optional[str] = None  # 标准化文件名（移动时用）
        if tmdb_info:
            # 计算干净标准化文件名（用于改名 + NFO）
            clean_name = self._build_clean_name(
                tmdb_info=tmdb_info,
                is_tv=is_tv,
                season_num=season_num,
                episode_num=episode_num,
                ext=os.path.splitext(video.name)[1],  # 保留原始扩展名
            )
            nfo_content = self._nfo_gen.generate(
                tmdb_info,
                media_type=meta.type,
                episode_detail=episode_detail,
            )
            # NFO 名和视频文件名相同（仅改后缀）
            nfo_name = os.path.splitext(clean_name)[0] + ".nfo" if clean_name else self._nfo_gen.nfo_name_for(video.name)

        # ── Step 6: 创建目标文件夹 ─────────────────────────
        target_folder: Optional[DriveFile] = None
        top_folder_id: Optional[str] = None   # 剧名文件夹（TV）或电影文件夹
        if not self._dry_run:
            target_folder = self._organizer.ensure_folder_for_meta(meta, label=video.name)
            if not target_folder:
                msg = "文件夹创建失败"
                print(f"      ❌  {msg}")
                result.status = "failed"
                result.reason = msg
                return result
            result.target_folder = target_folder
            if is_tv and tmdb_info:
                # 用 tmdb_id 缓存剧名文件夹 ID，避免每次靠 .parents[0] 反推
                tmdb_id_key = str(tmdb_info.get("tmdb_id") or tmdb_info.get("id") or "")
                if tmdb_id_key and tmdb_id_key in self._show_folder_cache:
                    top_folder_id = self._show_folder_cache[tmdb_id_key]
                elif target_folder.parents:
                    top_folder_id = target_folder.parents[0]
                    if tmdb_id_key:
                        self._show_folder_cache[tmdb_id_key] = top_folder_id
            else:
                top_folder_id = target_folder.id
            folder_path = self._organizer.folder_path_for_meta(meta)
            print(f"      文件夹：{folder_path}  [{target_folder.id[:12]}...]")
        else:
            folder_path = self._organizer.folder_path_for_meta(meta)
            print(f"      文件夹：{folder_path}  [dry-run，不创建]")

        # ── Step 7: 上传单集/电影 NFO ──────────────────────
        if nfo_content and nfo_name:
            if not self._dry_run and target_folder:
                try:
                    self._client.upload_text(
                        content=nfo_content,
                        name=nfo_name,
                        parent_id=target_folder.id,
                        mime_type="text/xml",
                        overwrite=True,
                    )
                    result.nfo_uploaded = True
                    print(f"      NFO：{nfo_name}  ✓")
                except Exception as e:
                    logger.warning("上传单集 NFO 失败：%s", e)
                    print(f"      NFO：上传失败 — {e}")
            else:
                print(f"      NFO：{nfo_name}  [dry-run，不上传]")
        elif not self._skip_tmdb and not tmdb_info:
            print("      NFO：跳过（TMDB 无数据）")

        # ── Step 8: tvshow.nfo → 剧名文件夹 ────────────────
        if is_tv and tmdb_info:
            if not self._dry_run and top_folder_id and top_folder_id not in self._tvshow_nfo_done:
                try:
                    xml = self._nfo_gen.generate_tvshow(tmdb_info)
                    self._client.upload_text(xml, "tvshow.nfo", parent_id=top_folder_id,
                                             mime_type="text/xml", overwrite=True)
                    self._tvshow_nfo_done.add(top_folder_id)
                    print(f"      tvshow.nfo：→ 剧名文件夹  ✓")
                except Exception as e:
                    logger.warning("上传 tvshow.nfo 失败：%s", e)
                    print(f"      tvshow.nfo：失败 — {e}")
            elif top_folder_id and top_folder_id in self._tvshow_nfo_done:
                pass  # 已上传，静默跳过
            elif self._dry_run:
                print(f"      tvshow.nfo：→ 剧名文件夹  [dry-run]")

        # ── Step 9: season.nfo → Season 文件夹 ─────────────
        if is_tv and tmdb_info and target_folder:
            season_folder_id = target_folder.id
            if not self._dry_run and season_folder_id not in self._season_nfo_done:
                try:
                    season_detail = self._get_season_detail_cached(
                        tmdb_info.get("tmdb_id") or tmdb_info.get("id"), season_num
                    )
                    xml = self._nfo_gen.generate_season(season_detail, season_num)
                    self._client.upload_text(xml, "season.nfo", parent_id=season_folder_id,
                                             mime_type="text/xml", overwrite=True)
                    self._season_nfo_done.add(season_folder_id)
                    print(f"      season.nfo：→ Season {season_num} 文件夹  ✓")
                except Exception as e:
                    logger.warning("上传 season.nfo 失败：%s", e)
                    print(f"      season.nfo：失败 — {e}")
            elif season_folder_id in self._season_nfo_done:
                pass  # 已上传，静默跳过
            elif self._dry_run:
                print(f"      season.nfo：→ Season {season_num} 文件夹  [dry-run]")

        # ── Step 10: poster.jpg / fanart.jpg ────────────────
        if tmdb_info and self._img_uploader and target_folder:
            # TV 的 poster/fanart 上传到剧名文件夹（顶层），使用 Step 6 缓存的 top_folder_id
            img_top_id = top_folder_id or target_folder.id

            if img_top_id not in self._poster_done:
                poster = tmdb_info.get("poster_path")
                fanart = tmdb_info.get("backdrop_path")
                uploaded_any = False
                if poster:
                    f = self._img_uploader.upload_poster(poster, img_top_id)
                    if f:
                        print(f"      poster.jpg：✓")
                        uploaded_any = True
                if fanart:
                    f = self._img_uploader.upload_fanart(fanart, img_top_id)
                    if f:
                        print(f"      fanart.jpg：✓")
                        uploaded_any = True
                if uploaded_any:
                    self._poster_done.add(img_top_id)

        elif tmdb_info and not self._img_uploader and not self._skip_images and not self._dry_run:
            pass  # img_uploader 未初始化时静默跳过
        elif tmdb_info and self._dry_run:
            print(f"      poster.jpg / fanart.jpg：[dry-run，不下载]")

        # ── Step 11: season poster（季封面）────────────────
        if is_tv and tmdb_info and self._img_uploader and target_folder and top_folder_id:
            season_poster_key = f"{top_folder_id}:s{season_num}"
            if season_poster_key not in self._season_poster_done:
                _sp_key = f"{tmdb_info.get('tmdb_id') or tmdb_info.get('id')}:{season_num}"
                season_detail_for_img = self._get_season_detail_cached(
                    tmdb_info.get("tmdb_id") or tmdb_info.get("id"), season_num
                )
                sp = season_detail_for_img.get("poster_path")
                if sp:
                    f = self._img_uploader.upload_season_poster(sp, season_num, top_folder_id)
                    if f:
                        print(f"      season{season_num:02d}-poster.jpg：✓")
                self._season_poster_done.add(season_poster_key)

        # ── Step 12: 移动视频文件（同时改名为标准格式）────────
        if not self._dry_run and target_folder:
            target_name = clean_name or video.name
            existing = self._client.find_file(target_name, folder_id=target_folder.id)
            if existing:
                print(f"      移动：跳过（目标位置已存在同名文件）")
            else:
                try:
                    self._client.move_file(
                        video.id,
                        new_folder_id=target_folder.id,
                        new_name=clean_name,  # 同时改名，None 时保持原名
                    )
                    result.moved = True
                    if clean_name and clean_name != video.name:
                        print(f"      移动+改名：{clean_name}  ✓")
                    else:
                        print(f"      移动：✓")
                except Exception as e:
                    logger.error("移动文件失败：%s", e)
                    print(f"      移动：失败 — {e}")
                    result.status = "failed"
                    result.reason = f"移动失败：{e}"
        else:
            target_name = clean_name or video.name
            print(f"      移动：{target_name}  [dry-run，不移动]")

        print()

        # 收集入库通知数据（成功且有 TMDB 元数据时；无 TMDB 的情况已在 miss 分支提前收集）
        if result.status == "ok" and tmdb_info:
            # 获取季封面（优先用于 TG 通知）
            _tmdb_id_for_sp = tmdb_info.get("tmdb_id") or tmdb_info.get("id")
            _sp_key = f"{_tmdb_id_for_sp}:{season_num}"
            _season_poster = self._season_poster_cache.get(_sp_key, "")
            if is_tv and not _season_poster and self._tmdb and _tmdb_id_for_sp:
                _sd = self._tmdb.get_season_detail(_tmdb_id_for_sp, season_num) or {}
                _season_poster = _sd.get("poster_path") or ""
                self._season_poster_cache[_sp_key] = _season_poster
            self._notify_items.append(_NotifyItem(
                title=meta.name,
                year=meta.year or "",
                is_tv=is_tv,
                season=season_num,
                episode=episode_num,
                ep_title=(episode_detail or {}).get("name") or "",
                poster_path=tmdb_info.get("poster_path") or "",
                season_poster_path=_season_poster,
                tmdb_id=str(_tmdb_id_for_sp or ""),
                no_tmdb=False,
            ))

        return result

    @staticmethod
    def _safe_filename(name: str) -> str:
        """ 去掉文件名中的非法字符 """
        import re
        return re.sub(r'[\\/:*?"<>|]', "", name).strip()

    @classmethod
    def _build_clean_name(
        cls,
        tmdb_info: dict,
        is_tv: bool,
        season_num: int,
        episode_num: Optional[int],
        ext: str,
    ) -> str:
        """
        根据 TMDB 数据生成标准化文件名。

        电影： Inception (2010).mkv
        剧集： Breaking Bad - S01E03.mkv
                (所有平台在 Google Drive 模式下准确识别剥集所需的格式)
        """
        if is_tv:
            title = tmdb_info.get("name") or tmdb_info.get("title") or "Unknown"
            ep_str = f"S{season_num:02d}E{episode_num:02d}" if episode_num else f"S{season_num:02d}"
            return f"{cls._safe_filename(title)} - {ep_str}{ext}"
        else:
            title = tmdb_info.get("title") or tmdb_info.get("name") or "Unknown"
            year = (tmdb_info.get("release_date") or "")[:4]
            year_str = f" ({year})" if year else ""
            return f"{cls._safe_filename(title)}{year_str}{ext}"

    @staticmethod
    def _print_summary(results: list) -> None:
        ok = sum(1 for r in results if r.status == "ok")
        skipped = sum(1 for r in results if r.status == "skipped")
        failed = sum(1 for r in results if r.status == "failed")

        print("=" * 68)
        print("  完成")
        print(f"  ✓  成功：{ok}    ⚠  跳过：{skipped}    ✗  失败：{failed}")
        if skipped:
            print("\n  跳过的文件：")
            for r in results:
                if r.status == "skipped":
                    print(f"    - {r.file.name}  （{r.reason}）")
        if failed:
            print("\n  失败的文件：")
            for r in results:
                if r.status == "failed":
                    print(f"    - {r.file.name}  （{r.reason}）")
        print("=" * 68)

    def _send_notifications(self) -> None:
        """按剧/电影分组，发送带封面的 Telegram 入库通知。"""
        tg = self._cfg.telegram
        if not tg.bot_token or not tg.chat_id:
            return
        if not self._notify_items:
            return

        # 按 tmdb_id 分组（同一剧/电影合并为一条消息）
        groups: dict = defaultdict(list)
        poster_map: dict = {}
        for item in self._notify_items:
            key = item.tmdb_id or f"{item.title}_{item.year}"
            groups[key].append(item)
            if key not in poster_map and item.poster_path:
                poster_map[key] = item.poster_path

        for key, items in groups.items():
            first = items[0]
            # 封面选择：TV 单季推送优先用季封面，多季混合或电影用整剧/整片封面
            if first.is_tv:
                seasons_in_group = {it.season for it in items}
                if len(seasons_in_group) == 1:
                    poster = (next((it.season_poster_path for it in items if it.season_poster_path), "")
                              or poster_map.get(key, ""))
                else:
                    poster = poster_map.get(key, "")
            else:
                poster = poster_map.get(key, "")

            if first.no_tmdb:
                # 无 TMDB 数据：发纯文字警告消息（文件未移动，显示原始文件名）
                media_icon = "📺" if first.is_tv else "🎬"
                lines = [f"{media_icon} <b>TMDB 未找到元数据，文件未整理</b>"]
                for it in items:
                    lines.append(f"• <code>{it.original_name or it.title}</code>")
                lines.append("\n请检查文件名后手动触发重新整理")
                caption = "\n".join(lines)
                self._send_tg_photo(tg.bot_token, tg.chat_id, "", caption)
                continue

            if first.is_tv:
                # 剧集：按季分组列出集数
                season_eps: dict = defaultdict(list)
                for it in items:
                    season_eps[it.season].append(it)

                lines = [f"📺 <b>{first.title} ({first.year})</b>"]
                for season in sorted(season_eps):
                    lines.append(f"\nSeason {season:02d}：")
                    for ep in sorted(season_eps[season], key=lambda x: x.episode or 0):
                        ep_str = f"S{ep.season:02d}E{ep.episode:02d}" if ep.episode else f"S{ep.season:02d}"
                        ep_title = f"  {ep.ep_title}" if ep.ep_title else ""
                        lines.append(f"  • {ep_str}{ep_title}")
                caption = "\n".join(lines)
            else:
                # 电影：一条简洁消息
                caption = f"🎬 <b>{first.title} ({first.year})</b>\n已入库"

            self._send_tg_photo(tg.bot_token, tg.chat_id, poster, caption)

    def _send_tg_photo(self, token: str, chat_id: str, poster_path: str, caption: str) -> None:
        """发送带封面图的 Telegram 消息，无封面时降级为纯文字消息。"""
        TMDB_IMG = "https://image.tmdb.org/t/p/w500"
        try:
            if poster_path:
                resp = requests.post(
                    f"https://api.telegram.org/bot{token}/sendPhoto",
                    json={
                        "chat_id": chat_id,
                        "photo": f"{TMDB_IMG}{poster_path}",
                        "caption": caption,
                        "parse_mode": "HTML",
                    },
                    timeout=15,
                )
            else:
                resp = requests.post(
                    f"https://api.telegram.org/bot{token}/sendMessage",
                    json={"chat_id": chat_id, "text": caption, "parse_mode": "HTML"},
                    timeout=10,
                )
            if not resp.json().get("ok"):
                logger.warning("Telegram 通知发送失败：%s", resp.text)
            else:
                logger.info("Telegram 通知已发送")
        except Exception as e:
            logger.warning("Telegram 通知异常：%s", e)


# ─────────────────────────────────────────────────────────────────
# CLI 入口
# ─────────────────────────────────────────────────────────────────

def main():
    # 强制让 Windows 下的输出使用 utf-8 编码，防止部分 Emoji 或特殊字符在 GBK 环境下报错崩溃
    if sys.stdout.encoding.lower() != 'utf-8':
        try:
            sys.stdout.reconfigure(encoding='utf-8')
        except Exception:
            pass

    parser = argparse.ArgumentParser(
        description="Metadata2GD — 扫描 Drive 媒体文件，查询 TMDB 元数据，生成 NFO，整理到目标文件夹",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
流程：
  扫描 Drive 源文件夹 → 解析文件名 → TMDB 查询（整剧+单集）→ 生成 NFO
  → 创建文件夹 → 上传 NFO + tvshow.nfo + season.nfo → 下载图片 → 移动文件

示例：
  python pipeline.py                  # 正式运行
  python pipeline.py --dry-run        # 预览计划，不操作 Drive
  python pipeline.py --no-tmdb        # 只整理文件夹，不查 TMDB/生成 NFO
  python pipeline.py --no-images      # 跳过图片下载上传
""",
    )
    parser.add_argument("--dry-run", action="store_true", help="只打印计划，不实际操作 Google Drive")
    parser.add_argument("--no-tmdb", action="store_true", help="跳过 TMDB 查询（不生成 NFO，只整理文件夹）")
    parser.add_argument("--no-images", action="store_true", help="跳过图片下载上传（poster/fanart）")
    parser.add_argument("--config", default=None, metavar="PATH", help="配置文件路径（默认自动查找 config/config.yaml）")
    parser.add_argument("--verbose", "-v", action="store_true", help="输出详细日志")
    args = parser.parse_args()

    if args.verbose:
        logging.getLogger().setLevel(logging.DEBUG)

    cfg = Config.load(args.config)

    drive_cfg = cfg.drive
    try:
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

    pipe = Pipeline(
        client=client,
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

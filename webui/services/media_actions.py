import logging
import re
from collections import defaultdict
from typing import Any, Set

from fastapi import HTTPException
from mediaparser import TmdbClient
from nfo import ImageUploader, NfoGenerator

from webui.core.app_logging import app_log
from webui.core.runtime import get_config, get_storage_provider, logger
from webui.library_store import get_library_store
from webui.services.library_data import build_seasons_status, fill_seasons_episodes, parse_episode_from_filename
from webui.services.tmdb_service import get_tmdb_cache, serialize_tmdb_result, tmdb_get, tmdb_image_url

try:
    from scraper.core.factory import SpiderFactory
except ImportError:
    SpiderFactory = None


def _movie_folder_name(title: str, year: str) -> str:
    normalized_title = str(title or "").strip()
    normalized_year = str(year or "").strip()
    if normalized_title and normalized_year:
        return f"{normalized_title} ({normalized_year})"
    return normalized_title


def _fetch_tmdb_info(tmdb_id: int, media_type: str) -> dict[str, Any]:
    def _fetch_with_credits(path: str, extra_keys: str) -> dict[str, Any]:
        info = tmdb_get(path, {"append_to_response": extra_keys}, use_cache=False)
        if not info:
            logger.info("append_to_response 失败，回退到基础请求：%s", path)
            info = tmdb_get(path, use_cache=False)
        if not info:
            return {}
        if not info.get("credits"):
            credits = tmdb_get(f"{path}/credits", use_cache=False)
            if credits:
                info["credits"] = credits
        return info

    if media_type == "movie":
        info = _fetch_with_credits(f"/movie/{tmdb_id}", "credits,external_ids,release_dates")
        if not info:
            raise ValueError(f"TMDB 未找到电影 tmdb_id={tmdb_id}，请稍候重试")
        info["tmdb_id"] = tmdb_id
        credits = info.get("credits") or {}
        info["directors"] = [
            {"id": p["id"], "name": p["name"], "profile_path": p.get("profile_path")}
            for p in (credits.get("crew") or [])
            if p.get("job") == "Director"
        ]
        info["actors"] = [
            {"id": p["id"], "name": p["name"], "character": p.get("character"), "profile_path": p.get("profile_path")}
            for p in (credits.get("cast") or [])[:20]
        ]
        return info

    if media_type == "tv":
        info = _fetch_with_credits(f"/tv/{tmdb_id}", "credits,external_ids,content_ratings")
        if not info:
            raise ValueError(f"TMDB 未找到剧集 tmdb_id={tmdb_id}，请稍候重试")
        info["tmdb_id"] = tmdb_id
        credits = info.get("credits") or {}
        info["directors"] = [
            {"id": p["id"], "name": p["name"], "profile_path": p.get("profile_path")}
            for p in (credits.get("crew") or [])
            if p.get("job") in ("Director", "Executive Producer")
        ][:5]
        info["actors"] = [
            {"id": p["id"], "name": p["name"], "character": p.get("character"), "profile_path": p.get("profile_path")}
            for p in (credits.get("cast") or [])[:20]
        ]
        return info

    raise ValueError(f"不支持的 media_type: {media_type}")


def _build_item_updates(info: dict[str, Any], media_type: str, drive_folder_id: str, client) -> dict[str, Any]:
    tmdb_id = int(info.get("tmdb_id") or info.get("id") or 0)
    updates = {
        "tmdb_id": tmdb_id,
        "media_type": media_type,
        "title": info.get("name") if media_type == "tv" else info.get("title"),
        "original_title": info.get("original_name") if media_type == "tv" else info.get("original_title"),
        "overview": info.get("overview") or "",
        "rating": round(info.get("vote_average") or 0, 1),
    }
    if info.get("poster_path"):
        updates["poster_url"] = tmdb_image_url(info["poster_path"], size="w500")
    if info.get("backdrop_path"):
        updates["backdrop_url"] = tmdb_image_url(info["backdrop_path"])
    if media_type == "tv":
        updates["year"] = (info.get("first_air_date") or "")[:4]
        updates["status"] = info.get("status") or ""
        if info.get("number_of_episodes") is not None:
            updates["total_episodes"] = info.get("number_of_episodes")
        if info.get("seasons"):
            drive_episodes: Set[tuple] = set()
            try:
                season_folders = [
                    f
                    for f in client.list_files(folder_id=drive_folder_id, page_size=200)
                    if f.is_folder and re.match(r"Season\s*\d+", f.name, re.IGNORECASE)
                ]
                for season_folder in season_folders:
                    match = re.search(r"(\d+)", season_folder.name)
                    if not match:
                        continue
                    season_num = int(match.group(1))
                    season_files = client.list_files(folder_id=season_folder.id, page_size=500)
                    for file in season_files:
                        if file.is_video:
                            episode = parse_episode_from_filename(file.name)
                            if episode and episode[0] == season_num:
                                drive_episodes.add((episode[0], episode[1]))
            except Exception as exc:
                logger.warning("扫描季文件夹失败，入库状态可能不准确: %s", exc)
            seasons_status, total_eps, in_lib_eps = build_seasons_status(
                tmdb_id,
                info,
                drive_episodes,
                tmdb_use_cache=False,
            )
            if seasons_status:
                updates["seasons"] = [s.model_dump() for s in seasons_status]
            if total_eps:
                updates["total_episodes"] = total_eps
            updates["in_library_episodes"] = in_lib_eps
    else:
        updates["year"] = (info.get("release_date") or "")[:4]
    return updates


def _persist_library_item(drive_folder_id: str, result: dict[str, Any]) -> dict[str, Any] | None:
    updates = result.get("updates", {})
    if not updates and result.get("tmdb_id"):
        updates = {"tmdb_id": result.get("tmdb_id")}
    if not updates or not drive_folder_id:
        return None
    store = get_library_store()
    try:
        store.patch_item(drive_folder_id, updates)
    except Exception as exc:
        logger.warning("patch_item 失败（不影响刷新结果）: %s", exc)
        return None
    return store.get_library_item_by_folder_id(drive_folder_id)


def _rename_library_folder(drive_folder_id: str, media_type: str, updates: dict[str, Any], client) -> dict[str, Any]:
    target_name = _movie_folder_name(str(updates.get("title") or ""), str(updates.get("year") or ""))
    if not target_name:
        raise ValueError("纠错后的媒体标题为空，无法重命名目录")

    # 某些 Provider（尤其 115）并不支持按 file_id 稳定 get_file，
    # 这里不把 get_file 作为重命名的前置依赖，避免误报“未找到文件”。
    current_name = ""
    try:
        current_folder = client.get_file(drive_folder_id)
        current_name = str(getattr(current_folder, "name", "") or "")
        if current_name == target_name:
            return {"renamed": False, "folder_name": current_name}
    except Exception as exc:
        logger.debug("重命名前读取当前目录名失败，将直接尝试 rename: %s", exc)

    renamed_folder = client.rename_file(drive_folder_id, target_name)
    renamed_name = str(getattr(renamed_folder, "name", "") or target_name)
    if current_name:
        logger.info("已按新 TMDB 信息重命名目录：%s -> %s [%s]", current_name, renamed_name, media_type)
    else:
        logger.info("已按新 TMDB 信息重命名目录：%s [%s]", renamed_name, media_type)
    return {"renamed": renamed_name != current_name, "folder_name": renamed_name}


def do_refresh_item(tmdb_id: int, media_type: str, drive_folder_id: str, title: str | None = None, year: str | None = None) -> dict:
    cfg = get_config()
    skip_metadata_upload = cfg.pipeline.skip_metadata_upload
    if not tmdb_id or tmdb_id <= 0:
        if not title:
            raise ValueError("该媒体项没有 TMDB ID 且未提供标题，无法搜索")
        if not cfg.is_tmdb_ready():
            raise ValueError("TMDB API Key 未配置，无法搜索")
        from mediaparser.types import MediaType as MType

        mtype = MType.TV if media_type == "tv" else MType.MOVIE
        tmdb_client = TmdbClient(
            api_key=cfg.tmdb.api_key,
            language=cfg.tmdb.language,
            proxy=cfg.tmdb_proxy,
            timeout=cfg.tmdb.timeout,
        )
        logger.info("tmdb_id=0，尝试按名称搜索：%r (%s) [%s]", title, year, media_type)
        found = tmdb_client._search_by_name(title, year, mtype)
        if not found:
            raise ValueError(f"TMDB 搜索无结果：{title!r}（{media_type}），请先手动确认 TMDB ID")
        tmdb_id = found.get("tmdb_id") or found.get("id")
        logger.info("按名称找到 tmdb_id=%s：%s", tmdb_id, found.get("name") or found.get("title"))

    client = get_storage_provider()
    gen = NfoGenerator(tmdb_image_base_url=cfg.tmdb_image_base_url)
    uploader = ImageUploader(client, overwrite=True, tmdb_image_base_url=cfg.tmdb_image_base_url)
    uploaded: list[str] = []
    errors: list[str] = []

    info = _fetch_tmdb_info(int(tmdb_id), media_type)
    if media_type == "movie":
        if not skip_metadata_upload:
            try:
                folder_files = client.list_files(folder_id=drive_folder_id, page_size=50)
                video_files = [f for f in folder_files if f.is_video]
            except Exception as exc:
                logger.warning("列出文件夹内容失败，将跳过视频同名 NFO: %s", exc)
                video_files = []
            if video_files:
                for video_file in video_files:
                    nfo_name = gen.nfo_name_for(video_file.name)
                    try:
                        xml = gen.generate(info, media_type=None)
                        client.upload_text(xml, nfo_name, parent_id=drive_folder_id, mime_type="text/xml", overwrite=True)
                        uploaded.append(nfo_name)
                    except Exception as exc:
                        errors.append(f"{nfo_name}: {exc}")
                        logger.warning("上传 NFO 失败: %s - %s", nfo_name, exc)
            else:
                try:
                    xml = gen.generate(info, media_type=None)
                    client.upload_text(xml, "movie.nfo", parent_id=drive_folder_id, mime_type="text/xml", overwrite=True)
                    uploaded.append("movie.nfo")
                except Exception as exc:
                    errors.append(f"movie.nfo: {exc}")
            if info.get("poster_path"):
                try:
                    uploader.upload_poster(info["poster_path"], drive_folder_id)
                    uploaded.append("poster.jpg")
                except Exception as exc:
                    errors.append(f"poster.jpg: {exc}")
            if info.get("backdrop_path"):
                try:
                    uploader.upload_fanart(info["backdrop_path"], drive_folder_id)
                    uploaded.append("fanart.jpg")
                except Exception as exc:
                    errors.append(f"fanart.jpg: {exc}")
    elif media_type == "tv":
        try:
            season_folders = [
                f
                for f in client.list_files(folder_id=drive_folder_id, page_size=200)
                if f.is_folder and re.match(r"Season\s*\d+", f.name, re.IGNORECASE)
            ]
        except Exception as exc:
            logger.warning("列出季文件夹失败，季状态可能不准确: %s", exc)
            season_folders = []

        if not skip_metadata_upload:
            try:
                xml = gen.generate_tvshow(info)
                client.upload_text(xml, "tvshow.nfo", parent_id=drive_folder_id, mime_type="text/xml", overwrite=True)
                uploaded.append("tvshow.nfo")
            except Exception as exc:
                errors.append(f"tvshow.nfo: {exc}")
            if info.get("poster_path"):
                try:
                    uploader.upload_poster(info["poster_path"], drive_folder_id)
                    uploaded.append("poster.jpg")
                except Exception as exc:
                    errors.append(f"poster.jpg: {exc}")
            if info.get("backdrop_path"):
                try:
                    uploader.upload_fanart(info["backdrop_path"], drive_folder_id)
                    uploaded.append("fanart.jpg")
                except Exception as exc:
                    errors.append(f"fanart.jpg: {exc}")
            for season_folder in season_folders:
                match = re.search(r"(\d+)", season_folder.name)
                if not match:
                    continue
                season_num = int(match.group(1))
                season_detail = tmdb_get(f"/tv/{tmdb_id}/season/{season_num}", use_cache=False)
                if not season_detail:
                    logger.info("跳过 Season %d（TMDB 无数据）", season_num)
                    continue
                try:
                    xml = gen.generate_season(season_detail, season_num)
                    client.upload_text(xml, "season.nfo", parent_id=season_folder.id, mime_type="text/xml", overwrite=True)
                    uploaded.append(f"Season {season_num}/season.nfo")
                except Exception as exc:
                    errors.append(f"Season {season_num}/season.nfo: {exc}")
                if season_detail.get("poster_path"):
                    try:
                        uploader.upload_season_poster(season_detail["poster_path"], season_num, drive_folder_id)
                        uploaded.append(f"season{season_num:02d}-poster.jpg")
                    except Exception as exc:
                        errors.append(f"season{season_num:02d}-poster.jpg: {exc}")
    updates = _build_item_updates(info, media_type, drive_folder_id, client)
    return {"ok": len(errors) == 0, "uploaded": uploaded, "errors": errors, "tmdb_id": tmdb_id, "updates": updates}


def refresh_item_payload(body) -> dict:
    if not body.drive_folder_id:
        raise HTTPException(status_code=400, detail="drive_folder_id 不能为空")
    try:
        result = do_refresh_item(body.tmdb_id, body.media_type, body.drive_folder_id, title=body.title, year=body.year)
        app_log(
            "library",
            "refresh_item",
            f"已刷新元数据：tmdb_id={body.tmdb_id} ({body.media_type})",
            level="SUCCESS" if result["ok"] else "WARNING",
            details={
                "tmdb_id": body.tmdb_id,
                "media_type": body.media_type,
                "drive_folder_id": body.drive_folder_id,
                "uploaded": result["uploaded"],
                "errors": result["errors"],
            },
        )
        item = _persist_library_item(body.drive_folder_id, result)
        if item is not None:
            result["item"] = item
        return result
    except ValueError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except HTTPException:
        raise
    except Exception as exc:
        logger.error(f"刷新单条目失败 (tmdb_id={body.tmdb_id}): {exc}")
        app_log(
            "library",
            "refresh_item_failed",
            f"刷新元数据失败：{exc}",
            level="ERROR",
            details={"tmdb_id": body.tmdb_id, "error": str(exc)},
        )
        raise HTTPException(status_code=500, detail=str(exc)) from exc


def reidentify_item_payload(body) -> dict:
    if not body.drive_folder_id:
        raise HTTPException(status_code=400, detail="drive_folder_id 不能为空")
    if not body.tmdb_id or body.tmdb_id <= 0:
        raise HTTPException(status_code=400, detail="tmdb_id 无效")
    try:
        result = do_refresh_item(body.tmdb_id, body.media_type, body.drive_folder_id, title=body.title, year=body.year)
        item = _persist_library_item(body.drive_folder_id, result)
        if item is not None:
            result["item"] = item

        rename_error = ""
        renamed = False
        folder_name = ""
        if body.rename_folder:
            try:
                rename_result = _rename_library_folder(
                    body.drive_folder_id,
                    body.media_type,
                    result.get("updates") or {},
                    get_storage_provider(),
                )
                renamed = bool(rename_result.get("renamed"))
                folder_name = str(rename_result.get("folder_name") or "")
            except Exception as exc:
                rename_error = str(exc)
                logger.warning("纠错后目录重命名失败 drive_folder_id=%s: %s", body.drive_folder_id, exc)

        result["renamed"] = renamed
        if folder_name:
            result["folder_name"] = folder_name
        if rename_error:
            result["rename_errors"] = [rename_error]
        result["partial"] = bool(result["errors"] or rename_error)
        result["ok"] = not result["partial"]

        app_log(
            "library",
            "reidentify_item",
            f"已修正识别：tmdb_id={body.tmdb_id} ({body.media_type})",
            level="SUCCESS" if result["ok"] else "WARNING",
            details={
                "tmdb_id": body.tmdb_id,
                "media_type": body.media_type,
                "drive_folder_id": body.drive_folder_id,
                "uploaded": result["uploaded"],
                "errors": result["errors"],
                "renamed": renamed,
                "rename_errors": result.get("rename_errors") or [],
            },
        )
        return result
    except ValueError as exc:
        raise HTTPException(status_code=404, detail=str(exc)) from exc
    except HTTPException:
        raise
    except Exception as exc:
        logger.error("修正识别失败 (tmdb_id=%s): %s", body.tmdb_id, exc)
        app_log(
            "library",
            "reidentify_item_failed",
            f"修正识别失败：{exc}",
            level="ERROR",
            details={"tmdb_id": body.tmdb_id, "error": str(exc)},
        )
        raise HTTPException(status_code=500, detail=str(exc)) from exc


def cache_stats_payload():
    return get_tmdb_cache().stats()


def cache_evict_payload():
    count = get_tmdb_cache().evict_expired()
    return {"evicted": count, "message": f"已清理 {count} 条过期缓存"}


def health_payload():
    return {"status": "ok"}


def tmdb_search_multi_payload(keyword: str):
    cfg = get_config()
    if not cfg.tmdb.api_key:
        raise HTTPException(status_code=400, detail="TMDB API Key 未配置")
    tmdb_client = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
        cache=get_tmdb_cache(),
    )
    try:
        raw_results = tmdb_client.search_raw_multi(keyword)
        serialized = []
        for item in raw_results:
            if item.get("media_type") not in ("movie", "tv"):
                continue
            serialized_item = serialize_tmdb_result(item)
            if serialized_item:
                serialized.append(serialized_item)
        return {"ok": True, "results": serialized}
    except Exception as exc:
        logger.error("TMDB 搜索失败: %s", exc)
        raise HTTPException(status_code=500, detail=str(exc)) from exc


def tmdb_detail_payload(tmdb_id: int, media_type: str):
    # 参数验证
    if not tmdb_id or tmdb_id <= 0:
        raise HTTPException(status_code=400, detail="tmdb_id 无效")
    if media_type not in ("movie", "tv"):
        raise HTTPException(status_code=400, detail=f"media_type 必须是 movie 或 tv，当前值：{media_type}")

    try:
        store = get_library_store()
        joined = store.get_joined_media_item(media_type, tmdb_id)
        # TV 类型必须有 seasons，否则缓存不完整，需要重新获取
        if joined and (media_type != "tv" or joined.get("seasons")):
            # 如果是 TV 且 seasons 没有 episodes 详情，需要补充
            if media_type == "tv" and joined.get("seasons"):
                needs_detail = False
                expected_total = int(joined.get("total_episodes") or 0)
                seasons_episode_total = 0
                for s in joined["seasons"]:
                    seasons_episode_total += int(s.get("episode_count") or len(s.get("episodes") or []) or 0)
                    if not s.get("episodes") or len(s["episodes"]) != s.get("episode_count", 0):
                        needs_detail = True
                        break
                if expected_total and seasons_episode_total and seasons_episode_total != expected_total:
                    needs_detail = True
                if needs_detail:
                    joined["seasons"] = fill_seasons_episodes(tmdb_id, joined["seasons"], tmdb_use_cache=False)
            return {"ok": True, "detail": joined}
        cfg = get_config()
        if not cfg.tmdb or not cfg.tmdb.api_key:
            raise ValueError("TMDB API Key 未配置")
        tmdb_client = TmdbClient(
            api_key=cfg.tmdb.api_key,
            language=cfg.tmdb.language,
            proxy=cfg.tmdb_proxy,
            timeout=cfg.tmdb.timeout,
            cache=get_tmdb_cache(),
        )
        raw = tmdb_client._get_movie_detail(tmdb_id) if media_type == "movie" else tmdb_client._get_tv_detail(tmdb_id)
        serialized = serialize_tmdb_result(raw)
        if not serialized:
            raise ValueError("解析详情失败")
        # TV 类型需要补充每季每集详情
        if media_type == "tv" and raw.get("seasons"):
            seasons_status, total_eps, _ = build_seasons_status(tmdb_id, raw)
            serialized["seasons"] = [s.model_dump() for s in seasons_status]
        serialized["in_library"] = False
        return {"ok": True, "detail": serialized}
    except Exception as exc:
        logger.error("TMDB 详情获取失败: %s", exc)
        raise HTTPException(status_code=500, detail=str(exc)) from exc


def tmdb_alternative_names_payload(tmdb_id: int, media_type: str):
    try:
        cfg = get_config()
        if not cfg.tmdb or not cfg.tmdb.api_key:
            raise ValueError("TMDB API Key 未配置")
        tmdb_client = TmdbClient(
            api_key=cfg.tmdb.api_key,
            language=cfg.tmdb.language,
            proxy=cfg.tmdb_proxy,
            timeout=cfg.tmdb.timeout,
            cache=get_tmdb_cache(),
        )
        media_path = "movie" if media_type == "movie" else "tv"
        data = tmdb_client._get(f"/{media_path}/{tmdb_id}/alternative_titles", use_cache=False)
        if not data:
            return {"ok": True, "alternative_names": []}
        raw_titles = data.get("titles") or data.get("results") or []
        lang_map = {"CN": "zh", "TW": "zh", "HK": "zh", "SG": "zh", "JP": "ja", "US": "en", "GB": "en"}
        seen: set[str] = set()
        result = []
        for item in raw_titles:
            name = item.get("title") or ""
            iso_3166 = (item.get("iso_3166_1") or "").upper()
            iso_639 = lang_map.get(iso_3166, iso_3166.lower()[:2] if iso_3166 else "")
            if name and name not in seen:
                seen.add(name)
                result.append({"name": name, "iso_639_1": iso_639})
        return {"ok": True, "alternative_names": result}
    except Exception as exc:
        logger.error("TMDB 别名获取失败: %s", exc)
        raise HTTPException(status_code=500, detail=str(exc)) from exc


def scraper_search_media_payload(keyword: str):
    if SpiderFactory is None:
        raise HTTPException(status_code=500, detail="Scraper module not loaded")
    results = SpiderFactory.search_all(keyword)
    grouped = defaultdict(list)
    for result in results:
        grouped[result.name].append(
            {
                "site": result.site,
                "media_id": result.media_id,
                "url": result.url,
                "cover_image": result.cover_image,
                "subgroup_id": getattr(result, "subgroup_id", None),
                "subgroup_name": getattr(result, "subgroup_name", None),
                "rss_url": getattr(result, "rss_url", None),
            }
        )
    aggregate_results = []
    for name, sources in grouped.items():
        cover_image = next((source["cover_image"] for source in sources if source["cover_image"]), None)
        aggregate_results.append({"name": name, "cover_image": cover_image, "sources": sources})
    return {"ok": True, "results": aggregate_results}


def scraper_sites_payload():
    if SpiderFactory is None:
        raise HTTPException(status_code=500, detail="Scraper module not loaded")
    return {"ok": True, "sites": SpiderFactory.list_sites()}


def scraper_get_episodes_payload(site: str, media_id: str, subgroup_id: str | None = None):
    if SpiderFactory is None:
        raise HTTPException(status_code=500, detail="Scraper module not loaded")
    try:
        spider = SpiderFactory.get_spider(site)
        episodes = spider.get_episodes(media_id, subgroup_id)
        return {"ok": True, "episodes": [episode.model_dump() for episode in episodes]}
    except ValueError as exc:
        raise HTTPException(status_code=400, detail=str(exc)) from exc
    except Exception as exc:
        logging.getLogger("scraper").warning(f"get_episodes failed for {site}/{media_id}/{subgroup_id}: {exc}")
        return {"ok": False, "episodes": [], "error": str(exc)}

"""
将本地媒体库 (library_media) 的所有条目按集数粒度同步写入入库记录 (ingest_history)。
- 电影：1条记录/部
- 剧集：1条记录/集（仅 in_library 的集数）
时间使用当前时间。
"""

import json
import os
import sqlite3
import sys
from datetime import datetime, timezone


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat()


def get_poster_path(raw: dict, conn: sqlite3.Connection, media_type: str, tmdb_id: int, season_num: int = None) -> str:
    """从 raw_json 或 tmdb_media 中获取 poster_path"""
    # 剧集：优先用季封面
    if season_num and raw.get("seasons"):
        for s in raw["seasons"]:
            if s.get("season_number") == season_num:
                sp = s.get("poster_path") or ""
                if sp:
                    return sp
                break
    # 其次用 show/movie 自身的 poster_path
    pp = raw.get("poster_path") or ""
    if pp:
        return pp
    # 从 tmdb_media 表中查
    row = conn.execute(
        "SELECT poster_path FROM tmdb_media WHERE media_type = ? AND tmdb_id = ?",
        (media_type, tmdb_id),
    ).fetchone()
    return row["poster_path"] if row else ""


def main():
    root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    db_path = os.path.join(root, "config", "data", "library.db")

    if not os.path.exists(db_path):
        print(f"数据库不存在: {db_path}")
        sys.exit(1)

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row

    # 删除之前脚本写入的 show 级别记录（无 season/episode 的）
    deleted = conn.execute(
        "DELETE FROM ingest_history WHERE season IS NULL AND episode IS NULL"
    ).rowcount
    if deleted:
        conn.commit()
        print(f"已清除 {deleted} 条旧的 show 级别记录")

    rows = conn.execute(
        "SELECT * FROM library_media WHERE in_library = 1 ORDER BY media_type, year DESC, title COLLATE NOCASE"
    ).fetchall()

    print(f"共找到 {len(rows)} 条媒体库记录")

    now = utc_now()
    count = 0
    skipped = 0

    for row in rows:
        raw = json.loads(row["raw_json"] or "{}")

        media_type = row["media_type"] or raw.get("media_type") or ""
        tmdb_id = int(row["tmdb_id"] or raw.get("tmdb_id") or 0)
        title = row["title"] or raw.get("title") or raw.get("name") or ""
        original_title = row["original_title"] or raw.get("original_title") or raw.get("original_name") or ""
        year = row["year"] or raw.get("year") or ""
        drive_folder_id = row["drive_folder_id"] or raw.get("drive_folder_id") or ""

        if media_type == "movie":
            # 检查是否已有
            if drive_folder_id:
                existing = conn.execute(
                    "SELECT id FROM ingest_history WHERE drive_folder_id = ?",
                    (drive_folder_id,),
                ).fetchone()
            else:
                existing = conn.execute(
                    "SELECT id FROM ingest_history WHERE media_type = 'movie' AND tmdb_id = ?",
                    (tmdb_id,),
                ).fetchone() if tmdb_id > 0 else None

            if existing:
                skipped += 1
                continue

            poster_path = get_poster_path(raw, conn, media_type, tmdb_id)
            conn.execute(
                """
                INSERT INTO ingest_history(
                    media_type, tmdb_id, title, original_title, year,
                    season, episode, episode_title, poster_path,
                    drive_folder_id, original_name, status, error_message,
                    ingested_at, created_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    media_type, tmdb_id, title, original_title, year,
                    None, None, "", poster_path,
                    drive_folder_id, "", "success", "", now, now,
                ),
            )
            count += 1

        elif media_type == "tv":
            seasons = raw.get("seasons", [])
            for season in seasons:
                season_num = season.get("season_number")
                if season_num is None:
                    continue
                episodes = season.get("episodes", [])
                for ep in episodes:
                    if not ep.get("in_library"):
                        continue
                    ep_num = ep.get("episode_number")
                    ep_title = ep.get("episode_title") or ""
                    ep_drive_id = ep.get("drive_folder_id") or drive_folder_id

                    # 按 drive_folder_id 去重
                    if ep_drive_id:
                        existing = conn.execute(
                            "SELECT id FROM ingest_history WHERE drive_folder_id = ? AND season = ? AND episode = ?",
                            (ep_drive_id, season_num, ep_num),
                        ).fetchone()
                    else:
                        existing = conn.execute(
                            "SELECT id FROM ingest_history WHERE media_type = 'tv' AND tmdb_id = ? AND season = ? AND episode = ?",
                            (tmdb_id, season_num, ep_num),
                        ).fetchone() if tmdb_id > 0 else None

                    if existing:
                        skipped += 1
                        continue

                    poster_path = get_poster_path(raw, conn, "tv", tmdb_id, season_num)
                    conn.execute(
                        """
                        INSERT INTO ingest_history(
                            media_type, tmdb_id, title, original_title, year,
                            season, episode, episode_title, poster_path,
                            drive_folder_id, original_name, status, error_message,
                            ingested_at, created_at
                        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                        """,
                        (
                            media_type, tmdb_id, title, original_title, year,
                            season_num, ep_num, ep_title, poster_path,
                            ep_drive_id, "", "success", "", now, now,
                        ),
                    )
                    count += 1

    conn.commit()
    conn.close()

    print(f"已写入 {count} 条入库记录，跳过 {skipped} 条（已存在）")


if __name__ == "__main__":
    main()

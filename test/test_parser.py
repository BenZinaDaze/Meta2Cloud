#!/usr/bin/env python3
"""
mediaparser 完整测试：文件名解析 + TMDB 元数据获取
配置从项目根目录的 config.yaml 读取。
"""
import sys
sys.path.insert(0, "/home/benz1/Code/github/Metadata2GD")

from mediaparser import MetaInfo, TmdbClient, Config
from mediaparser.release_group import ReleaseGroupsMatcher

# ── 加载配置 ───────────────────────────────────────
cfg = Config.load()
rg_matcher = ReleaseGroupsMatcher(custom_groups=cfg.parser.custom_release_groups or None)

# ── 第一部分：纯解析测试 ────────────────────────────────
TEST_CASES = [
    ("【FSD】东岛丹三郎想成为假面骑士[02][我是塔克尔].mp4", "Seihantai"),
]

print("=" * 70)
print("【解析测试】")
print("=" * 70)
for title, expected in TEST_CASES:
    meta = MetaInfo(
        title,
        isfile=True,
        custom_words=cfg.parser.custom_words,
        release_group_matcher=rg_matcher,
    )
    ok = expected.lower() in (meta.name or "").lower()
    status = "✓" if ok else "✗"
    print(f"{status} {title[:55]:<55}")
    print(f"   名称={meta.name!r}  类型={meta.type.value}  "
          f"季={meta.season!r}  集={meta.episode!r}  "
          f"分辨率={meta.resource_pix!r}  字幕组={meta.resource_team!r}")
    print()

# ── 第二部分：TMDB 查询测试 ────────────────────────────
TMDB_TESTS = [
    "【FSD】东岛丹三郎想成为假面骑士[02][我是塔克尔].mp4",
]

if not cfg.is_tmdb_ready():
    print("=" * 70)
    print("【TMDB 查询】跳过（config.yaml 中 tmdb.api_key 未填写）")
    print("=" * 70)
else:
    tmdb = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
    )
    print("=" * 70)
    print("【TMDB 查询测试】")
    print("=" * 70)
    for title in TMDB_TESTS:
        meta = MetaInfo(title, isfile=True, custom_words=cfg.parser.custom_words)
        print(f"\n→ 查询：{meta.name!r}（{meta.type.value}，年份={meta.year}）")
        info = tmdb.recognize(meta)
        if info:
            name = info.get("title") or info.get("name", "")
            orig = info.get("original_title") or info.get("original_name", "")
            tmdbid = info.get("tmdb_id") or info.get("id")
            overview = (info.get("overview") or "")[:80]
            genres = [g["name"] for g in (info.get("genres") or [])]
            vote = info.get("vote_average")
            poster = TmdbClient.image_url(info.get("poster_path"))
            print(f"  ✓ {name} ({orig})")
            print(f"    TMDB ID : {tmdbid}")
            print(f"    风格    : {', '.join(genres)}")
            print(f"    评分    : {vote}")
            print(f"    简介    : {overview}...")
            print(f"    海报    : {poster}")
        else:
            print("  ✗ 未找到")
    print()
    print("=" * 70)

# ── 第三部分：封面推送测试 ─────────────────────────────────────────
if not cfg.is_tmdb_ready():
    print("=" * 70)
    print("【封面推送测试】跳过（tmdb.api_key 未填写）")
    print("=" * 70)
else:
    print("=" * 70)
    print("【封面推送测试】整剧封面 vs 季封面")
    print("=" * 70)

    tmdb2 = TmdbClient(
        api_key=cfg.tmdb.api_key,
        language=cfg.tmdb.language,
        proxy=cfg.tmdb_proxy,
        timeout=cfg.tmdb.timeout,
    )

    for title in TMDB_TESTS:
        meta = MetaInfo(title, isfile=True, custom_words=cfg.parser.custom_words)
        season_num = int(meta.season_seq) if meta.season_seq else 1
        print(f"\n→ {meta.name!r}  S{season_num:02d}")

        info = tmdb2.recognize(meta)
        if not info:
            print("  ✗ TMDB 未找到，跳过")
            continue

        tmdb_id = info.get("tmdb_id") or info.get("id")
        show_poster = TmdbClient.image_url(info.get("poster_path"))

        # 查季封面
        season_detail = tmdb2.get_season_detail(tmdb_id, season_num) or {}
        season_poster_path = season_detail.get("poster_path") or ""
        season_poster = TmdbClient.image_url(season_poster_path) if season_poster_path else "（无）"

        # 推送选择逻辑（与 pipeline._send_notifications 一致）
        notify_poster = TmdbClient.image_url(season_poster_path) if season_poster_path else show_poster

        print(f"  整剧封面 : {show_poster}")
        print(f"  季  封面 : {season_poster}")
        print(f"  TG 推送  : {notify_poster}  ← {'季封面 ✓' if season_poster_path else '整剧封面（无季封面）'}")

    print()
    print("=" * 70)

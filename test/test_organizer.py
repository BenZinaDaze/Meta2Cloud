#!/usr/bin/env python3
"""
test_organizer.py —— MediaOrganizer 集成测试

测试覆盖：
  ✓ folder_path_for()     — 纯路径计算（无 Drive 操作）
  ✓ ensure_folder_for()   — Drive 文件夹创建（dry_run 模式）
  ✓ ensure_folder_for()   — Drive 文件夹真实创建（可选）

使用：
  python test_organizer.py           # dry_run 模式，不创建 Drive 文件夹
  python test_organizer.py --real    # 真实创建，需要配置好 Drive 认证
"""

import sys
import os
import logging
import argparse

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from organizer import MediaOrganizer
from drive import DriveClient

# ── 日志配置 ──────────────────────────────────────────────────
logging.basicConfig(
    level=logging.INFO,
    format="%(levelname)-8s %(name)s: %(message)s",
)
logger = logging.getLogger("test_organizer")

# ── 认证配置（与 test_drive.py 一致） ─────────────────────────
CONFIG = {
    "auth_mode": "oauth2",
    "credentials_json": "config/credentials.json",
    "token_json": "config/token.json",
    "test_folder_id": "15MrQz5ukLAAYOXzseE1s9CmFBGDvVA4x",
}

# ── 测试样本 ─────────────────────────────────────────────────
TEST_CASES = [
    # (文件名, 期望的路径结果)
    ("Inception.2010.1080p.BluRay.x264.mkv",        "Inception (2010)"),
    ("The.Dark.Knight.2008.4K.HEVC.mkv",             "The Dark Knight (2008)"),
    ("流浪地球2.2023.2160p.WEB-DL.mkv",              "流浪地球2 (2023)"),
    ("Breaking.Bad.S01E03.1080p.BluRay.HEVC.mkv",    "Breaking Bad/Season 1"),
    ("The.Crown.S03E04.2019.WEB.H264.mkv",           "The Crown (2019)/Season 3"),
    ("Stranger.Things.S04E01-E09.2022.HEVC.mkv",     "Stranger Things (2022)/Season 4"),
    ("权力的游戏.S06E10.2160p.mkv",                  "权力的游戏/Season 6"),
    ("Attack.on.Titan.S04E28.720p.mkv",              "Attack On Titan/Season 4"),  # parser 大写每个单词首字母
    # 无年份
    ("The.Wire.S02E05.mkv",                          "The Wire/Season 2"),
    # 单集文件名极简
    ("S01E01.mkv",                                   ""),   # 无法解析到标题，预期空串
]


def sep(title: str):
    print(f"\n{'─' * 64}")
    print(f"  {title}")
    print(f"{'─' * 64}")


def test_path_calculation():
    """纯路径计算测试（不需要 Drive 认证）"""
    sep("1. 路径计算测试（folder_path_for，无 Drive 操作）")

    # dry_run 时不需要真实 client，传 None 也能用 folder_path_for
    # 但为了代码简洁，用 dry_run 模式带一个假 client
    org = MediaOrganizer(client=None, root_folder_id="dummy", dry_run=True)

    pass_count = 0
    fail_count = 0

    for filename, expected in TEST_CASES:
        actual = org.folder_path_for(filename, isfile=True)
        ok = actual == expected
        status = "✓" if ok else "✗"
        if ok:
            pass_count += 1
        else:
            fail_count += 1
        print(f"  {status}  {filename!r}")
        print(f"       期望: {expected!r}")
        if not ok:
            print(f"       实际: {actual!r}")

    print(f"\n  结果：{pass_count} 通过，{fail_count} 失败")
    return fail_count == 0


def test_dry_run(client: DriveClient):
    """dry_run 模式：验证 ensure_folder_for 日志输出，不操作 Drive"""
    sep("2. dry_run 模式测试（ensure_folder_for，不创建 Drive 文件夹）")

    org = MediaOrganizer(
        client=client,
        root_folder_id=CONFIG["test_folder_id"],
        dry_run=True,
    )

    samples = [
        "Inception.2010.1080p.BluRay.mkv",
        "Breaking.Bad.S01E03.1080p.mkv",
        "流浪地球2.2023.2160p.mkv",
        "权力的游戏.S06E10.2160p.mkv",
    ]
    for fn in samples:
        result = org.ensure_folder_for(fn)
        path = org.folder_path_for(fn)
        print(f"  {'📁' if path else '⚠️ '}  {fn!r}  →  {path!r}")
        assert result is None, "dry_run 应返回 None"

    print("  ✓ 全部 dry_run 测试通过")
    return True


def test_real_create(client: DriveClient):
    """真实创建文件夹测试（会在 Drive 上产生文件夹，测试后自动清理）"""
    sep("3. 真实创建测试（ensure_folder_for，会操作 Drive）")

    org = MediaOrganizer(
        client=client,
        root_folder_id=CONFIG["test_folder_id"],
        dry_run=False,
    )

    samples = [
        ("Inception.2010.1080p.BluRay.mkv",         "电影"),
        ("Breaking.Bad.S01E03.1080p.BluRay.mkv",    "剧集"),
        ("流浪地球2.2023.2160p.mkv",                 "中文电影"),
        ("权力的游戏.S06E10.2160p.mkv",              "中文剧集"),
    ]

    created_top_ids = []
    for fn, label in samples:
        print(f"\n  [{label}] {fn!r}")
        folder = org.ensure_folder_for(fn)
        if folder:
            print(f"    → {folder}")
            # 记录顶层文件夹以便清理（Drive 删文件夹时会递归删子项）
            # 这里简单记录返回的叶子文件夹的 parents[0] 作为顶层（对剧集是 Season 文件夹）
        else:
            print(f"    → ⚠️  创建失败或解析失败")

    # 自动清理（删除本次创建的所有顶层文件夹）
    sep("  🧹 清理测试文件夹")
    test_root_files = client.list_files(folder_id=CONFIG["test_folder_id"])
    test_names = {fn for fn, _ in samples}
    # 计算期望的顶层文件夹名称
    org_tmp = MediaOrganizer(client=None, root_folder_id="x", dry_run=True)
    expected_tops = set()
    for fn, _ in samples:
        path = org_tmp.folder_path_for(fn)
        if path:
            expected_tops.add(path.split("/")[0])

    deleted = 0
    for f in test_root_files:
        if f.is_folder and f.name in expected_tops:
            client.delete_file(f.id)
            print(f"    已删除：{f}")
            deleted += 1

    print(f"  清理完成，共删除 {deleted} 个文件夹")
    return True


def main():
    parser = argparse.ArgumentParser(description="MediaOrganizer 测试")
    parser.add_argument("--real", action="store_true", help="执行真实 Drive 创建测试")
    args = parser.parse_args()

    print("=" * 64)
    print("  MediaOrganizer 测试")
    print("=" * 64)

    results = {}

    # 1. 纯路径计算（不需要 Drive）
    results["path_calculation"] = test_path_calculation()

    # 2. 需要 Drive 的测试
    try:
        mode = CONFIG["auth_mode"]
        if mode == "oauth2":
            client = DriveClient.from_oauth(
                credentials_path=CONFIG["credentials_json"],
                token_path=CONFIG["token_json"],
            )
        else:
            client = DriveClient.from_service_account(CONFIG["service_account_json"])

        results["dry_run"] = test_dry_run(client)

        if args.real:
            results["real_create"] = test_real_create(client)
        else:
            print("\n  （跳过真实创建测试，使用 --real 参数启用）")

    except FileNotFoundError as e:
        print(f"\n[跳过 Drive 测试] 找不到认证文件：{e}")
    except Exception as e:
        print(f"\n[Drive 测试失败] {type(e).__name__}: {e}")
        results["drive"] = False

    # 汇总
    print(f"\n{'=' * 64}")
    print("  测试结果汇总")
    print(f"{'=' * 64}")
    for name, ok in results.items():
        print(f"  {'✓' if ok else '✗'}  {name}")
    print()


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""
test_drive.py —— Google Drive 模块独立测试

测试覆盖：
  ✓ 认证连接（about）
  ✓ 列举文件夹（list_files）
  ✓ 列举媒体文件（list_media_files）
  ✓ 查找文件（find_file / exists）
  ✓ 创建文件夹（create_folder）
  ✓ 上传文本（upload_text）
  ✓ 重命名（rename_file）
  ✓ 移动文件（move_file）
  ✓ 回收站（trash_file）
  ✓ 彻底删除（delete_file）

使用前准备：
  1. 在 Google Cloud Console 选择一种认证方式：
     方式A（Service Account）：
       - 创建服务账号，下载 JSON Key → 改名为 service_account.json
       - 把要操作的 Drive 文件夹共享给服务账号邮箱
     方式B（OAuth2）：
       - 创建 OAuth2 Client ID（桌面应用），下载 JSON → 改名为 credentials.json
       - 首次运行会弹出浏览器授权

  2. 修改下方 CONFIG 字典，填入 FOLDER_ID（目标文件夹的 Drive ID）
"""

import sys
import os

# 允许从项目根目录直接运行
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from drive import DriveClient
from googleapiclient.errors import HttpError

# ═══════════════════════════════════════════════════════
#  ▶ 修改这里的配置
# ═══════════════════════════════════════════════════════
CONFIG = {
    # 认证方式："service_account" 或 "oauth2"
    "auth_mode": "oauth2",

    # Service Account JSON 路径（auth_mode = service_account 时使用）
    "service_account_json": "config/service_account.json",

    # OAuth2 凭据文件路径（auth_mode = oauth2 时使用）
    "credentials_json": "config/credentials.json",
    "token_json": "config/token.json",

    # 测试用的文件夹 ID（在 Drive 地址栏 URL 中找，格式类似 1AbCdEfGhIjKlMnOpQrStUvWxYz）
    # "root" 表示根目录（Service Account 场景下此项必须填具体 ID）
    "test_folder_id": "15MrQz5ukLAAYOXzseE1s9CmFBGDvVA4x",
}
# ═══════════════════════════════════════════════════════


def sep(title: str):
    print(f"\n{'─' * 60}")
    print(f"  {title}")
    print(f"{'─' * 60}")


def build_client() -> DriveClient:
    mode = CONFIG["auth_mode"]
    if mode == "service_account":
        return DriveClient.from_service_account(CONFIG["service_account_json"])
    elif mode == "oauth2":
        return DriveClient.from_oauth(
            credentials_path=CONFIG["credentials_json"],
            token_path=CONFIG["token_json"],
        )
    else:
        raise ValueError(f"未知认证模式：{mode}，应为 'service_account' 或 'oauth2'")


def test_about(client: DriveClient):
    sep("1. 账号信息（about）")
    info = client.about()
    user = info.get("user", {})
    quota = info.get("storageQuota", {})
    print(f"  用户：{user.get('displayName')}  <{user.get('emailAddress')}>")

    def mb(b):
        return f"{int(b) / 1024 / 1024:.1f} MB" if b else "N/A"

    print(f"  空间：已用 {mb(quota.get('usage'))} / 共 {mb(quota.get('limit'))}")
    return True


def test_list_files(client: DriveClient):
    sep("2. 列举文件夹内容（list_files）")
    folder_id = CONFIG["test_folder_id"]
    files = client.list_files(folder_id=folder_id, page_size=20)
    if not files:
        print("  （文件夹为空）")
    for f in files[:10]:
        print(f"  {f}")
    if len(files) > 10:
        print(f"  ... 共 {len(files)} 项")
    return True


def test_list_media(client: DriveClient):
    sep("3. 列举视频文件（list_media_files）")
    folder_id = CONFIG["test_folder_id"]
    videos = client.list_media_files(folder_id=folder_id)
    if not videos:
        print("  （未找到视频文件）")
    for v in videos[:5]:
        print(f"  {v}")
    return True


def test_upload_and_ops(client: DriveClient):
    """测试上传、查找、重命名、移动、删除"""
    folder_id = CONFIG["test_folder_id"]

    # ── 创建临时测试文件夹 ─────────────────────────────
    sep("4. 创建测试文件夹（create_folder）")
    tmp_folder = client.create_folder("__drive_test_tmp__", parent_id=folder_id)
    print(f"  已创建：{tmp_folder}")

    try:
        # ── 上传文本文件 ──────────────────────────────
        sep("5. 上传文本文件（upload_text）")
        nfo_content = """\
<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>Test Movie</title>
  <year>2024</year>
</movie>"""
        uploaded = client.upload_text(
            content=nfo_content,
            name="test_movie.nfo",
            parent_id=tmp_folder.id,
            mime_type="text/xml",
        )
        print(f"  已上传：{uploaded}")

        # ── 查找文件 ──────────────────────────────────
        sep("6. 查找文件（find_file / exists）")
        found = client.find_file("test_movie.nfo", folder_id=tmp_folder.id)
        print(f"  find_file: {found}")
        print(f"  exists:    {client.exists('test_movie.nfo', folder_id=tmp_folder.id)}")
        print(f"  不存在:    {client.exists('nonexistent.nfo', folder_id=tmp_folder.id)}")

        # ── 重命名 ────────────────────────────────────
        sep("7. 重命名（rename_file）")
        renamed = client.rename_file(uploaded.id, "test_movie_renamed.nfo")
        print(f"  重命名后：{renamed}")

        # ── overwrite 上传（更新内容） ─────────────────
        sep("8. 更新已有文件（upload_text with overwrite=True）")
        updated = client.upload_text(
            content="<movie><title>Updated</title></movie>",
            name="test_movie_renamed.nfo",
            parent_id=tmp_folder.id,
            overwrite=True,
        )
        print(f"  更新后：{updated}")

        # ── get_file ──────────────────────────────────
        sep("9. 获取单个文件元数据（get_file）")
        detail = client.get_file(updated.id)
        print(f"  ID           : {detail.id}")
        print(f"  名称         : {detail.name}")
        print(f"  MIME         : {detail.mime_type}")
        print(f"  修改时间     : {detail.modified_time}")
        print(f"  是视频       : {detail.is_video}")

        # ── 移动文件 ──────────────────────────────────
        sep("10. 移动文件（move_file）")
        sub_folder = client.create_folder("__sub__", parent_id=tmp_folder.id)
        moved = client.move_file(updated.id, new_folder_id=sub_folder.id)
        print(f"  移动后 parents：{moved.parents}")
        print(f"  文件：{moved}")

        # ── 回收站 ────────────────────────────────────
        sep("11. 移入回收站（trash_file）")
        trashed = client.trash_file(moved.id)
        print(f"  trashed = {trashed.trashed}")

    finally:
        # ── 清理：彻底删除测试文件夹（及其内容） ──────
        sep("12. 清理测试文件夹（delete_file）")
        try:
            client.delete_file(tmp_folder.id)
            print(f"  已删除测试文件夹：{tmp_folder.id}")
        except HttpError as e:
            print(f"  清理失败（可手动删除）：{e}")

    return True


def main():
    print("=" * 60)
    print("  Google Drive 模块测试")
    print("  认证方式：" + CONFIG["auth_mode"])
    print("=" * 60)

    try:
        client = build_client()
    except FileNotFoundError as e:
        print(f"\n[错误] {e}")
        sys.exit(1)

    results = {}

    for name, fn in [
        ("about", lambda: test_about(client)),
        ("list_files", lambda: test_list_files(client)),
        ("list_media", lambda: test_list_media(client)),
        ("upload_ops", lambda: test_upload_and_ops(client)),
    ]:
        try:
            ok = fn()
            results[name] = "✓" if ok else "✗"
        except HttpError as e:
            print(f"\n[HTTP错误] {e}")
            results[name] = "✗"
        except Exception as e:
            print(f"\n[异常] {type(e).__name__}: {e}")
            results[name] = "✗"

    # 汇总
    print(f"\n{'=' * 60}")
    print("  测试结果汇总")
    print(f"{'=' * 60}")
    for name, status in results.items():
        print(f"  {status}  {name}")
    print()


if __name__ == "__main__":
    main()

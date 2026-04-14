"""
test/test_storage.py —— storage 抽象层单元测试
"""

import os
import sys
import tempfile
import pytest

# 确保项目根目录在 path 中
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from mediaparser.config import Config
from storage.base import StorageProvider, CloudFile, FileType, _VIDEO_EXTENSIONS


class TestCloudFile:
    """CloudFile 数据模型测试"""

    def test_video_by_mime(self):
        """MIME 类型以 video/ 开头的文件应识别为视频"""
        f = CloudFile(id="1", name="test.dat", file_type=FileType.FILE, mime_type="video/mp4")
        assert f.is_video is True

    def test_video_by_extension(self):
        """无 MIME 但扩展名为常见视频格式的文件应识别为视频"""
        for ext in [".mkv", ".mp4", ".avi", ".rmvb", ".ts", ".m2ts"]:
            f = CloudFile(id="1", name=f"movie{ext}", file_type=FileType.FILE)
            assert f.is_video is True, f"Extension {ext} should be recognized as video"

    def test_non_video(self):
        """NFO 文件不应识别为视频"""
        f = CloudFile(id="1", name="movie.nfo", file_type=FileType.FILE, mime_type="text/xml")
        assert f.is_video is False

    def test_folder(self):
        """FOLDER 类型的 CloudFile 应识别为文件夹"""
        f = CloudFile(id="1", name="Season 01", file_type=FileType.FOLDER)
        assert f.is_folder is True
        assert f.is_video is False

    def test_folder_with_video_extension_is_not_video(self):
        """名字像视频的目录也不能被识别成视频文件"""
        f = CloudFile(id="1", name="fake.mkv", file_type=FileType.FOLDER)
        assert f.is_folder is True
        assert f.is_video is False

    def test_file_not_folder(self):
        """FILE 类型的 CloudFile 不应识别为文件夹"""
        f = CloudFile(id="1", name="test.mkv", file_type=FileType.FILE)
        assert f.is_folder is False

    def test_extension(self):
        """extension 属性应返回小写扩展名"""
        f = CloudFile(id="1", name="Movie.MKV", file_type=FileType.FILE)
        assert f.extension == ".mkv"

    def test_extension_no_ext(self):
        """无扩展名的文件应返回空字符串"""
        f = CloudFile(id="1", name="README", file_type=FileType.FILE)
        assert f.extension == ""

    def test_parents_and_parent_id(self):
        """parents 列表和 parent_id 应独立存储"""
        f = CloudFile(
            id="1", name="test.mkv", file_type=FileType.FILE,
            parent_id="p1", parents=["p1", "p2"],
        )
        assert f.parent_id == "p1"
        assert f.parents == ["p1", "p2"]

    def test_extra_metadata(self):
        """extra 字段应能存储平台特有数据"""
        f = CloudFile(
            id="1", name="test.mkv", file_type=FileType.FILE,
            extra={"pick_code": "abc123", "sha1": "deadbeef"},
        )
        assert f.extra["pick_code"] == "abc123"
        assert f.extra["sha1"] == "deadbeef"

    def test_repr(self):
        """repr 应包含文件名和 ID 前缀"""
        f = CloudFile(id="abcdefghijklmnop", name="test.mkv", file_type=FileType.FILE, size=100*1024*1024)
        r = repr(f)
        assert "test.mkv" in r
        assert "🎬" in r  # video icon
        assert "abcdefghijk" in r  # first 12 chars of ID

    def test_repr_folder(self):
        """repr 中文件夹应显示 📁 图标"""
        f = CloudFile(id="abcdefghijklmnop", name="Season 01", file_type=FileType.FOLDER)
        r = repr(f)
        assert "📁" in r

    def test_defaults(self):
        """默认值应正确初始化"""
        f = CloudFile(id="1", name="test", file_type=FileType.FILE)
        assert f.size is None
        assert f.modified_time is None
        assert f.parent_id is None
        assert f.parents == []
        assert f.mime_type is None
        assert f.trashed is False
        assert f.extra == {}


class TestProviderRegistry:
    """Provider 注册表测试"""

    def test_builtin_providers_registered(self):
        """内置 Provider 应自动注册"""
        from storage import list_providers
        providers = list_providers()
        assert "google_drive" in providers
        assert "pan115" in providers

    def test_unknown_provider_raises(self):
        """请求未注册的 Provider 应抛出 ValueError"""
        from storage import get_provider
        with pytest.raises(ValueError, match="未知的存储 Provider"):
            get_provider("nonexistent_cloud", None)

    def test_register_custom_provider(self):
        """应能注册自定义 Provider"""
        from storage import register_provider, list_providers

        class FakeProvider(StorageProvider):
            provider_name = "fake"
            @classmethod
            def from_config(cls, cfg): return cls()
            def list_files(self, **kw): return []
            def get_file(self, file_id): ...
            def read_text(self, file): ...
            def find_file(self, name, folder_id=None): ...
            def rename_file(self, file_id, new_name): ...
            def move_file(self, file_id, new_folder_id, new_name=None): ...
            def upload_text(self, content, name, **kw): ...
            def upload_bytes(self, content, name, **kw): ...
            def trash_file(self, file_id): ...
            def delete_file(self, file_id): ...
            def create_folder(self, name, parent_id=None): ...

        register_provider("fake_test", FakeProvider)
        assert "fake_test" in list_providers()


class TestGoogleDriveProvider:
    """GoogleDriveProvider 类型转换测试"""

    def test_to_cloud_file_video(self):
        """DriveFile → CloudFile 转换（视频文件）"""
        from drive.client import DriveFile
        from storage.google_drive import GoogleDriveProvider

        df = DriveFile(
            id="file123",
            name="movie.mkv",
            mime_type="video/x-matroska",
            size=1024*1024*500,
            modified_time="2024-01-01T00:00:00Z",
            parents=["parent1"],
        )
        cf = GoogleDriveProvider._to_cloud_file(df)

        assert cf.id == "file123"
        assert cf.name == "movie.mkv"
        assert cf.file_type == FileType.FILE
        assert cf.is_video is True
        assert cf.size == 1024*1024*500
        assert cf.parent_id == "parent1"
        assert cf.parents == ["parent1"]
        assert cf.mime_type == "video/x-matroska"

    def test_to_cloud_file_folder(self):
        """DriveFile → CloudFile 转换（文件夹）"""
        from drive.client import DriveFile
        from storage.google_drive import GoogleDriveProvider

        df = DriveFile(
            id="folder123",
            name="Season 01",
            mime_type="application/vnd.google-apps.folder",
            parents=["root"],
        )
        cf = GoogleDriveProvider._to_cloud_file(df)

        assert cf.id == "folder123"
        assert cf.is_folder is True
        assert cf.is_video is False
        assert cf.parent_id == "root"


class TestPan115Provider:
    """Pan115Provider 类型转换测试"""

    def test_to_cloud_file_video(self):
        """Pan115File → CloudFile 转换（视频文件）"""
        from u115pan.models import Pan115File
        from storage.pan115 import Pan115Provider

        pf = Pan115File(
            id="9876543",
            name="movie.mkv",
            category="1",  # 非 "0" 即为文件
            size=1024*1024*700,
            pick_code="xyz789",
            sha1="abcdef",
            parent_id="folder123",
        )
        cf = Pan115Provider._to_cloud_file(pf)

        assert cf.id == "9876543"
        assert cf.name == "movie.mkv"
        assert cf.file_type == FileType.FILE
        assert cf.is_video is True  # by extension
        assert cf.size == 1024*1024*700
        assert cf.parent_id == "folder123"
        assert cf.parents == ["folder123"]
        assert cf.mime_type is None  # 115 doesn't provide MIME
        assert cf.extra["pick_code"] == "xyz789"
        assert cf.extra["sha1"] == "abcdef"

    def test_to_cloud_file_folder(self):
        """Pan115File → CloudFile 转换（文件夹）"""
        from u115pan.models import Pan115File
        from storage.pan115 import Pan115Provider

        pf = Pan115File(
            id="111222",
            name="剧集目录",
            category="0",  # "0" = 文件夹
        )
        cf = Pan115Provider._to_cloud_file(pf)

        assert cf.is_folder is True
        assert cf.is_video is False

    def test_list_all_recursive_uses_full_listing(self):
        """115 递归遍历应使用 list_all_files，避免单页 1000 条截断。"""
        from storage.pan115 import Pan115Provider
        from u115pan.models import Pan115File

        class FakePan115Client:
            def list_all_files(self, cid, limit=1000):
                if str(cid) == "0":
                    return [
                        Pan115File(id="folder1", name="Season 01", category="0"),
                        Pan115File(id="file1", name="movie.mkv", category="1"),
                    ]
                if str(cid) == "folder1":
                    return [
                        Pan115File(id="file2", name="episode.mkv", category="1"),
                    ]
                return []

        provider = Pan115Provider(FakePan115Client())
        items = list(provider.list_all_recursive(folder_id="root"))

        assert [item.id for item in items] == ["folder1", "file2", "file1"]

    def test_compute_sign_val_includes_end_byte(self):
        """sign_check 的 end 字节应被包含，否则二次认证会失败。"""
        from u115pan.client import Pan115Client
        import hashlib

        with tempfile.NamedTemporaryFile(delete=False) as tmp:
            tmp.write(b"abcdef")
            tmp_path = tmp.name

        try:
            sign = Pan115Client.compute_sign_val(tmp_path, "1-3")
            assert sign == hashlib.sha1(b"bcd").hexdigest().upper()
        finally:
            os.unlink(tmp_path)


class TestConfigStorageSelection:
    """当前存储后端目录选择测试"""

    def test_google_drive_storage_paths(self):
        cfg = Config.from_dict({
            "storage": {"primary": "google_drive"},
            "drive": {
                "scan_folder_id": "scan-drive",
                "root_folder_id": "root-drive",
                "movie_root_id": "movie-drive",
                "tv_root_id": "tv-drive",
            },
            "u115": {
                "download_folder_id": "scan-115",
                "root_folder_id": "root-115",
                "movie_root_id": "movie-115",
                "tv_root_id": "tv-115",
            },
        })

        assert cfg.active_scan_folder_id() == "scan-drive"
        assert cfg.active_root_folder_id() == "root-drive"
        assert cfg.active_movie_root_id() == "movie-drive"
        assert cfg.active_tv_root_id() == "tv-drive"

    def test_pan115_storage_paths(self):
        cfg = Config.from_dict({
            "storage": {"primary": "pan115"},
            "drive": {
                "scan_folder_id": "scan-drive",
                "root_folder_id": "root-drive",
                "movie_root_id": "movie-drive",
                "tv_root_id": "tv-drive",
            },
            "u115": {
                "download_folder_id": "scan-115",
                "root_folder_id": "root-115",
                "movie_root_id": "movie-115",
                "tv_root_id": "tv-115",
            },
        })

        assert cfg.active_scan_folder_id() == "scan-115"
        assert cfg.active_root_folder_id() == "root-115"
        assert cfg.active_movie_root_id() == "movie-115"
        assert cfg.active_tv_root_id() == "tv-115"


class TestPan115ClientHelpers:
    """115 客户端辅助方法测试"""

    def test_get_download_url_parses_data_map(self):
        from u115pan.client import Pan115Client

        client = Pan115Client(client_id="test", token=None)

        def fake_request(*args, **kwargs):
            return {
                "state": True,
                "code": 0,
                "data": {
                    "123": {
                        "pick_code": "abc",
                        "url": {"url": "https://example.com/file.txt"},
                    }
                },
            }

        client._request = fake_request  # type: ignore[method-assign]
        assert client.get_download_url("abc") == "https://example.com/file.txt"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])

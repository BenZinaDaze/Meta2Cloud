import importlib.util
import sys
import types
from enum import Enum
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))


def _load_config_module():
    spec = importlib.util.spec_from_file_location(
        "mediaparser.config",
        ROOT / "mediaparser" / "config.py",
    )
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _load_storage_base_module():
    storage_pkg = types.ModuleType("storage")
    storage_pkg.__path__ = []
    sys.modules["storage"] = storage_pkg

    spec = importlib.util.spec_from_file_location(
        "storage.base",
        ROOT / "storage" / "base.py",
    )
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    storage_pkg.base = module
    return module


config_mod = _load_config_module()
storage_base_mod = _load_storage_base_module()


class MediaType(Enum):
    MOVIE = "Movie"
    TV = "TV"


class MetaInfo:
    def __init__(self, name, isfile=True, custom_words=None, release_group_matcher=None):
        self.name = Path(name).stem
        self.year = ""
        self.type = MediaType.MOVIE
        self.season_seq = None
        self.episode_seq = None


class TmdbClient:
    def __init__(self, *args, **kwargs):
        pass

    def recognize(self, meta):
        return {
            "title": meta.name,
            "tmdb_id": 1,
            "release_date": "",
            "poster_path": "",
            "backdrop_path": "",
        }


mediaparser_stub = types.ModuleType("mediaparser")
mediaparser_stub.__path__ = []
mediaparser_stub.Config = config_mod.Config
mediaparser_stub.ConfigParseError = config_mod.ConfigParseError
mediaparser_stub.MetaInfo = MetaInfo
mediaparser_stub.MetaInfoPath = MetaInfo
mediaparser_stub.TmdbClient = TmdbClient
mediaparser_stub.MediaType = MediaType
sys.modules["mediaparser"] = mediaparser_stub
sys.modules["mediaparser.config"] = config_mod

release_group_stub = types.ModuleType("mediaparser.release_group")


class ReleaseGroupsMatcher:
    def __init__(self, *args, **kwargs):
        pass


release_group_stub.ReleaseGroupsMatcher = ReleaseGroupsMatcher
sys.modules["mediaparser.release_group"] = release_group_stub

requests_stub = types.ModuleType("requests")
requests_stub.post = lambda *args, **kwargs: None
sys.modules["requests"] = requests_stub

nfo_stub = types.ModuleType("nfo")


class NfoGenerator:
    def generate(self, *args, **kwargs):
        return "<nfo />"

    def nfo_name_for(self, name):
        return f"{Path(name).stem}.nfo"

    def generate_tvshow(self, *args, **kwargs):
        return "<tvshow />"

    def generate_season(self, *args, **kwargs):
        return "<season />"


class ImageUploader:
    def __init__(self, *args, **kwargs):
        pass


nfo_stub.NfoGenerator = NfoGenerator
nfo_stub.ImageUploader = ImageUploader
sys.modules["nfo"] = nfo_stub

from core.pipeline import Pipeline
from mediaparser.config import Config, ConfigParseError
from storage.base import CloudFile, FileType


class FakeStorageProvider:
    def __init__(self):
        self.files = {}
        self.trashed_ids = []
        self.moved = []
        self.uploaded = []
        self.calls = []
        self.trash_error = None
        self.move_error = None

    def find_file(self, name, folder_id=None):
        self.calls.append(("find", name, folder_id))
        return self.files.get((folder_id, name))

    def trash_file(self, file_id):
        self.calls.append(("trash", file_id))
        if self.trash_error:
            raise self.trash_error
        self.trashed_ids.append(file_id)
        return make_video(file_id, "trashed.mkv", "target")

    def move_file(self, file_id, new_folder_id, new_name=None):
        self.calls.append(("move", file_id, new_folder_id, new_name))
        if self.move_error:
            raise self.move_error
        self.moved.append((file_id, new_folder_id, new_name))
        return make_video(file_id, new_name or "video.mkv", new_folder_id)

    def upload_text(self, content, name, parent_id=None, mime_type=None, overwrite=False):
        self.calls.append(("upload_text", name, parent_id))
        self.uploaded.append(("text", name, parent_id))
        return CloudFile(id=f"upload-{name}", name=name, file_type=FileType.FILE, parent_id=parent_id)

    def upload_bytes(self, content, name, parent_id=None, mime_type=None, overwrite=False):
        self.calls.append(("upload_bytes", name, parent_id))
        self.uploaded.append(("bytes", name, parent_id))
        return CloudFile(id=f"upload-{name}", name=name, file_type=FileType.FILE, parent_id=parent_id)


def make_video(file_id, name, folder_id):
    return CloudFile(id=file_id, name=name, file_type=FileType.FILE, parent_id=folder_id)


class FakeOrganizer:
    def __init__(self):
        self.target_folder = CloudFile(
            id="target",
            name="Target",
            file_type=FileType.FOLDER,
            parent_id="root",
        )

    def ensure_folder_for_meta(self, meta, label=""):
        return self.target_folder

    def folder_path_for_meta(self, meta):
        return "Target"


def make_pipeline(storage, replace_existing_video):
    cfg = Config.from_dict({
        "tmdb": {"api_key": "fake-api-key"},
        "drive": {"root_folder_id": "root"},
        "pipeline": {"replace_existing_video": replace_existing_video},
    })
    pipeline = Pipeline(storage, cfg, skip_images=True)
    pipeline._organizer = FakeOrganizer()
    return pipeline


class TestReplaceExistingVideo:
    def test_skip_when_replace_disabled(self):
        """关闭开关时跳过同名文件，不调用 trash_file 或 move_file"""
        storage = FakeStorageProvider()
        storage.files[("target", "video.mkv")] = make_video("existing", "video.mkv", "target")

        result = make_pipeline(storage, replace_existing_video=False)._process_one(
            make_video("source", "video.mkv", "source"),
            1,
            1,
        )

        assert result.status == "skipped"
        assert result.reason == "目标位置已存在同名文件"
        assert storage.trashed_ids == []
        assert storage.moved == []
        assert storage.uploaded == []

    def test_replace_when_enabled(self):
        """开启开关时先调用 trash_file 再调用 move_file，最后上传元数据"""
        storage = FakeStorageProvider()
        storage.files[("target", "video.mkv")] = make_video("existing", "video.mkv", "target")

        result = make_pipeline(storage, replace_existing_video=True)._process_one(
            make_video("source", "video.mkv", "source"),
            1,
            1,
        )

        call_names = [call[0] for call in storage.calls]
        assert result.status == "ok"
        assert storage.trashed_ids == ["existing"]
        assert storage.moved == [("source", "target", "video.mkv")]
        assert storage.uploaded == [("text", "video.nfo", "target")]
        assert call_names.index("trash") < call_names.index("move")
        assert call_names.index("move") < call_names.index("upload_text")

    def test_same_file_skipped(self):
        """文件已在目标位置时跳过"""
        storage = FakeStorageProvider()
        storage.files[("target", "video.mkv")] = make_video("source", "video.mkv", "target")

        result = make_pipeline(storage, replace_existing_video=True)._process_one(
            make_video("source", "video.mkv", "target"),
            1,
            1,
        )

        assert result.status == "skipped"
        assert result.reason == "文件已在目标位置"
        assert storage.trashed_ids == []
        assert storage.moved == []
        assert storage.uploaded == []

    def test_trash_failure_no_move(self):
        """移除失败时不调用 move_file，不上传元数据"""
        storage = FakeStorageProvider()
        storage.files[("target", "video.mkv")] = make_video("existing", "video.mkv", "target")
        storage.trash_error = Exception("trash failed")

        result = make_pipeline(storage, replace_existing_video=True)._process_one(
            make_video("source", "video.mkv", "source"),
            1,
            1,
        )

        assert result.status == "failed"
        assert result.reason == "移除同名文件失败：trash failed"
        assert storage.trashed_ids == []
        assert storage.moved == []
        assert storage.uploaded == []
        assert "move" not in [call[0] for call in storage.calls]

    def test_move_failure_after_trash(self):
        """移除成功但移动失败时，不上传元数据，reason 包含已移除的文件 ID"""
        storage = FakeStorageProvider()
        storage.files[("target", "video.mkv")] = make_video("existing", "video.mkv", "target")
        storage.move_error = Exception("move failed")

        result = make_pipeline(storage, replace_existing_video=True)._process_one(
            make_video("source", "video.mkv", "source"),
            1,
            1,
        )

        assert result.status == "failed"
        assert storage.trashed_ids == ["existing"]
        assert "existing" in result.reason
        assert "已移除的旧文件 ID" in result.reason
        assert storage.uploaded == []

    def test_no_metadata_upload_on_skip(self):
        """跳过同名文件时不生成 NFO/图片"""
        storage = FakeStorageProvider()
        storage.files[("target", "video.mkv")] = make_video("existing", "video.mkv", "target")

        result = make_pipeline(storage, replace_existing_video=False)._process_one(
            make_video("source", "video.mkv", "source"),
            1,
            1,
        )

        assert result.status == "skipped"
        assert result.nfo_uploaded is False
        assert storage.uploaded == []
        assert not any(call[0].startswith("upload") for call in storage.calls)


class TestConfigParseBoolStr:
    def test_valid_bool(self):
        """布尔值直接返回"""
        assert config_mod._parse_bool_str(True, "pipeline.dry_run") is True
        assert config_mod._parse_bool_str(False, "pipeline.dry_run") is False

    def test_valid_string_true(self):
        """字符串 'true'/'yes'/'1' 返回 True"""
        assert config_mod._parse_bool_str("true", "pipeline.dry_run") is True
        assert config_mod._parse_bool_str("yes", "pipeline.dry_run") is True
        assert config_mod._parse_bool_str("1", "pipeline.dry_run") is True
        assert config_mod._parse_bool_str(" TRUE ", "pipeline.dry_run") is True

    def test_valid_string_false(self):
        """字符串 'false'/'no'/'0' 返回 False"""
        assert config_mod._parse_bool_str("false", "pipeline.dry_run") is False
        assert config_mod._parse_bool_str("no", "pipeline.dry_run") is False
        assert config_mod._parse_bool_str("0", "pipeline.dry_run") is False
        assert config_mod._parse_bool_str(" FALSE ", "pipeline.dry_run") is False

    def test_invalid_string_raises(self):
        """无效字符串抛出 ConfigParseError"""
        with pytest.raises(ConfigParseError, match="无法解析布尔值"):
            config_mod._parse_bool_str("maybe", "pipeline.replace_existing_video")

    def test_invalid_type_raises(self):
        """无效类型抛出 ConfigParseError"""
        with pytest.raises(ConfigParseError, match="必须是布尔值"):
            config_mod._parse_bool_str(["true"], "pipeline.replace_existing_video")

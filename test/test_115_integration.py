from __future__ import annotations

import copy
import os
import sys
import time
import uuid
from pathlib import Path

import pytest

_ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _ROOT not in sys.path:
    sys.path.insert(0, _ROOT)

from core.pipeline import Pipeline
from mediaparser.config import Config
from storage.base import CloudFile, FileType
from storage.pan115 import Pan115Provider


pytestmark = pytest.mark.integration

ROOT = Path(__file__).resolve().parents[1]
CONFIG_PATH = ROOT / "config" / "config.yaml"
ROOT_FOLDER_ID = "0"


class FixedOrganizer:
    def __init__(self, target_folder: CloudFile):
        self.target_folder = target_folder

    def ensure_folder_for_meta(self, meta, label: str = "") -> CloudFile:
        return self.target_folder

    def folder_path_for_meta(self, meta) -> str:
        return self.target_folder.name


def _resolve_config_path(path: str) -> Path:
    p = Path(path)
    if not p.is_absolute():
        p = ROOT / p
    return p


def _load_115_config() -> Config:
    if not CONFIG_PATH.exists():
        pytest.skip(f"115 integration skipped: config file not found: {CONFIG_PATH}")

    cfg = Config.load(CONFIG_PATH)
    token_path = _resolve_config_path(cfg.u115.token_json)
    if not cfg.u115.client_id:
        pytest.skip("115 integration skipped: u115.client_id is not configured")
    if not cfg.u115.token_json or not token_path.exists():
        pytest.skip(f"115 integration skipped: token file not found: {token_path}")
    return cfg


def _build_provider() -> Pan115Provider:
    cfg = _load_115_config()
    try:
        provider = Pan115Provider.from_config(cfg)
        provider.list_files(ROOT_FOLDER_ID, page_size=1)
        return provider
    except Exception as exc:
        pytest.skip(f"115 integration skipped: unable to initialize authenticated client: {exc}")


@pytest.fixture
def pan115_case():
    provider = _build_provider()
    folder_name = f"Meta2CloudIntegration-{int(time.time())}-{uuid.uuid4().hex[:8]}"
    test_folder = provider.create_folder(folder_name, parent_id=ROOT_FOLDER_ID)
    try:
        yield provider, test_folder
    finally:
        try:
            provider.trash_file(test_folder.id)
        except Exception:
            pass


def _unique_content(label: str) -> bytes:
    return f"{label}-{time.time_ns()}-{uuid.uuid4().hex}\n".encode("utf-8")


def _files_named(provider: Pan115Provider, folder_id: str, name: str) -> list[CloudFile]:
    return [item for item in provider.list_files(folder_id=folder_id, page_size=1000) if item.name == name]


def _wait_for_named(
    provider: Pan115Provider,
    folder_id: str,
    name: str,
    *,
    min_count: int = 1,
    timeout: float = 20.0,
) -> list[CloudFile]:
    deadline = time.time() + timeout
    last: list[CloudFile] = []
    while time.time() < deadline:
        last = _files_named(provider, folder_id, name)
        if len(last) >= min_count:
            return last
        time.sleep(1.0)
    return last


def _wait_for_file_id(
    provider: Pan115Provider,
    folder_id: str,
    file_id: str,
    *,
    timeout: float = 20.0,
) -> CloudFile | None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        for item in provider.list_files(folder_id=folder_id, page_size=1000):
            if item.id == file_id:
                return item
        time.sleep(1.0)
    return None


def _make_child_folder(provider: Pan115Provider, parent_id: str, name: str) -> CloudFile:
    folder = provider.create_folder(f"{name}-{uuid.uuid4().hex[:8]}", parent_id=parent_id)
    assert folder.id
    assert folder.is_folder
    return folder


def _make_pipeline(provider: Pan115Provider, cfg: Config, target_folder: CloudFile, *, replace: bool) -> Pipeline:
    pipeline_cfg = copy.deepcopy(cfg)
    pipeline_cfg.pipeline.skip_tmdb = True
    pipeline_cfg.pipeline.dry_run = False
    pipeline_cfg.pipeline.replace_existing_video = replace
    pipeline = Pipeline(provider, pipeline_cfg, skip_images=True)
    pipeline._organizer = FixedOrganizer(target_folder)
    return pipeline


@pytest.mark.integration
def test_upload_same_name_files(pan115_case):
    """上传同名文件：115 会创建两个同名文件（不同 ID），但返回值可能相同（秒传）"""
    provider, test_folder = pan115_case
    same_name = "same-name-upload.txt"

    first = provider.upload_bytes(
        _unique_content("first"),
        same_name,
        parent_id=test_folder.id,
        mime_type="text/plain",
        overwrite=False,
    )
    second = provider.upload_bytes(
        _unique_content("second"),
        same_name,
        parent_id=test_folder.id,
        mime_type="text/plain",
        overwrite=False,
    )

    # 等待文件出现在目录中
    matches = _wait_for_named(provider, test_folder.id, same_name, min_count=2, timeout=30.0)

    # 115 会创建两个同名文件（即使 upload_bytes 返回的 ID 相同）
    # 这是因为 115 的秒传机制：相同内容返回相同 ID，但不同内容会创建新文件
    assert len(matches) >= 2, f"期望至少 2 个同名文件，实际 {len(matches)} 个"

    # 验证两个文件的 ID 不同（因为内容不同）
    ids = {item.id for item in matches}
    assert len(ids) >= 2, f"期望至少 2 个不同的文件 ID，实际 {len(ids)} 个"


@pytest.mark.integration
def test_replace_existing_video_disabled(pan115_case):
    provider, test_folder = pan115_case
    cfg = _load_115_config()
    source_folder = _make_child_folder(provider, test_folder.id, "source")
    target_folder = _make_child_folder(provider, test_folder.id, "target")
    same_name = "Replace.Disabled.2024.mkv"
    existing = provider.upload_bytes(_unique_content("existing"), same_name, parent_id=target_folder.id, overwrite=False)
    source = provider.upload_bytes(_unique_content("source"), same_name, parent_id=source_folder.id, overwrite=False)

    assert _wait_for_named(provider, target_folder.id, same_name, min_count=1)
    result = _make_pipeline(provider, cfg, target_folder, replace=False)._process_one(source, 1, 1)

    assert result.status == "skipped"
    assert result.reason == "目标位置已存在同名文件"
    assert _wait_for_file_id(provider, source_folder.id, source.id) is not None
    target_matches = _wait_for_named(provider, target_folder.id, same_name, min_count=1)
    assert existing.id in {item.id for item in target_matches}


@pytest.mark.integration
def test_replace_existing_video_enabled(pan115_case):
    provider, test_folder = pan115_case
    cfg = _load_115_config()
    source_folder = _make_child_folder(provider, test_folder.id, "source")
    target_folder = _make_child_folder(provider, test_folder.id, "target")
    same_name = "Replace.Enabled.2024.mkv"
    existing = provider.upload_bytes(_unique_content("existing"), same_name, parent_id=target_folder.id, overwrite=False)
    source = provider.upload_bytes(_unique_content("source"), same_name, parent_id=source_folder.id, overwrite=False)

    assert _wait_for_named(provider, target_folder.id, same_name, min_count=1)
    result = _make_pipeline(provider, cfg, target_folder, replace=True)._process_one(source, 1, 1)

    assert result.status == "ok"
    assert result.moved is True
    moved = _wait_for_file_id(provider, target_folder.id, source.id)
    assert moved is not None
    assert moved.name == same_name
    target_ids = {item.id for item in _files_named(provider, target_folder.id, same_name)}
    assert source.id in target_ids
    assert existing.id not in target_ids


@pytest.mark.integration
def test_move_to_folder_with_same_name(pan115_case):
    provider, test_folder = pan115_case
    source_folder = _make_child_folder(provider, test_folder.id, "source")
    target_folder = _make_child_folder(provider, test_folder.id, "target")
    same_name = "Move.AutoRename.2024.mkv"
    existing = provider.upload_bytes(_unique_content("existing"), same_name, parent_id=target_folder.id, overwrite=False)
    source = provider.upload_bytes(_unique_content("source"), same_name, parent_id=source_folder.id, overwrite=False)

    provider.move_file(source.id, new_folder_id=target_folder.id)
    moved = _wait_for_file_id(provider, target_folder.id, source.id)
    assert moved is not None
    assert moved.id == source.id

    target_items = provider.list_files(folder_id=target_folder.id, page_size=1000)
    existing_after_move = next((item for item in target_items if item.id == existing.id), None)
    assert existing_after_move is not None
    assert existing_after_move.name == same_name
    assert moved.name != same_name
    assert moved.name.startswith(Path(same_name).stem)
    assert moved.name.endswith(Path(same_name).suffix)

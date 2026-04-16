"""
test/test_u115_runtime.py —— 115 运行时共享 client 管理测试
"""

from __future__ import annotations

import os
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

from storage.pan115 import Pan115Provider
from u115pan.models import Pan115File, Pan115Token
from u115pan.runtime import U115RuntimeManager


class DummyClient:
    def __init__(self, label: str):
        self.label = label
        self.token = None
        self.token_path = None

    def list_files(self, cid="0", limit=1000):
        return [
            Pan115File(
                id=self.label,
                name=f"{self.label}.mkv",
                category="1",
                parent_id=cid,
                pick_code=f"pick-{self.label}",
            )
        ]


def test_runtime_manager_reuses_client_until_token_file_changes(tmp_path, monkeypatch):
    token_path = tmp_path / "115-token.json"
    token_path.write_text("{}", encoding="utf-8")

    build_calls = []

    def fake_from_token_file(*, client_id, token_path, **kwargs):
        build_calls.append((client_id, token_path))
        return DummyClient(f"client-{len(build_calls)}")

    monkeypatch.setattr("u115pan.runtime.Pan115Client.from_token_file", fake_from_token_file)

    manager = U115RuntimeManager()
    client1 = manager.get_client(client_id="cid-1", token_path=str(token_path))
    client2 = manager.get_client(client_id="cid-1", token_path=str(token_path))

    assert client1 is client2
    assert len(build_calls) == 1

    # 明确刷新 mtime，确保 manager 感知到外部 token 文件更新
    time.sleep(0.01)
    token_path.write_text('{"updated": true}', encoding="utf-8")
    os.utime(token_path, None)

    client3 = manager.get_client(client_id="cid-1", token_path=str(token_path))
    assert client3 is not client1
    assert len(build_calls) == 2


def test_runtime_manager_syncs_token_to_shared_client(tmp_path, monkeypatch):
    token_path = tmp_path / "115-token.json"
    token_path.write_text("{}", encoding="utf-8")

    client = DummyClient("shared")
    monkeypatch.setattr(
        "u115pan.runtime.Pan115Client.from_token_file",
        lambda *, client_id, token_path, **kwargs: client,
    )

    manager = U115RuntimeManager()
    token = Pan115Token(
        access_token="access",
        refresh_token="refresh",
        expires_in=3600,
        refresh_time=123456,
    )

    synced = manager.sync_token(
        client_id="cid-1",
        token_path=str(token_path),
        token=token,
    )

    assert synced is client
    assert client.token is token
    assert client.token_path == os.path.abspath(str(token_path))


def test_runtime_manager_isolated_by_client_id_and_token_path(tmp_path, monkeypatch):
    token_path_a = tmp_path / "a.json"
    token_path_b = tmp_path / "b.json"
    token_path_a.write_text("{}", encoding="utf-8")
    token_path_b.write_text("{}", encoding="utf-8")

    build_calls = []

    def fake_from_token_file(*, client_id, token_path, **kwargs):
        build_calls.append((client_id, token_path))
        return DummyClient(f"{client_id}:{Path(token_path).name}")

    monkeypatch.setattr("u115pan.runtime.Pan115Client.from_token_file", fake_from_token_file)

    manager = U115RuntimeManager()
    client_a1 = manager.get_client(client_id="cid-a", token_path=str(token_path_a))
    client_b = manager.get_client(client_id="cid-b", token_path=str(token_path_b))
    client_a2 = manager.get_client(client_id="cid-a", token_path=str(token_path_a))

    assert client_a1 is client_a2
    assert client_a1 is not client_b
    assert len(build_calls) == 2


def test_runtime_manager_normalizes_relative_and_absolute_token_paths(tmp_path, monkeypatch):
    token_path = tmp_path / "shared.json"
    token_path.write_text("{}", encoding="utf-8")

    build_calls = []

    def fake_from_token_file(*, client_id, token_path, **kwargs):
        build_calls.append((client_id, token_path))
        return DummyClient(Path(token_path).name)

    monkeypatch.setattr("u115pan.runtime.Pan115Client.from_token_file", fake_from_token_file)

    manager = U115RuntimeManager()
    cwd = os.getcwd()
    os.chdir(tmp_path)
    try:
        client_rel = manager.get_client(client_id="cid-1", token_path="shared.json")
        client_abs = manager.get_client(client_id="cid-1", token_path=str(token_path.resolve()))
    finally:
        os.chdir(cwd)

    assert client_rel is client_abs
    assert len(build_calls) == 1


def test_pan115_provider_uses_latest_client_from_getter():
    holder = {"client": DummyClient("first")}
    provider = Pan115Provider.from_client_getter(lambda: holder["client"])

    first_items = provider.list_files(folder_id="0")
    assert first_items[0].id == "first"

    holder["client"] = DummyClient("second")
    second_items = provider.list_files(folder_id="0")
    assert second_items[0].id == "second"

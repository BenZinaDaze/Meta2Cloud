import os
import sys
from datetime import datetime, timezone


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if ROOT not in sys.path:
    sys.path.insert(0, ROOT)


from mediaparser.config import Config
from storage.base import CloudFile, FileType
from webui.services import library_data


class FakeStorageProvider:
    def __init__(self, mapping):
        self.mapping = mapping

    def list_files(self, folder_id="root", page_size=100):
        return list(self.mapping.get(folder_id, []))

    def read_text(self, file):
        return None


def _folder(file_id: str, name: str, modified_time: str | None) -> CloudFile:
    return CloudFile(
        id=file_id,
        name=name,
        file_type=FileType.FOLDER,
        modified_time=modified_time,
    )


def test_normalize_modified_time_supports_unix_and_iso():
    assert library_data._normalize_modified_time(1777537875) == 1777537875
    assert library_data._normalize_modified_time("1777537875") == 1777537875
    assert library_data._normalize_modified_time("2026-04-29T03:25:21+00:00") == int(
        datetime(2026, 4, 29, 3, 25, 21, tzinfo=timezone.utc).timestamp()
    )
    assert library_data._normalize_modified_time("2026-04-29T03:25:21Z") == int(
        datetime(2026, 4, 29, 3, 25, 21, tzinfo=timezone.utc).timestamp()
    )
    assert library_data._normalize_modified_time(None) == 0
    assert library_data._normalize_modified_time("not-a-time") == 0


def test_scan_movies_accepts_iso_modified_time(monkeypatch):
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"movie_root_id": "movies"},
    })
    client = FakeStorageProvider({
        "movies": [_folder("m1", "Movie One (2024)", "2026-04-29T03:25:21+00:00")],
        "m1": [],
    })
    monkeypatch.setattr(library_data, "tmdb_get", lambda path: None)

    movies = library_data.scan_movies(client, cfg)

    assert len(movies) == 1
    assert movies[0].folder_modified_time == int(
        datetime(2026, 4, 29, 3, 25, 21, tzinfo=timezone.utc).timestamp()
    )


def test_scan_movies_incremental_accepts_iso_modified_time(monkeypatch):
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"movie_root_id": "movies"},
    })
    client = FakeStorageProvider({
        "movies": [_folder("m1", "Movie One (2024)", "2026-04-29T03:25:21+00:00")],
        "m1": [],
    })
    monkeypatch.setattr(library_data, "tmdb_get", lambda path: None)

    movies, mtimes = library_data.scan_movies_incremental(client, cfg, {})

    expected = int(datetime(2026, 4, 29, 3, 25, 21, tzinfo=timezone.utc).timestamp())
    assert len(movies) == 1
    assert movies[0].folder_modified_time == expected
    assert mtimes == {"m1": expected}

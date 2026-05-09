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


def _file(file_id: str, name: str, modified_time: str | None) -> CloudFile:
    return CloudFile(
        id=file_id,
        name=name,
        file_type=FileType.FILE,
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


def test_scan_tv_shows_uses_season_folder_mtime(monkeypatch):
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "Show One (2024)", "2026-04-29T03:25:21+00:00")],
        "show1": [
            _folder("season1", "Season 1", "2026-04-30T03:25:21+00:00"),
            _folder("season2", "Season 2", "2026-05-01T03:25:21+00:00"),
            _file("nfo1", "tvshow.nfo", "2026-05-02T03:25:21+00:00"),
        ],
        "season1": [_file("ep1", "Show.One.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
        "season2": [_file("ep2", "Show.One.S02E01.mkv", "2026-05-01T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "tmdb_get", lambda path: None)

    shows = library_data.scan_tv_shows(client, cfg)

    expected = int(datetime(2026, 5, 1, 3, 25, 21, tzinfo=timezone.utc).timestamp())
    assert len(shows) == 1
    assert shows[0].folder_modified_time == expected


def test_scan_tv_shows_incremental_uses_season_folder_mtime(monkeypatch):
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "Show One (2024)", "2026-04-29T03:25:21+00:00")],
        "show1": [
            _folder("season1", "Season 1", "2026-04-30T03:25:21+00:00"),
            _file("nfo1", "tvshow.nfo", "2026-05-02T03:25:21+00:00"),
        ],
        "season1": [_file("ep1", "Show.One.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "tmdb_get", lambda path: None)

    shows, mtimes = library_data.scan_tv_shows_incremental(client, cfg, {})

    expected = int(datetime(2026, 4, 30, 3, 25, 21, tzinfo=timezone.utc).timestamp())
    assert len(shows) == 1
    assert shows[0].folder_modified_time == expected
    assert mtimes == {"show1": expected}


def test_scan_tv_shows_incremental_ignores_tvshow_nfo_only_changes(monkeypatch):
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "Show One (2024)", "2026-04-29T03:25:21+00:00")],
        "show1": [
            _folder("season1", "Season 1", "2026-04-30T03:25:21+00:00"),
            _file("nfo1", "tvshow.nfo", "2026-05-02T03:25:21+00:00"),
        ],
        "season1": [_file("ep1", "Show.One.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "tmdb_get", lambda path: None)

    shows, mtimes = library_data.scan_tv_shows_incremental(client, cfg, {"show1": int(datetime(2026, 4, 30, 3, 25, 21, tzinfo=timezone.utc).timestamp())})

    assert shows == []
    assert mtimes == {"show1": int(datetime(2026, 4, 30, 3, 25, 21, tzinfo=timezone.utc).timestamp())}

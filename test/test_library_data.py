import os
import sys
from datetime import datetime, timezone


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if ROOT not in sys.path:
    sys.path.insert(0, ROOT)


from mediaparser.config import Config
from storage.base import CloudFile, FileType
from webui.ingest_store import IngestStore
from webui.library_store import LibraryStore
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


def test_scan_movies_restores_tmdb_from_library_store(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.save_snapshot(
        movies=[
            {
                "tmdb_id": 42,
                "title": "Movie One",
                "original_title": "Movie One",
                "year": "2024",
                "media_type": "movie",
                "overview": "cached overview",
                "rating": 7.5,
                "drive_folder_id": "m1",
                "folder_modified_time": 10,
            }
        ],
        tv_shows=[],
    )
    store.upsert_tmdb_detail(
        media_type="movie",
        tmdb_id=42,
        language="zh-CN",
        data={
            "tmdb_id": 42,
            "title": "Movie One",
            "original_title": "Movie One",
            "release_date": "2024-01-01",
            "overview": "cached overview",
            "vote_average": 7.5,
        },
        expires_at=9999999999,
    )
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"movie_root_id": "movies"},
    })
    client = FakeStorageProvider({
        "movies": [_folder("m1", "Movie One (2024)", "2026-04-29T03:25:21+00:00")],
        "m1": [],
    })
    monkeypatch.setattr(library_data, "get_library_store", lambda: store)
    monkeypatch.setattr(library_data, "get_ingest_store", lambda: IngestStore(str(db_path)))
    monkeypatch.setattr(library_data, "tmdb_get", lambda path: None)

    movies = library_data.scan_movies(client, cfg)

    assert len(movies) == 1
    assert movies[0].tmdb_id == 42
    assert movies[0].title == "Movie One"
    assert movies[0].overview == "cached overview"


def test_scan_tv_shows_restores_tmdb_from_ingest_history(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    ingest_store = IngestStore(str(db_path))
    ingest_store.record_ingest(
        media_type="tv",
        tmdb_id=99,
        title="Show One",
        year="2024",
        drive_folder_id="old-show-id",
        status="success",
    )
    store.upsert_tmdb_detail(
        media_type="tv",
        tmdb_id=99,
        language="zh-CN",
        data={
            "tmdb_id": 99,
            "name": "Show One",
            "original_name": "Show One",
            "first_air_date": "2024-02-02",
            "overview": "tv cached overview",
            "vote_average": 8.1,
            "status": "Returning Series",
            "seasons": [{"season_number": 1, "name": "Season 1", "episode_count": 1}],
        },
        expires_at=9999999999,
    )
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "Show One (2024)", "2026-04-29T03:25:21+00:00")],
        "show1": [
            _folder("season1", "Season 1", "2026-04-30T03:25:21+00:00"),
        ],
        "season1": [_file("ep1", "Show.One.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "get_library_store", lambda: store)
    monkeypatch.setattr(library_data, "get_ingest_store", lambda: ingest_store)
    monkeypatch.setattr(
        library_data,
        "tmdb_get",
        lambda path: {"episodes": [{"episode_number": 1, "name": "Episode 1", "air_date": "2024-02-02"}]}
        if path == "/tv/99/season/1"
        else None,
    )

    shows = library_data.scan_tv_shows(client, cfg)

    assert len(shows) == 1
    assert shows[0].tmdb_id == 99
    assert shows[0].title == "Show One"
    assert shows[0].overview == "tv cached overview"
    assert shows[0].in_library_episodes == 1


def test_scan_tv_shows_matches_long_subtitle_to_tmdb_alias(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.upsert_tmdb_detail(
        media_type="tv",
        tmdb_id=777,
        language="zh-CN",
        data={
            "tmdb_id": 777,
            "name": "使人误解的工房主",
            "original_name": "Kanchigai no Atelier Meister",
            "first_air_date": "2026-01-01",
            "overview": "overview",
            "vote_average": 7.9,
            "status": "Returning Series",
            "names": ["使人误解的工房主"],
            "seasons": [{"season_number": 1, "name": "Season 1", "episode_count": 1}],
        },
        expires_at=9999999999,
    )
    store.save_snapshot(
        movies=[],
        tv_shows=[
            {
                "tmdb_id": 777,
                "title": "使人误解的工房主",
                "original_title": "Kanchigai no Atelier Meister",
                "year": "2026",
                "media_type": "tv",
                "overview": "overview",
                "rating": 7.9,
                "drive_folder_id": "old-show-id",
                "folder_modified_time": 10,
            }
        ],
    )
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "使人误解的工房主～关于原英雄队伍的杂役人员，实际上除了战斗能力外全是SSS的故事～ (2026)", "2026-04-29T03:25:21+00:00")],
        "show1": [_folder("season1", "Season 1", "2026-04-30T03:25:21+00:00")],
        "season1": [_file("ep1", "Show.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "get_library_store", lambda: store)
    monkeypatch.setattr(library_data, "get_ingest_store", lambda: IngestStore(str(db_path)))
    monkeypatch.setattr(
        library_data,
        "tmdb_get",
        lambda path: {"episodes": [{"episode_number": 1, "name": "Episode 1", "air_date": "2026-01-02"}]}
        if path == "/tv/777/season/1"
        else None,
    )

    shows = library_data.scan_tv_shows(client, cfg)

    assert len(shows) == 1
    assert shows[0].tmdb_id == 777
    assert shows[0].title == "使人误解的工房主"


def test_scan_tv_shows_restores_from_tmdb_cache_without_history(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.upsert_tmdb_detail(
        media_type="tv",
        tmdb_id=80867,
        language="zh-CN",
        data={
            "tmdb_id": 80867,
            "name": "一脸嫌弃表情的妹子给你看胖次",
            "original_name": "嫌な顔されながらおパンツ見せてもらいたい",
            "first_air_date": "2018-07-14",
            "overview": "overview",
            "vote_average": 7.1,
            "status": "Ended",
            "names": ["一脸嫌弃表情的妹子给你看胖次"],
            "seasons": [{"season_number": 1, "name": "Season 1", "episode_count": 1}],
        },
        expires_at=9999999999,
    )
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "tmdb": {"api_key": "fake-api-key"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "一脸嫌弃表情的妹子给你看胖次 (2018)", "2026-04-29T03:25:21+00:00")],
        "show1": [_folder("season1", "Season 1", "2026-04-30T03:25:21+00:00")],
        "season1": [_file("ep1", "Show.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "get_library_store", lambda: store)
    monkeypatch.setattr(library_data, "get_ingest_store", lambda: IngestStore(str(db_path)))
    monkeypatch.setattr(
        library_data,
        "tmdb_get",
        lambda path: {"episodes": [{"episode_number": 1, "name": "Episode 1", "air_date": "2018-07-15"}]}
        if path == "/tv/80867/season/1"
        else None,
    )

    shows = library_data.scan_tv_shows(client, cfg)

    assert len(shows) == 1
    assert shows[0].tmdb_id == 80867
    assert shows[0].title == "一脸嫌弃表情的妹子给你看胖次"


def test_scan_tv_shows_searches_tmdb_when_local_history_missing(monkeypatch):
    cfg = Config.from_dict({
        "storage": {"primary": "pan115"},
        "tmdb": {"api_key": "fake-api-key"},
        "u115": {"tv_root_id": "tv"},
    })
    client = FakeStorageProvider({
        "tv": [_folder("show1", "公主殿下，“拷问”的时间到了 (2024)", "2026-04-29T03:25:21+00:00")],
        "show1": [_folder("season1", "Season 1", "2026-04-30T03:25:21+00:00")],
        "season1": [_file("ep1", "Show.S01E01.mkv", "2026-04-30T03:25:21+00:00")],
    })
    monkeypatch.setattr(library_data, "get_library_store", lambda: LibraryStore(":memory:"))
    monkeypatch.setattr(library_data, "get_ingest_store", lambda: IngestStore(":memory:"))
    monkeypatch.setattr(
        library_data,
        "_search_tmdb_identity",
        lambda media_type, title, year, cfg: (
            222222,
            {
                "tmdb_id": 222222,
                "name": "公主殿下，“拷问”的时间到了",
                "original_name": "姫様“拷問”の時間です",
                "first_air_date": "2024-01-09",
                "overview": "overview",
                "vote_average": 7.8,
                "status": "Ended",
                "seasons": [{"season_number": 1, "name": "Season 1", "episode_count": 1}],
            },
        ),
    )
    monkeypatch.setattr(
        library_data,
        "tmdb_get",
        lambda path: {"episodes": [{"episode_number": 1, "name": "Episode 1", "air_date": "2024-01-10"}]}
        if path == "/tv/222222/season/1"
        else None,
    )

    shows = library_data.scan_tv_shows(client, cfg)

    assert len(shows) == 1
    assert shows[0].tmdb_id == 222222


def test_fill_seasons_episodes_preserves_in_library_flags(monkeypatch):
    monkeypatch.setattr(
        library_data,
        "tmdb_get",
        lambda path: {
            "episodes": [
                {"episode_number": 1, "name": "Episode 1", "air_date": "2024-01-01"},
                {"episode_number": 2, "name": "Episode 2", "air_date": "2024-01-08"},
            ]
        },
    )

    seasons = library_data.fill_seasons_episodes(
        123,
        [
            {
                "season_number": 1,
                "season_name": "Season 1",
                "episode_count": 2,
                "in_library_count": 2,
                "episodes": [
                    {"episode_number": 1, "episode_title": "old 1", "air_date": "", "in_library": True},
                    {"episode_number": 2, "episode_title": "old 2", "air_date": "", "in_library": True},
                ],
            }
        ],
    )

    assert seasons[0]["in_library_count"] == 2
    assert seasons[0]["episodes"][0]["in_library"] is True
    assert seasons[0]["episodes"][1]["in_library"] is True

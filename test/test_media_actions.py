import os
import sys
from types import SimpleNamespace


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if ROOT not in sys.path:
    sys.path.insert(0, ROOT)


requests_mod = sys.modules.get("requests")
if requests_mod is not None and not hasattr(requests_mod, "Response"):
    requests_mod.Response = SimpleNamespace


from mediaparser.config import Config
from storage.base import CloudFile, FileType
from webui.library_store import LibraryStore
from webui.services import library_data, media_actions


class FakeStorageProvider:
    def __init__(self):
        self.uploaded = []
        self.renamed = []
        self.files = {
            "show-1": CloudFile(id="show-1", name="Show One (2024)", file_type=FileType.FOLDER),
            "movie-1": CloudFile(id="movie-1", name="Wrong Movie (2020)", file_type=FileType.FOLDER),
        }
        self.folders = {
            "show-1": [
                CloudFile(id="season-1", name="Season 1", file_type=FileType.FOLDER),
            ],
            "season-1": [
                CloudFile(id="ep-1", name="Show.One.S01E01.mkv", file_type=FileType.FILE),
            ],
            "movie-1": [
                CloudFile(id="video-1", name="Wrong.Movie.2020.mkv", file_type=FileType.FILE),
            ],
        }

    def list_files(self, folder_id=None, page_size=100):
        return list(self.folders.get(folder_id or "", []))

    def get_file(self, file_id):
        return self.files[file_id]

    def rename_file(self, file_id, new_name):
        current = self.files[file_id]
        renamed = CloudFile(id=current.id, name=new_name, file_type=current.file_type)
        self.files[file_id] = renamed
        self.renamed.append((file_id, new_name))
        return renamed

    def upload_text(self, content, name, parent_id=None, mime_type=None, overwrite=False):
        self.uploaded.append(("text", name, parent_id))


def test_do_refresh_item_skip_metadata_upload_only_updates_cache(monkeypatch):
    cfg = Config.from_dict({
        "tmdb": {"api_key": "fake-api-key"},
        "pipeline": {"skip_metadata_upload": True},
    })
    client = FakeStorageProvider()

    class FakeImageUploader:
        def __init__(self, *args, **kwargs):
            self.calls = []

        def upload_poster(self, poster_path, folder_id):
            self.calls.append(("poster", poster_path, folder_id))

        def upload_fanart(self, poster_path, folder_id):
            self.calls.append(("fanart", poster_path, folder_id))

        def upload_season_poster(self, poster_path, season_num, folder_id):
            self.calls.append(("season", poster_path, season_num, folder_id))

    monkeypatch.setattr(media_actions, "get_config", lambda: cfg)
    monkeypatch.setattr(media_actions, "get_storage_provider", lambda: client)
    monkeypatch.setattr(media_actions, "ImageUploader", FakeImageUploader)
    def fake_tmdb_get(path, *args, **kwargs):
        return (
            {
                "id": 99,
                "name": "Show One",
                "original_name": "Show One",
                "first_air_date": "2024-01-01",
                "overview": "overview",
                "vote_average": 8.2,
                "status": "Returning Series",
                "number_of_episodes": 1,
                "poster_path": "/poster.jpg",
                "backdrop_path": "/backdrop.jpg",
                "credits": {"crew": [], "cast": []},
                "seasons": [{"season_number": 1, "name": "Season 1", "episode_count": 1}],
            }
            if path == "/tv/99"
            else {"episodes": [{"episode_number": 1, "name": "Episode 1", "air_date": "2024-01-02"}]}
        )

    monkeypatch.setattr(media_actions, "tmdb_get", fake_tmdb_get)
    monkeypatch.setattr(library_data, "tmdb_get", fake_tmdb_get)

    result = media_actions.do_refresh_item(99, "tv", "show-1")

    assert result["ok"] is True
    assert result["uploaded"] == []
    assert result["errors"] == []
    assert result["updates"]["tmdb_id"] == 99
    assert result["updates"]["title"] == "Show One"
    assert result["updates"]["poster_url"]
    assert client.uploaded == []


def test_do_refresh_item_bypasses_cached_season_detail(monkeypatch):
    cfg = Config.from_dict({
        "tmdb": {"api_key": "fake-api-key"},
        "pipeline": {"skip_metadata_upload": True},
    })
    client = FakeStorageProvider()
    client.folders["season-1"] = [
        CloudFile(id=f"ep-{i}", name=f"Show.One.S01E{i:02d}.mkv", file_type=FileType.FILE)
        for i in range(1, 7)
    ]

    def fake_tmdb_get(path, *args, **kwargs):
        if path == "/tv/99":
            return {
                "id": 99,
                "name": "Show One",
                "original_name": "Show One",
                "first_air_date": "2024-01-01",
                "overview": "overview",
                "vote_average": 8.2,
                "status": "Returning Series",
                "number_of_episodes": 12,
                "credits": {"crew": [], "cast": []},
                "seasons": [{"season_number": 1, "name": "Season 1", "episode_count": 12}],
            }
        if path == "/tv/99/season/1":
            assert kwargs.get("use_cache") is False
            return {
                "episodes": [
                    {"episode_number": i, "name": f"Episode {i}", "air_date": "2024-01-02"}
                    for i in range(1, 13)
                ]
            }
        return None

    monkeypatch.setattr(media_actions, "get_config", lambda: cfg)
    monkeypatch.setattr(media_actions, "get_storage_provider", lambda: client)
    monkeypatch.setattr(media_actions, "tmdb_get", fake_tmdb_get)
    monkeypatch.setattr(library_data, "tmdb_get", fake_tmdb_get)

    result = media_actions.do_refresh_item(99, "tv", "show-1")

    assert result["updates"]["total_episodes"] == 12
    assert result["updates"]["in_library_episodes"] == 6
    season = result["updates"]["seasons"][0]
    assert season["episode_count"] == 12
    assert len(season["episodes"]) == 12
    assert sum(1 for ep in season["episodes"] if ep["in_library"]) == 6


def test_tmdb_detail_payload_refills_inconsistent_seasons(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.save_snapshot(
        movies=[],
        tv_shows=[
            {
                "tmdb_id": 99,
                "title": "Show One",
                "original_title": "Show One",
                "year": "2024",
                "media_type": "tv",
                "overview": "",
                "rating": 0.0,
                "drive_folder_id": "show-1",
                "total_episodes": 12,
                "in_library_episodes": 6,
                "seasons": [
                    {
                        "season_number": 1,
                        "season_name": "Season 1",
                        "poster_url": None,
                        "episode_count": 6,
                        "in_library_count": 6,
                        "episodes": [
                            {
                                "episode_number": i,
                                "episode_title": f"Episode {i}",
                                "air_date": "",
                                "in_library": True,
                            }
                            for i in range(1, 7)
                        ],
                    }
                ],
            }
        ],
    )

    def fake_fill(tmdb_id, seasons, tmdb_use_cache=True):
        assert tmdb_id == 99
        assert tmdb_use_cache is False
        existing_flags = {
            ep["episode_number"]: ep["in_library"]
            for season in seasons
            for ep in season.get("episodes", [])
        }
        return [
            {
                "season_number": 1,
                "season_name": "Season 1",
                "poster_url": None,
                "episode_count": 12,
                "in_library_count": 6,
                "episodes": [
                    {
                        "episode_number": i,
                        "episode_title": f"Episode {i}",
                        "air_date": "",
                        "in_library": existing_flags.get(i, False),
                    }
                    for i in range(1, 13)
                ],
            }
        ]

    monkeypatch.setattr(media_actions, "get_library_store", lambda: store)
    monkeypatch.setattr(media_actions, "fill_seasons_episodes", fake_fill)

    result = media_actions.tmdb_detail_payload(99, "tv")

    season = result["detail"]["seasons"][0]
    assert season["episode_count"] == 12
    assert len(season["episodes"]) == 12
    assert sum(1 for ep in season["episodes"] if ep["in_library"]) == 6


def test_reidentify_item_payload_updates_library_binding(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.save_snapshot(
        movies=[
            {
                "tmdb_id": 10,
                "title": "Wrong Movie",
                "original_title": "Wrong Movie",
                "year": "2020",
                "media_type": "movie",
                "overview": "",
                "rating": 0.0,
                "drive_folder_id": "movie-1",
                "folder_modified_time": 0,
            }
        ],
        tv_shows=[],
    )
    cfg = Config.from_dict({
        "tmdb": {"api_key": "fake-api-key"},
        "pipeline": {"skip_metadata_upload": True},
    })
    client = FakeStorageProvider()

    monkeypatch.setattr(media_actions, "get_config", lambda: cfg)
    monkeypatch.setattr(media_actions, "get_storage_provider", lambda: client)
    monkeypatch.setattr(media_actions, "get_library_store", lambda: store)
    monkeypatch.setattr(
        media_actions,
        "tmdb_get",
        lambda path, *args, **kwargs: {
            "id": 20,
            "title": "Correct Movie",
            "original_title": "Correct Movie",
            "release_date": "2024-04-05",
            "overview": "fixed overview",
            "vote_average": 8.8,
            "poster_path": "/poster.jpg",
            "backdrop_path": "/backdrop.jpg",
            "credits": {"crew": [], "cast": []},
        } if path == "/movie/20" else None,
    )

    body = SimpleNamespace(
        tmdb_id=20,
        media_type="movie",
        drive_folder_id="movie-1",
        title="Wrong Movie",
        year="2020",
        rename_folder=True,
    )
    result = media_actions.reidentify_item_payload(body)

    assert result["ok"] is True
    assert result["partial"] is False
    assert result["renamed"] is True
    assert result["folder_name"] == "Correct Movie (2024)"
    assert result["item"]["tmdb_id"] == 20
    assert result["item"]["title"] == "Correct Movie"
    assert client.renamed == [("movie-1", "Correct Movie (2024)")]

    updated = store.get_library_item_by_folder_id("movie-1")
    assert updated["tmdb_id"] == 20
    assert updated["title"] == "Correct Movie"


def test_reidentify_item_payload_keeps_metadata_when_rename_fails(tmp_path, monkeypatch):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.save_snapshot(
        movies=[
            {
                "tmdb_id": 10,
                "title": "Wrong Movie",
                "original_title": "Wrong Movie",
                "year": "2020",
                "media_type": "movie",
                "overview": "",
                "rating": 0.0,
                "drive_folder_id": "movie-1",
                "folder_modified_time": 0,
            }
        ],
        tv_shows=[],
    )
    cfg = Config.from_dict({
        "tmdb": {"api_key": "fake-api-key"},
        "pipeline": {"skip_metadata_upload": True},
    })
    client = FakeStorageProvider()

    def fail_rename(file_id, new_name):
        raise RuntimeError("rename blocked")

    monkeypatch.setattr(client, "rename_file", fail_rename)
    monkeypatch.setattr(media_actions, "get_config", lambda: cfg)
    monkeypatch.setattr(media_actions, "get_storage_provider", lambda: client)
    monkeypatch.setattr(media_actions, "get_library_store", lambda: store)
    monkeypatch.setattr(
        media_actions,
        "tmdb_get",
        lambda path, *args, **kwargs: {
            "id": 20,
            "title": "Correct Movie",
            "original_title": "Correct Movie",
            "release_date": "2024-04-05",
            "overview": "fixed overview",
            "vote_average": 8.8,
            "poster_path": "/poster.jpg",
            "backdrop_path": "/backdrop.jpg",
            "credits": {"crew": [], "cast": []},
        } if path == "/movie/20" else None,
    )

    body = SimpleNamespace(
        tmdb_id=20,
        media_type="movie",
        drive_folder_id="movie-1",
        title="Wrong Movie",
        year="2020",
        rename_folder=True,
    )
    result = media_actions.reidentify_item_payload(body)

    assert result["ok"] is False
    assert result["partial"] is True
    assert result["rename_errors"] == ["rename blocked"]
    assert result["item"]["tmdb_id"] == 20
    assert result["item"]["title"] == "Correct Movie"

    updated = store.get_library_item_by_folder_id("movie-1")
    assert updated["tmdb_id"] == 20
    assert updated["title"] == "Correct Movie"

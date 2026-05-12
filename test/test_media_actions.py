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
from webui.services import media_actions


class FakeStorageProvider:
    def __init__(self):
        self.uploaded = []
        self.folders = {
            "show-1": [
                CloudFile(id="season-1", name="Season 1", file_type=FileType.FOLDER),
            ],
            "season-1": [
                CloudFile(id="ep-1", name="Show.One.S01E01.mkv", file_type=FileType.FILE),
            ],
        }

    def list_files(self, folder_id=None, page_size=100):
        return list(self.folders.get(folder_id or "", []))

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
    monkeypatch.setattr(
        media_actions,
        "tmdb_get",
        lambda path, *args, **kwargs: (
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
        ),
    )

    result = media_actions.do_refresh_item(99, "tv", "show-1")

    assert result["ok"] is True
    assert result["uploaded"] == []
    assert result["errors"] == []
    assert result["updates"]["tmdb_id"] == 99
    assert result["updates"]["title"] == "Show One"
    assert result["updates"]["poster_url"]
    assert client.uploaded == []

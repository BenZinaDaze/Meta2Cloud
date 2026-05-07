import os
import sqlite3
import sys


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if ROOT not in sys.path:
    sys.path.insert(0, ROOT)


from mediaparser.config import Config
from mediaparser.tmdb import TmdbClient
from mediaparser.tmdb_image import (
    DEFAULT_TMDB_IMAGE_BASE_URL,
    build_tmdb_image_url,
    extract_tmdb_image_path,
    normalize_tmdb_image_base_url,
)
from webui.core.runtime import get_config
from webui.library_store import LibraryStore
from webui.rss_subscription_store import RSSSubscriptionStore


def test_tmdb_image_base_url_defaults_when_config_empty():
    cfg = Config.from_dict({"tmdb": {"image_base_url": "  "}})
    assert cfg.tmdb_image_base_url == DEFAULT_TMDB_IMAGE_BASE_URL


def test_tmdb_image_base_url_strips_trailing_slash():
    cfg = Config.from_dict({"tmdb": {"image_base_url": "https://cdn.example.com/"}})
    assert cfg.tmdb_image_base_url == "https://cdn.example.com"


def test_build_tmdb_image_url_with_custom_base_url():
    assert build_tmdb_image_url(
        "/poster.jpg",
        size="w500",
        base_url="https://cdn.example.com/",
    ) == "https://cdn.example.com/t/p/w500/poster.jpg"


def test_tmdb_client_image_url_uses_custom_base_url():
    assert TmdbClient.image_url(
        "/backdrop.jpg",
        base_url="https://cdn.example.com",
    ) == "https://cdn.example.com/t/p/original/backdrop.jpg"


def test_normalize_tmdb_image_base_url_falls_back_to_default():
    assert normalize_tmdb_image_base_url("") == DEFAULT_TMDB_IMAGE_BASE_URL


def test_extract_tmdb_image_path_from_full_url():
    assert extract_tmdb_image_path("https://proxy.example.com/t/p/w500/poster.jpg") == "/poster.jpg"


def test_library_store_persists_relative_image_paths(tmp_path):
    db_path = tmp_path / "library.db"
    store = LibraryStore(str(db_path))
    store.save_snapshot(
        movies=[
            {
                "tmdb_id": 1,
                "title": "Movie",
                "original_title": "Movie",
                "year": "2024",
                "media_type": "movie",
                "poster_url": "https://proxy.example.com/t/p/w500/poster.jpg",
                "backdrop_url": "https://proxy.example.com/t/p/original/backdrop.jpg",
                "overview": "",
                "rating": 0.0,
                "drive_folder_id": "folder-1",
                "folder_modified_time": 0,
            }
        ],
        tv_shows=[],
    )

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    row = conn.execute(
        "SELECT poster_url, backdrop_url, raw_json FROM library_media WHERE drive_folder_id = ?",
        ("folder-1",),
    ).fetchone()
    assert row["poster_url"] == "/poster.jpg"
    assert row["backdrop_url"] == "/backdrop.jpg"
    assert '"poster_url": "/poster.jpg"' in row["raw_json"]
    assert '"backdrop_url": "/backdrop.jpg"' in row["raw_json"]

    hydrated = store.get_library_item_by_tmdb("movie", 1)
    image_base_url = get_config().tmdb_image_base_url
    assert hydrated["poster_url"] == f"{image_base_url}/t/p/w500/poster.jpg"
    assert hydrated["backdrop_url"] == f"{image_base_url}/t/p/original/backdrop.jpg"


def test_rss_subscription_store_persists_relative_poster_path(tmp_path):
    db_path = tmp_path / "subscriptions.db"
    store = RSSSubscriptionStore(str(db_path))
    record = store.create_subscription(
        {
            "name": "Test",
            "media_title": "Show",
            "media_type": "tv",
            "tmdb_id": 1,
            "poster_url": "https://proxy.example.com/t/p/w500/poster.jpg",
            "site": "mikan",
            "rss_url": "https://example.com/feed.xml",
            "subgroup_name": "",
            "season_number": 1,
            "start_episode": 1,
            "keyword_all": "[]",
            "push_target": "aria2",
            "enabled": True,
        }
    )

    assert record.poster_url == "/poster.jpg"

    conn = sqlite3.connect(db_path)
    conn.row_factory = sqlite3.Row
    row = conn.execute(
        "SELECT poster_url FROM rss_subscriptions WHERE id = ?",
        (record.id,),
    ).fetchone()
    assert row["poster_url"] == "/poster.jpg"
